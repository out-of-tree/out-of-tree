package ubuntu

import (
	"fmt"
	"strings"
	"time"

	"code.dumpstack.io/tools/out-of-tree/config"
	"code.dumpstack.io/tools/out-of-tree/container"
	"code.dumpstack.io/tools/out-of-tree/distro"
)

func init() {
	releases := []string{
		"12.04",
		"14.04",
		"16.04",
		"18.04",
		"20.04",
		"22.04",
	}

	for _, release := range releases {
		container := "out_of_tree_ubuntu_" + release
		container = strings.Replace(container, ".", "__", -1)

		distro.Register(Ubuntu{
			release:   release,
			container: container,
		})
	}
}

type Ubuntu struct {
	release   string
	container string
}

func (u Ubuntu) ID() distro.ID {
	return distro.Ubuntu
}

func (u Ubuntu) Release() string {
	return u.release
}

func (u Ubuntu) Equal(d distro.Distro) bool {
	return u.release == d.Release && distro.Ubuntu == d.ID
}

func (u Ubuntu) Packages() (pkgs []string, err error) {
	c, err := container.New(u.container, time.Hour)
	if err != nil {
		return
	}

	cmd := "apt-cache search " +
		"--names-only '^linux-image-[0-9\\.\\-]*-generic' " +
		"| awk '{ print $1 }'"

	output, err := c.Run(config.Dir("tmp"), cmd)
	if err != nil {
		return
	}

	for _, pkg := range strings.Fields(output) {
		pkgs = append(pkgs, pkg)
	}

	return
}

func Envs(km config.Target) (envs []string) {
	envs = append(envs, "DEBIAN_FRONTEND=noninteractive")
	return
}

func Runs(km config.Target) (commands []string) {
	cmdf := func(f string, s ...interface{}) {
		commands = append(commands, fmt.Sprintf(f, s...))
	}

	if km.Distro.Release < "14.04" {
		cmdf("sed -i 's/archive.ubuntu.com/old-releases.ubuntu.com/' " +
			"/etc/apt/sources.list")
	}

	cmdf("apt-get update")
	cmdf("apt-get install -y build-essential libelf-dev")
	cmdf("apt-get install -y wget git")

	if km.Distro.Release == "12.04" {
		cmdf("apt-get install -y grub")
		cmdf("cp /bin/true /usr/sbin/grub-probe")
		cmdf("mkdir -p /boot/grub")
		cmdf("touch /boot/grub/menu.lst")
	}

	if km.Distro.Release < "14.04" {
		return
	}

	cmdf("apt-get install -y libseccomp-dev")

	// Install and remove a single kernel and headers.
	// This ensures that all dependencies are cached.

	cmd := "export HEADERS=$(apt-cache search " +
		"--names-only '^linux-headers-[0-9\\.\\-]*-generic' " +
		"| awk '{ print $1 }' | head -n 1)"

	cmd += " KERNEL=$(echo $HEADERS | sed 's/headers/image/')"
	cmd += " MODULES=$(echo $HEADERS | sed 's/headers/modules/')"

	cmd += " && apt-get install -y $HEADERS $KERNEL $MODULES"
	cmd += " && apt-get remove -y $HEADERS $KERNEL $MODULES"

	cmdf(cmd)

	return
}

func Install(km config.Target, pkgname string, headers bool) (commands []string, err error) {

	var headerspkg string
	if headers {
		headerspkg = strings.Replace(pkgname, "image", "headers", -1)
	}

	cmdf := func(f string, s ...interface{}) {
		commands = append(commands, fmt.Sprintf(f, s...))
	}

	cmdf("apt-get install -y %s %s", pkgname, headerspkg)

	return
}

func Cleanup(km config.Target, pkgname string) {
	return
}
