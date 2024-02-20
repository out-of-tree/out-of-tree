// Copyright 2023 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package kernel

import (
	"math/rand"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"

	"github.com/rs/zerolog/log"

	"code.dumpstack.io/tools/out-of-tree/artifact"
	"code.dumpstack.io/tools/out-of-tree/cache"
	"code.dumpstack.io/tools/out-of-tree/config/dotfiles"
	"code.dumpstack.io/tools/out-of-tree/fs"
)

func MatchPackages(km artifact.Target) (packages []string, err error) {
	pkgs, err := km.Distro.Packages()
	if err != nil {
		return
	}

	r, err := regexp.Compile(km.Kernel.Regex)
	if err != nil {
		return
	}

	exr, err := regexp.Compile(km.Kernel.ExcludeRegex)
	if err != nil {
		return
	}

	for _, pkg := range pkgs {
		if !r.MatchString(pkg) {
			continue
		}

		if km.Kernel.ExcludeRegex != "" && exr.MatchString(pkg) {
			continue
		}

		packages = append(packages, pkg)
	}

	return
}

func GenRootfsImage(imageFile string, download bool) (rootfs string, err error) {
	imagesPath := dotfiles.Dir("images")

	rootfs = filepath.Join(imagesPath, imageFile)
	if !fs.PathExists(rootfs) {
		if download {
			log.Info().Msgf("%v not available, start download", imageFile)
			err = cache.DownloadRootFS(imagesPath, imageFile)
		}
	}
	return
}

func ShuffleStrings(a []string) []string {
	// Fisher–Yates shuffle
	for i := len(a) - 1; i > 0; i-- {
		j := rand.Intn(i + 1)
		a[i], a[j] = a[j], a[i]
	}
	return a
}

func SetSigintHandler(variable *bool) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		counter := 0
		for range c {
			if counter == 0 {
				*variable = true
				log.Warn().Msg("shutdown requested, finishing work")
				log.Info().Msg("^C a couple of times more for an unsafe exit")
			} else if counter >= 3 {
				log.Fatal().Msg("unsafe exit")
			}

			counter += 1
		}
	}()

}
