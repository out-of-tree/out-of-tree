// Copyright 2023 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"os/user"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/naoina/toml"
	"github.com/rs/zerolog/log"

	"code.dumpstack.io/tools/out-of-tree/config"
)

type KernelCmd struct {
	NoDownload bool `help:"do not download qemu image while kernel generation"`
	UseHost    bool `help:"also use host kernels"`
	Force      bool `help:"force reinstall kernel"`

	List    KernelListCmd    `cmd:"" help:"list kernels"`
	Autogen KernelAutogenCmd `cmd:"" help:"generate kernels based on the current config"`
	Genall  KernelGenallCmd  `cmd:"" help:"generate all kernels for distro"`
	Install KernelInstallCmd `cmd:"" help:"install specific kernel"`
}

type KernelListCmd struct{}

func (cmd *KernelListCmd) Run(g *Globals) (err error) {
	kcfg, err := config.ReadKernelConfig(g.Config.Kernels)
	if err != nil {
		log.Debug().Err(err).Msg("read kernel config")
	}

	if len(kcfg.Kernels) == 0 {
		return errors.New("No kernels found")
	}

	for _, k := range kcfg.Kernels {
		fmt.Println(k.DistroType, k.DistroRelease, k.KernelRelease)
	}

	return
}

type KernelAutogenCmd struct {
	Max int64 `help:"download random kernels from set defined by regex in release_mask, but no more than X for each of release_mask" default:"100"`
}

func (cmd KernelAutogenCmd) Run(kernelCmd *KernelCmd, g *Globals) (err error) {
	ka, err := config.ReadArtifactConfig(g.WorkDir + "/.out-of-tree.toml")
	if err != nil {
		return
	}

	for _, sk := range ka.SupportedKernels {
		if sk.DistroRelease == "" {
			err = errors.New("Please set distro_release")
			return
		}

		err = generateKernels(sk, g.Config.Docker.Registry, g.Config.Docker.Commands, cmd.Max, !kernelCmd.NoDownload, kernelCmd.Force)
		if err != nil {
			return
		}
	}

	return updateKernelsCfg(kernelCmd.UseHost, !kernelCmd.NoDownload)
}

type KernelGenallCmd struct {
	Distro string `required:"" help:"distribution"`
	Ver    string `required:"" help:"distro version"`
}

func (cmd *KernelGenallCmd) Run(kernelCmd *KernelCmd, g *Globals) (err error) {
	distroType, err := config.NewDistroType(cmd.Distro)
	if err != nil {
		return
	}

	km := config.KernelMask{
		DistroType:    distroType,
		DistroRelease: cmd.Ver,
		ReleaseMask:   ".*",
	}
	err = generateKernels(km,
		g.Config.Docker.Registry,
		g.Config.Docker.Commands,
		math.MaxUint32,
		!kernelCmd.NoDownload,
		kernelCmd.Force,
	)
	if err != nil {
		return
	}

	return updateKernelsCfg(kernelCmd.UseHost, !kernelCmd.NoDownload)
}

type KernelInstallCmd struct {
	Distro string `required:"" help:"distribution"`
	Ver    string `required:"" help:"distro version"`
	Kernel string `required:"" help:"kernel release mask"`
}

func (cmd *KernelInstallCmd) Run(kernelCmd *KernelCmd, g *Globals) (err error) {
	distroType, err := config.NewDistroType(cmd.Distro)
	if err != nil {
		return
	}

	km := config.KernelMask{
		DistroType:    distroType,
		DistroRelease: cmd.Ver,
		ReleaseMask:   cmd.Kernel,
	}
	err = generateKernels(km,
		g.Config.Docker.Registry,
		g.Config.Docker.Commands,
		math.MaxUint32,
		!kernelCmd.NoDownload,
		kernelCmd.Force,
	)
	if err != nil {
		return
	}

	return updateKernelsCfg(kernelCmd.UseHost, !kernelCmd.NoDownload)
}

