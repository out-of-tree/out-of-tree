// Copyright 2019 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
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
