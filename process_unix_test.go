//go:build linux || freebsd

package main

import (
	"errors"
	"io"
	"os"
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
	wait    chan error
	signals chan syscall.Signal
}

func newFakeRunningProcess() *fakeRunningProcess {
	return &fakeRunningProcess{wait: make(chan error, 1), signals: make(chan syscall.Signal, 2)}
}

func (p *fakeRunningProcess) Wait() <-chan error { return p.wait }
func (p *fakeRunningProcess) SignalGroup(signal syscall.Signal) error {
	p.signals <- signal
	return nil
}
func (p *fakeRunningProcess) KillDirect() error         { return nil }
func (p *fakeRunningProcess) GroupAlive() (bool, error) { return false, nil }

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

func runFakeProcess(runner execRunner, path string, profile processProfile) <-chan asyncProcessResult {
	done := make(chan asyncProcessResult, 1)
	go func() {
		result, err := runner.Run(path, []string{"literal argument"}, profile)
		done <- asyncProcessResult{result: result, err: err}
	}()
	return done
}

func newFakeExecRunner() (execRunner, *fakeProcessClock, *fakeProcessStarter) {
	clock := newFakeProcessClock()
	starter := &fakeProcessStarter{
		process: newFakeRunningProcess(),
		started: make(chan processInvocation, 1),
	}
	return execRunner{clock: clock, starter: starter}, clock, starter
}

func TestProcessProfilesAreCompileTimeBounded(t *testing.T) {
	tests := []struct {
		profile processProfile
		want    processLimits
	}{
		{builtInQuery, processLimits{10 * time.Second, 1 << 20, 1 << 20, 2 * time.Second, 2 * time.Second}},
		{pkgInventory, processLimits{30 * time.Second, 16 << 20, 1 << 20, 2 * time.Second, 2 * time.Second}},
		{trustedCommand, processLimits{30 * time.Second, 1 << 20, 1 << 20, 2 * time.Second, 2 * time.Second}},
	}
	for _, tt := range tests {
		if got := tt.profile.Limits(); got != tt.want {
			t.Fatalf("profile %d got %+v, want %+v", tt.profile, got, tt.want)
		}
	}
}

func TestExecRunnerTimeoutTerminatesWithinExactFakeClockBounds(t *testing.T) {
	runner, clock, starter := newFakeExecRunner()
	done := runFakeProcess(runner, "/bin/true", builtInQuery)
	invocation := <-starter.started
	if invocation.path != "/bin/true" || len(invocation.args) != 1 || invocation.args[0] != "literal argument" {
		t.Fatalf("unexpected invocation: %#v", invocation)
	}
	timeout := <-clock.timers
	if timeout.duration != 10*time.Second {
		t.Fatalf("timeout got %v", timeout.duration)
	}
	timeout.fire()
	if signal := <-starter.process.signals; signal != syscall.SIGTERM {
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
	runner, clock, starter := newFakeExecRunner()
	done := runFakeProcess(runner, "/bin/true", trustedCommand)
	<-starter.started
	timeout := <-clock.timers
	if timeout.duration != 30*time.Second {
		t.Fatalf("timeout got %v", timeout.duration)
	}
	timeout.fire()
	<-starter.process.signals
	grace := <-clock.timers
	grace.fire()
	if signal := <-starter.process.signals; signal != syscall.SIGKILL {
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
			runner, clock, starter := newFakeExecRunner()
			done := runFakeProcess(runner, "/bin/true", builtInQuery)
			invocation := <-starter.started
			<-clock.timers
			if _, err := tt.writer(invocation).Write(make([]byte, 1<<20)); err != nil {
				t.Fatalf("write exact limit: %v", err)
			}
			if _, err := tt.writer(invocation).Write([]byte{'x'}); err != nil {
				t.Fatalf("write overflow marker: %v", err)
			}
			if signal := <-starter.process.signals; signal != syscall.SIGTERM {
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

func TestExecRunnerClassifiesStartAndWaitFailures(t *testing.T) {
	starter := &fakeProcessStarter{startError: os.ErrPermission, started: make(chan processInvocation, 1)}
	runner := execRunner{clock: newFakeProcessClock(), starter: starter}
	_, err := runner.Run("/denied", nil, builtInQuery)
	var startError *processStartError
	if !errors.As(err, &startError) {
		t.Fatalf("got %T, want start error", err)
	}

	runner2, clock, starter := newFakeExecRunner()
	done := runFakeProcess(runner2, "/bin/true", builtInQuery)
	<-starter.started
	<-clock.timers
	starter.process.wait <- errors.New("wait failed")
	outcome := <-done
	var waitError *processWaitError
	if !errors.As(outcome.err, &waitError) || waitError.kind != waitFailed {
		t.Fatalf("got %T %v, want wait error", outcome.err, outcome.err)
	}
}
