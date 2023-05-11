package mr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/rs/zerolog/log"
	"golang.org/x/time/rate"
)

const apiURL = "https://snapshot.debian.org/mr"

var Limiter = rate.NewLimiter(rate.Every(time.Second), 10)

// Retries in case of 5xx errors
var Retries = 10

// https://salsa.debian.org/snapshot-team/snapshot/blob/master/API

// /mr/package/<package>/
type Package struct {
	Comment string `json:"_comment"`
	Package string `json:"package"`
	Result  []struct {
		Version string `json:"version"`
	} `json:"result"`
}

// /mr/package/<package>/<version>/binpackages
type Binpackages struct {
	Comment string `json:"_comment"`
	Package string `json:"package"`
	Result  []struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"result"`
	Version string `json:"version"`
}

// /mr/binary/<binary>/
type Binary struct {
	Comment string `json:"_comment"`
	Binary  string `json:"binary"`
	Result  []struct {
		BinaryVersion string `json:"binary_version"`
		Name          string `json:"name"`
		Source        string `json:"source"`
		Version       string `json:"version"`
	} `json:"result"`
}

// /mr/binary/<binpkg>/<binversion>/binfiles
type Binfiles struct {
	Comment       string `json:"_comment"`
	Binary        string `json:"binary"`
	BinaryVersion string `json:"binary_version"`
	Result        []struct {
		Architecture string `json:"architecture"`
		Hash         string `json:"hash"`
	} `json:"result"`
}

type Fileinfo struct {
	ArchiveName string `json:"archive_name"`
	FirstSeen   string `json:"first_seen"`
	Name        string `json:"name"`
	Path        string `json:"path"`
	Size        int    `json:"size"`
}

// /mr/file/<hash>/info
type Info struct {
	Comment string     `json:"_comment"`
	Hash    string     `json:"hash"`
	Result  []Fileinfo `json:"result"`
}

func getJson(query string, target interface{}) (err error) {
	flog := log.With().Str("url", query).Logger()

	var resp *http.Response
	for i := Retries; i > 0; i-- {
		flog.Trace().Msg("wait")
		Limiter.Wait(context.Background())

		flog.Trace().Msg("start")
		resp, err = http.Get(query)
		if err != nil {
			flog.Error().Err(err).Msg("")
			return
		}
		defer resp.Body.Close()
		flog.Debug().Msgf("%s", resp.Status)

		if resp.StatusCode < 500 {
			break
		}

		flog.Debug().Msgf("retry (%d left)", i)
	}

	if resp.StatusCode >= 400 {
		err = fmt.Errorf("%d (%s)", resp.StatusCode, query)
	}
	return json.NewDecoder(resp.Body).Decode(target)
}

func GetPackage(name string) (pkg Package, err error) {
	query := fmt.Sprintf("%s/package/%s/", apiURL,
		url.QueryEscape(name),
	)
	err = getJson(query, &pkg)
	return
}

func GetBinpackages(name, version string) (binpkgs Binpackages, err error) {
	query := fmt.Sprintf("%s/package/%s/%s/binpackages", apiURL,
		url.QueryEscape(name),
		url.QueryEscape(version),
	)
	err = getJson(query, &binpkgs)
	return
}

func GetBinary(pkg string) (binary Binary, err error) {
	query := fmt.Sprintf("%s/binary/%s/", apiURL,
		url.QueryEscape(pkg),
	)
	err = getJson(query, &binary)
	return
}

func GetBinfiles(binpkg, binversion string) (binfiles Binfiles, err error) {
	query := fmt.Sprintf("%s/binary/%s/%s/binfiles", apiURL,
		url.QueryEscape(binpkg),
		url.QueryEscape(binversion),
	)
	err = getJson(query, &binfiles)
	return
}

func GetInfo(hash string) (info Info, err error) {
	query := fmt.Sprintf("%s/file/%s/info", apiURL,
		url.QueryEscape(hash),
	)

	err = getJson(query, &info)
	if err != nil {
		return
	}

	if len(info.Result) != 1 {
		err = errors.New("API mismatch")
	}
	return
}