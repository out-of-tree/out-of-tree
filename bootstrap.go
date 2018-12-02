// Copyright 2018 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package main

import (
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/user"
)

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

	_, err = io.Copy(out, resp.Body)
	return
}

func unpackTar(archive, destination string) (err error) {
	cmd := exec.Command("tar", "xf", archive)
	cmd.Dir = destination + "/"

	rawOutput, err := cmd.CombinedOutput()
	if err != nil {
		// I don't like when some errors printed inside
		// So if you know way to do it better - FIXME please
		log.Println("Unpack images error:", string(rawOutput), err)
		return
	}

	return
}

func bootstrapHandler() (err error) {
	log.Println("Download images...")

	usr, err := user.Current()
	if err != nil {
		return
	}

	imagesPath := usr.HomeDir + "/.out-of-tree/images/"
	os.MkdirAll(imagesPath, os.ModePerm)

	tmp, err := ioutil.TempDir("/tmp/", "out-of-tree_")
	if err != nil {
		log.Println("Temporary directory creation error:", err)
		return
	}
	defer os.RemoveAll(tmp)

	imagesArchive := tmp + "/images.tar.gz"

	err = downloadFile(imagesArchive, imagesURL)
	if err != nil {
		log.Println("Download file error:", err)
		return
	}

	err = unpackTar(imagesArchive, imagesPath)
	if err != nil {
		log.Println("Unpack images error:", err)
	}

	log.Println("Success!")
	return
}
