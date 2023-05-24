// Copyright 2023 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package main

import (
	"bufio"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/otiai10/copy"
	"github.com/remeh/sizedwaitgroup"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gopkg.in/logrusorgru/aurora.v2"

	"code.dumpstack.io/tools/out-of-tree/config"
	"code.dumpstack.io/tools/out-of-tree/container"
	"code.dumpstack.io/tools/out-of-tree/distro"
	"code.dumpstack.io/tools/out-of-tree/fs"
	"code.dumpstack.io/tools/out-of-tree/qemu"
)

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

	Threshold float64 `help:"reliablity threshold for exit code" default:"1.00"`

	Endless        bool          `help:"endless tests"`
	EndlessTimeout time.Duration `help:"timeout between tests" default:"1m"`
	EndlessStress  string        `help:"endless stress script" type:"existingfile"`

	db              *sql.DB
	kcfg            config.KernelConfig
	timeoutDeadline time.Time
}

func (cmd *PewCmd) Run(g *Globals) (err error) {
	cmd.kcfg, err = config.ReadKernelConfig(g.Config.Kernels)
	if err != nil {
		log.Fatal().Err(err).Msg("read kernels config")
	}

	if cmd.Timeout != 0 {
		log.Info().Msgf("Set global timeout to %s", cmd.Timeout)
		cmd.timeoutDeadline = time.Now().Add(cmd.Timeout)
	}

	cmd.db, err = openDatabase(g.Config.Database)
	if err != nil {
		log.Fatal().Err(err).
			Msgf("Cannot open database %s", g.Config.Database)
	}
	defer cmd.db.Close()

	var configPath string
	if cmd.ArtifactConfig == "" {
		configPath = g.WorkDir + "/.out-of-tree.toml"
	} else {
		configPath = cmd.ArtifactConfig
	}
	ka, err := config.ReadArtifactConfig(configPath)
	if err != nil {
		return
	}

	if len(ka.Targets) == 0 {
		log.Debug().Msg("no targets defined in .out-of-tree.toml, " +
			"will use all available")

		for _, dist := range distro.List() {
			ka.Targets = append(ka.Targets, config.Target{
				Distro: dist,
				Kernel: config.Kernel{
					Regex: ".*",
				},
			})
		}
	}

	if ka.SourcePath == "" {
		ka.SourcePath = g.WorkDir
	}

	if cmd.Kernel != "" {
		var km config.Target
		km, err = kernelMask(cmd.Kernel)
		if err != nil {
			return
		}

		ka.Targets = []config.Target{km}
	}

	if cmd.Guess {
		ka.Targets, err = genAllKernels()
		if err != nil {
			return
		}
	}

	if cmd.QemuTimeout != 0 {
		log.Info().Msgf("Set qemu timeout to %s", cmd.QemuTimeout)
	} else {
		cmd.QemuTimeout = g.Config.Qemu.Timeout.Duration
	}

	if cmd.DockerTimeout != 0 {
		log.Info().Msgf("Set docker timeout to %s", cmd.DockerTimeout)
	} else {
		cmd.DockerTimeout = g.Config.Docker.Timeout.Duration
	}

	if cmd.Tag == "" {
		cmd.Tag = fmt.Sprintf("%d", time.Now().Unix())
	}
	log.Info().Str("tag", cmd.Tag).Msg("")

	err = cmd.performCI(ka)
	if err != nil {
		return
	}

	log.Info().Msgf("Success rate: %.02f, Threshold: %.02f",
		successRate(state), cmd.Threshold)
	if successRate(state) < cmd.Threshold {
		err = errors.New("reliability threshold not met")
	}
	return
}

type runstate struct {
	Overall, Success float64
}

var (
	state runstate
)

func successRate(state runstate) float64 {
	return state.Success / state.Overall
}

const pathDevNull = "/dev/null"

func sh(workdir, command string) (output string, err error) {
	flog := log.With().
		Str("workdir", workdir).
		Str("command", command).
		Logger()

	cmd := exec.Command("sh", "-c", "cd "+workdir+" && "+command)

	flog.Debug().Msgf("%v", cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return
	}
	cmd.Stderr = cmd.Stdout

	err = cmd.Start()
	if err != nil {
		return
	}

	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			m := scanner.Text()
			output += m + "\n"
			flog.Trace().Str("stdout", m).Msg("")
		}
	}()

	err = cmd.Wait()

	if err != nil {
		e := fmt.Sprintf("%v %v output: %v", cmd, err, output)
		err = errors.New(e)
	}
	return
}

