package metasnap

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"golang.org/x/time/rate"
)

// Note: Metasnap does not have all the packages, and its API is
// rather buggy.

const apiURL = "http://metasnap.debian.net/cgi-bin/api?"

var (
	limiterTimeout time.Duration = time.Second
	limiterBurst   int           = 3

	Limiter = rate.NewLimiter(rate.Every(limiterTimeout), limiterBurst)
)

func lowerLimit() {
	limiterTimeout = limiterTimeout * 2
	log.Info().Msgf("limiter timeout set to %v", limiterTimeout)
	Limiter.SetLimit(rate.Every(limiterTimeout))
}

// Retries in case of 5xx errors
var Retries = 10

var ErrNotFound = errors.New("404 not found")

func query(q string) (result string, err error) {
	flog := log.With().Str("url", q).Logger()

	var resp *http.Response
	for i := Retries; i > 0; i-- {
		flog.Trace().Msg("wait")
		Limiter.Wait(context.Background())

		flog.Trace().Msg("start")
		resp, err = http.Get(q)
		if err != nil {
			if strings.Contains(err.Error(), "reset by peer") {
				flog.Debug().Err(err).Msg("")
				lowerLimit()
				continue
			}
			flog.Error().Err(err).Msg("")
			return
		}
		defer resp.Body.Close()

		flog.Debug().Msgf("%s", resp.Status)

		if resp.StatusCode == 404 {
			err = ErrNotFound
			return
		}

		if resp.StatusCode < 500 {
			break
		}

		flog.Debug().Msgf("retry (%d left)", i)
	}

	if resp.StatusCode >= 400 {
		err = fmt.Errorf("%d (%s)", resp.StatusCode, q)
	}

	buf, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}

	result = string(buf)
	return
}

func queryAPIf(f string, s ...interface{}) (result string, err error) {
	return query(apiURL + fmt.Sprintf(f, s...))
}

type Snapshot struct {
	First string
	Last  string
}

type Repo struct {
	Archive   string
	Suite     string
	Component string
	Snapshot  Snapshot
}

func GetRepo(archive, pkg, arch, ver string) (repos []Repo, err error) {
	result, err := queryAPIf("archive=%s&pkg=%s&arch=%s&ver=%s",
		archive, pkg, arch, ver)

	if err != nil {
		return
	}

	if result == "" {
		err = ErrNotFound
		return
	}

	for _, line := range strings.Split(result, "\n") {
		if line == "" {
			break
		}

		fields := strings.Split(line, " ")
		if len(fields) != 4 {
			err = fmt.Errorf("metasnap api returned %s", result)
			return
		}

		repo := Repo{
			Archive:   archive,
			Suite:     fields[0],
			Component: fields[1],
			Snapshot: Snapshot{
				First: fields[2],
				Last:  fields[3],
			},
		}

		repos = append(repos, repo)
	}

	if len(repos) == 0 {
		err = ErrNotFound
		return
	}

	return
}