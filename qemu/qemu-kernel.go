// Copyright 2018 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package qemu

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/ssh"
)

func readUntilEOF(pipe io.ReadCloser, buf *[]byte) (err error) {
	bufSize := 1024
	for err != io.EOF {
		stdout := make([]byte, bufSize)
		var n int

		n, err = pipe.Read(stdout)
		if err != nil && err != io.EOF {
			return
		}

		*buf = append(*buf, stdout[:n]...)
	}

	if err == io.EOF {
		err = nil
	}
	return
}

type arch string

const (
	// X86x64 is the qemu-system-x86_64
	X86x64 arch = "x86_64"
	// X86x32 is the qemu-system-i386
	X86x32 = "i386"
	// TODO add other

	unsupported = "unsupported" // for test purposes
)

// Kernel describe kernel parameters for qemu
type Kernel struct {
	Name       string
	KernelPath string
	InitrdPath string
}

// System describe qemu parameters and executed process
type System struct {
	arch      arch
	kernel    Kernel
	drivePath string

	Mutable bool

	Cpus   int
	Memory int

	debug bool
	gdb   string // tcp::1234

	noKASLR bool
	noSMEP  bool
	noSMAP  bool
	noKPTI  bool

	// Timeout works after Start invocation
	Timeout         time.Duration
	KilledByTimeout bool

	KernelPanic bool

	Died        bool
	sshAddrPort string

	// accessible while qemu is running
	cmd  *exec.Cmd
	pipe struct {
		stdin  io.WriteCloser
		stderr io.ReadCloser
		stdout io.ReadCloser
	}

	Stdout, Stderr []byte

	// accessible after qemu is closed
	exitErr error
}

// NewSystem constructor
func NewSystem(arch arch, kernel Kernel, drivePath string) (q *System, err error) {
	if _, err = exec.LookPath("qemu-system-" + string(arch)); err != nil {
		return
	}
	q = &System{}
	q.arch = arch

	if _, err = os.Stat(kernel.KernelPath); err != nil {
		return
	}
	q.kernel = kernel

	if _, err = os.Stat(drivePath); err != nil {
		return
	}
	q.drivePath = drivePath

	// Default values
	q.Cpus = 1
	q.Memory = 512 // megabytes

	return
}

func (q *System) SetSSHAddrPort(addr string, port int) (err error) {
	// TODO validate
	q.sshAddrPort = fmt.Sprintf("%s:%d", addr, port)
	return
}

func getRandomAddrPort() (addr string) {
	// 127.1-255.0-255.0-255:10000-50000
	ip := fmt.Sprintf("127.%d.%d.%d",
		rand.Int()%254+1, rand.Int()%255, rand.Int()%254)
	port := rand.Int()%40000 + 10000
	return fmt.Sprintf("%s:%d", ip, port)
}

func getRandomPort(ip string) (addr string) {
	// ip:1024-65535
	port := rand.Int()%(65536-1024) + 1024
	return fmt.Sprintf("%s:%d", ip, port)
}

func getFreeAddrPort() (addrPort string) {
	timeout := time.Now().Add(time.Second)
	for {
		if runtime.GOOS == "linux" {
			addrPort = getRandomAddrPort()
		} else {
			addrPort = getRandomPort("127.0.0.1")
		}
		ln, err := net.Listen("tcp", addrPort)
		if err == nil {
			ln.Close()
			return
		}

		if time.Now().After(timeout) {
			panic("Can't found free address:port on localhost")
		}
	}
}

func kvmExists() bool {
	if _, err := os.Stat("/dev/kvm"); err != nil {
		return false
	}

	file, err := os.OpenFile("/dev/kvm", os.O_WRONLY, 0666)
	if err != nil {
		if os.IsPermission(err) {
			return false
		}
	}
	file.Close()

	return true
}

func (q *System) panicWatcher() {
	for {
		time.Sleep(time.Second)
		if bytes.Contains(q.Stdout, []byte("Kernel panic")) {
			time.Sleep(time.Second)
			// There is no reason to stay alive after kernel panic
			q.Stop()
			q.KernelPanic = true
			return
		}
	}
}

