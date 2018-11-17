// Copyright 2018 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package main

import (
	"errors"
	"fmt"

	"github.com/jollheef/out-of-tree/config"
)

func kernelListHandler(kcfg config.KernelConfig) (err error) {
	if len(kcfg.Kernels) == 0 {
		return errors.New("No kernels found")
	}
	for _, k := range kcfg.Kernels {
		fmt.Println(k.DistroType, k.DistroRelease, k.KernelRelease)
	}
	return
}
