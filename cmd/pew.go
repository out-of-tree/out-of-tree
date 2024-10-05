// Copyright 2023 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package cmd

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/google/uuid"
	"github.com/remeh/sizedwaitgroup"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gopkg.in/logrusorgru/aurora.v2"

	"code.dumpstack.io/tools/out-of-tree/api"
	"code.dumpstack.io/tools/out-of-tree/artifact"
	"code.dumpstack.io/tools/out-of-tree/client"
	"code.dumpstack.io/tools/out-of-tree/config"
	"code.dumpstack.io/tools/out-of-tree/distro"
	"code.dumpstack.io/tools/out-of-tree/qemu"
)

const pathDevNull = "/dev/null"

type LevelWriter struct {
	io.Writer
	Level zerolog.Level
}

func (lw *LevelWriter) WriteLevel(l zerolog.Level, p []byte) (n int, err error) {
	if l >= lw.Level {
		return lw.Writer.Write(p)
	}
	return len(p), nil
}

var ConsoleWriter, FileWriter LevelWriter

var LogLevel zerolog.Level

type runstate struct {
	Overall, Success float64
	InternalErrors   int
}

var (
	state runstate
)

func successRate(state runstate) float64 {
	return state.Success / state.Overall
}

type PewCmd struct {
	Max     int64         `help:"test no more than X kernels" default:"100500"`
	Runs    int64         `help:"runs per each kernel" default:"1"`
	Kernel  string        `help:"override kernel regex"`
	RootFS  string        `help:"override rootfs image" type:"existingfile"`
	Guess   bool          `help:"try all defined kernels"`
	Shuffle bool          `help:"randomize kernels test order"`
	Binary  string        `help:"use binary, do not build"`
	Test    string        `help:"override path for test"`
	Dist    string        `help:"build result path" default:"/dev/null"`
	Threads int           `help:"threads" default:"1"`
	Tag     string        `help:"log tagging"`
	Timeout time.Duration `help:"timeout after tool will not spawn new tests"`

	ArtifactConfig string `help:"path to artifact config" type:"path"`

	QemuTimeout           time.Duration `help:"timeout for qemu"`
	QemuAfterStartTimeout time.Duration `help:"timeout after starting of the qemu vm before tests"`
	DockerTimeout         time.Duration `help:"timeout for docker"`

	Threshold             float64 `help:"reliablity threshold for exit code" default:"1.00"`
	IncludeInternalErrors bool    `help:"count internal errors as part of the success rate"`

	Endless        bool          `help:"endless tests"`
	EndlessTimeout time.Duration `help:"timeout between tests" default:"1m"`
	EndlessStress  string        `help:"endless stress script" type:"existingfile"`

	DB              *sql.DB             `kong:"-" json:"-"`
	Kcfg            config.KernelConfig `kong:"-" json:"-"`
	TimeoutDeadline time.Time           `kong:"-" json:"-"`

	Watch bool `help:"watch job status"`

	repoName string
	commit   string

	useRemote  bool
	remoteAddr string

	// UUID of the job set
	groupUUID string
}

func (cmd *PewCmd) getRepoName(worktree string, ka artifact.Artifact) {
	raw, err := exec.Command("git", "--work-tree="+worktree,
		"rev-list", "--max-parents=0", "HEAD").CombinedOutput()
	if err != nil {
		log.Error().Err(err).Msg(string(raw))
		return
	}

	cmd.repoName = fmt.Sprintf("%s-%s", ka.Name, string(raw[:7]))
}

func (cmd *PewCmd) syncRepo(worktree string, ka artifact.Artifact) (err error) {
	c := client.Client{RemoteAddr: cmd.remoteAddr}

	cmd.getRepoName(worktree, ka)

	raw, err := exec.Command("git", "--work-tree="+worktree,
		"rev-parse", "HEAD").CombinedOutput()
	if err != nil {
		return
	}
	cmd.commit = strings.TrimSuffix(string(raw), "\n")

	_, err = c.GetRepo(cmd.repoName)
	if err != nil && err != client.ErrRepoNotFound {
		log.Error().Err(err).Msg("GetRepo API error")
		return
	}

	if err == client.ErrRepoNotFound {
		log.Warn().Msg("repo not found")
		log.Info().Msg("add repo")
		log.Warn().Msgf("%v", spew.Sdump(ka))
		err = c.AddRepo(api.Repo{Name: cmd.repoName})
		if err != nil {
			return
		}
	}

	err = c.PushRepo(api.Repo{Name: cmd.repoName, Path: worktree})
	if err != nil {
		log.Error().Err(err).Msg("push repo error")
		return
	}

	return
}

