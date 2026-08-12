//go:build linux || freebsd

package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

type realProcessTimer struct {
	timer *time.Timer
}

func (t realProcessTimer) Chan() <-chan time.Time { return t.timer.C }
func (t realProcessTimer) Stop() bool             { return t.timer.Stop() }

type realProcessClock struct{}

func (realProcessClock) NewTimer(duration time.Duration) processTimer {
	return realProcessTimer{timer: time.NewTimer(duration)}
}

type osProcessStarter struct{}

func (osProcessStarter) Start(invocation processInvocation) (runningProcess, error) {
	if !filepath.IsAbs(invocation.path) {
		return nil, errors.New("process executable path must be absolute")
	}
	cmd := exec.Command(invocation.path, invocation.args...)
	cmd.Stdout = invocation.stdout
	cmd.Stderr = invocation.stderr
	// Setsid creates the dedicated group targeted below. Descendants that create
	// another session or process group have escaped and cannot be controlled.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.WaitDelay = invocation.waitDelay
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	process := &osRunningProcess{
		pid:     cmd.Process.Pid,
		process: cmd.Process,
		wait:    make(chan error, 1),
	}
	go func() {
		process.wait <- cmd.Wait()
	}()
	return process, nil
}

type osRunningProcess struct {
	pid     int
	process *os.Process
	wait    chan error
}

func (p *osRunningProcess) Wait() <-chan error { return p.wait }

func (p *osRunningProcess) SignalGroup(signal syscall.Signal) error {
	return syscall.Kill(-p.pid, signal)
}

func (p *osRunningProcess) KillDirect() error { return p.process.Kill() }

func (p *osRunningProcess) GroupAlive() (bool, error) {
	err := syscall.Kill(-p.pid, 0)
	if err == nil || errors.Is(err, syscall.EPERM) {
		return true, nil
	}
	if errors.Is(err, syscall.ESRCH) {
		return false, nil
	}
	return false, err
}
