// Copyright 2018 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"os/user"
	"regexp"
	"strings"
	"time"

	"code.dumpstack.io/tools/out-of-tree/config"
	"github.com/naoina/toml"
)

func kernelListHandler(kcfg config.KernelConfig) (err error) {
	if len(kcfg.Kernels) == 0 {
		return errors.New("No kernels found")
	}
	for _, k := range kcfg.Kernels {
		fmt.Println(k.DistroType, k.DistroRelease, k.KernelRelease)
	}
	return
}

func matchDebianHeadersPkg(container, mask string, generic bool) (
	pkgs []string, err error) {

	cmd := "apt-cache search linux-headers | cut -d ' ' -f 1"
	output, err := dockerRun(time.Minute, container, "/tmp", cmd)
	if err != nil {
		return
	}

	r, err := regexp.Compile("linux-headers-" + mask)
	if err != nil {
		return
	}

	kernels := r.FindAll([]byte(output), -1)

	for _, k := range kernels {
		pkg := string(k)
		if generic && !strings.HasSuffix(pkg, "generic") {
			continue
		}
		if pkg == "linux-headers-generic" {
			continue
		}
		pkgs = append(pkgs, pkg)
	}

	return
}

func dockerImagePath(sk config.KernelMask) (path string, err error) {
	usr, err := user.Current()
	if err != nil {
		return
	}

	path = usr.HomeDir + "/.out-of-tree/"
	path += sk.DistroType.String() + "/" + sk.DistroRelease
	return
}

func generateBaseDockerImage(sk config.KernelMask) (err error) {
	imagePath, err := dockerImagePath(sk)
	if err != nil {
		return
	}
	dockerPath := imagePath + "/Dockerfile"

	d := "# BASE\n"

	if exists(dockerPath) {
		log.Printf("Base image for %s:%s found",
			sk.DistroType.String(), sk.DistroRelease)
		return
	}

	log.Printf("Base image for %s:%s not found, start generating",
		sk.DistroType.String(), sk.DistroRelease)
	os.MkdirAll(imagePath, os.ModePerm)

	d += fmt.Sprintf("FROM %s:%s\n",
		strings.ToLower(sk.DistroType.String()),
		sk.DistroRelease,
	)

	switch sk.DistroType {
	case config.Ubuntu:
		d += "ENV DEBIAN_FRONTEND=noninteractive\n"
		d += "RUN apt-get update\n"
		d += "RUN apt-get install -y build-essential libelf-dev\n"
		d += "RUN apt-get install -y wget git\n"
	default:
		err = fmt.Errorf("%s not yet supported", sk.DistroType.String())
		return
	}

	d += "# END BASE\n\n"

	err = ioutil.WriteFile(dockerPath, []byte(d), 0644)
	if err != nil {
		return
	}

	cmd := exec.Command("docker", "build", "-t", sk.DockerName(), imagePath)
	rawOutput, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("Base image for %s:%s generating error, see log",
			sk.DistroType.String(), sk.DistroRelease)
		log.Println(string(rawOutput))
		return
	}

	log.Printf("Base image for %s:%s generating success",
		sk.DistroType.String(), sk.DistroRelease)

	return
}

func dockerImageAppend(sk config.KernelMask, pkgname string) (err error) {
	imagePath, err := dockerImagePath(sk)
	if err != nil {
		return
	}

	raw, err := ioutil.ReadFile(imagePath + "/Dockerfile")
	if err != nil {
		return
	}

	if strings.Contains(string(raw), pkgname) {
		// already installed kernel
		log.Printf("kernel %s for %s:%s is already exists",
			pkgname, sk.DistroType.String(), sk.DistroRelease)
		return
	}

	imagepkg := strings.Replace(pkgname, "headers", "image", -1)

	log.Printf("Start adding kernel %s for %s:%s",
		imagepkg, sk.DistroType.String(), sk.DistroRelease)

	s := fmt.Sprintf("RUN apt-get install -y %s %s\n", imagepkg, pkgname)

	err = ioutil.WriteFile(imagePath+"/Dockerfile",
		append(raw, []byte(s)...), 0644)
	if err != nil {
		return
	}

	cmd := exec.Command("docker", "build", "-t", sk.DockerName(), imagePath)
	rawOutput, err := cmd.CombinedOutput()
	if err != nil {
		// Fallback to previous state
		werr := ioutil.WriteFile(imagePath+"/Dockerfile", raw, 0644)
		if werr != nil {
			return
		}

		log.Printf("Add kernel %s for %s:%s error, see log",
			pkgname, sk.DistroType.String(), sk.DistroRelease)
		log.Println(string(rawOutput))
		return
	}

	log.Printf("Add kernel %s for %s:%s success",
		pkgname, sk.DistroType.String(), sk.DistroRelease)

	return
}

