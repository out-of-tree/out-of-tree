// Copyright 2018 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package config

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/naoina/toml"
)

type kernel struct {
	Version []int
	Major   []int
	Minor   []int
	Patch   []int
}

// KernelMask defines the kernel
type KernelMask struct {
	DistroType    DistroType
	DistroRelease string // 18.04/7.4.1708/9.1
	ReleaseMask   string

	// Overrides ReleaseMask
	Kernel kernel
}

// DockerName is returns stable name for docker container
func (km KernelMask) DockerName() string {
	distro := strings.ToLower(km.DistroType.String())
	release := strings.Replace(km.DistroRelease, ".", "__", -1)
	return fmt.Sprintf("out_of_tree_%s_%s", distro, release)
}

// ArtifactType is the kernel module or exploit
type ArtifactType int

const (
	// KernelModule is any kind of kernel module
	KernelModule ArtifactType = iota
	// KernelExploit is the privilege escalation exploit
	KernelExploit
)

func (at ArtifactType) String() string {
	return [...]string{"module", "exploit"}[at]
}

// UnmarshalTOML is for support github.com/naoina/toml
func (at *ArtifactType) UnmarshalTOML(data []byte) (err error) {
	stype := strings.Trim(string(data), `"`)
	stypelower := strings.ToLower(stype)
	if strings.Contains(stypelower, "module") {
		*at = KernelModule
	} else if strings.Contains(stypelower, "exploit") {
		*at = KernelExploit
	} else {
		err = fmt.Errorf("Type %s is unsupported", stype)
	}
	return
}

// MarshalTOML is for support github.com/naoina/toml
func (at ArtifactType) MarshalTOML() (data []byte, err error) {
	s := ""
	switch at {
	case KernelModule:
		s = "module"
	case KernelExploit:
		s = "exploit"
	default:
		err = fmt.Errorf("Cannot marshal %d", at)
	}
	data = []byte(`"` + s + `"`)
	return
}

// Duration type with toml unmarshalling support
type Duration struct {
	time.Duration
}

// UnmarshalTOML for Duration
func (d *Duration) UnmarshalTOML(data []byte) (err error) {
	duration := strings.Replace(string(data), "\"", "", -1)
	d.Duration, err = time.ParseDuration(duration)
	return
}

// MarshalTOML for Duration
func (d Duration) MarshalTOML() (data []byte, err error) {
	data = []byte(`"` + d.Duration.String() + `"`)
	return
}

type PreloadModule struct {
	Repo             string
	Path             string
	TimeoutAfterLoad Duration
}

// Artifact is for .out-of-tree.toml
type Artifact struct {
	Name             string
	Type             ArtifactType
	SourcePath       string
	SupportedKernels []KernelMask

	Qemu struct {
		Cpus    int
		Memory  int
		Timeout Duration
	}

	Docker struct {
		Timeout Duration
	}

	Mitigations struct {
		DisableSmep  bool
		DisableSmap  bool
		DisableKaslr bool
		DisableKpti  bool
	}

	Preload []PreloadModule
}

func (ka Artifact) checkSupport(ki KernelInfo, km KernelMask) (
	supported bool, err error) {

	if ki.DistroType != km.DistroType {
		supported = false
		return
	}

	// DistroRelease is optional
	if km.DistroRelease != "" && ki.DistroRelease != km.DistroRelease {
		supported = false
		return
	}

	supported, err = regexp.MatchString(km.ReleaseMask, ki.KernelRelease)
	return
}

// Supported returns true if given kernel is supported by artifact
func (ka Artifact) Supported(ki KernelInfo) (supported bool, err error) {
	for _, km := range ka.SupportedKernels {
		supported, err = ka.checkSupport(ki, km)
		if supported {
			break
		}

	}
	return
}

// DistroType is enum with all supported distros
type DistroType int

const (
	// Ubuntu https://ubuntu.com/
	Ubuntu DistroType = iota
	// CentOS https://www.centos.org/
	CentOS
	// Debian https://www.debian.org/
	Debian
)

// DistroTypeStrings is the string version of enum DistroType
var DistroTypeStrings = [...]string{"Ubuntu", "CentOS", "Debian"}

// NewDistroType is create new Distro object
func NewDistroType(dType string) (dt DistroType, err error) {
	err = dt.UnmarshalTOML([]byte(dType))
	return
}

