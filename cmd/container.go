// Copyright 2023 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package cmd

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"

	"code.dumpstack.io/tools/out-of-tree/container"
)

type ContainerCmd struct {
	Filter string `help:"filter by name"`

	List    ContainerListCmd    `cmd:"" help:"list containers"`
	Save    ContainerSaveCmd    `cmd:"" help:"save containers"`
	Cleanup ContainerCleanupCmd `cmd:"" help:"cleanup containers"`
}

func (cmd ContainerCmd) Containers() (names []string) {
	images, err := container.Images()
	if err != nil {
		log.Fatal().Err(err).Msg("")
	}

	for _, img := range images {
		if cmd.Filter != "" && !strings.Contains(img.Name, cmd.Filter) {
			continue
		}
		names = append(names, img.Name)
	}
	return
}

type ContainerListCmd struct{}

func (cmd ContainerListCmd) Run(containerCmd *ContainerCmd) (err error) {
	for _, name := range containerCmd.Containers() {
		fmt.Println(name)
	}
	return
}

type ContainerSaveCmd struct {
	OutDir string `help:"directory to save containers" default:"./" type:"existingdir"`
}

func (cmd ContainerSaveCmd) Run(containerCmd *ContainerCmd) (err error) {
	for _, name := range containerCmd.Containers() {
		nlog := log.With().Str("name", name).Logger()

		output := filepath.Join(cmd.OutDir, name+".tar")
		nlog.Info().Msgf("saving to %v")

		err = container.Save(name, output)
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
	var output []byte
	for _, name := range containerCmd.Containers() {
		output, err = exec.Command(container.Runtime, "image", "rm", name).
			CombinedOutput()
		if err != nil {
			log.Error().Err(err).Str("output", string(output)).Msg("")
			return
		}
	}
	return
}
