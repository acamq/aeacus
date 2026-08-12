//go:build linux

package main

import (
	"errors"
	"syscall"
)

func processTestPIDExecutable(pid int) (bool, error) {
	err := syscall.Kill(pid, 0)
	if errors.Is(err, syscall.ESRCH) {
		return false, nil
	}
	return err == nil, err
}
