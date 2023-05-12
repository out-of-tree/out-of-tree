package debian

import (
	"os/user"
	"regexp"
	"strings"

	"github.com/rs/zerolog/log"

	"code.dumpstack.io/tools/out-of-tree/config"
)

type CodeName int

const (
	Wheezy CodeName = iota
	Jessie
	Stretch
	Buster
	Bullseye
	Bookworm
)

var CodeNameStrings = [...]string{
	"Wheezy",
	"Jessie",
	"Stretch",
	"Buster",
	"Bullseye",
	"Bookworm",
}

func (cn CodeName) String() string {
	return CodeNameStrings[cn]
}

var (
	CachePath   string
	RefetchDays int = 7
)

func MatchImagePkg(km config.KernelMask) (pkgs []string, err error) {
	if CachePath == "" {
		var usr *user.User
		usr, err = user.Current()
		if err != nil {
			return
		}

		CachePath = usr.HomeDir + "/.out-of-tree/debian.cache"
		log.Debug().Msgf("Use default kernels cache path: %s", CachePath)
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

	r := regexp.MustCompile(km.ReleaseMask)

	for _, dk := range kernels {
		p := strings.Replace(dk.Image.Deb.Name, ".deb", "", -1)
		if r.MatchString(p) {
			pkgs = append(pkgs, p)
		}
	}

	return
}
