//go:build freebsd

package main

import (
	"encoding/binary"
	"errors"
	"os"
	"runtime"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestFreeBSDProcessGroupCloseWakesAndJoinsObserver(t *testing.T) {
	initialFDs, err := os.ReadDir("/dev/fd")
	if err != nil {
		t.Fatal(err)
	}
	initialGoroutines := runtime.NumGoroutine()
	for iteration := range 1000 {
		opened, openError := (osProcessGroupFactory{}).Open(os.Getpid())
		if openError != nil {
			t.Fatal(openError)
		}
		group := opened.(*freeBSDProcessGroup)
		closed := make(chan struct{})
		go func() {
			group.Close()
			close(closed)
		}()
		select {
		case <-closed:
		case <-time.After(time.Second):
			t.Fatalf("Close blocked at iteration %d", iteration)
		}
		select {
		case <-group.done:
		default:
			t.Fatalf("Close returned before observer joined at iteration %d", iteration)
		}
		group.Close()
	}
	runtime.GC()
	finalFDs, err := os.ReadDir("/dev/fd")
	if err != nil {
		t.Fatal(err)
	}
	if len(finalFDs) != len(initialFDs) {
		t.Fatalf("fd count changed from %d to %d", len(initialFDs), len(finalFDs))
	}
	if final := runtime.NumGoroutine(); final > initialGoroutines+1 {
		t.Fatalf("goroutine count changed from %d to %d", initialGoroutines, final)
	}
}

func TestFreeBSDProcessRecordsClassifyExecutableGroupMembers(t *testing.T) {
	tests := []struct {
		name         string
		records      []byte
		leaderExited bool
		wantAlive    bool
		wantError    error
	}{
		{"running leader", freeBSDProcessFixture(41, 41, freeBSDProcessRunning), false, true, nil},
		{"unobserved leader zombie", freeBSDProcessFixture(41, 41, freeBSDProcessZombie), false, true, nil},
		{"observed leader zombie", freeBSDProcessFixture(41, 41, freeBSDProcessZombie), true, false, nil},
		{"live descendant", append(freeBSDProcessFixture(41, 41, freeBSDProcessZombie), freeBSDProcessFixture(42, 41, freeBSDProcessSleeping)...), true, true, nil},
		{"descendant zombie", append(freeBSDProcessFixture(41, 41, freeBSDProcessZombie), freeBSDProcessFixture(42, 41, freeBSDProcessZombie)...), true, false, nil},
		{"wrong process group", freeBSDProcessFixture(42, 99, freeBSDProcessRunning), true, false, syscall.EINVAL},
		{"wrong record size", append([]byte{0, 8, 0, 0}, make([]byte, freeBSDKinfoProcSize-4)...), true, false, syscall.EINVAL},
		{"truncated record", []byte{8, 0, 0, 0, 0, 0, 0, 0}, true, false, syscall.EINVAL},
		{"empty group", nil, true, false, nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			alive, err := freeBSDExecutableGroupAlive(test.records, 41, test.leaderExited)
			if !errors.Is(err, test.wantError) {
				t.Fatalf("error got %v, want %v", err, test.wantError)
			}
			if alive != test.wantAlive {
				t.Fatalf("alive got %v, want %v", alive, test.wantAlive)
			}
		})
	}
}

func freeBSDProcessFixture(pid, pgid int, status byte) []byte {
	record := make([]byte, freeBSDKinfoProcSize)
	binary.NativeEndian.PutUint32(record[freeBSDKinfoStructSizeOffset:], freeBSDKinfoProcSize)
	binary.NativeEndian.PutUint32(record[freeBSDKinfoPIDOffset:], uint32(pid))
	binary.NativeEndian.PutUint32(record[freeBSDKinfoPGIDOffset:], uint32(pgid))
	record[freeBSDKinfoStatusOffset] = status
	return record
}

func processTestPIDExecutable(pid int) (bool, error) {
	data, err := unix.SysctlRaw("kern.proc.pid", pid)
	if errors.Is(err, syscall.ESRCH) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	for len(data) != 0 {
		if len(data) < freeBSDKinfoStatusOffset+1 {
			return false, syscall.EINVAL
		}
		size := int(binary.NativeEndian.Uint32(data[freeBSDKinfoStructSizeOffset:]))
		if size < freeBSDKinfoStatusOffset+1 || size > len(data) {
			return false, syscall.EINVAL
		}
		if int(int32(binary.NativeEndian.Uint32(data[freeBSDKinfoPIDOffset:]))) == pid {
			return data[freeBSDKinfoStatusOffset] != freeBSDProcessZombie, nil
		}
		data = data[size:]
	}
	return false, nil
}
