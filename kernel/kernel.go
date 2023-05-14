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
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/naoina/toml"
	"github.com/rs/zerolog/log"

	"code.dumpstack.io/tools/out-of-tree/cache"
	"code.dumpstack.io/tools/out-of-tree/config"
	"code.dumpstack.io/tools/out-of-tree/container"
	"code.dumpstack.io/tools/out-of-tree/distro/debian"
	"code.dumpstack.io/tools/out-of-tree/distro/debian/snapshot"
	"code.dumpstack.io/tools/out-of-tree/fs"
)

func matchDebImagePkg(containerName, mask string) (pkgs []string, err error) {

	cmd := "apt-cache search --names-only '^linux-image-[0-9\\.\\-]*-generic' | awk '{ print $1 }'"

	// FIXME timeout should be in global out-of-tree config
	c, err := container.New(containerName, time.Hour)
	if err != nil {
		return
	}

	output, err := c.Run(config.Dir("tmp"), cmd)
	if err != nil {
		return
	}

	r, err := regexp.Compile("linux-image-" + mask)
	if err != nil {
		return
	}

	for _, pkg := range strings.Fields(output) {
		if r.MatchString(pkg) || strings.Contains(pkg, mask) {
			pkgs = append(pkgs, pkg)
		}
	}

	return
}

func matchOracleLinuxPkg(containerName, mask string) (
	pkgs []string, err error) {

	cmd := "yum search kernel --showduplicates " +
		"| grep '^kernel-[0-9]\\|^kernel-uek-[0-9]' " +
		"| grep -v src " +
		"| cut -d ' ' -f 1"

	// FIXME timeout should be in global out-of-tree config
	c, err := container.New(containerName, time.Hour)
	if err != nil {
		return
	}

	output, err := c.Run(config.Dir("tmp"), cmd)
	if err != nil {
		return
	}

	r, err := regexp.Compile("kernel-" + mask)
	if err != nil {
		return
	}

	for _, pkg := range strings.Fields(output) {
		if r.MatchString(pkg) || strings.Contains(pkg, mask) {
			log.Trace().Msg(pkg)
			pkgs = append(pkgs, pkg)
		}
	}

	if len(pkgs) == 0 {
		log.Warn().Msg("no packages matched")
	}

	return
}

