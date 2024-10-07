// Copyright 2020 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package artifact

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/rs/zerolog/log"

	"code.dumpstack.io/tools/out-of-tree/config/dotfiles"
	"code.dumpstack.io/tools/out-of-tree/distro"
	"code.dumpstack.io/tools/out-of-tree/qemu"
)

func PreloadModules(q *qemu.System, ka Artifact, ki distro.KernelInfo,
	dockerTimeout time.Duration) (err error) {

	for _, pm := range ka.Preload {
		err = preload(q, ki, pm, dockerTimeout)
		if err != nil {
			return
		}
	}
	return
}

func preload(q *qemu.System, ki distro.KernelInfo, pm PreloadModule,
	dockerTimeout time.Duration) (err error) {

	var workPath, cache string
	if pm.Path != "" {
		log.Debug().Msg("Use non-git path for preload module (no cache)")
		workPath = pm.Path
	} else if pm.Repo != "" {
		workPath, cache, err = cloneOrPull(pm.Repo, ki)
		if err != nil {
			return
		}
	} else {
		err = errors.New("no repo/path in preload entry")
		return
	}

	err = buildAndInsmod(workPath, q, ki, dockerTimeout, cache)
	if err != nil {
		return
	}

	time.Sleep(pm.TimeoutAfterLoad.Duration)
	return
}

func buildAndInsmod(workPath string, q *qemu.System, ki distro.KernelInfo,
	dockerTimeout time.Duration, cache string) (err error) {

	tmp, err := tempDir()
	if err != nil {
		return
	}
	defer os.RemoveAll(tmp)

	var af string
	if pathExists(cache) {
		af = cache
	} else {
		af, err = buildPreload(workPath, tmp, ki, dockerTimeout)
		if err != nil {
			return
		}
		if cache != "" {
			err = CopyFile(af, cache)
			if err != nil {
				return
			}
		}
	}

	output, err := q.CopyAndInsmod(af)
	if err != nil {
		log.Error().Err(err).Msg(output)
		return
	}
	return
}

func buildPreload(workPath, tmp string, ki distro.KernelInfo,
	dockerTimeout time.Duration) (af string, err error) {

	ka, err := Artifact{}.Read(workPath + "/.out-of-tree.toml")
	if err != nil {
		log.Warn().Err(err).Msg("preload")
	}

	ka.SourcePath = workPath

	km := Target{
		Distro: ki.Distro,
		Kernel: Kernel{Regex: ki.KernelRelease},
	}
	ka.Targets = []Target{km}

	if ka.Docker.Timeout.Duration != 0 {
		dockerTimeout = ka.Docker.Timeout.Duration
	}

	_, af, _, err = Build(log.Logger, tmp, ka, ki, dockerTimeout, false)
	return
}

func pathExists(path string) bool {
	if _, err := os.Stat(path); err != nil {
		return false
	}
	return true
}

func tempDir() (string, error) {
	return os.MkdirTemp(dotfiles.Dir("tmp"), "")
}

func cloneOrPull(repo string, ki distro.KernelInfo) (workPath, cache string,
	err error) {

	base := dotfiles.Dir("preload")
	workPath = filepath.Join(base, "/repos/", sha1sum(repo))

	var r *git.Repository
	if pathExists(workPath) {
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
			log.Error().Err(err).Msgf("pull %s error", repo)
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
