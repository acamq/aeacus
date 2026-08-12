//go:build freebsd

package main

import (
	"errors"
	"syscall"
)

type osGroupSignalFactory struct{}

func (osGroupSignalFactory) Open(pid int) (groupSignalTarget, error) {
	return &freeBSDGroupSignalTarget{pid: pid}, nil
}

type freeBSDGroupSignalTarget struct {
	pid int
}

func (g *freeBSDGroupSignalTarget) Close() {}

func (g *freeBSDGroupSignalTarget) Signal(signal syscall.Signal) error {
	err := syscall.Kill(-g.pid, signal)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}
