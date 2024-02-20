// Copyright 2023 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package main

import (
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/natefinch/lumberjack"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/alecthomas/kong"

	_ "code.dumpstack.io/tools/out-of-tree/distro/centos"
	_ "code.dumpstack.io/tools/out-of-tree/distro/debian"
	_ "code.dumpstack.io/tools/out-of-tree/distro/opensuse"
	_ "code.dumpstack.io/tools/out-of-tree/distro/oraclelinux"
	_ "code.dumpstack.io/tools/out-of-tree/distro/ubuntu"

	"code.dumpstack.io/tools/out-of-tree/cache"
	"code.dumpstack.io/tools/out-of-tree/cmd"
	"code.dumpstack.io/tools/out-of-tree/config"
	"code.dumpstack.io/tools/out-of-tree/container"
	"code.dumpstack.io/tools/out-of-tree/fs"
)

type CLI struct {
	cmd.Globals

	Pew       cmd.PewCmd       `cmd:"" help:"build, run, and test module/exploit"`
	Kernel    cmd.KernelCmd    `cmd:"" help:"manipulate kernels"`
	Debug     cmd.DebugCmd     `cmd:"" help:"debug environment"`
	Log       cmd.LogCmd       `cmd:"" help:"query logs"`
	Pack      cmd.PackCmd      `cmd:"" help:"exploit pack test"`
	Gen       cmd.GenCmd       `cmd:"" help:"generate .out-of-tree.toml skeleton"`
	Image     cmd.ImageCmd     `cmd:"" help:"manage images"`
	Container cmd.ContainerCmd `cmd:"" help:"manage containers"`
	Distro    cmd.DistroCmd    `cmd:"" help:"distro-related helpers"`

	Version VersionFlag `name:"version" help:"print version information and quit"`

	LogLevel LogLevelFlag `enum:"trace,debug,info,warn,error" default:"info"`

	ContainerRuntime string `enum:"podman,docker" default:"podman"`
}

func last(s []string) string {
	return s[len(s)-1]
}

func debugLevel(pc uintptr, file string, line int) string {
	function := runtime.FuncForPC(pc).Name()
	if strings.Contains(function, ".") {
		function = last(strings.Split(function, "."))
	}
	return function
}

func traceLevel(pc uintptr, file string, line int) string {
	function := runtime.FuncForPC(pc).Name()
	if strings.Contains(function, "/") {
		function = last(strings.Split(function, "/"))
	}
	return fmt.Sprintf("%s:%s:%d", file, function, line)
}

type LogLevelFlag string

func (loglevel LogLevelFlag) AfterApply() error {
	switch loglevel {
	case "debug":
		zerolog.CallerMarshalFunc = debugLevel
		log.Logger = log.With().Caller().Logger()

	case "trace":
		zerolog.CallerMarshalFunc = traceLevel
		log.Logger = log.With().Caller().Logger()
	}
	return nil
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
			"version": "2.1.2",
		},
	)

	switch cli.LogLevel {
	case "trace":
		cmd.LogLevel = zerolog.TraceLevel
	case "debug":
		cmd.LogLevel = zerolog.DebugLevel
	case "info":
		cmd.LogLevel = zerolog.InfoLevel
	case "warn":
		cmd.LogLevel = zerolog.WarnLevel
	case "error":
		cmd.LogLevel = zerolog.ErrorLevel
	}

	cmd.ConsoleWriter = cmd.LevelWriter{Writer: zerolog.NewConsoleWriter(
		func(w *zerolog.ConsoleWriter) {
			w.Out = os.Stderr
		},
	),
		Level: cmd.LogLevel,
	}

	cmd.FileWriter = cmd.LevelWriter{Writer: &lumberjack.Logger{
		Filename: config.File("logs/out-of-tree.log"),
	},
		Level: zerolog.TraceLevel,
	}

	log.Logger = log.Output(zerolog.MultiLevelWriter(
		&cmd.ConsoleWriter,
		&cmd.FileWriter,
	))

	log.Trace().Msg("start out-of-tree")
	log.Debug().Msgf("%v", os.Args)
	log.Debug().Msgf("%v", cli)

	if buildInfo, ok := debug.ReadBuildInfo(); ok {
		log.Debug().Msgf("%v", buildInfo.GoVersion)
		log.Debug().Msgf("%v", buildInfo.Settings)
	}

	path := config.Dir()
	yes, err := fs.CaseInsensitive(path)
	if err != nil {
		log.Fatal().Err(err).Msg(path)
	}
	if yes {
		log.Warn().Msg("case-insensitive file system not supported")
	}

	_, err = exec.LookPath(cli.ContainerRuntime)
	if err != nil {
		if cli.ContainerRuntime == "podman" { // default value
			log.Debug().Msgf("podman is not found in $PATH, " +
				"fall back to docker")
			cli.ContainerRuntime = "docker"
		}

		_, err = exec.LookPath(cli.ContainerRuntime)
		if err != nil {
			log.Fatal().Msgf("%v is not found in $PATH",
				cli.ContainerRuntime)
		}
	}
	container.Runtime = cli.ContainerRuntime

	if cli.Globals.CacheURL.String() != "" {
		cache.URL = cli.Globals.CacheURL.String()
	}
	log.Debug().Msgf("set cache url to %s", cache.URL)

	err = ctx.Run(&cli.Globals)
	ctx.FatalIfErrorf(err)
}
