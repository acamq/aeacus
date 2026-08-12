//go:build linux

package main

import (
	"runtime"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestLinuxProcessGroupCloseJoinsObserver(t *testing.T) {
	for range 100 {
		pidfd, err := unix.PidfdOpen(unix.Getpid(), 0)
		if err != nil {
			t.Fatal(err)
		}
		group := &linuxProcessGroup{
			pid: unix.Getpid(), pidfd: pidfd, exited: make(chan error, 1), done: make(chan struct{}),
		}
		go group.observeLeader()
		group.Close()
		select {
		case <-group.done:
		default:
			t.Fatal("Close returned before observer exited")
		}
	}
	stack := make([]byte, 1<<20)
	stack = stack[:runtime.Stack(stack, true)]
	if strings.Contains(string(stack), "linuxProcessGroup).observeLeader") {
		t.Fatalf("Linux process observer remained after Close:\n%s", stack)
	}
}
