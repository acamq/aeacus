//go:build linux || freebsd

package main

import (
	"errors"
	"io"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

type fakeProcessTimer struct {
	duration time.Duration
	ch       chan time.Time
	stopped  atomic.Bool
}

func (t *fakeProcessTimer) Chan() <-chan time.Time { return t.ch }
func (t *fakeProcessTimer) Stop() bool             { return !t.stopped.Swap(true) }
func (t *fakeProcessTimer) fire() {
	if !t.stopped.Load() {
		t.ch <- time.Unix(0, 0)
	}
}

type fakeProcessClock struct {
	timers chan *fakeProcessTimer
}

func newFakeProcessClock() *fakeProcessClock {
	return &fakeProcessClock{timers: make(chan *fakeProcessTimer, 16)}
}

func (c *fakeProcessClock) NewTimer(duration time.Duration) processTimer {
	timer := &fakeProcessTimer{duration: duration, ch: make(chan time.Time, 1)}
	select {
	case c.timers <- timer:
	default:
	}
	return timer
}

func (c *fakeProcessClock) assertNoActiveTimer(t *testing.T, duration time.Duration) {
	t.Helper()
	for {
		select {
		case timer := <-c.timers:
			if timer.duration == duration && !timer.stopped.Load() {
				t.Fatalf("active timer remains at %v", duration)
			}
		default:
			return
		}
	}
}

type fakeRunningProcess struct {
	wait            chan error
	directKillError error
	directKillCalls atomic.Int32
	waitCalls       atomic.Int32
	waitCalled      chan struct{}
}

func newFakeRunningProcess() *fakeRunningProcess {
	return &fakeRunningProcess{
		wait: make(chan error, 1), waitCalled: make(chan struct{}, 1),
	}
}

func (p *fakeRunningProcess) PID() int { return 42 }
func (p *fakeRunningProcess) Wait() <-chan error {
	p.waitCalls.Add(1)
	select {
	case p.waitCalled <- struct{}{}:
	default:
	}
	return p.wait
}
func (p *fakeRunningProcess) KillDirect() error {
	p.directKillCalls.Add(1)
	return p.directKillError
}

type fakeProcessGroup struct {
	signals chan syscall.Signal
	errors  []error
	exited  chan error
	alive   atomic.Bool
	aliveAt func(int32) (bool, error)
	checks  atomic.Int32
}

func (g *fakeProcessGroup) Signal(signal syscall.Signal) error {
	g.signals <- signal
	if signal == syscall.SIGKILL {
		g.alive.Store(false)
	}
	if len(g.errors) == 0 {
		return nil
	}
	err := g.errors[0]
	g.errors = g.errors[1:]
	return err
}

func (g *fakeProcessGroup) LeaderExited() <-chan error { return g.exited }
func (g *fakeProcessGroup) Alive() (bool, error) {
	check := g.checks.Add(1)
	if g.aliveAt != nil {
		return g.aliveAt(check)
	}
	return g.alive.Load(), nil
}
func (g *fakeProcessGroup) Close() {}

type fakeProcessGroupFactory struct {
	target *fakeProcessGroup
	err    error
}

func (f fakeProcessGroupFactory) Open(int) (processGroup, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.target, nil
}

type fakeProcessStarter struct {
	process    *fakeRunningProcess
	started    chan processInvocation
	startError error
}

func (s *fakeProcessStarter) Start(invocation processInvocation) (runningProcess, error) {
	if s.startError != nil {
		return nil, s.startError
	}
	s.started <- invocation
	return s.process, nil
}

type asyncProcessResult struct {
	result processResult
	err    error
}

func runFakeProcess(runner execRunner, path string, limits processLimits) <-chan asyncProcessResult {
	done := make(chan asyncProcessResult, 1)
	go func() {
		result, err := runner.run(path, []string{"literal argument"}, limits)
		done <- asyncProcessResult{result: result, err: err}
	}()
	return done
}

func newFakeExecRunner() (execRunner, *fakeProcessClock, *fakeProcessStarter, *fakeProcessGroup) {
	clock := newFakeProcessClock()
	starter := &fakeProcessStarter{
		process: newFakeRunningProcess(),
		started: make(chan processInvocation, 1),
	}
	target := &fakeProcessGroup{
		signals: make(chan syscall.Signal, 2), exited: make(chan error, 1),
	}
	target.alive.Store(true)
	return execRunner{clock: clock, starter: starter, groups: fakeProcessGroupFactory{target: target}}, clock, starter, target
}

func TestProcessProfilesAreCompileTimeBounded(t *testing.T) {
	tests := []struct {
		got  processLimits
		want processLimits
	}{
		{builtInQueryLimits(), processLimits{10 * time.Second, 1 << 20, 1 << 20, 2 * time.Second, 2 * time.Second}},
		{pkgInventoryLimits(), processLimits{30 * time.Second, 16 << 20, 1 << 20, 2 * time.Second, 2 * time.Second}},
		{trustedCommandLimits(), processLimits{30 * time.Second, 1 << 20, 1 << 20, 2 * time.Second, 2 * time.Second}},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Fatalf("profile got %+v, want %+v", tt.got, tt.want)
		}
	}
}

