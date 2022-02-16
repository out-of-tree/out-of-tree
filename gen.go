// Copyright 2018 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package main

import (
	"fmt"

	"github.com/naoina/toml"

	"code.dumpstack.io/tools/out-of-tree/config"
)

func genConfig(at config.ArtifactType) (err error) {
	a := config.Artifact{
		Name: "Put name here",
		Type: at,
	}
	a.SupportedKernels = append(a.SupportedKernels, config.KernelMask{
		DistroType:    config.Ubuntu,
		DistroRelease: "18.04",
		ReleaseMask:   ".*",
	})
	a.Preload = append(a.Preload, config.PreloadModule{
		Repo: "Repo name (e.g. https://github.com/openwall/lkrg)",
	})

	buf, err := toml.Marshal(&a)
	if err != nil {
		return
	}

	fmt.Print(string(buf))
	return
}
