// Copyright 2023 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package main

import (
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/alecthomas/kong"

	"code.dumpstack.io/tools/out-of-tree/config"
)

type Globals struct {
	Config config.OutOfTree `help:"path to out-of-tree configuration" default:"~/.out-of-tree/out-of-tree.toml"`

	WorkDir string `help:"path to work directory" default:"./" type:"path"`
}

type CLI struct {
	Globals

	Pew    PewCmd    `cmd:"" help:"build, run, and test module/exploit"`
	Kernel KernelCmd `cmd:"" help:"manipulate kernels"`
	Debug  DebugCmd  `cmd:"" help:"debug environment"`
	Log    LogCmd    `cmd:"" help:"query logs"`
	Pack   PackCmd   `cmd:"" help:"exploit pack test"`
	Gen    GenCmd    `cmd:"" help:"generate .out-of-tree.toml skeleton"`
	Image  ImageCmd  `cmd:"" help:"manage images"`

	Version VersionFlag `name:"version" help:"print version information and quit"`
}

type VersionFlag string

func (v VersionFlag) Decode(ctx *kong.DecodeContext) error { return nil }
func (v VersionFlag) IsBool() bool                         { return true }
func (v VersionFlag) BeforeApply(app *kong.Kong, vars kong.Vars) error {
	fmt.Println(vars["version"])
	app.Exit(0)
	return nil
}

func main() {
	log.SetFlags(log.Lshortfile)

	rand.Seed(time.Now().UnixNano())

	cli := CLI{}
	ctx := kong.Parse(&cli,
		kong.Name("out-of-tree"),
		kong.Description("kernel {module, exploit} development tool"),
		kong.UsageOnError(),
		kong.ConfigureHelp(kong.HelpOptions{
			Compact: true,
		}),
		kong.Vars{
			"version": "1.4.0",
		},
	)

	err := ctx.Run(&cli.Globals)
	ctx.FatalIfErrorf(err)
}
