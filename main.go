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
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/logrusorgru/aurora"
	"github.com/naoina/toml"
	"github.com/otiai10/copy"
	"github.com/remeh/sizedwaitgroup"
	kingpin "gopkg.in/alecthomas/kingpin.v2"

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

func (at *artifactType) UnmarshalTOML(data []byte) (err error) {
	stype := strings.Trim(string(data), `"`)
	stypelower := strings.ToLower(stype)
	if strings.Contains(stypelower, "module") {
		*at = KernelModule
	} else if strings.Contains(stypelower, "exploit") {
		*at = KernelExploit
	} else {
		err = errors.New(fmt.Sprintf("Type %s is unsupported", stype))
	}
	return
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
		if supported {
			break
		}

	}
	return
}

type distroType int

const (
	Ubuntu distroType = iota
	CentOS
	Debian
)

var distroTypeStrings = [...]string{"Ubuntu", "CentOS", "Debian"}

func (dt distroType) String() string {
	return distroTypeStrings[dt]
}

func (dt *distroType) UnmarshalTOML(data []byte) (err error) {
	sDistro := strings.Trim(string(data), `"`)
	if strings.EqualFold(sDistro, "Ubuntu") {
		*dt = Ubuntu
	} else if strings.EqualFold(sDistro, "CentOS") {
		*dt = CentOS
	} else if strings.EqualFold(sDistro, "Debian") {
		*dt = Debian
	} else {
		err = errors.New(fmt.Sprintf("Distro %s is unsupported", sDistro))
	}
	return
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

func build(tmp string, ka artifact, ki kernelInfo) (outPath, output string, err error) {
	target := fmt.Sprintf("%d_%s", rand.Int(), ki.KernelRelease)

	tmpSourcePath := tmp + "/source"

	err = copy.Copy(ka.SourcePath, tmpSourcePath)
	if err != nil {
		return
	}

	outPath = tmpSourcePath + "/" + target
	if ka.Type == KernelModule {
		outPath += ".ko"
	}

	kernel := "/lib/modules/" + ki.KernelRelease + "/build"

	cmd := dockerCommand(ki.ContainerName, tmpSourcePath, "1m", // TODO CFG
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

func testKernelModule(q *qemu.QemuSystem, ka artifact, test string) (output string, err error) {
	output, err = q.Command("root", test)
	// TODO generic checks for WARNING's and so on
	return
}

func testKernelExploit(q *qemu.QemuSystem, ka artifact, test, exploit string) (output string, err error) {
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

func dumpResult(q *qemu.QemuSystem, ka artifact, ki kernelInfo, build_ok, run_ok, test_ok *bool) {
	distroInfo := fmt.Sprintf("%s-%s {%s}", ki.DistroType,
		ki.DistroRelease, ki.KernelRelease)

	colored := ""
	if ka.Type == KernelExploit {
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
	defer dumpResult(q, ka, ki, &build_ok, &run_ok, &test_ok)

	// TODO Write build log to file or database
	outFile, output, err := build(tmp, ka, ki)
	if err != nil {
		log.Println(output)
		return
	}
	build_ok = true

	err = cleanDmesg(q)
	if err != nil {
		return
	}

	testPath := outFile + "_test"

	remoteTest := fmt.Sprintf("/tmp/test_%d", rand.Int())
	err = q.CopyFile("user", testPath, remoteTest)
	if err != nil {
		return
	}

	_, err = q.Command("root", "chmod +x "+remoteTest)
	if err != nil {
		return
	}

	if ka.Type == KernelModule {
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
	} else if ka.Type == KernelExploit {
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

func pewHandler(workPath, kcfgPath, overridedKernel string) (err error) {
	ka, err := readArtifactConfig(workPath + "/.out-of-tree.toml")
	if err != nil {
		return
	}

	if ka.SourcePath == "" {
		ka.SourcePath = workPath
	}

	if overridedKernel != "" {
		parts := strings.Split(overridedKernel, ":")
		if len(parts) != 2 {
			return errors.New("Kernel is not 'distroType:regex'")
		}

		var dt distroType
		err = dt.UnmarshalTOML([]byte(parts[0]))
		if err != nil {
			return
		}

		km := kernelMask{DistroType: dt, ReleaseMask: parts[1]}
		ka.SupportedKernels = []kernelMask{km}
	}

	kcfg, err := readKernelConfig(kcfgPath)
	if err != nil {
		return
	}

	err = performCI(ka, kcfg)
	if err != nil {
		return
	}

	return
}

func main() {
	app := kingpin.New(
		"out-of-tree",
		"kernel {module, exploit} development tool",
	)

	app.Author("Mikhail Klementev <jollheef@riseup.net>")
	app.Version("0.1.0")

	pathFlag := app.Flag("path", "Path to work directory")
	path := pathFlag.Default(".").ExistingDir()

	kcfgFlag := app.Flag("kernels", "Path to kernels config")
	kcfg := kcfgFlag.Envar("OUT_OF_TREE_KCFG").Required().ExistingFile()

	pewCommand := app.Command("pew", "Build, run and test module/exploit")
	pewKernelFlag := pewCommand.Flag("kernel", "Override kernel regex")
	pewKernel := pewKernelFlag.String()

	var err error
	switch kingpin.MustParse(app.Parse(os.Args[1:])) {
	case pewCommand.FullCommand():
		err = pewHandler(*path, *kcfg, *pewKernel)
	}

	if err != nil {
		log.Println(err)
	}
}
