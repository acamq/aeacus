//go:build linux || freebsd

package main

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestExecRunnerRealProcessClassifiesExitAndSignal(t *testing.T) {
	t.Setenv(processHelperEnvironment, "1")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	runner := newExecRunner()

	_, err = runner.RunBuiltIn(executable, helperArgs("exit", "7"))
	var exitError *processExitError
	if !errors.As(err, &exitError) || exitError.code != 7 {
		t.Fatalf("got %T %v, want exit 7", err, err)
	}
	_, err = runner.RunBuiltIn(executable, helperArgs("signal"))
	var signalError *processSignalError
	if !errors.As(err, &signalError) || signalError.signal != syscall.SIGTERM {
		t.Fatalf("got %T %v, want SIGTERM", err, err)
	}
}

func TestExecRunnerKillsGroupAfterLeaderExitsOnTERM(t *testing.T) {
	testLeaderExitWithLiveDescendant(t, "leader-exits-on-term")
}

func TestExecRunnerKillsGroupWhenExitedLeaderDescendantHoldsPipes(t *testing.T) {
	testExitedLeaderWithPipeHoldingDescendant(t)
}

func TestExecRunnerReapsLeaderExactlyOnceAfterGroupCleanup(t *testing.T) {
	t.Setenv(processHelperEnvironment, "1")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	clock := newFakeProcessClock()
	starter := &countingProcessStarter{delegate: osProcessStarter{}, started: make(chan *countingRunningProcess, 1)}
	runner := execRunner{clock: clock, starter: starter, groups: osProcessGroupFactory{}}
	socket, socketPath := newProcessReadySocket(t)
	done := make(chan asyncProcessResult, 1)
	go func() {
		result, runErr := runner.RunBuiltIn(executable, helperArgs("leader-exits-on-term", socketPath))
		done <- asyncProcessResult{result: result, err: runErr}
	}()
	process := <-starter.started
	descendantPID := readHelperPID(t, socket)
	readHelperMessage(t, socket)
	(<-clock.timers).fire()
	grace := <-clock.timers
	if got := process.waitCalls.Load(); got != 0 {
		t.Fatalf("leader reaped before group cleanup: Wait calls=%d", got)
	}
	grace.fire()
	<-clock.timers
	outcome := <-done
	if got := process.waitCalls.Load(); got != 1 {
		t.Fatalf("leader Wait calls=%d, want exactly 1", got)
	}
	var timeoutError *processTimeoutError
	if !errors.As(outcome.err, &timeoutError) || timeoutError.cleanup != cleanupKilled {
		t.Fatalf("got %T %v, want killed timeout", outcome.err, outcome.err)
	}
	if err := syscall.Kill(descendantPID, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("descendant %d remains: %v", descendantPID, err)
	}
}

type countingProcessStarter struct {
	delegate processStarter
	started  chan *countingRunningProcess
}

func (s *countingProcessStarter) Start(invocation processInvocation) (runningProcess, error) {
	process, err := s.delegate.Start(invocation)
	if err != nil {
		return nil, err
	}
	counted := &countingRunningProcess{runningProcess: process}
	s.started <- counted
	return counted, nil
}

type countingRunningProcess struct {
	runningProcess
	waitCalls atomic.Int32
}

func (p *countingRunningProcess) Wait() <-chan error {
	p.waitCalls.CompareAndSwap(0, 1)
	return p.runningProcess.Wait()
}

