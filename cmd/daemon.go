// Copyright 2024 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/rs/zerolog/log"

	"code.dumpstack.io/tools/out-of-tree/api"
	"code.dumpstack.io/tools/out-of-tree/client"
)

type daemonCmd struct {
	Addr string `default:":63527"`

	Job  DaemonJobCmd  `cmd:"" aliases:"jobs" help:"manage jobs"`
	Repo DaemonRepoCmd `cmd:"" aliases:"repos" help:"manage repositories"`
}

type DaemonJobCmd struct {
	List   DaemonJobsListCmd   `cmd:"" help:"list jobs"`
	Status DaemonJobsStatusCmd `cmd:"" help:"show job status"`
	Log    DaemonJobsLogsCmd   `cmd:"" help:"job logs"`
}

type DaemonJobsListCmd struct {
	Group  string `help:"group uuid"`
	Repo   string `help:"repo name"`
	Commit string `help:"commit sha"`
	Status string `help:"job status"`
}

func (cmd *DaemonJobsListCmd) Run(dm *DaemonCmd, g *Globals) (err error) {
	c := client.Client{RemoteAddr: g.RemoteAddr}

	params := api.ListJobsParams{
		Group:  cmd.Group,
		Repo:   cmd.Repo,
		Commit: cmd.Commit,
		Status: api.Status(cmd.Status),
	}

	jobs, err := c.Jobs(params)
	if err != nil {
		log.Error().Err(err).Msg("")
		return
	}

	b, err := json.MarshalIndent(jobs, "", "  ")
	if err != nil {
		log.Error().Err(err).Msg("")
	}

	fmt.Println(string(b))
	return
}

type DaemonJobsStatusCmd struct {
	UUID string `arg:""`
}

func (cmd *DaemonJobsStatusCmd) Run(dm *DaemonCmd, g *Globals) (err error) {
	c := client.Client{RemoteAddr: g.RemoteAddr}
	st, err := c.JobStatus(cmd.UUID)
	if err != nil {
		log.Error().Err(err).Msg("")
		return
	}

	fmt.Println(st)
	return
}

type DaemonJobsLogsCmd struct {
	UUID string `arg:""`
}

func (cmd *DaemonJobsLogsCmd) Run(dm *DaemonCmd, g *Globals) (err error) {
	c := client.Client{RemoteAddr: g.RemoteAddr}
	logs, err := c.JobLogs(cmd.UUID)
	if err != nil {
		log.Error().Err(err).Msg("")
		return
	}

	for _, l := range logs {
		log.Info().Msg(l.Name)
		fmt.Println(l.Text)
	}
	return
}

type DaemonRepoCmd struct {
	List DaemonRepoListCmd `cmd:"" help:"list repos"`
}

type DaemonRepoListCmd struct{}

func (cmd *DaemonRepoListCmd) Run(dm *DaemonCmd, g *Globals) (err error) {
	c := client.Client{RemoteAddr: g.RemoteAddr}
	repos, err := c.Repos()
	if err != nil {
		return
	}

	b, err := json.MarshalIndent(repos, "", "  ")
	if err != nil {
		log.Error().Err(err).Msg("")
	}

	fmt.Println(string(b))
	return
}
