//go:build freebsd

package main

import (
	"errors"
	"sync"
	"sync/atomic"
	"syscall"

	"golang.org/x/sys/unix"
)

const freeBSDCancelEventID = 1

type osProcessGroupFactory struct{}

func (osProcessGroupFactory) Open(pid int) (processGroup, error) {
	kqueue, err := unix.Kqueue()
	if err != nil {
		return nil, err
	}
	changes := []unix.Kevent_t{
		{Ident: uint64(pid), Filter: unix.EVFILT_PROC, Flags: unix.EV_ADD | unix.EV_ONESHOT, Fflags: unix.NOTE_EXIT},
		{Ident: freeBSDCancelEventID, Filter: unix.EVFILT_USER, Flags: unix.EV_ADD},
	}
	if _, err := unix.Kevent(kqueue, changes, nil, nil); err != nil {
		unix.Close(kqueue)
		return nil, err
	}
	group := &freeBSDProcessGroup{
		pid: pid, kqueue: kqueue, exited: make(chan error, 1), done: make(chan struct{}),
	}
	go group.observeLeader()
	return group, nil
}

type freeBSDProcessGroup struct {
	pid          int
	kqueue       int
	exited       chan error
	done         chan struct{}
	leaderExited atomic.Bool
	close        sync.Once
}

func (g *freeBSDProcessGroup) LeaderExited() <-chan error { return g.exited }
func (g *freeBSDProcessGroup) Close() {
	g.close.Do(func() {
		trigger := unix.Kevent_t{
			Ident: freeBSDCancelEventID, Filter: unix.EVFILT_USER, Fflags: unix.NOTE_TRIGGER,
		}
		for {
			_, err := unix.Kevent(g.kqueue, []unix.Kevent_t{trigger}, nil, nil)
			if !errors.Is(err, syscall.EINTR) {
				break
			}
		}
		<-g.done
		unix.Close(g.kqueue)
	})
}

func (g *freeBSDProcessGroup) observeLeader() {
	defer close(g.done)
	events := make([]unix.Kevent_t, 2)
	count, err := unix.Kevent(g.kqueue, nil, events, nil)
	if err != nil {
		g.exited <- err
		return
	}
	for _, event := range events[:count] {
		if event.Filter == unix.EVFILT_PROC && event.Ident == uint64(g.pid) {
			g.leaderExited.Store(true)
			g.exited <- nil
			return
		}
	}
}

func (g *freeBSDProcessGroup) Signal(signal syscall.Signal) error {
	err := syscall.Kill(-g.pid, signal)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}

func (g *freeBSDProcessGroup) Alive() (bool, error) {
	data, err := unix.SysctlRaw("kern.proc.pgrp", g.pid)
	if err != nil {
		return false, err
	}
	return freeBSDExecutableGroupAlive(data, g.pid, g.leaderExited.Load())
}