func TestExecRunnerTimeoutTerminatesWithinExactFakeClockBounds(t *testing.T) {
	runner, clock, starter, group := newFakeExecRunner()
	done := runFakeProcess(runner, "/bin/true", builtInQueryLimits())
	invocation := <-starter.started
	if invocation.path != "/bin/true" || len(invocation.args) != 1 || invocation.args[0] != "literal argument" {
		t.Fatalf("unexpected invocation: %#v", invocation)
	}
	timeout := <-clock.timers
	if timeout.duration != 10*time.Second {
		t.Fatalf("timeout got %v", timeout.duration)
	}
	timeout.fire()
	if signal := <-group.signals; signal != syscall.SIGTERM {
		t.Fatalf("first signal got %v", signal)
	}
	grace := <-clock.timers
	if grace.duration != 2*time.Second {
		t.Fatalf("TERM grace got %v", grace.duration)
	}
	group.exited <- nil
	group.alive.Store(false)
	grace.fire()
	starter.process.wait <- nil
	outcome := <-done
	var timeoutError *processTimeoutError
	if !errors.As(outcome.err, &timeoutError) || timeoutError.cleanup == cleanupNone {
		t.Fatalf("got %T %v, want completed cleanup", outcome.err, outcome.err)
	}
}

func TestExecRunnerEscalatesAndBoundsWaitAtExactFakeClockBoundaries(t *testing.T) {
	runner, clock, starter, group := newFakeExecRunner()
	done := runFakeProcess(runner, "/bin/true", trustedCommandLimits())
	<-starter.started
	timeout := <-clock.timers
	if timeout.duration != 30*time.Second {
		t.Fatalf("timeout got %v", timeout.duration)
	}
	timeout.fire()
	<-group.signals
	grace := <-clock.timers
	group.alive.Store(true)
	grace.fire()
	if signal := <-group.signals; signal != syscall.SIGKILL {
		t.Fatalf("second signal got %v", signal)
	}
	waitBound := requireFakeTimerDuration(t, clock, 2*time.Second, "final wait bound")
	if waitBound.duration != 2*time.Second {
		t.Fatalf("wait bound got %v", waitBound.duration)
	}
	group.exited <- nil
	group.alive.Store(false)
	starter.process.wait <- nil
	outcome := <-done
	if !waitBound.stopped.Load() {
		t.Fatal("final wait bound was not stopped")
	}
	var timeoutError *processTimeoutError
	if !errors.As(outcome.err, &timeoutError) || timeoutError.cleanup != cleanupKilled {
		t.Fatalf("got %T %v, want killed timeout", outcome.err, outcome.err)
	}
}

