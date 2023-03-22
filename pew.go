// Copyright 2023 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package main

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"os/user"
	"strings"
	"time"

	"github.com/otiai10/copy"
	"github.com/remeh/sizedwaitgroup"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gopkg.in/logrusorgru/aurora.v2"

	"code.dumpstack.io/tools/out-of-tree/config"
	"code.dumpstack.io/tools/out-of-tree/qemu"
)

type PewCmd struct {
	Max     int64         `help:"test no more than X kernels" default:"100500"`
	Runs    int64         `help:"runs per each kernel" default:"1"`
	Kernel  string        `help:"override kernel regex"`
	Guess   bool          `help:"try all defined kernels"`
	Binary  string        `help:"use binary, do not build"`
	Test    string        `help:"override path for test"`
	Dist    string        `help:"build result path" default:"/dev/null"`
	Threads int           `help:"threads" default:"1"`
	Tag     string        `help:"log tagging"`
	Timeout time.Duration `help:"timeout after tool will not spawn new tests"`

	ArtifactConfig string `help:"path to artifact config" type:"path"`

	QemuTimeout   time.Duration `help:"timeout for qemu"`
	DockerTimeout time.Duration `help:"timeout for docker"`

	Threshold float64 `help:"reliablity threshold for exit code" default:"1.00"`
}

