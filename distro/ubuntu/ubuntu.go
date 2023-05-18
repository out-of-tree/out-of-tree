package ubuntu

import (
	"fmt"

	"code.dumpstack.io/tools/out-of-tree/config"
)

func Envs(km config.KernelMask) (envs []string) {
	envs = append(envs, "DEBIAN_FRONTEND=noninteractive")
	return
}

func Runs(km config.KernelMask) (commands []string) {
	cmdf := func(f string, s ...interface{}) {
		commands = append(commands, fmt.Sprintf(f, s...))
	}

	if km.DistroRelease < "14.04" {
		cmdf("sed -i 's/archive.ubuntu.com/old-releases.ubuntu.com/' " +
			"/etc/apt/sources.list")
	}

	cmdf("apt-get update")
	cmdf("apt-get install -y build-essential libelf-dev")
	cmdf("apt-get install -y wget git")

	if km.DistroRelease < "14.04" {
		return
	}

	cmdf("apt-get install -y libseccomp-dev")

	// Install and remove a single kernel and headers.
	// This ensures that all dependencies are cached.

	cmdf("export TMP_HEADERS=$(apt-cache search " +
		"--names-only '^linux-headers-[0-9\\.\\-]*-generic' " +
		"| awk '{ print $1 }' | head -n 1)")

	cmdf("export TMP_KERNEL=$(echo $TMP_HEADERS | sed 's/headers/image/')")
	cmdf("export TMP_MODULES=$(echo $TMP_HEADERS | sed 's/headers/modules/')")

	cmdf("apt-get install -y $TMP_HEADERS $TMP_KERNEL $TMP_MODULES")
	cmdf("apt-get remove -y $TMP_HEADERS $TMP_KERNEL $TMP_MODULES")

	return
}
