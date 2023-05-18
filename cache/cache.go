// Copyright 2023 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package cache

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/cavaliergopher/grab/v3"
	"github.com/rs/zerolog/log"

	"code.dumpstack.io/tools/out-of-tree/distro"
	"code.dumpstack.io/tools/out-of-tree/fs"
)

var URL = "https://out-of-tree.fra1.digitaloceanspaces.com/1.0.0/"

func unpackTar(archive, destination string) (err error) {
	// NOTE: If you're change anything in tar command please check also
	// BSD tar (or if you're using macOS, do not forget to check GNU Tar)
	// Also make sure that sparse files are extracting correctly
	cmd := exec.Command("tar", "-Sxf", archive)
	cmd.Dir = destination + "/"

	log.Debug().Msgf("%v", cmd)

	rawOutput, err := cmd.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("%v: %s", err, rawOutput)
		return
	}

	return
}

func DownloadQemuImage(path, file string) (err error) {
	tmp, err := fs.TempDir()
	if err != nil {
		return
	}
	defer os.RemoveAll(tmp)

	fileurl, err := url.JoinPath(URL, file+".tar.gz")
	if err != nil {
		return
	}

	log.Debug().Msgf("download qemu image from %s", fileurl)

	resp, err := grab.Get(tmp, fileurl)
	if err != nil {
		err = fmt.Errorf("Cannot download %s. It looks like you need "+
			"to generate it manually and place it "+
			"to ~/.out-of-tree/images/. "+
			"Check documentation for additional information.",
			fileurl)
		return
	}

	err = unpackTar(resp.Filename, path)
	if err != nil {
		return
	}

	return os.Remove(resp.Filename)
}

func DownloadDebianCache(cachePath string) (err error) {
	tmp, err := fs.TempDir()
	if err != nil {
		return
	}
	defer os.RemoveAll(tmp)

	file := filepath.Base(cachePath)

	fileurl, err := url.JoinPath(URL, file)
	if err != nil {
		return
	}

	log.Debug().Msgf("download debian cache from %s", fileurl)

	resp, err := grab.Get(tmp, fileurl)
	if err != nil {
		return
	}

	return os.Rename(resp.Filename, cachePath)
}

func PackageURL(dt distro.ID, orig string) (found bool, fileurl string) {
	if dt != distro.Debian {
		return
	}

	filename := filepath.Base(orig)

	fileurl, err := url.JoinPath(URL, "packages/debian", filename)
	if err != nil {
		return
	}

	resp, err := http.Head(fileurl)
	if err != nil {
		return
	}
	if resp.StatusCode != http.StatusOK {
		return
	}

	found = true
	return
}
