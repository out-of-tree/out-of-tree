package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/cavaliergopher/grab/v3"
	"github.com/davecgh/go-spew/spew"
	"github.com/remeh/sizedwaitgroup"
	"github.com/rs/zerolog/log"

	"code.dumpstack.io/tools/out-of-tree/cache"
	"code.dumpstack.io/tools/out-of-tree/distro"
	"code.dumpstack.io/tools/out-of-tree/distro/debian"
	"code.dumpstack.io/tools/out-of-tree/distro/debian/snapshot"
	"code.dumpstack.io/tools/out-of-tree/fs"
)

type DistroCmd struct {
	List DistroListCmd `cmd:"" help:"list available distros"`

	Debian DebianCmd `cmd:"" hidden:""`
}

type DebianCmd struct {
	Cache DebianCacheCmd `cmd:"" help:"populate cache"`
	Fetch DebianFetchCmd `cmd:"" help:"download deb packages"`

	Regex string `help:"match deb pkg names by regex" default:".*"`
}

type DebianCacheCmd struct {
	Path    string `help:"path to cache"`
	Refetch int    `help:"days before refetch versions without deb package" default:"7"`
	Dump    bool   `help:"dump cache"`
}

func (cmd *DebianCacheCmd) Run(dcmd *DebianCmd) (err error) {
	if cmd.Path != "" {
		debian.CachePath = cmd.Path
	}
	debian.RefetchDays = cmd.Refetch

	log.Info().Msg("Fetching kernels...")

	kernels, err := debian.GetKernels()
	if err != nil {
		log.Error().Err(err).Msg("")
		return
	}

	if cmd.Dump {
		re, err := regexp.Compile(dcmd.Regex)
		if err != nil {
			log.Fatal().Err(err).Msg("regex")
		}

		for _, kernel := range kernels {
			if !re.MatchString(kernel.Image.Deb.Name) {
				continue
			}
			fmt.Println(spew.Sdump(kernel))
		}
	}

	log.Info().Msg("Success")
	return
}

type DebianFetchCmd struct {
	Path         string `help:"path to download directory" type:"existingdir" default:"./"`
	IgnoreMirror bool   `help:"ignore check if packages on the mirror"`

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
		found, _ := cache.PackageURL(distro.Debian, pkg.Deb.URL)
		if found {
			flog.Debug().Msg("found on the mirror")
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

func (cmd *DebianFetchCmd) Run(dcmd *DebianCmd) (err error) {
	re, err := regexp.Compile(dcmd.Regex)
	if err != nil {
		log.Fatal().Err(err).Msg("regex")
	}

	log.Info().Msg("will not download packages that exist on the mirror")
	log.Info().Msg("use --ignore-mirror if you really need it")

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

type DistroListCmd struct{}

func (cmd *DistroListCmd) Run() (err error) {
	for _, d := range distro.List() {
		fmt.Println(d.ID, strings.Title(d.Release))
	}
	return
}
