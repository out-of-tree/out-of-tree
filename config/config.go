// Copyright 2018 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package config

import (
	"io"
	"os"

	"code.dumpstack.io/tools/out-of-tree/distro"

	"github.com/naoina/toml"
)

// KernelConfig is the ~/.out-of-tree/kernels.toml configuration description
type KernelConfig struct {
	Kernels []distro.KernelInfo
}

func readFileAll(path string) (buf []byte, err error) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	buf, err = io.ReadAll(f)
	return
}

// ReadKernelConfig is for read kernels.toml
func ReadKernelConfig(path string) (kernelCfg KernelConfig, err error) {
	buf, err := readFileAll(path)
	if err != nil {
		return
	}

	err = toml.Unmarshal(buf, &kernelCfg)
	if err != nil {
		return
	}

	return
}
