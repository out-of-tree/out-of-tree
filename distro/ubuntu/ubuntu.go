package ubuntu

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"code.dumpstack.io/tools/out-of-tree/config"
	"code.dumpstack.io/tools/out-of-tree/container"
)

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

func Match(km config.Target) (pkgs []string, err error) {
	// FIXME timeout should be in global out-of-tree config
	c, err := container.New(km.DockerName(), time.Hour)
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

	r, err := regexp.Compile("linux-image-" + km.ReleaseMask)
	if err != nil {
		return
	}

	for _, pkg := range strings.Fields(output) {
		if r.MatchString(pkg) || strings.Contains(pkg, km.ReleaseMask) {
			pkgs = append(pkgs, pkg)
		}
	}

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
