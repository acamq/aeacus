//go:build linux || freebsd

package main

import (
	"errors"
	"io"
	"syscall"
	"testing"
	"time"
)

type fakeProcessTimer struct {
	duration time.Duration
	ch       chan time.Time
}

func (t *fakeProcessTimer) Chan() <-chan time.Time { return t.ch }
func (t *fakeProcessTimer) Stop() bool             { return true }
func (t *fakeProcessTimer) fire()                  { t.ch <- time.Unix(0, 0) }

type fakeProcessClock struct {
	timers chan *fakeProcessTimer
}

func newFakeProcessClock() *fakeProcessClock {
	return &fakeProcessClock{timers: make(chan *fakeProcessTimer)}
}

func (c *fakeProcessClock) NewTimer(duration time.Duration) processTimer {
	timer := &fakeProcessTimer{duration: duration, ch: make(chan time.Time, 1)}
	c.timers <- timer
	return timer
}

type fakeRunningProcess struct {
	wait            chan error
	directKillError error
}

func newFakeRunningProcess() *fakeRunningProcess {
	return &fakeRunningProcess{
		wait: make(chan error, 1),
	}
}

func (p *fakeRunningProcess) PID() int           { return 42 }
func (p *fakeRunningProcess) Wait() <-chan error { return p.wait }
func (p *fakeRunningProcess) KillDirect() error  { return p.directKillError }

type fakeGroupSignalTarget struct {
	signals chan syscall.Signal
	errors  []error
}

func (g *fakeGroupSignalTarget) Signal(signal syscall.Signal) error {
	g.signals <- signal
	if len(g.errors) == 0 {
		return nil
	}
	err := g.errors[0]
	g.errors = g.errors[1:]
	return err
}

func (g *fakeGroupSignalTarget) Close() {}

type fakeGroupSignalFactory struct {
	target *fakeGroupSignalTarget
	err    error
}

func (f fakeGroupSignalFactory) Open(int) (groupSignalTarget, error) {
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

func newFakeExecRunner() (execRunner, *fakeProcessClock, *fakeProcessStarter, *fakeGroupSignalTarget) {
	clock := newFakeProcessClock()
	starter := &fakeProcessStarter{
		process: newFakeRunningProcess(),
		started: make(chan processInvocation, 1),
	}
	target := &fakeGroupSignalTarget{signals: make(chan syscall.Signal, 2)}
	return execRunner{clock: clock, starter: starter, groupSignals: fakeGroupSignalFactory{target: target}}, clock, starter, target
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
	starter.process.wait <- nil
	outcome := <-done
	var timeoutError *processTimeoutError
	if !errors.As(outcome.err, &timeoutError) || timeoutError.cleanup != cleanupTerminated {
		t.Fatalf("got %T %v, want terminated timeout", outcome.err, outcome.err)
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
	grace.fire()
	if signal := <-group.signals; signal != syscall.SIGKILL {
		t.Fatalf("second signal got %v", signal)
	}
	waitBound := <-clock.timers
	if waitBound.duration != 2*time.Second {
		t.Fatalf("wait bound got %v", waitBound.duration)
	}
	waitBound.fire()
	outcome := <-done
	var waitError *processWaitError
	if !errors.As(outcome.err, &waitError) || waitError.kind != waitBoundExceeded {
		t.Fatalf("got %T %v, want bounded wait error", outcome.err, outcome.err)
	}
	var timeoutError *processTimeoutError
	if !errors.As(waitError.primary, &timeoutError) {
		t.Fatalf("wait error primary got %T", waitError.primary)
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
			<-clock.timers
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

func TestExecRunnerWaitsForLeaderReapAfterTERM(t *testing.T) {
	runner, clock, starter, group := newFakeExecRunner()
	done := runFakeProcess(runner, "/bin/true", builtInQueryLimits())
	<-starter.started
	(<-clock.timers).fire()
	if signal := <-group.signals; signal != syscall.SIGTERM {
		t.Fatalf("first signal got %v", signal)
	}
	<-clock.timers
	starter.process.wait <- nil
	outcome := <-done
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
	<-clock.timers
	starter.process.wait <- nil
	<-done
	select {
	case signal := <-group.signals:
		t.Fatalf("unexpected extra group signal %v", signal)
	default:
	}
}
