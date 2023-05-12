package main

import (
	"github.com/rs/zerolog/log"

	"code.dumpstack.io/tools/out-of-tree/distro/debian"
)

type DebianCmd struct {
	Cache DebianCacheCmd `cmd:"" help:"populate cache"`
}

type DebianCacheCmd struct {
	Path    string `help:"path to cache" default:"debian.cache"`
	Refetch int    `help:"days before refetch versions without deb package" default:"7"`
}

func (cmd *DebianCacheCmd) Run() (err error) {
	c, err := debian.NewCache(cmd.Path)
	if err != nil {
		log.Error().Err(err).Msg("cache")
		return
	}
	defer c.Close()

	log.Info().Msg("Fetching kernels...")

	_, err = debian.GetKernels(c, cmd.Refetch)
	if err != nil {
		log.Error().Err(err).Msg("")
		return
	}

	log.Info().Msg("Success")
	return
}