func applyPatches(src string, ka config.Artifact) (err error) {
	for i, patch := range ka.Patches {
		name := fmt.Sprintf("patch_%02d", i)

		path := src + "/" + name + ".diff"
		if patch.Source != "" && patch.Path != "" {
			err = errors.New("path and source are mutually exclusive")
			return
		} else if patch.Source != "" {
			err = os.WriteFile(path, []byte(patch.Source), 0644)
			if err != nil {
				return
			}
		} else if patch.Path != "" {
			err = copy.Copy(patch.Path, path)
			if err != nil {
				return
			}
		}

		if patch.Source != "" || patch.Path != "" {
			_, err = sh(src, "patch < "+path)
			if err != nil {
				return
			}
		}

		if patch.Script != "" {
			script := src + "/" + name + ".sh"
			err = os.WriteFile(script, []byte(patch.Script), 0755)
			if err != nil {
				return
			}
			_, err = sh(src, script)
			if err != nil {
				return
			}
		}
	}
	return
}

func build(flog zerolog.Logger, tmp string, ka config.Artifact,
	ki distro.KernelInfo, dockerTimeout time.Duration) (
	outdir, outpath, output string, err error) {

	target := fmt.Sprintf("%d", rand.Int())

	outdir = tmp + "/source"

	err = copy.Copy(ka.SourcePath, outdir)
	if err != nil {
		return
	}

	err = applyPatches(outdir, ka)
	if err != nil {
		return
	}

	outpath = outdir + "/" + target
	if ka.Type == config.KernelModule {
		outpath += ".ko"
	}

	if ki.KernelVersion == "" {
		ki.KernelVersion = ki.KernelRelease
	}

	kernel := "/lib/modules/" + ki.KernelVersion + "/build"
	if ki.KernelSource != "" {
		kernel = ki.KernelSource
	}

	buildCommand := "make KERNEL=" + kernel + " TARGET=" + target
	if ka.Make.Target != "" {
		buildCommand += " " + ka.Make.Target
	}

	if ki.ContainerName != "" {
		var c container.Container
		container.Timeout = dockerTimeout
		c, err = container.NewFromKernelInfo(ki)
		c.Log = flog
		if err != nil {
			log.Fatal().Err(err).Msg("container creation failure")
		}

		output, err = c.Run(outdir, []string{
			buildCommand + " && chmod -R 777 /work",
		})
	} else {
		cmd := exec.Command("bash", "-c", "cd "+outdir+" && "+
			buildCommand)

		log.Debug().Msgf("%v", cmd)

		timer := time.AfterFunc(dockerTimeout, func() {
			cmd.Process.Kill()
		})
		defer timer.Stop()

		var raw []byte
		raw, err = cmd.CombinedOutput()
		if err != nil {
			e := fmt.Sprintf("error `%v` for cmd `%v` with output `%v`",
				err, buildCommand, string(raw))
			err = errors.New(e)
			return
		}

		output = string(raw)
	}
	return
}

func runScript(q *qemu.System, script string) (output string, err error) {
	return q.Command("root", script)
}

func testKernelModule(q *qemu.System, ka config.Artifact,
	test string) (output string, err error) {

	output, err = q.Command("root", test)
	// TODO generic checks for WARNING's and so on
	return
}

func testKernelExploit(q *qemu.System, ka config.Artifact,
	test, exploit string) (output string, err error) {

	output, err = q.Command("user", "chmod +x "+exploit)
	if err != nil {
		return
	}

	randFilePath := fmt.Sprintf("/root/%d", rand.Int())

	cmd := fmt.Sprintf("%s %s %s", test, exploit, randFilePath)
	output, err = q.Command("user", cmd)
	if err != nil {
		return
	}

	_, err = q.Command("root", "stat "+randFilePath)
	if err != nil {
		return
	}

	return
}

func genOkFail(name string, ok bool) (aurv aurora.Value) {
	state.Overall += 1
	s := " " + name
	if name == "" {
		s = ""
	}
	if ok {
		state.Success += 1
		s += " SUCCESS "
		aurv = aurora.BgGreen(aurora.Black(s))
	} else {
		s += " FAILURE "
		aurv = aurora.BgRed(aurora.White(aurora.Bold(s)))
	}
	return
}

