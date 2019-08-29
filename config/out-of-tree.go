// Copyright 2019 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package config

import (
	"os/user"

	"github.com/naoina/toml"
)

type OutOfTree struct {
	Kernels     string
	UserKernels string

	Database string

	Qemu struct {
		Timeout string
	}

	Docker struct {
		Timeout  string
		Registry string
	}
}

func ReadOutOfTreeConf(path string) (c OutOfTree, err error) {
	buf, err := readFileAll(path)
	if err == nil {
		err = toml.Unmarshal(buf, &c)
		if err != nil {
			return
		}
	} else {
		// It's ok if there's no configuration
		// then we'll just set default values
		err = nil
	}

	usr, err := user.Current()
	if err != nil {
		return
	}

	if c.Kernels == "" {
		c.Kernels = usr.HomeDir + "/.out-of-tree/kernels.toml"
	}

	if c.UserKernels == "" {
		c.UserKernels = usr.HomeDir + "/.out-of-tree/kernels.user.toml"
	}

	if c.Database == "" {
		c.Database = usr.HomeDir + "/.out-of-tree/db.sqlite"
	}

	if c.Qemu.Timeout == "" {
		c.Qemu.Timeout = "1m"
	}

	if c.Docker.Timeout == "" {
		c.Docker.Timeout = "1m"
	}

	return
}
