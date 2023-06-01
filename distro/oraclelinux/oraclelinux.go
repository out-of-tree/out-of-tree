package oraclelinux

import (
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"

	"code.dumpstack.io/tools/out-of-tree/config"
	"code.dumpstack.io/tools/out-of-tree/container"
	"code.dumpstack.io/tools/out-of-tree/distro"
)

func init() {
	releases := []string{"6", "7", "8", "9"}

	for _, release := range releases {
		distro.Register(OracleLinux{release: release})
	}
}

type OracleLinux struct {
	release string
}

func (ol OracleLinux) Equal(d distro.Distro) bool {
	return ol.release == d.Release && distro.OracleLinux == d.ID
}

func (ol OracleLinux) Distro() distro.Distro {
	return distro.Distro{ID: distro.OracleLinux, Release: ol.release}
}

func (ol OracleLinux) Packages() (pkgs []string, err error) {
	c, err := container.New(ol.Distro())
	if err != nil {
		return
	}

	err = c.Build("oraclelinux:"+ol.release, ol.envs(), ol.runs())
	if err != nil {
		return
	}

	if ol.release == "8" {
		// Image for ol9 is required for some kernels
		// See notes in OracleLinux.Kernels()
		_, err = OracleLinux{release: "9"}.Packages()
		if err != nil {
			return
		}
	}

	cmd := "yum search kernel --showduplicates 2>/dev/null " +
		"| grep '^kernel-[0-9]\\|^kernel-uek-[0-9]' " +
		"| grep -v src " +
		"| cut -d ' ' -f 1"

	output, err := c.Run(config.Dir("tmp"), []string{cmd})
	if err != nil {
		return
	}

	for _, pkg := range strings.Fields(output) {
		pkgs = append(pkgs, pkg)
	}

	return
}

func (ol OracleLinux) Kernels() (kernels []distro.KernelInfo, err error) {
	c, err := container.New(ol.Distro())
	if err != nil {
		return
	}

	kernels, err = c.Kernels()
	if err != nil {
		return
	}

	// Some kernels do not work with the smap enabled
	//
	// BUG: unable to handle kernel paging request at 00007fffc64b2fda
	// IP: [<ffffffff8127a9ed>] strnlen+0xd/0x40"
	// ...
	// Call Trace:
	//  [<ffffffff81123bf8>] dtrace_psinfo_alloc+0x138/0x390
	//  [<ffffffff8118b143>] do_execve_common.isra.24+0x3c3/0x460
	//  [<ffffffff81554d70>] ? rest_init+0x80/0x80
	//  [<ffffffff8118b1f8>] do_execve+0x18/0x20
	//  [<ffffffff81554dc2>] kernel_init+0x52/0x180
	//  [<ffffffff8157cd2c>] ret_from_fork+0x7c/0xb0
	//
	smapBlocklist := []string{
		"3.8.13-16",
		"3.8.13-26",
		"3.8.13-35",
		"3.8.13-44",
		"3.8.13-55",
		"3.8.13-68",
		"3.8.13-98",
	}

	for i, k := range kernels {
		// The latest uek kernels require gcc-11, which is
		// only present in el8 with scl load, so not so
		// convinient. It is possible to just build from
		// the next release container.
		if strings.Contains(k.KernelVersion, "5.15.0") {
			cnt := strings.Replace(k.ContainerName, "8", "9", -1)
			kernels[i].ContainerName = cnt
		}

		for _, ver := range smapBlocklist {
			if strings.Contains(k.KernelVersion, ver) {
				kernels[i].CPU.Flags = append(
					kernels[i].CPU.Flags, "smap=off",
				)
			}
		}
	}

	return
}

func (ol OracleLinux) envs() (envs []string) {
	return
}

func (ol OracleLinux) runs() (commands []string) {
	cmdf := func(f string, s ...interface{}) {
		commands = append(commands, fmt.Sprintf(f, s...))
	}

	if ol.release < "6" {
		log.Fatal().Msgf("no support for pre-EL6")
	}

	cmdf("sed -i 's/enabled=0/enabled=1/' /etc/yum.repos.d/*")
	cmdf("sed -i 's;installonly_limit=;installonly_limit=100500;' /etc/yum.conf /etc/dnf/dnf.conf || true")
	cmdf("yum -y update")
	cmdf("yum -y groupinstall 'Development Tools'")

	packages := "linux-firmware grubby"
	if ol.release <= "7" {
		packages += " libdtrace-ctf"
	}

	cmdf("yum -y install %s", packages)

	return
}

func (ol OracleLinux) Install(pkgname string, headers bool) (err error) {
	var headerspkg string
	if headers {
		if strings.Contains(pkgname, "uek") {
			headerspkg = strings.Replace(pkgname,
				"kernel-uek", "kernel-uek-devel", -1)
		} else {
			headerspkg = strings.Replace(pkgname,
				"kernel", "kernel-devel", -1)
		}
	}

	var commands []string
	cmdf := func(f string, s ...interface{}) {
		commands = append(commands, fmt.Sprintf(f, s...))
	}

	cmdf("yum -y install %s %s", pkgname, headerspkg)

	var version string
	if strings.Contains(pkgname, "uek") {
		version = strings.Replace(pkgname, "kernel-uek-", "", -1)
	} else {
		version = strings.Replace(pkgname, "kernel-", "", -1)
	}

	if ol.release <= "7" {
		cmdf("dracut -v --add-drivers 'e1000 ext4' -f "+
			"/boot/initramfs-%s.img %s", version, version)
	} else {
		cmdf("dracut -v --add-drivers 'ata_piix libata' "+
			"--force-drivers 'e1000 ext4 sd_mod' -f "+
			"/boot/initramfs-%s.img %s", version, version)
	}

	cmdf("cp -r /boot /target/")
	cmdf("cp -r /lib/modules /target/lib/")
	cmdf("cp -r /usr/src /target/usr/")

	c, err := container.New(ol.Distro())
	if err != nil {
		return
	}

	for i := range c.Volumes {
		c.Volumes[i].Dest = "/target" + c.Volumes[i].Dest
	}

	_, err = c.Run("", commands)
	if err != nil {
		return
	}

	return
}
