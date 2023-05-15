package main

import (
	"os"
	"path/filepath"
	"regexp"

	"github.com/cavaliergopher/grab/v3"
	"github.com/rs/zerolog/log"

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
}

func (cmd DebianGetDebCmd) Run() (err error) {
	kernels, err := debian.GetKernels()
	if err != nil {
		log.Error().Err(err).Msg("")
		return
	}

	re := regexp.MustCompile(cmd.Regexp)

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

	for _, pkg := range packages {
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
	}

	return
}