func (dt DistroType) String() string {
	return DistroTypeStrings[dt]
}

// UnmarshalTOML is for support github.com/naoina/toml
func (dt *DistroType) UnmarshalTOML(data []byte) (err error) {
	sDistro := strings.Trim(string(data), `"`)
	if strings.EqualFold(sDistro, "Ubuntu") {
		*dt = Ubuntu
	} else if strings.EqualFold(sDistro, "CentOS") {
		*dt = CentOS
	} else if strings.EqualFold(sDistro, "Debian") {
		*dt = Debian
	} else {
		err = fmt.Errorf("Distro %s is unsupported", sDistro)
	}
	return
}

// MarshalTOML is for support github.com/naoina/toml
func (dt DistroType) MarshalTOML() (data []byte, err error) {
	s := ""
	switch dt {
	case Ubuntu:
		s = "Ubuntu"
	case CentOS:
		s = "CentOS"
	case Debian:
		s = "Debian"
	default:
		err = fmt.Errorf("Cannot marshal %d", dt)
	}
	data = []byte(`"` + s + `"`)
	return
}

// ByRootFS is sorting by .RootFS lexicographically
type ByRootFS []KernelInfo

func (a ByRootFS) Len() int           { return len(a) }
func (a ByRootFS) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByRootFS) Less(i, j int) bool { return a[i].RootFS < a[j].RootFS }

// KernelInfo defines kernels.toml entries
type KernelInfo struct {
	DistroType    DistroType
	DistroRelease string // 18.04/7.4.1708/9.1

	// Must be *exactly* same as in `uname -r`
	KernelRelease string

	// Build-time information
	KernelSource  string // module/exploit will be build on host
	ContainerName string

	// Runtime information
	KernelPath string
	InitrdPath string
	RootFS     string

	// Debug symbols
	VmlinuxPath string
}

// KernelConfig is the ~/.out-of-tree/kernels.toml configuration description
type KernelConfig struct {
	Kernels []KernelInfo
}

func readFileAll(path string) (buf []byte, err error) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	buf, err = ioutil.ReadAll(f)
	return
}

// ReadKernelConfig is for read kernels.toml
func ReadKernelConfig(path string) (kernelCfg KernelConfig, err error) {
	buf, err := readFileAll(path)
	if err != nil {
		return
	}

	err = toml.Unmarshal(buf, &kernelCfg)
	if err != nil {
		return
	}

	return
}

func rangeRegexp(start, end int) (s string) {
	s += "("
	for i := start; i <= end; i++ {
		s += strconv.Itoa(i)
		if i != end {
			s += "|"
		}
	}
	s += ")"
	return
}

func versionRegexp(l []int) (s string, err error) {
	switch len(l) {
	case 1:
		s += strconv.Itoa(l[0])
	case 2:
		s += rangeRegexp(l[0], l[1])
	default:
		err = errors.New("version must contain one value or range")
		return
	}
	return
}

func genReleaseMask(km kernel) (mask string, err error) {
	s, err := versionRegexp(km.Version)
	if err != nil {
		return
	}
	mask += s + "[.]"

	s, err = versionRegexp(km.Major)
	if err != nil {
		return
	}
	mask += s + "[.]"

	s, err = versionRegexp(km.Minor)
	if err != nil {
		return
	}
	mask += s

	switch len(km.Patch) {
	case 0:
		// ok
	case 1:
		mask += "-" + strconv.Itoa(km.Patch[0]) + "-"
	case 2:
		mask += "-" + rangeRegexp(km.Patch[0], km.Patch[1]) + "-"
	default:
		err = errors.New("version must contain one value or range")
		return
	}

	mask += ".*"
	return
}

// ReadArtifactConfig is for read .out-of-tree.toml
func ReadArtifactConfig(path string) (ka Artifact, err error) {
	buf, err := readFileAll(path)
	if err != nil {
		return
	}

	err = toml.Unmarshal(buf, &ka)
	if err != nil {
		return
	}

	for i, _ := range ka.SupportedKernels {
		km := &ka.SupportedKernels[i]
		if len(km.Kernel.Version) != 0 && km.ReleaseMask != "" {
			s := "Only one way to define kernel version is allowed"
			err = errors.New(s)
			return
		}

		if km.ReleaseMask == "" {
			km.ReleaseMask, err = genReleaseMask(km.Kernel)
			if err != nil {
				return
			}
		}
	}

	return
}
