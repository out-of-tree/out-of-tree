// Copyright 2019 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"time"

	"code.dumpstack.io/tools/out-of-tree/config"
	"code.dumpstack.io/tools/out-of-tree/qemu"
)

type ImageCmd struct {
	List ImageListCmd `cmd:"" help:"list images"`
	Edit ImageEditCmd `cmd:"" help:"edit image"`
}

type ImageListCmd struct{}

func (cmd *ImageListCmd) Run(g *Globals) (err error) {
	usr, err := user.Current()
	if err != nil {
		return
	}

	entries, err := os.ReadDir(usr.HomeDir + "/.out-of-tree/images/")
	if err != nil {
		return
	}

	for _, e := range entries {
		fmt.Println(e.Name())
	}

	return
}

type ImageEditCmd struct {
	Name string `help:"image name" required:""`
}

func (cmd *ImageEditCmd) Run(g *Globals) (err error) {
	usr, err := user.Current()
	if err != nil {
		return
	}

	image := usr.HomeDir + "/.out-of-tree/images/" + cmd.Name
	if !exists(image) {
		fmt.Println("image does not exist")
	}

	kcfg, err := config.ReadKernelConfig(g.Config.Kernels)
	if err != nil {
		return
	}

	if len(kcfg.Kernels) == 0 {
		return errors.New("No kernels found")
	}

	ki := config.KernelInfo{}
	for _, k := range kcfg.Kernels {
		if k.RootFS == image {
			ki = k
			break
		}
	}

	kernel := qemu.Kernel{
		KernelPath: ki.KernelPath,
		InitrdPath: ki.InitrdPath,
	}

	q, err := qemu.NewSystem(qemu.X86x64, kernel, ki.RootFS)

	q.Mutable = true

	err = q.Start()
	if err != nil {
		fmt.Println("Qemu start error:", err)
		return
	}
	defer q.Stop()

	fmt.Print("ssh command:\n\n\t")
	fmt.Println(q.GetSSHCommand())

	fmt.Print("\npress enter to stop")
	fmt.Scanln()

	q.Command("root", "poweroff")

	for !q.Died {
		time.Sleep(time.Second)
	}
	return
}

// inspired by Edd Turtle code
func downloadFile(filepath string, url string) (err error) {
	out, err := os.Create(filepath)
	if err != nil {
		return
	}
	defer out.Close()

	resp, err := http.Get(url)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		break
	case http.StatusForbidden, http.StatusNotFound:
		err = fmt.Errorf("Cannot download %s. It looks like you need "+
			"to generate it manually and place it "+
			"to ~/.out-of-tree/images/. "+
			"Check documentation for additional information.", url)
		return
	default:
		err = fmt.Errorf("Something weird happens while "+
			"download file: %d", resp.StatusCode)
		return
	}

	_, err = io.Copy(out, resp.Body)
	return
}

func unpackTar(archive, destination string) (err error) {
	// NOTE: If you're change anything in tar command please check also
	// BSD tar (or if you're using macOS, do not forget to check GNU Tar)
	// Also make sure that sparse files are extracting correctly
	cmd := exec.Command("tar", "-Sxf", archive)
	cmd.Dir = destination + "/"

	rawOutput, err := cmd.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("%v: %s", err, rawOutput)
		return
	}

	return
}

func downloadImage(path, file string) (err error) {
	tmp, err := ioutil.TempDir("/tmp/", "out-of-tree_")
	if err != nil {
		return
	}
	defer os.RemoveAll(tmp)

	archive := tmp + "/" + file + ".tar.gz"
	url := imagesBaseURL + file + ".tar.gz"

	err = downloadFile(archive, url)
	if err != nil {
		return
	}

	err = unpackTar(archive, path)
	return
}
