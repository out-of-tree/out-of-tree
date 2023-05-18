// Copyright 2023 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package kernel

import (
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"runtime"
	"strings"
	"time"

	"github.com/naoina/toml"
	"github.com/rs/zerolog/log"

	"code.dumpstack.io/tools/out-of-tree/cache"
	"code.dumpstack.io/tools/out-of-tree/config"
	"code.dumpstack.io/tools/out-of-tree/container"
	"code.dumpstack.io/tools/out-of-tree/distro/centos"
	"code.dumpstack.io/tools/out-of-tree/distro/debian"
	"code.dumpstack.io/tools/out-of-tree/distro/oraclelinux"
	"code.dumpstack.io/tools/out-of-tree/distro/ubuntu"
	"code.dumpstack.io/tools/out-of-tree/fs"
)

func MatchPackages(km config.KernelMask) (pkgs []string, err error) {
	// TODO interface for kernels match
	switch km.DistroType {
	case config.Ubuntu:
		pkgs, err = ubuntu.Match(km)
	case config.OracleLinux, config.CentOS:
		pkgs, err = oraclelinux.Match(km)
	case config.Debian:
		pkgs, err = debian.Match(km)
	default:
		err = fmt.Errorf("%s not yet supported", km.DistroType.String())
	}
	return
}

func vsyscallAvailable() (available bool, err error) {
	if runtime.GOOS != "linux" {
		// Docker for non-Linux systems is not using the host
		// kernel but uses kernel inside a virtual machine, so
		// it builds by the Docker team with vsyscall support.
		available = true
		return
	}

	buf, err := ioutil.ReadFile("/proc/self/maps")
	if err != nil {
		return
	}

	available = strings.Contains(string(buf), "[vsyscall]")
	return
}

func GenerateBaseDockerImage(registry string, commands []config.DockerCommand,
	sk config.KernelMask, forceUpdate bool) (err error) {

	imagePath := container.ImagePath(sk)
	dockerPath := imagePath + "/Dockerfile"

	d := "# BASE\n"

	// TODO move as function to container.go
	cmd := exec.Command(container.Runtime, "images", "-q", sk.DockerName())
	log.Debug().Msgf("run %v", cmd)

	rawOutput, err := cmd.CombinedOutput()
	if err != nil {
		log.Error().Err(err).Msg(string(rawOutput))
		return
	}

	if fs.PathExists(dockerPath) && string(rawOutput) != "" {
		log.Debug().Msgf("Base image for %s:%s found",
			sk.DistroType.String(), sk.DistroRelease)
		if !forceUpdate {
			return
		} else {
			log.Info().Msgf("Update Containerfile")
		}
	}

	log.Debug().Msgf("Base image for %s:%s not found, start generating",
		sk.DistroType.String(), sk.DistroRelease)
	os.MkdirAll(imagePath, os.ModePerm)

	d += "FROM "
	if registry != "" {
		d += registry + "/"
	}

	switch sk.DistroType {
	case config.Debian:
		d += debian.ContainerImage(sk) + "\n"
	default:
		d += fmt.Sprintf("%s:%s\n",
			strings.ToLower(sk.DistroType.String()),
			sk.DistroRelease)
	}

	for _, c := range commands {
		d += "RUN " + c.Command + "\n"
	}

	// TODO container runs/envs interface
	switch sk.DistroType {
	case config.Ubuntu:
		for _, e := range ubuntu.Envs(sk) {
			d += "ENV " + e + "\n"
		}
		for _, c := range ubuntu.Runs(sk) {
			d += "RUN " + c + "\n"
		}
	case config.CentOS:
		for _, e := range centos.Envs(sk) {
			d += "ENV " + e + "\n"
		}
		for _, c := range centos.Runs(sk) {
			d += "RUN " + c + "\n"
		}
	case config.OracleLinux:
		for _, e := range oraclelinux.Envs(sk) {
			d += "ENV " + e + "\n"
		}
		for _, c := range oraclelinux.Runs(sk) {
			d += "RUN " + c + "\n"
		}
	case config.Debian:
		for _, e := range debian.Envs(sk) {
			d += "ENV " + e + "\n"
		}
		for _, c := range debian.Runs(sk) {
			d += "RUN " + c + "\n"
		}
	default:
		err = fmt.Errorf("%s not yet supported", sk.DistroType.String())
		return
	}

	d += "# END BASE\n\n"

	err = ioutil.WriteFile(dockerPath, []byte(d), 0644)
	if err != nil {
		return
	}

	c, err := container.New(sk.DockerName(), time.Hour)
	if err != nil {
		return
	}

	output, err := c.Build(imagePath)
	if err != nil {
		log.Error().Err(err).Msgf("Base image for %s:%s generating error",
			sk.DistroType.String(), sk.DistroRelease)
		log.Fatal().Msg(output)
		return
	}

	log.Debug().Msgf("Base image for %s:%s generating success",
		sk.DistroType.String(), sk.DistroRelease)

	return
}