func (q System) cmdline() (s string) {
	s = "root=/dev/sda ignore_loglevel console=ttyS0 rw"

	if q.noKASLR {
		s += " nokaslr"
	}

	if q.noSMEP {
		s += " nosmep"
	}

	if q.noSMAP {
		s += " nosmap"
	}

	if q.noKPTI {
		s += " nokpti"
	}

	return
}

// Start qemu process
func (q *System) Start() (err error) {
	rand.Seed(time.Now().UnixNano()) // Are you sure?
	if q.sshAddrPort == "" {
		q.sshAddrPort = getFreeAddrPort()
	}
	hostfwd := fmt.Sprintf("hostfwd=tcp:%s-:22", q.sshAddrPort)
	qemuArgs := []string{"-nographic",
		"-hda", q.drivePath,
		"-kernel", q.kernel.KernelPath,
		"-smp", fmt.Sprintf("%d", q.Cpus),
		"-m", fmt.Sprintf("%d", q.Memory),
		"-device", "e1000,netdev=n1",
		"-netdev", "user,id=n1," + hostfwd,
	}

	if !q.Mutable {
		qemuArgs = append(qemuArgs, "-snapshot")
	}

	if q.debug {
		qemuArgs = append(qemuArgs, "-gdb", q.gdb)
	}

	if q.kernel.InitrdPath != "" {
		qemuArgs = append(qemuArgs, "-initrd", q.kernel.InitrdPath)
	}

	if (q.arch == X86x64 || q.arch == X86x32) && kvmExists() {
		qemuArgs = append(qemuArgs, "-enable-kvm", "-cpu", "host")
	}

	if q.arch == X86x64 && runtime.GOOS == "darwin" {
		qemuArgs = append(qemuArgs, "-accel", "hvf", "-cpu", "host")
	}

	qemuArgs = append(qemuArgs, "-append", q.cmdline())

	q.cmd = exec.Command("qemu-system-"+string(q.arch), qemuArgs...)
	log.Debug().Msgf("%v", q.cmd)

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

	go readUntilEOF(q.pipe.stdout, &q.Stdout)
	go readUntilEOF(q.pipe.stderr, &q.Stderr)

	go func() {
		q.exitErr = q.cmd.Wait()
		q.Died = true
	}()

	time.Sleep(time.Second / 10) // wait for immediately die

	if q.Died {
		err = errors.New("qemu died immediately: " + string(q.Stderr))
	}

	go q.panicWatcher()

	if q.Timeout != 0 {
		go func() {
			time.Sleep(q.Timeout)
			q.KilledByTimeout = true
			q.Stop()
		}()
	}

	return
}

// Stop qemu process
func (q *System) Stop() {
	// 1  00/01   01  01  SOH  (Ctrl-A)  START OF HEADING
	fmt.Fprintf(q.pipe.stdin, "%cx", 1)
	// wait for die
	time.Sleep(time.Second / 10)
	if !q.Died {
		q.cmd.Process.Signal(syscall.SIGTERM)
		time.Sleep(time.Second / 10)
		q.cmd.Process.Signal(syscall.SIGKILL)
	}
}

func (q System) WaitForSSH(timeout time.Duration) error {
	for start := time.Now(); time.Since(start) < timeout; {
		client, err := q.ssh("root")
		if err != nil {
			time.Sleep(time.Second / 10)
			continue
		}
		client.Close()
		return nil
	}

	return errors.New("no ssh (timeout)")
}

