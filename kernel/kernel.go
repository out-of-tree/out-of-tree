// Copyright 2023 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package kernel

import (
	"io/ioutil"
	"math/rand"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/rs/zerolog/log"

	"code.dumpstack.io/tools/out-of-tree/cache"
	"code.dumpstack.io/tools/out-of-tree/config"
	"code.dumpstack.io/tools/out-of-tree/container"
	"code.dumpstack.io/tools/out-of-tree/fs"
)

func MatchPackages(km config.Target) (packages []string, err error) {
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

func vsyscallAvailable() (available bool, err error) {
	if runtime.GOOS != "linux" {
		// Docker for non-Linux systems is not using the host
		// kernel but uses kernel inside a virtual machine, so
		// it builds by the Docker team with vsyscall support.
		available = true
		return
	}

	buf, err := ioutil.ReadFile("/proc/self/maps")
	if err != nil {
		return
	}

	available = strings.Contains(string(buf), "[vsyscall]")
	return
}

func GenRootfsImage(d container.Image, download bool) (rootfs string, err error) {
	imagesPath := config.Dir("images")
	imageFile := d.Name + ".img"

	rootfs = filepath.Join(imagesPath, imageFile)
	if !fs.PathExists(rootfs) {
		if download {
			log.Info().Msgf("%v not available, start download", imageFile)
			err = cache.DownloadQemuImage(imagesPath, imageFile)
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
		for _ = range c {
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
