// Copyright 2018 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package main

import (
	"errors"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/user"
	"regexp"
	"strings"

	system "github.com/jollheef/go-system"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

func matchBody(url, pattern string) bool {
	resp, err := http.Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return false
	}

	matched, err := regexp.MatchString(pattern, string(body))
	if err != nil {
		return false
	}
	if !matched {
		return false
	}

	return true
}

func isAptCacherExists(url string) bool {
	return matchBody(url, "Apt-Cacher")
}

func runInChroot(chroot, cmd string) (string, string, int, error) {
	return system.System("chroot", chroot, "/bin/sh", "-c", cmd)
}

func generateImage(repo, release, path, size string) (err error) {
	log.Println("Check current user")
	usr, err := user.Current()
	if err != nil {
		return
	}
	if usr.Name != "root" {
		err = errors.New("Run as root is required")
		return
	}

	log.Println("Create qcow2 image")
	stdout, stderr, _, err := system.System("qemu-img", "create", path, size)
	if err != nil {
		log.Println(stdout, stderr)
		return
	}

	log.Println("Create ext4 file system")
	stdout, stderr, _, err = system.System("mkfs.ext4", "-F", path)
	if err != nil {
		log.Println(stdout, stderr)
		return
	}

	log.Println("Create temporary directory")
	stdout, stderr, _, err = system.System("mktemp", "-d")
	if err != nil {
		log.Println(stdout, stderr)
		return
	}
	tmpdir := strings.TrimSpace(stdout)
	defer func() {
		log.Println("Remove temporary directory")
		err := os.RemoveAll(tmpdir)
		if err != nil {
			log.Println(err)
		}
	}()

	log.Println("Mount qemu image")
	stdout, stderr, _, err = system.System("mount", "-o", "loop", path, tmpdir)
	if err != nil {
		log.Println(stdout, stderr)
		return
	}
	defer func() {
		log.Println("Umount qemu image")
		stdout, stderr, _, err := system.System("umount", tmpdir)
		if err != nil {
			log.Println(err, stdout, stderr)
		}
	}()

	log.Println("debootstrap (may be more than 10-15 minutes)")
	stdout, stderr, _, err = system.System("debootstrap",
		"--include=openssh-server",
		release, tmpdir, repo)
	if err != nil {
		log.Println(stdout, stderr)
		return
	}

	log.Println("Create new user")
	stdout, stderr, _, err = runInChroot(tmpdir, "useradd -m user")
	if err != nil {
		log.Println(stdout, stderr)
		return
	}

	log.Println("Login without password")
	stdout, stderr, _, err = runInChroot(tmpdir, "passwd -d root")
	if err != nil {
		log.Println(stdout, stderr)
		return
	}
	stdout, stderr, _, err = runInChroot(tmpdir, "passwd -d user")
	if err != nil {
		log.Println(stdout, stderr)
		return
	}

	log.Println("Allow ssh login without password (NOTE: fixed sshd pam.d)")
	stdout, stderr, _, err = runInChroot(tmpdir,
		"echo auth sufficient pam_permit.so > /etc/pam.d/sshd"+
			" && sed -i '/PermitEmptyPasswords/d' /etc/ssh/sshd_config"+
			" && echo PermitEmptyPasswords yes >> /etc/ssh/sshd_config"+
			" && sed -i '/PermitRootLogin/d' /etc/ssh/sshd_config"+
			" && echo PermitRootLogin yes >> /etc/ssh/sshd_config")
	if err != nil {
		log.Println(stdout, stderr)
		return
	}

	log.Println("Add dhclient to rc.local")
	stdout, stderr, _, err = runInChroot(tmpdir,
		"echo '#!/bin/sh\ndhclient' > /etc/rc.local"+
			" && chmod +x /etc/rc.local")
	if err != nil {
		log.Println(stdout, stderr)
		return
	}

	return
}

func main() {
	generate := kingpin.Command("generate", "Generate qemu image")
	repository := generate.Flag("repository", "Debian/ubuntu repository").Default(
		"deb.debian.org/debian").String()
	aptCacherURL := generate.Flag("apt-cacher", "Local apt cacher url").Default(
		"http://localhost:3142/").String()
	release := generate.Flag("release", "Debian/ubuntu release name").Default(
		"sid").String()
	size := generate.Flag("size", "Image size").Default("8G").String()
	path := generate.Arg("path", "Generated image path").Required().String()

	switch kingpin.Parse() {
	case "generate":
		repo := *repository
		if isAptCacherExists(*aptCacherURL) {
			log.Println("Use local apt cache")
			repo = *aptCacherURL + "/" + repo
		}
		log.Println("Repository:", repo)

		err := generateImage(repo, *release, *path, *size)
		if err != nil {
			log.Fatalln(err)
		}
	}
}
