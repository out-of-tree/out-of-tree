// Copyright 2018 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a GPLv3 license
// (or later) that can be found in the LICENSE file.

package qemukernel

import (
	"net"
	"testing"
)

func TestQemuSystemNew_InvalidKernelPath(t *testing.T) {
	kernel := Kernel{Name: "Invalid", Path: "/invalid/path"}
	if _, err := NewQemuSystem(X86_64, kernel, "/bin/sh"); err == nil {
		t.Fatal(err)
	}
}

func TestQemuSystemNew_InvalidQemuArch(t *testing.T) {
	// FIXME put kernel image to path not just "any valid path"
	kernel := Kernel{Name: "Valid path", Path: "/bin/sh"}
	if _, err := NewQemuSystem(unsupported, kernel, "/bin/sh"); err == nil {
		t.Fatal(err)
	}
}

func TestQemuSystemNew_InvalidQemuDrivePath(t *testing.T) {
	// FIXME put kernel image to path not just "any valid path"
	kernel := Kernel{Name: "Valid path", Path: "/bin/sh"}
	if _, err := NewQemuSystem(X86_64, kernel, "/invalid/path"); err == nil {
		t.Fatal(err)
	}
}

func TestQemuSystemNew(t *testing.T) {
	// FIXME put kernel image to path not just "any valid path"
	kernel := Kernel{Name: "Valid path", Path: "/bin/sh"}
	if _, err := NewQemuSystem(X86_64, kernel, "/bin/sh"); err != nil {
		t.Fatal(err)
	}
}

func TestQemuSystemStart(t *testing.T) {
	// TODO check kernel path on other distros than gentoo
	kernel := Kernel{Name: "Host kernel", Path: "/boot/vmlinuz-4.18.8"}
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