func (cmd *PewCmd) Run(g *Globals) (err error) {
	cmd.groupUUID = uuid.New().String()
	log.Info().Str("group", cmd.groupUUID).Msg("")
	cmd.useRemote = g.Remote
	cmd.remoteAddr = g.RemoteAddr

	if cmd.useRemote {
		c := client.Client{RemoteAddr: cmd.remoteAddr}
		cmd.Kcfg.Kernels, err = c.Kernels()
		if err != nil {
			log.Fatal().Err(err).Msg("read kernels config")
		}
	} else {
		cmd.Kcfg, err = config.ReadKernelConfig(
			g.Config.Kernels)
		if err != nil {
			log.Fatal().Err(err).Msg("read kernels config")
		}
	}

	if cmd.Timeout != 0 {
		log.Info().Msgf("Set global timeout to %s", cmd.Timeout)
		cmd.TimeoutDeadline = time.Now().Add(cmd.Timeout)
	}

	cmd.DB, err = openDatabase(g.Config.Database)
	if err != nil {
		log.Fatal().Err(err).
			Msgf("Cannot open database %s", g.Config.Database)
	}
	defer cmd.DB.Close()

	var configPath string
	if cmd.ArtifactConfig == "" {
		configPath = g.WorkDir + "/.out-of-tree.toml"
	} else {
		configPath = cmd.ArtifactConfig
	}

	ka, err := artifact.Artifact{}.Read(configPath)
	if err != nil {
		return
	}

	if cmd.useRemote {
		err = cmd.syncRepo(g.WorkDir, ka)
		if err != nil {
			return
		}
	}

	if len(ka.Targets) == 0 || cmd.Guess {
		log.Debug().Msg("will use all available targets")

		for _, dist := range distro.List() {
			ka.Targets = append(ka.Targets, artifact.Target{
				Distro: dist,
				Kernel: artifact.Kernel{
					Regex: ".*",
				},
			})
		}
	}

	if ka.SourcePath == "" {
		ka.SourcePath = g.WorkDir
	}

	if cmd.Kernel != "" {
		var km artifact.Target
		km, err = kernelMask(cmd.Kernel)
		if err != nil {
			return
		}

		ka.Targets = []artifact.Target{km}
	}

	if ka.Qemu.Timeout.Duration == 0 {
		ka.Qemu.Timeout.Duration = g.Config.Qemu.Timeout.Duration
	}

	if ka.Docker.Timeout.Duration == 0 {
		ka.Docker.Timeout.Duration = g.Config.Docker.Timeout.Duration
	}

	if cmd.QemuTimeout != 0 {
		log.Info().Msgf("Set qemu timeout to %s", cmd.QemuTimeout)
		g.Config.Qemu.Timeout.Duration = cmd.QemuTimeout
		ka.Qemu.Timeout.Duration = cmd.QemuTimeout
	}

	if cmd.DockerTimeout != 0 {
		log.Info().Msgf("Set docker timeout to %s", cmd.DockerTimeout)
		g.Config.Docker.Timeout.Duration = cmd.DockerTimeout
		ka.Docker.Timeout.Duration = cmd.DockerTimeout
	}

	if cmd.Tag == "" {
		cmd.Tag = fmt.Sprintf("%d", time.Now().Unix())
	}
	if !cmd.useRemote {
		log.Info().Str("tag", cmd.Tag).Msg("")
	}

	err = cmd.performCI(ka)
	if err != nil {
		return
	}

	if cmd.useRemote {
		return
	}

	if state.InternalErrors > 0 {
		s := "not counted towards success rate"
		if cmd.IncludeInternalErrors {
			s = "included in success rate"
		}
		log.Warn().Msgf("%d internal errors "+
			"(%s)", state.InternalErrors, s)
	}

	if cmd.IncludeInternalErrors {
		state.Overall += float64(state.InternalErrors)
	}

	msg := fmt.Sprintf("Success rate: %.02f (%d/%d), Threshold: %.02f",
		successRate(state),
		int(state.Success), int(state.Overall),
		cmd.Threshold)

	if successRate(state) < cmd.Threshold {
		log.Error().Msg(msg)
		err = errors.New("reliability threshold not met")
	} else {
		log.Info().Msg(msg)
	}

	return
}

