package debian

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/rs/zerolog/log"

	"code.dumpstack.io/tools/out-of-tree/cache"
	"code.dumpstack.io/tools/out-of-tree/config"
	"code.dumpstack.io/tools/out-of-tree/container"
	"code.dumpstack.io/tools/out-of-tree/distro/debian/snapshot"
	"code.dumpstack.io/tools/out-of-tree/fs"
)

type Release int

const (
	None Release = iota
	Buzz
	Hamm
	Woody
	Etch
	Lenny
	Squeeze
	Wheezy
	Jessie
	Stretch
	Buster
	Bullseye
	Bookworm
)

var ReleaseStrings = [...]string{
	"",
	"buzz",
	"hamm",
	"woody",
	"etch",
	"lenny",
	"squeeze",
	"wheezy",
	"jessie",
	"stretch",
	"buster",
	"bullseye",
	"bookworm",
}

func (cn Release) String() string {
	return ReleaseStrings[cn]
}

func releaseFromString(s string) (r Release) {
	switch strings.ToLower(s) {
	case "7", "wheezy":
		r = Wheezy
	case "8", "jessie":
		r = Jessie
	case "9", "stretch":
		r = Stretch
	case "10", "buster":
		r = Buster
	case "11", "bullseye":
		r = Bullseye
	default:
		r = None
	}
	return
}

func kernelRelease(deb string) (r Release, err error) {
	// linux-image-4.17.0-2-amd64 -> 4.17
	re := regexp.MustCompile(`([0-9]*\.[0-9]*)`)
	sver := re.FindString(deb)
	if sver == "" {
		err = errors.New("empty result")
		return
	}
	version := kver(sver)

	if version.LessThan(kver("3.0-rc0")) {
		err = errors.New("not supported")
		return
	}

	if version.LessThan(kver("3.8-rc0")) {
		// Wheezy 3.2
		// >=3.8 breaks initramfs-tools << 0.110~
		// Wheezy initramfs-tools version is 0.109.1
		r = Wheezy
	} else if version.LessThan(kver("4.9-rc0")) {
		// Jessie 3.16
		r = Jessie
	} else if version.LessThan(kver("4.19-rc0")) {
		// Stretch 4.9
		r = Stretch
	} else if version.LessThan(kver("5.10-rc0")) {
		// Buster 4.19
		r = Buster
	} else {
		// Bullseye 5.10
		r = Bullseye
	}

	return
}

func Match(km config.Target) (pkgs []string, err error) {
	kernels, err := GetKernels()
	if err != nil {
		log.Error().Err(err).Msg("get kernels")
		return
	}

	release := releaseFromString(km.Distro.Release)

	r := regexp.MustCompile(km.ReleaseMask)

	for _, dk := range kernels {
		p := strings.Replace(dk.Image.Deb.Name, ".deb", "", -1)

		var kr Release
		kr, err = kernelRelease(p)
		if err != nil {
			log.Warn().Err(err).Msg("")
			continue
		}
		if kr != release {
			continue
		}

		if r.MatchString(p) {
			pkgs = append(pkgs, p)
		}
	}

	return
}

func Envs(km config.Target) (envs []string) {
	envs = append(envs, "DEBIAN_FRONTEND=noninteractive")
	return
}

func ContainerImage(km config.Target) (image string) {
	image += "debian:"

	switch releaseFromString(km.Distro.Release) {
	case Wheezy:
		image += "wheezy-20190228"
	case Jessie:
		image += "jessie-20210326"
	case Stretch:
		image += "stretch-20220622"
	default:
		image += km.Distro.Release
	}

	return
}

func repositories(release Release) (repos []string) {
	var snapshot string

	switch release {
	// Latest snapshots that include release
	case Wheezy:
		// doesn't include snapshot repos in /etc/apt/source.list
		snapshot = "20190321T212815Z"
	case Jessie:
		snapshot = "20230322T152120Z"
	case Stretch:
		snapshot = "20230423T032533Z"
	default:
		return
	}

	repo := func(archive, s string) {
		format := "deb [check-valid-until=no trusted=yes] " +
			"http://snapshot.debian.org/archive/%s/%s " +
			"%s%s main"
		r := fmt.Sprintf(format, archive, snapshot, release, s)
		repos = append(repos, r)
	}

	repo("debian", "")
	repo("debian", "-updates")
	repo("debian-security", "/updates")

	return
}

