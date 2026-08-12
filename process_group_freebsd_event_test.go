//go:build freebsd

package main

import (
	"errors"
	"testing"

	"golang.org/x/sys/unix"
)

func TestFreeBSDLeaderObservationAdjudicatesEventBatch(t *testing.T) {
	cleanExit := unix.Kevent_t{Ident: 41, Filter: unix.EVFILT_PROC, Fflags: unix.NOTE_EXIT}
	erroredExit := unix.Kevent_t{
		Ident: 41, Filter: unix.EVFILT_PROC, Flags: unix.EV_ERROR,
		Fflags: unix.NOTE_EXIT, Data: int64(unix.EBADF),
	}
	kernelError := unix.Kevent_t{Flags: unix.EV_ERROR, Data: int64(unix.EBADF)}
	cancel := unix.Kevent_t{Ident: freeBSDCancelEventID, Filter: unix.EVFILT_USER}
	tests := []struct {
		name       string
		events     []unix.Kevent_t
		wantExited bool
		wantError  error
	}{
		{name: "errored exit", events: []unix.Kevent_t{erroredExit}, wantError: unix.EBADF},
		{name: "errored then clean exit", events: []unix.Kevent_t{erroredExit, cleanExit}, wantExited: true},
		{name: "clean then errored exit", events: []unix.Kevent_t{cleanExit, erroredExit}, wantExited: true},
		{name: "kernel error without exit", events: []unix.Kevent_t{kernelError}, wantError: unix.EBADF},
		{name: "clean exit", events: []unix.Kevent_t{cleanExit}, wantExited: true},
		{name: "cancellation", events: []unix.Kevent_t{cancel}},
		{name: "cancellation and errored exit", events: []unix.Kevent_t{cancel, erroredExit}, wantError: unix.EBADF},
		{name: "cancellation and clean exit", events: []unix.Kevent_t{cancel, cleanExit}, wantExited: true},
		{name: "wrong pid", events: []unix.Kevent_t{{Ident: 42, Filter: unix.EVFILT_PROC, Fflags: unix.NOTE_EXIT}}},
		{name: "unrelated filter", events: []unix.Kevent_t{{Ident: 41, Filter: unix.EVFILT_READ, Fflags: unix.NOTE_EXIT}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			exited, err := freeBSDLeaderObservation(test.events, 41)
			if exited != test.wantExited || !errors.Is(err, test.wantError) {
				t.Fatalf("got exited=%v error=%v, want exited=%v error=%v", exited, err, test.wantExited, test.wantError)
			}
		})
	}
}
