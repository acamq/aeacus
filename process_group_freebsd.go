//go:build freebsd

package main

import (
	"encoding/binary"
	"errors"
	"sync/atomic"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

var hostByteOrder = func() binary.ByteOrder {
	var value uint16 = 1
	if *(*byte)(unsafe.Pointer(&value)) == 1 {
		return binary.LittleEndian
	}
	return binary.BigEndian
}()

const pointerSize = 8

type osProcessGroupFactory struct{}

func (osProcessGroupFactory) Open(pid int) (processGroup, error) {
	kqueue, err := unix.Kqueue()
	if err != nil {
		return nil, err
	}
	change := unix.Kevent_t{Ident: uint64(pid), Filter: unix.EVFILT_PROC, Flags: unix.EV_ADD | unix.EV_ONESHOT, Fflags: unix.NOTE_EXIT}
	if _, err := unix.Kevent(kqueue, []unix.Kevent_t{change}, nil, nil); err != nil {
		unix.Close(kqueue)
		return nil, err
	}
	group := &freeBSDProcessGroup{pid: pid, kqueue: kqueue, exited: make(chan error, 1), changed: make(chan struct{})}
	go group.observeLeader()
	return group, nil
}

type freeBSDProcessGroup struct {
	pid          int
	kqueue       int
	exited       chan error
	changed      chan struct{}
	leaderExited atomic.Bool
}

func (g *freeBSDProcessGroup) LeaderExited() <-chan error { return g.exited }
func (g *freeBSDProcessGroup) Changed() <-chan struct{}   { return g.changed }
func (g *freeBSDProcessGroup) Close()                     { unix.Close(g.kqueue) }

func (g *freeBSDProcessGroup) observeLeader() {
	events := make([]unix.Kevent_t, 1)
	count, err := unix.Kevent(g.kqueue, nil, events, nil)
	if err == nil && count == 1 {
		g.leaderExited.Store(true)
	}
	g.exited <- err
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
	for len(data) >= 8 {
		size := int(hostByteOrder.Uint32(data[:4]))
		if size < 8 || size > len(data) {
			return false, syscall.EINVAL
		}
		pidOffset := 8 + 8*pointerSize
		if size >= pidOffset+4 {
			pid := int(int32(hostByteOrder.Uint32(data[pidOffset : pidOffset+4])))
			if pid != g.pid || !g.leaderExited.Load() {
				return true, nil
			}
		}
		data = data[size:]
	}
	return false, nil
}
