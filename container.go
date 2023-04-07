// Copyright 2023 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"regexp"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"code.dumpstack.io/tools/out-of-tree/config"
)

var containerRuntime = "docker"

type ContainerCmd struct {
	Filter string `help:"filter by name"`

	List    ContainerListCmd    `cmd:"" help:"list containers"`
	Cleanup ContainerCleanupCmd `cmd:"" help:"cleanup containers"`
}

func (cmd ContainerCmd) Containers() (names []string) {
	images, err := listContainerImages()
	if err != nil {
		log.Fatal().Err(err).Msg("")
	}

	for _, img := range images {
		if cmd.Filter != "" && !strings.Contains(img.Name, cmd.Filter) {
			continue
		}
		names = append(names, img.Name)
	}
	return
}

type ContainerListCmd struct{}

func (cmd ContainerListCmd) Run(containerCmd *ContainerCmd) (err error) {
	for _, name := range containerCmd.Containers() {
		fmt.Println(name)
	}
	return
}

type ContainerCleanupCmd struct{}

func (cmd ContainerCleanupCmd) Run(containerCmd *ContainerCmd) (err error) {
	var output []byte
	for _, name := range containerCmd.Containers() {
		output, err = exec.Command(containerRuntime, "image", "rm", name).
			CombinedOutput()
		if err != nil {
			log.Error().Err(err).Str("output", string(output)).Msg("")
			return
		}
	}
	return
}

type containerImageInfo struct {
	Name          string
	DistroType    config.DistroType
	DistroRelease string // 18.04/7.4.1708/9.1
}

func listContainerImages() (diis []containerImageInfo, err error) {
	cmd := exec.Command(containerRuntime, "images")
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
		container := strings.Fields(string(c))[0]

		s := strings.Replace(container, "__", ".", -1)
		values := strings.Split(s, "_")
		distro, ver := values[3], values[4]

		dii := containerImageInfo{
			Name:          container,
			DistroRelease: ver,
		}

		dii.DistroType, err = config.NewDistroType(distro)
		if err != nil {
			return
		}

		diis = append(diis, dii)
	}
	return
}

type container struct {
	name    string
	timeout time.Duration
	Volumes struct {
		LibModules string
		UsrSrc     string
		Boot       string
	}
	// Additional arguments
	Args []string
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

	cmd := exec.Command(containerRuntime, args...)

	flog := log.With().
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

func (c container) Run(workdir string, command string) (output string, err error) {
	flog := log.With().
		Str("container", c.name).
		Str("workdir", workdir).
		Str("command", command).
		Logger()

	var args []string
	args = append(args, "run", "--rm")
	args = append(args, c.Args...)
	if workdir != "" {
		args = append(args, "-v", workdir+":/work")
	}
	if c.Volumes.LibModules != "" {
		args = append(args, "-v", c.Volumes.LibModules+":/lib/modules")
	}
	if c.Volumes.UsrSrc != "" {
		args = append(args, "-v", c.Volumes.UsrSrc+":/usr/src")
	}
	if c.Volumes.Boot != "" {
		args = append(args, "-v", c.Volumes.Boot+":/boot")
	}
	args = append(args, c.name, "bash", "-c")
	if workdir != "" {
		args = append(args, "cd /work && "+command)
	} else {
		args = append(args, command)
	}

	cmd := exec.Command(containerRuntime, args...)

	log.Debug().Msgf("%v", cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return
	}
	cmd.Stderr = cmd.Stdout

	timer := time.AfterFunc(c.timeout, func() {
		flog.Info().Msg("killing container by timeout")
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
