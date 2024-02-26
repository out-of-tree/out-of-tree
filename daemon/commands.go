package daemon

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/davecgh/go-spew/spew"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"code.dumpstack.io/tools/out-of-tree/api"
	"code.dumpstack.io/tools/out-of-tree/config"
	"code.dumpstack.io/tools/out-of-tree/config/dotfiles"
	"code.dumpstack.io/tools/out-of-tree/daemon/db"
)

type cmdenv struct {
	Conn net.Conn

	Log zerolog.Logger

	DB *sql.DB

	WG *sync.WaitGroup

	KernelConfig string
}

func command(req *api.Req, resp *api.Resp, e cmdenv) (err error) {
	e.Log.Trace().Msgf("%v", spew.Sdump(req))
	defer e.Log.Trace().Msgf("%v", spew.Sdump(resp))

	e.WG.Add(1)
	defer e.WG.Done()

	e.Log.Debug().Msgf("%v", req.Command)

	switch req.Command {
	case api.RawMode:
		err = rawMode(req, e)
	case api.AddJob:
		err = addJob(req, resp, e)
	case api.ListJobs:
		err = listJobs(req, resp, e)
	case api.AddRepo:
		err = addRepo(req, resp, e)
	case api.ListRepos:
		err = listRepos(resp, e)
	case api.Kernels:
		err = kernels(resp, e)
	case api.JobStatus:
		err = jobStatus(req, resp, e)
	case api.JobLogs:
		err = jobLogs(req, resp, e)
	default:
		err = errors.New("unknown command")
	}

	resp.Err = err
	return
}

type logWriter struct {
	log zerolog.Logger
}

func (lw logWriter) Write(p []byte) (n int, err error) {
	n = len(p)
	//lw.log.Trace().Msgf("%v", strconv.Quote(string(p)))
	return
}

func rawMode(req *api.Req, e cmdenv) (err error) {
	uuid := uuid.New().String()

	lwsend := logWriter{log.With().Str("uuid", uuid).Str("git", "send").Logger()}
	lwrecv := logWriter{log.With().Str("uuid", uuid).Str("git", "recv").Logger()}

	conn, err := net.Dial("tcp", ":9418")
	if err != nil {
		log.Error().Err(err).Msg("dial")
		return
	}

	go io.Copy(e.Conn, io.TeeReader(conn, lwrecv))
	io.Copy(conn, io.TeeReader(e.Conn, lwsend))

	return
}

func listJobs(req *api.Req, resp *api.Resp, e cmdenv) (err error) {
	var params api.ListJobsParams
	err = req.GetData(&params)
	if err != nil {
		return
	}

	jobs, err := db.Jobs(e.DB)
	if err != nil {
		return
	}

	var result []api.Job
	for _, j := range jobs {
		if params.Group != "" && j.Group != params.Group {
			continue
		}
		if params.Repo != "" && j.RepoName != params.Repo {
			continue
		}
		if params.Commit != "" && j.Commit != params.Commit {
			continue
		}
		if params.Status != "" && j.Status != params.Status {
			continue
		}

		result = append(result, j)
	}

	resp.SetData(&result)
	return
}

func addJob(req *api.Req, resp *api.Resp, e cmdenv) (err error) {
	var job api.Job
	err = req.GetData(&job)
	if err != nil {
		return
	}

	job.GenUUID()

	var repos []api.Repo
	repos, err = db.Repos(e.DB)
	if err != nil {
		return
	}

	var found bool
	for _, r := range repos {
		if job.RepoName == r.Name {
			found = true
		}
	}
	if !found {
		err = errors.New("repo does not exist")
		return
	}

	if job.RepoName == "" {
		err = errors.New("repo name cannot be empty")
		return
	}

	if job.Commit == "" {
		err = errors.New("invalid commit")
		return
	}

	err = db.AddJob(e.DB, &job)
	if err != nil {
		return
	}

	resp.SetData(&job.UUID)
	return
}

func listRepos(resp *api.Resp, e cmdenv) (err error) {
	repos, err := db.Repos(e.DB)

	if err != nil {
		e.Log.Error().Err(err).Msg("")
		return
	}

	for i := range repos {
		repos[i].Path = dotfiles.Dir("daemon/repos",
			repos[i].Name)
	}

	log.Trace().Msgf("%v", spew.Sdump(repos))
	resp.SetData(&repos)
	return
}

func addRepo(req *api.Req, resp *api.Resp, e cmdenv) (err error) {
	var repo api.Repo
	err = req.GetData(&repo)
	if err != nil {
		return
	}

	var repos []api.Repo
	repos, err = db.Repos(e.DB)
	if err != nil {
		return
	}

	for _, r := range repos {
		log.Debug().Msgf("%v, %v", r, repo.Name)
		if repo.Name == r.Name {
			err = fmt.Errorf("repo already exist")
			return
		}
	}

	cmd := exec.Command("git", "init", "--bare")

	cmd.Dir = dotfiles.Dir("daemon/repos", repo.Name)

	var out []byte
	out, err = cmd.Output()
	e.Log.Debug().Msgf("%v -> %v\n%v", cmd, err, string(out))
	if err != nil {
		return
	}

	err = db.AddRepo(e.DB, &repo)
	return
}

func kernels(resp *api.Resp, e cmdenv) (err error) {
	kcfg, err := config.ReadKernelConfig(e.KernelConfig)
	if err != nil {
		e.Log.Error().Err(err).Msg("read kernels config")
		return
	}

	e.Log.Info().Msgf("send back %d kernels", len(kcfg.Kernels))
	resp.SetData(&kcfg.Kernels)
	return
}

func jobLogs(req *api.Req, resp *api.Resp, e cmdenv) (err error) {
	var uuid string
	err = req.GetData(&uuid)
	if err != nil {
		return
	}

	logdir := filepath.Join(dotfiles.File("daemon/logs"), uuid)
	if _, err = os.Stat(logdir); err != nil {
		return
	}

	files, err := os.ReadDir(logdir)
	if err != nil {
		return
	}

	var logs []api.JobLog

	for _, f := range files {
		if f.IsDir() {
			continue
		}

		logfile := filepath.Join(logdir, f.Name())

		var buf []byte
		buf, err = os.ReadFile(logfile)
		if err != nil {
			return
		}

		logs = append(logs, api.JobLog{
			Name: f.Name(),
			Text: string(buf),
		})
	}

	resp.SetData(&logs)
	return
}

func jobStatus(req *api.Req, resp *api.Resp, e cmdenv) (err error) {
	var uuid string
	err = req.GetData(&uuid)
	if err != nil {
		return
	}

	st, err := db.JobStatus(e.DB, uuid)
	if err != nil {
		return
	}
	resp.SetData(&st)
	return
}
