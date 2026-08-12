//go:build linux || freebsd

package main

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestProcessEventLatchPrioritizesTimeoutOverStdout(t *testing.T) {
	for range 1000 {
		latch := newProcessEventLatch("/bin/true", builtInQueryLimits())
		latch.hold()
		latch.timeout()
		latch.overflow(processStdout)
		latch.release()
		var timeoutError *processTimeoutError
		if event := latch.wait(); !errors.As(event.primary, &timeoutError) {
			t.Fatalf("got %T, want timeout", event.primary)
		}
	}
}

func TestProcessEventLatchPrioritizesStdoutOverStderr(t *testing.T) {
	for range 1000 {
		latch := newProcessEventLatch("/bin/true", builtInQueryLimits())
		latch.hold()
		latch.overflow(processStderr)
		latch.overflow(processStdout)
		latch.release()
		var overflowError *processOverflowError
		event := latch.wait()
		if !errors.As(event.primary, &overflowError) || overflowError.stream != processStdout {
			t.Fatalf("got %T %v, want stdout overflow", event.primary, event.primary)
		}
	}
}

func TestProcessEventsPrioritizeSimultaneousTimeoutOverStdout(t *testing.T) {
	for range 1000 {
		active, timeout := synchronizedProcessEvents()
		var ready sync.WaitGroup
		var fired sync.WaitGroup
		ready.Add(2)
		fired.Add(2)
		release := make(chan struct{})
		go func() { defer fired.Done(); ready.Done(); <-release; timeout.fire() }()
		go func() { defer fired.Done(); ready.Done(); <-release; active.stdout.overflow <- processStdout }()
		ready.Wait()
		close(release)
		fired.Wait()
		var timeoutError *processTimeoutError
		if event := active.firstEvent(timeout); !errors.As(event.primary, &timeoutError) {
			t.Fatalf("got %T %v, want timeout", event.primary, event.primary)
		}
	}
}

func TestProcessEventsPrioritizeSimultaneousStdoutOverStderr(t *testing.T) {
	for range 1000 {
		active, timeout := synchronizedProcessEvents()
		var ready sync.WaitGroup
		var fired sync.WaitGroup
		ready.Add(2)
		fired.Add(2)
		release := make(chan struct{})
		go func() { defer fired.Done(); ready.Done(); <-release; active.stderr.overflow <- processStderr }()
		go func() { defer fired.Done(); ready.Done(); <-release; active.stdout.overflow <- processStdout }()
		ready.Wait()
		close(release)
		fired.Wait()
		var overflowError *processOverflowError
		event := active.firstEvent(timeout)
		if !errors.As(event.primary, &overflowError) || overflowError.stream != processStdout {
			t.Fatalf("got %T %v, want stdout overflow", event.primary, event.primary)
		}
	}
}

func synchronizedProcessEvents() (*activeProcess, *fakeProcessTimer) {
	limits := builtInQueryLimits()
	group := &fakeProcessGroup{exited: make(chan error), changed: make(chan struct{})}
	active := &activeProcess{
		group: group, path: "/bin/true", limits: limits,
		stdout: newCappedProcessOutput(limits.stdoutLimit, processStdout),
		stderr: newCappedProcessOutput(limits.stderrLimit, processStderr),
	}
	return active, &fakeProcessTimer{ch: make(chan time.Time, 1)}
}