func testLeaderExitWithLiveDescendant(t *testing.T, mode string) {
	t.Helper()
	t.Setenv(processHelperEnvironment, "1")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	clock := newFakeProcessClock()
	runner := newExecRunner()
	runner.clock = clock
	socket, socketPath := newProcessReadySocket(t)
	done := make(chan asyncProcessResult, 1)
	go func() {
		result, runErr := runner.RunBuiltIn(executable, helperArgs(mode, socketPath))
		done <- asyncProcessResult{result: result, err: runErr}
	}()
	descendantPID := readHelperPID(t, socket)
	if ready := readHelperMessage(t, socket); ready != "descendant-ready" {
		t.Fatalf("descendant readiness got %q", ready)
	}
	timeout := <-clock.timers
	select {
	case outcome := <-done:
		t.Fatalf("Run returned before timeout with live descendant: %v", outcome.err)
	default:
	}
	timeout.fire()
	grace := <-clock.timers
	select {
	case outcome := <-done:
		t.Fatalf("Run returned before TERM grace with live descendant: %v", outcome.err)
	default:
	}
	grace.fire()
	if waitBound := requireFakeTimerDuration(t, clock, 2*time.Second, "real process final wait"); waitBound.duration != 2*time.Second {
		t.Fatalf("wait bound got %v", waitBound.duration)
	}
	outcome := <-done
	var timeoutError *processTimeoutError
	if !errors.As(outcome.err, &timeoutError) || timeoutError.cleanup != cleanupKilled {
		t.Fatalf("got %T %v, want killed timeout", outcome.err, outcome.err)
	}
	if err := syscall.Kill(descendantPID, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("descendant %d remains: %v", descendantPID, err)
	}
}

func testExitedLeaderWithPipeHoldingDescendant(t *testing.T) {
	t.Helper()
	t.Setenv(processHelperEnvironment, "1")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	clock := newFakeProcessClock()
	runner := newExecRunner()
	runner.clock = clock
	socket, socketPath := newProcessReadySocket(t)
	done := make(chan asyncProcessResult, 1)
	go func() {
		result, runErr := runner.RunBuiltIn(executable, helperArgs("leader-exits-descendant-holds-pipes", socketPath))
		done <- asyncProcessResult{result: result, err: runErr}
	}()
	descendantPID := readHelperPID(t, socket)
	if ready := readHelperMessage(t, socket); ready != "descendant-ready" {
		t.Fatalf("descendant readiness got %q", ready)
	}
	grace := waitForCleanupTimer(t, clock, done)
	select {
	case outcome := <-done:
		t.Fatalf("Run returned before TERM grace with live pipe holder: %v", outcome.err)
	default:
	}
	grace.fire()
	waitBound := requireFakeTimerDuration(t, clock, 2*time.Second, "WaitDelay final wait")
	if waitBound.duration != 2*time.Second {
		t.Fatalf("wait bound got %v", waitBound.duration)
	}
	outcome := <-done
	if outcome.err != nil {
		t.Fatalf("successful leader completion got %T %v", outcome.err, outcome.err)
	}
	if err := syscall.Kill(descendantPID, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("pipe-holding descendant %d remains: %v", descendantPID, err)
	}
}

func waitForCleanupTimer(t *testing.T, clock *fakeProcessClock, done <-chan asyncProcessResult) *fakeProcessTimer {
	t.Helper()
	timeout := <-clock.timers
	select {
	case grace := <-clock.timers:
		stopProcessTimer(timeout)
		return grace
	case outcome := <-done:
		t.Fatalf("Run returned on WaitDelay before group cleanup: %v", outcome.err)
	case <-time.After(3 * time.Second):
		timeout.fire()
	}
	return <-clock.timers
}

func readHelperPID(t *testing.T, socket *net.UnixConn) int {
	t.Helper()
	message := readHelperMessage(t, socket)
	pid, err := strconv.Atoi(message)
	if err != nil {
		t.Fatalf("parse descendant pid: %v", err)
	}
	return pid
}

func readHelperMessage(t *testing.T, socket *net.UnixConn) string {
	t.Helper()
	buffer := make([]byte, 64)
	size, _, err := socket.ReadFromUnix(buffer)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(buffer[:size]))
}