type phasesResult struct {
	BuildDir         string
	BuildArtifact    string
	Build, Run, Test struct {
		Output string
		Ok     bool
	}
}

func copyFile(sourcePath, destinationPath string) (err error) {
	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return
	}
	defer sourceFile.Close()

	destinationFile, err := os.Create(destinationPath)
	if err != nil {
		return err
	}
	if _, err := io.Copy(destinationFile, sourceFile); err != nil {
		destinationFile.Close()
		return err
	}
	return destinationFile.Close()
}

func dumpResult(q *qemu.System, ka config.Artifact, ki distro.KernelInfo,
	res *phasesResult, dist, tag, binary string, db *sql.DB) {

	colored := ""
	switch ka.Type {
	case config.KernelExploit:
		colored = aurora.Sprintf("%s %s",
			genOkFail("BUILD", res.Build.Ok),
			genOkFail("LPE", res.Test.Ok))
	case config.KernelModule:
		colored = aurora.Sprintf("%s %s %s",
			genOkFail("BUILD", res.Build.Ok),
			genOkFail("INSMOD", res.Run.Ok),
			genOkFail("TEST", res.Test.Ok))
	case config.Script:
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
		if ka.Type != config.KernelExploit {
			path += ".ko"
		}

		err = copyFile(res.BuildArtifact, path)
		if err != nil {
			log.Warn().Err(err).Msgf("copy file (%v)", ka)
		}
	}
}

func copyArtifactAndTest(slog zerolog.Logger, q *qemu.System, ka config.Artifact,
	res *phasesResult, remoteTest string) (err error) {

	// Copy all test files to the remote machine
	for _, f := range ka.TestFiles {
		if f.Local[0] != '/' {
			if res.BuildDir != "" {
				f.Local = res.BuildDir + "/" + f.Local
			}
		}
		err = q.CopyFile(f.User, f.Local, f.Remote)
		if err != nil {
			slog.Error().Err(err).Msg("copy test file")
			return
		}
	}

	switch ka.Type {
	case config.KernelModule:
		res.Run.Output, err = q.CopyAndInsmod(res.BuildArtifact)
		if err != nil {
			slog.Error().Err(err).Msg(res.Run.Output)
			return
		}
		res.Run.Ok = true

		res.Test.Output, err = testKernelModule(q, ka, remoteTest)
		if err != nil {
			slog.Error().Err(err).Msg(res.Test.Output)
			return
		}
		res.Test.Ok = true
	case config.KernelExploit:
		remoteExploit := fmt.Sprintf("/tmp/exploit_%d", rand.Int())
		err = q.CopyFile("user", res.BuildArtifact, remoteExploit)
		if err != nil {
			return
		}

		res.Test.Output, err = testKernelExploit(q, ka, remoteTest,
			remoteExploit)
		if err != nil {
			slog.Error().Err(err).Msg(res.Test.Output)
			return
		}
		res.Run.Ok = true // does not really used
		res.Test.Ok = true
	case config.Script:
		res.Test.Output, err = runScript(q, remoteTest)
		if err != nil {
			slog.Error().Err(err).Msg(res.Test.Output)
			return
		}
		slog.Debug().Msg(res.Test.Output)
		res.Run.Ok = true
		res.Test.Ok = true
	default:
		slog.Fatal().Msg("Unsupported artifact type")
	}

	return
}

func copyTest(q *qemu.System, testPath string, ka config.Artifact) (
	remoteTest string, err error) {

	remoteTest = fmt.Sprintf("/tmp/test_%d", rand.Int())
	err = q.CopyFile("user", testPath, remoteTest)
	if err != nil {
		if ka.Type == config.KernelExploit {
			q.Command("user",
				"echo -e '#!/bin/sh\necho touch $2 | $1' "+
					"> "+remoteTest+
					" && chmod +x "+remoteTest)
		} else {
			q.Command("user", "echo '#!/bin/sh' "+
				"> "+remoteTest+" && chmod +x "+remoteTest)
		}
	}

	_, err = q.Command("root", "chmod +x "+remoteTest)
	return
}

