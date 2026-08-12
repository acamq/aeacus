//go:build linux || freebsd

package main

import (
	"errors"
	"syscall"
	"testing"
	"time"
)

func TestExecRunnerBoundsEveryDirectChildReapPath(t *testing.T) {
	tests := []struct {
		name  string
		start func(*testing.T, execRunner, *fakeProcessClock, *fakeProcessStarter, *fakeProcessGroup) <-chan asyncProcessResult
	}{
		{"normal completion", startNormalCompletionReap},
		{"TERM group disappearance", startTermDisappearanceReap},
		{"KILL group disappearance", startKillDisappearanceReap},
		{"TERM liveness failure", startTermLivenessFailureReap},
		{"KILL liveness failure", startKillLivenessFailureReap},
		{"leader observation failure", startObservationFailureReap},
		{"identity open abort", startIdentityOpenAbortReap},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner, clock, starter, group := newFakeExecRunner()
			done := tt.start(t, runner, clock, starter, group)
			bound := requireFakeTimerDuration(t, clock, 2*time.Second, "final direct-child wait")
			select {
			case outcome := <-done:
				t.Fatalf("runner returned before exact final bound: %v", outcome.err)
			default:
			}
			bound.fire()
			outcome := requireFakeOutcome(t, done)
			var waitError *processWaitError
			if !errors.As(outcome.err, &waitError) || waitError.kind != waitBoundExceeded || waitError.bound != 2*time.Second {
				t.Fatalf("got %T %v, want exact 2s wait-bound error", outcome.err, outcome.err)
			}
			if calls := starter.process.waitCalls.Load(); calls != 1 {
				t.Fatalf("Wait calls=%d, want exactly one", calls)
			}
			clock.assertNoActiveTimer(t, 2*time.Second)
		})
	}
}

func TestExecRunnerUsesOneFinalDeadlineForKillDisappearanceAndReap(t *testing.T) {
	runner, clock, starter, group := newFakeExecRunner()
	group.aliveAt = func(check int32) (bool, error) { return check < 4, nil }
	done := runFakeProcess(runner, "/bin/true", builtInQueryLimits())
	<-starter.started
	(<-clock.timers).fire()
	if signal := <-group.signals; signal != syscall.SIGTERM {
		t.Fatalf("first signal got %v", signal)
	}
	grace := requireFakeTimerDuration(t, clock, 2*time.Second, "TERM grace")
	grace.fire()
	if signal := <-group.signals; signal != syscall.SIGKILL {
		t.Fatalf("second signal got %v", signal)
	}
	finalBound := requireFakeTimerDuration(t, clock, 2*time.Second, "shared final deadline")
	poll := requireFakeTimerDuration(t, clock, unixGroupPollInterval, "group disappearance poll")
	poll.fire()
	select {
	case timer := <-clock.timers:
		if timer.duration == 2*time.Second {
			t.Fatal("started a new final deadline after group disappearance")
		}
	default:
	}
	select {
	case outcome := <-done:
		t.Fatalf("runner returned before shared final deadline: %v", outcome.err)
	default:
	}
	finalBound.fire()
	outcome := requireFakeOutcome(t, done)
	var waitError *processWaitError
	if !errors.As(outcome.err, &waitError) || waitError.kind != waitBoundExceeded {
		t.Fatalf("got %T %v, want wait-bound error", outcome.err, outcome.err)
	}
}

func startNormalCompletionReap(
	t *testing.T,
	runner execRunner,
	clock *fakeProcessClock,
	starter *fakeProcessStarter,
	group *fakeProcessGroup,
) <-chan asyncProcessResult {
	t.Helper()
	done := runFakeProcess(runner, "/bin/true", builtInQueryLimits())
	<-starter.started
	<-clock.timers
	group.alive.Store(false)
	group.exited <- nil
	return done
}

func startTermDisappearanceReap(
	t *testing.T,
	runner execRunner,
	clock *fakeProcessClock,
	starter *fakeProcessStarter,
	group *fakeProcessGroup,
) <-chan asyncProcessResult {
	t.Helper()
	done := startTimeoutCleanup(t, runner, clock, starter, group)
	group.alive.Store(false)
	(<-clock.timers).fire()
	return done
}

func startKillDisappearanceReap(
	t *testing.T,
	runner execRunner,
	clock *fakeProcessClock,
	starter *fakeProcessStarter,
	group *fakeProcessGroup,
) <-chan asyncProcessResult {
	t.Helper()
	done := startTimeoutCleanup(t, runner, clock, starter, group)
	(<-clock.timers).fire()
	<-group.signals
	return done
}

func startTermLivenessFailureReap(
	t *testing.T,
	runner execRunner,
	clock *fakeProcessClock,
	starter *fakeProcessStarter,
	group *fakeProcessGroup,
) <-chan asyncProcessResult {
	t.Helper()
	group.aliveAt = func(check int32) (bool, error) {
		if check == 1 {
			return false, errors.New("TERM liveness failed")
		}
		return false, nil
	}
	done := startTimeoutCleanup(t, runner, clock, starter, group)
	(<-clock.timers).fire()
	<-group.signals
	return done
}

func startKillLivenessFailureReap(
	t *testing.T,
	runner execRunner,
	clock *fakeProcessClock,
	starter *fakeProcessStarter,
	group *fakeProcessGroup,
) <-chan asyncProcessResult {
	t.Helper()
	group.aliveAt = func(check int32) (bool, error) {
		if check < 2 {
			return true, nil
		}
		return false, errors.New("KILL liveness failed")
	}
	done := startTimeoutCleanup(t, runner, clock, starter, group)
	(<-clock.timers).fire()
	<-group.signals
	return done
}

func startObservationFailureReap(
	t *testing.T,
	runner execRunner,
	clock *fakeProcessClock,
	starter *fakeProcessStarter,
	group *fakeProcessGroup,
) <-chan asyncProcessResult {
	t.Helper()
	done := runFakeProcess(runner, "/bin/true", builtInQueryLimits())
	<-starter.started
	<-clock.timers
	group.alive.Store(false)
	group.exited <- errors.New("leader observation failed")
	return done
}

func startIdentityOpenAbortReap(
	t *testing.T,
	runner execRunner,
	clock *fakeProcessClock,
	starter *fakeProcessStarter,
	group *fakeProcessGroup,
) <-chan asyncProcessResult {
	t.Helper()
	runner.groups = fakeProcessGroupFactory{err: errors.New("identity open failed")}
	done := runFakeProcess(runner, "/bin/true", builtInQueryLimits())
	<-starter.started
	return done
}

func startTimeoutCleanup(
	t *testing.T,
	runner execRunner,
	clock *fakeProcessClock,
	starter *fakeProcessStarter,
	group *fakeProcessGroup,
) <-chan asyncProcessResult {
	t.Helper()
	done := runFakeProcess(runner, "/bin/true", builtInQueryLimits())
	<-starter.started
	(<-clock.timers).fire()
	if signal := <-group.signals; signal != syscall.SIGTERM {
		t.Fatalf("first signal got %v", signal)
	}
	return done
}
