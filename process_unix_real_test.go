//go:build linux || freebsd

package main

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
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

func TestExecRunnerRealProcessHandlesTERMAndKILLEscalation(t *testing.T) {
	t.Setenv(processHelperEnvironment, "1")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		mode        string
		wantCleanup processCleanup
		escalate    bool
	}{
		{"term", cleanupTerminated, false},
		{"ignore-term", cleanupKilled, true},
	}
	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			clock := newFakeProcessClock()
			runner := execRunner{clock: clock, starter: osProcessStarter{}, groupSignals: osGroupSignalFactory{}}
			socket, socketPath := newProcessReadySocket(t)
			done := make(chan asyncProcessResult, 1)
			go func() {
				result, runErr := runner.RunBuiltIn(executable, helperArgs(tt.mode, socketPath))
				done <- asyncProcessResult{result: result, err: runErr}
			}()
			ready := make([]byte, 8)
			if _, _, err := socket.ReadFromUnix(ready); err != nil {
				t.Fatal(err)
			}
			timeout := <-clock.timers
			timeout.fire()
			grace := <-clock.timers
			if tt.escalate {
				grace.fire()
				<-clock.timers
			}
			outcome := <-done
			var timeoutError *processTimeoutError
			if !errors.As(outcome.err, &timeoutError) || timeoutError.cleanup != tt.wantCleanup {
				t.Fatalf("got %T %v, want cleanup %v", outcome.err, outcome.err, tt.wantCleanup)
			}
		})
	}
}

func TestExecRunnerTerminatesProcessGroupDescendant(t *testing.T) {
	t.Setenv(processHelperEnvironment, "1")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	clock := newFakeProcessClock()
	runner := execRunner{clock: clock, starter: osProcessStarter{}, groupSignals: osGroupSignalFactory{}}
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
	<-clock.timers
	outcome := <-done
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
