package artifact

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/otiai10/copy"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"code.dumpstack.io/tools/out-of-tree/container"
	"code.dumpstack.io/tools/out-of-tree/distro"
	"code.dumpstack.io/tools/out-of-tree/qemu"
)

func sh(workdir, command string) (output string, err error) {
	flog := log.With().
		Str("workdir", workdir).
		Str("command", command).
		Logger()

	cmd := exec.Command("sh", "-c", "cd "+workdir+" && "+command)

	flog.Debug().Msgf("%v", cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return
	}
	cmd.Stderr = cmd.Stdout

	err = cmd.Start()
	if err != nil {
		return
	}

	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			m := scanner.Text()
			output += m + "\n"
			flog.Trace().Str("stdout", m).Msg("")
		}
	}()

	err = cmd.Wait()

	if err != nil {
		err = fmt.Errorf("%v %v output: %v", cmd, err, output)
	}
	return
}

func applyPatches(src string, ka Artifact) (err error) {
	for i, patch := range ka.Patches {
		name := fmt.Sprintf("patch_%02d", i)

		path := src + "/" + name + ".diff"
		if patch.Source != "" && patch.Path != "" {
			err = errors.New("path and source are mutually exclusive")
			return
		} else if patch.Source != "" {
			err = os.WriteFile(path, []byte(patch.Source), 0644)
			if err != nil {
				return
			}
		} else if patch.Path != "" {
			err = copy.Copy(patch.Path, path)
			if err != nil {
				return
			}
		}

		if patch.Source != "" || patch.Path != "" {
			_, err = sh(src, "patch < "+path)
			if err != nil {
				return
			}
		}

		if patch.Script != "" {
			script := src + "/" + name + ".sh"
			err = os.WriteFile(script, []byte(patch.Script), 0755)
			if err != nil {
				return
			}
			_, err = sh(src, script)
			if err != nil {
				return
			}
		}
	}
	return
}

func Build(flog zerolog.Logger, tmp string, ka Artifact,
	ki distro.KernelInfo, dockerTimeout time.Duration, realtimeOutput bool) (
	outdir, outpath, output string, err error) {

	target := strings.Replace(ka.Name, " ", "_", -1)
	if target == "" {
		target = fmt.Sprintf("%d", rand.Int())
	}

	outdir = tmp + "/source"

	if len(ka.SourceFiles) == 0 {
		err = copy.Copy(ka.SourcePath, outdir)
	} else {
		err = CopyFiles(ka.SourcePath, ka.SourceFiles, outdir)
	}
	if err != nil {
		return
	}

	err = applyPatches(outdir, ka)
	if err != nil {
		return
	}

	outpath = outdir + "/" + target
	if ka.Type == KernelModule {
		outpath += ".ko"
	}

	if ki.KernelVersion == "" {
		ki.KernelVersion = ki.KernelRelease
	}

	kernel := "/lib/modules/" + ki.KernelVersion + "/build"
	if ki.KernelSource != "" {
		kernel = ki.KernelSource
	}

	buildCommand := "make KERNEL=" + kernel + " TARGET=" + target
	if ka.Make.Target != "" {
		buildCommand += " " + ka.Make.Target
	}

	if ki.ContainerName != "" {
		var c container.Container
		container.Timeout = dockerTimeout
		c, err = container.NewFromKernelInfo(ki)
		c.Log = flog
		if err != nil {
			log.Fatal().Err(err).Msg("container creation failure")
		}

		c.Args = append(c.Args, "--network", "none")

		if realtimeOutput {
			c.SetCommandsOutputHandler(func(s string) {
				fmt.Printf("%s\n", s)
			})
		}

		output, err = c.Run(outdir, []string{
			buildCommand + " && chmod -R 777 /work",
		})

		if realtimeOutput {
			c.CloseCommandsOutputHandler()
		}
	} else {
		cmd := exec.Command("bash", "-c", "cd "+outdir+" && "+
			buildCommand)

		log.Debug().Msgf("%v", cmd)

		timer := time.AfterFunc(dockerTimeout, func() {
			cmd.Process.Kill()
		})
		defer timer.Stop()

		var raw []byte
		raw, err = cmd.CombinedOutput()
		if err != nil {
			e := fmt.Sprintf("error `%v` for cmd `%v` with output `%v`",
				err, buildCommand, string(raw))
			err = errors.New(e)
			return
		}

		output = string(raw)
	}
	return
}

func runScript(q *qemu.System, script string) (output string, err error) {
	return q.Command("root", script)
}

func testKernelModule(q *qemu.System, ka Artifact,
	test string) (output string, err error) {

	output, err = q.Command("root", test)
	// TODO generic checks for WARNING's and so on
	return
}

func testKernelExploit(q *qemu.System, ka Artifact,
	test, exploit string) (output string, err error) {

	output, err = q.Command("user", "chmod +x "+exploit)
	if err != nil {
		return
	}

	randFilePath := fmt.Sprintf("/root/%d", rand.Int())

	cmd := fmt.Sprintf("%s %s %s", test, exploit, randFilePath)
	output, err = q.Command("user", cmd)
	if err != nil {
		return
	}

	_, err = q.Command("root", "stat "+randFilePath)
	if err != nil {
		return
	}

	return
}

