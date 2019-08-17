// Copyright 2018 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package qemu

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

func TestSystemNew_InvalidKernelPath(t *testing.T) {
	kernel := Kernel{Name: "Invalid", KernelPath: "/invalid/path"}
	if _, err := NewSystem(X86_64, kernel, "/bin/sh"); err == nil {
		t.Fatal(err)
	}
}

func TestSystemNew_InvalidQemuArch(t *testing.T) {
	kernel := Kernel{Name: "Valid path", KernelPath: testConfigVmlinuz}
	if _, err := NewSystem(unsupported, kernel, "/bin/sh"); err == nil {
		t.Fatal(err)
	}
}

func TestSystemNew_InvalidQemuDrivePath(t *testing.T) {
	kernel := Kernel{Name: "Valid path", KernelPath: testConfigVmlinuz}
	if _, err := NewSystem(X86_64, kernel, "/invalid/path"); err == nil {
		t.Fatal(err)
	}
}

func TestSystemNew(t *testing.T) {
	kernel := Kernel{Name: "Valid path", KernelPath: testConfigVmlinuz}
	if _, err := NewSystem(X86_64, kernel, "/bin/sh"); err != nil {
		t.Fatal(err)
	}
}

func TestSystemStart(t *testing.T) {
	kernel := Kernel{Name: "Test kernel", KernelPath: testConfigVmlinuz}
	q, err := NewSystem(X86_64, kernel, "/bin/sh")
	if err != nil {
		t.Fatal(err)
	}

	if err = q.Start(); err != nil {
		t.Fatal(err)
	}

	q.Stop()
}

func TestGetFreeAddrPort(t *testing.T) {
	addrPort := getFreeAddrPort()
	ln, err := net.Listen("tcp", addrPort)
	if err != nil {
		t.Fatal(err)
	}
	ln.Close()
}

func TestSystemStart_Timeout(t *testing.T) {
	t.Parallel()
	kernel := Kernel{Name: "Test kernel", KernelPath: testConfigVmlinuz}
	q, err := NewSystem(X86_64, kernel, "/bin/sh")
	if err != nil {
		t.Fatal(err)
	}

	q.Timeout = time.Second

	if err = q.Start(); err != nil {
		t.Fatal(err)
	}

	time.Sleep(2 * time.Second)

	if !q.Died {
		t.Fatal("qemu does not died :c")
	}

	if !q.KilledByTimeout {
		t.Fatal("qemu died not because of timeout O_o")
	}
}

func startTestQemu(t *testing.T, timeout time.Duration) (q *System, err error) {
	t.Parallel()
	kernel := Kernel{
		Name:       "Test kernel",
		KernelPath: testConfigVmlinuz,
		InitrdPath: testConfigInitrd,
	}
	q, err = NewSystem(X86_64, kernel, testConfigRootfs)
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

func TestSystemCommand(t *testing.T) {
	q, err := startTestQemu(t, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer q.Stop()

	output, err := q.Command("root", "cat /etc/shadow")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "root::") {
		t.Fatal("Wrong output from `cat /etc/shadow` by root")
	}

	output, err = q.Command("user", "cat /etc/passwd")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "root:x:0:0:root:/root:/bin/bash") {
		t.Fatal("Wrong output from `cat /etc/passwd` by user")
	}

	_, err = q.Command("user", "cat /etc/shadow")
	// unsuccessful is good because user must not read /etc/shadow
	if err == nil {
		t.Fatal("User have rights for /etc/shadow. WAT?!")
	}
}

func TestSystemCopyFile(t *testing.T) {
	q, err := startTestQemu(t, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer q.Stop()

	localPath := "/bin/sh"

	content, err := ioutil.ReadFile(localPath)
	if err != nil {
		return
	}

	shaLocal := fmt.Sprintf("%x", sha512.Sum512(content))

	err = q.CopyFile("user", localPath, "/tmp/test")
	if err != nil {
		t.Fatal(err)
	}

	shaRemote, err := q.Command("user", "sha512sum /tmp/test")
	if err != nil {
		t.Fatal(err)
	}
	shaRemote = strings.Split(shaRemote, " ")[0]

	if shaLocal != shaRemote {
		t.Fatal(fmt.Sprintf("Broken file (%s instead of %s)",
			shaRemote, shaLocal))
	}
}

func TestSystemCopyAndRun(t *testing.T) {
	q, err := startTestQemu(t, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer q.Stop()

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

	output, err := q.CopyAndRun("user", tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}

	if output != randStr {
		t.Fatal("Wrong output from copyied executable (" +
			output + "," + randStr + ")")
	}
}

func TestSystemCopyAndInsmod(t *testing.T) {
	q, err := startTestQemu(t, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer q.Stop()

	lsmodBefore, err := q.Command("root", "lsmod | wc -l")
	if err != nil {
		t.Fatal(err)
	}

	_, err = q.CopyAndInsmod(testConfigSampleKo)
	if err != nil {
		t.Fatal(err)
	}

	lsmodAfter, err := q.Command("root", "lsmod | wc -l")
	if err != nil {
		t.Fatal(err)
	}

	if lsmodBefore == lsmodAfter {
		t.Fatal("insmod returns ok but there is no new kernel modules")
	}
}

func TestSystemKernelPanic(t *testing.T) {
	q, err := startTestQemu(t, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer q.Stop()

	// Enable sysrq
	_, err = q.Command("root", "echo 1 > /proc/sys/kernel/sysrq")
	if err != nil {
		t.Fatal(err)
	}

	// Trigger kernel panic
	err = q.AsyncCommand("root", "sleep 1s && echo c > /proc/sysrq-trigger")
	if err != nil {
		t.Fatal(err)
	}

	// Wait for panic watcher timeout
	time.Sleep(5 * time.Second)

	if q.KilledByTimeout {
		t.Fatal("qemu is killed by timeout, not because of panic")
	}

	if !q.Died {
		t.Fatal("qemu is not killed after kernel panic")
	}

	if !q.KernelPanic {
		t.Fatal("qemu is died but there's no information about panic")
	}
}

func TestSystemRun(t *testing.T) {
	q, err := startTestQemu(t, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer q.Stop()

	for {
		_, err := q.Command("root", "echo")
		if err == nil {
			break
		}
	}

	start := time.Now()
	err = q.AsyncCommand("root", "sleep 1m")
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 10*time.Second {
		t.Fatalf("q.AsyncCommand does not async (waited %s)",
			time.Since(start))
	}

}

func openedPort(port int) bool {
	conn, err := net.Dial("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func TestSystemDebug(t *testing.T) {
	t.Parallel()
	kernel := Kernel{
		KernelPath: testConfigVmlinuz,
		InitrdPath: testConfigInitrd,
	}
	q, err := NewSystem(X86_64, kernel, testConfigRootfs)
	if err != nil {
		return
	}

	port := 45256

	q.Debug(fmt.Sprintf("tcp::%d", port))

	if openedPort(port) {
		t.Fatal("Port opened before qemu starts")
	}

	if err = q.Start(); err != nil {
		return
	}
	defer q.Stop()

	time.Sleep(time.Second)

	if !openedPort(port) {
		t.Fatal("Qemu debug port does not opened")
	}

	q.Stop()

	if openedPort(port) {
		t.Fatal("Qemu listens after die")
	}
}
