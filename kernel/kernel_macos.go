// Copyright 2023 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

//go:build darwin
// +build darwin

package kernel

import (
	"errors"

	"code.dumpstack.io/tools/out-of-tree/distro"
)

func GenHostKernels(download bool) (kernels []distro.KernelInfo, err error) {
	err = errors.New("generate host kernels for macOS is not supported")
	return
}
