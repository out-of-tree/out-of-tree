package daemon

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"code.dumpstack.io/tools/out-of-tree/api"
	"code.dumpstack.io/tools/out-of-tree/artifact"
	"code.dumpstack.io/tools/out-of-tree/config/dotfiles"
	"code.dumpstack.io/tools/out-of-tree/daemon/db"
	"code.dumpstack.io/tools/out-of-tree/distro"
	"code.dumpstack.io/tools/out-of-tree/qemu"
)

type jobProcessor struct {
	job api.Job
	log zerolog.Logger
	db  *sql.DB
}

func newJobProcessor(job api.Job, db *sql.DB) (pj jobProcessor) {
	pj.job = job
	pj.db = db
	pj.log = log.With().
		Str("uuid", job.UUID).
		Str("group", job.Group).
		Logger()
	return
}

func (pj jobProcessor) Update() (err error) {
	err = db.UpdateJob(pj.db, &pj.job)
	if err != nil {
		pj.log.Error().Err(err).Msgf("update job %v", pj.job)
	}
	return
}

func (pj jobProcessor) SetStatus(status api.Status) (err error) {
	pj.log.Info().Msgf(`%v -> %v`, pj.job.Status, status)
	pj.job.Status = status
	err = pj.Update()
	return
}

func (pj *jobProcessor) Process(res *Resources) (err error) {
	if pj.job.Status != api.StatusWaiting {
		err = errors.New("job is not available to process")
		return
	}

	if pj.job.Artifact.Qemu.Cpus == 0 {
		pj.job.Artifact.Qemu.Cpus = qemu.DefaultCPUs
	}

	if pj.job.Artifact.Qemu.Memory == 0 {
		pj.job.Artifact.Qemu.Memory = qemu.DefaultMemory
	}

	err = res.Allocate(pj.job)
	if err != nil {
		return
	}

	defer func() {
		res.Release(pj.job)
	}()

	log.Info().Msgf("process job %v", pj.job.UUID)

	pj.SetStatus(api.StatusRunning)
	pj.job.Started = time.Now()

	defer func() {
		pj.job.Finished = time.Now()
		if err != nil {
			pj.SetStatus(api.StatusFailure)
		} else {
			pj.SetStatus(api.StatusSuccess)
		}
	}()

	var tmp string
	tmp, err = os.MkdirTemp(dotfiles.Dir("tmp"), "")
	if err != nil {
		pj.log.Error().Err(err).Msg("mktemp")
		return
	}
	defer os.RemoveAll(tmp)

	tmprepo := filepath.Join(tmp, "repo")

	pj.log.Debug().Msgf("temp repo: %v", tmprepo)

	remote := fmt.Sprintf("git://localhost:9418/%s", pj.job.RepoName)

	pj.log.Debug().Msgf("remote: %v", remote)

	var raw []byte

	cmd := exec.Command("git", "clone", remote, tmprepo)

	raw, err = cmd.CombinedOutput()
	pj.log.Trace().Msgf("%v\n%v", cmd, string(raw))
	if err != nil {
		pj.log.Error().Msgf("%v\n%v", cmd, string(raw))
		return
	}

	cmd = exec.Command("git", "checkout", pj.job.Commit)

	cmd.Dir = tmprepo

	raw, err = cmd.CombinedOutput()
	pj.log.Trace().Msgf("%v\n%v", cmd, string(raw))
	if err != nil {
		pj.log.Error().Msgf("%v\n%v", cmd, string(raw))
		return
	}

	pj.job.Artifact.SourcePath = tmprepo

	var result *artifact.Result
	var dq *qemu.System

	pj.job.Artifact.Process(pj.log, pj.job.Target, false, false, "", "", 0,
		func(q *qemu.System, ka artifact.Artifact, ki distro.KernelInfo,
			res *artifact.Result) {

			result = res
			dq = q
		},
	)

	logdir := dotfiles.Dir("daemon/logs", pj.job.UUID)

	err = os.WriteFile(filepath.Join(logdir, "build.log"),
		[]byte(result.Build.Output), 0644)
	if err != nil {
		pj.log.Error().Err(err).Msg("")
	}

	err = os.WriteFile(filepath.Join(logdir, "run.log"),
		[]byte(result.Run.Output), 0644)
	if err != nil {
		pj.log.Error().Err(err).Msg("")
	}

	err = os.WriteFile(filepath.Join(logdir, "test.log"),
		[]byte(result.Test.Output), 0644)
	if err != nil {
		pj.log.Error().Err(err).Msg("")
	}

	err = os.WriteFile(filepath.Join(logdir, "qemu.log"),
		[]byte(dq.Stdout), 0644)
	if err != nil {
		pj.log.Error().Err(err).Msg("")
	}

	pj.log.Info().Msgf("build %v, run %v, test %v",
		result.Build.Ok, result.Run.Ok, result.Test.Ok)

	if !result.Test.Ok {
		err = errors.New("tests failed")
	}

	return
}
