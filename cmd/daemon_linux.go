//go:build linux
// +build linux

package cmd

import (
	"github.com/rs/zerolog/log"

	"code.dumpstack.io/tools/out-of-tree/daemon"
)

type DaemonCmd struct {
	daemonCmd

	Threads int `help:"number of threads to use"`

	OvercommitMemory float64 `help:"overcommit memory factor"`
	OvercommitCPU    float64 `help:"overcommit CPU factor"`

	Serve DaemonServeCmd `cmd:"" help:"start daemon"`
}

type DaemonServeCmd struct{}

func (cmd *DaemonServeCmd) Run(dm *DaemonCmd, g *Globals) (err error) {
	d, err := daemon.Init(g.Config.Kernels)
	if err != nil {
		log.Fatal().Err(err).Msg("")
	}
	defer d.Kill()

	if dm.Threads > 0 {
		d.Threads = dm.Threads
	}

	if dm.OvercommitMemory > 0 {
		d.Resources.CPU.SetOvercommit(dm.OvercommitMemory)
	}

	if dm.OvercommitCPU > 0 {
		d.Resources.CPU.SetOvercommit(dm.OvercommitCPU)
	}

	go d.Daemon()
	d.Listen(dm.Addr)
	return
}
