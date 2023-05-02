// Copyright 2020 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package main

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/rs/zerolog/log"

	"code.dumpstack.io/tools/out-of-tree/config"
	"code.dumpstack.io/tools/out-of-tree/qemu"
)

func preloadModules(q *qemu.System, ka config.Artifact, ki config.KernelInfo,
	dockerTimeout time.Duration) (err error) {

	for _, pm := range ka.Preload {
		err = preload(q, ki, pm, dockerTimeout)
		if err != nil {
			return
		}
	}
	return
}

func preload(q *qemu.System, ki config.KernelInfo, pm config.PreloadModule,
	dockerTimeout time.Duration) (err error) {

	var workPath, cache string
	if pm.Path != "" {
		log.Print("Use non-git path for preload module (no cache)")
		workPath = pm.Path
	} else if pm.Repo != "" {
		workPath, cache, err = cloneOrPull(pm.Repo, ki)
		if err != nil {
			return
		}
	} else {
		errors.New("No repo/path in preload entry")
	}

	err = buildAndInsmod(workPath, q, ki, dockerTimeout, cache)
	if err != nil {
		return
	}

	time.Sleep(pm.TimeoutAfterLoad.Duration)
	return
}

func buildAndInsmod(workPath string, q *qemu.System, ki config.KernelInfo,
	dockerTimeout time.Duration, cache string) (err error) {

	tmp, err := ioutil.TempDir(tempDirBase, "out-of-tree_")
	if err != nil {
		return
	}
	defer os.RemoveAll(tmp)

	var artifact string
	if exists(cache) {
		artifact = cache
	} else {
		artifact, err = buildPreload(workPath, tmp, ki, dockerTimeout)
		if err != nil {
			return
		}
		if cache != "" {
			err = copyFile(artifact, cache)
			if err != nil {
				return
			}
		}
	}

	output, err := q.CopyAndInsmod(artifact)
	if err != nil {
		log.Print(output)
		return
	}
	return
}

func buildPreload(workPath, tmp string, ki config.KernelInfo,
	dockerTimeout time.Duration) (artifact string, err error) {

	ka, err := config.ReadArtifactConfig(workPath + "/.out-of-tree.toml")
	if err != nil {
		return
	}

	ka.SourcePath = workPath

	km := config.KernelMask{DistroType: ki.DistroType,
		DistroRelease: ki.DistroRelease,
		ReleaseMask:   ki.KernelRelease,
	}
	ka.SupportedKernels = []config.KernelMask{km}

	if ka.Docker.Timeout.Duration != 0 {
		dockerTimeout = ka.Docker.Timeout.Duration
	}

	_, artifact, _, err = build(log.Logger, tmp, ka, ki, dockerTimeout)
	return
}

func cloneOrPull(repo string, ki config.KernelInfo) (workPath, cache string, err error) {
	usr, err := user.Current()
	if err != nil {
		return
	}
	base := filepath.Join(usr.HomeDir, "/.out-of-tree/preload/")
	workPath = filepath.Join(base, "/repos/", sha1sum(repo))

	var r *git.Repository
	if exists(workPath) {
		r, err = git.PlainOpen(workPath)
		if err != nil {
			return
		}

		var w *git.Worktree
		w, err = r.Worktree()
		if err != nil {
			return
		}

		err = w.Pull(&git.PullOptions{})
		if err != nil && err != git.NoErrAlreadyUpToDate {
			log.Print(repo, "pull error:", err)
		}
	} else {
		r, err = git.PlainClone(workPath, false, &git.CloneOptions{URL: repo})
		if err != nil {
			return
		}
	}

	ref, err := r.Head()
	if err != nil {
		return
	}

	cachedir := filepath.Join(base, "/cache/")
	os.MkdirAll(cachedir, 0700)

	filename := sha1sum(repo + ki.KernelPath + ref.Hash().String())
	cache = filepath.Join(cachedir, filename)
	return
}

func sha1sum(data string) string {
	h := sha1.Sum([]byte(data))
	return hex.EncodeToString(h[:])
}
