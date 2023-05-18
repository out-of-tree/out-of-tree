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

	"code.dumpstack.io/tools/out-of-tree/distro"

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
	Distro distro.Distro

	ReleaseMask string

	// Overrides ReleaseMask
	Kernel kernel
}

// DockerName is returns stable name for docker container
func (km KernelMask) DockerName() string {
	distro := strings.ToLower(km.Distro.ID.String())
	release := strings.Replace(km.Distro.Release, ".", "__", -1)
	return fmt.Sprintf("out_of_tree_%s_%s", distro, release)
}

// ArtifactType is the kernel module or exploit
type ArtifactType int

const (
	// KernelModule is any kind of kernel module
	KernelModule ArtifactType = iota
	// KernelExploit is the privilege escalation exploit
	KernelExploit
	// Script for information gathering or automation
	Script
)

func (at ArtifactType) String() string {
	return [...]string{"module", "exploit", "script"}[at]
}

// UnmarshalTOML is for support github.com/naoina/toml
func (at *ArtifactType) UnmarshalTOML(data []byte) (err error) {
	stype := strings.Trim(string(data), `"`)
	stypelower := strings.ToLower(stype)
	if strings.Contains(stypelower, "module") {
		*at = KernelModule
	} else if strings.Contains(stypelower, "exploit") {
		*at = KernelExploit
	} else if strings.Contains(stypelower, "script") {
		*at = Script
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
	case Script:
		s = "script"
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

// Extra test files to copy over
type FileTransfer struct {
	User   string
	Local  string
	Remote string
}

type Patch struct {
	Path   string
	Source string
	Script string
}

// Artifact is for .out-of-tree.toml
type Artifact struct {
	Name             string
	Type             ArtifactType
	TestFiles        []FileTransfer
	SourcePath       string
	Targets []KernelMask

	Script string

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

	Patches []Patch

	Make struct {
		Target string
	}

	StandardModules bool

	Preload []PreloadModule
}

func (ka Artifact) checkSupport(ki KernelInfo, km KernelMask) (
	supported bool, err error) {

	if ki.Distro.ID != km.Distro.ID {
		supported = false
		return
	}

	// DistroRelease is optional
	if km.Distro.Release != "" && ki.Distro.Release != km.Distro.Release {
		supported = false
		return
	}

	supported, err = regexp.MatchString(km.ReleaseMask, ki.KernelRelease)
	return
}

// Supported returns true if given kernel is supported by artifact
func (ka Artifact) Supported(ki KernelInfo) (supported bool, err error) {
	for _, km := range ka.Targets {
		supported, err = ka.checkSupport(ki, km)
		if supported {
			break
		}

	}
	return
}

// ByRootFS is sorting by .RootFS lexicographically
type ByRootFS []KernelInfo

func (a ByRootFS) Len() int           { return len(a) }
func (a ByRootFS) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByRootFS) Less(i, j int) bool { return a[i].RootFS < a[j].RootFS }

// KernelInfo defines kernels.toml entries
type KernelInfo struct {
	Distro distro.Distro

	// Must be *exactly* same as in `uname -r`
	KernelVersion string

	KernelRelease string

	// Build-time information
	KernelSource  string // module/exploit will be build on host
	ContainerName string

	// Runtime information
	KernelPath  string
	InitrdPath  string
	ModulesPath string

	RootFS string

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

	for i, _ := range ka.Targets {
		km := &ka.Targets[i]
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