func matchDebianHeadersPkg(container, mask string, generic bool) (
	pkgs []string, err error) {

	cmd := "apt-cache search linux-headers | cut -d ' ' -f 1"

	c, err := NewContainer(container, time.Minute)
	if err != nil {
		return
	}

	output, err := c.Run("/tmp", cmd)
	if err != nil {
		return
	}

	r, err := regexp.Compile("linux-headers-" + mask)
	if err != nil {
		return
	}

	kernels := r.FindAll([]byte(output), -1)

	for _, k := range kernels {
		pkg := string(k)
		if generic && !strings.HasSuffix(pkg, "generic") {
			continue
		}
		if pkg == "linux-headers-generic" {
			continue
		}
		pkgs = append(pkgs, pkg)
	}

	return
}

func matchCentOSDevelPkg(container, mask string, generic bool) (
	pkgs []string, err error) {

	cmd := "yum search kernel-devel --showduplicates | " +
		"grep '^kernel-devel' | cut -d ' ' -f 1"

	c, err := NewContainer(container, time.Minute)
	if err != nil {
		return
	}

	output, err := c.Run("/tmp", cmd)
	if err != nil {
		return
	}

	r, err := regexp.Compile("kernel-devel-" + mask)
	if err != nil {
		return
	}

	for _, k := range r.FindAll([]byte(output), -1) {
		pkgs = append(pkgs, string(k))
	}

	return
}

