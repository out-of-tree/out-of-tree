// Copyright 2023 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"math"

	"github.com/rs/zerolog/log"

	"code.dumpstack.io/tools/out-of-tree/config"
	"code.dumpstack.io/tools/out-of-tree/container"
	"code.dumpstack.io/tools/out-of-tree/kernel"
)

type KernelCmd struct {
	NoDownload bool  `help:"do not download qemu image while kernel generation"`
	UseHost    bool  `help:"also use host kernels"`
	Force      bool  `help:"force reinstall kernel"`
	NoHeaders  bool  `help:"do not install kernel headers"`
	Shuffle    bool  `help:"randomize kernels installation order"`
	Retries    int64 `help:"amount of tries for each kernel" default:"10"`
	Update     bool  `help:"update container"`

	List        KernelListCmd        `cmd:"" help:"list kernels"`
	ListRemote  KernelListRemoteCmd  `cmd:"" help:"list remote kernels"`
	Autogen     KernelAutogenCmd     `cmd:"" help:"generate kernels based on the current config"`
	Genall      KernelGenallCmd      `cmd:"" help:"generate all kernels for distro"`
	Install     KernelInstallCmd     `cmd:"" help:"install specific kernel"`
	ConfigRegen KernelConfigRegenCmd `cmd:"" help:"regenerate config"`
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
		fmt.Println(k.DistroType, k.DistroRelease, k.KernelRelease)
	}

	return
}

type KernelListRemoteCmd struct {
	Distro string `required:"" help:"distribution"`
	Ver    string `required:"" help:"distro version"`
}

func (cmd *KernelListRemoteCmd) Run(kernelCmd *KernelCmd, g *Globals) (err error) {
	distroType, err := config.NewDistroType(cmd.Distro)
	if err != nil {
		return
	}

	km := config.KernelMask{
		DistroType:    distroType,
		DistroRelease: cmd.Ver,
		ReleaseMask:   ".*",
	}

	_, err = kernel.GenRootfsImage(container.Image{Name: km.DockerName()}, false)
	if err != nil {
		return
	}

	err = kernel.GenerateBaseDockerImage(
		g.Config.Docker.Registry,
		g.Config.Docker.Commands,
		km,
		kernelCmd.Update,
	)
	if err != nil {
		return
	}

	pkgs, err := kernel.MatchPackages(km)
	// error check skipped on purpose

	for _, k := range pkgs {
		fmt.Println(k)
	}

	return
}

type KernelAutogenCmd struct {
	Max int64 `help:"download kernels from set defined by regex in release_mask, but no more than X for each of release_mask" default:"100500"`
}

func (cmd KernelAutogenCmd) Run(kernelCmd *KernelCmd, g *Globals) (err error) {
	ka, err := config.ReadArtifactConfig(g.WorkDir + "/.out-of-tree.toml")
	if err != nil {
		return
	}

	shutdown := false
	kernel.SetSigintHandler(&shutdown)

	for _, sk := range ka.SupportedKernels {
		if sk.DistroRelease == "" {
			err = errors.New("Please set distro_release")
			return
		}

		err = kernel.GenerateKernels(sk,
			g.Config.Docker.Registry,
			g.Config.Docker.Commands,
			cmd.Max, kernelCmd.Retries,
			!kernelCmd.NoDownload,
			kernelCmd.Force,
			!kernelCmd.NoHeaders,
			kernelCmd.Shuffle,
			kernelCmd.Update,
			&shutdown,
		)
		if err != nil {
			return
		}
		if shutdown {
			break
		}
	}

	return kernel.UpdateKernelsCfg(kernelCmd.UseHost, !kernelCmd.NoDownload)
}

type KernelGenallCmd struct {
	Distro string `required:"" help:"distribution"`
	Ver    string `required:"" help:"distro version"`
}

func (cmd *KernelGenallCmd) Run(kernelCmd *KernelCmd, g *Globals) (err error) {
	distroType, err := config.NewDistroType(cmd.Distro)
	if err != nil {
		return
	}

	shutdown := false
	kernel.SetSigintHandler(&shutdown)

	km := config.KernelMask{
		DistroType:    distroType,
		DistroRelease: cmd.Ver,
		ReleaseMask:   ".*",
	}
	err = kernel.GenerateKernels(km,
		g.Config.Docker.Registry,
		g.Config.Docker.Commands,
		math.MaxUint32, kernelCmd.Retries,
		!kernelCmd.NoDownload,
		kernelCmd.Force,
		!kernelCmd.NoHeaders,
		kernelCmd.Shuffle,
		kernelCmd.Update,
		&shutdown,
	)
	if err != nil {
		return
	}

	return kernel.UpdateKernelsCfg(kernelCmd.UseHost, !kernelCmd.NoDownload)
}

type KernelInstallCmd struct {
	Distro string `required:"" help:"distribution"`
	Ver    string `required:"" help:"distro version"`
	Kernel string `required:"" help:"kernel release mask"`
}

func (cmd *KernelInstallCmd) Run(kernelCmd *KernelCmd, g *Globals) (err error) {
	distroType, err := config.NewDistroType(cmd.Distro)
	if err != nil {
		return
	}

	shutdown := false
	kernel.SetSigintHandler(&shutdown)

	km := config.KernelMask{
		DistroType:    distroType,
		DistroRelease: cmd.Ver,
		ReleaseMask:   cmd.Kernel,
	}
	err = kernel.GenerateKernels(km,
		g.Config.Docker.Registry,
		g.Config.Docker.Commands,
		math.MaxUint32, kernelCmd.Retries,
		!kernelCmd.NoDownload,
		kernelCmd.Force,
		!kernelCmd.NoHeaders,
		kernelCmd.Shuffle,
		kernelCmd.Update,
		&shutdown,
	)
	if err != nil {
		return
	}

	return kernel.UpdateKernelsCfg(kernelCmd.UseHost, !kernelCmd.NoDownload)
}

type KernelConfigRegenCmd struct{}

func (cmd *KernelConfigRegenCmd) Run(kernelCmd *KernelCmd, g *Globals) (err error) {
	return kernel.UpdateKernelsCfg(kernelCmd.UseHost, !kernelCmd.NoDownload)
}
