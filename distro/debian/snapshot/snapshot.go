package snapshot

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/rs/zerolog/log"

	"code.dumpstack.io/tools/out-of-tree/distro/debian/snapshot/mr"
)

const timeLayout = "20060102T150405Z"

const URL = "https://snapshot.debian.org"

func SourcePackageVersions(name string) (versions []string, err error) {
	pkg, err := mr.GetPackage(name)
	if err != nil {
		return
	}

	for _, res := range pkg.Result {
		versions = append(versions, res.Version)
	}
	return
}

type Package struct {
	Name    string
	Source  string
	Version string
	Arch    string

	Deb struct {
		Name string
		Hash string
		URL  string
	}

	Repo struct {
		Snapshot string

		Archive string

		Component string
	}
}

func NewPackage(name, srcname, version string, archs []string) (
	p Package, err error) {

	p.Name = name
	p.Source = srcname
	p.Version = version

	p.Arch, p.Deb.Hash, err = p.getHash(archs)
	if err != nil {
		return
	}

	info, err := mr.GetInfo(p.Deb.Hash)
	if err != nil {
		return
	}

	p.Deb.Name = info.Result[0].Name

	p.Repo.Archive = info.Result[0].ArchiveName
	p.Repo.Snapshot = info.Result[0].FirstSeen

	p.Deb.URL, err = url.JoinPath(URL, "archive", p.Repo.Archive,
		p.Repo.Snapshot, info.Result[0].Path, p.Deb.Name)
	if err != nil {
		return
	}

	split := strings.Split(info.Result[0].Path, "/")
	if split[1] != "pool" || len(split) < 3 {
		err = fmt.Errorf("incorrect path: %s", info.Result[0].Path)
		return
	}
	p.Repo.Component = split[2]

	return
}

func (p Package) getHash(archs []string) (arch, hash string, err error) {
	binfiles, err := mr.GetBinfiles(p.Name, p.Version)
	if err != nil {
		return
	}

	for _, res := range binfiles.Result {
		for _, allowedArch := range archs {
			if res.Architecture == allowedArch {
				arch = res.Architecture
				hash = res.Hash
				return
			}
		}
	}

	err = errors.New("hash not found")
	return
}

func contains(pkgs []Package, pkg Package) bool {
	for _, p := range pkgs {
		if p.Name == pkg.Name {
			return true
		}
	}
	return false
}

func filtered(s string, filter []string) bool {
	for _, f := range filter {
		if strings.Contains(s, f) {
			return true
		}
	}
	return false
}

func Packages(srcname, version, regex string, archs, filter []string) (
	pkgs []Package, err error) {

	binpkgs, err := mr.GetBinpackages(srcname, version)
	if err == mr.ErrNotFound {
		err = nil
		return
	}
	if err != nil {
		return
	}

	r := regexp.MustCompile(regex)

	for _, res := range binpkgs.Result {
		if res.Version != version {
			continue
		}
		if !r.MatchString(res.Name) || filtered(res.Name, filter) {
			continue
		}

		log.Trace().Msgf("matched %v", res.Name)

		var pkg Package
		pkg, err = NewPackage(res.Name, srcname, version, archs)
		if err != nil {
			return
		}

		if contains(pkgs, pkg) {
			log.Trace().Msgf("%v already in slice O_o", pkg.Name)
			continue
		}

		log.Trace().Msgf("append %v", pkg.Name)
		pkgs = append(pkgs, pkg)
	}

	return
}
