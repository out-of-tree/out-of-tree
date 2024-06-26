package ubuntu

import (
	"fmt"
	"strings"

	"code.dumpstack.io/tools/out-of-tree/config/dotfiles"
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
		"24.04",
	}

	for _, release := range releases {
		distro.Register(Ubuntu{release: release})
	}
}

type Ubuntu struct {
	release string
}

func (u Ubuntu) Equal(d distro.Distro) bool {
	return u.release == d.Release && distro.Ubuntu == d.ID
}

func (u Ubuntu) Distro() distro.Distro {
	return distro.Distro{ID: distro.Ubuntu, Release: u.release}
}

func (u Ubuntu) Packages() (pkgs []string, err error) {
	c, err := container.New(u.Distro())
	if err != nil {
		return
	}

	err = c.Build("ubuntu:"+u.release, u.envs(), u.runs())
	if err != nil {
		return
	}

	cmd := "apt-cache search " +
		"--names-only '^linux-image-[0-9\\.\\-]*-generic$' " +
		"| awk '{ print $1 }'"

	output, err := c.Run(dotfiles.Dir("tmp"), []string{cmd})
	if err != nil {
		return
	}

	pkgs = append(pkgs, strings.Fields(output)...)
	return
}

func (u Ubuntu) Kernels() (kernels []distro.KernelInfo, err error) {
	c, err := container.New(u.Distro())
	if err != nil {
		return
	}

	return c.Kernels()
}

func (u Ubuntu) envs() (envs []string) {
	envs = append(envs, "DEBIAN_FRONTEND=noninteractive")
	return
}

func (u Ubuntu) runs() (commands []string) {
	cmdf := func(f string, s ...interface{}) {
		commands = append(commands, fmt.Sprintf(f, s...))
	}

	if u.release < "14.04" {
		cmdf("sed -i 's/archive.ubuntu.com/old-releases.ubuntu.com/' " +
			"/etc/apt/sources.list")
	}

	cmdf("apt-get update")
	cmdf("apt-get install -y build-essential libelf-dev")
	cmdf("apt-get install -y wget git")

	if u.release == "12.04" {
		cmdf("apt-get install -y grub")
		cmdf("cp /bin/true /usr/sbin/grub-probe")
		cmdf("mkdir -p /boot/grub")
		cmdf("touch /boot/grub/menu.lst")
	}

	if u.release < "14.04" {
		return
	}

	if u.release == "22.04" {
		cmdf("apt-get install -y gcc-12")
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

func (u Ubuntu) Install(pkgname string, headers bool) (err error) {
	var headerspkg string
	if headers {
		headerspkg = strings.Replace(pkgname, "image", "headers", -1)
	}

	var commands []string
	cmdf := func(f string, s ...interface{}) {
		commands = append(commands, fmt.Sprintf(f, s...))
	}

	cmdf("apt-get install -y %s %s", pkgname, headerspkg)
	cmdf("cp -r /boot /target/")
	cmdf("cp -r /lib/modules /target/lib/")
	cmdf("cp -r /usr/src /target/usr/")

	c, err := container.New(u.Distro())
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

func (u Ubuntu) RootFS() string {
	return fmt.Sprintf("out_of_tree_ubuntu_%s.img",
		strings.Replace(u.release, ".", "__", -1))
}
