// Copyright 2023 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"code.dumpstack.io/tools/out-of-tree/config"
	"code.dumpstack.io/tools/out-of-tree/distro"
	"code.dumpstack.io/tools/out-of-tree/fs"
	"code.dumpstack.io/tools/out-of-tree/qemu"
)

type ImageCmd struct {
	List ImageListCmd `cmd:"" help:"list images"`
	Edit ImageEditCmd `cmd:"" help:"edit image"`
}

type ImageListCmd struct{}

func (cmd *ImageListCmd) Run(g *Globals) (err error) {
	entries, err := os.ReadDir(config.Dir("images"))
	if err != nil {
		return
	}

	for _, e := range entries {
		fmt.Println(e.Name())
	}

	return
}

type ImageEditCmd struct {
	Name   string `help:"image name" required:""`
	DryRun bool   `help:"do nothing, just print commands"`
}

func (cmd *ImageEditCmd) Run(g *Globals) (err error) {
	image := filepath.Join(config.Dir("images"), cmd.Name)
	if !fs.PathExists(image) {
		fmt.Println("image does not exist")
	}

	kcfg, err := config.ReadKernelConfig(g.Config.Kernels)
	if err != nil {
		return
	}

	if len(kcfg.Kernels) == 0 {
		return errors.New("No kernels found")
	}

	ki := distro.KernelInfo{}
	for _, k := range kcfg.Kernels {
		if k.RootFS == image {
			ki = k
			break
		}
	}

	kernel := qemu.Kernel{
		KernelPath: ki.KernelPath,
		InitrdPath: ki.InitrdPath,
	}

	q, err := qemu.NewSystem(qemu.X86x64, kernel, ki.RootFS)

	q.Mutable = true

	if cmd.DryRun {
		s := q.Executable()
		for _, arg := range q.Args() {
			if strings.Contains(arg, " ") ||
				strings.Contains(arg, ",") {

				s += fmt.Sprintf(` "%s"`, arg)
			} else {
				s += fmt.Sprintf(" %s", arg)
			}
		}
		fmt.Println(s)
		fmt.Println(q.GetSSHCommand())
		return
	}

	err = q.Start()
	if err != nil {
		fmt.Println("Qemu start error:", err)
		return
	}
	defer q.Stop()

	fmt.Print("ssh command:\n\n\t")
	fmt.Println(q.GetSSHCommand())

	fmt.Print("\npress enter to stop")
	fmt.Scanln()

	q.Command("root", "poweroff")

	for !q.Died {
		time.Sleep(time.Second)
	}
	return
}