func installKernel(sk config.KernelMask, pkgname string, force, headers bool) (err error) {
	slog := log.With().
		Str("distro_type", sk.DistroType.String()).
		Str("distro_release", sk.DistroRelease).
		Str("pkg", pkgname).
		Logger()

	c, err := container.New(sk.DockerName(), time.Hour) // TODO conf
	if err != nil {
		return
	}

	searchdir := c.Volumes.LibModules

	if sk.DistroType == config.Debian {
		// TODO We need some kind of API for that
		searchdir = config.Dir("volumes", sk.DockerName())
	}

	moddirs, err := ioutil.ReadDir(searchdir)
	if err != nil {
		return
	}

	for _, krel := range moddirs {
		if strings.Contains(pkgname, krel.Name()) {
			if force {
				slog.Info().Msg("Reinstall")
			} else {
				slog.Info().Msg("Already installed")
				return
			}
		}
	}

	if sk.DistroType == config.Debian {
		// Debian has different kernels (package version) by the
		// same name (ABI), so we need to separate /boot
		c.Volumes = debian.Volumes(sk, pkgname)
	}

	volumes := c.Volumes

	c.Volumes.LibModules = ""
	c.Volumes.UsrSrc = ""
	c.Volumes.Boot = ""

	slog.Debug().Msgf("Installing kernel")

	// TODO use list of commands instead of appending to string
	cmd := "true"

	// TODO install/cleanup kernel interface
	switch sk.DistroType {
	case config.Ubuntu:
		var headerspkg string
		if headers {
			headerspkg = strings.Replace(pkgname, "image", "headers", -1)
		}

		cmd += fmt.Sprintf(" && apt-get install -y %s %s", pkgname, headerspkg)
	case config.OracleLinux, config.CentOS:
		var commands []string
		commands, err = oraclelinux.Install(sk, pkgname, headers)
		if err != nil {
			return
		}
		defer func() {
			if err != nil {
				oraclelinux.Cleanup(sk, pkgname)
			}
		}()

		for _, command := range commands {
			cmd += fmt.Sprintf(" && %s", command)
		}
	case config.Debian:
		var commands []string
		commands, err = debian.Install(sk, pkgname, headers)
		if err != nil {
			return
		}
		defer func() {
			if err != nil {
				debian.Cleanup(sk, pkgname)
			}
		}()

		for _, command := range commands {
			cmd += fmt.Sprintf(" && %s", command)
		}
	default:
		err = fmt.Errorf("%s not yet supported", sk.DistroType.String())
		return
	}

	c.Args = append(c.Args, "-v", volumes.LibModules+":/target/lib/modules")
	c.Args = append(c.Args, "-v", volumes.UsrSrc+":/target/usr/src")
	c.Args = append(c.Args, "-v", volumes.Boot+":/target/boot")

	cmd += " && cp -r /boot /target/"
	cmd += " && cp -r /lib/modules /target/lib/"
	if sk.DistroType == config.Debian {
		cmd += " && cp -rL /usr/src /target/usr/"
	} else {
		cmd += " && cp -r /usr/src /target/usr/"
	}

	_, err = c.Run("", cmd)
	if err != nil {
		return
	}

	slog.Debug().Msgf("Success")
	return
}

func findKernelFile(files []os.FileInfo, kname string) (name string, err error) {
	for _, file := range files {
		if strings.HasPrefix(file.Name(), "vmlinuz") {
			if strings.Contains(file.Name(), kname) {
				name = file.Name()
				return
			}
		}
	}

	err = errors.New("cannot find kernel")
	return
}

func findInitrdFile(files []os.FileInfo, kname string) (name string, err error) {
	for _, file := range files {
		if strings.HasPrefix(file.Name(), "initrd") ||
			strings.HasPrefix(file.Name(), "initramfs") {

			if strings.Contains(file.Name(), kname) {
				name = file.Name()
				return
			}
		}
	}

	err = errors.New("cannot find kernel")
	return
}

func GenRootfsImage(d container.Image, download bool) (rootfs string, err error) {
	usr, err := user.Current()
	if err != nil {
		return
	}
	imageFile := d.Name + ".img"

	imagesPath := usr.HomeDir + "/.out-of-tree/images/"
	os.MkdirAll(imagesPath, os.ModePerm)

	rootfs = imagesPath + imageFile
	if !fs.PathExists(rootfs) {
		if download {
			log.Info().Msgf("%v not available, start download", imageFile)
			err = cache.DownloadQemuImage(imagesPath, imageFile)
		}
	}
	return
}