type Result struct {
	BuildDir         string
	BuildArtifact    string
	Build, Run, Test struct {
		Output string
		Ok     bool
	}

	InternalError       error
	InternalErrorString string
}

func CopyFiles(path string, files []string, dest string) (err error) {
	err = os.MkdirAll(dest, os.ModePerm)
	if err != nil {
		return
	}

	for _, sf := range files {
		if sf[0] == '/' {
			err = CopyFile(sf, filepath.Join(dest, filepath.Base(sf)))
			if err != nil {
				return
			}
			continue
		}

		err = os.MkdirAll(filepath.Join(dest, filepath.Dir(sf)), os.ModePerm)
		if err != nil {
			return
		}

		err = CopyFile(filepath.Join(path, sf), filepath.Join(dest, sf))
		if err != nil {
			return
		}
	}

	return
}

func CopyFile(sourcePath, destinationPath string) (err error) {
	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return
	}
	defer sourceFile.Close()

	destinationFile, err := os.Create(destinationPath)
	if err != nil {
		return err
	}
	if _, err := io.Copy(destinationFile, sourceFile); err != nil {
		destinationFile.Close()
		return err
	}
	return destinationFile.Close()
}

func copyArtifactAndTest(slog zerolog.Logger, q *qemu.System, ka Artifact,
	res *Result, remoteTest string, outputOnSuccess, realtimeOutput bool) (err error) {

	// Copy all test files to the remote machine
	for _, f := range ka.TestFiles {
		if f.Local[0] != '/' {
			if res.BuildDir != "" {
				f.Local = res.BuildDir + "/" + f.Local
			}
		}
		err = q.CopyFile(f.User, f.Local, f.Remote)
		if err != nil {
			res.InternalError = err
			slog.Error().Err(err).Msg("copy test file")
			return
		}
	}

	switch ka.Type {
	case KernelModule:
		res.Run.Output, err = q.CopyAndInsmod(res.BuildArtifact)
		if err != nil {
			slog.Error().Err(err).Msg(res.Run.Output)
			// TODO errors.As
			if strings.Contains(err.Error(), "connection refused") {
				res.InternalError = err
			}
			return
		}
		res.Run.Ok = true

		res.Test.Output, err = testKernelModule(q, ka, remoteTest)
		if err != nil {
			break
		}
		res.Test.Ok = true
	case KernelExploit:
		remoteExploit := fmt.Sprintf("/tmp/exploit_%d", rand.Int())
		err = q.CopyFile("user", res.BuildArtifact, remoteExploit)
		if err != nil {
			return
		}

		res.Test.Output, err = testKernelExploit(q, ka, remoteTest,
			remoteExploit)
		if err != nil {
			break
		}
		res.Run.Ok = true // does not really used
		res.Test.Ok = true
	case Script:
		res.Test.Output, err = runScript(q, remoteTest)
		if err != nil {
			break
		}
		res.Run.Ok = true
		res.Test.Ok = true
	default:
		slog.Fatal().Msg("Unsupported artifact type")
	}

	if err != nil || !res.Test.Ok {
		slog.Error().Err(err).Msgf("test error\n%v\n", res.Test.Output)
		return
	}

	if outputOnSuccess && !realtimeOutput {
		slog.Info().Msgf("test success\n%v\n", res.Test.Output)
	} else {
		slog.Info().Msg("test success")
	}

	_, err = q.Command("root", "echo")
	if err != nil {
		slog.Error().Err(err).Msg("after-test ssh reconnect")
		res.Test.Ok = false
		return
	}

	return
}

func copyTest(q *qemu.System, testPath string, ka Artifact) (
	remoteTest string, err error) {

	remoteTest = fmt.Sprintf("/tmp/test_%d", rand.Int())
	err = q.CopyFile("user", testPath, remoteTest)
	if err != nil {
		if ka.Type == KernelExploit {
			q.Command("user",
				"echo -e '#!/bin/sh\necho touch $2 | $1' "+
					"> "+remoteTest+
					" && chmod +x "+remoteTest)
		} else {
			q.Command("user", "echo '#!/bin/sh' "+
				"> "+remoteTest+" && chmod +x "+remoteTest)
		}
	}

	_, err = q.Command("root", "chmod +x "+remoteTest)
	return
}

func CopyStandardModules(q *qemu.System, ki distro.KernelInfo) (err error) {
	_, err = q.Command("root", "mkdir -p /lib/modules/"+ki.KernelVersion)
	if err != nil {
		return
	}

	remotePath := "/lib/modules/" + ki.KernelVersion + "/"

	err = q.CopyDirectory("root", ki.ModulesPath+"/kernel", remotePath+"/kernel")
	if err != nil {
		return
	}

	files, err := os.ReadDir(ki.ModulesPath)
	if err != nil {
		return
	}

	for _, de := range files {
		var fi fs.FileInfo
		fi, err = de.Info()
		if err != nil {
			continue
		}
		if fi.Mode()&os.ModeSymlink == os.ModeSymlink {
			continue
		}
		if !strings.HasPrefix(fi.Name(), "modules") {
			continue
		}
		err = q.CopyFile("root", ki.ModulesPath+"/"+fi.Name(), remotePath)
	}

	return
}
