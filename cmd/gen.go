// Copyright 2018 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package cmd

import (
	"fmt"

	"github.com/naoina/toml"

	"code.dumpstack.io/tools/out-of-tree/artifact"
	"code.dumpstack.io/tools/out-of-tree/distro"
)

type GenCmd struct {
	Type string `enum:"module,exploit" required:"" help:"module/exploit"`
}

func (cmd *GenCmd) Run(g *Globals) (err error) {
	switch cmd.Type {
	case "module":
		err = genConfig(artifact.KernelModule)
	case "exploit":
		err = genConfig(artifact.KernelExploit)
	}
	return
}

func genConfig(at artifact.ArtifactType) (err error) {
	a := artifact.Artifact{
		Name: "Put name here",
		Type: at,
	}
	a.Targets = append(a.Targets, artifact.Target{
		Distro: distro.Distro{ID: distro.Ubuntu, Release: "18.04"},
		Kernel: artifact.Kernel{Regex: ".*"},
	})
	a.Targets = append(a.Targets, artifact.Target{
		Distro: distro.Distro{ID: distro.Debian, Release: "8"},
		Kernel: artifact.Kernel{Regex: ".*"},
	})
	a.Preload = append(a.Preload, artifact.PreloadModule{
		Repo: "Repo name (e.g. https://github.com/openwall/lkrg)",
	})
	a.Patches = append(a.Patches, artifact.Patch{
		Path: "/path/to/profiling.patch",
	})

	buf, err := toml.Marshal(&a)
	if err != nil {
		return
	}

	fmt.Print(string(buf))
	return
}
