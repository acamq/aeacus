//go:build linux || freebsd

package main

import (
	"errors"
	"syscall"
	"testing"
)

func TestProcessLeaderClassificationSamplesOutputAfterJoinedWait(t *testing.T) {
	tests := []struct {
		name             string
		writeStdout      int
		writeStderr      int
		observationError error
		wantStream       processOutputStream
		wantOverflow     bool
	}{
		{name: "exact limit", writeStdout: 1 << 20},
		{name: "stdout limit plus one", writeStdout: (1 << 20) + 1, wantStream: processStdout, wantOverflow: true},
		{name: "stderr limit plus one", writeStderr: (1 << 20) + 1, wantStream: processStderr, wantOverflow: true},
		{name: "both overflow", writeStdout: (1 << 20) + 1, writeStderr: (1 << 20) + 1, wantStream: processStdout, wantOverflow: true},
		{name: "leader error plus late overflow", writeStdout: (1 << 20) + 1, observationError: syscall.EBADF, wantStream: processStdout, wantOverflow: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			active, timeout, admitted, cutReady, releaseCut := synchronizedProcessEvents()
			process := newFakeRunningProcess()
			active.process = process
			first := make(chan processEvent, 1)
			go func() { first <- active.firstEvent(timeout) }()
			active.group.(*fakeProcessGroup).exited <- test.observationError
			requireAdmittedEvent(t, admitted, processLeaderEvent)
			<-cutReady
			close(releaseCut)
			event := <-first
			if active.stdout.Exceeded() || active.stderr.Exceeded() {
				t.Fatal("output exceeded before direct-child Wait")
			}
			if test.writeStdout > 0 {
				_, _ = active.stdout.Write(make([]byte, test.writeStdout))
			}
			if test.writeStderr > 0 {
				_, _ = active.stderr.Write(make([]byte, test.writeStderr))
			}
			process.wait <- nil
			err := (execRunner{clock: newFakeProcessClock()}).reapAndClassify(active, event.observationError)
			var overflow *processOverflowError
			if errors.As(err, &overflow) != test.wantOverflow {
				t.Fatalf("overflow got %T %v, want %v", err, err, test.wantOverflow)
			}
			if test.wantOverflow && overflow.stream != test.wantStream {
				t.Fatalf("stream got %v, want %v", overflow.stream, test.wantStream)
			}
			if test.observationError != nil && !errors.Is(err, test.observationError) {
				t.Fatalf("leader observation error missing from %v", err)
			}
		})
	}
}

func TestProcessJoinedOutputPreservesTimeoutAndCleanupErrors(t *testing.T) {
	active, _, _, _, _ := synchronizedProcessEvents()
	_, _ = active.stdout.Write(make([]byte, active.limits.stdoutLimit+1))
	timeout := &processTimeoutError{path: active.path, timeout: active.limits.timeout}
	cleanupError := syscall.EPERM
	err := active.finishReap(nil, processReapRequest{primary: timeout, cleanup: cleanupKilled, cleanupError: cleanupError})
	var gotTimeout *processTimeoutError
	if !errors.As(err, &gotTimeout) || gotTimeout != timeout {
		t.Fatalf("got %T %v, want original timeout", err, err)
	}
	if !errors.Is(err, cleanupError) {
		t.Fatalf("cleanup error missing from %v", err)
	}
}
