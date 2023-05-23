package centos

import (
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"

	"code.dumpstack.io/tools/out-of-tree/config"
	"code.dumpstack.io/tools/out-of-tree/container"
	"code.dumpstack.io/tools/out-of-tree/distro"
)

func init() {
	releases := []string{"6", "7", "8"}

	for _, release := range releases {
		distro.Register(CentOS{release: release})
	}
}

type CentOS struct {
	release string
}

func (centos CentOS) Equal(d distro.Distro) bool {
	return centos.release == d.Release && distro.CentOS == d.ID
}

func (centos CentOS) Distro() distro.Distro {
	return distro.Distro{ID: distro.CentOS, Release: centos.release}
}

func (centos CentOS) Packages() (pkgs []string, err error) {
	c, err := container.New(centos.Distro())
	if err != nil {
		return
	}

	err = c.Build("centos:"+centos.release, centos.envs(), centos.runs())
	if err != nil {
		return
	}

	cmd := "yum search kernel --showduplicates 2>/dev/null " +
		"| grep '^kernel-[0-9]' " +
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

func (centos CentOS) Kernels() (kernels []distro.KernelInfo, err error) {
	c, err := container.New(centos.Distro())
	if err != nil {
		return
	}

	return c.Kernels()
}

func (centos CentOS) envs() (envs []string) {
	return
}

func (centos CentOS) runs() (commands []string) {
	cmdf := func(f string, s ...interface{}) {
		commands = append(commands, fmt.Sprintf(f, s...))
	}

	var repos []string

	// TODO refactor
	switch centos.release {
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
		log.Fatal().Msgf("no support for centos %s", centos.release)
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

	if centos.release < "8" {
		cmdf("yum -y install deltarpm")
	} else {
		cmdf("yum -y install grub2-tools-minimal elfutils-libelf-devel")
	}

	var flags string
	if centos.release >= "8" {
		flags = "--noautoremove"
	}

	// Install and remove a single kernel and headers.
	// This ensures that all dependencies are cached.

	cmd := "export HEADERS=$(yum search kernel-devel --showduplicates " +
		"| grep '^kernel-devel' | cut -d ' ' -f 1 | head -n 1)"

	cmd += " KERNEL=$(echo $HEADERS | sed 's/-devel//')"
	cmd += " MODULES=$(echo $HEADERS | sed 's/-devel/-modules/')"
	cmd += " CORE=$(echo $HEADERS | sed 's/-devel/-core/')"

	cmd += " && yum -y install $KERNEL $HEADERS"
	cmd += " && yum -y remove %s $KERNEL $HEADERS $MODULES $CORE"

	cmdf(cmd, flags)

	return
}
