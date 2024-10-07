// Copyright 2024 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package cmd

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"

	"github.com/rs/zerolog/log"

	"code.dumpstack.io/tools/out-of-tree/container"
	"code.dumpstack.io/tools/out-of-tree/distro"
)

type ContainerCmd struct {
	DistroID      string `help:"filter by distribution"`
	DistroRelease string `help:"filter by distribution release"`

	List    ContainerListCmd    `cmd:"" help:"list containers"`
	Update  ContainerUpdateCmd  `cmd:"" help:"update containers"`
	Save    ContainerSaveCmd    `cmd:"" help:"save containers"`
	Cleanup ContainerCleanupCmd `cmd:"" help:"cleanup containers"`

	RealtimeOutput RealtimeContainerOutputFlag `help:"show realtime output"`
}

type RealtimeContainerOutputFlag bool

func (f RealtimeContainerOutputFlag) AfterApply() (err error) {
	container.Stdout = bool(f)
	return
}

func (cmd ContainerCmd) Containers() (diis []container.Image, err error) {
	images, err := container.Images()
	if err != nil {
		return
	}

	var dt distro.Distro
	if cmd.DistroID != "" {
		dt.ID, err = distro.NewID(cmd.DistroID)
		if err != nil {
			return
		}

		if cmd.DistroRelease != "" {
			dt.Release = cmd.DistroRelease
		}
	} else if cmd.DistroRelease != "" {
		err = errors.New("--distro-release has no use on its own")
		return
	}

	for _, img := range images {
		if dt.ID != distro.None && dt.ID != img.Distro.ID {
			log.Debug().Msgf("skip %s", img.Name)
			continue
		}

		if dt.Release != "" && dt.Release != img.Distro.Release {
			log.Debug().Msgf("skip %s", img.Name)
			continue
		}

		log.Debug().Msgf("append %s", img.Name)
		diis = append(diis, img)
	}
	return
}

type ContainerListCmd struct{}

func (cmd ContainerListCmd) Run(containerCmd *ContainerCmd) (err error) {
	images, err := containerCmd.Containers()
	if err != nil {
		return
	}

	for _, img := range images {
		fmt.Printf("%s\n", img.Distro.String())
	}
	return
}

type ContainerUpdateCmd struct{}

func (cmd ContainerUpdateCmd) Run(g *Globals, containerCmd *ContainerCmd) (err error) {
	images, err := containerCmd.Containers()
	if err != nil {
		return
	}

	container.UseCache = false
	container.UsePrebuilt = false

	// TODO move from all commands to main command line handler
	container.Commands = g.Config.Docker.Commands
	container.Registry = g.Config.Docker.Registry
	container.Timeout = g.Config.Docker.Timeout.Duration

	for _, img := range images {
		_, err = img.Distro.Packages()
		if err != nil {
			return
		}
	}

	return
}

type ContainerSaveCmd struct {
	OutDir string `help:"directory to save containers" default:"./" type:"existingdir"`
}

func (cmd ContainerSaveCmd) Run(containerCmd *ContainerCmd) (err error) {
	images, err := containerCmd.Containers()
	if err != nil {
		return
	}

	for _, img := range images {
		nlog := log.With().Str("name", img.Name).Logger()

		output := filepath.Join(cmd.OutDir, img.Name+".tar")
		nlog.Info().Msgf("saving to %v", output)

		err = container.Save(img.Name, output)
		if err != nil {
			return
		}

		compressed := output + ".gz"
		nlog.Info().Msgf("compressing to %v", compressed)

		var raw []byte
		raw, err = exec.Command("gzip", output).CombinedOutput()
		if err != nil {
			nlog.Error().Err(err).Msg(string(raw))
			return
		}

		nlog.Info().Msg("done")
	}
	return
}

type ContainerCleanupCmd struct{}

func (cmd ContainerCleanupCmd) Run(containerCmd *ContainerCmd) (err error) {
	images, err := containerCmd.Containers()
	if err != nil {
		return
	}

	var output []byte
	for _, img := range images {
		output, err = exec.Command(container.Runtime, "image", "rm", img.Name).
			CombinedOutput()
		if err != nil {
			log.Error().Err(err).Str("output", string(output)).Msg("")
			return
		}
	}
	return
}