func TestExecRunnerAllowsExactLimitAndRejectsNextByte(t *testing.T) {
	tests := []struct {
		name   string
		stream processOutputStream
		writer func(processInvocation) io.Writer
	}{
		{"stdout", processStdout, func(i processInvocation) io.Writer { return i.stdout }},
		{"stderr", processStderr, func(i processInvocation) io.Writer { return i.stderr }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner, clock, starter, group := newFakeExecRunner()
			done := runFakeProcess(runner, "/bin/true", builtInQueryLimits())
			invocation := <-starter.started
			<-clock.timers
			if _, err := tt.writer(invocation).Write(make([]byte, 1<<20)); err != nil {
				t.Fatalf("write exact limit: %v", err)
			}
			if _, err := tt.writer(invocation).Write([]byte{'x'}); err != nil {
				t.Fatalf("write overflow marker: %v", err)
			}
			if signal := <-group.signals; signal != syscall.SIGTERM {
				t.Fatalf("overflow signal got %v", signal)
			}
			(<-clock.timers).fire()
			group.exited <- nil
			group.alive.Store(false)
			starter.process.wait <- nil
			outcome := <-done
			var overflow *processOverflowError
			if !errors.As(outcome.err, &overflow) || overflow.stream != tt.stream || overflow.limit != 1<<20 {
				t.Fatalf("got %T %v, want %v overflow", outcome.err, outcome.err, tt.stream)
			}
			if tt.stream == processStdout && len(outcome.result.stdout) != 1<<20 {
				t.Fatalf("retained stdout got %d", len(outcome.result.stdout))
			}
			if tt.stream == processStderr && len(outcome.result.stderr) != 1<<20 {
				t.Fatalf("retained stderr got %d", len(outcome.result.stderr))
			}
		})
	}
}

func TestExecRunnerWaitsForLeaderReapAfterGroupCleanup(t *testing.T) {
	runner, clock, starter, group := newFakeExecRunner()
	done := runFakeProcess(runner, "/bin/true", builtInQueryLimits())
	<-starter.started
	(<-clock.timers).fire()
	if signal := <-group.signals; signal != syscall.SIGTERM {
		t.Fatalf("first signal got %v", signal)
	}
	group.exited <- nil
	poll := requirePollAndGraceTimers(t, clock)
	group.alive.Store(false)
	poll.fire()
	reapBound := requireFakeTimerDuration(t, clock, 2*time.Second, "TERM reap bound")
	starter.process.wait <- nil
	outcome := <-done
	if !reapBound.stopped.Load() {
		t.Fatal("reap bound was not stopped")
	}
	var timeoutError *processTimeoutError
	if !errors.As(outcome.err, &timeoutError) || timeoutError.cleanup != cleanupTerminated {
		t.Fatalf("got %T %v, want terminated timeout", outcome.err, outcome.err)
	}
}

func TestExecRunnerUsesPreopenedGroupTargetAfterLeaderWait(t *testing.T) {
	runner, clock, starter, group := newFakeExecRunner()
	done := runFakeProcess(runner, "/bin/true", builtInQueryLimits())
	<-starter.started
	(<-clock.timers).fire()
	<-group.signals
	group.exited <- nil
	poll := requirePollAndGraceTimers(t, clock)
	group.alive.Store(false)
	poll.fire()
	reapBound := requireFakeTimerDuration(t, clock, 2*time.Second, "TERM reap bound")
	starter.process.wait <- nil
	<-done
	if !reapBound.stopped.Load() {
		t.Fatal("reap bound was not stopped")
	}
	select {
	case signal := <-group.signals:
		t.Fatalf("unexpected extra group signal %v", signal)
	default:
	}
}

func TestExecRunnerRechecksKilledGroupWithoutChangeNotification(t *testing.T) {
	runner, clock, starter, group := newFakeExecRunner()
	group.aliveAt = func(check int32) (bool, error) { return check < 3, nil }
	done := runFakeProcess(runner, "/bin/true", builtInQueryLimits())
	<-starter.started
	(<-clock.timers).fire()
	<-group.signals
	(<-clock.timers).fire()
	<-group.signals
	finalBound := requireFakeTimer(t, clock, "final wait bound")
	if finalBound.duration != 2*time.Second {
		t.Fatalf("final bound got %v", finalBound.duration)
	}
	poll := requireFakeTimer(t, clock, "group recheck")
	if poll.duration >= finalBound.duration {
		t.Fatalf("group recheck got %v, want shorter than final bound", poll.duration)
	}
	poll.fire()
	<-starter.process.waitCalled
	select {
	case timer := <-clock.timers:
		t.Fatalf("group disappearance reset final bound with %v timer", timer.duration)
	default:
	}
	starter.process.wait <- nil
	outcome := requireFakeOutcome(t, done)
	if !finalBound.stopped.Load() {
		t.Fatal("shared final bound was not stopped after reap")
	}
	var timeoutError *processTimeoutError
	if !errors.As(outcome.err, &timeoutError) || timeoutError.cleanup != cleanupKilled {
		t.Fatalf("got %T %v, want killed timeout", outcome.err, outcome.err)
	}
}

