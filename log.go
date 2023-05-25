// Copyright 2023 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"

	"github.com/olekukonko/tablewriter"
	"github.com/rs/zerolog/log"
	"gopkg.in/logrusorgru/aurora.v2"

	"code.dumpstack.io/tools/out-of-tree/config"
)

type LogCmd struct {
	Query    LogQueryCmd    `cmd:"" help:"query logs"`
	Dump     LogDumpCmd     `cmd:"" help:"show all info for log entry with ID"`
	Json     LogJsonCmd     `cmd:"" help:"generate json statistics"`
	Markdown LogMarkdownCmd `cmd:"" help:"generate markdown statistics"`
}

type LogQueryCmd struct {
	Num  int    `help:"how much lines" default:"50"`
	Rate bool   `help:"show artifact success rate"`
	Tag  string `help:"filter tag"`
}

func (cmd *LogQueryCmd) Run(g *Globals) (err error) {
	db, err := openDatabase(g.Config.Database)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	var les []logEntry

	ka, kaErr := config.ReadArtifactConfig(g.WorkDir + "/.out-of-tree.toml")
	if kaErr == nil {
		log.Print(".out-of-tree.toml found, filter by artifact name")
		les, err = getAllArtifactLogs(db, cmd.Tag, cmd.Num, ka)
	} else {
		les, err = getAllLogs(db, cmd.Tag, cmd.Num)
	}
	if err != nil {
		return
	}

	s := "\nS"
	if cmd.Rate {
		if kaErr != nil {
			err = kaErr
			return
		}

		s = fmt.Sprintf("{[%s] %s} Overall s", ka.Type, ka.Name)

		les, err = getAllArtifactLogs(db, cmd.Tag, math.MaxInt64, ka)
		if err != nil {
			return
		}
	} else {
		for _, l := range les {
			logLogEntry(l)
		}
	}

	success := 0
	for _, l := range les {
		if l.Test.Ok {
			success++
		}
	}

	overall := float64(success) / float64(len(les))
	fmt.Printf("%success rate: %.04f (~%.0f%%)\n",
		s, overall, overall*100)

	return
}

type LogDumpCmd struct {
	ID int `help:"id" default:"-1"`
}

func (cmd *LogDumpCmd) Run(g *Globals) (err error) {
	db, err := openDatabase(g.Config.Database)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	var l logEntry
	if cmd.ID > 0 {
		l, err = getLogByID(db, cmd.ID)
	} else {
		l, err = getLastLog(db)
	}
	if err != nil {
		return
	}

	fmt.Println("ID:", l.ID)
	fmt.Println("Date:", l.Timestamp.Format("2006-01-02 15:04"))
	fmt.Println("Tag:", l.Tag)
	fmt.Println()

	fmt.Println("Type:", l.Type.String())
	fmt.Println("Name:", l.Name)
	fmt.Println()

	fmt.Println("Distro:", l.Distro.ID.String(), l.Distro.Release)
	fmt.Println("Kernel:", l.KernelRelease)
	fmt.Println()

	fmt.Println("Build ok:", l.Build.Ok)
	if l.Type == config.KernelModule {
		fmt.Println("Insmod ok:", l.Run.Ok)
	}
	fmt.Println("Test ok:", l.Test.Ok)
	fmt.Println()

	fmt.Printf("Build output:\n%s\n", l.Build.Output)
	fmt.Println()

	if l.Type == config.KernelModule {
		fmt.Printf("Insmod output:\n%s\n", l.Run.Output)
		fmt.Println()
	}

	fmt.Printf("Test output:\n%s\n", l.Test.Output)
	fmt.Println()

	fmt.Printf("Qemu stdout:\n%s\n", l.Stdout)
	fmt.Println()

	fmt.Printf("Qemu stderr:\n%s\n", l.Stderr)
	fmt.Println()

	return
}

type LogJsonCmd struct {
	Tag string `required:"" help:"filter tag"`
}

func (cmd *LogJsonCmd) Run(g *Globals) (err error) {
	db, err := openDatabase(g.Config.Database)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	distros, err := getStats(db, g.WorkDir, cmd.Tag)
	if err != nil {
		return
	}

	bytes, err := json.Marshal(&distros)
	if err != nil {
		return
	}

	fmt.Println(string(bytes))
	return
}

