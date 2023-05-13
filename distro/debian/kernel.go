package debian

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"code.dumpstack.io/tools/out-of-tree/distro/debian/snapshot"
)

type DebianKernelVersion struct {
	// linux-headers-4.17.0-2-amd64_4.17.14-1_amd64.deb

	// Package version, e.g. "4.17.14-1"
	// See tags in https://salsa.debian.org/kernel-team/linux
	Package string

	// ABI version, e.g. "4.17.0-2"
	ABI string
}

type DebianKernel struct {
	Version DebianKernelVersion
	Image   snapshot.Package
	Headers snapshot.Package

	// FIXME There is a better way
	Internal struct {
		Invalid   bool
		LastFetch time.Time
	}
}

var (
	ErrNoBinaryPackages = errors.New("no binary packages found")
	ErrNoHeadersPackage = errors.New("no headers package found")
	ErrNoImagePackage   = errors.New("no image package found")
)

func GetDebianKernel(version string) (dk DebianKernel, err error) {
	dk.Version.Package = version

	regex := `^linux-(image|headers)-[a-z+~0-9\.\-]*-(amd64|amd64-unsigned)$`

	filter := []string{
		"rt-amd64",
		"cloud-amd64",
		"all-amd64",
	}

	packages, err := snapshot.Packages("linux", version, "amd64",
		regex, filter)
	if err != nil {
		return
	}

	if len(packages) == 0 {
		err = ErrNoBinaryPackages
		return
	} else if len(packages) == 1 {
		if strings.Contains(packages[0].Name, "image") {
			err = ErrNoHeadersPackage
			return
		} else if strings.Contains(packages[0].Name, "headers") {
			err = ErrNoImagePackage
			return
		} else {
			err = fmt.Errorf("wtf? %v", packages[0].Name)
			return
		}
	} else if len(packages) > 2 {
		err = errors.New("more than two binary packages found")
		return
	}

	var imageFound, headersFound bool
	for _, p := range packages {
		if strings.Contains(p.Name, "image") {
			imageFound = true
			dk.Image = p
		} else if strings.Contains(p.Name, "headers") {
			headersFound = true
			dk.Headers = p
		}
	}

	if !imageFound {
		err = errors.New("image not found")
		return
	}

	if !headersFound {
		err = errors.New("headers not found")
		return
	}

	s := strings.Replace(dk.Headers.Name, "linux-headers-", "", -1)
	dk.Version.ABI = strings.Replace(s, "-amd64", "", -1)

	return
}

// GetCachedKernel by deb package name
func GetCachedKernel(deb string) (dk DebianKernel, err error) {
	c, err := NewCache(CachePath)
	if err != nil {
		log.Error().Err(err).Msg("cache")
		return
	}
	defer c.Close()

	versions, err := c.GetVersions()
	if err != nil {
		log.Error().Err(err).Msg("get source package versions from cache")
		return
	}

	for _, version := range versions {
		var tmpdk DebianKernel
		tmpdk, err = c.Get(version)
		if err != nil {
			continue
		}

		switch deb {
		case tmpdk.Image.Deb.Name, tmpdk.Headers.Deb.Name:
			dk = tmpdk
			return
		}
	}

	return
}

func GetKernels(c *Cache, refetchDays int) (kernels []DebianKernel, err error) {
	versions, err := snapshot.SourcePackageVersions("linux")
	if err != nil {
		log.Error().Err(err).Msg("get source package versions")
		return
	}

	err = c.PutVersions(versions)
	if err != nil {
		log.Error().Err(err).Msg("put source package versions to cache")
		return
	}

	for i, version := range versions {
		slog := log.With().Str("version", version).Logger()
		slog.Debug().Msgf("%03d/%03d", i, len(versions))

		var dk DebianKernel

		dk, err = c.Get(version)
		if err == nil && !dk.Internal.Invalid {
			slog.Debug().Msgf("found in cache")
			kernels = append(kernels, dk)
			continue
		}

		if dk.Internal.Invalid {
			refetch := dk.Internal.LastFetch.AddDate(0, 0, refetchDays)
			if refetch.After(time.Now()) {
				slog.Debug().Msgf("refetch at %v", refetchDays)
				continue
			}
		}

		dk, err = GetDebianKernel(version)
		if err != nil {
			if err == ErrNoBinaryPackages {
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

		slog.Debug().Msgf("%s cached", version)
	}

	return
}
