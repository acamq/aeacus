//go:build freebsd

package main

import (
	"encoding/binary"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"sync"
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
		select {
		case observation := <-group.LeaderExited():
			t.Fatalf("cancellation published leader observation %v at iteration %d", observation, iteration)
		default:
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

func TestFreeBSDProcessGroupCloseAfterLeaderExitJoinsObserver(t *testing.T) {
	command := exec.Command("/bin/sleep", "60")
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	group := openFreeBSDProcessGroup(t, command.Process.Pid)
	if err := command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-group.LeaderExited():
		if err != nil {
			t.Fatalf("observe leader exit: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("leader exit was not observed")
	}
	group.Close()
	if err := command.Wait(); err == nil {
		t.Fatal("killed child returned successful wait")
	}
}

func TestFreeBSDProcessGroupConcurrentCloseIsIdempotent(t *testing.T) {
	group := openFreeBSDProcessGroup(t, os.Getpid())
	var callers sync.WaitGroup
	callers.Add(32)
	for range 32 {
		go func() {
			defer callers.Done()
			group.Close()
		}()
	}
	closed := make(chan struct{})
	go func() {
		callers.Wait()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("concurrent Close calls blocked")
	}
}

func TestFreeBSDProcessGroupCloseUsesPipeWhenUserEventIsMissing(t *testing.T) {
	group := openFreeBSDProcessGroup(t, os.Getpid())
	removeUserEvent := unix.Kevent_t{
		Ident: freeBSDCancelEventID, Filter: unix.EVFILT_USER, Flags: unix.EV_DELETE,
	}
	if _, err := unix.Kevent(group.kqueue, []unix.Kevent_t{removeUserEvent}, nil, nil); err != nil {
		t.Fatal(err)
	}
	closed := make(chan struct{})
	go func() {
		group.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("Close blocked after NOTE_TRIGGER failure")
	}
}

func TestFreeBSDProcessGroupOpenRegistrationErrorClosesKqueue(t *testing.T) {
	initialFDs := freeBSDOpenFDCount(t)
	group, err := (osProcessGroupFactory{}).Open(1 << 30)
	if err == nil || group != nil {
		t.Fatalf("Open invalid PID got (%T, %v), want registration error", group, err)
	}
	if finalFDs := freeBSDOpenFDCount(t); finalFDs != initialFDs {
		t.Fatalf("registration error changed fd count from %d to %d", initialFDs, finalFDs)
	}
}

func TestFreeBSDLeaderObservationPrioritizesExitOverCancellation(t *testing.T) {
	exit := unix.Kevent_t{Ident: 41, Filter: unix.EVFILT_PROC, Fflags: unix.NOTE_EXIT}
	cancel := unix.Kevent_t{Ident: freeBSDCancelEventID, Filter: unix.EVFILT_USER}
	for _, events := range [][]unix.Kevent_t{{cancel, exit}, {exit, cancel}} {
		exited, err := freeBSDLeaderObservation(events, 41)
		if err != nil || !exited {
			t.Fatalf("simultaneous events got exited=%v error=%v", exited, err)
		}
	}
}

func TestFreeBSDLeaderObservationTreatsCancellationAsNoExit(t *testing.T) {
	cancel := unix.Kevent_t{Ident: freeBSDCancelEventID, Filter: unix.EVFILT_USER}
	exited, err := freeBSDLeaderObservation([]unix.Kevent_t{cancel}, 41)
	if err != nil || exited {
		t.Fatalf("cancellation got exited=%v error=%v", exited, err)
	}
}

func TestFreeBSDLeaderObservationReportsKernelError(t *testing.T) {
	event := unix.Kevent_t{Flags: unix.EV_ERROR, Data: int64(unix.EBADF)}
	exited, err := freeBSDLeaderObservation([]unix.Kevent_t{event}, 41)
	if exited || !errors.Is(err, unix.EBADF) {
		t.Fatalf("kernel error got exited=%v error=%v", exited, err)
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
		{"stopped descendant", append(freeBSDProcessFixture(41, 41, freeBSDProcessZombie), freeBSDProcessFixture(42, 41, freeBSDProcessStopped)...), true, true, nil},
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

func openFreeBSDProcessGroup(t *testing.T, pid int) *freeBSDProcessGroup {
	t.Helper()
	opened, err := (osProcessGroupFactory{}).Open(pid)
	if err != nil {
		t.Fatal(err)
	}
	return opened.(*freeBSDProcessGroup)
}

func freeBSDOpenFDCount(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir("/dev/fd")
	if err != nil {
		t.Fatal(err)
	}
	return len(entries)
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
	return freeBSDPIDExecutable(data, pid)
}