type LogMarkdownCmd struct {
	Tag string `required:"" help:"filter tag"`
}

func (cmd *LogMarkdownCmd) Run(g *Globals) (err error) {
	db, err := openDatabase(g.Config.Database)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	distros, err := getStats(db, g.WorkDir, cmd.Tag)
	if err != nil {
		return
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Distro", "Release", "Kernel", "Reliability"})
	table.SetBorders(tablewriter.Border{
		Left: true, Top: false, Right: true, Bottom: false})
	table.SetCenterSeparator("|")

	for distro, releases := range distros {
		for release, kernels := range releases {
			for kernel, stats := range kernels {
				all := float64(stats.All)
				ok := float64(stats.TestOK)
				r := fmt.Sprintf("%6.02f%%", (ok/all)*100)
				table.Append([]string{distro, release, kernel, r})
			}
		}
	}

	table.Render()
	return
}

func center(s string, w int) string {
	return fmt.Sprintf("%[1]*s", -w, fmt.Sprintf("%[1]*s", (w+len(s))/2, s))
}

func genOkFailCentered(name string, ok bool) (aurv aurora.Value) {
	name = center(name, 10)
	if ok {
		aurv = aurora.BgGreen(aurora.Black(name))
	} else {
		aurv = aurora.BgRed(aurora.White(aurora.Bold(name)))
	}
	return
}

func logLogEntry(l logEntry) {
	distroInfo := fmt.Sprintf("%s-%s {%s}", l.Distro.ID,
		l.Distro.Release, l.KernelRelease)

	artifactInfo := fmt.Sprintf("{[%s] %s}", l.Type, l.Name)

	timestamp := l.Timestamp.Format("2006-01-02 15:04")

	var status aurora.Value
	if l.InternalErrorString != "" {
		status = genOkFailCentered("INTERNAL", false)
	} else if l.Type == config.KernelExploit {
		if l.Build.Ok {
			status = genOkFailCentered("LPE", l.Test.Ok)
		} else {
			status = genOkFailCentered("BUILD", l.Build.Ok)
		}
	} else {
		if l.Build.Ok {
			if l.Run.Ok {
				status = genOkFailCentered("TEST", l.Test.Ok)
			} else {
				status = genOkFailCentered("INSMOD", l.Run.Ok)
			}
		} else {
			status = genOkFailCentered("BUILD", l.Build.Ok)
		}
	}

	additional := ""
	if l.KernelPanic {
		additional = "(panic)"
	} else if l.KilledByTimeout {
		additional = "(timeout)"
	}

	colored := aurora.Sprintf("[%4d %4s] [%s] %-40s %-70s: %s %s",
		l.ID, l.Tag, timestamp, artifactInfo, distroInfo, status,
		additional)

	fmt.Println(colored)
}

type runstat struct {
	All, BuildOK, RunOK, TestOK, Timeout, Panic int
}

func getStats(db *sql.DB, path, tag string) (
	distros map[string]map[string]map[string]runstat, err error) {

	var les []logEntry

	ka, kaErr := config.ReadArtifactConfig(path + "/.out-of-tree.toml")
	if kaErr == nil {
		les, err = getAllArtifactLogs(db, tag, -1, ka)
	} else {
		les, err = getAllLogs(db, tag, -1)
	}
	if err != nil {
		return
	}

	distros = make(map[string]map[string]map[string]runstat)

	for _, l := range les {
		_, ok := distros[l.Distro.ID.String()]
		if !ok {
			distros[l.Distro.ID.String()] = make(map[string]map[string]runstat)
		}

		_, ok = distros[l.Distro.ID.String()][l.Distro.Release]
		if !ok {
			distros[l.Distro.ID.String()][l.Distro.Release] = make(map[string]runstat)
		}

		rs := distros[l.Distro.ID.String()][l.Distro.Release][l.KernelRelease]

		rs.All++
		if l.Build.Ok {
			rs.BuildOK++
		}
		if l.Run.Ok {
			rs.RunOK++
		}
		if l.Test.Ok {
			rs.TestOK++
		}
		if l.KernelPanic {
			rs.Panic++
		}
		if l.KilledByTimeout {
			rs.Timeout++
		}

		distros[l.Distro.ID.String()][l.Distro.Release][l.KernelRelease] = rs
	}

	return
}
