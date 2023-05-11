package debian

import (
	"errors"
	"strings"

	"code.dumpstack.io/tools/out-of-tree/distro/debian/snapshot"
)

type DebianKernelVersion struct {
	// linux-headers-4.17.0-2-amd64_4.17.14-1_amd64.deb

	// Package version, e.g. "4.17.14-1"
	// See tags in https://salsa.debian.org/kernel-team/linux
	Package string

	// ABI version, e.g. "4.17.0-2"
	ABI string
}

type DebianKernel struct {
	Version DebianKernelVersion
	Image   snapshot.Package
	Headers snapshot.Package
}

func GetDebianKernel(version string) (dk DebianKernel, err error) {
	dk.Version.Package = version

	regex := `^linux-(image|headers)-[0-9\.\-]*-(amd64|amd64-unsigned)$`

	packages, err := snapshot.Packages("linux", version, "amd64", regex)
	if len(packages) != 2 {
		err = errors.New("len(packages) != 2")
		return
	}

	var imageFound, headersFound bool
	for _, p := range packages {
		if strings.Contains(p.Name, "image") {
			imageFound = true
			dk.Image = p
		} else if strings.Contains(p.Name, "headers") {
			headersFound = true
			dk.Headers = p
		}
	}

	if !imageFound {
		err = errors.New("image not found")
		return
	}

	if !headersFound {
		err = errors.New("headers not found")
		return
	}

	s := strings.Replace(dk.Headers.Name, "linux-headers-", "", -1)
	dk.Version.ABI = strings.Replace(s, "-amd64", "", -1)

	return
}
