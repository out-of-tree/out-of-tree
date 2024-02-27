package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"reflect"
	"time"

	"code.dumpstack.io/tools/out-of-tree/artifact"
	"code.dumpstack.io/tools/out-of-tree/distro"

	"github.com/davecgh/go-spew/spew"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

var ErrInvalid = errors.New("")

type Status string

const (
	StatusNew     Status = "new"
	StatusWaiting Status = "waiting"
	StatusRunning Status = "running"
	StatusSuccess Status = "success"
	StatusFailure Status = "failure"
)

type Command string

const (
	RawMode Command = "rawmode"

	AddJob    Command = "add_job"
	ListJobs  Command = "list_jobs"
	JobLogs   Command = "job_logs"
	JobStatus Command = "job_status"

	AddRepo   Command = "add_repo"
	ListRepos Command = "list_repos"

	Kernels Command = "kernels"
)

type Job struct {
	ID int64

	// Job UUID
	UUID string
	// Group UUID
	Group string

	RepoName string
	Commit   string

	Artifact artifact.Artifact
	Target   distro.KernelInfo

	Created  time.Time
	Started  time.Time
	Finished time.Time

	Status Status
}

func (job *Job) GenUUID() {
	job.UUID = uuid.New().String()
}

// ListJobsParams is the parameters for ListJobs command
type ListJobsParams struct {
	// Group UUID
	Group string

	// Repo name
	Repo string

	// Commit hash
	Commit string

	// Status of the job
	Status Status

	// Time range (unix timestamps)
	Time struct {
		After  int64
		Before int64
	}
}

type Repo struct {
	ID   int64
	Name string
	Path string
}

type JobLog struct {
	Name string
	Text string
}

type Req struct {
	Command Command

	Type string
	Data []byte
}

func (r *Req) SetData(data any) {
	r.Type = fmt.Sprintf("%v", reflect.TypeOf(data))
	r.Data = Marshal(data)
}

func (r *Req) GetData(data any) (err error) {
	if len(r.Data) == 0 {
		return
	}

	t := fmt.Sprintf("%v", reflect.TypeOf(data))
	if r.Type != t {
		err = fmt.Errorf("type mismatch (%v != %v)", r.Type, t)
		return
	}

	log.Trace().Msgf("unmarshal %v", string(r.Data))
	err = json.Unmarshal(r.Data, &data)
	return
}

func (r Req) Encode(conn net.Conn) {
	log.Debug().Msgf("encode %v", r.Command)
	err := json.NewEncoder(conn).Encode(&r)
	if err != nil {
		log.Fatal().Msgf("encode %v", r.Command)
	}
}

func (r *Req) Decode(conn net.Conn) (err error) {
	err = json.NewDecoder(conn).Decode(r)
	return
}

func (r Req) Marshal() (bytes []byte) {
	return Marshal(r)
}

func (Req) Unmarshal(data []byte) (r Req, err error) {
	err = json.Unmarshal(data, &r)
	log.Trace().Msgf("unmarshal %v", spew.Sdump(r))
	return
}

type Resp struct {
	UUID string

	Error string

	Err error `json:"-"`

	Type string
	Data []byte
}

func NewResp() (resp Resp) {
	resp.UUID = uuid.New().String()
	return
}

func (r *Resp) SetData(data any) {
	r.Type = fmt.Sprintf("%v", reflect.TypeOf(data))
	r.Data = Marshal(data)
}

func (r *Resp) GetData(data any) (err error) {
	if len(r.Data) == 0 {
		return
	}

	t := fmt.Sprintf("%v", reflect.TypeOf(data))
	if r.Type != t {
		err = fmt.Errorf("type mismatch (%v != %v)", r.Type, t)
		return
	}

	log.Trace().Msgf("unmarshal %v", string(r.Data))
	err = json.Unmarshal(r.Data, &data)
	return
}

func (r *Resp) Encode(conn net.Conn) {
	if r.Err != nil && r.Err != ErrInvalid && r.Error == "" {
		r.Error = fmt.Sprintf("%v", r.Err)
	}
	log.Debug().Msgf("encode %v", r.UUID)
	err := json.NewEncoder(conn).Encode(r)
	if err != nil {
		log.Fatal().Msgf("encode %v", r.UUID)
	}
}

func (r *Resp) Decode(conn net.Conn) (err error) {
	err = json.NewDecoder(conn).Decode(r)
	r.Err = ErrInvalid
	return
}

func (r *Resp) Marshal() (bytes []byte) {
	if r.Err != nil && r.Err != ErrInvalid && r.Error == "" {
		r.Error = fmt.Sprintf("%v", r.Err)
	}

	return Marshal(r)
}

func (Resp) Unmarshal(data []byte) (r Resp, err error) {
	err = json.Unmarshal(data, &r)
	log.Trace().Msgf("unmarshal %v", spew.Sdump(r))
	r.Err = ErrInvalid
	return
}

func Marshal(data any) (bytes []byte) {
	bytes, err := json.Marshal(data)
	if err != nil {
		log.Fatal().Err(err).Msgf("marshal %v", data)
	}
	log.Trace().Msgf("marshal %v", string(bytes))
	return
}
