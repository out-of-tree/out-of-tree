// Copyright 2019 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package config

import (
	"errors"
	"os"
	"time"

	"code.dumpstack.io/tools/out-of-tree/artifact"
	"code.dumpstack.io/tools/out-of-tree/config/dotfiles"
	"code.dumpstack.io/tools/out-of-tree/distro"

	"github.com/alecthomas/kong"
	"github.com/mitchellh/go-homedir"
	"github.com/naoina/toml"
)

type OutOfTree struct {
	// Directory for all files if not explicitly specified
	Directory string

	Kernels     string
	UserKernels string

	Database string

	Qemu struct {
		Timeout artifact.Duration
	}

	Docker struct {
		Timeout  artifact.Duration
		Registry string

		// Commands that will be executed before
		// the base layer of Dockerfile
		Commands []distro.Command
	}
}

func (c *OutOfTree) Decode(ctx *kong.DecodeContext) (err error) {
	if ctx.Value.Set {
		return
	}

	s, err := homedir.Expand(ctx.Scan.Pop().String())
	if err != nil {
		return
	}

	defaultValue, err := homedir.Expand(ctx.Value.Default)
	if err != nil {
		return
	}

	_, err = os.Stat(s)
	if s != defaultValue && errors.Is(err, os.ErrNotExist) {
		return errors.New("'" + s + "' does not exist")
	}

	*c, err = ReadOutOfTreeConf(s)
	return
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

	if c.Directory != "" {
		dotfiles.Directory = c.Directory
	} else {
		c.Directory = dotfiles.Dir("")
	}

	if c.Kernels == "" {
		c.Kernels = dotfiles.File("kernels.toml")
	}

	if c.UserKernels == "" {
		c.UserKernels = dotfiles.File("kernels.user.toml")
	}

	if c.Database == "" {
		c.Database = dotfiles.File("db.sqlite")
	}

	if c.Qemu.Timeout.Duration == 0 {
		c.Qemu.Timeout.Duration = time.Minute
	}

	if c.Docker.Timeout.Duration == 0 {
		c.Docker.Timeout.Duration = 8 * time.Minute
	}

	return
}
