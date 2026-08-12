//go:build linux || freebsd

package main

import (
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestProcessFirstEventPrioritizesTimeoutAdmittedBeforeStdoutCut(t *testing.T) {
	for range 1000 {
		active, timeout, admitted, cutReady, releaseCut := synchronizedProcessEvents()
		result := make(chan processEvent, 1)
		go func() { result <- active.firstEvent(timeout) }()

		active.stdout.overflow <- processStdout
		requireAdmittedEvent(t, admitted, processStdoutEvent)
		<-cutReady
		timeout.fire()
		requireAdmittedEvent(t, admitted, processTimeoutEvent)
		close(releaseCut)

		var timeoutError *processTimeoutError
		if event := <-result; !errors.As(event.primary, &timeoutError) {
			t.Fatalf("got %T %v, want timeout", event.primary, event.primary)
		}
	}
}

func TestProcessFirstEventPrioritizesStdoutAdmittedBeforeStderrCut(t *testing.T) {
	for range 1000 {
		active, timeout, admitted, cutReady, releaseCut := synchronizedProcessEvents()
		result := make(chan processEvent, 1)
		go func() { result <- active.firstEvent(timeout) }()

		active.stderr.overflow <- processStderr
		requireAdmittedEvent(t, admitted, processStderrEvent)
		<-cutReady
		active.stdout.overflow <- processStdout
		requireAdmittedEvent(t, admitted, processStdoutEvent)
		close(releaseCut)

		var overflowError *processOverflowError
		event := <-result
		if !errors.As(event.primary, &overflowError) || overflowError.stream != processStdout {
			t.Fatalf("got %T %v, want stdout overflow", event.primary, event.primary)
		}
	}
}

func TestProcessFirstEventIgnoresOverflowAfterCompletionCut(t *testing.T) {
	active, timeout, admitted, cutReady, releaseCut := synchronizedProcessEvents()
	result := make(chan processEvent, 1)
	go func() { result <- active.firstEvent(timeout) }()

	active.group.(*fakeProcessGroup).exited <- nil
	requireAdmittedEvent(t, admitted, processLeaderEvent)
	<-cutReady
	close(releaseCut)
	event := <-result
	if event.primary != nil || event.observationError != nil {
		t.Fatalf("completion got primary=%v observation=%v", event.primary, event.observationError)
	}
	for range 100 {
		if _, err := active.stdout.Write(make([]byte, active.limits.stdoutLimit+1)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestProcessFirstEventJoinsDormantProducers(t *testing.T) {
	for range 100 {
		active, timeout, admitted, cutReady, releaseCut := synchronizedProcessEvents()
		result := make(chan processEvent, 1)
		go func() { result <- active.firstEvent(timeout) }()
		active.group.(*fakeProcessGroup).exited <- nil
		requireAdmittedEvent(t, admitted, processLeaderEvent)
		<-cutReady
		close(releaseCut)
		<-result
	}
	stack := make([]byte, 1<<20)
	stack = stack[:runtime.Stack(stack, true)]
	if strings.Contains(string(stack), "processEventCoordinator).watch") {
		t.Fatalf("event producer remained after coordinator return:\n%s", stack)
	}
}

func synchronizedProcessEvents() (*activeProcess, *fakeProcessTimer, <-chan processEventKind, <-chan struct{}, chan struct{}) {
	limits := builtInQueryLimits()
	admitted := make(chan processEventKind, 4)
	cutReady := make(chan struct{}, 1)
	releaseCut := make(chan struct{})
	active := &activeProcess{
		group: &fakeProcessGroup{exited: make(chan error, 1)}, path: "/bin/true", limits: limits,
		stdout:        newCappedProcessOutput(limits.stdoutLimit, processStdout),
		stderr:        newCappedProcessOutput(limits.stderrLimit, processStderr),
		eventAdmitted: admitted, cutReady: cutReady, releaseCut: releaseCut,
	}
	return active, &fakeProcessTimer{ch: make(chan time.Time, 1)}, admitted, cutReady, releaseCut
}

func requireAdmittedEvent(t *testing.T, admitted <-chan processEventKind, want processEventKind) {
	t.Helper()
	select {
	case got := <-admitted:
		if got != want {
			t.Fatalf("admitted event got %d, want %d", got, want)
		}
	case <-time.After(time.Second):
		t.Fatalf("event %d was not admitted", want)
	}
}