func MatchPackages(km config.KernelMask) (pkgs []string, err error) {
	switch km.DistroType {
	case config.Ubuntu:
		pkgs, err = matchDebImagePkg(km.DockerName(), km.ReleaseMask)
	case config.OracleLinux, config.CentOS:
		pkgs, err = matchOracleLinuxPkg(km.DockerName(), km.ReleaseMask)
	case config.Debian:
		pkgs, err = debian.MatchImagePkg(km)
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

	switch sk.DistroType {
	case config.Ubuntu:
		if sk.DistroRelease < "14.04" {
			d += "RUN sed -i 's/archive.ubuntu.com/old-releases.ubuntu.com/' /etc/apt/sources.list\n"
		}
		d += "ENV DEBIAN_FRONTEND=noninteractive\n"
		d += "RUN apt-get update\n"
		d += "RUN apt-get install -y build-essential libelf-dev\n"
		d += "RUN apt-get install -y wget git\n"
		// Install a single kernel and headers to ensure all dependencies are cached
		if sk.DistroRelease >= "14.04" {
			d += "RUN export PKGNAME=$(apt-cache search --names-only '^linux-headers-[0-9\\.\\-]*-generic' | awk '{ print $1 }' | head -n 1); " +
				"apt-get install -y $PKGNAME $(echo $PKGNAME | sed 's/headers/image/'); " +
				"apt-get remove -y $PKGNAME $(echo $PKGNAME | sed 's/headers/image/')\n"
			d += "RUN apt-get install -y libseccomp-dev\n"
		}
		d += "RUN mkdir -p /lib/modules\n"
	case config.CentOS:
		var repos []string

		switch sk.DistroRelease {
		case "6":
			repofmt := "[6.%d-%s]\\nbaseurl=https://vault.centos.org/6.%d/%s/$basearch/\\ngpgcheck=0"
			for i := 0; i <= 10; i++ {
				repos = append(repos, fmt.Sprintf(repofmt, i, "os", i, "os"))
				repos = append(repos, fmt.Sprintf(repofmt, i, "updates", i, "updates"))
			}
			d += "RUN rm /etc/yum.repos.d/*\n"
		case "7":
			repofmt := "[%s-%s]\\nbaseurl=https://vault.centos.org/%s/%s/$basearch/\\ngpgcheck=0"
			for _, ver := range []string{
				"7.0.1406", "7.1.1503", "7.2.1511",
				"7.3.1611", "7.4.1708", "7.5.1804",
				"7.6.1810", "7.7.1908", "7.8.2003",
			} {
				repos = append(repos, fmt.Sprintf(repofmt, ver, "os", ver, "os"))
				repos = append(repos, fmt.Sprintf(repofmt, ver, "updates", ver, "updates"))
			}

			// FIXME http/gpgcheck=0
			repofmt = "[%s-%s]\\nbaseurl=http://mirror.centos.org/centos-7/%s/%s/$basearch/\\ngpgcheck=0"
			repos = append(repos, fmt.Sprintf(repofmt, "7.9.2009", "os", "7.9.2009", "os"))
			repos = append(repos, fmt.Sprintf(repofmt, "7.9.2009", "updates", "7.9.2009", "updates"))
		case "8":
			repofmt := "[%s]\\nbaseurl=https://vault.centos.org/%s/BaseOS/$basearch/os/\\ngpgcheck=0"

			for _, ver := range []string{
				"8.0.1905", "8.1.1911", "8.2.2004",
				"8.3.2011", "8.4.2105", "8.5.2111",
			} {
				repos = append(repos, fmt.Sprintf(repofmt, ver, ver))
			}
		default:
			err = fmt.Errorf("no support for %s %s", sk.DistroType, sk.DistroRelease)
			return
		}

		d += "RUN sed -i 's/enabled=1/enabled=0/' /etc/yum.repos.d/* || true\n"

		for _, repo := range repos {
			d += fmt.Sprintf("RUN echo -e '%s' >> /etc/yum.repos.d/oot.repo\n", repo)
		}

		// do not remove old kernels
		d += "RUN sed -i 's;installonly_limit=;installonly_limit=100500;' /etc/yum.conf\n"
		d += "RUN yum -y update\n"

		d += "RUN yum -y groupinstall 'Development Tools'\n"

		if sk.DistroRelease < "8" {
			d += "RUN yum -y install deltarpm\n"
		} else {
			d += "RUN yum -y install grub2-tools-minimal " +
				"elfutils-libelf-devel\n"
		}

		var flags string
		if sk.DistroRelease >= "8" {
			flags = "--noautoremove"
		}

		// Cache kernel package dependencies
		d += "RUN export PKGNAME=$(yum search kernel-devel --showduplicates | grep '^kernel-devel' | cut -d ' ' -f 1 | head -n 1); " +
			"yum -y install $PKGNAME $(echo $PKGNAME | sed 's/-devel//'); " +
			fmt.Sprintf("yum -y remove $PKGNAME "+
				"$(echo $PKGNAME | sed 's/-devel//') "+
				"$(echo $PKGNAME | sed 's/-devel/-modules/') "+
				"$(echo $PKGNAME | sed 's/-devel/-core/') %s\n", flags)
	case config.OracleLinux:
		if sk.DistroRelease < "6" {
			err = fmt.Errorf("no support for pre-EL6")
			return
		}
		d += "RUN sed -i 's/enabled=0/enabled=1/' /etc/yum.repos.d/*\n"
		d += "RUN sed -i 's;installonly_limit=;installonly_limit=100500;' /etc/yum.conf /etc/dnf/dnf.conf || true\n"
		d += "RUN yum -y update\n"
		d += "RUN yum -y groupinstall 'Development Tools'\n"
		packages := "linux-firmware grubby"
		if sk.DistroRelease <= "7" {
			packages += " libdtrace-ctf"
		}
		d += fmt.Sprintf("RUN yum -y install %s\n", packages)
	case config.Debian:
		for _, e := range debian.ContainerEnvs(sk) {
			d += "ENV " + e + "\n"
		}
		for _, c := range debian.ContainerCommands(sk) {
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

	volumes := c.Volumes

	c.Volumes.LibModules = ""
	c.Volumes.UsrSrc = ""
	c.Volumes.Boot = ""

	slog.Debug().Msgf("Installing kernel")

	// TODO use list of commands instead of appending to string
	cmd := "true"

	switch sk.DistroType {
	case config.Ubuntu:
		var headerspkg string
		if headers {
			headerspkg = strings.Replace(pkgname, "image", "headers", -1)
		}

		cmd += fmt.Sprintf(" && apt-get install -y %s %s", pkgname, headerspkg)
	case config.OracleLinux, config.CentOS:
		var headerspkg string
		if headers {
			if strings.Contains(pkgname, "uek") {
				headerspkg = strings.Replace(pkgname,
					"kernel-uek", "kernel-uek-devel", -1)
			} else {
				headerspkg = strings.Replace(pkgname,
					"kernel", "kernel-devel", -1)
			}
		}

		cmd += fmt.Sprintf(" && yum -y install %s %s", pkgname, headerspkg)

		var version string
		if strings.Contains(pkgname, "uek") {
			version = strings.Replace(pkgname, "kernel-uek-", "", -1)
		} else {
			version = strings.Replace(pkgname, "kernel-", "", -1)
		}

		if sk.DistroRelease <= "7" {
			cmd += fmt.Sprintf(" && dracut -v --add-drivers 'e1000 ext4' -f "+
				"/boot/initramfs-%s.img %s", version, version)
		} else {
			cmd += fmt.Sprintf(" && dracut -v --add-drivers 'ata_piix libata' --force-drivers 'e1000 ext4 sd_mod' -f "+
				"/boot/initramfs-%s.img %s", version, version)
		}
	case config.Debian:
		var dk debian.DebianKernel
		dk, err = debian.GetCachedKernel(pkgname + ".deb")
		if err != nil {
			return
		}

		// Debian has different kernels (package version) by the
		// same name (ABI), so we need to separate /boot

		volumes.LibModules = config.Dir("volumes", sk.DockerName(),
			pkgname, "/lib/modules")
		volumes.UsrSrc = config.Dir("volumes", sk.DockerName(),
			pkgname, "/usr/src")
		volumes.Boot = config.Dir("volumes", sk.DockerName(),
			pkgname, "/boot")

		pkgs := []snapshot.Package{dk.Image}
		if headers {
			pkgs = append(pkgs, dk.Headers)
		}

		for _, pkg := range pkgs {
			cmd += fmt.Sprintf(" && wget --no-check-certificate %s",
				pkg.Deb.URL)
			cmd += fmt.Sprintf(" && dpkg -i %s",
				pkg.Deb.Name)
			cmd += fmt.Sprintf(" && rm %s",
				pkg.Deb.Name)
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
	cmd += " && cp -r /usr/src /target/usr/"

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
