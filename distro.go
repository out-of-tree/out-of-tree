package main

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/cavaliergopher/grab/v3"
	"github.com/remeh/sizedwaitgroup"
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
	Cache DebianCacheCmd `cmd:"" help:"populate cache"`
	Fetch DebianFetchCmd `cmd:"" help:"download deb packages"`
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

type DebianFetchCmd struct {
	Path   string `help:"path to download directory" type:"existingdir" default:"./"`
	Regexp string `help:"match deb pkg names by regexp" default:".*"`

	IgnoreMirror bool `help:"ignore check if packages on the mirror"`

	Max int `help:"do not download more than X" default:"100500"`

	Threads int `help:"parallel download threads" default:"8"`

	Timeout time.Duration `help:"timeout for each download" default:"1m"`

	swg        sizedwaitgroup.SizedWaitGroup
	hasResults bool
}

func (cmd *DebianFetchCmd) fetch(pkg snapshot.Package) {
	flog := log.With().
		Str("pkg", pkg.Deb.Name).
		Logger()

	defer cmd.swg.Done()

	if !cmd.IgnoreMirror {
		flog.Debug().Msg("check mirror")
		found, _ := cache.PackageURL(config.Debian, pkg.Deb.URL)
		if found {
			flog.Info().Msg("found on the mirror")
			return
		}
	}

	target := filepath.Join(cmd.Path, filepath.Base(pkg.Deb.URL))

	if fs.PathExists(target) {
		flog.Debug().Msg("already exists")
		return
	}

	tmp, err := os.MkdirTemp(cmd.Path, "tmp-")
	if err != nil {
		flog.Fatal().Err(err).Msg("mkdir")
		return
	}
	defer os.RemoveAll(tmp)

	flog.Info().Msg("fetch")
	flog.Debug().Msg(pkg.Deb.URL)

	ctx, cancel := context.WithTimeout(context.Background(), cmd.Timeout)
	defer cancel()

	req, err := grab.NewRequest(tmp, pkg.Deb.URL)
	if err != nil {
		flog.Warn().Err(err).Msg("cannot create request")
		return
	}
	req = req.WithContext(ctx)

	resp := grab.DefaultClient.Do(req)
	if err := resp.Err(); err != nil {
		flog.Warn().Err(err).Msg("request cancelled")
		return
	}

	err = os.Rename(resp.Filename, target)
	if err != nil {
		flog.Fatal().Err(err).Msg("mv")
	}

	cmd.hasResults = true
	cmd.Max--
}

func (cmd *DebianFetchCmd) Run() (err error) {
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

	cmd.swg = sizedwaitgroup.New(cmd.Threads)
	for _, pkg := range packages {
		if cmd.Max <= 0 {
			break
		}

		cmd.swg.Add()
		go cmd.fetch(pkg)
	}
	cmd.swg.Wait()

	if !cmd.hasResults {
		log.Fatal().Msg("no packages found to download")
	}
	return
}