func dockerImagePath(sk config.KernelMask) (path string, err error) {
	usr, err := user.Current()
	if err != nil {
		return
	}

	path = usr.HomeDir + "/.out-of-tree/containers/"
	path += sk.DistroType.String() + "/" + sk.DistroRelease
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

func generateBaseDockerImage(registry string, commands []config.DockerCommand,
	sk config.KernelMask) (err error) {

	imagePath, err := dockerImagePath(sk)
	if err != nil {
		return
	}
	dockerPath := imagePath + "/Dockerfile"

	d := "# BASE\n"

	cmd := exec.Command("docker", "images", "-q", sk.DockerName())
	log.Debug().Msgf("run %v", cmd)

	rawOutput, err := cmd.CombinedOutput()
	if err != nil {
		return
	}

	if exists(dockerPath) && string(rawOutput) != "" {
		log.Info().Msgf("Base image for %s:%s found",
			sk.DistroType.String(), sk.DistroRelease)
		return
	}

	log.Info().Msgf("Base image for %s:%s not found, start generating",
		sk.DistroType.String(), sk.DistroRelease)
	os.MkdirAll(imagePath, os.ModePerm)

	d += "FROM "
	if registry != "" {
		d += registry + "/"
	}

	d += fmt.Sprintf("%s:%s\n",
		strings.ToLower(sk.DistroType.String()),
		sk.DistroRelease,
	)

	vsyscall, err := vsyscallAvailable()
	if err != nil {
		return
	}

	for _, c := range commands {
		switch c.DistroType {
		case config.Ubuntu:
			d += "RUN " + c.Command + "\n"
		case config.CentOS:
			d += "RUN " + c.Command + "\n"
		case config.Debian:
			d += "RUN " + c.Command + "\n"
		default:
			err = fmt.Errorf("%s not yet supported",
				sk.DistroType.String())
			return
		}
	}

	switch sk.DistroType {
	case config.Ubuntu:
		d += "ENV DEBIAN_FRONTEND=noninteractive\n"
		d += "RUN apt-get update\n"
		d += "RUN apt-get install -y build-essential libelf-dev\n"
		d += "RUN apt-get install -y wget git\n"
		if sk.DistroRelease >= "14.04" {
			d += "RUN apt-get install -y libseccomp-dev\n"
		}
		d += "RUN mkdir -p /lib/modules\n"
	case config.CentOS:
		if sk.DistroRelease < "7" && !vsyscall {
			log.Print("Old CentOS requires `vsyscall=emulate` " +
				"on the latest kernels")
			log.Print("Check out `A note about vsyscall` " +
				"at https://hub.docker.com/_/centos")
			log.Print("See also https://lwn.net/Articles/446528/")
			err = fmt.Errorf("vsyscall is not available")
			return
		}

		// enable rpms from old minor releases
		d += "RUN sed -i 's/enabled=0/enabled=1/' /etc/yum.repos.d/CentOS-Vault.repo\n"
		// do not remove old kernels
		d += "RUN sed -i 's;installonly_limit=;installonly_limit=100500;' /etc/yum.conf\n"
		d += "RUN yum -y update\n"

		if sk.DistroRelease == "8" {
			// FIXME CentOS Vault repository list for 8 is empty
			// at the time of this fix; check for it and use a
			// workaround if it's still empty
			d += `RUN grep enabled /etc/yum.repos.d/CentOS-Vault.repo` +
				` || echo -e '[8.0.1905]\nbaseurl=http://vault.centos.org/8.0.1905/BaseOS/$basearch/os/'` +
				` >> /etc/yum.repos.d/CentOS-Vault.repo` + "\n"
		}

		d += "RUN yum -y groupinstall 'Development Tools'\n"

		if sk.DistroRelease < "8" {
			d += "RUN yum -y install deltarpm\n"
		} else {
			d += "RUN yum -y install drpm grub2-tools-minimal " +
				"elfutils-libelf-devel\n"
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

	c, err := NewContainer(sk.DockerName(), time.Hour)
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

	log.Info().Msgf("Base image for %s:%s generating success",
		sk.DistroType.String(), sk.DistroRelease)

	return
}

func installKernel(sk config.KernelMask, pkgname string, force bool) (err error) {
	slog := log.With().
		Str("distro_type", sk.DistroType.String()).
		Str("distro_release", sk.DistroRelease).
		Str("pkg", pkgname).
		Logger()

	c, err := NewContainer(sk.DockerName(), time.Hour) // TODO conf
	if err != nil {
		return
	}

	moddirs, err := ioutil.ReadDir(c.Volumes.LibModules)
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

	slog.Debug().Msgf("Installing kernel")

	switch sk.DistroType {
	case config.Ubuntu:
		imagepkg := strings.Replace(pkgname, "headers", "image", -1)

		cmd := fmt.Sprintf("apt-get install -y %s %s", imagepkg,
			pkgname)

		_, err = c.Run("/tmp", cmd)
		if err != nil {
			return
		}
	case config.CentOS:
		imagepkg := strings.Replace(pkgname, "-devel", "", -1)

		version := strings.Replace(pkgname, "kernel-devel-", "", -1)

		cmd := fmt.Sprintf("yum -y install %s %s\n", imagepkg,
			pkgname)
		_, err = c.Run("/tmp", cmd)
		if err != nil {
			return
		}

		cmd = fmt.Sprintf("dracut --add-drivers 'e1000 ext4' -f "+
			"/boot/initramfs-%s.img %s\n", version, version)
		_, err = c.Run("/tmp", cmd)
		if err != nil {
			return
		}
	default:
		err = fmt.Errorf("%s not yet supported", sk.DistroType.String())
		return
	}

	slog.Debug().Msgf("Success")
	return
}

func genKernelPath(files []os.FileInfo, kname string) string {
	for _, file := range files {
		if strings.HasPrefix(file.Name(), "vmlinuz") {
			if strings.Contains(file.Name(), kname) {
				return file.Name()
			}
		}
	}

	log.Fatal().Msgf("cannot find kernel %s", kname)
	return ""
}

func genInitrdPath(files []os.FileInfo, kname string) string {
	for _, file := range files {
		if strings.HasPrefix(file.Name(), "initrd") ||
			strings.HasPrefix(file.Name(), "initramfs") {

			if strings.Contains(file.Name(), kname) {
				return file.Name()
			}
		}
	}

	log.Fatal().Msgf("cannot find initrd %s", kname)
	return ""
}

func genRootfsImage(d dockerImageInfo, download bool) (rootfs string, err error) {
	usr, err := user.Current()
	if err != nil {
		return
	}
	imageFile := d.ContainerName + ".img"

	imagesPath := usr.HomeDir + "/.out-of-tree/images/"
	os.MkdirAll(imagesPath, os.ModePerm)

	rootfs = imagesPath + imageFile
	if !exists(rootfs) {
		if download {
			log.Print(imageFile, "not exists, start downloading...")
			err = downloadImage(imagesPath, imageFile)
		}
	}
	return
}

type dockerImageInfo struct {
	ContainerName string
	DistroType    config.DistroType
	DistroRelease string // 18.04/7.4.1708/9.1
}

func listDockerImages() (diis []dockerImageInfo, err error) {
	cmd := exec.Command("docker", "images")
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

		dii := dockerImageInfo{
			ContainerName: container,
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

func updateKernelsCfg(host, download bool) (err error) {
	newkcfg := config.KernelConfig{}

	if host {
		// Get host kernels
		newkcfg, err = genHostKernels(download)
		if err != nil {
			return
		}
	}

	// Get docker kernels
	dockerImages, err := listDockerImages()
	if err != nil {
		return
	}

	for _, d := range dockerImages {
		err = genDockerKernels(d, &newkcfg, download)
		if err != nil {
			log.Print("gen kernels", d.ContainerName, ":", err)
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

func genDockerKernels(dii dockerImageInfo, newkcfg *config.KernelConfig,
	download bool) (err error) {

	rootfs, err := genRootfsImage(dii, download)
	if err != nil {
		return
	}

	c, err := NewContainer(dii.ContainerName, time.Minute)
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
		ki := config.KernelInfo{
			DistroType:    dii.DistroType,
			DistroRelease: dii.DistroRelease,
			KernelRelease: krel.Name(),
			ContainerName: dii.ContainerName,

			KernelPath: c.Volumes.Boot + "/" +
				genKernelPath(bootfiles, krel.Name()),
			InitrdPath: c.Volumes.Boot + "/" +
				genInitrdPath(bootfiles, krel.Name()),
			ModulesPath: c.Volumes.LibModules + "/" + krel.Name(),

			RootFS: rootfs,
		}
		newkcfg.Kernels = append(newkcfg.Kernels, ki)

		for _, cmd := range []string{
			"find /boot -type f -exec chmod a+r {} \\;",
			"find /boot -type d -exec chmod a+rx {} \\;",
			"find /usr/src -type f -exec chmod a+r {} \\;",
			"find /usr/src -type d -exec chmod a+rx {} \\;",
			"find /lib/modules -type f -exec chmod a+r {} \\;",
			"find /lib/modules -type d -exec chmod a+rx {} \\;",
		} {
			_, err = c.Run("/tmp", cmd)
			if err != nil {
				return
			}
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

func shuffle(a []string) []string {
	// Fisher–Yates shuffle
	for i := len(a) - 1; i > 0; i-- {
		j := rand.Intn(i + 1)
		a[i], a[j] = a[j], a[i]
	}
	return a
}

func generateKernels(km config.KernelMask, registry string,
	commands []config.DockerCommand, max int64,
	download, force bool) (err error) {

	log.Info().Msgf("Generating for kernel mask %v", km)

	_, err = genRootfsImage(dockerImageInfo{ContainerName: km.DockerName()},
		download)
	if err != nil {
		return
	}

	err = generateBaseDockerImage(registry, commands, km)
	if err != nil {
		return
	}

	var pkgs []string
	switch km.DistroType {
	case config.Ubuntu:
		pkgs, err = matchDebianHeadersPkg(km.DockerName(),
			km.ReleaseMask, true)
	case config.CentOS:
		pkgs, err = matchCentOSDevelPkg(km.DockerName(),
			km.ReleaseMask, true)
	default:
		err = fmt.Errorf("%s not yet supported", km.DistroType.String())
	}
	if err != nil {
		return
	}

	for i, pkg := range shuffle(pkgs) {
		if max <= 0 {
			log.Print("Max is reached")
			break
		}

		log.Info().Msgf("%d/%d %s", i+1, len(pkgs), pkg)

		err = installKernel(km, pkg, force)
		if err == nil {
			max--
		} else {
			log.Fatal().Err(err).Msg("install kernel")
		}
	}

	return
}