func (cmd PewCmd) Run(g *Globals) (err error) {
	kcfg, err := config.ReadKernelConfig(g.Config.Kernels)
	if err != nil {
		log.Debug().Err(err).Msg("read kernels config")
	}

	stop := time.Time{} // never stop
	if cmd.Timeout != 0 {
		log.Info().Msgf("Set global timeout to %s", cmd.Timeout)
		stop = time.Now().Add(cmd.Timeout)
	}

	db, err := openDatabase(g.Config.Database)
	if err != nil {
		log.Fatal().Err(err).
			Msgf("Cannot open database %s", g.Config.Database)
	}
	defer db.Close()

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

	if ka.SourcePath == "" {
		ka.SourcePath = g.WorkDir
	}

	if cmd.Kernel != "" {
		var km config.KernelMask
		km, err = kernelMask(cmd.Kernel)
		if err != nil {
			return
		}

		ka.SupportedKernels = []config.KernelMask{km}
	}

	if cmd.Guess {
		ka.SupportedKernels, err = genAllKernels()
		if err != nil {
			return
		}
	}

	qemuTimeout := g.Config.Qemu.Timeout.Duration
	if cmd.QemuTimeout != 0 {
		log.Info().Msgf("Set qemu timeout to %s", cmd.QemuTimeout)
		qemuTimeout = cmd.QemuTimeout
	}

	dockerTimeout := g.Config.Docker.Timeout.Duration
	if cmd.DockerTimeout != 0 {
		log.Info().Msgf("Set docker timeout to %s", cmd.DockerTimeout)
		dockerTimeout = cmd.DockerTimeout
	}

	if cmd.Tag == "" {
		cmd.Tag = fmt.Sprintf("%d", time.Now().Unix())
	}
	log.Info().Str("tag", cmd.Tag).Msg("log")

	err = performCI(ka, kcfg, cmd.Binary, cmd.Test, stop,
		qemuTimeout, dockerTimeout,
		cmd.Max, cmd.Runs, cmd.Dist, cmd.Tag,
		cmd.Threads, db)
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

func sh(workdir, cmd string) (output string, err error) {
	command := exec.Command("sh", "-c", "cd "+workdir+" && "+cmd)

	log.Debug().Msgf("%v", command)

	raw, err := command.CombinedOutput()
	output = string(raw)
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

func build(tmp string, ka config.Artifact, ki config.KernelInfo,
	dockerTimeout time.Duration) (outdir, outpath, output string, err error) {

	target := fmt.Sprintf("%d_%s", rand.Int(), ki.KernelRelease)

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

	kernel := "/lib/modules/" + ki.KernelRelease + "/build"
	if ki.KernelSource != "" {
		kernel = ki.KernelSource
	}

	buildCommand := "make KERNEL=" + kernel + " TARGET=" + target
	if ka.Make.Target != "" {
		buildCommand += " " + ka.Make.Target
	}

	if ki.ContainerName != "" {
		var c container
		c, err = NewContainer(ki.ContainerName, dockerTimeout)
		if err != nil {
			log.Fatal().Err(err).Msg("container creation failure")
		}

		output, err = c.Run(outdir, buildCommand+" && chmod -R 777 /work")
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

func dumpResult(q *qemu.System, ka config.Artifact, ki config.KernelInfo,
	res *phasesResult, dist, tag, binary string, db *sql.DB) {

	// TODO merge (problem is it's not 100% same) with log.go:logLogEntry

	distroInfo := fmt.Sprintf("%s-%s {%s}", ki.DistroType,
		ki.DistroRelease, ki.KernelRelease)

	colored := ""
	switch ka.Type {
	case config.KernelExploit:
		colored = aurora.Sprintf("[*] %40s: %s %s", distroInfo,
			genOkFail("BUILD", res.Build.Ok),
			genOkFail("LPE", res.Test.Ok))
	case config.KernelModule:
		colored = aurora.Sprintf("[*] %40s: %s %s %s", distroInfo,
			genOkFail("BUILD", res.Build.Ok),
			genOkFail("INSMOD", res.Run.Ok),
			genOkFail("TEST", res.Test.Ok))
	case config.Script:
		colored = aurora.Sprintf("[*] %40s: %s", distroInfo,
			genOkFail("", res.Test.Ok))
	}

	additional := ""
	if q.KernelPanic {
		additional = "(panic)"
	} else if q.KilledByTimeout {
		additional = "(timeout)"
	}

	if additional != "" {
		fmt.Println(colored, additional)
	} else {
		fmt.Println(colored)
	}

	err := addToLog(db, q, ka, ki, res, tag)
	if err != nil {
		log.Warn().Err(err).Msgf("[db] addToLog (%v)", ka)
	}

	if binary == "" && dist != pathDevNull {
		err = os.MkdirAll(dist, os.ModePerm)
		if err != nil {
			log.Warn().Err(err).Msgf("os.MkdirAll (%v)", ka)
		}

		path := fmt.Sprintf("%s/%s-%s-%s", dist, ki.DistroType,
			ki.DistroRelease, ki.KernelRelease)
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
		slog.Info().Msg(res.Test.Output)
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

func copyStandardModules(q *qemu.System, ki config.KernelInfo) (err error) {
	_, err = q.Command("root", "mkdir -p /lib/modules")
	if err != nil {
		return
	}

	files, err := ioutil.ReadDir(ki.ModulesPath)
	if err != nil {
		return
	}

	// FIXME scp cannot ignore symlinks
	for _, f := range files {
		if f.Mode()&os.ModeSymlink == os.ModeSymlink {
			continue
		}

		path := ki.ModulesPath + "/" + f.Name()
		err = q.CopyDirectory("root", path, "/lib/modules/"+ki.KernelRelease+"/")
		if err != nil {
			return
		}
	}

	return
}

func testArtifact(swg *sizedwaitgroup.SizedWaitGroup, ka config.Artifact,
	ki config.KernelInfo, binaryPath, testPath string,
	qemuTimeout, dockerTimeout time.Duration, dist, tag string,
	db *sql.DB) {

	defer swg.Done()

	slog := log.With().
		Str("distro_type", ki.DistroType.String()).
		Str("distro_release", ki.DistroRelease).
		Str("kernel", ki.KernelRelease).
		Logger()

	slog.Info().Msg("start")

	kernel := qemu.Kernel{KernelPath: ki.KernelPath, InitrdPath: ki.InitrdPath}
	q, err := qemu.NewSystem(qemu.X86x64, kernel, ki.RootFS)
	if err != nil {
		slog.Error().Err(err).Msg("qemu init")
		return
	}
	q.Timeout = qemuTimeout

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
		dockerTimeout = ka.Docker.Timeout.Duration
	}

	q.SetKASLR(!ka.Mitigations.DisableKaslr)
	q.SetSMEP(!ka.Mitigations.DisableSmep)
	q.SetSMAP(!ka.Mitigations.DisableSmap)
	q.SetKPTI(!ka.Mitigations.DisableKpti)

	err = q.Start()
	if err != nil {
		slog.Error().Err(err).Msg("qemu start")
		return
	}
	defer q.Stop()

	go func() {
		for !q.Died {
			time.Sleep(time.Minute)
			slog.Debug().Msg("still alive")
		}
	}()

	usr, err := user.Current()
	if err != nil {
		return
	}
	tmpdir := usr.HomeDir + "/.out-of-tree/tmp"
	os.MkdirAll(tmpdir, os.ModePerm)

	tmp, err := ioutil.TempDir(tmpdir, "out-of-tree_")
	if err != nil {
		slog.Error().Err(err).Msg("making tmp directory")
		return
	}
	defer os.RemoveAll(tmp)

	result := phasesResult{}
	defer dumpResult(q, ka, ki, &result, dist, tag, binaryPath, db)

	if ka.Type == config.Script {
		result.Build.Ok = true
		testPath = ka.Script
	} else if binaryPath == "" {
		// TODO: build should return structure
		start := time.Now()
		result.BuildDir, result.BuildArtifact, result.Build.Output, err =
			build(tmp, ka, ki, dockerTimeout)
		slog.Debug().Str("duration", time.Now().Sub(start).String()).
			Msg("build done")
		if err != nil {
			log.Error().Err(err).Msg("build")
			return
		}
		result.Build.Ok = true
	} else {
		result.BuildArtifact = binaryPath
		result.Build.Ok = true
	}

	if testPath == "" {
		testPath = result.BuildArtifact + "_test"
		if !exists(testPath) {
			testPath = tmp + "/source/" + "test.sh"
		}
	}

	err = q.WaitForSSH(qemuTimeout)
	if err != nil {
		return
	}

	remoteTest, err := copyTest(q, testPath, ka)
	if err != nil {
		return
	}

	if ka.StandardModules {
		// Module depends on one of the standard modules
		start := time.Now()
		err = copyStandardModules(q, ki)
		if err != nil {
			slog.Fatal().Err(err).Msg("copy standard modules")
			return
		}
		slog.Debug().Str("duration", time.Now().Sub(start).String()).
			Msg("copy standard modules")
	}

	err = preloadModules(q, ka, ki, dockerTimeout)
	if err != nil {
		slog.Error().Err(err).Msg("preload modules")
		return
	}

	start := time.Now()
	copyArtifactAndTest(slog, q, ka, &result, remoteTest)
	slog.Debug().Str("duration", time.Now().Sub(start).String()).
		Msg("test completed")
}

func shuffleKernels(a []config.KernelInfo) []config.KernelInfo {
	// Fisher–Yates shuffle
	for i := len(a) - 1; i > 0; i-- {
		j := rand.Intn(i + 1)
		a[i], a[j] = a[j], a[i]
	}
	return a
}

func performCI(ka config.Artifact, kcfg config.KernelConfig, binaryPath,
	testPath string, stop time.Time,
	qemuTimeout, dockerTimeout time.Duration,
	max, runs int64, dist, tag string, threads int,
	db *sql.DB) (err error) {

	found := false

	swg := sizedwaitgroup.New(threads)
	for _, kernel := range shuffleKernels(kcfg.Kernels) {
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
			for i := int64(0); i < runs; i++ {
				if !stop.IsZero() && time.Now().After(stop) {
					break
				}
				swg.Add()
				go testArtifact(&swg, ka, kernel, binaryPath,
					testPath, qemuTimeout, dockerTimeout,
					dist, tag, db)
			}
		}
	}
	swg.Wait()

	if !found {
		err = errors.New("No supported kernels found")
	}

	return
}

func exists(path string) bool {
	if _, err := os.Stat(path); err != nil {
		log.Debug().Msgf("%s does not exist", path)
		return false
	}
	log.Debug().Msgf("%s exist", path)
	return true
}

func kernelMask(kernel string) (km config.KernelMask, err error) {
	parts := strings.Split(kernel, ":")
	if len(parts) != 2 {
		err = errors.New("Kernel is not 'distroType:regex'")
		return
	}

	dt, err := config.NewDistroType(parts[0])
	if err != nil {
		return
	}

	km = config.KernelMask{DistroType: dt, ReleaseMask: parts[1]}
	return
}

func genAllKernels() (sk []config.KernelMask, err error) {
	for _, dType := range config.DistroTypeStrings {
		var dt config.DistroType
		dt, err = config.NewDistroType(dType)
		if err != nil {
			return
		}

		sk = append(sk, config.KernelMask{
			DistroType:  dt,
			ReleaseMask: ".*",
		})
	}
	return
}
