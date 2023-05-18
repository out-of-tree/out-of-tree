package oraclelinux

import (
	"fmt"

	"github.com/rs/zerolog/log"

	"code.dumpstack.io/tools/out-of-tree/config"
)

func Envs(km config.KernelMask) (envs []string) {
	return
}

func Runs(km config.KernelMask) (commands []string) {
	cmdf := func(f string, s ...interface{}) {
		commands = append(commands, fmt.Sprintf(f, s...))
	}

	if sk.DistroRelease < "6" {
		log.Fatal().Msgf("no support for pre-EL6")
	}

	cmdf("sed -i 's/enabled=0/enabled=1/' /etc/yum.repos.d/*")
	cmdf("sed -i 's;installonly_limit=;installonly_limit=100500;' /etc/yum.conf /etc/dnf/dnf.conf || true")
	cmdf("yum -y update")
	cmdf("yum -y groupinstall 'Development Tools'")

	packages := "linux-firmware grubby"
	if sk.DistroRelease <= "7" {
		packages += " libdtrace-ctf"
	}

	cmdf("yum -y install %s", packages)

	return
}
