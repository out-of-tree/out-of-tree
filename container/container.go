// Copyright 2023 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package container

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"code.dumpstack.io/tools/out-of-tree/config"
	"code.dumpstack.io/tools/out-of-tree/distro"
)

var Runtime = "docker"

type Image struct {
	Name   string
	Distro distro.Distro
}

func Images() (diis []Image, err error) {
	cmd := exec.Command(Runtime, "images")
	log.Debug().Msgf("%v", cmd)

	rawOutput, err := cmd.CombinedOutput()
	if err != nil {
		return
	}

	r, err := regexp.Compile("out_of_tree_.*")
	if err != nil {
		return
	}

	containers := r.FindAll(rawOutput, -1)
	for _, c := range containers {
		containerName := strings.Fields(string(c))[0]

		s := strings.Replace(containerName, "__", ".", -1)
		values := strings.Split(s, "_")
		distroName, ver := values[3], values[4]

		dii := Image{
			Name: containerName,
		}

		dii.Distro.Release = ver
		dii.Distro.ID, err = distro.NewID(distroName)
		if err != nil {
			return
		}

		diis = append(diis, dii)
	}
	return
}

func ImagePath(sk config.Target) string {
	return config.Dir("containers", sk.Distro.ID.String(), sk.Distro.Release)
}

type Volume struct {
	Src, Dest string
}

type Container struct {
	name string

	timeout time.Duration

	Volumes []Volume

	// Additional arguments
	Args []string

	Log zerolog.Logger
}

func New(name string, timeout time.Duration) (c Container, err error) {
	c.Log = log.With().
		Str("container", name).
		Logger()

	c.name = name
	c.timeout = timeout

	c.Volumes = append(c.Volumes, Volume{
		Src:  config.Dir("volumes", name, "lib", "modules"),
		Dest: "/lib/modules",
	})

	c.Volumes = append(c.Volumes, Volume{
		Src:  config.Dir("volumes", name, "usr", "src"),
		Dest: "/usr/src",
	})

	c.Volumes = append(c.Volumes, Volume{
		Src:  config.Dir("volumes", name, "boot"),
		Dest: "/boot",
	})

	return
}

func NewFromKernelInfo(ki config.KernelInfo, timeout time.Duration) (
	c Container, err error) {

	c.name = ki.ContainerName
	c.timeout = timeout

	c.Log = log.With().
		Str("container", c.name).
		Logger()

	c.Volumes = append(c.Volumes, Volume{
		Src:  path.Dir(ki.ModulesPath),
		Dest: "/lib/modules",
	})

	c.Volumes = append(c.Volumes, Volume{
		Src:  filepath.Join(path.Dir(ki.KernelPath), "../usr/src"),
		Dest: "/usr/src",
	})

	c.Volumes = append(c.Volumes, Volume{
		Src:  path.Dir(ki.KernelPath),
		Dest: "/boot",
	})

	return
}

func (c Container) Build(imagePath string) (output string, err error) {
	args := []string{"build"}
	args = append(args, "-t", c.name, imagePath)

	cmd := exec.Command(Runtime, args...)

	flog := c.Log.With().
		Str("command", fmt.Sprintf("%v", cmd)).
		Logger()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return
	}
	cmd.Stderr = cmd.Stdout

	err = cmd.Start()
	if err != nil {
		return
	}

	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			m := scanner.Text()
			output += m + "\n"
			flog.Trace().Str("stdout", m).Msg("")
		}
	}()

	err = cmd.Wait()
	return
}

func (c Container) Run(workdir string, command string) (output string, err error) {
	flog := c.Log.With().
		Str("workdir", workdir).
		Str("command", command).
		Logger()

	var args []string
	args = append(args, "run", "--rm")
	args = append(args, c.Args...)
	if workdir != "" {
		args = append(args, "-v", workdir+":/work")
	}

	for _, volume := range c.Volumes {
		mount := fmt.Sprintf("%s:%s", volume.Src, volume.Dest)
		args = append(args, "-v", mount)
	}

	args = append(args, c.name, "bash", "-c")
	if workdir != "" {
		args = append(args, "cd /work && "+command)
	} else {
		args = append(args, command)
	}

	cmd := exec.Command(Runtime, args...)

	flog.Debug().Msgf("%v", cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return
	}
	cmd.Stderr = cmd.Stdout

	timer := time.AfterFunc(c.timeout, func() {
		flog.Info().Msg("killing container by timeout")

		flog.Debug().Msg("SIGINT")
		cmd.Process.Signal(os.Interrupt)

		time.Sleep(time.Minute)

		flog.Debug().Msg("SIGKILL")
		cmd.Process.Kill()
	})
	defer timer.Stop()

	err = cmd.Start()
	if err != nil {
		return
	}

	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			m := scanner.Text()
			output += m + "\n"
			flog.Trace().Str("stdout", m).Msg("")
		}
	}()

	err = cmd.Wait()
	if err != nil {
		e := fmt.Sprintf("error `%v` for cmd `%v` with output `%v`",
			err, command, output)
		err = errors.New(e)
		return
	}

	return
}
