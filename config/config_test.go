// Copyright 2018 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package config

import (
	"testing"

	"code.dumpstack.io/tools/out-of-tree/distro"

	"github.com/naoina/toml"
)

func TestMarshalUnmarshal(t *testing.T) {
	artifactCfg := Artifact{
		Name: "Put name here",
		Type: KernelModule,
	}
	artifactCfg.Targets = append(artifactCfg.Targets,
		Target{
			Distro: distro.Distro{
				ID:      distro.Ubuntu,
				Release: "18.04",
			},
			Kernel: Kernel{
				Regex: ".*",
			},
		})
	buf, err := toml.Marshal(&artifactCfg)
	if err != nil {
		t.Fatal(err)
	}

	var artifactCfgNew Artifact
	err = toml.Unmarshal(buf, &artifactCfgNew)
	if err != nil {
		t.Fatal(err)
	}
}
