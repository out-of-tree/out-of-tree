// Copyright 2018 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package config

import (
	"testing"

	"github.com/naoina/toml"
)

func TestMarshalUnmarshal(t *testing.T) {
	artifactCfg := Artifact{
		Name: "Put name here",
		Type: KernelModule,
	}
	artifactCfg.SupportedKernels = append(artifactCfg.SupportedKernels,
		KernelMask{Ubuntu, "18.04", ".*"})
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
