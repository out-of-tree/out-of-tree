package daemon

import (
	"crypto/tls"
	"database/sql"
	"io"
	"net"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"github.com/remeh/sizedwaitgroup"
	"github.com/rs/zerolog/log"

	"code.dumpstack.io/tools/out-of-tree/api"
	"code.dumpstack.io/tools/out-of-tree/config/dotfiles"
	"code.dumpstack.io/tools/out-of-tree/daemon/db"
	"code.dumpstack.io/tools/out-of-tree/fs"
)

type Daemon struct {
	Threads   int
	Resources *Resources

	db           *sql.DB
	kernelConfig string

	shutdown bool
	wg       sync.WaitGroup
}

func Init(kernelConfig string) (d *Daemon, err error) {
	d = &Daemon{}
	d.Threads = runtime.NumCPU()
	d.Resources = NewResources()

	d.kernelConfig = kernelConfig

	d.wg.Add(1) // matches with db.Close()
	d.db, err = db.OpenDatabase(dotfiles.File("daemon/daemon.db"))
	if err != nil {
		log.Error().Err(err).Msg("cannot open daemon.db")
	}

	log.Info().Msgf("database %s", dotfiles.File("daemon/daemon.db"))
	return
}

func (d *Daemon) Kill() {
	d.shutdown = true

	d.db.Close()
	d.wg.Done()
}

func (d *Daemon) Daemon() {
	if d.db == nil {
		log.Fatal().Msg("db is not initialized")
	}

	swg := sizedwaitgroup.New(d.Threads)
	log.Info().Int("threads", d.Threads).Msg("start")

	first := true

	for !d.shutdown {
		d.wg.Add(1)

		jobs, err := db.Jobs(d.db, "")
		if err != nil && !d.shutdown {
			log.Error().Err(err).Msg("")
			d.wg.Done()
			time.Sleep(time.Minute)
			continue
		}

		for _, job := range jobs {
			if d.shutdown {
				break
			}

			pj := newJobProcessor(job, d.db)

			if first && job.Status == api.StatusRunning {
				pj.SetStatus(api.StatusWaiting)
				continue
			}

			if job.Status == api.StatusNew {
				pj.SetStatus(api.StatusWaiting)
				continue
			}

			if job.Status != api.StatusWaiting {
				continue
			}

			swg.Add()
			go func(pj jobProcessor) {
				defer swg.Done()
				pj.Process(d.Resources)
				time.Sleep(time.Second)
			}(pj)
		}

		first = false

		d.wg.Done()
		time.Sleep(time.Second)
	}

	swg.Wait()
}

func handler(conn net.Conn, e cmdenv) {
	defer conn.Close()

	resp := api.NewResp()

	e.Log = log.With().
		Str("resp_uuid", resp.UUID).
		Str("remote_addr", conn.RemoteAddr().String()).
		Logger()

	e.Log.Info().Msg("")

	var req api.Req

	defer func() {
		if req.Command != api.RawMode {
			resp.Encode(conn)
		} else {
			log.Debug().Msg("raw mode, not encode response")
		}
	}()

	err := req.Decode(conn)
	if err != nil {
		e.Log.Error().Err(err).Msg("cannot decode")
		return
	}

	err = command(&req, &resp, e)
	if err != nil {
		e.Log.Error().Err(err).Msg("")
		return
	}
}

func (d *Daemon) Listen(addr string) {
	if d.db == nil {
		log.Fatal().Msg("db is not initialized")
	}

	go func() {
		repodir := dotfiles.Dir("daemon/repos")
		git := exec.Command("git", "daemon", "--port=9418", "--verbose",
			"--reuseaddr",
			"--export-all", "--base-path="+repodir,
			"--enable=receive-pack",
			"--enable=upload-pack",
			repodir)

		stdout, err := git.StdoutPipe()
		if err != nil {
			log.Fatal().Err(err).Msgf("%v", git)
			return
		}

		go io.Copy(logWriter{log: log.Logger}, stdout)

		stderr, err := git.StderrPipe()
		if err != nil {
			log.Fatal().Err(err).Msgf("%v", git)
			return
		}

		go io.Copy(logWriter{log: log.Logger}, stderr)

		log.Debug().Msgf("start %v", git)
		git.Start()
		defer func() {
			log.Debug().Msgf("stop %v", git)
		}()

		err = git.Wait()
		if err != nil {
			log.Fatal().Err(err).Msgf("%v", git)
			return
		}
	}()

	if !fs.PathExists(dotfiles.File("daemon/cert.pem")) {
		log.Info().Msg("No cert.pem, generating...")
		cmd := exec.Command("openssl",
			"req", "-batch", "-newkey", "rsa:2048",
			"-new", "-nodes", "-x509",
			"-subj", "/CN=*",
			"-addext", "subjectAltName = DNS:*",
			"-out", dotfiles.File("daemon/cert.pem"),
			"-keyout", dotfiles.File("daemon/key.pem"))

		out, err := cmd.Output()
		if err != nil {
			log.Error().Err(err).Msg(string(out))
			return
		}
	}

	log.Info().Msg("copy to client:")
	log.Info().Msgf("cert: %s, key: %s",
		dotfiles.File("daemon/cert.pem"),
		dotfiles.File("daemon/key.pem"))

	cert, err := tls.LoadX509KeyPair(dotfiles.File("daemon/cert.pem"),
		dotfiles.File("daemon/key.pem"))
	if err != nil {
		log.Fatal().Err(err).Msg("LoadX509KeyPair")
	}
	tlscfg := &tls.Config{Certificates: []tls.Certificate{cert}}

	l, err := tls.Listen("tcp", addr, tlscfg)
	if err != nil {
		log.Fatal().Err(err).Msg("listen")
	}

	log.Info().Str("addr", ":9418").Msg("git")
	log.Info().Str("addr", addr).Msg("daemon")

	for {
		conn, err := l.Accept()
		if err != nil {
			log.Fatal().Err(err).Msg("accept")
		}
		log.Info().Msgf("accept %s", conn.RemoteAddr())

		e := cmdenv{
			DB:           d.db,
			WG:           &d.wg,
			Conn:         conn,
			KernelConfig: d.kernelConfig,
		}

		go handler(conn, e)
	}
}
