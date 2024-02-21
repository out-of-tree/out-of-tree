// Copyright 2023 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/naoina/toml"
	"github.com/remeh/sizedwaitgroup"
	"github.com/rs/zerolog/log"

	"code.dumpstack.io/tools/out-of-tree/artifact"
	"code.dumpstack.io/tools/out-of-tree/cache"
	"code.dumpstack.io/tools/out-of-tree/config"
	"code.dumpstack.io/tools/out-of-tree/config/dotfiles"
	"code.dumpstack.io/tools/out-of-tree/container"
	"code.dumpstack.io/tools/out-of-tree/distro"
	"code.dumpstack.io/tools/out-of-tree/kernel"
)

type KernelCmd struct {
	NoDownload     bool `help:"do not download qemu image while kernel generation"`
	UseHost        bool `help:"also use host kernels"`
	Force          bool `help:"force reinstall kernel"`
	NoHeaders      bool `help:"do not install kernel headers"`
	Shuffle        bool `help:"randomize kernels installation order"`
	Retries        int  `help:"amount of tries for each kernel" default:"2"`
	Threads        int  `help:"threads for parallel installation" default:"1"`
	Update         bool `help:"update container"`
	ContainerCache bool `help:"try prebuilt container images first" default:"true" negatable:""`
	Max            int  `help:"maximum kernels to download" default:"100500"`
	NoPrune        bool `help:"do not remove dangling or unused images from local storage after build"`
	NoCfgRegen     bool `help:"do not update kernels.toml"`

	ContainerTimeout time.Duration `help:"container timeout"`

	List        KernelListCmd        `cmd:"" help:"list kernels"`
	ListRemote  KernelListRemoteCmd  `cmd:"" help:"list remote kernels"`
	Autogen     KernelAutogenCmd     `cmd:"" help:"generate kernels based on the current config"`
	Genall      KernelGenallCmd      `cmd:"" help:"generate all kernels for distro"`
	Install     KernelInstallCmd     `cmd:"" help:"install specific kernel"`
	ConfigRegen KernelConfigRegenCmd `cmd:"" help:"regenerate config"`

	shutdown bool
	kcfg     config.KernelConfig

	stats struct {
		overall int
		success int
	}
}

func (cmd KernelCmd) UpdateConfig() (err error) {
	if cmd.stats.success != cmd.stats.overall {
		log.Warn().Msgf("%d kernels failed to install",
			cmd.stats.overall-cmd.stats.success)
	}

	if cmd.NoCfgRegen {
		log.Info().Msgf("kernels.toml is not updated")
		return
	}

	log.Info().Msgf("updating kernels.toml")
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

	err = os.WriteFile(dotfiles.File("kernels.toml"), buf, os.ModePerm)
	if err != nil {
		return
	}

	log.Info().Msgf("kernels.toml successfully updated")
	return
}

