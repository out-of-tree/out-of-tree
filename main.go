// Copyright 2023 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package main

import (
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"time"

	"github.com/natefinch/lumberjack"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/alecthomas/kong"

	"code.dumpstack.io/tools/out-of-tree/config"
)

type Globals struct {
	Config config.OutOfTree `help:"path to out-of-tree configuration" default:"~/.out-of-tree/out-of-tree.toml"`

	WorkDir string `help:"path to work directory" default:"./" type:"path"`
}

type CLI struct {
	Globals

	Pew       PewCmd       `cmd:"" help:"build, run, and test module/exploit"`
	Kernel    KernelCmd    `cmd:"" help:"manipulate kernels"`
	Debug     DebugCmd     `cmd:"" help:"debug environment"`
	Log       LogCmd       `cmd:"" help:"query logs"`
	Pack      PackCmd      `cmd:"" help:"exploit pack test"`
	Gen       GenCmd       `cmd:"" help:"generate .out-of-tree.toml skeleton"`
	Image     ImageCmd     `cmd:"" help:"manage images"`
	Container ContainerCmd `cmd:"" help:"manage containers"`
	Debian    DebianCmd    `cmd:"" help:"" hidden:""`

	Version VersionFlag `name:"version" help:"print version information and quit"`

	LogLevel LogLevelFlag `enum:"trace,debug,info,warn,error" default:"info"`

	ContainerRuntime string `enum:"podman,docker" default:"podman"`
}

type LogLevelFlag string

func (loglevel LogLevelFlag) AfterApply() error {
	switch loglevel {
	case "debug", "trace":
		zerolog.CallerMarshalFunc = func(pc uintptr, file string, line int) string {
			short := file
			for i := len(file) - 1; i > 0; i-- {
				if file[i] == '/' {
					short = file[i+1:]
					break
				}
			}
			file = short
			return file + ":" + strconv.Itoa(line)
		}
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

type LevelWriter struct {
	io.Writer
	Level zerolog.Level
}

func (lw *LevelWriter) WriteLevel(l zerolog.Level, p []byte) (n int, err error) {
	if l >= lw.Level {
		return lw.Writer.Write(p)
	}
	return len(p), nil
}

func isFsCaseInsensitive(dir string) (yes bool, err error) {
	pathLowercase := filepath.Join(dir, "file")
	fLowercase, err := os.Create(pathLowercase)
	if err != nil {
		return
	}
	defer fLowercase.Close()
	defer os.Remove(pathLowercase)

	pathUppercase := filepath.Join(dir, "FILE")
	fUppercase, err := os.Create(pathUppercase)
	if err != nil {
		return
	}
	defer fUppercase.Close()
	defer os.Remove(pathUppercase)

	statLowercase, err := fLowercase.Stat()
	if err != nil {
		return
	}

	statUppercase, err := fUppercase.Stat()
	if err != nil {
		return
	}

	yes = os.SameFile(statLowercase, statUppercase)
	return
}

var tempDirBase string

var consoleWriter, fileWriter LevelWriter

var loglevel zerolog.Level

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
		loglevel = zerolog.TraceLevel
	case "debug":
		loglevel = zerolog.DebugLevel
	case "info":
		loglevel = zerolog.InfoLevel
	case "warn":
		loglevel = zerolog.WarnLevel
	case "error":
		loglevel = zerolog.ErrorLevel
	}

	usr, err := user.Current()
	if err != nil {
		return
	}

	tempDirBase = usr.HomeDir + "/.out-of-tree/tmp/"
	os.MkdirAll(tempDirBase, os.ModePerm)

	consoleWriter = LevelWriter{Writer: zerolog.NewConsoleWriter(
		func(w *zerolog.ConsoleWriter) {
			w.Out = os.Stderr
		},
	),
		Level: loglevel,
	}

	fileWriter = LevelWriter{Writer: &lumberjack.Logger{
		Filename: usr.HomeDir + "/.out-of-tree/logs/out-of-tree.log",
	},
		Level: zerolog.TraceLevel,
	}

	log.Logger = log.Output(zerolog.MultiLevelWriter(
		&consoleWriter,
		&fileWriter,
	))

	log.Trace().Msg("start out-of-tree")
	log.Debug().Msgf("%v", os.Args)
	log.Debug().Msgf("%v", cli)

	if buildInfo, ok := debug.ReadBuildInfo(); ok {
		log.Debug().Msgf("%v", buildInfo.GoVersion)
		log.Debug().Msgf("%v", buildInfo.Settings)
	}

	path := filepath.Join(usr.HomeDir, ".out-of-tree")
	yes, err := isFsCaseInsensitive(path)
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
	containerRuntime = cli.ContainerRuntime

	err = ctx.Run(&cli.Globals)
	ctx.FatalIfErrorf(err)
}
