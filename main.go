// Copyright 2018 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package main

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"sort"
	"strconv"
	"time"

	kingpin "gopkg.in/alecthomas/kingpin.v2"

	"code.dumpstack.io/tools/out-of-tree/config"
)

func findFallback(kcfg config.KernelConfig, ki config.KernelInfo) (rootfs string) {
	for _, k := range kcfg.Kernels {
		if !exists(k.RootFS) || k.DistroType != ki.DistroType {
			continue
		}
		if k.RootFS < ki.RootFS {
			rootfs = k.RootFS
			return
		}
	}
	return
}

func handleFallbacks(kcfg config.KernelConfig) {
	sort.Sort(sort.Reverse(config.ByRootFS(kcfg.Kernels)))

	for i, k := range kcfg.Kernels {
		if !exists(k.RootFS) {
			newRootFS := findFallback(kcfg, k)

			s := k.RootFS + " does not exists "
			if newRootFS != "" {
				s += "(fallback to " + newRootFS + ")"
			} else {
				s += "(no fallback found)"
			}

			kcfg.Kernels[i].RootFS = newRootFS
			log.Println(s)
		}
	}
}

func checkRequiredUtils() (err error) {
	// Check for required commands
	for _, cmd := range []string{"docker", "qemu-system-x86_64"} {
		_, err := exec.Command("which", cmd).CombinedOutput()
		if err != nil {
			return fmt.Errorf("Command not found: %s", cmd)
		}
	}
	return
}

func checkDockerPermissions() (err error) {
	output, err := exec.Command("docker", "ps").CombinedOutput()
	if err != nil {
		err = fmt.Errorf("%s", output)
	}
	return
}

