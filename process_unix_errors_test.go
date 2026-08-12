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
	grace := <-clock.timers
	grace.fire()
	<-group.signals
	(<-clock.timers).fire()
	starter.process.wait <- nil
	outcome := <-done
	if !errors.Is(outcome.err, termFailure) || !errors.Is(outcome.err, killFailure) || !errors.Is(outcome.err, directFailure) {
		t.Fatalf("error chain hides cleanup causes: %v", outcome.err)
	}
}

func TestExecRunnerDoesNotAttemptDirectKillWhenGroupKillSucceeds(t *testing.T) {
	runner, clock, starter, group := newFakeExecRunner()
	directFailure := errors.New("unattempted direct kill failure")
	starter.process.directKillError = directFailure
	done := runFakeProcess(runner, "/bin/true", builtInQueryLimits())
	<-starter.started
	(<-clock.timers).fire()
	<-group.signals
	(<-clock.timers).fire()
	<-group.signals
	<-clock.timers
	starter.process.wait <- nil
	outcome := requireFakeOutcome(t, done)
	if calls := starter.process.directKillCalls.Load(); calls != 0 {
		t.Fatalf("direct kill calls=%d, want zero after successful group KILL", calls)
	}
	if errors.Is(outcome.err, directFailure) {
		t.Fatalf("error exposes unattempted direct-kill failure: %v", outcome.err)
	}
}

func TestExecRunnerExposesGroupAndFallbackDirectKillFailures(t *testing.T) {
	runner, clock, starter, group := newFakeExecRunner()
	killFailure := errors.New("group KILL failed")
	directFailure := errors.New("fallback direct kill failed")
	group.errors = []error{nil, killFailure}
	starter.process.directKillError = directFailure
	done := runFakeProcess(runner, "/bin/true", builtInQueryLimits())
	<-starter.started
	(<-clock.timers).fire()
	<-group.signals
	(<-clock.timers).fire()
	<-group.signals
	<-clock.timers
	starter.process.wait <- nil
	outcome := requireFakeOutcome(t, done)
	if calls := starter.process.directKillCalls.Load(); calls != 1 {
		t.Fatalf("direct kill calls=%d, want one fallback attempt", calls)
	}
	if !errors.Is(outcome.err, killFailure) || !errors.Is(outcome.err, directFailure) {
		t.Fatalf("error chain hides attempted KILL cause: %v", outcome.err)
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

	runner2, clock, starter, group := newFakeExecRunner()
	done := runFakeProcess(runner2, "/bin/true", builtInQueryLimits())
	<-starter.started
	<-clock.timers
	group.alive.Store(false)
	group.exited <- nil
	starter.process.wait <- errors.New("wait failed")
	outcome := <-done
	var waitError *processWaitError
	if !errors.As(outcome.err, &waitError) || waitError.kind != waitFailed {
		t.Fatalf("got %T %v, want wait error", outcome.err, outcome.err)
	}
}

func TestExecRunnerReapsChildWhenGroupIdentityOpenFails(t *testing.T) {
	runner, clock, starter, group := newFakeExecRunner()
	openFailure := errors.New("identity open failed")
	runner.groups = fakeProcessGroupFactory{err: openFailure}
	done := runFakeProcess(runner, "/bin/true", builtInQueryLimits())
	<-starter.started
	waitBound := <-clock.timers
	if waitBound.duration != 2*time.Second {
		t.Fatalf("wait bound got %v", waitBound.duration)
	}
	group.exited <- nil
	starter.process.wait <- nil
	outcome := <-done
	if !errors.Is(outcome.err, openFailure) {
		t.Fatalf("got %v, want identity-open cause", outcome.err)
	}
}

func TestExecRunnerBoundsAbortStartAndExposesAttemptedErrors(t *testing.T) {
	clock := newFakeProcessClock()
	process := newFakeRunningProcess()
	openFailure := errors.New("identity open failed")
	directFailure := errors.New("direct kill failed")
	process.directKillError = directFailure
	done := make(chan error, 1)
	go func() {
		done <- (execRunner{clock: clock}).abortStart(process, "/bin/true", builtInQueryLimits(), openFailure)
	}()
	bound := requireFakeTimer(t, clock, "abortStart final bound")
	if bound.duration != 2*time.Second {
		t.Fatalf("abortStart bound got %v", bound.duration)
	}
	bound.fire()
	err := <-done
	var waitError *processWaitError
	if !errors.As(err, &waitError) || waitError.kind != waitBoundExceeded || waitError.bound != 2*time.Second {
		t.Fatalf("got %T %v, want exact abortStart bound", err, err)
	}
	if !errors.Is(err, openFailure) || !errors.Is(err, directFailure) {
		t.Fatalf("abortStart hides attempted error: %v", err)
	}
	if process.waitCalls.Load() != 1 || process.directKillCalls.Load() != 1 {
		t.Fatalf("abortStart calls Wait=%d KillDirect=%d, want one each", process.waitCalls.Load(), process.directKillCalls.Load())
	}
}

func TestExecRunnerExposesObservationAndWaitFailures(t *testing.T) {
	runner, clock, starter, group := newFakeExecRunner()
	observationFailure := errors.New("leader observation failed")
	waitFailure := errors.New("wait failed")
	done := runFakeProcess(runner, "/bin/true", builtInQueryLimits())
	<-starter.started
	<-clock.timers
	group.alive.Store(false)
	group.exited <- observationFailure
	<-clock.timers
	starter.process.wait <- waitFailure
	outcome := requireFakeOutcome(t, done)
	if !errors.Is(outcome.err, observationFailure) || !errors.Is(outcome.err, waitFailure) {
		t.Fatalf("error chain hides observation or wait cause: %v", outcome.err)
	}
}

func TestExecRunnerExposesLivenessAndWaitFailures(t *testing.T) {
	runner, clock, starter, group := newFakeExecRunner()
	livenessFailure := errors.New("group liveness failed")
	waitFailure := errors.New("wait failed")
	group.aliveAt = func(check int32) (bool, error) {
		if check == 1 {
			return true, nil
		}
		return false, livenessFailure
	}
	done := runFakeProcess(runner, "/bin/true", builtInQueryLimits())
	<-starter.started
	(<-clock.timers).fire()
	<-group.signals
	(<-clock.timers).fire()
	<-group.signals
	bound := requireFakeTimerDuration(t, clock, 2*time.Second, "liveness failure final wait")
	starter.process.wait <- waitFailure
	outcome := requireFakeOutcome(t, done)
	if !bound.stopped.Load() {
		t.Fatal("final wait bound was not stopped")
	}
	if !errors.Is(outcome.err, livenessFailure) || !errors.Is(outcome.err, waitFailure) {
		t.Fatalf("error chain hides liveness or wait cause: %v", outcome.err)
	}
}

func TestExecRunnerUsesFirstCoordinatorEventWithoutResampling(t *testing.T) {
	runner, clock, starter, group := newFakeExecRunner()
	done := runFakeProcess(runner, "/bin/true", builtInQueryLimits())
	invocation := <-starter.started
	<-clock.timers
	group.alive.Store(false)
	group.exited <- nil
	starter.process.wait <- nil
	outcome := <-done
	if _, err := invocation.stdout.Write(make([]byte, (1<<20)+1)); err != nil {
		t.Fatal(err)
	}
	if outcome.err != nil {
		t.Fatalf("completed process got %T %v, want success", outcome.err, outcome.err)
	}
}
