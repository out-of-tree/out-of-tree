// Copyright 2018 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"os/user"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/naoina/toml"

	"code.dumpstack.io/tools/out-of-tree/config"
)

type KernelCmd struct {
	NoDownload bool `help:"do not download qemu image while kernel generation"`
	UseHost    bool `help:"also use host kernels"`

	List        KernelListCmd        `cmd:"" help:"list kernels"`
	Autogen     KernelAutogenCmd     `cmd:"" help:"generate kernels based on the current config"`
	DockerRegen KernelDockerRegenCmd `cmd:"" help:"regenerate kernels config from out_of_tree_* docker images"`
	Genall      KernelGenallCmd      `cmd:"" help:"generate all kernels for distro"`
}

type KernelListCmd struct{}

func (cmd *KernelListCmd) Run(g *Globals) (err error) {
	kcfg, err := config.ReadKernelConfig(g.Config.Kernels)
	if err != nil {
		return
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

		err = generateKernels(sk, g.Config.Docker.Registry, g.Config.Docker.Commands, cmd.Max, !kernelCmd.NoDownload)
		if err != nil {
			return
		}
	}

	return updateKernelsCfg(kernelCmd.UseHost, !kernelCmd.NoDownload)
}

type KernelDockerRegenCmd struct{}

func (cmd *KernelDockerRegenCmd) Run(kernelCmd *KernelCmd, g *Globals) (err error) {
	dockerImages, err := listDockerImages()
	if err != nil {
		return
	}

	for _, d := range dockerImages {
		var imagePath string
		imagePath, err = dockerImagePath(config.KernelMask{
			DistroType:    d.DistroType,
			DistroRelease: d.DistroRelease,
		})
		if err != nil {
			return
		}

		args := []string{"build"}
		args = append(args, "-t", d.ContainerName, imagePath)

		cmd := exec.Command("docker", args...)
		var rawOutput []byte
		rawOutput, err = cmd.CombinedOutput()
		if err != nil {
			log.Println("docker build:", string(rawOutput))
			return
		}

		err = kickImage(d.ContainerName)
		if err != nil {
			log.Println("kick image", d.ContainerName, ":", err)
			continue
		}

		err = copyKernels(d.ContainerName)
		if err != nil {
			log.Println("copy kernels", d.ContainerName, ":", err)
			continue
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
		100, // FIXME docker-related limit is 127 layers
		!kernelCmd.NoDownload,
	)
	if err != nil {
		return
	}

	return updateKernelsCfg(kernelCmd.UseHost, !kernelCmd.NoDownload)
}

func matchDebianHeadersPkg(container, mask string, generic bool) (
	pkgs []string, err error) {

	cmd := "apt-cache search linux-headers | cut -d ' ' -f 1"
	output, err := dockerRun(time.Minute, container, "/tmp", cmd)
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
	output, err := dockerRun(time.Minute, container, "/tmp", cmd)
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

	path = usr.HomeDir + "/.out-of-tree/"
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
	rawOutput, err := cmd.CombinedOutput()
	if err != nil {
		return
	}

	if exists(dockerPath) && string(rawOutput) != "" {
		log.Printf("Base image for %s:%s found",
			sk.DistroType.String(), sk.DistroRelease)
		return
	}

	log.Printf("Base image for %s:%s not found, start generating",
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
			log.Println("Old CentOS requires `vsyscall=emulate` " +
				"on the latest kernels")
			log.Println("Check out `A note about vsyscall` " +
				"at https://hub.docker.com/_/centos")
			log.Println("See also https://lwn.net/Articles/446528/")
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

	args := []string{"build"}
	args = append(args, "-t", sk.DockerName(), imagePath)

	cmd = exec.Command("docker", args...)
	rawOutput, err = cmd.CombinedOutput()
	if err != nil {
		log.Printf("Base image for %s:%s generating error, see log",
			sk.DistroType.String(), sk.DistroRelease)
		log.Println(string(rawOutput))
		return
	}

	log.Printf("Base image for %s:%s generating success",
		sk.DistroType.String(), sk.DistroRelease)

	return
}

func dockerImageAppend(sk config.KernelMask, pkgname string) (err error) {
	imagePath, err := dockerImagePath(sk)
	if err != nil {
		return
	}

	raw, err := ioutil.ReadFile(imagePath + "/Dockerfile")
	if err != nil {
		return
	}

	if strings.Contains(string(raw), pkgname) {
		// already installed kernel
		log.Printf("kernel %s for %s:%s is already exists",
			pkgname, sk.DistroType.String(), sk.DistroRelease)
		return
	}

	var s string

	switch sk.DistroType {
	case config.Ubuntu:
		imagepkg := strings.Replace(pkgname, "headers", "image", -1)

		log.Printf("Start adding kernel %s for %s:%s",
			imagepkg, sk.DistroType.String(), sk.DistroRelease)

		s = fmt.Sprintf("RUN apt-get install -y %s %s\n", imagepkg,
			pkgname)
	case config.CentOS:
		imagepkg := strings.Replace(pkgname, "-devel", "", -1)

		log.Printf("Start adding kernel %s for %s:%s",
			imagepkg, sk.DistroType.String(), sk.DistroRelease)

		version := strings.Replace(pkgname, "kernel-devel-", "", -1)

		s = fmt.Sprintf("RUN yum -y install %s %s\n", imagepkg,
			pkgname)
		s += fmt.Sprintf("RUN dracut --add-drivers 'e1000 ext4' -f "+
			"/boot/initramfs-%s.img %s\n", version, version)
	default:
		err = fmt.Errorf("%s not yet supported", sk.DistroType.String())
		return
	}

	err = ioutil.WriteFile(imagePath+"/Dockerfile",
		append(raw, []byte(s)...), 0644)
	if err != nil {
		return
	}

	args := []string{"build"}
	args = append(args, "-t", sk.DockerName(), imagePath)

	cmd := exec.Command("docker", args...)
	rawOutput, err := cmd.CombinedOutput()
	if err != nil {
		// Fallback to previous state
		werr := ioutil.WriteFile(imagePath+"/Dockerfile", raw, 0644)
		if werr != nil {
			return
		}

		log.Printf("Add kernel %s for %s:%s error, see log",
			pkgname, sk.DistroType.String(), sk.DistroRelease)
		log.Println(string(rawOutput))
		return
	}

	log.Printf("Add kernel %s for %s:%s success",
		pkgname, sk.DistroType.String(), sk.DistroRelease)

	return
}

func kickImage(name string) (err error) {
	cmd := exec.Command("docker", "run", name, "bash", "-c", "ls")
	_, err = cmd.CombinedOutput()
	return
}

func copyKernels(name string) (err error) {
	cmd := exec.Command("docker", "ps", "-a")
	rawOutput, err := cmd.CombinedOutput()
	if err != nil {
		log.Println(string(rawOutput))
		return
	}

	r, err := regexp.Compile(".*" + name)
	if err != nil {
		return
	}

	var containerID string

	what := r.FindAll(rawOutput, -1)
	for _, w := range what {
		containerID = strings.Fields(string(w))[0]
		_, err = exec.Command("which", "podman").CombinedOutput()
		if err != nil {
			break
		}
	}

	usr, err := user.Current()
	if err != nil {
		return
	}

	target := usr.HomeDir + "/.out-of-tree/kernels/" + name + "/"
	if !exists(target) {
		os.MkdirAll(target, os.ModePerm)
	}

	cmd = exec.Command("docker", "cp", containerID+":/boot/.", target)
	rawOutput, err = cmd.CombinedOutput()
	if err != nil {
		log.Println(string(rawOutput))
		return
	}

	cmd = exec.Command("docker", "cp", containerID+":/lib/modules", target)
	rawOutput, err = cmd.CombinedOutput()
	if err != nil {
		log.Println(string(rawOutput))
		return
	}

	cmd = exec.Command("find", target+"modules", "-type", "l", "-delete")
	rawOutput, err = cmd.CombinedOutput()
	if err != nil {
		log.Println(string(rawOutput))
		return
	}

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
	return "unknown"
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
	return "unknown"
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
			log.Println(imageFile, "not exists, start downloading...")
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
			log.Println("gen kernels", d.ContainerName, ":", err)
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

	log.Println(kernelsCfgPath, "is successfully updated")
	return
}

func genDockerKernels(dii dockerImageInfo, newkcfg *config.KernelConfig,
	download bool) (err error) {

	name := dii.ContainerName
	cmd := exec.Command("docker", "run", name, "ls", "/lib/modules")
	rawOutput, err := cmd.CombinedOutput()
	if err != nil {
		log.Println(string(rawOutput), err)
		return
	}

	usr, err := user.Current()
	if err != nil {
		return
	}
	kernelsBase := usr.HomeDir + "/.out-of-tree/kernels/" + name + "/"
	files, err := ioutil.ReadDir(kernelsBase)
	if err != nil {
		return
	}

	rootfs, err := genRootfsImage(dii, download)
	if err != nil {
		return
	}

	for _, k := range strings.Fields(string(rawOutput)) {
		ki := config.KernelInfo{
			DistroType:    dii.DistroType,
			DistroRelease: dii.DistroRelease,
			KernelRelease: k,
			ContainerName: name,

			KernelPath:  kernelsBase + genKernelPath(files, k),
			InitrdPath:  kernelsBase + genInitrdPath(files, k),
			ModulesPath: kernelsBase + "modules/" + k,

			RootFS: rootfs,
		}
		newkcfg.Kernels = append(newkcfg.Kernels, ki)
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
	download bool) (err error) {

	log.Println("Generating for kernel mask", km)

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
			log.Println("Max is reached")
			break
		}

		log.Println(i, "/", len(pkgs), pkg)

		err = dockerImageAppend(km, pkg)
		if err == nil {
			max--
		} else {
			log.Println("dockerImageAppend", err)
		}
	}

	err = kickImage(km.DockerName())
	if err != nil {
		log.Println("kick image", km.DockerName(), ":", err)
		return
	}

	err = copyKernels(km.DockerName())
	if err != nil {
		log.Println("copy kernels", km.DockerName(), ":", err)
		return
	}
	return
}
