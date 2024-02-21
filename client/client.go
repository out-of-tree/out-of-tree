package client

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"sync"

	"github.com/davecgh/go-spew/spew"
	"github.com/rs/zerolog/log"

	"code.dumpstack.io/tools/out-of-tree/api"
	"code.dumpstack.io/tools/out-of-tree/config/dotfiles"
	"code.dumpstack.io/tools/out-of-tree/distro"
	"code.dumpstack.io/tools/out-of-tree/fs"
	"code.dumpstack.io/tools/out-of-tree/qemu"
)

type Client struct {
	RemoteAddr string
}

func (c Client) client() *tls.Conn {
	if !fs.PathExists(dotfiles.File("daemon/cert.pem")) {
		log.Fatal().Msgf("no {cert,key}.pem at %s",
			dotfiles.Dir("daemon"))
	}

	cert, err := tls.LoadX509KeyPair(
		dotfiles.File("daemon/cert.pem"),
		dotfiles.File("daemon/key.pem"))
	if err != nil {
		log.Fatal().Err(err).Msg("")
	}

	cacert, err := os.ReadFile(dotfiles.File("daemon/cert.pem"))
	if err != nil {
		log.Fatal().Err(err).Msg("")
	}
	certpool := x509.NewCertPool()
	certpool.AppendCertsFromPEM(cacert)

	tlscfg := &tls.Config{
		RootCAs:      certpool,
		Certificates: []tls.Certificate{cert},
	}

	conn, err := tls.Dial("tcp", c.RemoteAddr, tlscfg)
	if err != nil {
		log.Fatal().Err(err).Msg("")
	}

	return conn // conn.Close()
}

func (c Client) request(cmd api.Command, data any) (resp api.Resp, err error) {
	req := api.Req{Command: cmd}
	if data != nil {
		req.SetData(data)
	}

	conn := c.client()
	defer conn.Close()

	req.Encode(conn)

	err = resp.Decode(conn)
	if err != nil {
		log.Fatal().Err(err).Msgf("request %v", req)
	}

	log.Debug().Msgf("resp: %v", resp)

	if resp.Error != "" {
		err = errors.New(resp.Error)
		log.Fatal().Err(err).Msg("")
	}

	return
}

func (c Client) Jobs() (jobs []api.Job, err error) {
	resp, _ := c.request(api.ListJobs, nil)

	err = resp.GetData(&jobs)
	if err != nil {
		log.Error().Err(err).Msg("")
	}

	return
}

func (c Client) AddJob(job api.Job) (uuid string, err error) {
	resp, err := c.request(api.AddJob, &job)
	if err != nil {
		return
	}

	err = resp.GetData(&uuid)
	return
}

func (c Client) Repos() (repos []api.Repo, err error) {
	resp, _ := c.request(api.ListRepos, nil)

	log.Debug().Msgf("resp: %v", spew.Sdump(resp))

	err = resp.GetData(&repos)
	if err != nil {
		log.Error().Err(err).Msg("")
	}

	return
}

type logWriter struct {
	tag string
}

func (lw logWriter) Write(p []byte) (n int, err error) {
	n = len(p)
	log.Trace().Str("tag", lw.tag).Msgf("%v", strconv.Quote(string(p)))
	return
}

func (c Client) handler(cConn net.Conn) {
	defer cConn.Close()

	dConn := c.client()
	defer dConn.Close()

	req := api.Req{Command: api.RawMode}
	req.Encode(dConn)

	go io.Copy(cConn, io.TeeReader(dConn, logWriter{"recv"}))
	io.Copy(dConn, io.TeeReader(cConn, logWriter{"send"}))
}

var ErrRepoNotFound = errors.New("repo not found")

// GetRepo virtual API call
func (c Client) GetRepo(name string) (repo api.Repo, err error) {
	// TODO add API call

	repos, err := c.Repos()
	if err != nil {
		return
	}

	for _, r := range repos {
		if r.Name == name {
			repo = r
			return
		}
	}

	err = ErrRepoNotFound
	return
}

func (c Client) GitProxy(addr string, ready *sync.Mutex) {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal().Err(err).Msg("git proxy listen")
	}
	defer l.Close()

	log.Debug().Msgf("git proxy listen on %v", addr)

	for {
		ready.Unlock()
		conn, err := l.Accept()
		if err != nil {
			log.Fatal().Err(err).Msg("accept")
		}
		log.Debug().Msgf("git proxy accept %s", conn.RemoteAddr())

		go c.handler(conn)
	}
}

func (c Client) PushRepo(repo api.Repo) (err error) {
	addr := qemu.GetFreeAddrPort()

	ready := &sync.Mutex{}

	ready.Lock()
	go c.GitProxy(addr, ready)

	ready.Lock()

	remote := fmt.Sprintf("git://%s/%s", addr, repo.Name)
	log.Debug().Msgf("git proxy remote: %v", remote)

	raw, err := exec.Command("git", "--work-tree", repo.Path, "push", "--force", remote).
		CombinedOutput()
	if err != nil {
		return
	}

	log.Info().Msgf("push repo %v\n%v", repo, string(raw))
	return
}

func (c Client) AddRepo(repo api.Repo) (err error) {
	_, err = c.request(api.AddRepo, &repo)
	if err != nil {
		return
	}

	log.Info().Msgf("add repo %v", repo)
	return
}

func (c Client) Kernels() (kernels []distro.KernelInfo, err error) {
	resp, err := c.request(api.Kernels, nil)
	if err != nil {
		return
	}

	err = resp.GetData(&kernels)
	if err != nil {
		log.Error().Err(err).Msg("")
	}

	log.Info().Msgf("got %d kernels", len(kernels))
	return
}

func (c Client) JobStatus(uuid string) (st api.Status, err error) {
	resp, err := c.request(api.JobStatus, &uuid)
	if err != nil {
		return
	}

	err = resp.GetData(&st)
	if err != nil {
		log.Error().Err(err).Msg("")
	}

	return
}

func (c Client) JobLogs(uuid string) (logs []api.JobLog, err error) {
	resp, err := c.request(api.JobLogs, &uuid)
	if err != nil {
		return
	}

	err = resp.GetData(&logs)
	if err != nil {
		log.Error().Err(err).Msg("")
	}

	return
}