func copyStandardModules(q *qemu.System, ki distro.KernelInfo) (err error) {
	_, err = q.Command("root", "mkdir -p /lib/modules/"+ki.KernelVersion)
	if err != nil {
		return
	}

	remotePath := "/lib/modules/" + ki.KernelVersion + "/"

	err = q.CopyDirectory("root", ki.ModulesPath+"/kernel", remotePath+"/kernel")
	if err != nil {
		return
	}

	files, err := ioutil.ReadDir(ki.ModulesPath)
	if err != nil {
		return
	}

	for _, f := range files {
		if f.Mode()&os.ModeSymlink == os.ModeSymlink {
			continue
		}
		if !strings.HasPrefix(f.Name(), "modules") {
			continue
		}
		err = q.CopyFile("root", ki.ModulesPath+"/"+f.Name(), remotePath)
	}

	return
}

func (cmd PewCmd) testArtifact(swg *sizedwaitgroup.SizedWaitGroup,
	ka config.Artifact, ki distro.KernelInfo) {

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
		&consoleWriter,
		&fileWriter,
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

	switch loglevel {
	case zerolog.TraceLevel, zerolog.DebugLevel:
		slog = slog.With().Caller().Logger()
	}

	slog = slog.With().Timestamp().
		Str("distro_type", ki.Distro.ID.String()).
		Str("distro_release", ki.Distro.Release).
		Str("kernel", ki.KernelRelease).
		Logger()

	slog.Info().Msg("start")

	kernel := qemu.Kernel{KernelPath: ki.KernelPath, InitrdPath: ki.InitrdPath}
	if cmd.RootFS != "" {
		ki.RootFS = cmd.RootFS
	}
	q, err := qemu.NewSystem(qemu.X86x64, kernel, ki.RootFS)
	if err != nil {
		slog.Error().Err(err).Msg("qemu init")
		return
	}
	q.Log = slog

	q.Timeout = cmd.QemuTimeout

	if ka.Qemu.Timeout.Duration != 0 {
		q.Timeout = ka.Qemu.Timeout.Duration
	}
	if ka.Qemu.Cpus != 0 {
		q.Cpus = ka.Qemu.Cpus
	}
	if ka.Qemu.Memory != 0 {
		q.Memory = ka.Qemu.Memory
	}

	if ka.Docker.Timeout.Duration != 0 {
		cmd.DockerTimeout = ka.Docker.Timeout.Duration
	}

	q.SetKASLR(!ka.Mitigations.DisableKaslr)
	q.SetSMEP(!ka.Mitigations.DisableSmep)
	q.SetSMAP(!ka.Mitigations.DisableSmap)
	q.SetKPTI(!ka.Mitigations.DisableKpti)

	if cmd.Endless {
		q.Timeout = 0
	}

	err = q.Start()
	if err != nil {
		slog.Error().Err(err).Msg("qemu start")
		return
	}
	defer q.Stop()

	slog.Debug().Msgf("wait %v", cmd.QemuAfterStartTimeout)
	time.Sleep(cmd.QemuAfterStartTimeout)

	go func() {
		time.Sleep(time.Minute)
		for !q.Died {
			slog.Debug().Msg("still alive")
			time.Sleep(time.Minute)
		}
	}()

	tmp, err := fs.TempDir()
	if err != nil {
		slog.Error().Err(err).Msg("making tmp directory")
		return
	}
	defer os.RemoveAll(tmp)

	result := phasesResult{}
	if !cmd.Endless {
		defer dumpResult(q, ka, ki, &result, cmd.Dist, cmd.Tag, cmd.Binary, cmd.db)
	}

	if ka.Type == config.Script {
		result.Build.Ok = true
		cmd.Test = ka.Script
	} else if cmd.Binary == "" {
		// TODO: build should return structure
		start := time.Now()
		result.BuildDir, result.BuildArtifact, result.Build.Output, err =
			build(slog, tmp, ka, ki, cmd.DockerTimeout)
		slog.Debug().Str("duration", time.Now().Sub(start).String()).
			Msg("build done")
		if err != nil {
			log.Error().Err(err).Msg("build")
			return
		}
		result.Build.Ok = true
	} else {
		result.BuildArtifact = cmd.Binary
		result.Build.Ok = true
	}

	if cmd.Test == "" {
		cmd.Test = result.BuildArtifact + "_test"
		if !fs.PathExists(cmd.Test) {
			slog.Debug().Msgf("%s does not exist", cmd.Test)
			cmd.Test = tmp + "/source/" + "test.sh"
		} else {
			slog.Debug().Msgf("%s exist", cmd.Test)
		}
	}

	err = q.WaitForSSH(cmd.QemuTimeout)
	if err != nil {
		return
	}

	remoteTest, err := copyTest(q, cmd.Test, ka)
	if err != nil {
		slog.Error().Err(err).Msg("copy test script")
		return
	}

	if ka.StandardModules {
		// Module depends on one of the standard modules
		start := time.Now()
		err = copyStandardModules(q, ki)
		if err != nil {
			slog.Error().Err(err).Msg("copy standard modules")
			return
		}
		slog.Debug().Str("duration", time.Now().Sub(start).String()).
			Msg("copy standard modules")
	}

	err = preloadModules(q, ka, ki, cmd.DockerTimeout)
	if err != nil {
		slog.Error().Err(err).Msg("preload modules")
		return
	}

	start := time.Now()
	copyArtifactAndTest(slog, q, ka, &result, remoteTest)
	slog.Debug().Str("duration", time.Now().Sub(start).String()).
		Msgf("test completed (success: %v)", result.Test.Ok)

	if !cmd.Endless {
		return
	}

	dumpResult(q, ka, ki, &result, cmd.Dist, cmd.Tag, cmd.Binary, cmd.db)

	if !result.Build.Ok || !result.Run.Ok || !result.Test.Ok {
		return
	}

	slog.Info().Msg("start endless tests")

	if cmd.EndlessStress != "" {
		slog.Debug().Msg("copy and run endless stress script")
		err = q.CopyAndRunAsync("root", cmd.EndlessStress)
		if err != nil {
			q.Stop()
			f.Sync()
			slog.Fatal().Err(err).Msg("cannot copy/run stress")
			return
		}
	}

	for {
		output, err := q.Command("root", remoteTest)
		if err != nil {
			q.Stop()
			f.Sync()
			slog.Fatal().Err(err).Msg(output)
			return
		}
		slog.Debug().Msg(output)

		slog.Info().Msg("test success")

		slog.Debug().Msgf("wait %v", cmd.EndlessTimeout)
		time.Sleep(cmd.EndlessTimeout)
	}
}

