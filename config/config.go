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
	"time"

	"code.dumpstack.io/tools/out-of-tree/distro"

	"github.com/naoina/toml"
)

type Kernel struct {
	// TODO
	// Version string
	// From    string
	// To      string

	// prev. ReleaseMask
	Regex string
}

// Target defines the kernel
type Target struct {
	Distro distro.Distro

	Kernel Kernel
}

// DockerName is returns stable name for docker container
func (km Target) DockerName() string {
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
	Name       string
	Type       ArtifactType
	TestFiles  []FileTransfer
	SourcePath string
	Targets    []Target

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

func (ka Artifact) checkSupport(ki distro.KernelInfo, target Target) (
	supported bool, err error) {

	if target.Distro.Release == "" {
		if ki.Distro.ID != target.Distro.ID {
			return
		}
	} else {
		if !ki.Distro.Equal(target.Distro) {
			return
		}
	}

	supported, err = regexp.MatchString(target.Kernel.Regex, ki.KernelRelease)
	return
}

// Supported returns true if given kernel is supported by artifact
func (ka Artifact) Supported(ki distro.KernelInfo) (supported bool, err error) {
	for _, km := range ka.Targets {
		supported, err = ka.checkSupport(ki, km)
		if supported {
			break
		}

	}
	return
}

// KernelConfig is the ~/.out-of-tree/kernels.toml configuration description
type KernelConfig struct {
	Kernels []distro.KernelInfo
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

// ReadArtifactConfig is for read .out-of-tree.toml
func ReadArtifactConfig(path string) (ka Artifact, err error) {
	buf, err := readFileAll(path)
	if err != nil {
		return
	}

	err = toml.Unmarshal(buf, &ka)
	return
}
