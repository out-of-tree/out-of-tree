// Copyright 2023 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

//go:build linux
// +build linux

package main

import (
	"io/ioutil"
	"os/exec"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/zcalusic/sysinfo"

	"code.dumpstack.io/tools/out-of-tree/config"
)

func genHostKernels(download bool) (kcfg config.KernelConfig, err error) {
	si := sysinfo.SysInfo{}
	si.GetSysInfo()

	distroType, err := config.NewDistroType(si.OS.Vendor)
	if err != nil {
		return
	}

	cmd := exec.Command("ls", "/lib/modules")
	log.Debug().Msgf("%v", cmd)

	rawOutput, err := cmd.CombinedOutput()
	if err != nil {
		log.Print(string(rawOutput), err)
		return
	}

	kernelsBase := "/boot/"
	files, err := ioutil.ReadDir(kernelsBase)
	if err != nil {
		return
	}

	// only for compatibility, docker is not really used
	dii := containerImageInfo{
		Name: config.KernelMask{
			DistroType:    distroType,
			DistroRelease: si.OS.Version,
		}.DockerName(),
	}

	rootfs, err := genRootfsImage(dii, download)
	if err != nil {
		return
	}

	for _, k := range strings.Fields(string(rawOutput)) {
		ki := config.KernelInfo{
			DistroType:    distroType,
			DistroRelease: si.OS.Version,
			KernelRelease: k,

			KernelSource: "/lib/modules/" + k + "/build",

			KernelPath: kernelsBase + genKernelPath(files, k),
			InitrdPath: kernelsBase + genInitrdPath(files, k),
			RootFS:     rootfs,
		}

		vmlinux := "/usr/lib/debug/boot/vmlinux-" + k
		log.Print("vmlinux", vmlinux)
		if exists(vmlinux) {
			ki.VmlinuxPath = vmlinux
		}

		kcfg.Kernels = append(kcfg.Kernels, ki)
	}

	return
}
