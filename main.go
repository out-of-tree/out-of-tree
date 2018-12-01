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

	kcfgPathFlag := app.Flag("kernels", "Path to main kernels config")
	kcfgPath := kcfgPathFlag.Default(defaultKcfgPath).ExistingFile()

	defaultUserKcfgPath := usr.HomeDir + "/.out-of-tree/kernels.user.toml"
	userKcfgPathFlag := app.Flag("user-kernels", "User kernels config")
	userKcfgPathEnv := userKcfgPathFlag.Envar("OUT_OF_TREE_KCFG")
	userKcfgPath := userKcfgPathEnv.Default(defaultUserKcfgPath).String()

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
	kernelAutogenCommand := kernelCommand.Command("autogen",
		"Generate kernels based on a current config")
	kernelDockerRegenCommand := kernelCommand.Command("docker-regen",
		"Regenerate kernels config from out_of_tree_* docker images")

	genCommand := app.Command("gen", "Generate .out-of-tree.toml skeleton")
	genModuleCommand := genCommand.Command("module",
		"Generate .out-of-tree.toml skeleton for kernel module")
	genExploitCommand := genCommand.Command("exploit",
		"Generate .out-of-tree.toml skeleton for kernel exploit")

	debugCommand := app.Command("debug", "Kernel debug environment")
	debugCommandFlag := debugCommand.Flag("kernel", "Regex (first match)")
	debugKernel := debugCommandFlag.Required().String()
	debugFlagGDB := debugCommand.Flag("gdb", "Set gdb listen address")
	debugGDB := debugFlagGDB.Default("tcp::1234").String()

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

	if exists(*userKcfgPath) {
		userKcfg, err := config.ReadKernelConfig(*userKcfgPath)
		if err != nil {
			log.Fatalln(err)
		}

		for _, nk := range userKcfg.Kernels {
			if !hasKernel(nk, kcfg) {
				kcfg.Kernels = append(kcfg.Kernels, nk)
			}
		}
	}

	switch kingpin.MustParse(app.Parse(os.Args[1:])) {
	case pewCommand.FullCommand():
		err = pewHandler(kcfg, *path, *pewKernel, *pewBinary,
			*pewTest, *pewGuess, *qemuTimeout, *dockerTimeout)
	case kernelListCommand.FullCommand():
		err = kernelListHandler(kcfg)
	case kernelAutogenCommand.FullCommand():
		err = kernelAutogenHandler(*path)
	case kernelDockerRegenCommand.FullCommand():
		err = kernelDockerRegenHandler()
	case genModuleCommand.FullCommand():
		err = genConfig(config.KernelModule)
	case genExploitCommand.FullCommand():
		err = genConfig(config.KernelExploit)
	case debugCommand.FullCommand():
		err = debugHandler(kcfg, *path, *debugKernel, *debugGDB,
			*dockerTimeout)
	}

	if err != nil {
		log.Fatalln(err)
	}
}
