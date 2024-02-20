package artifact

import (
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/naoina/toml"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"code.dumpstack.io/tools/out-of-tree/config/dotfiles"
	"code.dumpstack.io/tools/out-of-tree/distro"
	"code.dumpstack.io/tools/out-of-tree/qemu"
)

type Kernel struct {
	// TODO
	// Version string
	// From    string
	// To      string

	// prev. ReleaseMask
	Regex        string
	ExcludeRegex string
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
		err = fmt.Errorf("type %s is unsupported", stype)
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
		err = fmt.Errorf("cannot marshal %d", at)
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
		Cpus              int
		Memory            int
		Timeout           Duration
		AfterStartTimeout Duration
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

// Read is for read .out-of-tree.toml
func (Artifact) Read(path string) (ka Artifact, err error) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	buf, err := io.ReadAll(f)
	if err != nil {
		return
	}

	err = toml.Unmarshal(buf, &ka)

	if len(strings.Fields(ka.Name)) != 1 {
		err = errors.New("artifact name should not contain spaces")
	}
	return
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

	r, err := regexp.Compile(target.Kernel.Regex)
	if err != nil {
		return
	}

	exr, err := regexp.Compile(target.Kernel.ExcludeRegex)
	if err != nil {
		return
	}

	if !r.MatchString(ki.KernelRelease) {
		return
	}

	if target.Kernel.ExcludeRegex != "" && exr.MatchString(ki.KernelRelease) {
		return
	}

	supported = true
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

func (ka Artifact) Process(slog zerolog.Logger, ki distro.KernelInfo,
	endless bool, cBinary,
	cEndlessStress string, cEndlessTimeout time.Duration,
	dump func(q *qemu.System, ka Artifact, ki distro.KernelInfo,
		result *Result)) {

	slog.Info().Msg("start")
	testStart := time.Now()
	defer func() {
		slog.Debug().Str("test_duration",
			time.Since(testStart).String()).
			Msg("")
	}()

	kernel := qemu.Kernel{KernelPath: ki.KernelPath, InitrdPath: ki.InitrdPath}
	q, err := qemu.NewSystem(qemu.X86x64, kernel, ki.RootFS)
	if err != nil {
		slog.Error().Err(err).Msg("qemu init")
		return
	}
	q.Log = slog

	if ka.Qemu.Timeout.Duration != 0 {
		q.Timeout = ka.Qemu.Timeout.Duration
	}
	if ka.Qemu.Cpus != 0 {
		q.Cpus = ka.Qemu.Cpus
	}
	if ka.Qemu.Memory != 0 {
		q.Memory = ka.Qemu.Memory
	}

	q.SetKASLR(!ka.Mitigations.DisableKaslr)
	q.SetSMEP(!ka.Mitigations.DisableSmep)
	q.SetSMAP(!ka.Mitigations.DisableSmap)
	q.SetKPTI(!ka.Mitigations.DisableKpti)

	if ki.CPU.Model != "" {
		q.CPU.Model = ki.CPU.Model
	}

	if len(ki.CPU.Flags) != 0 {
		q.CPU.Flags = ki.CPU.Flags
	}

	if endless {
		q.Timeout = 0
	}

	qemuStart := time.Now()

	slog.Debug().Msgf("qemu start %v", qemuStart)
	err = q.Start()
	if err != nil {
		slog.Error().Err(err).Msg("qemu start")
		return
	}
	defer q.Stop()

	slog.Debug().Msgf("wait %v", ka.Qemu.AfterStartTimeout)
	time.Sleep(ka.Qemu.AfterStartTimeout.Duration)

	go func() {
		time.Sleep(time.Minute)
		for !q.Died {
			slog.Debug().Msg("still alive")
			time.Sleep(time.Minute)
		}
	}()

	tmp, err := os.MkdirTemp(dotfiles.Dir("tmp"), "")
	if err != nil {
		slog.Error().Err(err).Msg("making tmp directory")
		return
	}
	defer os.RemoveAll(tmp)

	result := Result{}
	if !endless {
		defer dump(q, ka, ki, &result)
	}

	var cTest string

	if ka.Type == Script {
		result.Build.Ok = true
		cTest = ka.Script
	} else if cBinary == "" {
		// TODO: build should return structure
		start := time.Now()
		result.BuildDir, result.BuildArtifact, result.Build.Output, err =
			Build(slog, tmp, ka, ki, ka.Docker.Timeout.Duration)
		slog.Debug().Str("duration", time.Since(start).String()).
			Msg("build done")
		if err != nil {
			log.Error().Err(err).Msg("build")
			return
		}
		result.Build.Ok = true
	} else {
		result.BuildArtifact = cBinary
		result.Build.Ok = true
	}

	if cTest == "" {
		cTest = result.BuildArtifact + "_test"
		if _, err := os.Stat(cTest); err != nil {
			slog.Debug().Msgf("%s does not exist", cTest)
			cTest = tmp + "/source/" + "test.sh"
		} else {
			slog.Debug().Msgf("%s exist", cTest)
		}
	}

	if ka.Qemu.Timeout.Duration == 0 {
		ka.Qemu.Timeout.Duration = time.Minute
	}

	err = q.WaitForSSH(ka.Qemu.Timeout.Duration)
	if err != nil {
		result.InternalError = err
		return
	}
	slog.Debug().Str("qemu_startup_duration",
		time.Since(qemuStart).String()).
		Msg("ssh is available")

	remoteTest, err := copyTest(q, cTest, ka)
	if err != nil {
		result.InternalError = err
		slog.Error().Err(err).Msg("copy test script")
		return
	}

	if ka.StandardModules {
		// Module depends on one of the standard modules
		start := time.Now()
		err = CopyStandardModules(q, ki)
		if err != nil {
			result.InternalError = err
			slog.Error().Err(err).Msg("copy standard modules")
			return
		}
		slog.Debug().Str("duration", time.Since(start).String()).
			Msg("copy standard modules")
	}

	err = PreloadModules(q, ka, ki, ka.Docker.Timeout.Duration)
	if err != nil {
		result.InternalError = err
		slog.Error().Err(err).Msg("preload modules")
		return
	}

	start := time.Now()
	copyArtifactAndTest(slog, q, ka, &result, remoteTest)
	slog.Debug().Str("duration", time.Since(start).String()).
		Msgf("test completed (success: %v)", result.Test.Ok)

	if !endless {
		return
	}

	dump(q, ka, ki, &result)

	if !result.Build.Ok || !result.Run.Ok || !result.Test.Ok {
		return
	}

	slog.Info().Msg("start endless tests")

	if cEndlessStress != "" {
		slog.Debug().Msg("copy and run endless stress script")
		err = q.CopyAndRunAsync("root", cEndlessStress)
		if err != nil {
			q.Stop()
			//f.Sync()
			slog.Fatal().Err(err).Msg("cannot copy/run stress")
			return
		}
	}

	for {
		output, err := q.Command("root", remoteTest)
		if err != nil {
			q.Stop()
			//f.Sync()
			slog.Fatal().Err(err).Msg(output)
			return
		}
		slog.Debug().Msg(output)

		slog.Info().Msg("test success")

		slog.Debug().Msgf("wait %v", cEndlessTimeout)
		time.Sleep(cEndlessTimeout)
	}
}
