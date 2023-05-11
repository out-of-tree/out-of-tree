package main

import (
	"github.com/rs/zerolog/log"

	"code.dumpstack.io/tools/out-of-tree/distro/debian"
	"code.dumpstack.io/tools/out-of-tree/distro/debian/snapshot"
)

type DebianCmd struct {
	Cache DebianCacheCmd `cmd:"" help:"populate cache"`
}

type DebianCacheCmd struct {
	Path string `help:"path to cache" default:"debian.cache"`
}

func (cmd *DebianCacheCmd) Run() (err error) {
	c, err := debian.NewCache(cmd.Path)
	if err != nil {
		log.Error().Err(err).Msg("cache")
		return
	}
	defer c.Close()

	versions, err := snapshot.SourcePackageVersions("linux")
	if err != nil {
		log.Error().Err(err).Msg("get source package versions")
		return
	}

	for i, version := range versions {
		slog := log.With().Str("version", version).Logger()
		slog.Info().Msgf("%03d/%03d", i, len(versions))

		_, err = c.Get(version)
		if err == nil {
			slog.Info().Msgf("found in cache")
			continue
		}

		var dk debian.DebianKernel
		dk, err = debian.GetDebianKernel(version)
		if err == debian.ErrNoBinaryPackages {
			slog.Warn().Err(err).Msg("")
		} else if err != nil {
			slog.Error().Err(err).Msg("get debian kernel")
			continue
		}

		err = c.Put(dk)
		if err != nil {
			slog.Error().Err(err).Msg("put to cache")
			return
		}

		slog.Info().Msgf("%s cached", version)
	}

	return
}
