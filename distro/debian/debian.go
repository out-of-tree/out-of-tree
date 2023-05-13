package debian

import (
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"

	"code.dumpstack.io/tools/out-of-tree/cache"
	"code.dumpstack.io/tools/out-of-tree/config"
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
	"Buzz",
	"Hamm",
	"Woody",
	"Etch",
	"Lenny",
	"Squeeze",
	"Wheezy",
	"Jessie",
	"Stretch",
	"Buster",
	"Bullseye",
	"Bookworm",
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

	if major < 3 {
		err = errors.New("not supported")
		return
	} else if major <= 3 && minor < 16 {
		r = Wheezy // 3.2
	} else if major <= 4 && minor < 9 {
		r = Jessie // 3.16
	} else if major <= 4 && minor < 19 {
		r = Stretch // 4.9
	} else if major <= 5 && minor < 10 {
		r = Buster // 4.19
	} else {
		r = Bullseye // 5.10
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

		err = cache.DownloadDebianCache(CachePath)
		if err != nil {
			log.Debug().Err(err).Msg("No remote cache, will take some time")
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
