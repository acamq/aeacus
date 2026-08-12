//go:build linux

package main

import (
	"errors"
	"sync"
	"syscall"

	"golang.org/x/sys/unix"
)

type osProcessGroupFactory struct{}

func (osProcessGroupFactory) Open(pid int) (processGroup, error) {
	pidfd, err := unix.PidfdOpen(pid, 0)
	if err != nil {
		return nil, err
	}
	group := &linuxProcessGroup{
		pid: pid, pidfd: pidfd, exited: make(chan error, 1), done: make(chan struct{}),
	}
	go group.observeLeader()
	return group, nil
}

type linuxProcessGroup struct {
	pid    int
	pidfd  int
	exited chan error
	done   chan struct{}
	close  sync.Once
}

func (g *linuxProcessGroup) LeaderExited() <-chan error { return g.exited }
func (g *linuxProcessGroup) Close() {
	g.close.Do(func() {
		unix.Close(g.pidfd)
		if g.done != nil {
			<-g.done
		}
	})
}

func (g *linuxProcessGroup) observeLeader() {
	defer close(g.done)
	var info unix.Siginfo
	err := unix.Waitid(unix.P_PIDFD, g.pidfd, &info, unix.WEXITED|unix.WNOWAIT, nil)
	g.exited <- err
}

func (g *linuxProcessGroup) Signal(signal syscall.Signal) error {
	err := syscall.Kill(-g.pid, signal)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}

func (g *linuxProcessGroup) Alive() (bool, error) {
	entries, err := readLinuxProcesses()
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if entry.group == g.pid && (entry.pid != g.pid || entry.state != 'Z') {
			return true, nil
		}
	}
	return false, nil
}
