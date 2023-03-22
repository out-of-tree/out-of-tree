// Copyright 2023 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"time"

	"github.com/rs/zerolog/log"
)

type container struct {
	name    string
	timeout time.Duration
	Volumes struct {
		LibModules string
		UsrSrc     string
		Boot       string
	}
}

func NewContainer(name string, timeout time.Duration) (c container, err error) {
	c.name = name
	c.timeout = timeout

	usr, err := user.Current()
	if err != nil {
		return
	}

	c.Volumes.LibModules = fmt.Sprintf(
		"%s/.out-of-tree/volumes/%s/lib/modules", usr.HomeDir, name)
	os.MkdirAll(c.Volumes.LibModules, 0777)

	c.Volumes.UsrSrc = fmt.Sprintf(
		"%s/.out-of-tree/volumes/%s/usr/src", usr.HomeDir, name)
	os.MkdirAll(c.Volumes.UsrSrc, 0777)

	c.Volumes.Boot = fmt.Sprintf(
		"%s/.out-of-tree/volumes/%s/boot", usr.HomeDir, name)
	os.MkdirAll(c.Volumes.Boot, 0777)

	return
}

func (c container) Build(imagePath string) (output string, err error) {
	args := []string{"build"}
	args = append(args, "-t", c.name, imagePath)

	cmd := exec.Command("docker", args...)
	log.Debug().Msgf("%v", cmd)

	rawOutput, err := cmd.CombinedOutput()
	output = string(rawOutput)
	return
}

func (c container) Run(workdir string, command string) (output string, err error) {
	cmd := exec.Command("docker", "run", "--rm",
		"-v", workdir+":/work",
		"-v", c.Volumes.LibModules+":/lib/modules",
		"-v", c.Volumes.UsrSrc+":/usr/src",
		"-v", c.Volumes.Boot+":/boot",
		c.name, "bash", "-c", "cd /work && "+command)

	log.Debug().Msgf("%v", cmd)

	timer := time.AfterFunc(c.timeout, func() {
		log.Info().Str("container", c.name).
			Str("workdir", workdir).
			Str("command", command).
			Msg("killing container by timeout")
		cmd.Process.Kill()
	})
	defer timer.Stop()

	raw, err := cmd.CombinedOutput()
	if err != nil {
		e := fmt.Sprintf("error `%v` for cmd `%v` with output `%v`",
			err, command, string(raw))
		err = errors.New(e)
		return
	}

	output = string(raw)
	return
}