func UpdateKernelsCfg(host, download bool) (err error) {
	newkcfg := config.KernelConfig{}

	if host {
		// Get host kernels
		newkcfg, err = genHostKernels(download)
		if err != nil {
			return
		}
	}

	// Get docker kernels
	dockerImages, err := container.Images()
	if err != nil {
		return
	}

	for _, d := range dockerImages {
		//  TODO Requires changing the idea of how we list
		//  kernels from containers to distro/-related
		//  functions.
		if strings.Contains(d.Name, "debian") {
			err = debian.ContainerKernels(d, &newkcfg)
			if err != nil {
				log.Print("gen kernels", d.Name, ":", err)
			}
			continue
		}

		err = listContainersKernels(d, &newkcfg, download)
		if err != nil {
			log.Print("gen kernels", d.Name, ":", err)
			continue
		}
	}

	stripkcfg := config.KernelConfig{}
	for _, nk := range newkcfg.Kernels {
		if !hasKernel(nk, stripkcfg) {
			stripkcfg.Kernels = append(stripkcfg.Kernels, nk)
		}
	}

	buf, err := toml.Marshal(&stripkcfg)
	if err != nil {
		return
	}

	buf = append([]byte("# Autogenerated\n# DO NOT EDIT\n\n"), buf...)

	usr, err := user.Current()
	if err != nil {
		return
	}

	// TODO move all cfg path values to one provider
	kernelsCfgPath := usr.HomeDir + "/.out-of-tree/kernels.toml"
	err = ioutil.WriteFile(kernelsCfgPath, buf, 0644)
	if err != nil {
		return
	}

	log.Info().Msgf("%s is successfully updated", kernelsCfgPath)
	return
}

func listContainersKernels(dii container.Image, newkcfg *config.KernelConfig,
	download bool) (err error) {

	rootfs, err := GenRootfsImage(dii, download)
	if err != nil {
		return
	}

	c, err := container.New(dii.Name, time.Hour)
	if err != nil {
		return
	}

	moddirs, err := ioutil.ReadDir(c.Volumes.LibModules)
	if err != nil {
		return
	}

	bootfiles, err := ioutil.ReadDir(c.Volumes.Boot)
	if err != nil {
		return
	}

	for _, krel := range moddirs {
		log.Debug().Msgf("generate config entry for %s", krel.Name())

		var kernelFile, initrdFile string
		kernelFile, err = findKernelFile(bootfiles, krel.Name())
		if err != nil {
			log.Warn().Msgf("cannot find kernel %s", krel.Name())
			continue
		}

		initrdFile, err = findInitrdFile(bootfiles, krel.Name())
		if err != nil {
			log.Warn().Msgf("cannot find initrd %s", krel.Name())
			continue
		}

		ki := config.KernelInfo{
			DistroType:    dii.DistroType,
			DistroRelease: dii.DistroRelease,
			KernelVersion: krel.Name(),
			KernelRelease: krel.Name(),
			ContainerName: dii.Name,

			KernelPath:  c.Volumes.Boot + "/" + kernelFile,
			InitrdPath:  c.Volumes.Boot + "/" + initrdFile,
			ModulesPath: c.Volumes.LibModules + "/" + krel.Name(),

			RootFS: rootfs,
		}
		newkcfg.Kernels = append(newkcfg.Kernels, ki)
	}

	for _, cmd := range []string{
		"find /boot -type f -exec chmod a+r {} \\;",
	} {
		_, err = c.Run(config.Dir("tmp"), cmd)
		if err != nil {
			return
		}
	}

	return
}

func hasKernel(ki config.KernelInfo, kcfg config.KernelConfig) bool {
	for _, sk := range kcfg.Kernels {
		if sk == ki {
			return true
		}
	}
	return false
}

func shuffleStrings(a []string) []string {
	// Fisher–Yates shuffle
	for i := len(a) - 1; i > 0; i-- {
		j := rand.Intn(i + 1)
		a[i], a[j] = a[j], a[i]
	}
	return a
}

func SetSigintHandler(variable *bool) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		counter := 0
		for _ = range c {
			if counter == 0 {
				*variable = true
				log.Warn().Msg("shutdown requested, finishing work")
				log.Info().Msg("^C a couple of times more for an unsafe exit")
			} else if counter >= 3 {
				log.Fatal().Msg("unsafe exit")
			}

			counter += 1
		}
	}()

}

// FIXME too many parameters
func GenerateKernels(km config.KernelMask, registry string,
	commands []config.DockerCommand, max, retries int64,
	download, force, headers, shuffle, update bool,
	shutdown *bool) (err error) {

	log.Info().Msgf("Generating for kernel mask %v", km)

	_, err = GenRootfsImage(container.Image{Name: km.DockerName()},
		download)
	if err != nil || *shutdown {
		return
	}

	err = GenerateBaseDockerImage(registry, commands, km, update)
	if err != nil || *shutdown {
		return
	}

	pkgs, err := MatchPackages(km)
	if err != nil || *shutdown {
		return
	}

	if shuffle {
		pkgs = shuffleStrings(pkgs)
	}
	for i, pkg := range pkgs {
		if max <= 0 {
			log.Print("Max is reached")
			break
		}

		if *shutdown {
			err = nil
			return
		}
		log.Info().Msgf("%d/%d %s", i+1, len(pkgs), pkg)

		var attempt int64
		for {
			attempt++

			if *shutdown {
				err = nil
				return
			}

			err = installKernel(km, pkg, force, headers)
			if err == nil {
				max--
				break
			} else if attempt >= retries {
				log.Error().Err(err).Msg("install kernel")
				log.Debug().Msg("skip")
				break
			} else {
				log.Warn().Err(err).Msg("install kernel")
				time.Sleep(time.Second)
				log.Info().Msg("retry")
			}
		}
	}

	return
}
