// Copyright 2018 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"time"

	"gopkg.in/logrusorgru/aurora.v1"

	"code.dumpstack.io/tools/out-of-tree/config"
	"code.dumpstack.io/tools/out-of-tree/qemu"
)

func firstSupported(kcfg config.KernelConfig, ka config.Artifact,
	kernel string) (ki config.KernelInfo, err error) {

	km, err := kernelMask(kernel)
	if err != nil {
		return
	}

	ka.SupportedKernels = []config.KernelMask{km}

	for _, ki = range kcfg.Kernels {
		var supported bool
		supported, err = ka.Supported(ki)
		if err != nil || supported {
			return
		}
	}

	err = errors.New("No supported kernel found")
	return
}

func handleLine(q *qemu.System) (err error) {
	fmt.Print("out-of-tree> ")
	rawLine := "help"
	fmt.Scanf("%s", &rawLine)
	params := strings.Fields(rawLine)
	cmd := params[0]

	switch cmd {
	case "h", "help":
		fmt.Printf("help\t: print this help message\n")
		fmt.Printf("log\t: print qemu log\n")
		fmt.Printf("clog\t: print qemu log and cleanup buffer\n")
		fmt.Printf("cleanup\t: cleanup qemu log buffer\n")
		fmt.Printf("ssh\t: print arguments to ssh command\n")
		fmt.Printf("quit\t: quit\n")
	case "l", "log":
		fmt.Println(string(q.Stdout))
	case "cl", "clog":
		fmt.Println(string(q.Stdout))
		q.Stdout = []byte{}
	case "c", "cleanup":
		q.Stdout = []byte{}
	case "s", "ssh":
		fmt.Println(q.GetSSHCommand())
	case "q", "quit":
		return errors.New("end of session")
	default:
		fmt.Println("No such command")
	}
	return
}

func interactive(q *qemu.System) (err error) {
	for {
		err = handleLine(q)
		if err != nil {
			return
		}
	}
}

func debugHandler(kcfg config.KernelConfig, workPath, kernRegex, gdb string,
	dockerTimeout time.Duration, yekaslr, yesmep, yesmap, yekpti,
	nokaslr, nosmep, nosmap, nokpti bool) (err error) {

	ka, err := config.ReadArtifactConfig(workPath + "/.out-of-tree.toml")
	if err != nil {
		return
	}

	if ka.SourcePath == "" {
		ka.SourcePath = workPath
	}

	ki, err := firstSupported(kcfg, ka, kernRegex)
	if err != nil {
		return
	}

	kernel := qemu.Kernel{KernelPath: ki.KernelPath, InitrdPath: ki.InitrdPath}
	q, err := qemu.NewSystem(qemu.X86x64, kernel, ki.RootFS)
	if err != nil {
		return
	}

	if ka.Qemu.Cpus != 0 {
		q.Cpus = ka.Qemu.Cpus
	}
	if ka.Qemu.Memory != 0 {
		q.Memory = ka.Qemu.Memory
	}

	q.SetKASLR(false) // set KASLR to false by default because of gdb
	q.SetSMEP(!ka.Mitigations.DisableSmep)
	q.SetSMAP(!ka.Mitigations.DisableSmap)
	q.SetKPTI(!ka.Mitigations.DisableKpti)

	if yekaslr {
		q.SetKASLR(true)
	} else if nokaslr {
		q.SetKASLR(false)
	}

	if yesmep {
		q.SetSMEP(true)
	} else if nosmep {
		q.SetSMEP(false)
	}

	if yesmap {
		q.SetSMAP(true)
	} else if nosmap {
		q.SetSMAP(false)
	}

	if yekpti {
		q.SetKPTI(true)
	} else if nokpti {
		q.SetKPTI(false)
	}

	redgreen := func(name string, enabled bool) aurora.Value {
		if enabled {
			return aurora.BgGreen(aurora.Black(name))
		}

		return aurora.BgRed(aurora.Gray(name))
	}

	fmt.Printf("[*] %s %s %s %s\n",
		redgreen("KASLR", q.GetKASLR()),
		redgreen("SMEP", q.GetSMEP()),
		redgreen("SMAP", q.GetSMAP()),
		redgreen("KPTI", q.GetKPTI()))

	fmt.Printf("[*] SMP: %d CPUs\n", q.Cpus)
	fmt.Printf("[*] Memory: %d MB\n", q.Memory)

	q.Debug(gdb)
	coloredGdbAddress := aurora.BgGreen(aurora.Black(gdb))
	fmt.Printf("[*] gdb is listening on %s\n", coloredGdbAddress)

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

	outFile, output, err := build(tmp, ka, ki, dockerTimeout)
	if err != nil {
		log.Println(err, output)
		return
	}

	remoteFile := "/tmp/exploit"
	if ka.Type == config.KernelModule {
		remoteFile = "/tmp/module.ko"
	}

	err = q.CopyFile("user", outFile, remoteFile)
	if err != nil {
		return
	}

	coloredRemoteFile := aurora.BgGreen(aurora.Black(remoteFile))
	fmt.Printf("[*] build result copied to %s\n", coloredRemoteFile)

	fmt.Printf("\n%s\n", q.GetSSHCommand())
	fmt.Printf("gdb %s -ex 'target remote %s'\n\n", ki.VmlinuxPath, gdb)

	// TODO set substitute-path /build/.../linux-... /path/to/linux-source

	err = interactive(q)
	return
}
