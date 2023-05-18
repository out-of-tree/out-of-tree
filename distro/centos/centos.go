package centos

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

	var repos []string

	// TODO refactor
	switch km.DistroRelease {
	case "6":
		repofmt := "[6.%d-%s]\\nbaseurl=https://vault.centos.org/6.%d/%s/$basearch/\\ngpgcheck=0"
		for i := 0; i <= 10; i++ {
			repos = append(repos, fmt.Sprintf(repofmt, i, "os", i, "os"))
			repos = append(repos, fmt.Sprintf(repofmt, i, "updates", i, "updates"))
		}
		cmdf("rm /etc/yum.repos.d/*")
	case "7":
		repofmt := "[%s-%s]\\nbaseurl=https://vault.centos.org/%s/%s/$basearch/\\ngpgcheck=0"
		for _, ver := range []string{
			"7.0.1406", "7.1.1503", "7.2.1511",
			"7.3.1611", "7.4.1708", "7.5.1804",
			"7.6.1810", "7.7.1908", "7.8.2003",
		} {
			repos = append(repos, fmt.Sprintf(repofmt, ver, "os", ver, "os"))
			repos = append(repos, fmt.Sprintf(repofmt, ver, "updates", ver, "updates"))
		}

		// FIXME http/gpgcheck=0
		repofmt = "[%s-%s]\\nbaseurl=http://mirror.centos.org/centos-7/%s/%s/$basearch/\\ngpgcheck=0"
		repos = append(repos, fmt.Sprintf(repofmt, "7.9.2009", "os", "7.9.2009", "os"))
		repos = append(repos, fmt.Sprintf(repofmt, "7.9.2009", "updates", "7.9.2009", "updates"))
	case "8":
		repofmt := "[%s-%s]\\nbaseurl=https://vault.centos.org/%s/%s/$basearch/os/\\ngpgcheck=0"

		for _, ver := range []string{
			"8.0.1905", "8.1.1911", "8.2.2004",
			"8.3.2011", "8.4.2105", "8.5.2111",
		} {
			repos = append(repos, fmt.Sprintf(repofmt, ver, "baseos", ver, "BaseOS"))
			repos = append(repos, fmt.Sprintf(repofmt, ver, "appstream", ver, "AppStream"))
		}
	default:
		log.Fatal().Msgf("no support for %s %s", km.DistroType, km.DistroRelease)
		return
	}

	cmdf("sed -i 's/enabled=1/enabled=0/' /etc/yum.repos.d/* || true")

	for _, repo := range repos {
		cmdf("echo -e '%s' >> /etc/yum.repos.d/oot.repo\n", repo)
	}

	// do not remove old kernels

	cmdf("sed -i 's;installonly_limit=;installonly_limit=100500;' /etc/yum.conf")
	cmdf("yum -y update")

	cmdf("yum -y groupinstall 'Development Tools'")

	if km.DistroRelease < "8" {
		cmdf("yum -y install deltarpm")
	} else {
		cmdf("yum -y install grub2-tools-minimal elfutils-libelf-devel")
	}

	var flags string
	if km.DistroRelease >= "8" {
		flags = "--noautoremove"
	}

	// Install and remove a single kernel and headers.
	// This ensures that all dependencies are cached.

	cmdf("export TMP_HEADERS=$(yum search kernel-devel --showduplicates " +
		"| grep '^kernel-devel' | cut -d ' ' -f 1 | head -n 1)")

	cmdf("export TMP_KERNEL=$(echo $TMP_HEADERS | sed 's/-devel//')")
	cmdf("export TMP_MODULES=$(echo $TMP_HEADERS | sed 's/-devel/-modules/')")
	cmdf("export TMP_CORE=$(echo $TMP_HEADERS | sed 's/-devel/-core/')")

	cmdf("yum -y install $TMP_KERNEL $TMP_HEADERS")
	cmdf("yum -y remove %s $TMP_KERNEL $TMP_HEADERS $TMP_MODULES $TMP_CORE",
		flags)

	return
}