func main() {
	log.SetFlags(log.Lshortfile)

	rand.Seed(time.Now().UnixNano())

	app := kingpin.New(
		"out-of-tree",
		"kernel {module, exploit} development tool",
	)

	app.Author("Mikhail Klementev <root@dumpstack.io>")
	app.Version("0.2.0")

	pathFlag := app.Flag("path", "Path to work directory")
	path := pathFlag.Default(".").ExistingDir()

	usr, err := user.Current()
	if err != nil {
		return
	}
	os.MkdirAll(usr.HomeDir+"/.out-of-tree", os.ModePerm)

	defaultKcfgPath := usr.HomeDir + "/.out-of-tree/kernels.toml"

	kcfgPathFlag := app.Flag("kernels", "Path to main kernels config")
	kcfgPath := kcfgPathFlag.Default(defaultKcfgPath).String()

	defaultDbPath := usr.HomeDir + "/.out-of-tree/db.sqlite"
	dbPathFlag := app.Flag("db", "Path to database")
	dbPath := dbPathFlag.Default(defaultDbPath).String()

	defaultUserKcfgPath := usr.HomeDir + "/.out-of-tree/kernels.user.toml"
	userKcfgPathFlag := app.Flag("user-kernels", "User kernels config")
	userKcfgPathEnv := userKcfgPathFlag.Envar("OUT_OF_TREE_KCFG")
	userKcfgPath := userKcfgPathEnv.Default(defaultUserKcfgPath).String()

	qemuTimeoutFlag := app.Flag("qemu-timeout", "Timeout for qemu")
	qemuTimeout := qemuTimeoutFlag.Default("1m").Duration()

	dockerTimeoutFlag := app.Flag("docker-timeout", "Timeout for docker")
	dockerTimeout := dockerTimeoutFlag.Default("1m").Duration()

	pewCommand := app.Command("pew", "Build, run and test module/exploit")

	pewMax := pewCommand.Flag("max", "Test no more than X kernels").
		PlaceHolder("X").Default(fmt.Sprint(kernelsAll)).Int64()

	pewRuns := pewCommand.Flag("runs", "Runs per each kernel").
		Default("1").Int64()

	pewKernelFlag := pewCommand.Flag("kernel", "Override kernel regex")
	pewKernel := pewKernelFlag.String()

	pewGuessFlag := pewCommand.Flag("guess", "Try all defined kernels")
	pewGuess := pewGuessFlag.Bool()

	pewBinaryFlag := pewCommand.Flag("binary", "Use binary, do not build")
	pewBinary := pewBinaryFlag.String()

	pewTestFlag := pewCommand.Flag("test", "Override path test")
	pewTest := pewTestFlag.String()

	pewDistFlag := pewCommand.Flag("dist", "Build result path")
	pewDist := pewDistFlag.Default(pathDevNull).String()

	pewThreadsFlag := pewCommand.Flag("threads", "Build result path")
	pewThreads := pewThreadsFlag.Default(strconv.Itoa(runtime.NumCPU())).Int()

	pewTagFlag := pewCommand.Flag("tag", "Log tagging")
	pewTag := pewTagFlag.String()

	kernelCommand := app.Command("kernel", "Manipulate kernels")
	kernelUseHost := kernelCommand.Flag("host", "Use also host kernels").Bool()
	kernelListCommand := kernelCommand.Command("list", "List kernels")
	kernelAutogenCommand := kernelCommand.Command("autogen",
		"Generate kernels based on a current config")
	kernelAutogenMax := kernelAutogenCommand.Flag("max",
		"Download random kernels from set defined by regex in "+
			"release_mask, but no more than X for each of "+
			"release_mask").PlaceHolder("X").Default(
		fmt.Sprint(kernelsAll)).Int64()
	kernelDockerRegenCommand := kernelCommand.Command("docker-regen",
		"Regenerate kernels config from out_of_tree_* docker images")
	kernelGenallCommand := kernelCommand.Command("genall",
		"Generate all kernels for distro")

	genallDistroFlag := kernelGenallCommand.Flag("distro", "Distributive")
	distro := genallDistroFlag.Required().String()

	genallVerFlag := kernelGenallCommand.Flag("ver", "Distro version")
	version := genallVerFlag.Required().String()

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

	yekaslr := debugCommand.Flag("enable-kaslr", "Enable KASLR").Bool()
	yesmep := debugCommand.Flag("enable-smep", "Enable SMEP").Bool()
	yesmap := debugCommand.Flag("enable-smap", "Enable SMAP").Bool()
	yekpti := debugCommand.Flag("enable-kpti", "Enable KPTI").Bool()

	nokaslr := debugCommand.Flag("disable-kaslr", "Disable KASLR").Bool()
	nosmep := debugCommand.Flag("disable-smep", "Disable SMEP").Bool()
	nosmap := debugCommand.Flag("disable-smap", "Disable SMAP").Bool()
	nokpti := debugCommand.Flag("disable-kpti", "Disable KPTI").Bool()

	bootstrapCommand := app.Command("bootstrap",
		"Create directories && download images")

	logCommand := app.Command("log", "Logs")

	logQueryCommand := logCommand.Command("query", "Query logs")
	logNum := logQueryCommand.Flag("num", "How much lines").Default("50").Int()
	logRate := logQueryCommand.Flag("rate", "Show artifact success rate").Bool()
	logTag := logQueryCommand.Flag("tag", "Filter tag").String()

	logDumpCommand := logCommand.Command("dump",
		"Show all info for log entry with ID")
	logDumpID := logDumpCommand.Arg("ID", "").Required().Int()

	logJSONCommand := logCommand.Command("json", "Generate json statistics")
	logJSONTag := logJSONCommand.Flag("tag", "Filter tag").Required().String()

	logMarkdownCommand := logCommand.Command("markdown", "Generate markdown statistics")
	logMarkdownTag := logMarkdownCommand.Flag("tag", "Filter tag").Required().String()

	packCommand := app.Command("pack", "Exploit pack test")
	packAutogen := packCommand.Flag("autogen", "Kernel autogeneration").Bool()
	packExploitRuns := packCommand.Flag("exploit-runs",
		"Amount of runs of each exploit").Default("4").Int64()
	packKernelRuns := packCommand.Flag("kernel-runs",
		"Amount of runs of each kernel").Default("1").Int64()

	err = checkRequiredUtils()
	if err != nil {
		log.Fatalln(err)
	}

	err = checkDockerPermissions()
	if err != nil {
		log.Println(err)
		log.Println("You have two options:")
		log.Println("\t1. Add user to group docker;")
		log.Println("\t2. Run out-of-tree with sudo.")
		os.Exit(1)
	}

	if !exists(usr.HomeDir + "/.out-of-tree/images") {
		log.Println("No ~/.out-of-tree/images: " +
			"Probably you need to run `out-of-tree bootstrap`" +
			" for downloading basic images")
	}

	if !exists(usr.HomeDir + "/.out-of-tree/kernels.toml") {
		log.Println("No ~/.out-of-tree/kernels.toml: Probably you " +
			"need to run `out-of-tree kernel autogen` in " +
			"directory that contains .out-of-tree.toml " +
			"with defined kernel masks " +
			"(see docs at https://out-of-tree.io)")
	}

	kingpin.MustParse(app.Parse(os.Args[1:]))

	if *yekaslr && *nokaslr {
		log.Fatalln("Only one of disable/enable can be used at once")
	}

	if *yesmep && *nosmep {
		log.Fatalln("Only one of disable/enable can be used at once")
	}

	if *yesmap && *nosmap {
		log.Fatalln("Only one of disable/enable can be used at once")
	}

	kcfg, err := config.ReadKernelConfig(*kcfgPath)
	if err != nil {
		log.Println(err)
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

	handleFallbacks(kcfg)

	db, err := openDatabase(*dbPath)
	if err != nil {
		log.Fatalln(err)
	}
	defer db.Close()

	switch kingpin.MustParse(app.Parse(os.Args[1:])) {
	case pewCommand.FullCommand():
		err = pewHandler(kcfg, *path, *pewKernel, *pewBinary,
			*pewTest, *pewGuess, *qemuTimeout, *dockerTimeout,
			*pewMax, *pewRuns, *pewDist, *pewTag, *pewThreads, db)
	case kernelListCommand.FullCommand():
		err = kernelListHandler(kcfg)
	case kernelAutogenCommand.FullCommand():
		err = kernelAutogenHandler(*path, *kernelAutogenMax, *kernelUseHost)
	case kernelDockerRegenCommand.FullCommand():
		err = kernelDockerRegenHandler(*kernelUseHost)
	case kernelGenallCommand.FullCommand():
		err = kernelGenallHandler(*distro, *version, *kernelUseHost)
	case genModuleCommand.FullCommand():
		err = genConfig(config.KernelModule)
	case genExploitCommand.FullCommand():
		err = genConfig(config.KernelExploit)
	case debugCommand.FullCommand():
		err = debugHandler(kcfg, *path, *debugKernel, *debugGDB,
			*dockerTimeout, *yekaslr, *yesmep, *yesmap, *yekpti,
			*nokaslr, *nosmep, *nosmap, *nokpti)
	case bootstrapCommand.FullCommand():
		err = bootstrapHandler()
	case logQueryCommand.FullCommand():
		err = logHandler(db, *path, *logTag, *logNum, *logRate)
	case logDumpCommand.FullCommand():
		err = logDumpHandler(db, *logDumpID)
	case logJSONCommand.FullCommand():
		err = logJSONHandler(db, *path, *logJSONTag)
	case logMarkdownCommand.FullCommand():
		err = logMarkdownHandler(db, *path, *logMarkdownTag)
	case packCommand.FullCommand():
		err = packHandler(db, *path, kcfg, *packAutogen,
			*packExploitRuns, *packKernelRuns)
	}

	if err != nil {
		log.Fatalln(err)
	}

	if somethingFailed {
		os.Exit(1)
	}
}
