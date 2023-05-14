package debian

import (
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/Masterminds/semver"
	"github.com/rs/zerolog/log"

	"code.dumpstack.io/tools/out-of-tree/cache"
	"code.dumpstack.io/tools/out-of-tree/config"
	"code.dumpstack.io/tools/out-of-tree/distro/debian/snapshot"
	"code.dumpstack.io/tools/out-of-tree/fs"
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
	Version      DebianKernelVersion
	Image        snapshot.Package
	Headers      []snapshot.Package
	Dependencies []snapshot.Package

	// FIXME There is a better way
	Internal struct {
		Invalid   bool
		LastFetch time.Time
	}
}

func (dk DebianKernel) HasDependency(pkgname string) bool {
	for _, deppkg := range dk.Dependencies {
		if strings.Contains(deppkg.Name, pkgname) {
			return true
		}
	}
	return false
}

// use only for inline comparison
func kver(ver string) *semver.Version {
	ver = strings.Replace(ver, "~", "-", -1)
	ver = strings.Replace(ver, "+", "-", -1)
	return semver.MustParse(ver)
}

var (
	ErrNoBinaryPackages = errors.New("no binary packages found")
	ErrNoHeadersPackage = errors.New("no headers package found")
	ErrNoImagePackage   = errors.New("no image package found")
)

func GetDebianKernel(version string) (dk DebianKernel, err error) {
	dk.Version.Package = version

	regex := `^(linux-(image|headers)-[a-z+~0-9\.\-]*-(common|amd64|amd64-unsigned)|linux-kbuild-.*)$`

	filter := []string{
		"rt-amd64",
		"cloud-amd64",
		"all-amd64",
		"dbg",
		"exp",
	}

	packages, err := snapshot.Packages("linux", version, regex,
		[]string{"amd64", "all"}, filter)
	if err != nil {
		return
	}

	if len(packages) == 0 {
		err = ErrNoBinaryPackages
		return
	}

	var imageFound, headersFound bool
	for _, p := range packages {
		if strings.Contains(p.Name, "image") {
			imageFound = true
			dk.Image = p
		} else if strings.Contains(p.Name, "headers") {
			headersFound = true
			dk.Headers = append(dk.Headers, p)
		} else {
			dk.Dependencies = append(dk.Dependencies, p)
		}
	}

	if !imageFound {
		err = ErrNoImagePackage
		return
	}

	if !headersFound {
		err = ErrNoHeadersPackage
		return
	}

	s := strings.Replace(dk.Image.Name, "linux-image-", "", -1)
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

		if deb == tmpdk.Image.Deb.Name {
			dk = tmpdk
		}

		for _, h := range tmpdk.Headers {
			if deb == h.Deb.Name {
				dk = tmpdk
				return
			}
		}
	}

	return
}

func kbuildVersion(versions []string, kpkgver string) string {
	sort.Slice(versions, func(i, j int) bool {
		return kver(versions[i]).GreaterThan(kver(versions[j]))
	})

	for _, v := range versions {
		if v == kpkgver {
			return v
		}
	}

	ver := kver(kpkgver)

	// Not able to find the exact version, try similar
	for _, v := range versions {
		cver := kver(v)

		// It's certainly not fit for purpose if the major and
		// minor versions aren't the same

		if ver.Major() != cver.Major() {
			continue
		}

		if ver.Minor() != cver.Minor() {
			continue
		}

		// Use the first version that is newer than the kernel

		if ver.LessThan(cver) {
			continue
		}

		return v
	}

	return ""
}

func findKbuild(versions []string, kpkgver string) (
	pkg snapshot.Package, err error) {

	version := kbuildVersion(versions, kpkgver)
	if version == "" {
		err = errors.New("cannot find kbuild version")
		return
	}

	packages, err := snapshot.Packages("linux-tools", version,
		`^linux-kbuild`, []string{"amd64"}, []string{"dbg"})
	if err != nil {
		return
	}

	if len(packages) == 0 {
		err = errors.New("cannot find kbuild package")
	}

	pkg = packages[0]
	return
}

var (
	CachePath   string
	RefetchDays int = 7
)

func GetKernels() (kernels []DebianKernel, err error) {
	if CachePath == "" {
		CachePath = config.File("debian.cache")
		log.Debug().Msgf("Use default kernels cache path: %s", CachePath)

		if !fs.PathExists(CachePath) {
			log.Debug().Msgf("No cache, download")
			err = cache.DownloadDebianCache(CachePath)
			if err != nil {
				log.Debug().Err(err).Msg(
					"No remote cache, will take some time")
			}
		}
	} else {
		log.Debug().Msgf("Debian kernels cache path: %s", CachePath)
	}

	c, err := NewCache(CachePath)
	if err != nil {
		log.Error().Err(err).Msg("cache")
		return
	}
	defer c.Close()

	linuxToolsVersions, err := snapshot.SourcePackageVersions("linux-tools")
	if err != nil {
		log.Error().Err(err).Msg("get linux-tools source pkg versions")
		return
	}

	versions, err := snapshot.SourcePackageVersions("linux")
	if err != nil {
		log.Error().Err(err).Msg("get linux source package versions")
		return
	}

	err = c.PutVersions(versions)
	if err != nil {
		log.Error().Err(err).Msg("put source package versions to cache")
		return
	}

	for i, version := range versions {
		// TODO move this scope to function
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
			refetch := dk.Internal.LastFetch.AddDate(0, 0, RefetchDays)
			if refetch.After(time.Now()) {
				slog.Debug().Msgf("refetch at %v", RefetchDays)
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

		if !dk.HasDependency("kbuild") {
			if !kver(dk.Version.Package).LessThan(kver("4.5-rc0")) {
				dk.Internal.Invalid = true
				dk.Internal.LastFetch = time.Now()
			} else {
				// Debian kernels prior to the 4.5 package
				// version did not have a kbuild built from
				// the linux source itself, but used the
				// linux-tools source package.
				kbuildpkg, err := findKbuild(
					linuxToolsVersions,
					dk.Version.Package,
				)
				if err != nil {
					dk.Internal.Invalid = true
					dk.Internal.LastFetch = time.Now()
				} else {
					dk.Dependencies = append(
						dk.Dependencies,
						kbuildpkg,
					)
				}
			}
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
