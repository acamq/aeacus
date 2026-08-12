//go:build freebsd

package main

import (
	"encoding/binary"
	"syscall"
)

const (
	freeBSDKinfoProcSize         = 1088
	freeBSDKinfoStructSizeOffset = 0
	freeBSDKinfoPIDOffset        = 72
	freeBSDKinfoPGIDOffset       = 80
	freeBSDKinfoStatusOffset     = 388
	freeBSDProcessIdle           = 1
	freeBSDProcessRunning        = 2
	freeBSDProcessSleeping       = 3
	freeBSDProcessStopped        = 4
	freeBSDProcessZombie         = 5
	freeBSDProcessWaiting        = 6
	freeBSDProcessLocked         = 7
)

func freeBSDExecutableGroupAlive(data []byte, leaderPID int, leaderExited bool) (bool, error) {
	for len(data) != 0 {
		if len(data) < freeBSDKinfoStatusOffset+1 {
			return false, syscall.EINVAL
		}
		size := int(binary.NativeEndian.Uint32(data[freeBSDKinfoStructSizeOffset:]))
		if size != freeBSDKinfoProcSize || size > len(data) {
			return false, syscall.EINVAL
		}
		pid := int(int32(binary.NativeEndian.Uint32(data[freeBSDKinfoPIDOffset:])))
		pgid := int(int32(binary.NativeEndian.Uint32(data[freeBSDKinfoPGIDOffset:])))
		status := data[freeBSDKinfoStatusOffset]
		if pid <= 0 || pgid != leaderPID || status < freeBSDProcessIdle || status > freeBSDProcessLocked {
			return false, syscall.EINVAL
		}
		if pid == leaderPID && !leaderExited || pid != leaderPID && status != freeBSDProcessZombie {
			return true, nil
		}
		data = data[size:]
	}
	return false, nil
}
