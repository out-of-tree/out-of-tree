// Copyright 2023 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package kernel

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/naoina/toml"
	"github.com/rs/zerolog/log"

	"code.dumpstack.io/tools/out-of-tree/cache"
	"code.dumpstack.io/tools/out-of-tree/config"
	"code.dumpstack.io/tools/out-of-tree/container"
	"code.dumpstack.io/tools/out-of-tree/distro"
	"code.dumpstack.io/tools/out-of-tree/distro/debian"
	"code.dumpstack.io/tools/out-of-tree/distro/oraclelinux"
	"code.dumpstack.io/tools/out-of-tree/distro/ubuntu"
	"code.dumpstack.io/tools/out-of-tree/fs"
)

func MatchPackages(km config.Target) (packages []string, err error) {
	pkgs, err := km.Distro.Packages()
	if err != nil {
		return
	}

	r, err := regexp.Compile(km.Kernel.Regex)
	if err != nil {
		return
	}

	for _, pkg := range pkgs {
		if r.MatchString(pkg) {
			packages = append(packages, pkg)
		}
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

func InstallKernel(sk config.Target, pkgname string, force, headers bool) (err error) {
	slog := log.With().
		Str("distro_type", sk.Distro.ID.String()).
		Str("distro_release", sk.Distro.Release).
		Str("pkg", pkgname).
		Logger()

	c, err := container.New(sk.Distro) // TODO conf
	if err != nil {
		return
	}

	searchdir := ""
	for _, volume := range c.Volumes {
		if volume.Dest == "/lib/modules" {
			searchdir = volume.Src
		}
	}

	if sk.Distro.ID == distro.Debian {
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

	if sk.Distro.ID == distro.Debian {
		// Debian has different kernels (package version) by the
		// same name (ABI), so we need to separate /boot
		c.Volumes = debian.Volumes(sk, pkgname)
	}

	slog.Debug().Msgf("Installing kernel")

	var commands []string

	// TODO install/cleanup kernel interface
	switch sk.Distro.ID {
	case distro.Ubuntu:
		commands, err = ubuntu.Install(sk, pkgname, headers)
		if err != nil {
			return
		}
		defer func() {
			if err != nil {
				ubuntu.Cleanup(sk, pkgname)
			}
		}()
	case distro.OracleLinux, distro.CentOS:
		commands, err = oraclelinux.Install(sk, pkgname, headers)
		if err != nil {
			return
		}
		defer func() {
			if err != nil {
				oraclelinux.Cleanup(sk, pkgname)
			}
		}()
	case distro.Debian:
		commands, err = debian.Install(sk, pkgname, headers)
		if err != nil {
			return
		}
		defer func() {
			if err != nil {
				debian.Cleanup(sk, pkgname)
			}
		}()
	default:
		err = fmt.Errorf("%s not yet supported", sk.Distro.ID.String())
		return
	}

	cmd := "true"
	for _, command := range commands {
		cmd += fmt.Sprintf(" && %s", command)
	}

	for i := range c.Volumes {
		c.Volumes[i].Dest = "/target" + c.Volumes[i].Dest
	}

	cmd += " && cp -r /boot /target/"
	cmd += " && cp -r /lib/modules /target/lib/"
	if sk.Distro.ID == distro.Debian {
		cmd += " && cp -rL /usr/src /target/usr/"
	} else {
		cmd += " && cp -r /usr/src /target/usr/"
	}

	_, err = c.Run("", []string{cmd})
	if err != nil {
		return
	}

	slog.Debug().Msgf("Success")
	return
}

func GenRootfsImage(d container.Image, download bool) (rootfs string, err error) {
	imagesPath := config.Dir("images")
	imageFile := d.Name + ".img"

	rootfs = filepath.Join(imagesPath, imageFile)
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

	kernelsCfgPath := config.File("kernels.toml")
	err = ioutil.WriteFile(kernelsCfgPath, buf, 0644)
	if err != nil {
		return
	}

	log.Info().Msgf("%s is successfully updated", kernelsCfgPath)
	return
}

func listContainersKernels(dii container.Image, newkcfg *config.KernelConfig,
	download bool) (err error) {

	c, err := container.New(dii.Distro)
	if err != nil {
		return
	}

	kernels, err := c.Kernels()
	if err != nil {
		return
	}

	newkcfg.Kernels = append(newkcfg.Kernels, kernels...)
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

func ShuffleStrings(a []string) []string {
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
