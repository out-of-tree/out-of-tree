// Copyright 2019 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package config

import (
	"errors"
	"os"
	"time"

	"github.com/alecthomas/kong"
	"github.com/mitchellh/go-homedir"
	"github.com/naoina/toml"
)

type DockerCommand struct {
	DistroType DistroType
	Command    string
}

type OutOfTree struct {
	// Directory for all files if not explicitly specified
	Directory string

	Kernels     string
	UserKernels string

	Database string

	Qemu struct {
		Timeout Duration
	}

	Docker struct {
		Timeout  Duration
		Registry string

		// Commands that will be executed before
		// the base layer of Dockerfile
		Commands []DockerCommand
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
		Directory = c.Directory
	}

	if c.Kernels == "" {
		c.Kernels = File("kernels.toml")
	}

	if c.UserKernels == "" {
		c.UserKernels = File("kernels.user.toml")
	}

	if c.Database == "" {
		c.Database = File("db.sqlite")
	}

	if c.Qemu.Timeout.Duration == 0 {
		c.Qemu.Timeout.Duration = time.Minute
	}

	if c.Docker.Timeout.Duration == 0 {
		c.Docker.Timeout.Duration = time.Minute
	}

	return
}
