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
	"sync"
	"time"

	"github.com/cavaliergopher/grab/v3"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"code.dumpstack.io/tools/out-of-tree/cache"
	"code.dumpstack.io/tools/out-of-tree/config/dotfiles"
	"code.dumpstack.io/tools/out-of-tree/distro"
	"code.dumpstack.io/tools/out-of-tree/fs"
)

var Runtime = "docker"

var Registry = ""

var Timeout time.Duration

// Commands that are executed before (prepend) and after (append) the
// base layer of the Dockerfile.
var Commands struct {
	Prepend []distro.Command
	Append  []distro.Command
}

var UseCache = true

var UsePrebuilt = true

var Prune = true

var Stdout = false

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

func Load(localpath string, name string) (err error) {
	exist := Container{name: name}.Exist()
	if exist && UseCache {
		return
	}

	cmd := exec.Command(Runtime, "load", "-i", localpath)
	log.Debug().Msgf("%v", cmd)

	raw, err := cmd.CombinedOutput()
	if err != nil {
		log.Debug().Err(err).Msg(string(raw))
		return
	}

	if strings.Contains(Runtime, "docker") {
		var err2 error
		cmd = exec.Command(Runtime, "tag", "localhost/"+name, name)
		log.Debug().Msgf("%v", cmd)

		raw, err2 = cmd.CombinedOutput()
		if err2 != nil {
			log.Debug().Err(err2).Msg(string(raw))
		}

		cmd = exec.Command(Runtime, "rmi", "localhost/"+name)
		log.Debug().Msgf("%v", cmd)

		raw, err2 = cmd.CombinedOutput()
		if err2 != nil {
			log.Debug().Err(err2).Msg(string(raw))
		}
	}

	return
}

func Import(path, name string) (err error) {
	exist := Container{name: name}.Exist()
	if exist && UseCache {
		return
	}

	cmd := exec.Command(Runtime, "import", path, name)
	log.Debug().Msgf("%v", cmd)

	raw, err := cmd.CombinedOutput()
	if err != nil {
		log.Debug().Err(err).Msg(string(raw))
		return
	}

	return
}

func Save(name, path string) (err error) {
	exist := Container{name: name}.Exist()
	if !exist {
		err = errors.New("container does not exist")
		log.Error().Err(err).Msg("")
		return
	}

	cmd := exec.Command(Runtime, "save", name, "-o", path)
	log.Debug().Msgf("%v", cmd)

	raw, err := cmd.CombinedOutput()
	if err != nil {
		log.Error().Err(err).Msg(string(raw))
		return
	}

	return
}

type Volume struct {
	Src, Dest string
}

type Container struct {
	name string
	dist distro.Distro

	Volumes []Volume

	// Additional arguments
	Args []string

	// Base of container is local-only
	LocalBase bool

	Log zerolog.Logger

	commandsOutput struct {
		listener chan string
		mu       sync.Mutex
	}
}

func New(dist distro.Distro) (c Container, err error) {
	distro := strings.ToLower(dist.ID.String())
	release := strings.Replace(dist.Release, ".", "__", -1)
	c.name = fmt.Sprintf("out_of_tree_%s_%s", distro, release)

	c.Log = log.With().
		Str("container", c.name).
		Logger()

	c.dist = dist

	c.Volumes = append(c.Volumes, Volume{
		Src:  dotfiles.Dir("volumes", c.name, "lib", "modules"),
		Dest: "/lib/modules",
	})

	c.Volumes = append(c.Volumes, Volume{
		Src:  dotfiles.Dir("volumes", c.name, "usr", "src"),
		Dest: "/usr/src",
	})

	c.Volumes = append(c.Volumes, Volume{
		Src:  dotfiles.Dir("volumes", c.name, "boot"),
		Dest: "/boot",
	})

	return
}

