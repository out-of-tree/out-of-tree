// Copyright 2018 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package main

import (
	"log"
	"os"
	"os/exec"
	"os/user"

	kingpin "gopkg.in/alecthomas/kingpin.v2"

	"github.com/jollheef/out-of-tree/config"
)

func main() {
	app := kingpin.New(
		"out-of-tree",
		"kernel {module, exploit} development tool",
	)

	app.Author("Mikhail Klementev <jollheef@riseup.net>")
	app.Version("0.1.0")

	pathFlag := app.Flag("path", "Path to work directory")
	path := pathFlag.Default(".").ExistingDir()

	usr, err := user.Current()
	if err != nil {
		return
	}
	defaultKcfgPath := usr.HomeDir + "/.out-of-tree/kernels.toml"

	kcfgPathFlag := app.Flag("kernels", "Path to kernels config")
	kcfgPathEnv := kcfgPathFlag.Envar("OUT_OF_TREE_KCFG")
	kcfgPath := kcfgPathEnv.Default(defaultKcfgPath).ExistingFile()

	qemuTimeoutFlag := app.Flag("qemu-timeout", "Timeout for qemu")
	qemuTimeout := qemuTimeoutFlag.Default("1m").Duration()

	dockerTimeoutFlag := app.Flag("docker-timeout", "Timeout for docker")
	dockerTimeout := dockerTimeoutFlag.Default("1m").Duration()
	pewCommand := app.Command("pew", "Build, run and test module/exploit")
	pewKernelFlag := pewCommand.Flag("kernel", "Override kernel regex")
	pewKernel := pewKernelFlag.String()

	pewGuessFlag := pewCommand.Flag("guess", "Try all defined kernels")
	pewGuess := pewGuessFlag.Bool()

	pewBinaryFlag := pewCommand.Flag("binary", "Use binary, do not build")
	pewBinary := pewBinaryFlag.String()

	pewTestFlag := pewCommand.Flag("test", "Override path test")
	pewTest := pewTestFlag.String()

	kernelCommand := app.Command("kernel", "Manipulate kernels")
	kernelListCommand := kernelCommand.Command("list", "List kernels")

	genCommand := app.Command("gen", "Generate .out-of-tree.toml skeleton")
	genModuleCommand := genCommand.Command("module",
		"Generate .out-of-tree.toml skeleton for kernel module")
	genExploitCommand := genCommand.Command("exploit",
		"Generate .out-of-tree.toml skeleton for kernel exploit")

	debugCommand := app.Command("debug", "Kernel debug environment")
	debugCommandFlag := debugCommand.Flag("kernel", "Regex (first match)")
	debugKernel := debugCommandFlag.Required().String()

	// Check for required commands
	for _, cmd := range []string{"timeout", "docker", "qemu"} {
		_, err := exec.Command("which", cmd).CombinedOutput()
		if err != nil {
			log.Fatalln("Command not found:", cmd)
		}
	}

	kingpin.MustParse(app.Parse(os.Args[1:]))

	kcfg, err := config.ReadKernelConfig(*kcfgPath)
	if err != nil {
		log.Fatalln(err)
	}

	switch kingpin.MustParse(app.Parse(os.Args[1:])) {
	case pewCommand.FullCommand():
		err = pewHandler(kcfg, *path, *pewKernel, *pewBinary,
			*pewTest, *pewGuess, *qemuTimeout, *dockerTimeout)
	case kernelListCommand.FullCommand():
		err = kernelListHandler(kcfg)
	case genModuleCommand.FullCommand():
		err = genConfig(config.KernelModule)
	case genExploitCommand.FullCommand():
		err = genConfig(config.KernelExploit)
	case debugCommand.FullCommand():
		err = debugHandler(kcfg, *path, *debugKernel, *dockerTimeout)
	}

	if err != nil {
		log.Fatalln(err)
	}
}
