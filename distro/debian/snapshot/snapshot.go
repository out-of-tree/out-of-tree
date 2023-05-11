package snapshot

import (
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/rs/zerolog/log"
	"github.com/ulikunitz/xz"
	"golang.org/x/time/rate"

	"code.dumpstack.io/tools/out-of-tree/distro/debian/snapshot/mr"
)

const timeLayout = "20060102T150405Z"

const URL = "https://snapshot.debian.org"

var Limiter = rate.NewLimiter(rate.Every(time.Second), 1)

// Retries in case of 5xx errors
var Retries = 10

var HttpTimeout = time.Second * 5

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
		Snapshot      string
		SnapshotDists []string

		Archive string

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

	p.Repo.SnapshotDists, err = p.dists()
	if err != nil {
		return
	}

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

	err = errors.New("hash not found")
	return
}

// Because the snapshot date is when the package was first introduced,
// it will probably always be sid or experimental.
func (p Package) GetCodename() (dist string, err error) {
	for _, dist = range p.Repo.SnapshotDists {
		var distHasPackage bool
		distHasPackage, err = p.isDistHasPackage(dist)
		if err != nil {
			return
		}
		if distHasPackage {
			return
		}
	}

	err = errors.New("codename not found")
	return
}

func (p Package) dists() (dists []string, err error) {
	query, err := url.JoinPath(URL, "archive", p.Repo.Archive,
		p.Repo.Snapshot, "dists/")
	if err != nil {
		return
	}

	resp, err := httpGetWithRetry(query)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		err = fmt.Errorf("%d (%s)", resp.StatusCode, query)
		return
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return
	}

	doc.Find("table tr").Each(func(i int, s *goquery.Selection) {
		html, err := s.Html()
		if err != nil {
			return
		}
		if !strings.Contains(html, "<td>d</td") {
			return
		}

		s.Find("a").Each(func(i int, s *goquery.Selection) {
			if strings.Contains(s.Text(), "..") {
				return
			}

			dist := strings.Replace(s.Text(), "/", "", -1)
			dists = append(dists, dist)
		})
	})

	return
}

func (p Package) isDistHasPackage(dist string) (yes bool, err error) {
	var buf []byte
	for _, ext := range []string{"xz", "gz", "bz2"} {
		buf, err = p.distPackage(dist, ext)
		if err == nil {
			break
		}
	}
	if err != nil {
		return
	}

	yes = bytes.Contains(buf, []byte(p.Deb.Name))
	return
}

func (p Package) distPackage(dist string, ext string) (buf []byte, err error) {
	query, err := url.JoinPath(URL, "archive", p.Repo.Archive,
		p.Repo.Snapshot, "dists", dist,
		fmt.Sprintf("main/binary-%s/Packages.%s", p.Arch, ext),
	)
	if err != nil {
		return
	}

	resp, err := httpGetWithRetry(query)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	var reader io.Reader
	switch ext {
	case "xz":
		reader, err = xz.NewReader(resp.Body)
	case "gz":
		reader, err = gzip.NewReader(resp.Body)
	case "bz2":
		reader = bzip2.NewReader(resp.Body)
	default:
		err = fmt.Errorf("%s not supported", ext)
	}
	if err != nil {
		return
	}

	buf, err = io.ReadAll(reader)
	return
}

func httpGetWithRetry(query string) (resp *http.Response, err error) {
	flog := log.With().Str("url", query).Logger()
	for i := Retries; i > 0; i-- {
		flog.Trace().Msg("wait")
		Limiter.Wait(context.Background())

		client := http.Client{Timeout: HttpTimeout}

		flog.Trace().Msg("start")
		resp, err = client.Get(query)
		if err != nil {
			if os.IsTimeout(err) {
				flog.Debug().Msgf("timeout; retry (%d left)", i)
				continue
			}
			flog.Error().Err(err).Msg("")
			return
		}
		flog.Debug().Msgf("%s", resp.Status)

		if resp.StatusCode < 500 {
			break
		}

		resp.Body.Close()
		flog.Debug().Msgf("retry (%d left)", i)
	}

	if resp.StatusCode >= 400 {
		err = fmt.Errorf("%d (%s)", resp.StatusCode, query)
	}
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