func (cmd *KernelCmd) GenKernel(km artifact.Target, pkg string) {
	flog := log.With().
		Str("kernel", pkg).
		Str("distro", km.Distro.String()).
		Logger()

	reinstall := false
	for _, kinfo := range cmd.kcfg.Kernels {
		if !km.Distro.Equal(kinfo.Distro) {
			continue
		}

		var found bool
		if kinfo.Distro.ID == distro.Debian { // FIXME
			found = pkg == kinfo.Package
		} else if kinfo.Distro.ID == distro.OpenSUSE {
			found = strings.Contains(pkg, kinfo.KernelRelease)
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

	cmd.stats.overall += 1

	var attempt int
	for {
		attempt++

		if cmd.shutdown {
			return
		}

		err := km.Distro.Install(pkg, !cmd.NoHeaders)
		if err == nil {
			cmd.stats.success += 1
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

func (cmd *KernelCmd) Generate(g *Globals, km artifact.Target) (err error) {
	defer func() {
		if err != nil {
			log.Warn().Err(err).Msg("")
		} else {
			log.Debug().Err(err).Msg("")
		}
	}()

	if cmd.Update {
		container.UseCache = false
	}
	if cmd.NoPrune {
		container.Prune = false
	}

	cmd.kcfg, err = config.ReadKernelConfig(g.Config.Kernels)
	if err != nil {
		log.Debug().Err(err).Msg("read kernels config")
	}

	container.Commands = g.Config.Docker.Commands
	container.Registry = g.Config.Docker.Registry
	container.Timeout = g.Config.Docker.Timeout.Duration
	if cmd.ContainerTimeout != 0 {
		container.Timeout = cmd.ContainerTimeout
	}

	log.Info().Msgf("Generating for target %v", km)

	_, err = kernel.GenRootfsImage(km.Distro.RootFS(), !cmd.NoDownload)
	if err != nil || cmd.shutdown {
		return
	}

	c, err := container.New(km.Distro)
	if err != nil || cmd.shutdown {
		return
	}

	if cmd.ContainerCache {
		path := cache.ContainerURL(c.Name())
		err = container.Import(path, c.Name())
		if err != nil || cmd.shutdown {
			return
		}
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

		if cmd.shutdown {
			err = nil
			swg.Done()
			return
		}

		if cmd.stats.success >= cmd.Max {
			log.Print("Max is reached")
			swg.Done()
			break
		}

		log.Info().Msgf("%d/%d %s", i+1, len(pkgs), pkg)

		go func(p string) {
			defer swg.Done()
			cmd.GenKernel(km, p)
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
		return errors.New("no kernels found")
	}

	for _, k := range kcfg.Kernels {
		fmt.Println(k.Distro.ID, k.Distro.Release, k.KernelRelease)
	}

	return
}

type KernelListRemoteCmd struct {
	Distro string `required:"" help:"distribution"`
	Ver    string `help:"distro version"`
}

func (cmd *KernelListRemoteCmd) Run(kernelCmd *KernelCmd, g *Globals) (err error) {
	if kernelCmd.Update {
		container.UseCache = false
	}
	if kernelCmd.NoPrune {
		container.Prune = false
	}

	distroType, err := distro.NewID(cmd.Distro)
	if err != nil {
		return
	}

	km := artifact.Target{
		Distro: distro.Distro{ID: distroType, Release: cmd.Ver},
		Kernel: artifact.Kernel{Regex: ".*"},
	}

	_, err = kernel.GenRootfsImage(km.Distro.RootFS(), false)
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

type KernelAutogenCmd struct{}

func (cmd *KernelAutogenCmd) Run(kernelCmd *KernelCmd, g *Globals) (err error) {
	ka, err := artifact.Artifact{}.Read(g.WorkDir + "/.out-of-tree.toml")
	if err != nil {
		return
	}

	kernel.SetSigintHandler(&kernelCmd.shutdown)

	for _, sk := range ka.Targets {
		if sk.Distro.Release == "" {
			err = errors.New("please set distro_release")
			return
		}

		err = kernelCmd.Generate(g, sk)
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
		if kernelCmd.shutdown {
			break
		}

		if distroType != distro.None && distroType != dist.ID {
			continue
		}

		if cmd.Ver != "" && dist.Release != cmd.Ver {
			continue
		}

		target := artifact.Target{
			Distro: dist,
			Kernel: artifact.Kernel{Regex: ".*"},
		}

		err = kernelCmd.Generate(g, target)
		if err != nil {
			continue
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

	km := artifact.Target{
		Distro: distro.Distro{ID: distroType, Release: cmd.Ver},
		Kernel: artifact.Kernel{Regex: cmd.Kernel},
	}
	err = kernelCmd.Generate(g, km)
	if err != nil {
		return
	}

	return kernelCmd.UpdateConfig()
}

type KernelConfigRegenCmd struct{}

func (cmd *KernelConfigRegenCmd) Run(kernelCmd *KernelCmd, g *Globals) (err error) {
	return kernelCmd.UpdateConfig()
}
