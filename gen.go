// Copyright 2018 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package main

import (
	"fmt"

	"github.com/jollheef/out-of-tree/config"
	"github.com/naoina/toml"
)

func genConfig(at config.ArtifactType) (err error) {
	a := config.Artifact{
		Name: "Put name here",
		Type: at,
	}
	a.SupportedKernels = append(a.SupportedKernels, config.KernelMask{
		config.Ubuntu, "18.04", ".*",
	})

	buf, err := toml.Marshal(&a)
	if err != nil {
		return
	}

	fmt.Print(string(buf))
	return
}