func TestExecRunnerBoundsReapWhenTerminatedGroupIsAbsent(t *testing.T) {
	runner, clock, starter, group := newFakeExecRunner()
	done := runFakeProcess(runner, "/bin/true", builtInQueryLimits())
	<-starter.started
	(<-clock.timers).fire()
	<-group.signals
	grace := <-clock.timers
	group.alive.Store(false)
	grace.fire()
	finalBound := requireFakeTimerDuration(t, clock, 2*time.Second, "bounded reap")
	if finalBound.duration != 2*time.Second {
		t.Fatalf("reap bound got %v", finalBound.duration)
	}
	finalBound.fire()
	outcome := requireFakeOutcome(t, done)
	var waitError *processWaitError
	if !errors.As(outcome.err, &waitError) || waitError.kind != waitBoundExceeded || waitError.bound != 2*time.Second {
		t.Fatalf("got %T %v, want exact bounded-reap error", outcome.err, outcome.err)
	}
}

func TestExecRunnerBoundsReapAfterSuccessfulLeaderCompletion(t *testing.T) {
	runner, clock, starter, group := newFakeExecRunner()
	done := runFakeProcess(runner, "/bin/true", builtInQueryLimits())
	<-starter.started
	<-clock.timers
	group.alive.Store(false)
	group.exited <- nil
	finalBound := requireFakeTimer(t, clock, "successful completion reap")
	if finalBound.duration != 2*time.Second {
		t.Fatalf("reap bound got %v", finalBound.duration)
	}
	finalBound.fire()
	outcome := requireFakeOutcome(t, done)
	var waitError *processWaitError
	if !errors.As(outcome.err, &waitError) || waitError.kind != waitBoundExceeded {
		t.Fatalf("got %T %v, want bounded-reap error", outcome.err, outcome.err)
	}
}

func requireFakeTimer(t *testing.T, clock *fakeProcessClock, operation string) *fakeProcessTimer {
	t.Helper()
	for {
		select {
		case timer := <-clock.timers:
			if !timer.stopped.Load() {
				return timer
			}
		case <-time.After(time.Second):
			t.Fatalf("runner did not create timer for %s", operation)
			return nil
		}
	}
}

func requireFakeTimerDuration(
	t *testing.T,
	clock *fakeProcessClock,
	duration time.Duration,
	operation string,
) *fakeProcessTimer {
	t.Helper()
	for {
		timer := requireFakeTimer(t, clock, operation)
		if timer.duration == duration {
			return timer
		}
		timer.fire()
	}
}

func requirePollAndGraceTimers(t *testing.T, clock *fakeProcessClock) *fakeProcessTimer {
	t.Helper()
	first := requireFakeTimer(t, clock, "TERM grace or group recheck")
	second := requireFakeTimer(t, clock, "TERM grace or group recheck")
	for _, timer := range []*fakeProcessTimer{first, second} {
		if timer.duration == unixGroupPollInterval {
			return timer
		}
	}
	t.Fatalf("timers got %v and %v, want TERM grace and group recheck", first.duration, second.duration)
	return nil
}

func requireFakeOutcome(t *testing.T, done <-chan asyncProcessResult) asyncProcessResult {
	t.Helper()
	select {
	case outcome := <-done:
		return outcome
	case <-time.After(time.Second):
		t.Fatal("runner did not return")
		return asyncProcessResult{}
	}
}
