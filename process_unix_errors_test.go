//go:build linux || freebsd

package main

import (
	"errors"
	"os"
	"testing"
	"time"
)

func TestProcessWaitErrorExposesPrimaryAndCleanupCauses(t *testing.T) {
	primary := &processTimeoutError{path: "/bin/true", timeout: 10 * time.Second}
	signalFailure := errors.New("group signal failed")
	directFailure := errors.New("direct kill failed")
	waitFailure := &processWaitError{
		path: "/bin/true", kind: waitFailed, err: errors.Join(signalFailure, directFailure), primary: primary,
	}
	if !errors.Is(waitFailure, primary) || !errors.Is(waitFailure, signalFailure) || !errors.Is(waitFailure, directFailure) {
		t.Fatalf("error chain hides a cause: %v", waitFailure)
	}
}

func TestExecRunnerExposesTERMGroupKILLAndDirectKillFailures(t *testing.T) {
	runner, clock, starter, group := newFakeExecRunner()
	termFailure := errors.New("TERM failed")
	killFailure := errors.New("KILL failed")
	directFailure := errors.New("direct kill failed")
	group.errors = []error{termFailure, killFailure}
	starter.process.directKillError = directFailure
	done := runFakeProcess(runner, "/bin/true", builtInQueryLimits())
	<-starter.started
	(<-clock.timers).fire()
	<-group.signals
	<-group.signals
	<-clock.timers
	starter.process.wait <- nil
	outcome := <-done
	if !errors.Is(outcome.err, termFailure) || !errors.Is(outcome.err, killFailure) || !errors.Is(outcome.err, directFailure) {
		t.Fatalf("error chain hides cleanup causes: %v", outcome.err)
	}
}

func TestExecRunnerClassifiesStartAndWaitFailures(t *testing.T) {
	starter := &fakeProcessStarter{startError: os.ErrPermission, started: make(chan processInvocation, 1)}
	runner := execRunner{clock: newFakeProcessClock(), starter: starter}
	_, err := runner.RunBuiltIn("/denied", nil)
	var startError *processStartError
	if !errors.As(err, &startError) {
		t.Fatalf("got %T, want start error", err)
	}

	runner2, clock, starter, _ := newFakeExecRunner()
	done := runFakeProcess(runner2, "/bin/true", builtInQueryLimits())
	<-starter.started
	<-clock.timers
	starter.process.wait <- errors.New("wait failed")
	outcome := <-done
	var waitError *processWaitError
	if !errors.As(outcome.err, &waitError) || waitError.kind != waitFailed {
		t.Fatalf("got %T %v, want wait error", outcome.err, outcome.err)
	}
}

func TestExecRunnerReapsChildWhenGroupIdentityOpenFails(t *testing.T) {
	runner, clock, starter, _ := newFakeExecRunner()
	openFailure := errors.New("identity open failed")
	runner.groupSignals = fakeGroupSignalFactory{err: openFailure}
	done := runFakeProcess(runner, "/bin/true", builtInQueryLimits())
	<-starter.started
	waitBound := <-clock.timers
	if waitBound.duration != 2*time.Second {
		t.Fatalf("wait bound got %v", waitBound.duration)
	}
	starter.process.wait <- nil
	outcome := <-done
	if !errors.Is(outcome.err, openFailure) {
		t.Fatalf("got %v, want identity-open cause", outcome.err)
	}
}

func TestExecRunnerUsesFirstCoordinatorEventWithoutResampling(t *testing.T) {
	runner, clock, starter, _ := newFakeExecRunner()
	done := runFakeProcess(runner, "/bin/true", builtInQueryLimits())
	invocation := <-starter.started
	<-clock.timers
	starter.process.wait <- nil
	outcome := <-done
	if _, err := invocation.stdout.Write(make([]byte, (1<<20)+1)); err != nil {
		t.Fatal(err)
	}
	if outcome.err != nil {
		t.Fatalf("completed process got %T %v, want success", outcome.err, outcome.err)
	}
}
