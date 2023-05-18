// Copyright 2018 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"gopkg.in/logrusorgru/aurora.v2"

	"code.dumpstack.io/tools/out-of-tree/config"
	"code.dumpstack.io/tools/out-of-tree/fs"
	"code.dumpstack.io/tools/out-of-tree/qemu"
)

type DebugCmd struct {
	Kernel string `help:"regexp (first match)" required:""`
	Gdb    string `help:"gdb listen address" default:"tcp::1234"`

	SshAddr string `help:"ssh address to listen" default:"127.0.0.1"`
	SshPort int    `help:"ssh port to listen" default:"50022"`

	ArtifactConfig string `help:"path to artifact config" type:"path"`

	Kaslr bool `help:"Enable KASLR"`
	Smep  bool `help:"Enable SMEP"`
	Smap  bool `help:"Enable SMAP"`
	Kpti  bool `help:"Enable KPTI"`

	NoKaslr bool `help:"Disable KASLR"`
	NoSmep  bool `help:"Disable SMEP"`
	NoSmap  bool `help:"Disable SMAP"`
	NoKpti  bool `help:"Disable KPTI"`
}

// TODO: merge with pew.go
func (cmd *DebugCmd) Run(g *Globals) (err error) {
	kcfg, err := config.ReadKernelConfig(g.Config.Kernels)
	if err != nil {
		log.Print(err)
	}

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

	ki, err := firstSupported(kcfg, ka, cmd.Kernel)
	if err != nil {
		return
	}

	kernel := qemu.Kernel{KernelPath: ki.KernelPath, InitrdPath: ki.InitrdPath}
	q, err := qemu.NewSystem(qemu.X86x64, kernel, ki.RootFS)
	if err != nil {
		return
	}

	err = q.SetSSHAddrPort(cmd.SshAddr, cmd.SshPort)
	if err != nil {
		return
	}

	if ka.Qemu.Cpus != 0 {
		q.Cpus = ka.Qemu.Cpus
	}
	if ka.Qemu.Memory != 0 {
		q.Memory = ka.Qemu.Memory
	}

	if ka.Docker.Timeout.Duration != 0 {
		g.Config.Docker.Timeout.Duration = ka.Docker.Timeout.Duration
	}

	q.SetKASLR(false) // set KASLR to false by default because of gdb
	q.SetSMEP(!ka.Mitigations.DisableSmep)
	q.SetSMAP(!ka.Mitigations.DisableSmap)
	q.SetKPTI(!ka.Mitigations.DisableKpti)

	if cmd.Kaslr {
		q.SetKASLR(true)
	} else if cmd.NoKaslr {
		q.SetKASLR(false)
	}

	if cmd.Smep {
		q.SetSMEP(true)
	} else if cmd.NoSmep {
		q.SetSMEP(false)
	}

	if cmd.Smap {
		q.SetSMAP(true)
	} else if cmd.NoSmap {
		q.SetSMAP(false)
	}

	if cmd.Kpti {
		q.SetKPTI(true)
	} else if cmd.NoKpti {
		q.SetKPTI(false)
	}

	redgreen := func(name string, enabled bool) aurora.Value {
		if enabled {
			return aurora.BgGreen(aurora.Black(name))
		}

		return aurora.BgRed(aurora.White(name))
	}

	fmt.Printf("[*] %s %s %s %s\n",
		redgreen("KASLR", q.GetKASLR()),
		redgreen("SMEP", q.GetSMEP()),
		redgreen("SMAP", q.GetSMAP()),
		redgreen("KPTI", q.GetKPTI()))

	fmt.Printf("[*] SMP: %d CPUs\n", q.Cpus)
	fmt.Printf("[*] Memory: %d MB\n", q.Memory)

	q.Debug(cmd.Gdb)
	coloredGdbAddress := aurora.BgGreen(aurora.Black(cmd.Gdb))
	fmt.Printf("[*] gdb is listening on %s\n", coloredGdbAddress)

	err = q.Start()
	if err != nil {
		return
	}
	defer q.Stop()

	tmp, err := fs.TempDir()
	if err != nil {
		return
	}
	defer os.RemoveAll(tmp)

	err = q.WaitForSSH(time.Minute)
	if err != nil {
		return
	}

	if ka.StandardModules {
		// Module depends on one of the standard modules
		err = copyStandardModules(q, ki)
		if err != nil {
			log.Print(err)
			return
		}
	}

	err = preloadModules(q, ka, ki, g.Config.Docker.Timeout.Duration)
	if err != nil {
		log.Print(err)
		return
	}

	var buildDir, outFile, output, remoteFile string

	if ka.Type == config.Script {
		err = q.CopyFile("root", ka.Script, ka.Script)
		if err != nil {
			return
		}
	} else {
		buildDir, outFile, output, err = build(log.Logger, tmp, ka, ki, g.Config.Docker.Timeout.Duration)
		if err != nil {
			log.Print(err, output)
			return
		}

		remoteFile = "/tmp/exploit"
		if ka.Type == config.KernelModule {
			remoteFile = "/tmp/module.ko"
		}

		err = q.CopyFile("user", outFile, remoteFile)
		if err != nil {
			return
		}
	}

	// Copy all test files to the remote machine
	for _, f := range ka.TestFiles {
		if f.Local[0] != '/' {
			if buildDir != "" {
				f.Local = buildDir + "/" + f.Local
			}
		}
		err = q.CopyFile(f.User, f.Local, f.Remote)
		if err != nil {
			log.Print("error copy err:", err, f.Local, f.Remote)
			return
		}
	}

	coloredRemoteFile := aurora.BgGreen(aurora.Black(remoteFile))
	fmt.Printf("[*] build result copied to %s\n", coloredRemoteFile)

	fmt.Printf("\n%s\n", q.GetSSHCommand())
	fmt.Printf("gdb %s -ex 'target remote %s'\n\n", ki.VmlinuxPath, cmd.Gdb)

	// TODO set substitute-path /build/.../linux-... /path/to/linux-source

	err = interactive(q)
	return
}

func firstSupported(kcfg config.KernelConfig, ka config.Artifact,
	kernel string) (ki config.KernelInfo, err error) {

	km, err := kernelMask(kernel)
	if err != nil {
		return
	}

	ka.Targets = []config.Target{km}

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
		fmt.Println(q.Stdout)
	case "cl", "clog":
		fmt.Println(q.Stdout)
		q.Stdout = ""
	case "c", "cleanup":
		q.Stdout = ""
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
