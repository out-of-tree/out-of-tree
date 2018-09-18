// Copyright 2018 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a GPLv3 license
// (or later) that can be found in the LICENSE file.

package qemukernel

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"
)

func readBytesUntilEOF(pipe io.ReadCloser) (buf []byte, err error) {
	bufSize := 1024
	for err != io.EOF {
		stdout := make([]byte, bufSize)
		var n int

		n, err = pipe.Read(stdout)
		if err != nil && err != io.EOF {
			return
		}

		buf = append(buf, stdout[:n]...)
	}

	if err == io.EOF {
		err = nil
	}
	return
}

func readUntilEOF(pipe io.ReadCloser) (str string, err error) {
	buf, err := readBytesUntilEOF(pipe)
	str = string(buf)
	return
}

type arch string

const (
	X86_64 arch = "x86_64"
	I386        = "i386"
	// TODO add other

	unsupported = "unsupported" // for test purposes
)

// Kernel describe kernel parameters for qemu
type Kernel struct {
	Name string
	Path string
}

// QemuSystem describe qemu parameters and runned process
type QemuSystem struct {
	arch   arch
	kernel Kernel

	// accessible while qemu is runned
	cmd  *exec.Cmd
	pipe struct {
		stdin  io.WriteCloser
		stderr io.ReadCloser
		stdout io.ReadCloser
	}
	died bool

	// accessible after qemu is closed
	Stdout, Stderr string
	exitErr        error
}

// NewQemuSystem constructor
func NewQemuSystem(arch arch, kernel Kernel) (q QemuSystem, err error) {
	if _, err = exec.LookPath("qemu-system-" + string(arch)); err != nil {
		return
	}
	q.arch = arch

	if _, err = os.Stat(kernel.Path); err != nil {
		return
	}
	q.kernel = kernel

	return
}

// Start qemu process
func (q *QemuSystem) Start() (err error) {
	q.cmd = exec.Command("qemu-system-"+string(q.arch),
		// TODO
		"-snapshot",
		"-nographic")

	if q.pipe.stdin, err = q.cmd.StdinPipe(); err != nil {
		return
	}

	if q.pipe.stdout, err = q.cmd.StdoutPipe(); err != nil {
		return
	}

	if q.pipe.stderr, err = q.cmd.StderrPipe(); err != nil {
		return
	}

	err = q.cmd.Start()
	if err != nil {
		return
	}

	go func() {
		q.Stdout, _ = readUntilEOF(q.pipe.stdout)
		q.Stderr, _ = readUntilEOF(q.pipe.stderr)
		q.exitErr = q.cmd.Wait()
		q.died = true
	}()

	time.Sleep(time.Second / 10) // wait for immediately die

	if q.died {
		err = errors.New("qemu died immediately: " + q.Stderr)
	}

	return
}

// Stop qemu process
func (q *QemuSystem) Stop() {
	// 1  00/01   01  01  SOH  (Ctrl-A)  START OF HEADING
	fmt.Fprintf(q.pipe.stdin, "%cx", 1)
	// wait for die
	time.Sleep(time.Second / 10)
	if !q.died {
		q.cmd.Process.Signal(syscall.SIGTERM)
		time.Sleep(time.Second / 10)
		q.cmd.Process.Signal(syscall.SIGKILL)
	}
}
