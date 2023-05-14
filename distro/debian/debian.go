package debian

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"

	"code.dumpstack.io/tools/out-of-tree/cache"
	"code.dumpstack.io/tools/out-of-tree/config"
	"code.dumpstack.io/tools/out-of-tree/container"
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
		if minor < 16 {
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

var (
	CachePath   string
	RefetchDays int = 7
)

func MatchImagePkg(km config.KernelMask) (pkgs []string, err error) {
	if CachePath == "" {
		CachePath = config.File("debian.cache")
		log.Debug().Msgf("Use default kernels cache path: %s", CachePath)

		if !fs.PathExists(CachePath) {
			log.Debug().Msgf("No cache, download")
			err = cache.DownloadDebianCache(CachePath)
			if err != nil {
				log.Debug().Err(err).Msg(
					"No remote cache, will take some time")
			}
		}
	} else {
		log.Debug().Msgf("Debian kernels cache path: %s", CachePath)
	}

	c, err := NewCache(CachePath)
	if err != nil {
		log.Error().Err(err).Msg("cache")
		return
	}
	defer c.Close()

	kernels, err := GetKernels(c, RefetchDays)
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

func ContainerCommands(km config.KernelMask) (commands []string) {
	release := releaseFromString(km.DistroRelease)

	var snapshot string

	switch release {
	case Wheezy:
		snapshot = "20160605T173546Z"
	case Jessie:
		snapshot = "20190212T020859Z"
	case Stretch:
		snapshot = "20200719T110630Z"
	case Buster:
		snapshot = "20220911T180237Z"
	case Bullseye:
		snapshot = "20221218T103830Z"
	default:
		log.Fatal().Msgf("%s not supported", release)
		return
	}

	params := "[check-valid-until=no trusted=yes]"
	mirror := "http://snapshot.debian.org"
	repourl := fmt.Sprintf("%s/archive/debian/%s/", mirror, snapshot)

	cmdf := func(f string, s ...interface{}) {
		commands = append(commands, fmt.Sprintf(f, s...))
	}

	repo := fmt.Sprintf("deb %s %s %s main contrib",
		params, repourl, release)

	cmdf("echo '%s' > /etc/apt/sources.list", repo)

	repo = fmt.Sprintf("deb %s %s %s-updates main contrib",
		params, repourl, release)

	cmdf("echo '%s' >> /etc/apt/sources.list", repo)

	cmdf("apt-get update")
	cmdf("apt-get install -y wget")
	cmdf("mkdir -p /lib/modules")

	return
}

func ContainerKernels(d container.Image, kcfg *config.KernelConfig) (err error) {
	err = errors.New("TODO not implemented")
	return
}