func kickImage(name string) (err error) {
	cmd := exec.Command("docker", "run", name, "bash", "-c", "ls")
	_, err = cmd.CombinedOutput()
	return
}

func copyKernels(name string) (err error) {
	cmd := exec.Command("docker", "ps", "-a")
	rawOutput, err := cmd.CombinedOutput()
	if err != nil {
		log.Println(string(rawOutput))
		return
	}

	r, err := regexp.Compile(".*" + name)
	if err != nil {
		return
	}

	var containerID string

	what := r.FindAll(rawOutput, -1)
	for _, w := range what {
		containerID = strings.Fields(string(w))[0]
		break
	}

	usr, err := user.Current()
	if err != nil {
		return
	}

	target := usr.HomeDir + "/.out-of-tree/kernels/"
	if !exists(target) {
		os.MkdirAll(target, os.ModePerm)
	}

	cmd = exec.Command("docker", "cp", containerID+":/boot/.", target)
	rawOutput, err = cmd.CombinedOutput()
	if err != nil {
		log.Println(string(rawOutput))
		return
	}

	return
}

func genKernelPath(files []os.FileInfo, kname string) string {
	for _, file := range files {
		if strings.Contains(file.Name(), "vmlinuz") {
			if strings.Contains(file.Name(), kname) {
				return file.Name()
			}
		}
	}
	return "unknown"
}

func genInitrdPath(files []os.FileInfo, kname string) string {
	for _, file := range files {
		if strings.Contains(file.Name(), "initrd") {
			if strings.Contains(file.Name(), kname) {
				return file.Name()
			}
		}
	}
	return "unknown"
}

func genRootfsImage(d dockerImageInfo) string {
	usr, err := user.Current()
	if err != nil {
		return fmt.Sprintln(err)
	}
	imageFile := d.ContainerName + ".img"
	return usr.HomeDir + "/.out-of-tree/images/" + imageFile
}

type dockerImageInfo struct {
	ContainerName string
	DistroType    config.DistroType
	DistroRelease string // 18.04/7.4.1708/9.1
}

func listDockerImages() (diis []dockerImageInfo, err error) {
	cmd := exec.Command("docker", "images")
	rawOutput, err := cmd.CombinedOutput()
	if err != nil {
		return
	}

	r, err := regexp.Compile("out_of_tree_.*")
	if err != nil {
		return
	}

	containers := r.FindAll(rawOutput, -1)
	for _, c := range containers {
		container := strings.Fields(string(c))[0]

		s := strings.Replace(container, "__", ".", -1)
		values := strings.Split(s, "_")
		distro, ver := values[3], values[4]

		dii := dockerImageInfo{
			ContainerName: container,
			DistroRelease: ver,
		}

		dii.DistroType, err = config.NewDistroType(distro)
		if err != nil {
			return
		}

		diis = append(diis, dii)
	}
	return
}

func updateKernelsCfg() (err error) {
	dockerImages, err := listDockerImages()
	if err != nil {
		return
	}

	newkcfg := config.KernelConfig{}

	for _, d := range dockerImages {
		err = genKernels(d, &newkcfg)
		if err != nil {
			log.Println("gen kernels", d.ContainerName, ":", err)
			continue
		}
	}

	stripkcfg := config.KernelConfig{}
	for _, nk := range newkcfg.Kernels {
		if !hasKernel(nk, stripkcfg) {
			stripkcfg.Kernels = append(stripkcfg.Kernels, nk)
		}
	}

	buf, err := toml.Marshal(&stripkcfg)
	if err != nil {
		return
	}

	buf = append([]byte("# Autogenerated\n# DO NOT EDIT\n\n"), buf...)

	usr, err := user.Current()
	if err != nil {
		return
	}

	// TODO move all cfg path values to one provider
	kernelsCfgPath := usr.HomeDir + "/.out-of-tree/kernels.toml"
	err = ioutil.WriteFile(kernelsCfgPath, buf, 0644)
	if err != nil {
		return
	}

	log.Println(kernelsCfgPath, "is successfully updated")
	return
}