func NewFromKernelInfo(ki distro.KernelInfo) (
	c Container, err error) {

	c.name = ki.ContainerName

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

// c.SetCommandsOutputHandler(func(s string) { fmt.Println(s) })
// defer c.CloseCommandsOutputHandler()
func (c *Container) SetCommandsOutputHandler(handler func(s string)) {
	c.commandsOutput.mu.Lock()
	defer c.commandsOutput.mu.Unlock()

	c.commandsOutput.listener = make(chan string)

	go func(l chan string) {
		for m := range l {
			if m != "" {
				handler(m)
			}
		}
	}(c.commandsOutput.listener)
}

func (c *Container) CloseCommandsOutputHandler() {
	c.commandsOutput.mu.Lock()
	defer c.commandsOutput.mu.Unlock()

	close(c.commandsOutput.listener)
	c.commandsOutput.listener = nil
}

func (c *Container) handleCommandsOutput(m string) {
	if c.commandsOutput.listener == nil {
		return
	}
	c.commandsOutput.mu.Lock()
	defer c.commandsOutput.mu.Unlock()

	if c.commandsOutput.listener != nil {
		c.commandsOutput.listener <- m
	}
}

func (c Container) Name() string {
	return c.name
}

func (c Container) Exist() (yes bool) {
	cmd := exec.Command(Runtime, "images", "-q", c.name)

	c.Log.Debug().Msgf("run %v", cmd)

	raw, err := cmd.CombinedOutput()
	if err != nil {
		c.Log.Error().Err(err).Msg(string(raw))
		return false
	}

	yes = string(raw) != ""

	if yes {
		c.Log.Debug().Msg("exist")
	} else {
		c.Log.Debug().Msg("does not exist")
	}

	return
}

func (c Container) loadPrebuilt() (err error) {
	if c.Exist() && UseCache {
		return
	}

	tmp, err := fs.TempDir()
	if err != nil {
		return
	}
	defer os.RemoveAll(tmp)

	log.Info().Msgf("download prebuilt container %s", c.Name())
	resp, err := grab.Get(tmp, cache.ContainerURL(c.Name()))
	if err != nil {
		return
	}

	defer os.Remove(resp.Filename)

	err = Load(resp.Filename, c.Name())
	if err == nil {
		log.Info().Msgf("use prebuilt container %s", c.Name())
	}

	return
}

func (c Container) Build(image string, envs, runs []string) (err error) {
	if c.Exist() && UseCache {
		return
	}

	cdir := dotfiles.Dir("containers", c.name)
	cfile := filepath.Join(cdir, "Dockerfile")

	cf := "FROM "
	if Registry != "" {
		cf += Registry + "/"
	}
	cf += image + "\n"

	for _, cmd := range Commands.Prepend {
		if cmd.Distro.ID != distro.None && cmd.Distro.ID != c.dist.ID {
			continue
		}
		if cmd.Distro.Release != "" && cmd.Distro.Release != c.dist.Release {
			continue
		}

		cf += "RUN " + cmd.Command + "\n"
	}

	for _, e := range envs {
		cf += "ENV " + e + "\n"
	}

	for _, c := range runs {
		cf += "RUN " + c + "\n"
	}

	for _, cmd := range Commands.Append {
		if cmd.Distro.ID != distro.None && cmd.Distro.ID != c.dist.ID {
			continue
		}
		if cmd.Distro.Release != "" && cmd.Distro.Release != c.dist.Release {
			continue
		}

		cf += "RUN " + cmd.Command + "\n"
	}

	buf, err := os.ReadFile(cfile)
	if err != nil {
		err = os.WriteFile(cfile, []byte(cf), os.ModePerm)
		if err != nil {
			return
		}
	}

	if string(buf) == cf && c.Exist() && UseCache {
		return
	}

	err = os.WriteFile(cfile, []byte(cf), os.ModePerm)
	if err != nil {
		return
	}

	if c.Exist() {
		c.Log.Info().Msg("update")
	} else {
		c.Log.Info().Msg("build")
	}

	if UsePrebuilt {
		err = c.loadPrebuilt()
	}

	if err != nil || !UsePrebuilt {
		var output string
		output, err = c.build(cdir)
		if err != nil {
			c.Log.Error().Err(err).Msg(output)
			return
		}
	}

	c.Log.Info().Msg("success")
	return
}

func (c Container) prune() error {
	c.Log.Debug().Msg("remove dangling or unused images from local storage")
	return exec.Command(Runtime, "image", "prune", "-f").Run()
}

func (c Container) build(imagePath string) (output string, err error) {
	if Prune {
		defer c.prune()
	}

	args := []string{"build"}
	if !UseCache {
		if !c.LocalBase {
			args = append(args, "--pull")
		}
		args = append(args, "--no-cache")
	}
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
			if Stdout {
				fmt.Println(m)
			}
			c.handleCommandsOutput(m)
			output += m + "\n"
			flog.Trace().Str("stdout", m).Msg("")
		}
	}()

	err = cmd.Wait()
	return
}

