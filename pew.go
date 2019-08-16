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
	"runtime"
	"strings"
	"time"

	"github.com/otiai10/copy"
	"github.com/remeh/sizedwaitgroup"
	"gopkg.in/logrusorgru/aurora.v1"

	"code.dumpstack.io/tools/out-of-tree/config"
	"code.dumpstack.io/tools/out-of-tree/qemu"
)

var somethingFailed = false

const PATH_DEV_NULL = "/dev/null"

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

	output, err = dockerRun(dockerTimeout, ki.ContainerName,
		tmpSourcePath, "make KERNEL="+kernel+" TARGET="+target+
			" && chmod -R 777 /work")
	return
}

func cleanDmesg(q *qemu.QemuSystem) (err error) {
	start := time.Now()
	for {
		_, err = q.Command("root", "dmesg -c")
		if err == nil {
			break
		}
		time.Sleep(time.Second)

		if time.Now().After(start.Add(time.Minute)) {
			err = errors.New("Can't connect to qemu")
			break
		}
	}
	return
}

func testKernelModule(q *qemu.QemuSystem, ka config.Artifact,
	test string) (output string, err error) {

	output, err = q.Command("root", test)
	// TODO generic checks for WARNING's and so on
	return
}

func testKernelExploit(q *qemu.QemuSystem, ka config.Artifact,
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
	if ok {
		s := " " + name + " SUCCESS "
		aurv = aurora.BgGreen(aurora.Black(s))
	} else {
		somethingFailed = true
		s := " " + name + " FAILURE "
		aurv = aurora.BgRed(aurora.Gray(aurora.Bold(s)))
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

func dumpResult(q *qemu.QemuSystem, ka config.Artifact, ki config.KernelInfo,
	res *phasesResult, dist, binary string, db *sql.DB) {

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

	err := addToLog(db, q, ka, ki, res)
	if err != nil {
		log.Println("[db] addToLog (", ka, ") error:", err)
	}

	if binary == "" && dist != PATH_DEV_NULL {
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

func whatever(swg *sizedwaitgroup.SizedWaitGroup, ka config.Artifact,
	ki config.KernelInfo, binaryPath, testPath string,
	qemuTimeout, dockerTimeout time.Duration, dist string, db *sql.DB) {

	defer swg.Done()

	kernel := qemu.Kernel{KernelPath: ki.KernelPath, InitrdPath: ki.InitrdPath}
	q, err := qemu.NewQemuSystem(qemu.X86_64, kernel, ki.RootFS)
	if err != nil {
		log.Println("Qemu creation error:", err)
		return
	}
	q.Timeout = qemuTimeout

	err = q.Start()
	if err != nil {
		log.Println("Qemu start error:", err)
		return
	}
	defer q.Stop()

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
	defer dumpResult(q, ka, ki, &result, dist, binaryPath, db)

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

	err = cleanDmesg(q)
	if err != nil {
		return
	}

	if testPath == "" {
		testPath = result.BuildArtifact + "_test"
	}

	remoteTest := fmt.Sprintf("/tmp/test_%d", rand.Int())
	err = q.CopyFile("user", testPath, remoteTest)
	if err != nil {
		if ka.Type == config.KernelExploit {
			log.Println("Use `echo touch FILE | exploit` for test")
			q.Command("user",
				"echo -e '#!/bin/sh\necho touch $2 | $1' "+
					"> "+remoteTest+
					" && chmod +x "+remoteTest)
		} else {
			log.Println("No test, use dummy")
			q.Command("user", "echo '#!/bin/sh' "+
				"> "+remoteTest+" && chmod +x "+remoteTest)
		}
	} else {
		_, err = q.Command("root", "chmod +x "+remoteTest)
		if err != nil {
			return
		}
	}

	switch ka.Type {
	case config.KernelModule:
		result.Run.Output, err = q.CopyAndInsmod(result.BuildArtifact)
		if err != nil {
			log.Println(result.Run.Output, err)
			return
		}
		result.Run.Ok = true

		result.Test.Output, err = testKernelModule(q, ka, remoteTest)
		if err != nil {
			log.Println(result.Test.Output, err)
			return
		}
		result.Test.Ok = true
	case config.KernelExploit:
		remoteExploit := fmt.Sprintf("/tmp/exploit_%d", rand.Int())
		err = q.CopyFile("user", result.BuildArtifact, remoteExploit)
		if err != nil {
			return
		}

		result.Test.Output, err = testKernelExploit(q, ka, remoteTest,
			remoteExploit)
		if err != nil {
			log.Println(result.Test.Output)
			return
		}
		result.Run.Ok = true // does not really used
		result.Test.Ok = true
	default:
		log.Println("Unsupported artifact type")
	}
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
	testPath string, qemuTimeout, dockerTimeout time.Duration,
	max int64, dist string, db *sql.DB) (err error) {

	found := false

	swg := sizedwaitgroup.New(runtime.NumCPU())
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
			max -= 1
			swg.Add()
			go whatever(&swg, ka, kernel, binaryPath, testPath,
				qemuTimeout, dockerTimeout, dist, db)
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

func pewHandler(kcfg config.KernelConfig,
	workPath, ovrrdKrnl, binary, test string, guess bool,
	qemuTimeout, dockerTimeout time.Duration,
	max int64, dist string, db *sql.DB) (err error) {

	ka, err := config.ReadArtifactConfig(workPath + "/.out-of-tree.toml")
	if err != nil {
		return
	}

	if ka.SourcePath == "" {
		ka.SourcePath = workPath
	}

	if ovrrdKrnl != "" {
		var km config.KernelMask
		km, err = kernelMask(ovrrdKrnl)
		if err != nil {
			return
		}

		ka.SupportedKernels = []config.KernelMask{km}
	}

	if guess {
		ka.SupportedKernels, err = genAllKernels()
		if err != nil {
			return
		}
	}

	err = performCI(ka, kcfg, binary, test, qemuTimeout, dockerTimeout,
		max, dist, db)
	if err != nil {
		return
	}

	return
}
