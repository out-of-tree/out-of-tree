// Copyright 2018 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/logrusorgru/aurora"
	"github.com/otiai10/copy"
	"github.com/remeh/sizedwaitgroup"

	"github.com/jollheef/out-of-tree/config"
	qemu "github.com/jollheef/out-of-tree/qemu"
)

func dockerCommand(container, workdir, timeout, command string) *exec.Cmd {
	return exec.Command("timeout", "-k", timeout, timeout, "docker", "run",
		"-v", workdir+":/work", container,
		"bash", "-c", "cd /work && "+command)
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

	seconds := fmt.Sprintf("%ds", dockerTimeout/time.Second)
	cmd := dockerCommand(ki.ContainerName, tmpSourcePath, seconds,
		"make KERNEL="+kernel+" TARGET="+target)
	rawOutput, err := cmd.CombinedOutput()
	output = string(rawOutput)
	if err != nil {
		err = errors.New("make execution error")
		return
	}

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

func genOkFail(name string, ok bool) aurora.Value {
	if ok {
		s := " " + name + " SUCCESS "
		return aurora.BgGreen(aurora.Black(s))
	} else {
		s := " " + name + " FAILURE "
		return aurora.BgRed(aurora.Gray(aurora.Bold(s)))
	}
}

func dumpResult(q *qemu.QemuSystem, ka config.Artifact, ki config.KernelInfo,
	build_ok, run_ok, test_ok *bool) {

	distroInfo := fmt.Sprintf("%s-%s {%s}", ki.DistroType,
		ki.DistroRelease, ki.KernelRelease)

	colored := ""
	if ka.Type == config.KernelExploit {
		colored = aurora.Sprintf("[*] %40s: %s %s", distroInfo,
			genOkFail("BUILD", *build_ok),
			genOkFail("LPE", *test_ok))
	} else {
		colored = aurora.Sprintf("[*] %40s: %s %s %s", distroInfo,
			genOkFail("BUILD", *build_ok),
			genOkFail("INSMOD", *run_ok),
			genOkFail("TEST", *test_ok))
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
}

func whatever(swg *sizedwaitgroup.SizedWaitGroup, ka config.Artifact,
	ki config.KernelInfo, binaryPath, testPath string,
	qemuTimeout, dockerTimeout time.Duration) {

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

	tmp, err := ioutil.TempDir("/tmp/", "out-of-tree_")
	if err != nil {
		log.Println("Temporary directory creation error:", err)
		return
	}
	defer os.RemoveAll(tmp)

	build_ok := false
	run_ok := false
	test_ok := false
	defer dumpResult(q, ka, ki, &build_ok, &run_ok, &test_ok)

	var outFile, output string
	if binaryPath == "" {
		// TODO Write build log to file or database
		outFile, output, err = build(tmp, ka, ki, dockerTimeout)
		if err != nil {
			log.Println(output)
			return
		}
		build_ok = true
	} else {
		outFile = binaryPath
		build_ok = true
	}

	err = cleanDmesg(q)
	if err != nil {
		return
	}

	if testPath == "" {
		testPath = outFile + "_test"
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
			log.Println("copy file err", err)
			// we should not exit because of testing 'insmod' part
			// for kernel module
		}
	}

	_, err = q.Command("root", "chmod +x "+remoteTest)
	if err != nil {
		return
	}

	if ka.Type == config.KernelModule {
		// TODO Write insmod log to file or database
		output, err := q.CopyAndInsmod(outFile)
		if err != nil {
			log.Println(output, err)
			return
		}
		run_ok = true

		// TODO Write test results to file or database
		output, err = testKernelModule(q, ka, remoteTest)
		if err != nil {
			log.Println(output, err)
			return
		}
		test_ok = true
	} else if ka.Type == config.KernelExploit {
		remoteExploit := fmt.Sprintf("/tmp/exploit_%d", rand.Int())
		err = q.CopyFile("user", outFile, remoteExploit)
		if err != nil {
			return
		}

		// TODO Write test results to file or database
		output, err = testKernelExploit(q, ka, remoteTest, remoteExploit)
		if err != nil {
			log.Println(output)
			return
		}
		run_ok = true // does not really used
		test_ok = true
	} else {
		err = errors.New("Unsupported artifact type")
	}
	return
}

func performCI(ka config.Artifact, kcfg config.KernelConfig, binaryPath,
	testPath string, qemuTimeout, dockerTimeout time.Duration) (err error) {

	found := false

	swg := sizedwaitgroup.New(runtime.NumCPU())
	for _, kernel := range kcfg.Kernels {
		var supported bool
		supported, err = ka.Supported(kernel)
		if err != nil {
			return
		}

		if supported {
			found = true
			swg.Add()
			go whatever(&swg, ka, kernel, binaryPath, testPath,
				qemuTimeout, dockerTimeout)
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

func pewHandler(kcfg config.KernelConfig,
	workPath, ovrrdKrnl, binary, test string, guess bool,
	qemuTimeout, dockerTimeout time.Duration) (err error) {

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
		ka.SupportedKernels = []config.KernelMask{}
		for _, dType := range config.DistroTypeStrings {
			var dt config.DistroType
			dt, err = config.NewDistroType(dType)
			if err != nil {
				return
			}

			km := config.KernelMask{DistroType: dt, ReleaseMask: ".*"}
			ka.SupportedKernels = append(ka.SupportedKernels, km)
		}
	}

	err = performCI(ka, kcfg, binary, test, qemuTimeout, dockerTimeout)
	if err != nil {
		return
	}

	return
}
