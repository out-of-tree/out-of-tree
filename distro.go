package main

import (
	"os"
	"path/filepath"
	"regexp"

	"github.com/cavaliergopher/grab/v3"
	"github.com/rs/zerolog/log"

	"code.dumpstack.io/tools/out-of-tree/cache"
	"code.dumpstack.io/tools/out-of-tree/config"
	"code.dumpstack.io/tools/out-of-tree/distro/debian"
	"code.dumpstack.io/tools/out-of-tree/distro/debian/snapshot"
	"code.dumpstack.io/tools/out-of-tree/fs"
)

type DistroCmd struct {
	Debian DebianCmd `cmd:""`
}

type DebianCmd struct {
	Cache  DebianCacheCmd  `cmd:"" help:"populate cache"`
	GetDeb DebianGetDebCmd `cmd:"" help:"download deb packages"`
}

type DebianCacheCmd struct {
	Path    string `help:"path to cache"`
	Refetch int    `help:"days before refetch versions without deb package" default:"7"`
}

func (cmd *DebianCacheCmd) Run() (err error) {
	if cmd.Path != "" {
		debian.CachePath = cmd.Path
	}
	debian.RefetchDays = cmd.Refetch

	log.Info().Msg("Fetching kernels...")

	_, err = debian.GetKernels()
	if err != nil {
		log.Error().Err(err).Msg("")
		return
	}

	log.Info().Msg("Success")
	return
}

type DebianGetDebCmd struct {
	Path   string `help:"path to download directory" type:"existingdir" default:"./"`
	Regexp string `help:"match deb pkg names by regexp" default:".*"`

	IgnoreCached bool `help:"ignore packages found on remote mirror"`
}

func (cmd DebianGetDebCmd) Run() (err error) {
	re, err := regexp.Compile(cmd.Regexp)
	if err != nil {
		log.Fatal().Err(err).Msg("regexp")
	}

	kernels, err := debian.GetKernels()
	if err != nil {
		log.Error().Err(err).Msg("")
		return
	}

	var packages []snapshot.Package
	for _, kernel := range kernels {
		for _, pkg := range kernel.Packages() {
			if !re.MatchString(pkg.Deb.Name) {
				continue
			}

			packages = append(packages, pkg)
		}
	}

	tmp, err := os.MkdirTemp(cmd.Path, "tmp-")
	if err != nil {
		return
	}
	defer os.RemoveAll(tmp)

	hasresults := false

	for _, pkg := range packages {
		if cmd.IgnoreCached {
			log.Debug().Msgf("check cache for %s", pkg.Deb.Name)
			found, _ := cache.PackageURL(config.Debian, pkg.Deb.URL)
			if found {
				log.Debug().Msgf("%s already cached", pkg.Deb.Name)
				continue
			}
		}

		target := filepath.Join(cmd.Path, filepath.Base(pkg.Deb.URL))

		if fs.PathExists(target) {
			log.Info().Msgf("%s already exists", pkg.Deb.URL)
			continue
		}

		log.Info().Msgf("downloading %s", pkg.Deb.URL)

		resp, err := grab.Get(tmp, pkg.Deb.URL)
		if err != nil {
			log.Warn().Err(err).Msg("download")
			continue
		}

		err = os.Rename(resp.Filename, target)
		if err != nil {
			log.Fatal().Err(err).Msg("mv")
		}

		hasresults = true
	}

	if !hasresults {
		log.Fatal("no packages found to download")
	}
	return
}
