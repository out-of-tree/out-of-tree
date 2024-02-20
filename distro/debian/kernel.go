package debian

import (
	"errors"
	"math"
	"strings"
	"time"

	"github.com/Masterminds/semver"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"code.dumpstack.io/tools/out-of-tree/cache"
	"code.dumpstack.io/tools/out-of-tree/config/dotfiles"
	"code.dumpstack.io/tools/out-of-tree/distro/debian/snapshot"
	"code.dumpstack.io/tools/out-of-tree/distro/debian/snapshot/metasnap"
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

func ParseKernelVersion(pkg string) (dkv DebianKernelVersion, err error) {
	// -> 4.11.0-trunk-amd64_4.11-1~exp2_amd64.deb
	pkg = strings.Replace(pkg, "linux-image-", "", -1)

	// -> [4.11.0-trunk-amd64 4.11-1~exp2 amd64.deb]
	fields := strings.Split(pkg, "_")

	if len(fields) != 3 {
		err = errors.New("incorrect input format")
		return
	}

	// 4.11.0-trunk-amd64 -> 4.11.0-trunk
	// TODO other archs?
	dkv.ABI = strings.Split(fields[0], "-amd64")[0]
	if dkv.ABI == "" {
		err = errors.New("incorrect input format")
		return
	}

	dkv.Package = fields[1]
	if dkv.Package == "" {
		err = errors.New("incorrect input format")
		return
	}

	return
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

	Release Release
}

func (dk DebianKernel) HasDependency(pkgname string) bool {
	for _, deppkg := range dk.Dependencies {
		if strings.Contains(deppkg.Name, pkgname) {
			return true
		}
	}
	return false
}

func (dk DebianKernel) Packages() (pkgs []snapshot.Package) {
	pkgs = append(pkgs, dk.Image)
	pkgs = append(pkgs, dk.Headers...)
	pkgs = append(pkgs, dk.Dependencies...)
	return
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

func getDebianKernel(version string) (dk DebianKernel, err error) {
	flog := log.With().
		Str("version", version).
		Logger()

	dk.Version.Package = version

	regex := `^(linux-(image|headers)-[a-z+~0-9\.\-]*-(common|amd64|amd64-unsigned)|linux-kbuild-.*|linux-compiler-.*-x86)$`

	filter := []string{
		"rt-amd64",
		"cloud-amd64",
		"all-amd64",
		"dbg",
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

	dk.Release = getRelease(dk.Image)
	if dk.Release == None {
		flog.Warn().Msg("release not found")
	} else {
		flog.Debug().Msgf("release is %s", dk.Release.Name())
	}

	return
}

func getRelease(p snapshot.Package) Release {
	repos, err := metasnap.GetRepos(p.Repo.Archive, p.Name, p.Arch, p.Version)
	if err != nil {
		log.Debug().Err(err).Msg("metasnap")
		return None
	}

	for _, repo := range repos {
		for _, rel := range ReleaseStrings[1:] {
			switch repo.Suite {
			case rel, rel + "-backports",
				rel + "-updates",
				rel + "-proposed-updates":

				return ReleaseFromString(rel)
			}
		}
	}

	return None
}

// GetCachedKernel by deb package name
func getCachedKernel(deb string) (dk DebianKernel, err error) {
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
		var tmpdks []DebianKernel
		tmpdks, err = c.Get(version)
		if err != nil {
			continue
		}
		tmpdk := tmpdks[0]

		if deb == tmpdk.Image.Deb.Name {
			dk = tmpdk
			return
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

func updateKbuild(toolsVersions []string, dk *DebianKernel) {
	if !kver(dk.Version.Package).LessThan(kver("4.5-rc0")) {
		return
	}

	var deps []snapshot.Package
	for _, pkg := range dk.Dependencies {
		if strings.Contains(pkg.Name, "kbuild") {
			continue
		}
		deps = append(deps, pkg)
	}
	dk.Dependencies = deps

	kbuildpkg, err := findKbuild(toolsVersions, dk.Version.Package)
	if err != nil {
		dk.Internal.Invalid = true
		return
	}

	dk.Dependencies = append(dk.Dependencies, kbuildpkg)
}

func getKernelsByVersion(slog zerolog.Logger, c *Cache, toolsVersions []string,
	version string, mode GetKernelsMode) (kernels []DebianKernel,
	fromcache bool) {

	var dk DebianKernel
	dks, err := c.Get(version)
	if err == nil {
		dk = dks[0]
		if !dk.Internal.Invalid {
			// TODO refactor
			slog.Trace().Msgf("found in cache")
			if dk.Release == None && mode&UpdateRelease != 0 {
				slog.Debug().Msg("update release")
				dk.Release = getRelease(dk.Image)
				if dk.Release != None {
					slog.Debug().Msg("update cache")
					err = c.Put([]DebianKernel{dk})
					if err != nil {
						slog.Error().Err(err).Msg("")
						return
					}
				}
			}
			if mode&UpdateKbuild != 0 {
				slog.Debug().Msg("update kbuild")
				updateKbuild(toolsVersions, &dk)
				slog.Debug().Msg("update cache")
				err = c.Put([]DebianKernel{dk})
				if err != nil {
					slog.Error().Err(err).Msg("")
					return
				}
			}
			kernels = append(kernels, dk)
			fromcache = true
			return
		}
	}

	if dk.Internal.Invalid {
		refetch := dk.Internal.LastFetch.AddDate(0, 0, RefetchDays)
		if refetch.After(time.Now()) {
			slog.Trace().Msgf("refetch at %v", refetch)
			return
		}
	}

	dk, err = getDebianKernel(version)
	if err != nil {
		if err == ErrNoBinaryPackages {
			slog.Warn().Err(err).Msg("")
		} else {
			slog.Error().Err(err).Msg("get debian kernel")
		}

		dk.Internal.Invalid = true
	}

	if !dk.HasDependency("kbuild") {
		// Debian kernels prior to the 4.5 package
		// version did not have a kbuild built from
		// the linux source itself, but used the
		// linux-tools source package.
		if !kver(dk.Version.Package).LessThan(kver("4.5-rc0")) {
			dk.Internal.Invalid = true
		} else {
			updateKbuild(toolsVersions, &dk)
		}
	}

	dk.Internal.LastFetch = time.Now()

	if !dk.Internal.Invalid {
		kernels = append(kernels, dk)
	}

	err = c.Put([]DebianKernel{dk})
	if err != nil {
		slog.Error().Err(err).Msg("put to cache")
		return
	}

	slog.Debug().Msgf("%s cached", version)
	return
}

var (
	CachePath   string
	RefetchDays int = 14
)

type GetKernelsMode int

const (
	NoMode GetKernelsMode = iota
	UpdateRelease
	UpdateKbuild
)

// GetKernelsWithLimit is workaround for testing and building the
// first cache, which is heavily rate limited by snapshot.debian.org
func GetKernelsWithLimit(limit int, mode GetKernelsMode) (kernels []DebianKernel,
	err error) {

	if CachePath == "" {
		CachePath = dotfiles.File("debian.cache")
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

	toolsVersions, err := snapshot.SourcePackageVersions("linux-tools")
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
		slog := log.With().Str("version", version).Logger()
		slog.Trace().Msgf("%03d/%03d", i, len(versions))
		vkernels, fromcache := getKernelsByVersion(slog, c, toolsVersions, version, mode)
		kernels = append(kernels, vkernels...)
		if !fromcache {
			limit--
		}
		if limit <= 0 {
			return
		}
	}

	return
}

func GetKernels() (kernels []DebianKernel, err error) {
	return GetKernelsWithLimit(math.MaxInt32, NoMode)
}
