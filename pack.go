// Copyright 2019 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"runtime"
	"time"

	"code.dumpstack.io/tools/out-of-tree/config"
)

type PackCmd struct {
	Autogen     bool  `help:"kernel autogeneration"`
	NoDownload  bool  `help:"do not download qemu image while kernel generation"`
	ExploitRuns int64 `default:"4" help:"amount of runs of each exploit"`
	KernelRuns  int64 `default:"1" help:"amount of runs of each kernel"`

	Tag string `help:"filter tag"`

	Timeout time.Duration `help:"timeout after tool will not spawn new tests"`
}

func (cmd *PackCmd) Run(g *Globals) (err error) {
	kcfg, err := config.ReadKernelConfig(g.Config.Kernels)
	if err != nil {
		log.Println(err)
	}

	db, err := openDatabase(g.Config.Database)
	if err != nil {
		log.Fatalln(err)
	}
	defer db.Close()

	stop := time.Time{} // never stop
	if cmd.Timeout != 0 {
		stop = time.Now().Add(cmd.Timeout)
	}

	threads := runtime.NumCPU()

	tag := fmt.Sprintf("pack_run_%d", time.Now().Unix())
	log.Println("Tag:", tag)

	files, err := ioutil.ReadDir(g.WorkDir)
	if err != nil {
		return
	}

	for _, f := range files {
		workPath := g.WorkDir + "/" + f.Name()

		if !exists(workPath + "/.out-of-tree.toml") {
			continue
		}

		if cmd.Autogen {
			var perRegex int64 = 1
			err = kernelAutogenHandler(workPath,
				g.Config.Docker.Registry,
				g.Config.Docker.Commands,
				perRegex, false, !cmd.NoDownload)
			if err != nil {
				return
			}
		}

		log.Println(f.Name())

		pewHandler(kcfg, workPath, "", "", "", false, stop,
			g.Config.Docker.Timeout.Duration,
			g.Config.Qemu.Timeout.Duration,
			cmd.KernelRuns, cmd.ExploitRuns, pathDevNull,
			tag, threads, db, false)
	}

	return
}
