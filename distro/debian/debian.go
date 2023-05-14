package debian

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"

	"code.dumpstack.io/tools/out-of-tree/config"
	"code.dumpstack.io/tools/out-of-tree/container"
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

func parseKernelMajorMinor(deb string) (major, minor int, err error) {
	// linux-image-4.17.0-2-amd64 -> 4.17
	re := regexp.MustCompile(`([0-9]*\.[0-9]*)`)

	s := re.FindString(deb)
	if s == "" {
		err = errors.New("empty result")
		return
	}

	split := strings.Split(s, ".")
	if len(split) != 2 {
		err = errors.New("unexpected input")
		return
	}

	major, err = strconv.Atoi(split[0])
	if err != nil {
		return
	}

	minor, err = strconv.Atoi(split[1])
	if err != nil {
		return
	}

	return
}

func kernelRelease(deb string) (r Release, err error) {
	major, minor, err := parseKernelMajorMinor(deb)
	if err != nil {
		return
	}

	// Wheezy 3.2
	// Jessie 3.16
	// Stretch 4.9
	// Buster 4.19
	// Bullseye 5.10

	if major < 3 {
		err = errors.New("not supported")
		return
	}

	switch major {
	case 3:
		//if minor < 16 {
		// >=3.8 breaks initramfs-tools << 0.110~
		// Wheezy initramfs-tools version is 0.109.1
		if minor < 8 {
			r = Wheezy
		} else {
			r = Jessie
		}
	case 4:
		if minor < 9 {
			r = Jessie
		} else if minor < 19 {
			r = Stretch
		} else {
			r = Buster
		}
	case 5:
		if minor < 10 {
			r = Buster
		} else {
			r = Bullseye
		}
	default:
		r = Bullseye // latest release
	}

	return
}

func MatchImagePkg(km config.KernelMask) (pkgs []string, err error) {
	kernels, err := GetKernels()
	if err != nil {
		log.Error().Err(err).Msg("get kernels")
		return
	}

	release := releaseFromString(km.DistroRelease)

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

func ContainerEnvs(km config.KernelMask) (envs []string) {
	envs = append(envs, "DEBIAN_FRONTEND=noninteractive")
	return
}

func ContainerImage(km config.KernelMask) (image string) {
	image += "debian:"

	switch releaseFromString(km.DistroRelease) {
	case Wheezy:
		image += "wheezy-20190228"
	case Jessie:
		image += "jessie-20210326"
	case Stretch:
		image += "stretch-20220622"
	default:
		image += km.DistroType.String()
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
	// case Jessie:
	// 	snapshot = "20230322T152120Z"
	// case Stretch:
	// 	snapshot = "20230423T032533Z"
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

func ContainerCommands(km config.KernelMask) (commands []string) {
	release := releaseFromString(km.DistroRelease)

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
		cmdf("sed -e '/snapshot/!d' -e 's/# deb/deb [check-valid-until=no trusted=yes]/' /etc/apt/sources.list")
	}

	cmdf("apt-get update")
	cmdf("apt-get install -y wget build-essential libelf-dev git")
	cmdf("apt-get install -y kmod module-init-tools linux-base")
	cmdf("apt-get install -y initramfs-tools")
	cmdf("mkdir -p /lib/modules")

	return
}

func ContainerKernels(d container.Image, kcfg *config.KernelConfig) (err error) {
	err = errors.New("TODO not implemented")
	return
}
