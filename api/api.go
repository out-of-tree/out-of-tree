package api

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	"net"
	"reflect"
	"time"

	"code.dumpstack.io/tools/out-of-tree/artifact"
	"code.dumpstack.io/tools/out-of-tree/distro"

	"github.com/google/uuid"
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

	UpdatedAt time.Time

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

	UpdatedAfter int64
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

func (r *Req) SetData(data any) (err error) {
	r.Type = fmt.Sprintf("%v", reflect.TypeOf(data))
	var buf bytes.Buffer
	err = gob.NewEncoder(&buf).Encode(data)
	r.Data = buf.Bytes()
	return
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

	buf := bytes.NewBuffer(r.Data)
	return gob.NewDecoder(buf).Decode(data)
}

func (r *Req) Encode(conn net.Conn) (err error) {
	return gob.NewEncoder(conn).Encode(r)
}

func (r *Req) Decode(conn net.Conn) (err error) {
	return gob.NewDecoder(conn).Decode(r)
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

func (r *Resp) SetData(data any) (err error) {
	r.Type = fmt.Sprintf("%v", reflect.TypeOf(data))
	var buf bytes.Buffer
	err = gob.NewEncoder(&buf).Encode(data)
	r.Data = buf.Bytes()
	return
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

	buf := bytes.NewBuffer(r.Data)
	return gob.NewDecoder(buf).Decode(data)
}

func (r *Resp) Encode(conn net.Conn) (err error) {
	if r.Err != nil && r.Err != ErrInvalid && r.Error == "" {
		r.Error = fmt.Sprintf("%v", r.Err)
	}
	return gob.NewEncoder(conn).Encode(r)
}

func (r *Resp) Decode(conn net.Conn) (err error) {
	err = gob.NewDecoder(conn).Decode(r)
	r.Err = ErrInvalid
	return
}
