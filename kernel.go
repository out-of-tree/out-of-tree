// Copyright 2023 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"github.com/naoina/toml"
	"github.com/remeh/sizedwaitgroup"
	"github.com/rs/zerolog/log"

	"code.dumpstack.io/tools/out-of-tree/config"
	"code.dumpstack.io/tools/out-of-tree/container"
	"code.dumpstack.io/tools/out-of-tree/distro"
	"code.dumpstack.io/tools/out-of-tree/kernel"
)

type KernelCmd struct {
	NoDownload bool `help:"do not download qemu image while kernel generation"`
	UseHost    bool `help:"also use host kernels"`
	Force      bool `help:"force reinstall kernel"`
	NoHeaders  bool `help:"do not install kernel headers"`
	Shuffle    bool `help:"randomize kernels installation order"`
	Retries    int  `help:"amount of tries for each kernel" default:"10"`
	Threads    int  `help:"threads for parallel installation" default:"1"`
	Update     bool `help:"update container"`

	List        KernelListCmd        `cmd:"" help:"list kernels"`
	ListRemote  KernelListRemoteCmd  `cmd:"" help:"list remote kernels"`
	Autogen     KernelAutogenCmd     `cmd:"" help:"generate kernels based on the current config"`
	Genall      KernelGenallCmd      `cmd:"" help:"generate all kernels for distro"`
	Install     KernelInstallCmd     `cmd:"" help:"install specific kernel"`
	ConfigRegen KernelConfigRegenCmd `cmd:"" help:"regenerate config"`

	shutdown bool
	kcfg     config.KernelConfig
}

func (cmd KernelCmd) UpdateConfig() (err error) {
	kcfg := config.KernelConfig{}

	if cmd.UseHost {
		// Get host kernels
		kcfg.Kernels, err = kernel.GenHostKernels(!cmd.NoDownload)
		if err != nil {
			return
		}
	}

	for _, dist := range distro.List() {
		var kernels []distro.KernelInfo
		kernels, err = dist.Kernels()
		if err != nil {
			return
		}

		kcfg.Kernels = append(kcfg.Kernels, kernels...)
	}

	buf, err := toml.Marshal(&kcfg)
	if err != nil {
		return
	}

	err = os.WriteFile(config.File("kernels.toml"), buf, os.ModePerm)
	if err != nil {
		return
	}

	log.Info().Msgf("kernels.toml successfully updated")
	return
}

func (cmd *KernelCmd) GenKernel(km config.Target, pkg string, max *int) {
	flog := log.With().
		Str("kernel", pkg).
		Str("distro", km.Distro.String()).
		Logger()

	reinstall := false
	for _, kinfo := range cmd.kcfg.Kernels {
		var found bool
		if kinfo.Distro.ID == distro.Debian { // FIXME
			found = pkg == kinfo.Package
		} else {
			found = strings.Contains(pkg, kinfo.KernelVersion)
		}

		if found {
			if !cmd.Force {
				flog.Info().Msg("already installed")
				return
			}
			reinstall = true
			break
		}
	}

	if reinstall {
		flog.Info().Msg("reinstall")
	} else {
		flog.Info().Msg("install")
	}

	var attempt int
	for {
		attempt++

		if cmd.shutdown {
			return
		}

		err := km.Distro.Install(pkg, !cmd.NoHeaders)
		if err == nil {
			*max--
			flog.Info().Msg("success")
			break
		} else if attempt >= cmd.Retries {
			flog.Error().Err(err).Msg("install kernel")
			flog.Debug().Msg("skip")
			break
		} else {
			flog.Warn().Err(err).Msg("install kernel")
			time.Sleep(time.Second)
			flog.Info().Msg("retry")
		}
	}
}

func (cmd *KernelCmd) Generate(g *Globals, km config.Target, max int) (err error) {
	if cmd.Update {
		container.Update = true
	}

	cmd.kcfg, err = config.ReadKernelConfig(g.Config.Kernels)
	if err != nil {
		log.Debug().Err(err).Msg("read kernels config")
	}

	container.Commands = g.Config.Docker.Commands
	container.Registry = g.Config.Docker.Registry

	log.Info().Msgf("Generating for target %v", km)

	_, err = kernel.GenRootfsImage(container.Image{Name: km.DockerName()},
		!cmd.NoDownload)
	if err != nil || cmd.shutdown {
		return
	}

	pkgs, err := kernel.MatchPackages(km)
	if err != nil || cmd.shutdown {
		return
	}

	if cmd.Shuffle {
		pkgs = kernel.ShuffleStrings(pkgs)
	}

	swg := sizedwaitgroup.New(cmd.Threads)

	for i, pkg := range pkgs {
		if cmd.shutdown {
			err = nil
			return
		}

		swg.Add()

		if max <= 0 {
			log.Print("Max is reached")
			swg.Done()
			break
		}

		log.Info().Msgf("%d/%d %s", i+1, len(pkgs), pkg)

		go func(p string) {
			defer swg.Done()
			cmd.GenKernel(km, p, &max)
		}(pkg)
	}
	swg.Wait()

	return
}