func shuffleKernels(a []distro.KernelInfo) []distro.KernelInfo {
	// Fisher–Yates shuffle
	for i := len(a) - 1; i > 0; i-- {
		j := rand.Intn(i + 1)
		a[i], a[j] = a[j], a[i]
	}
	return a
}

func (cmd PewCmd) performCI(ka config.Artifact) (err error) {
	found := false
	max := cmd.Max

	swg := sizedwaitgroup.New(cmd.Threads)
	if cmd.Shuffle {
		cmd.kcfg.Kernels = shuffleKernels(cmd.kcfg.Kernels)
	}
	for _, kernel := range cmd.kcfg.Kernels {
		if max <= 0 {
			break
		}

		var supported bool
		supported, err = ka.Supported(kernel)
		if err != nil {
			return
		}

		if supported {
			found = true
			max--
			for i := int64(0); i < cmd.Runs; i++ {
				if !cmd.timeoutDeadline.IsZero() &&
					time.Now().After(cmd.timeoutDeadline) {

					break
				}
				swg.Add()
				go cmd.testArtifact(&swg, ka, kernel)
			}
		}
	}
	swg.Wait()

	if !found {
		err = errors.New("No supported kernels found")
	}

	return
}

func kernelMask(kernel string) (km config.Target, err error) {
	parts := strings.Split(kernel, ":")
	if len(parts) != 2 {
		err = errors.New("Kernel is not 'distroType:regex'")
		return
	}

	dt, err := distro.NewID(parts[0])
	if err != nil {
		return
	}

	km = config.Target{
		Distro: distro.Distro{ID: dt},
		Kernel: config.Kernel{Regex: parts[1]},
	}
	return
}

func genAllKernels() (sk []config.Target, err error) {
	for _, id := range distro.IDs {
		sk = append(sk, config.Target{
			Distro: distro.Distro{ID: id},
			Kernel: config.Kernel{Regex: ".*"},
		})
	}
	return
}
