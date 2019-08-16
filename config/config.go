// Copyright 2018 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package config

import (
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strings"

	"github.com/naoina/toml"
)

type KernelMask struct {
	DistroType    DistroType
	DistroRelease string // 18.04/7.4.1708/9.1
	ReleaseMask   string
}

func (km KernelMask) DockerName() string {
	distro := strings.ToLower(km.DistroType.String())
	release := strings.Replace(km.DistroRelease, ".", "__", -1)
	return fmt.Sprintf("out_of_tree_%s_%s", distro, release)
}

type ArtifactType int

const (
	KernelModule ArtifactType = iota
	KernelExploit
)

func (at ArtifactType) String() string {
	return [...]string{"module", "exploit"}[at]
}

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

type Artifact struct {
	Name             string
	Type             ArtifactType
	SourcePath       string
	SupportedKernels []KernelMask
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

func (ka Artifact) Supported(ki KernelInfo) (supported bool, err error) {
	for _, km := range ka.SupportedKernels {
		supported, err = ka.checkSupport(ki, km)
		if supported {
			break
		}

	}
	return
}

type DistroType int

const (
	Ubuntu DistroType = iota
	CentOS
	Debian
)

var DistroTypeStrings = [...]string{"Ubuntu", "CentOS", "Debian"}

func NewDistroType(dType string) (dt DistroType, err error) {
	err = dt.UnmarshalTOML([]byte(dType))
	return
}

func (dt DistroType) String() string {
	return DistroTypeStrings[dt]
}

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

type ByRootFS []KernelInfo

func (a ByRootFS) Len() int           { return len(a) }
func (a ByRootFS) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByRootFS) Less(i, j int) bool { return a[i].RootFS < a[j].RootFS }

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

func ReadArtifactConfig(path string) (artifactCfg Artifact, err error) {
	buf, err := readFileAll(path)
	if err != nil {
		return
	}

	err = toml.Unmarshal(buf, &artifactCfg)
	if err != nil {
		return
	}

	return
}
