package snapshot

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"code.dumpstack.io/tools/out-of-tree/distro/debian/snapshot/mr"
)

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
		Archive  string

		Component string
	}
}

func NewPackage(name, srcname, version, arch string) (p Package, err error) {
	p.Name = name
	p.Source = srcname
	p.Version = version
	p.Arch = arch

	p.Deb.Hash, err = p.getHash()
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

func (p Package) getHash() (hash string, err error) {
	binfiles, err := mr.GetBinfiles(p.Name, p.Version)
	if err != nil {
		return
	}

	for _, res := range binfiles.Result {
		if res.Architecture == p.Arch {
			hash = res.Hash
			return
		}
	}

	err = errors.New("not found")
	return
}

func Packages(srcname, version, arch, regex string) (pkgs []Package, err error) {
	binpkgs, err := mr.GetBinpackages(srcname, version)
	if err != nil {
		return
	}

	r := regexp.MustCompile(regex)

	for _, res := range binpkgs.Result {
		if !r.MatchString(res.Name) {
			continue
		}

		var pkg Package
		pkg, err = NewPackage(res.Name, srcname, version, arch)
		if err != nil {
			return
		}

		pkgs = append(pkgs, pkg)
	}

	return
}