func (cmd PewCmd) watchJob(swg *sizedwaitgroup.SizedWaitGroup,
	slog zerolog.Logger, uuid string) {

	defer swg.Done() // FIXME

	c := client.Client{RemoteAddr: cmd.remoteAddr}

	var err error
	var st api.Status

	for {
		st, err = c.JobStatus(uuid)
		if err != nil {
			slog.Error().Err(err).Msg("")
			continue
		}
		if st == api.StatusSuccess || st == api.StatusFailure {
			break
		}

		time.Sleep(time.Second)
	}

	switch st {
	case api.StatusSuccess:
		slog.Info().Msg("success")
	case api.StatusFailure:
		slog.Warn().Msg("failure")
	}
}

func (cmd PewCmd) remote(swg *sizedwaitgroup.SizedWaitGroup,
	ka artifact.Artifact, ki distro.KernelInfo) {

	defer swg.Done()

	slog := log.With().
		Str("distro_type", ki.Distro.ID.String()).
		Str("distro_release", ki.Distro.Release).
		Str("kernel", ki.KernelRelease).
		Logger()

	job := api.Job{}
	job.Group = cmd.groupUUID
	job.RepoName = cmd.repoName
	job.Commit = cmd.commit

	job.Artifact = ka
	job.Target = ki

	c := client.Client{RemoteAddr: cmd.remoteAddr}
	uuid, err := c.AddJob(job)
	slog = slog.With().Str("uuid", uuid).Logger()
	if err != nil {
		slog.Error().Err(err).Msg("cannot add job")
		return
	}

	slog.Info().Msg("add")

	if cmd.Watch {
		// FIXME dummy (almost)
		go cmd.watchJob(swg, slog, uuid)
	}
}

func (cmd PewCmd) testArtifact(swg *sizedwaitgroup.SizedWaitGroup,
	ka artifact.Artifact, ki distro.KernelInfo) {

	defer swg.Done()

	logdir := "logs/" + cmd.Tag
	err := os.MkdirAll(logdir, os.ModePerm)
	if err != nil {
		log.Error().Err(err).Msgf("mkdir %s", logdir)
		return
	}

	logfile := fmt.Sprintf("logs/%s/%s-%s-%s.log",
		cmd.Tag,
		ki.Distro.ID.String(),
		ki.Distro.Release,
		ki.KernelRelease,
	)
	f, err := os.Create(logfile)
	if err != nil {
		log.Error().Err(err).Msgf("create %s", logfile)
		return
	}
	defer f.Close()

	slog := zerolog.New(zerolog.MultiLevelWriter(
		&ConsoleWriter,
		&FileWriter,
		&zerolog.ConsoleWriter{
			Out: f,
			FieldsExclude: []string{
				"distro_release",
				"distro_type",
				"kernel",
			},
			NoColor: true,
		},
	))

	switch LogLevel {
	case zerolog.TraceLevel, zerolog.DebugLevel:
		slog = slog.With().Caller().Logger()
	}

	slog = slog.With().Timestamp().
		Str("distro_type", ki.Distro.ID.String()).
		Str("distro_release", ki.Distro.Release).
		Str("kernel", ki.KernelRelease).
		Logger()

	ka.Process(slog, ki,
		cmd.Endless, cmd.Binary, cmd.EndlessStress, cmd.EndlessTimeout,
		func(q *qemu.System, ka artifact.Artifact, ki distro.KernelInfo, result *artifact.Result) {
			dumpResult(q, ka, ki, result, cmd.Dist, cmd.Tag, cmd.Binary, cmd.DB)
		},
	)
}

func shuffleKernels(a []distro.KernelInfo) []distro.KernelInfo {
	// Fisher–Yates shuffle
	for i := len(a) - 1; i > 0; i-- {
		j := rand.Intn(i + 1)
		a[i], a[j] = a[j], a[i]
	}
	return a
}

func (cmd PewCmd) process(swg *sizedwaitgroup.SizedWaitGroup,
	ka artifact.Artifact, kernel distro.KernelInfo) {

	if cmd.useRemote {
		go cmd.remote(swg, ka, kernel)
	} else {
		go cmd.testArtifact(swg, ka, kernel)
	}
}