type KernelListCmd struct{}

func (cmd *KernelListCmd) Run(g *Globals) (err error) {
	kcfg, err := config.ReadKernelConfig(g.Config.Kernels)
	if err != nil {
		log.Debug().Err(err).Msg("read kernel config")
	}

	if len(kcfg.Kernels) == 0 {
		return errors.New("No kernels found")
	}

	for _, k := range kcfg.Kernels {
		fmt.Println(k.Distro.ID, k.Distro.Release, k.KernelRelease)
	}

	return
}

type KernelListRemoteCmd struct {
	Distro string `required:"" help:"distribution"`
	Ver    string `required:"" help:"distro version"`
}

func (cmd *KernelListRemoteCmd) Run(kernelCmd *KernelCmd, g *Globals) (err error) {
	distroType, err := distro.NewID(cmd.Distro)
	if err != nil {
		return
	}

	km := config.Target{
		Distro: distro.Distro{ID: distroType, Release: cmd.Ver},
		Kernel: config.Kernel{Regex: ".*"},
	}

	_, err = kernel.GenRootfsImage(container.Image{Name: km.DockerName()}, false)
	if err != nil {
		return
	}

	container.Registry = g.Config.Docker.Registry
	container.Commands = g.Config.Docker.Commands

	pkgs, err := kernel.MatchPackages(km)
	// error check skipped on purpose

	for _, k := range pkgs {
		fmt.Println(k)
	}

	return
}

type KernelAutogenCmd struct {
	Max int `help:"download kernels from set defined by regex in release_mask, but no more than X for each of release_mask" default:"100500"`
}

func (cmd KernelAutogenCmd) Run(kernelCmd *KernelCmd, g *Globals) (err error) {
	ka, err := config.ReadArtifactConfig(g.WorkDir + "/.out-of-tree.toml")
	if err != nil {
		return
	}

	kernel.SetSigintHandler(&kernelCmd.shutdown)

	for _, sk := range ka.Targets {
		if sk.Distro.Release == "" {
			err = errors.New("Please set distro_release")
			return
		}

		err = kernelCmd.Generate(g, sk, cmd.Max)
		if err != nil {
			return
		}
		if kernelCmd.shutdown {
			break
		}
	}

	return kernelCmd.UpdateConfig()
}

type KernelGenallCmd struct {
	Distro string `help:"distribution"`
	Ver    string `help:"distro version"`
}

func (cmd *KernelGenallCmd) Run(kernelCmd *KernelCmd, g *Globals) (err error) {
	distroType, err := distro.NewID(cmd.Distro)
	if err != nil {
		return
	}

	kernel.SetSigintHandler(&kernelCmd.shutdown)

	for _, dist := range distro.List() {
		if distroType != distro.None && distroType != dist.ID {
			continue
		}

		if cmd.Ver != "" && dist.Release != cmd.Ver {
			continue
		}

		target := config.Target{
			Distro: dist,
			Kernel: config.Kernel{Regex: ".*"},
		}

		err = kernelCmd.Generate(g, target, math.MaxUint32)
		if err != nil {
			return
		}
	}

	return kernelCmd.UpdateConfig()
}

type KernelInstallCmd struct {
	Distro string `required:"" help:"distribution"`
	Ver    string `required:"" help:"distro version"`
	Kernel string `required:"" help:"kernel release mask"`
}

func (cmd *KernelInstallCmd) Run(kernelCmd *KernelCmd, g *Globals) (err error) {
	distroType, err := distro.NewID(cmd.Distro)
	if err != nil {
		return
	}

	kernel.SetSigintHandler(&kernelCmd.shutdown)

	km := config.Target{
		Distro: distro.Distro{ID: distroType, Release: cmd.Ver},
		Kernel: config.Kernel{Regex: cmd.Kernel},
	}
	err = kernelCmd.Generate(g, km, math.MaxUint32)
	if err != nil {
		return
	}

	return kernelCmd.UpdateConfig()
}

type KernelConfigRegenCmd struct{}

func (cmd *KernelConfigRegenCmd) Run(kernelCmd *KernelCmd, g *Globals) (err error) {
	return kernelCmd.UpdateConfig()
}
