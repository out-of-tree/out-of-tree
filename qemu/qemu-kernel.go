// Copyright 2018 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package qemu

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/povsister/scp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/ssh"
)

type arch string

const (
	// X86x64 is the qemu-system-x86_64
	X86x64 arch = "x86_64"
	// X86x32 is the qemu-system-i386
	X86x32 = "i386"
	// TODO add other

	unsupported = "unsupported" // for test purposes
)

const (
	DefaultCPUs            = 1
	DefaultMemory          = 512 // megabytes
	DefaultSSHRetries      = 4
	DefaultSSHRetryTimeout = time.Second / 4
)

// Kernel describe kernel parameters for qemu
type Kernel struct {
	Name       string
	KernelPath string
	InitrdPath string
}

type CPU struct {
	Model string
	Flags []string
}

// System describe qemu parameters and executed process
type System struct {
	arch      arch
	kernel    Kernel
	drivePath string

	Mutable bool

	Cpus   int
	Memory int

	CPU CPU

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

	Died bool

	SSH struct {
		AddrPort     string
		Retries      int
		RetryTimeout time.Duration
	}

	// accessible while qemu is running
	cmd  *exec.Cmd
	pipe struct {
		stdin  io.WriteCloser
		stderr io.ReadCloser
		stdout io.ReadCloser
	}

	Stdout, Stderr string

	qemuOutput struct {
		listener chan string
		mu       sync.Mutex
	}

	commandsOutput struct {
		listener chan string
		mu       sync.Mutex
	}

	// accessible after qemu is closed
	exitErr error

	Log zerolog.Logger
}

// NewSystem constructor
func NewSystem(arch arch, kernel Kernel, drivePath string) (q *System, err error) {
	q = &System{}
	q.Log = log.With().
		Str("kernel", kernel.KernelPath).
		Logger()

	if _, err = exec.LookPath("qemu-system-" + string(arch)); err != nil {
		return
	}

	q.arch = arch

	if _, err = os.Stat(kernel.KernelPath); err != nil {
		return
	}
	q.kernel = kernel

	if _, err = os.Stat(drivePath); err != nil {
		return
	}
	q.drivePath = drivePath

	q.Cpus = DefaultCPUs
	q.Memory = DefaultMemory
	q.SSH.Retries = DefaultSSHRetries
	q.SSH.RetryTimeout = DefaultSSHRetryTimeout

	return
}

// q.SetQemuOutputHandler(func(s string) { fmt.Println(s) })
// defer q.CloseQemuOutputHandler()
func (q *System) SetQemuOutputHandler(handler func(s string)) {
	q.qemuOutput.mu.Lock()
	defer q.qemuOutput.mu.Unlock()

	q.qemuOutput.listener = make(chan string)

	go func(l chan string) {
		for m := range l {
			if m != "" {
				handler(m)
			}
		}
	}(q.qemuOutput.listener)
}

func (q *System) CloseQemuOutputHandler() {
	q.qemuOutput.mu.Lock()
	defer q.qemuOutput.mu.Unlock()

	close(q.qemuOutput.listener)
	q.qemuOutput.listener = nil
}

func (q *System) handleQemuOutput(m string) {
	if q.qemuOutput.listener == nil {
		return
	}
	q.qemuOutput.mu.Lock()
	defer q.qemuOutput.mu.Unlock()

	if q.qemuOutput.listener != nil {
		q.qemuOutput.listener <- m
	}
}

// q.SetCommandsOutputHandler(func(s string) { fmt.Println(s) })
// defer q.CloseCommandsOutputHandler()
func (q *System) SetCommandsOutputHandler(handler func(s string)) {
	q.commandsOutput.mu.Lock()
	defer q.commandsOutput.mu.Unlock()

	q.commandsOutput.listener = make(chan string)

	go func(l chan string) {
		for m := range l {
			if m != "" {
				handler(m)
			}
		}
	}(q.commandsOutput.listener)
}

func (q *System) CloseCommandsOutputHandler() {
	q.commandsOutput.mu.Lock()
	defer q.commandsOutput.mu.Unlock()

	close(q.commandsOutput.listener)
	q.commandsOutput.listener = nil
}

func (q *System) handleCommandsOutput(m string) {
	if q.commandsOutput.listener == nil {
		return
	}
	q.commandsOutput.mu.Lock()
	defer q.commandsOutput.mu.Unlock()

	if q.commandsOutput.listener != nil {
		q.commandsOutput.listener <- m
	}
}

