//go:build linux

package main

import (
	"errors"
	"syscall"

	"golang.org/x/sys/unix"
)

type osGroupSignalFactory struct{}

func (osGroupSignalFactory) Open(pid int) (groupSignalTarget, error) {
	fd, err := unix.PidfdOpen(pid, 0)
	if err != nil {
		return nil, err
	}
	return &linuxGroupSignalTarget{pid: pid, pidfd: fd}, nil
}

type linuxGroupSignalTarget struct {
	pid    int
	pidfd  int
	exited bool
}

func (g *linuxGroupSignalTarget) Close() { unix.Close(g.pidfd) }

func (g *linuxGroupSignalTarget) Signal(signal syscall.Signal) error {
	if g.exited {
		return nil
	}
	if err := unix.Waitid(unix.P_PIDFD, g.pidfd, nil, unix.WEXITED|unix.WNOHANG|unix.WNOWAIT, nil); err != nil {
		if errors.Is(err, syscall.ESRCH) || errors.Is(err, syscall.ECHILD) {
			g.exited = true
			return nil
		}
		return err
	}
	err := syscall.Kill(-g.pid, signal)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}