func TestExecRunnerRealProcessEnforcesExactOutputLimits(t *testing.T) {
	t.Setenv(processHelperEnvironment, "1")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, stream := range []string{"stdout", "stderr"} {
		t.Run(stream, func(t *testing.T) {
			runner := newExecRunner()
			result, runErr := runner.RunBuiltIn(executable, helperArgs(stream, strconv.Itoa(1<<20)))
			if runErr != nil {
				t.Fatalf("exact limit: %v", runErr)
			}
			if stream == "stdout" && len(result.stdout) != 1<<20 {
				t.Fatalf("stdout length got %d", len(result.stdout))
			}
			if stream == "stderr" && len(result.stderr) != 1<<20 {
				t.Fatalf("stderr length got %d", len(result.stderr))
			}

			overflowResult, runErr := runner.RunBuiltIn(executable, helperArgs(stream, strconv.Itoa((1<<20)+1)))
			var overflow *processOverflowError
			if !errors.As(runErr, &overflow) {
				t.Fatalf("limit+1 got %T %v", runErr, runErr)
			}
			if stream == "stdout" && len(overflowResult.stdout) != 1<<20 {
				t.Fatalf("overflow stdout length got %d", len(overflowResult.stdout))
			}
			if stream == "stderr" && len(overflowResult.stderr) != 1<<20 {
				t.Fatalf("overflow stderr length got %d", len(overflowResult.stderr))
			}
		})
	}
}

func TestExecRunnerInventoryEnforcesSixteenMiBOutputLimit(t *testing.T) {
	t.Setenv(processHelperEnvironment, "1")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	runner := newExecRunner()
	result, runErr := runner.RunInventory(executable, helperArgs("stdout", strconv.Itoa(16<<20)))
	if runErr != nil || len(result.stdout) != 16<<20 {
		t.Fatalf("exact inventory limit got (%d, %v)", len(result.stdout), runErr)
	}
	result, runErr = runner.RunInventory(executable, helperArgs("stdout", strconv.Itoa((16<<20)+1)))
	var overflow *processOverflowError
	if !errors.As(runErr, &overflow) || overflow.limit != 16<<20 || len(result.stdout) != 16<<20 {
		t.Fatalf("inventory limit+1 got (%d, %T %v)", len(result.stdout), runErr, runErr)
	}
}

func TestExecRunnerTerminatesProcessGroupDescendant(t *testing.T) {
	t.Setenv(processHelperEnvironment, "1")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	clock := newFakeProcessClock()
	runner := execRunner{clock: clock, starter: osProcessStarter{}, groups: osProcessGroupFactory{}}
	socket, socketPath := newProcessReadySocket(t)
	done := make(chan asyncProcessResult, 1)
	go func() {
		result, runErr := runner.RunBuiltIn(executable, helperArgs("descendant", socketPath))
		done <- asyncProcessResult{result: result, err: runErr}
	}()
	pidBuffer := make([]byte, 32)
	pidSize, _, err := socket.ReadFromUnix(pidBuffer)
	if err != nil {
		t.Fatal(err)
	}
	(<-clock.timers).fire()
	outcome := fireCleanupTimerUntilDone(t, clock, done)
	var timeoutError *processTimeoutError
	if !errors.As(outcome.err, &timeoutError) {
		t.Fatalf("got %T %v, want timeout", outcome.err, outcome.err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidBuffer[:pidSize])))
	if err != nil {
		t.Fatalf("parse descendant pid: %v", err)
	}
	if err := syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("descendant %d remains: %v", pid, err)
	}
}

func fireCleanupTimerUntilDone(t *testing.T, clock *fakeProcessClock, done <-chan asyncProcessResult) asyncProcessResult {
	t.Helper()
	for {
		select {
		case timer := <-clock.timers:
			timer.fire()
		case outcome := <-done:
			return outcome
		case <-time.After(3 * time.Second):
			t.Fatal("Run did not finish process-group cleanup")
		}
	}
}

func newProcessReadySocket(t *testing.T) (*net.UnixConn, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ready.sock")
	socket, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: path, Net: "unixgram"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := socket.Close(); err != nil {
			t.Errorf("close readiness socket: %v", err)
		}
	})
	return socket, path
}