func (q System) ssh(user string) (client *ssh.Client, err error) {
	cfg := &ssh.ClientConfig{
		User:            user,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	client, err = ssh.Dial("tcp", q.sshAddrPort, cfg)
	return
}

// Command executes shell commands on qemu system
func (q System) Command(user, cmd string) (output string, err error) {
	log.Debug().Str("kernel", q.kernel.KernelPath).
		Str("user", user).Str("cmd", cmd).Msg("qemu command")

	client, err := q.ssh(user)
	if err != nil {
		return
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return
	}

	bytesOutput, err := session.CombinedOutput(cmd)
	output = string(bytesOutput)
	return
}

// AsyncCommand executes command on qemu system but does not wait for exit
func (q System) AsyncCommand(user, cmd string) (err error) {
	client, err := q.ssh(user)
	if err != nil {
		return
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return
	}

	return session.Run(fmt.Sprintf(
		"nohup sh -c '%s' > /dev/null 2> /dev/null < /dev/null &", cmd))
}

func (q System) scp(user, localPath, remotePath string, recursive bool) (err error) {
	addrPort := strings.Split(q.sshAddrPort, ":")
	addr := addrPort[0]
	port := addrPort[1]

	args := []string{
		"-P", port,
		"-o", "StrictHostKeyChecking=no",
		"-o", "LogLevel=error",
	}

	if recursive {
		var output []byte
		output, err = exec.Command("ssh", "-V").CombinedOutput()
		if err != nil {
			return
		}
		sshVersion := string(output)

		log.Debug().Str("ssh version", sshVersion).Msg("")

		if strings.Contains(sshVersion, "OpenSSH_9") {
			// This release switches scp from using the
			// legacy scp/rcp protocol to using the SFTP
			// protocol by default.
			//
			// To keep compatibility with old distros,
			// using -O flag to use the legacy scp/rcp.
			//
			// Note: old ssh doesn't support -O flag
			args = append(args, "-O")
		}

		args = append(args, "-r")
	}

	args = append(args, localPath, user+"@"+addr+":"+remotePath)

	cmd := exec.Command("scp", args...)
	log.Debug().Msgf("%v", cmd)

	output, err := cmd.CombinedOutput()
	if err != nil || string(output) != "" {
		return errors.New(string(output))
	}

	return
}

// CopyFile from local machine to remote via scp
func (q System) CopyFile(user, localPath, remotePath string) (err error) {
	return q.scp(user, localPath, remotePath, false)
}

// CopyDirectory from local machine to remote via scp
func (q System) CopyDirectory(user, localPath, remotePath string) (err error) {
	return q.scp(user, localPath, remotePath, true)
}

// CopyAndInsmod copy kernel module to temporary file on qemu then insmod it
func (q *System) CopyAndInsmod(localKoPath string) (output string, err error) {
	remoteKoPath := fmt.Sprintf("/tmp/module_%d.ko", rand.Int())
	err = q.CopyFile("root", localKoPath, remoteKoPath)
	if err != nil {
		return
	}

	return q.Command("root", "insmod "+remoteKoPath)
}

// CopyAndRun is copy local file to qemu vm then run it
func (q *System) CopyAndRun(user, path string) (output string, err error) {
	remotePath := fmt.Sprintf("/tmp/executable_%d", rand.Int())
	err = q.CopyFile(user, path, remotePath)
	if err != nil {
		return
	}

	return q.Command(user, "chmod +x "+remotePath+" && "+remotePath)
}

// Debug is for enable qemu debug and set hostname and port for listen
func (q *System) Debug(conn string) {
	q.debug = true
	q.gdb = conn
}

// SetKASLR is changing KASLR state through kernel boot args
func (q *System) SetKASLR(state bool) {
	q.noKASLR = !state
}

// SetSMEP is changing SMEP state through kernel boot args
func (q *System) SetSMEP(state bool) {
	q.noSMEP = !state
}

// SetSMAP is changing SMAP state through kernel boot args
func (q *System) SetSMAP(state bool) {
	q.noSMAP = !state
}

// SetKPTI is changing KPTI state through kernel boot args
func (q *System) SetKPTI(state bool) {
	q.noKPTI = !state
}

// GetKASLR is retrieve KASLR settings
func (q *System) GetKASLR() bool {
	return !q.noKASLR
}

// GetSMEP is retrieve SMEP settings
func (q *System) GetSMEP() bool {
	return !q.noSMEP
}

// GetSMAP is retrieve SMAP settings
func (q *System) GetSMAP() bool {
	return !q.noSMAP
}

// GetKPTI is retrieve KPTI settings
func (q *System) GetKPTI() bool {
	return !q.noKPTI
}

// GetSSHCommand returns command for connect to qemu machine over ssh
func (q System) GetSSHCommand() (cmd string) {
	addrPort := strings.Split(q.sshAddrPort, ":")
	addr := addrPort[0]
	port := addrPort[1]

	cmd = "ssh -o StrictHostKeyChecking=no"
	cmd += " -p " + port + " root@" + addr
	return
}
