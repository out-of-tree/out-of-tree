package main

import (
	"github.com/rs/zerolog/log"

	"code.dumpstack.io/tools/out-of-tree/distro/debian"
)

type DistroCmd struct {
	Debian DebianCmd `cmd:""`
}

type DebianCmd struct {
	Cache DebianCacheCmd `cmd:"" help:"populate cache"`
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
