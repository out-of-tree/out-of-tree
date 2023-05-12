package main

import (
	"time"

	"github.com/rs/zerolog/log"

	"code.dumpstack.io/tools/out-of-tree/distro/debian"
	"code.dumpstack.io/tools/out-of-tree/distro/debian/snapshot"
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

	versions, err := snapshot.SourcePackageVersions("linux")
	if err != nil {
		log.Error().Err(err).Msg("get source package versions")
		return
	}

	for i, version := range versions {
		slog := log.With().Str("version", version).Logger()
		slog.Info().Msgf("%03d/%03d", i, len(versions))

		var dk debian.DebianKernel

		dk, err = c.Get(version)
		if err == nil && !dk.Internal.Invalid {
			slog.Info().Msgf("found in cache")
			continue
		}

		if dk.Internal.Invalid {
			refetch := dk.Internal.LastFetch.AddDate(0, 0, cmd.Refetch)
			if refetch.After(time.Now()) {
				slog.Info().Msgf("refetch at %v", refetch)
				continue
			}
		}

		dk, err = debian.GetDebianKernel(version)
		if err != nil {
			if err == debian.ErrNoBinaryPackages {
				slog.Warn().Err(err).Msg("")
			} else {
				slog.Error().Err(err).Msg("get debian kernel")
			}

			dk.Internal.Invalid = true
			dk.Internal.LastFetch = time.Now()
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