func (cmd PewCmd) performCI(ka artifact.Artifact) (err error) {
	found := false
	max := cmd.Max

	threadCounter := 0

	swg := sizedwaitgroup.New(cmd.Threads)
	if cmd.Shuffle {
		cmd.Kcfg.Kernels = shuffleKernels(cmd.Kcfg.Kernels)
	}
	for _, kernel := range cmd.Kcfg.Kernels {
		if max <= 0 {
			break
		}

		var supported bool
		supported, err = ka.Supported(kernel)
		if err != nil {
			return
		}

		if kernel.Blocklisted {
			log.Debug().Str("kernel", kernel.KernelVersion).
				Msgf("skip (blocklisted)")
			continue
		}

		if cmd.RootFS != "" {
			kernel.RootFS = cmd.RootFS
		}

		if supported {
			found = true
			max--
			for i := int64(0); i < cmd.Runs; i++ {
				if !cmd.TimeoutDeadline.IsZero() &&
					time.Now().After(cmd.TimeoutDeadline) {

					break
				}
				swg.Add()
				if threadCounter < cmd.Threads {
					time.Sleep(time.Second)
					threadCounter++
				}

				go cmd.process(&swg, ka, kernel)
			}
		}
	}
	swg.Wait()

	if !found {
		err = errors.New("no supported kernels found")
	}

	return
}

func kernelMask(kernel string) (km artifact.Target, err error) {
	parts := strings.Split(kernel, ":")
	if len(parts) != 2 {
		err = errors.New("kernel is not 'distroType:regex'")
		return
	}

	dt, err := distro.NewID(parts[0])
	if err != nil {
		return
	}

	km = artifact.Target{
		Distro: distro.Distro{ID: dt},
		Kernel: artifact.Kernel{Regex: parts[1]},
	}
	return
}

func genOkFail(name string, ok bool) (aurv aurora.Value) {
	s := " " + name
	if name == "" {
		s = ""
	}
	if ok {
		s += " SUCCESS "
		aurv = aurora.BgGreen(aurora.Black(s))
	} else {
		s += " FAILURE "
		aurv = aurora.BgRed(aurora.White(aurora.Bold(s)))
	}
	return
}

func dumpResult(q *qemu.System, ka artifact.Artifact, ki distro.KernelInfo,
	res *artifact.Result, dist, tag, binary string, db *sql.DB) {

	// TODO refactor

	if res.InternalError != nil {
		q.Log.Warn().Err(res.InternalError).
			Str("panic", fmt.Sprintf("%v", q.KernelPanic)).
			Str("timeout", fmt.Sprintf("%v", q.KilledByTimeout)).
			Msg("internal")
		res.InternalErrorString = res.InternalError.Error()
		state.InternalErrors += 1
	} else {
		colored := ""

		state.Overall += 1

		if res.Test.Ok {
			state.Success += 1
		}

		switch ka.Type {
		case artifact.KernelExploit:
			colored = aurora.Sprintf("%s %s",
				genOkFail("BUILD", res.Build.Ok),
				genOkFail("LPE", res.Test.Ok))
		case artifact.KernelModule:
			colored = aurora.Sprintf("%s %s %s",
				genOkFail("BUILD", res.Build.Ok),
				genOkFail("INSMOD", res.Run.Ok),
				genOkFail("TEST", res.Test.Ok))
		case artifact.Script:
			colored = aurora.Sprintf("%s",
				genOkFail("", res.Test.Ok))
		}

		additional := ""
		if q.KernelPanic {
			additional = "(panic)"
		} else if q.KilledByTimeout {
			additional = "(timeout)"
		}

		if additional != "" {
			q.Log.Info().Msgf("%v %v", colored, additional)
		} else {
			q.Log.Info().Msgf("%v", colored)
		}
	}

	err := addToLog(db, q, ka, ki, res, tag)
	if err != nil {
		q.Log.Warn().Err(err).Msgf("[db] addToLog (%v)", ka)
	}

	if binary == "" && dist != pathDevNull {
		err = os.MkdirAll(dist, os.ModePerm)
		if err != nil {
			log.Warn().Err(err).Msgf("os.MkdirAll (%v)", ka)
		}

		path := fmt.Sprintf("%s/%s-%s-%s", dist, ki.Distro.ID,
			ki.Distro.Release, ki.KernelRelease)
		if ka.Type != artifact.KernelExploit {
			path += ".ko"
		}

		err = artifact.CopyFile(res.BuildArtifact, path)
		if err != nil {
			log.Warn().Err(err).Msgf("copy file (%v)", ka)
		}
	}
}