func genKernels(dii dockerImageInfo, newkcfg *config.KernelConfig) (
	err error) {

	name := dii.ContainerName
	cmd := exec.Command("docker", "run", name, "ls", "/lib/modules")
	rawOutput, err := cmd.CombinedOutput()
	if err != nil {
		log.Println(string(rawOutput), err)
		return
	}

	usr, err := user.Current()
	if err != nil {
		return
	}
	kernelsBase := usr.HomeDir + "/.out-of-tree/kernels/"
	files, err := ioutil.ReadDir(kernelsBase)
	if err != nil {
		return
	}

	for _, k := range strings.Fields(string(rawOutput)) {
		ki := config.KernelInfo{
			DistroType:    dii.DistroType,
			DistroRelease: dii.DistroRelease,
			KernelRelease: k,
			ContainerName: name,

			KernelPath: kernelsBase + genKernelPath(files, k),
			InitrdPath: kernelsBase + genInitrdPath(files, k),
			RootFS:     genRootfsImage(dii),
		}
		newkcfg.Kernels = append(newkcfg.Kernels, ki)
	}

	return
}

func hasKernel(ki config.KernelInfo, kcfg config.KernelConfig) bool {
	for _, sk := range kcfg.Kernels {
		if sk == ki {
			return true
		}
	}
	return false
}

func generateKernels(km config.KernelMask) (err error) {
	err = generateBaseDockerImage(km)
	if err != nil {
		return
	}

	var pkgs []string
	pkgs, err = matchDebianHeadersPkg(km.DockerName(),
		km.ReleaseMask, true)
	if err != nil {
		return
	}

	for i, pkg := range pkgs {
		log.Println(i, "/", len(pkgs))
		dockerImageAppend(km, pkg)
	}

	err = kickImage(km.DockerName())
	if err != nil {
		log.Println("kick image", km.DockerName(), ":", err)
		return
	}

	err = copyKernels(km.DockerName())
	if err != nil {
		log.Println("copy kernels", km.DockerName(), ":", err)
		return
	}
	return
}

func kernelAutogenHandler(workPath string) (err error) {
	ka, err := config.ReadArtifactConfig(workPath + "/.out-of-tree.toml")
	if err != nil {
		return
	}

	for _, sk := range ka.SupportedKernels {
		if sk.DistroRelease == "" {
			err = errors.New("Please set distro_release")
			return
		}

		err = generateKernels(sk)
		if err != nil {
			return
		}
	}

	err = updateKernelsCfg()
	return
}

func kernelDockerRegenHandler() (err error) {
	dockerImages, err := listDockerImages()
	if err != nil {
		return
	}

	for _, d := range dockerImages {
		var imagePath string
		imagePath, err = dockerImagePath(config.KernelMask{
			DistroType:    d.DistroType,
			DistroRelease: d.DistroRelease,
		})
		if err != nil {
			return
		}

		cmd := exec.Command("docker", "build", "-t",
			d.ContainerName, imagePath)
		var rawOutput []byte
		rawOutput, err = cmd.CombinedOutput()
		if err != nil {
			log.Println("docker build:", string(rawOutput))
			return
		}

		err = kickImage(d.ContainerName)
		if err != nil {
			log.Println("kick image", d.ContainerName, ":", err)
			continue
		}

		err = copyKernels(d.ContainerName)
		if err != nil {
			log.Println("copy kernels", d.ContainerName, ":", err)
			continue
		}
	}

	return updateKernelsCfg()
}

func kernelGenallHandler(distro, version string) (err error) {
	distroType, err := config.NewDistroType(distro)
	if err != nil {
		return
	}

	km := config.KernelMask{
		DistroType:    distroType,
		DistroRelease: version,
		ReleaseMask:   ".*",
	}
	err = generateKernels(km)
	if err != nil {
		return
	}

	return updateKernelsCfg()
}
