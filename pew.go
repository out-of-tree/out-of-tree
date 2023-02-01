// Copyright 2018 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package main

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"os/user"
	"strings"
	"time"

	"github.com/otiai10/copy"
	"github.com/remeh/sizedwaitgroup"
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
	Verbose bool          `help:"show more information"`
	Timeout time.Duration `help:"timeout after tool will not spawn new tests"`

	ArtifactConfig string `help:"path to artifact config" type:"path"`

	QemuTimeout   time.Duration `help:"timeout for qemu"`
	DockerTimeout time.Duration `help:"timeout for docker"`

	Threshold float64 `help:"reliablity threshold for exit code" default:"1.00"`
}

func (cmd PewCmd) Run(g *Globals) (err error) {
	kcfg, err := config.ReadKernelConfig(g.Config.Kernels)
	if err != nil {
		log.Println(err)
	}

	stop := time.Time{} // never stop
	if cmd.Timeout != 0 {
		stop = time.Now().Add(cmd.Timeout)
	}

	db, err := openDatabase(g.Config.Database)
	if err != nil {
		log.Fatalln(err)
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
		qemuTimeout = cmd.QemuTimeout
	}

	dockerTimeout := g.Config.Docker.Timeout.Duration
	if cmd.DockerTimeout != 0 {
		dockerTimeout = cmd.DockerTimeout
	}

	err = performCI(ka, kcfg, cmd.Binary, cmd.Test, stop,
		qemuTimeout, dockerTimeout,
		cmd.Max, cmd.Runs, cmd.Dist, cmd.Tag,
		cmd.Threads, db, cmd.Verbose)
	if err != nil {
		return
	}

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

func dockerRun(timeout time.Duration, container, workdir, command string) (
	output string, err error) {

	cmd := exec.Command("docker", "run", "-v", workdir+":/work",
		container, "bash", "-c", "cd /work && "+command)

	timer := time.AfterFunc(timeout, func() {
		cmd.Process.Kill()
	})
	defer timer.Stop()

	raw, err := cmd.CombinedOutput()
	if err != nil {
		e := fmt.Sprintf("error `%v` for cmd `%v` with output `%v`",
			err, command, string(raw))
		err = errors.New(e)
		return
	}

	output = string(raw)
	return
}

func build(tmp string, ka config.Artifact, ki config.KernelInfo,
	dockerTimeout time.Duration) (outPath, output string, err error) {

	target := fmt.Sprintf("%d_%s", rand.Int(), ki.KernelRelease)

	tmpSourcePath := tmp + "/source"

	err = copy.Copy(ka.SourcePath, tmpSourcePath)
	if err != nil {
		return
	}

	outPath = tmpSourcePath + "/" + target
	if ka.Type == config.KernelModule {
		outPath += ".ko"
	}

	kernel := "/lib/modules/" + ki.KernelRelease + "/build"
	if ki.KernelSource != "" {
		kernel = ki.KernelSource
	}

	if ki.ContainerName != "" {
		output, err = dockerRun(dockerTimeout, ki.ContainerName,
			tmpSourcePath, "make KERNEL="+kernel+" TARGET="+target+
				" && chmod -R 777 /work")
	} else {
		command := "make KERNEL=" + kernel + " TARGET=" + target
		cmd := exec.Command("bash", "-c", "cd "+tmpSourcePath+" && "+command)
		timer := time.AfterFunc(dockerTimeout, func() {
			cmd.Process.Kill()
		})
		defer timer.Stop()

		var raw []byte
		raw, err = cmd.CombinedOutput()
		if err != nil {
			e := fmt.Sprintf("error `%v` for cmd `%v` with output `%v`",
				err, command, string(raw))
			err = errors.New(e)
			return
		}

		output = string(raw)
	}
	return
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
	if ok {
		state.Success += 1
		s := " " + name + " SUCCESS "
		aurv = aurora.BgGreen(aurora.Black(s))
	} else {
		s := " " + name + " FAILURE "
		aurv = aurora.BgRed(aurora.White(aurora.Bold(s)))
	}
	return
}

type phasesResult struct {
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
	if ka.Type == config.KernelExploit {
		colored = aurora.Sprintf("[*] %40s: %s %s", distroInfo,
			genOkFail("BUILD", res.Build.Ok),
			genOkFail("LPE", res.Test.Ok))
	} else {
		colored = aurora.Sprintf("[*] %40s: %s %s %s", distroInfo,
			genOkFail("BUILD", res.Build.Ok),
			genOkFail("INSMOD", res.Run.Ok),
			genOkFail("TEST", res.Test.Ok))
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
		log.Println("[db] addToLog (", ka, ") error:", err)
	}

	if binary == "" && dist != pathDevNull {
		err = os.MkdirAll(dist, os.ModePerm)
		if err != nil {
			log.Println("os.MkdirAll (", ka, ") error:", err)
		}

		path := fmt.Sprintf("%s/%s-%s-%s", dist, ki.DistroType,
			ki.DistroRelease, ki.KernelRelease)
		if ka.Type != config.KernelExploit {
			path += ".ko"
		}

		err = copyFile(res.BuildArtifact, path)
		if err != nil {
			log.Println("copyFile (", ka, ") error:", err)
		}
	}
}

func copyArtifactAndTest(q *qemu.System, ka config.Artifact,
	res *phasesResult, remoteTest string) (err error) {

	switch ka.Type {
	case config.KernelModule:
		res.Run.Output, err = q.CopyAndInsmod(res.BuildArtifact)
		if err != nil {
			log.Println(res.Run.Output, err)
			return
		}
		res.Run.Ok = true

		// Copy all test files to the remote machine
		for _, f := range ka.TestFiles {
			err = q.CopyFile(f.User, f.Local, f.Remote)
			if err != nil {
				log.Println("error copy err:", err, f.Local, f.Remote)
				return
			}
		}

		res.Test.Output, err = testKernelModule(q, ka, remoteTest)
		if err != nil {
			log.Println(res.Test.Output, err)
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
			log.Println(res.Test.Output)
			return
		}
		res.Run.Ok = true // does not really used
		res.Test.Ok = true
	default:
		log.Println("Unsupported artifact type")
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

func whatever(swg *sizedwaitgroup.SizedWaitGroup, ka config.Artifact,
	ki config.KernelInfo, binaryPath, testPath string,
	qemuTimeout, dockerTimeout time.Duration, dist, tag string,
	db *sql.DB, verbose bool) {

	defer swg.Done()

	kernel := qemu.Kernel{KernelPath: ki.KernelPath, InitrdPath: ki.InitrdPath}
	q, err := qemu.NewSystem(qemu.X86x64, kernel, ki.RootFS)
	if err != nil {
		log.Println("Qemu creation error:", err)
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
		log.Println("Qemu start error:", err)
		return
	}
	defer q.Stop()

	if verbose {
		go func() {
			for !q.Died {
				time.Sleep(time.Minute)
				log.Println(ka.Name, ki.DistroType,
					ki.DistroRelease, ki.KernelRelease,
					"still alive")

			}
		}()
	}

	usr, err := user.Current()
	if err != nil {
		return
	}
	tmpdir := usr.HomeDir + "/.out-of-tree/tmp"
	os.MkdirAll(tmpdir, os.ModePerm)

	tmp, err := ioutil.TempDir(tmpdir, "out-of-tree_")
	if err != nil {
		log.Println("Temporary directory creation error:", err)
		return
	}
	defer os.RemoveAll(tmp)

	result := phasesResult{}
	defer dumpResult(q, ka, ki, &result, dist, tag, binaryPath, db)

	if binaryPath == "" {
		result.BuildArtifact, result.Build.Output, err = build(tmp, ka,
			ki, dockerTimeout)
		if err != nil {
			log.Println(err)
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

	remoteTest, err := copyTest(q, testPath, ka)
	if err != nil {
		return
	}

	err = preloadModules(q, ka, ki, dockerTimeout)
	if err != nil {
		log.Println(err)
		return
	}

	copyArtifactAndTest(q, ka, &result, remoteTest)
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
	db *sql.DB, verbose bool) (err error) {

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
				go whatever(&swg, ka, kernel, binaryPath,
					testPath, qemuTimeout, dockerTimeout,
					dist, tag, db, verbose)
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
		return false
	}
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
