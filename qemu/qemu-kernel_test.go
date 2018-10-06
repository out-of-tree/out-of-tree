// Copyright 2018 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a GPLv3 license
// (or later) that can be found in the LICENSE file.

package qemukernel

import (
	"crypto/sha512"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net"
	"os"
	"strings"
	"testing"
	"time"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

func TestQemuSystemNew_InvalidKernelPath(t *testing.T) {
	kernel := Kernel{Name: "Invalid", KernelPath: "/invalid/path"}
	if _, err := NewQemuSystem(X86_64, kernel, "/bin/sh"); err == nil {
		t.Fatal(err)
	}
}

func TestQemuSystemNew_InvalidQemuArch(t *testing.T) {
	kernel := Kernel{Name: "Valid path", KernelPath: testConfigVmlinuz}
	if _, err := NewQemuSystem(unsupported, kernel, "/bin/sh"); err == nil {
		t.Fatal(err)
	}
}

func TestQemuSystemNew_InvalidQemuDrivePath(t *testing.T) {
	kernel := Kernel{Name: "Valid path", KernelPath: testConfigVmlinuz}
	if _, err := NewQemuSystem(X86_64, kernel, "/invalid/path"); err == nil {
		t.Fatal(err)
	}
}

func TestQemuSystemNew(t *testing.T) {
	kernel := Kernel{Name: "Valid path", KernelPath: testConfigVmlinuz}
	if _, err := NewQemuSystem(X86_64, kernel, "/bin/sh"); err != nil {
		t.Fatal(err)
	}
}

func TestQemuSystemStart(t *testing.T) {
	kernel := Kernel{Name: "Test kernel", KernelPath: testConfigVmlinuz}
	qemu, err := NewQemuSystem(X86_64, kernel, "/bin/sh")
	if err != nil {
		t.Fatal(err)
	}

	if err = qemu.Start(); err != nil {
		t.Fatal(err)
	}

	qemu.Stop()
}

func TestGetFreeAddrPort(t *testing.T) {
	addrPort := getFreeAddrPort()
	ln, err := net.Listen("tcp", addrPort)
	if err != nil {
		t.Fatal(err)
	}
	ln.Close()
}

func TestQemuSystemStart_Timeout(t *testing.T) {
	t.Parallel()
	kernel := Kernel{Name: "Test kernel", KernelPath: testConfigVmlinuz}
	qemu, err := NewQemuSystem(X86_64, kernel, "/bin/sh")
	if err != nil {
		t.Fatal(err)
	}

	qemu.Timeout = time.Second

	if err = qemu.Start(); err != nil {
		t.Fatal(err)
	}

	time.Sleep(2 * time.Second)

	if !qemu.Died {
		t.Fatal("qemu does not died :c")
	}

	if !qemu.KilledByTimeout {
		t.Fatal("qemu died not because of timeout O_o")
	}
}

func startTestQemu(t *testing.T, timeout time.Duration) (q *QemuSystem, err error) {
	t.Parallel()
	kernel := Kernel{
		Name:       "Test kernel",
		KernelPath: testConfigVmlinuz,
		InitrdPath: testConfigInitrd,
	}
	q, err = NewQemuSystem(X86_64, kernel, testConfigRootfs)
	if err != nil {
		return
	}

	if timeout != 0 {
		q.Timeout = timeout
	}

	if err = q.Start(); err != nil {
		return
	}

	return
}

func TestQemuSystemCommand(t *testing.T) {
	qemu, err := startTestQemu(t, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer qemu.Stop()

	output, err := qemu.Command("root", "cat /etc/shadow")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "root::") {
		t.Fatal("Wrong output from `cat /etc/shadow` by root")
	}

	output, err = qemu.Command("user", "cat /etc/passwd")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "root:x:0:0:root:/root:/bin/bash") {
		t.Fatal("Wrong output from `cat /etc/passwd` by user")
	}

	output, err = qemu.Command("user", "cat /etc/shadow")
	if err == nil { // unsucessful is good because user must not read /etc/shadow
		t.Fatal("User have rights for /etc/shadow. WAT?!")
	}
}

func TestQemuSystemCopyFile(t *testing.T) {
	qemu, err := startTestQemu(t, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer qemu.Stop()

	localPath := "/bin/sh"

	content, err := ioutil.ReadFile(localPath)
	if err != nil {
		return
	}

	sha_local := fmt.Sprintf("%x", sha512.Sum512(content))

	err = qemu.CopyFile("user", localPath, "/tmp/test")
	if err != nil {
		t.Fatal(err)
	}

	sha_remote, err := qemu.Command("user", "sha512sum /tmp/test")
	if err != nil {
		t.Fatal(err)
	}
	sha_remote = strings.Split(sha_remote, " ")[0]

	if sha_local != sha_remote {
		t.Fatal(fmt.Sprintf("Broken file (%s instead of %s)", sha_remote, sha_local))
	}
}

func TestQemuSystemCopyAndRun(t *testing.T) {
	qemu, err := startTestQemu(t, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer qemu.Stop()

	randStr := fmt.Sprintf("%d", rand.Int())
	content := []byte("#!/bin/sh\n echo -n " + randStr + "\n")

	tmpfile, err := ioutil.TempFile("", "executable")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	output, err := qemu.CopyAndRun("user", tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}

	if output != randStr {
		t.Fatal("Wrong output from copyied executable (" + output + "," + randStr + ")")
	}
}

func TestQemuSystemCopyAndInsmod(t *testing.T) {
	qemu, err := startTestQemu(t, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer qemu.Stop()

	lsmodBefore, err := qemu.Command("root", "lsmod | wc -l")
	if err != nil {
		t.Fatal(err)
	}

	_, err = qemu.CopyAndInsmod(testConfigSampleKo)
	if err != nil {
		t.Fatal(err)
	}

	lsmodAfter, err := qemu.Command("root", "lsmod | wc -l")
	if err != nil {
		t.Fatal(err)
	}

	if lsmodBefore == lsmodAfter {
		t.Fatal("insmod returns ok but there is no new kernel modules")
	}
}

func TestQemuSystemRun(t *testing.T) {
	qemu, err := startTestQemu(t, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer qemu.Stop()

	for {
		_, err := qemu.Command("root", "echo")
		if err == nil {
			break
		}
	}

	start := time.Now()
	err = qemu.AsyncCommand("root", "sleep 10s")
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > time.Second {
		t.Fatalf("qemu.Run does not async (waited %s)", +time.Since(start))
	}

}
