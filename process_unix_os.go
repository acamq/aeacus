//go:build linux || freebsd

package main

import (
	"errors"
	"os/exec"
	"path/filepath"
	"sync"
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
	process := &osRunningProcess{pid: cmd.Process.Pid, cmd: cmd, wait: make(chan error, 1)}
	return process, nil
}

type osRunningProcess struct {
	pid      int
	cmd      *exec.Cmd
	wait     chan error
	waitOnce sync.Once
}

func (p *osRunningProcess) PID() int { return p.pid }
func (p *osRunningProcess) Wait() <-chan error {
	p.waitOnce.Do(func() {
		go func() { p.wait <- p.cmd.Wait() }()
	})
	return p.wait
}

func (p *osRunningProcess) KillDirect() error { return p.cmd.Process.Kill() }
