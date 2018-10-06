// Copyright 2018 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a GPLv3 license
// (or later) that can be found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"time"

	"github.com/logrusorgru/aurora"
	"github.com/naoina/toml"
	"github.com/otiai10/copy"
	"github.com/remeh/sizedwaitgroup"

	qemu "github.com/jollheef/out-of-tree/qemu"
)

type kernelMask struct {
	DistroType  distroType
	ReleaseMask string
}

type artifactType int

const (
	KernelModule artifactType = iota
	KernelExploit
)

func (at artifactType) String() string {
	return [...]string{"module", "exploit"}[at]
}

type artifact struct {
	Name             string
	Type             artifactType
	SourcePath       string
	SupportedKernels []kernelMask
}

func (ka artifact) checkSupport(ki kernelInfo, km kernelMask) (
	supported bool, err error) {

	if ki.DistroType != km.DistroType {
		supported = false
		return
	}

	supported, err = regexp.MatchString(km.ReleaseMask, ki.KernelRelease)
	return
}

func (ka artifact) Supported(ki kernelInfo) (supported bool, err error) {
	for _, km := range ka.SupportedKernels {
		supported, err = ka.checkSupport(ki, km)
	}
	return
}

type distroType int

const (
	Ubuntu distroType = iota
	CentOS
	Debian
)

func (dt distroType) String() string {
	return [...]string{"Ubuntu", "CentOS", "Debian"}[dt]
}

type kernelInfo struct {
	DistroType    distroType
	DistroRelease string // 18.04/7.4.1708/9.1

	// Must be *exactly* same as in `uname -r`
	KernelRelease string

	// Build-time information
	ContainerName string

	// Runtime information
	KernelPath string
	InitrdPath string
	RootFS     string
}

func dockerCommand(container, workdir, timeout, command string) *exec.Cmd {
	return exec.Command("timeout", "-k", timeout, timeout, "docker", "run",
		"-v", workdir+":/work", container,
		"bash", "-c", "cd /work && "+command)
}

func build(tmp string, ka artifact, ki kernelInfo) (outPath string, err error) {
	target := fmt.Sprintf("%s_%s-%s-%s", ka.Name, ki.DistroType,
		ki.DistroRelease, ki.KernelRelease)

	tmpSourcePath := tmp + "/source"

	err = copy.Copy(ka.SourcePath, tmpSourcePath)
	if err != nil {
		return
	}

	outPath = tmpSourcePath + "/" + target + ".ko"

	kernel := "/lib/modules/" + ki.KernelRelease + "/build"

	cmd := dockerCommand(ki.ContainerName, tmpSourcePath, "1m", // TODO CFG
		"make KERNEL="+kernel+" TARGET="+target)
	output, err := cmd.CombinedOutput()
	if err != nil {
		err = errors.New(string(output))
		return
	}

	return
}

func run(q *qemu.QemuSystem, ka artifact, ki kernelInfo, file string) (output string, err error) {
	start := time.Now()
	for {
		output, err = q.Command("root", "dmesg -c")
		if err == nil {
			break
		}
		time.Sleep(time.Second)

		if time.Now().After(start.Add(time.Minute)) {
			err = errors.New("Can't connect to qemu")
			return
		}
	}

	switch ka.Type {
	case KernelModule:
		output, err = q.CopyAndInsmod(file)
	case KernelExploit:
		output, err = q.CopyAndRun("user", file)
	default:
		err = errors.New("Unsupported artifact type")
	}

	return
}

func test(q *qemu.QemuSystem, ka artifact) (output string, err error) {
	// TODO
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

func dumpResult(ka artifact, ki kernelInfo, build_ok, run_ok, test_ok *bool) {
	var stest aurora.Value
	if ka.Type == KernelExploit {
		stest = genOkFail("LPE", *test_ok)
	} else {
		stest = genOkFail("TEST", *test_ok)
	}

	var srun aurora.Value
	if ka.Type == KernelExploit {
		srun = genOkFail("RUN", *run_ok)
	} else {
		srun = genOkFail("INSMOD", *run_ok)
	}

	colored := aurora.Sprintf("[*] %40s: %s %s %s",
		fmt.Sprintf("%s-%s {%s}", ki.DistroType, ki.DistroRelease,
			ki.KernelRelease),
		genOkFail("BUILD", *build_ok),
		srun, stest)

	fmt.Println(colored)
}

func whatever(swg *sizedwaitgroup.SizedWaitGroup, ka artifact, ki kernelInfo) {
	defer swg.Done()

	kernel := qemu.Kernel{KernelPath: ki.KernelPath, InitrdPath: ki.InitrdPath}
	q, err := qemu.NewQemuSystem(qemu.X86_64, kernel, ki.RootFS)
	if err != nil {
		return
	}
	q.Timeout = time.Minute

	err = q.Start()
	if err != nil {
		return
	}
	defer q.Stop()

	tmp, err := ioutil.TempDir("/tmp/", "out-of-tree_")
	if err != nil {
		return
	}
	defer os.RemoveAll(tmp)

	build_ok := false
	run_ok := false
	test_ok := false
	defer dumpResult(ka, ki, &build_ok, &run_ok, &test_ok)

	// TODO Write build log to file or database
	outFile, err := build(tmp, ka, ki)
	if err != nil {
		return
	}
	build_ok = true

	// TODO Write run log to file or database
	_, err = run(q, ka, ki, outFile)
	if err != nil {
		return
	}
	run_ok = true

	// TODO Write test results to file or database
	_, err = test(q, ka)
	if err != nil {
		return
	}
	test_ok = true
}

type kernelConfig struct {
	Kernels []kernelInfo
}

func readFileAll(path string) (buf []byte, err error) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	buf, err = ioutil.ReadAll(f)
	return
}

func readKernelConfig(path string) (kernelCfg kernelConfig, err error) {
	buf, err := readFileAll(path)
	if err != nil {
		return
	}

	err = toml.Unmarshal(buf, &kernelCfg)
	if err != nil {
		return
	}

	return
}

func readArtifactConfig(path string) (artifactCfg artifact, err error) {
	buf, err := readFileAll(path)
	if err != nil {
		return
	}

	err = toml.Unmarshal(buf, &artifactCfg)
	if err != nil {
		return
	}

	return
}

func performCI(ka artifact, kcfg kernelConfig) (err error) {
	swg := sizedwaitgroup.New(runtime.NumCPU())
	for _, kernel := range kcfg.Kernels {
		var supported bool
		supported, err = ka.Supported(kernel)
		if err != nil {
			return
		}

		if supported {
			swg.Add()
			go whatever(&swg, ka, kernel)
		}
	}
	swg.Wait()
	return
}

func exists(path string) bool {
	if _, err := os.Stat(path); err != nil {
		return false
	}
	return true
}

func main() {
	ka, err := readArtifactConfig(".out-of-tree.toml")
	if err != nil {
		log.Fatalln(err)
	}

	kcfgEnv := "OUT_OF_TREE_KERNELS_CONFIG"
	kcfgPath := os.Getenv(kcfgEnv)
	if !exists(kcfgPath) {
		kcfgPath = os.Getenv("GOPATH") + "/src/github.com/jollheef/" +
			"out-of-tree/tools/kernel-factory/output/kernels.toml"
	}
	if !exists(kcfgPath) {
		log.Fatalln("Please specify kernels config path in " + kcfgEnv)
	}

	kcfg, err := readKernelConfig(kcfgPath)
	if err != nil {
		log.Fatalln(err)
	}

	err = performCI(ka, kcfg)
	if err != nil {
		log.Fatalln(err)
	}
}
