// Copyright 2023 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

//go:build linux
// +build linux

package kernel

import (
	"io/ioutil"
	"os/exec"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/zcalusic/sysinfo"

	"code.dumpstack.io/tools/out-of-tree/distro"
	"code.dumpstack.io/tools/out-of-tree/fs"
)

func GenHostKernels(download bool) (kernels []distro.KernelInfo, err error) {
	si := sysinfo.SysInfo{}
	si.GetSysInfo()

	distroType, err := distro.NewID(si.OS.Vendor)
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
	bootfiles, err := ioutil.ReadDir(kernelsBase)
	if err != nil {
		return
	}

	// only for compatibility, docker is not really used
	dist := distro.Distro{
		ID:      distroType,
		Release: si.OS.Version,
	}

	rootfs, err := GenRootfsImage(dist.RootFS(), download)
	if err != nil {
		return
	}

	for _, krel := range strings.Fields(string(rawOutput)) {
		log.Debug().Msgf("generate config entry for %s", krel)

		var kernelFile, initrdFile string
		kernelFile, err = fs.FindKernel(bootfiles, krel)
		if err != nil {
			log.Warn().Msgf("cannot find kernel %s", krel)
			continue
		}

		initrdFile, err = fs.FindInitrd(bootfiles, krel)
		if err != nil {
			log.Warn().Msgf("cannot find initrd %s", krel)
			continue
		}

		ki := distro.KernelInfo{
			Distro: distro.Distro{
				ID:      distroType,
				Release: si.OS.Version,
			},

			KernelRelease: krel,

			KernelSource: "/lib/modules/" + krel + "/build",

			KernelPath: kernelsBase + kernelFile,
			InitrdPath: kernelsBase + initrdFile,
			RootFS:     rootfs,
		}

		vmlinux := "/usr/lib/debug/boot/vmlinux-" + krel
		log.Print("vmlinux", vmlinux)
		if fs.PathExists(vmlinux) {
			ki.VmlinuxPath = vmlinux
		}

		kernels = append(kernels, ki)
	}

	return
}