func (q *System) SetSSHAddrPort(addr string, port int) (err error) {
	// TODO validate
	q.SSH.AddrPort = fmt.Sprintf("%s:%d", addr, port)
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

func GetFreeAddrPort() (addrPort string) {
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

func (q *System) checkOopsPanic(s string) {
	if strings.Contains(s, "Kernel panic") {
		q.KernelPanic = true
		q.Log.Warn().Msg("kernel panic")
		time.Sleep(time.Second)
		// There is no reason to stay alive after kernel panic
		q.Stop()
	} else if strings.Contains(s, "BUG") {
		q.Log.Warn().Msg(s)
		time.Sleep(time.Second)
		// consider BUG() as non-recoverable state
		q.Stop()
	} else if strings.Contains(s, "WARNING") {
		q.Log.Warn().Msg(s)
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

func (q System) Executable() string {
	return "qemu-system-" + string(q.arch)
}

func (q *System) Args() (qemuArgs []string) {
	if q.SSH.AddrPort == "" {
		q.SSH.AddrPort = GetFreeAddrPort()
	}
	hostfwd := fmt.Sprintf("hostfwd=tcp:%s-:22", q.SSH.AddrPort)
	qemuArgs = []string{"-nographic",
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

	cpu := "max"
	if q.CPU.Model != "" {
		cpu = q.CPU.Model
	}
	for _, flag := range q.CPU.Flags {
		cpu += "," + flag
	}
	qemuArgs = append(qemuArgs, "-cpu", cpu)

	if q.arch == X86x64 || q.arch == X86x32 {
		if kvmExists() {
			qemuArgs = append(qemuArgs, "-enable-kvm")
		}
	}

	if q.arch == X86x64 && runtime.GOOS == "darwin" {
		qemuArgs = append(qemuArgs, "-accel", "hvf")
	}

	qemuArgs = append(qemuArgs, "-append", q.cmdline())
	return
}

// Start qemu process
func (q *System) Start() (err error) {
	q.cmd = exec.Command(q.Executable(), q.Args()...)
	q.Log.Debug().Msgf("%v", q.cmd)

	if q.pipe.stdin, err = q.cmd.StdinPipe(); err != nil {
		return
	}

	if q.pipe.stdout, err = q.cmd.StdoutPipe(); err != nil {
		return
	}

	if q.pipe.stderr, err = q.cmd.StderrPipe(); err != nil {
		return
	}

	q.Log.Debug().Msg("start qemu")
	err = q.cmd.Start()
	if err != nil {
		return
	}

	go func() {
		scanner := bufio.NewScanner(q.pipe.stdout)
		for scanner.Scan() {
			m := strings.TrimSpace(scanner.Text())
			q.handleQemuOutput(m)
			q.Stdout += m + "\n"
			q.Log.Trace().Str("stdout", m).Msg("qemu")
			go q.checkOopsPanic(m)
		}
	}()

	go func() {
		scanner := bufio.NewScanner(q.pipe.stderr)
		for scanner.Scan() {
			m := strings.TrimSpace(scanner.Text())
			q.handleQemuOutput(m)
			q.Stderr += m + "\n"
			q.Log.Trace().Str("stderr", m).Msg("qemu")
		}
	}()

	go func() {
		q.exitErr = q.cmd.Wait()
		q.Died = true
		q.Log.Debug().Msg("qemu died")
	}()

	time.Sleep(time.Second / 10) // wait for immediately die

	if q.Died {
		err = errors.New("qemu died immediately: " + string(q.Stderr))
	}

	if q.Timeout != 0 {
		go func() {
			q.Log.Debug().Msgf("qemu wait for %s before kill", q.Timeout)
			time.Sleep(q.Timeout)
			q.KilledByTimeout = true
			q.Stop()
		}()
	}

	return
}

// Stop qemu process
func (q *System) Stop() {
	q.Log.Debug().Msg("stop qemu process")

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

func (q *System) WaitForSSH(timeout time.Duration) error {
	q.Log.Debug().Msgf("wait for ssh for %v", timeout)

	for start := time.Now(); time.Since(start) < timeout; {
		time.Sleep(time.Second / 4)

		if q.Died || q.KernelPanic {
			return errors.New("no ssh (qemu is dead)")
		}

		client, err := q.ssh("root")
		if err != nil {
			q.Log.Debug().Err(err).Msg("")
			continue
		}

		session, err := client.NewSession()
		if err != nil {
			client.Close()
			q.Log.Debug().Err(err).Msg("")
			continue
		}

		_, err = session.CombinedOutput("echo")
		if err != nil {
			client.Close()
			q.Log.Debug().Err(err).Msg("")
			continue
		}

		client.Close()
		q.Log.Debug().Msg("ssh is available")
		return nil
	}

	return errors.New("no ssh (timeout)")
}

func (q *System) ssh(user string) (client *ssh.Client, err error) {
	cfg := &ssh.ClientConfig{
		User:            user,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	for retries := q.SSH.Retries; retries > 0; retries-- {
		if q.Died {
			err = errors.New("qemu is dead")
			return
		}

		client, err = ssh.Dial("tcp", q.SSH.AddrPort, cfg)
		if err == nil {
			break
		}
		time.Sleep(q.SSH.RetryTimeout)
	}
	return
}

// Command executes shell commands on qemu system
func (q System) Command(user, cmd string) (output string, err error) {
	flog := q.Log.With().Str("kernel", q.kernel.KernelPath).
		Str("user", user).
		Str("cmd", cmd).
		Logger()

	flog.Debug().Msg("qemu command")

	client, err := q.ssh(user)
	if err != nil {
		flog.Debug().Err(err).Msg("ssh connection")
		return
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		flog.Debug().Err(err).Msg("new session")
		return
	}

	stdout, err := session.StdoutPipe()
	if err != nil {
		flog.Debug().Err(err).Msg("get stdout pipe")
		return
	}

	stderr, err := session.StderrPipe()
	if err != nil {
		flog.Debug().Err(err).Msg("get stderr pipe")
		return
	}

	err = session.Start(cmd)
	if err != nil {
		flog.Debug().Err(err).Msg("start session")
		return
	}

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()

		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			m := strings.TrimSpace(scanner.Text())
			q.handleCommandsOutput(m)
			output += m + "\n"
			flog.Trace().Str("stdout", m).Msg("qemu command")
		}
		output = strings.TrimSuffix(output, "\n")
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			m := strings.TrimSpace(scanner.Text())
			q.handleCommandsOutput(m)
			output += m + "\n"
			// Note: it prints stderr as stdout
			flog.Trace().Str("stdout", m).Msg("qemu command")
		}
		output = strings.TrimSuffix(output, "\n")
	}()

	err = session.Wait()

	wg.Wait()
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
	q.Log.Debug().Msgf("scp[%s] %s -> %s", user, localPath, remotePath)

	sshClient, err := q.ssh(user)
	if err != nil {
		return
	}
	defer sshClient.Close()

	client, err := scp.NewClientFromExistingSSH(sshClient, &scp.ClientOption{})
	if err != nil {
		return
	}

	if recursive {
		err = client.CopyDirToRemote(
			localPath,
			remotePath,
			&scp.DirTransferOption{},
		)
	} else {
		err = client.CopyFileToRemote(
			localPath,
			remotePath,
			&scp.FileTransferOption{},
		)
	}
	return
}

func (q *System) scpWithRetry(user, localPath, remotePath string, recursive bool) (err error) {
	for retries := q.SSH.Retries; retries > 0; retries-- {
		if q.Died {
			err = errors.New("qemu is dead")
			return
		}

		err = q.scp(user, localPath, remotePath, recursive)
		if err == nil {
			break
		}

		q.Log.Debug().Err(err).Msgf(
			"scp[%v] (r: %v) %v -> %v: failed",
			user, recursive, localPath, remotePath)

		time.Sleep(q.SSH.RetryTimeout)
		q.Log.Warn().Msgf("scp: %d retries left", retries)
	}
	return
}

// CopyFile from local machine to remote via scp
func (q System) CopyFile(user, localPath, remotePath string) (err error) {
	return q.scpWithRetry(user, localPath, remotePath, false)
}

// CopyDirectory from local machine to remote via scp
func (q System) CopyDirectory(user, localPath, remotePath string) (err error) {
	return q.scpWithRetry(user, localPath, remotePath, true)
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

// CopyAndRunAsync is copy local file to qemu vm then run it w/o wait for exit
func (q *System) CopyAndRunAsync(user, path string) (err error) {
	remotePath := fmt.Sprintf("/tmp/executable_%d", rand.Int())
	err = q.CopyFile(user, path, remotePath)
	if err != nil {
		return
	}

	return q.AsyncCommand(user, "chmod +x "+remotePath+" && "+remotePath)
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
	addrPort := strings.Split(q.SSH.AddrPort, ":")
	addr := addrPort[0]
	port := addrPort[1]

	cmd = "ssh -o StrictHostKeyChecking=no"
	cmd += " -p " + port + " root@" + addr
	return
}
