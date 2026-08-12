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

type freeBSDProcessRecord struct {
	pid    int
	pgid   int
	status byte
}

func freeBSDExecutableGroupAlive(data []byte, leaderPID int, leaderExited bool) (bool, error) {
	for len(data) != 0 {
		record, size, err := parseFreeBSDProcessRecord(data)
		if err != nil {
			return false, err
		}
		if record.pgid != leaderPID {
			return false, syscall.EINVAL
		}
		if record.pid == leaderPID && !leaderExited || record.pid != leaderPID && record.status != freeBSDProcessZombie {
			return true, nil
		}
		data = data[size:]
	}
	return false, nil
}

func freeBSDPIDExecutable(data []byte, pid int) (bool, error) {
	for len(data) != 0 {
		record, size, err := parseFreeBSDProcessRecord(data)
		if err != nil {
			return false, err
		}
		if record.pid == pid {
			return record.status != freeBSDProcessZombie, nil
		}
		data = data[size:]
	}
	return false, nil
}

func parseFreeBSDProcessRecord(data []byte) (freeBSDProcessRecord, int, error) {
	if len(data) < freeBSDKinfoStatusOffset+1 {
		return freeBSDProcessRecord{}, 0, syscall.EINVAL
	}
	size := int(binary.NativeEndian.Uint32(data[freeBSDKinfoStructSizeOffset:]))
	if size != freeBSDKinfoProcSize || size > len(data) {
		return freeBSDProcessRecord{}, 0, syscall.EINVAL
	}
	record := freeBSDProcessRecord{
		pid:    int(int32(binary.NativeEndian.Uint32(data[freeBSDKinfoPIDOffset:]))),
		pgid:   int(int32(binary.NativeEndian.Uint32(data[freeBSDKinfoPGIDOffset:]))),
		status: data[freeBSDKinfoStatusOffset],
	}
	if record.pid <= 0 || record.status < freeBSDProcessIdle || record.status > freeBSDProcessLocked {
		return freeBSDProcessRecord{}, 0, syscall.EINVAL
	}
	return record, size, nil
}
