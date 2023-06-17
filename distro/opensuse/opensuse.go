package opensuse

import (
	"fmt"
	"strings"

	"code.dumpstack.io/tools/out-of-tree/cache"
	"code.dumpstack.io/tools/out-of-tree/container"
	"code.dumpstack.io/tools/out-of-tree/distro"
)

func init() {
	releases := []string{
		"13.1", "13.2",
		"42.1", "42.2", "42.3",
		"15.0", "15.1", "15.2", "15.3", "15.4", "15.5",
	}

	for _, release := range releases {
		distro.Register(OpenSUSE{release: release})
	}
}

type OpenSUSE struct {
	release string
}

func (suse OpenSUSE) Equal(d distro.Distro) bool {
	return suse.release == d.Release && distro.OpenSUSE == d.ID
}

func (suse OpenSUSE) Distro() distro.Distro {
	return distro.Distro{ID: distro.OpenSUSE, Release: suse.release}
}

func (suse OpenSUSE) Packages() (pkgs []string, err error) {
	c, err := container.New(suse.Distro())
	if err != nil {
		return
	}

	var name string
	if strings.HasPrefix(suse.release, "13") {
		name = "opensuse:13"
		cnturl := cache.ContainerURL("openSUSE-13.2")
		err = container.Import(cnturl, name)
		if err != nil {
			return
		}
	} else if strings.HasPrefix(suse.release, "42") {
		name = "opensuse/leap:42"
	} else if strings.HasPrefix(suse.release, "15") {
		name = "opensuse/leap:" + suse.release
	}

	err = c.Build(name, suse.envs(), suse.runs())
	if err != nil {
		return
	}

	cmd := "zypper search -s --match-exact kernel-default | grep x86_64 " +
		"| cut -d '|' -f 4 | sed 's/ //g'"

	output, err := c.Run("", []string{cmd})
	if err != nil {
		return
	}

	for _, pkg := range strings.Fields(output) {
		pkgs = append(pkgs, pkg)
	}

	return
}

func (suse OpenSUSE) Kernels() (kernels []distro.KernelInfo, err error) {
	c, err := container.New(suse.Distro())
	if err != nil {
		return
	}

	return c.Kernels()
}

func (suse OpenSUSE) envs() (envs []string) {
	return
}

func (suse OpenSUSE) runs() (commands []string) {
	cmdf := func(f string, s ...interface{}) {
		commands = append(commands, fmt.Sprintf(f, s...))
	}

	main := "http://download.opensuse.org/"
	discontinued := "http://ftp.gwdg.de/pub/opensuse/discontinued/"

	var repourls []string

	if strings.HasPrefix(suse.release, "13") {
		dist := discontinued + "distribution/%s/repo/oss/"
		update := discontinued + "update/%s/"
		repourls = append(repourls,
			fmt.Sprintf(dist, suse.release),
			fmt.Sprintf(update, suse.release),
		)
	} else if strings.HasPrefix(suse.release, "42") {
		dist := discontinued + "distribution/leap/%s/repo/oss/suse/"
		update := discontinued + "update/leap/%s/oss/"
		repourls = append(repourls,
			fmt.Sprintf(dist, suse.release),
			fmt.Sprintf(update, suse.release),
		)
	} else if strings.HasPrefix(suse.release, "15") {
		dist := main + "distribution/leap/%s/repo/oss/"
		update := main + "update/leap/%s/oss/"
		repourls = append(repourls,
			fmt.Sprintf(dist, suse.release),
			fmt.Sprintf(update, suse.release),
		)

		switch suse.release {
		case "15.3", "15.4", "15.5":
			sle := main + "update/leap/%s/sle/"
			repourls = append(repourls,
				fmt.Sprintf(sle, suse.release),
			)
		}
	}

	cmdf("rm /etc/zypp/repos.d/*")

	for i, repourl := range repourls {
		cmdf(`echo -e `+
			`"[%d]\n`+
			`name=%d\n`+
			`enabled=1\n`+
			`autorefresh=0\n`+
			`gpgcheck=0\n`+
			`baseurl=%s" > /etc/zypp/repos.d/%d.repo`,
			i, i, repourl, i,
		)
	}

	cmdf("zypper -n refresh")

	params := "--no-recommends --force-resolution --replacefiles"

	cmdf("zypper -n update %s", params)

	cmdf("zypper --no-refresh -n install %s -t pattern devel_kernel", params)

	// Cache dependencies
	cmdf("zypper -n install %s kernel-default kernel-default-devel "+
		"&& zypper -n remove -U kernel-default kernel-default-devel",
		params)

	cmdf("zypper --no-refresh -n install %s kmod which", params)

	if strings.HasPrefix(suse.release, "13") {
		cmdf("zypper --no-refresh -n install %s kernel-firmware", params)
	}
	return
}

func (suse OpenSUSE) Install(version string, headers bool) (err error) {
	var commands []string
	cmdf := func(f string, s ...interface{}) {
		commands = append(commands, fmt.Sprintf(f, s...))
	}

	installcmd := "zypper --no-refresh -n install " +
		"--replacefiles --no-recommends --force-resolution --capability"
	cmdf("%s kernel-default=%s", installcmd, version)
	if headers {
		cmdf("%s kernel-default-devel=%s", installcmd, version)
	}

	cmdf("mkdir /usr/lib/dracut/modules.d/42workaround")
	wsetuppath := "/usr/lib/dracut/modules.d/42workaround/module-setup.sh"

	cmdf("echo 'check() { return 0; }' >> %s", wsetuppath)
	cmdf("echo 'depends() { return 0; }' >> %s", wsetuppath)
	cmdf(`echo 'install() { `+
		`inst_hook pre-mount 91 "$moddir/workaround.sh"; `+
		`}' >> %s`, wsetuppath)
	cmdf("echo 'installkernel() { instmods af_packet; }' >> %s", wsetuppath)

	wpath := "/usr/lib/dracut/modules.d/42workaround/workaround.sh"

	cmdf("echo '#!/bin/sh' >> %s", wpath)
	cmdf("echo 'modprobe af_packet' >> %s", wpath)

	cmdf("dracut " +
		"-a workaround " +
		"--add-drivers 'ata_piix libata' " +
		"--force-drivers 'e1000 ext4 sd_mod rfkill af_packet' " +
		"-f /boot/initrd-$(ls /lib/modules) $(ls /lib/modules)")

	cmdf("cp -r /boot /target/")
	cmdf("cp -r /lib/modules /target/lib/")
	cmdf("cp -r /usr/src /target/usr/")

	c, err := container.New(suse.Distro())
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

func (suse OpenSUSE) RootFS() string {
	return fmt.Sprintf("out_of_tree_opensuse_%s.img",
		strings.Split(suse.release, ".")[0])
}
