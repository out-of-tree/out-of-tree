// Copyright 2023 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package cmd

import (
	"fmt"
	"os"
	"time"

	"code.dumpstack.io/tools/out-of-tree/fs"

	"github.com/rs/zerolog/log"
)

type PackCmd struct {
	Autogen     bool  `help:"kernel autogeneration"`
	UseHost     bool  `help:"also use host kernels"`
	NoDownload  bool  `help:"do not download qemu image while kernel generation"`
	ExploitRuns int64 `default:"4" help:"amount of runs of each exploit"`
	KernelRuns  int64 `default:"1" help:"amount of runs of each kernel"`
	Max         int   `help:"download random kernels from set defined by regex in release_mask, but no more than X for each of release_mask" default:"1"`

	Threads int `help:"threads" default:"4"`

	Tag string `help:"filter tag"`

	Timeout       time.Duration `help:"timeout after tool will not spawn new tests"`
	QemuTimeout   time.Duration `help:"timeout for qemu"`
	DockerTimeout time.Duration `help:"timeout for docker"`
}

func (cmd *PackCmd) Run(g *Globals) (err error) {
	tag := fmt.Sprintf("pack_run_%d", time.Now().Unix())
	log.Info().Msgf("Tag: %s", tag)

	files, err := os.ReadDir(g.WorkDir)
	if err != nil {
		return
	}

	for _, f := range files {
		workPath := g.WorkDir + "/" + f.Name()

		if !fs.PathExists(workPath + "/.out-of-tree.toml") {
			continue
		}

		if cmd.Autogen {
			autogen := KernelAutogenCmd{}
			err = autogen.Run(
				&KernelCmd{
					NoDownload: cmd.NoDownload,
					UseHost:    cmd.UseHost,
					Max:        cmd.Max,
				},
				&Globals{
					Config:  g.Config,
					WorkDir: workPath,
				},
			)
			if err != nil {
				return
			}
		}

		log.Info().Msg(f.Name())

		pew := PewCmd{
			Max:           cmd.KernelRuns,
			Runs:          cmd.ExploitRuns,
			Threads:       cmd.Threads,
			Tag:           tag,
			Timeout:       cmd.Timeout,
			QemuTimeout:   cmd.QemuTimeout,
			DockerTimeout: cmd.DockerTimeout,
			Dist:          pathDevNull,
		}

		pew.Run(&Globals{
			Config:  g.Config,
			WorkDir: workPath,
		})
	}

	return
}