func (c *Container) Run(workdir string, cmds []string) (out string, err error) {
	flog := c.Log.With().
		Str("workdir", workdir).
		Str("command", fmt.Sprintf("%v", cmds)).
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

	command := "true"
	for _, c := range cmds {
		command += fmt.Sprintf(" && %s", c)
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

	if Timeout != 0 {
		timer := time.AfterFunc(Timeout, func() {
			flog.Info().Msg("killing container by timeout")

			flog.Debug().Msg("SIGINT")
			cmd.Process.Signal(os.Interrupt)

			time.Sleep(time.Minute)

			flog.Debug().Msg("SIGKILL")
			cmd.Process.Kill()
		})
		defer timer.Stop()
	}

	err = cmd.Start()
	if err != nil {
		return
	}

	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			m := scanner.Text()
			if Stdout {
				fmt.Println(m)
			}
			c.handleCommandsOutput(m)
			out += m + "\n"
			flog.Trace().Str("container stdout", m).Msg("")
		}
	}()

	err = cmd.Wait()
	if err != nil {
		return
	}

	return
}

func FindKernel(entries []os.DirEntry, kname string) (name string, err error) {
	for _, e := range entries {
		var fi os.FileInfo
		fi, err = e.Info()
		if err != nil {
			return
		}

		if strings.HasPrefix(fi.Name(), "vmlinuz") {
			if strings.Contains(fi.Name(), kname) {
				name = fi.Name()
				return
			}
		}
	}

	err = errors.New("cannot find kernel")
	return
}

func FindInitrd(entries []os.DirEntry, kname string) (name string, err error) {
	for _, e := range entries {
		var fi os.FileInfo
		fi, err = e.Info()
		if err != nil {
			return
		}

		if strings.HasPrefix(fi.Name(), "initrd") ||
			strings.HasPrefix(fi.Name(), "initramfs") {

			if strings.Contains(fi.Name(), kname) {
				name = fi.Name()
				return
			}
		}
	}

	err = errors.New("cannot find kernel")
	return
}

func (c Container) Kernels() (kernels []distro.KernelInfo, err error) {
	if !c.Exist() {
		return
	}

	var libmodules, boot string
	for _, volume := range c.Volumes {
		switch volume.Dest {
		case "/lib/modules":
			libmodules = volume.Src
		case "/boot":
			boot = volume.Src
		}
	}

	moddirs, err := os.ReadDir(libmodules)
	if err != nil {
		return
	}

	bootfiles, err := os.ReadDir(boot)
	if err != nil {
		return
	}

	for _, e := range moddirs {
		var krel os.FileInfo
		krel, err = e.Info()
		if err != nil {
			return
		}

		c.Log.Debug().Msgf("generate config entry for %s", krel.Name())

		var kernelFile, initrdFile string
		kernelFile, err = FindKernel(bootfiles, krel.Name())
		if err != nil {
			c.Log.Warn().Msgf("cannot find kernel %s", krel.Name())
			continue
		}

		initrdFile, err = FindInitrd(bootfiles, krel.Name())
		if err != nil {
			c.Log.Warn().Msgf("cannot find initrd %s", krel.Name())
			continue
		}

		ki := distro.KernelInfo{
			Distro:        c.dist,
			KernelVersion: krel.Name(),
			KernelRelease: krel.Name(),
			ContainerName: c.name,

			KernelPath:  filepath.Join(boot, kernelFile),
			InitrdPath:  filepath.Join(boot, initrdFile),
			ModulesPath: filepath.Join(libmodules, krel.Name()),

			RootFS: dotfiles.File("images", c.dist.RootFS()),
		}

		kernels = append(kernels, ki)
	}

	for _, cmd := range []string{
		"find /boot -type f -exec chmod a+r {} \\;",
	} {
		_, err = c.Run(dotfiles.Dir("tmp"), []string{cmd})
		if err != nil {
			return
		}
	}

	return
}