func Runs(km config.Target) (commands []string) {
	release := releaseFromString(km.Distro.Release)

	cmdf := func(f string, s ...interface{}) {
		commands = append(commands, fmt.Sprintf(f, s...))
	}

	repos := repositories(release)

	if len(repos) != 0 {
		cmdf("rm /etc/apt/sources.list")
		for _, repo := range repos {
			cmdf("echo '%s' >> /etc/apt/sources.list", repo)
		}
	} else {
		cmdf("apt-get update || sed -i " +
			"-e '/snapshot/!d' " +
			"-e 's/# deb/deb [check-valid-until=no trusted=yes]/' " +
			"/etc/apt/sources.list")
	}

	cmdf("apt-get update || apt-get update || apt-get update")

	pkglist := []string{
		"wget", "build-essential", "libelf-dev", "git",
		"kmod", "linux-base", "initramfs-tools", "libssl-dev",
		"'^(gcc-[0-9].[0-9]|gcc-[0-9])$'",
	}

	if release < 9 {
		pkglist = append(pkglist, "module-init-tools")
	}

	var packages string
	for _, pkg := range pkglist {
		packages += fmt.Sprintf("%s ", pkg)
	}

	cmdf("timeout 5m apt-get install -y %s "+
		"|| timeout 10m apt-get install -y %s "+
		"|| apt-get install -y %s", packages, packages, packages)

	cmdf("mkdir -p /lib/modules")

	return
}

func ContainerKernels(d container.Image, kcfg *config.KernelConfig) (err error) {
	cpath := config.Dir("volumes", d.Name)
	rootfs := config.File("images", d.Name+".img")

	files, err := os.ReadDir(cpath)
	if err != nil {
		return
	}

	for _, file := range files {
		if !strings.Contains(file.Name(), "linux-image") {
			continue
		}

		pkgname := file.Name()

		kpkgdir := filepath.Join(cpath, pkgname)

		bootdir := filepath.Join(kpkgdir, "boot")

		vmlinuz, err := fs.FindBySubstring(bootdir, "vmlinuz")
		if err != nil {
			log.Warn().Msgf("cannot find vmlinuz for %s", pkgname)
			continue
		}

		initrd, err := fs.FindBySubstring(bootdir, "initrd")
		if err != nil {
			log.Warn().Msgf("cannot find initrd for %s", pkgname)
			continue
		}

		modulesdir := filepath.Join(kpkgdir, "lib/modules")

		modules, err := fs.FindBySubstring(modulesdir, "")
		if err != nil {
			log.Warn().Msgf("cannot find modules for %s", pkgname)
			continue
		}

		log.Debug().Msgf("%s %s %s", vmlinuz, initrd, modules)

		release := strings.Replace(pkgname, "linux-image-", "", -1)

		ki := config.KernelInfo{
			Distro:        d.Distro,
			KernelVersion: path.Base(modules),
			KernelRelease: release,
			ContainerName: d.Name,

			KernelPath:  vmlinuz,
			InitrdPath:  initrd,
			ModulesPath: modules,

			RootFS: rootfs,
		}

		kcfg.Kernels = append(kcfg.Kernels, ki)
	}

	return
}

func Volumes(km config.Target, pkgname string) (volumes container.Volumes) {
	pkgdir := filepath.Join("volumes", km.DockerName(), pkgname)

	volumes.LibModules = config.Dir(pkgdir, "/lib/modules")
	volumes.UsrSrc = config.Dir(pkgdir, "/usr/src")
	volumes.Boot = config.Dir(pkgdir, "/boot")

	return
}

func Install(km config.Target, pkgname string, headers bool) (cmds []string, err error) {
	dk, err := getCachedKernel(pkgname + ".deb")
	if err != nil {
		return
	}

	var pkgs []snapshot.Package
	if headers {
		pkgs = dk.Packages()
	} else {
		pkgs = []snapshot.Package{dk.Image}
	}

	for _, pkg := range pkgs {
		found, newurl := cache.PackageURL(
			km.Distro.ID,
			pkg.Deb.URL,
		)
		if found {
			log.Debug().Msgf("cached deb found %s", newurl)
			pkg.Deb.URL = newurl
		}

		// TODO use faketime on old releases?
		pkg.Deb.URL = strings.Replace(pkg.Deb.URL, "https", "http", -1)

		cmds = append(cmds, "wget "+
			"--timeout=10 --waitretry=1 --tries=10 "+
			"--no-check-certificate "+pkg.Deb.URL)
	}

	cmds = append(cmds, "dpkg -i ./*.deb")

	return
}

func Cleanup(km config.Target, pkgname string) {
	pkgdir := config.Dir(filepath.Join("volumes", km.DockerName(), pkgname))

	log.Debug().Msgf("cleanup %s", pkgdir)

	err := os.RemoveAll(pkgdir)
	if err != nil {
		log.Warn().Err(err).Msg("cleanup")
	}
}
