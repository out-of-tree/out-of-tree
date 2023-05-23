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

func Install(km config.Target, pkgname string, headers bool) (commands []string, err error) {
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

	if km.Distro.Release <= "7" {
		cmdf("dracut -v --add-drivers 'e1000 ext4' -f "+
			"/boot/initramfs-%s.img %s", version, version)
	} else {
		cmdf("dracut -v --add-drivers 'ata_piix libata' "+
			"--force-drivers 'e1000 ext4 sd_mod' -f "+
			"/boot/initramfs-%s.img %s", version, version)
	}

	return
}

func Cleanup(km config.Target, pkgname string) {
	return
}
