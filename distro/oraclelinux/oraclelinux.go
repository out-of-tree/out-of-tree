package oraclelinux

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"code.dumpstack.io/tools/out-of-tree/config"
	"code.dumpstack.io/tools/out-of-tree/container"
)

func Envs(km config.KernelMask) (envs []string) {
	return
}

func Runs(km config.KernelMask) (commands []string) {
	cmdf := func(f string, s ...interface{}) {
		commands = append(commands, fmt.Sprintf(f, s...))
	}

	if km.DistroRelease < "6" {
		log.Fatal().Msgf("no support for pre-EL6")
	}

	cmdf("sed -i 's/enabled=0/enabled=1/' /etc/yum.repos.d/*")
	cmdf("sed -i 's;installonly_limit=;installonly_limit=100500;' /etc/yum.conf /etc/dnf/dnf.conf || true")
	cmdf("yum -y update")
	cmdf("yum -y groupinstall 'Development Tools'")

	packages := "linux-firmware grubby"
	if km.DistroRelease <= "7" {
		packages += " libdtrace-ctf"
	}

	cmdf("yum -y install %s", packages)

	return
}

func Match(km config.KernelMask) (pkgs []string, err error) {
	// FIXME timeout should be in global out-of-tree config
	c, err := container.New(km.DockerName(), time.Hour)
	if err != nil {
		return
	}

	cmd := "yum search kernel --showduplicates " +
		"| grep '^kernel-[0-9]\\|^kernel-uek-[0-9]' " +
		"| grep -v src " +
		"| cut -d ' ' -f 1"

	output, err := c.Run(config.Dir("tmp"), cmd)
	if err != nil {
		return
	}

	r, err := regexp.Compile("kernel-" + km.ReleaseMask)
	if err != nil {
		return
	}

	for _, pkg := range strings.Fields(output) {
		if r.MatchString(pkg) || strings.Contains(pkg, km.ReleaseMask) {
			log.Trace().Msg(pkg)
			pkgs = append(pkgs, pkg)
		}
	}

	if len(pkgs) == 0 {
		log.Warn().Msg("no packages matched")
	}

	return
}
