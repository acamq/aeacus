//go:build linux || freebsd

package main

import (
	"errors"
	"os/exec"
	"syscall"
)

type execRunner struct {
	clock   processClock
	starter processStarter
}

type activeProcess struct {
	process runningProcess
	path    string
	limits  processLimits
	stdout  *cappedProcessOutput
	stderr  *cappedProcessOutput
}

func newExecRunner() execRunner {
	return execRunner{clock: realProcessClock{}, starter: osProcessStarter{}}
}

func (r execRunner) Run(path string, args []string, profile processProfile) (processResult, error) {
	limits := profile.Limits()
	stdout := newCappedProcessOutput(limits.stdoutLimit, processStdout)
	stderr := newCappedProcessOutput(limits.stderrLimit, processStderr)
	process, err := r.starter.Start(processInvocation{
		path: path, args: append([]string(nil), args...), stdout: stdout, stderr: stderr, waitDelay: limits.waitBound,
	})
	if err != nil {
		return processResult{exitCode: -1}, &processStartError{path: path, err: err}
	}
	active := activeProcess{process: process, path: path, limits: limits, stdout: stdout, stderr: stderr}
	timeout := r.clock.NewTimer(limits.timeout)
	waitError, primary, waited := active.await(timeout)
	stopProcessTimer(timeout)
	if primary != nil {
		if waited {
			if err := active.process.SignalGroup(syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
				return collectProcessResult(stdout, stderr, waitError), &processWaitError{
					path: path, kind: waitFailed, err: err, primary: primary,
				}
			}
			setProcessCleanup(primary, cleanupTerminated)
			return collectProcessResult(stdout, stderr, waitError), primary
		}
		cleanup := r.cleanup(active, primary)
		return collectProcessResult(stdout, stderr, waitError), cleanup
	}
	result := collectProcessResult(stdout, stderr, waitError)
	return result, classifyProcessWait(path, waitError)
}

func (p activeProcess) await(timeout processTimer) (error, error, bool) {
	var waitError error
	var primary error
	waited := false
	select {
	case waitError = <-p.process.Wait():
		waited = true
	case <-timeout.Chan():
		primary = &processTimeoutError{path: p.path, timeout: p.limits.timeout}
	case <-p.stdout.overflow:
		primary = &processOverflowError{path: p.path, stream: processStdout, limit: p.limits.stdoutLimit}
	case <-p.stderr.overflow:
		primary = &processOverflowError{path: p.path, stream: processStderr, limit: p.limits.stderrLimit}
	}
	if _, isTimeout := primary.(*processTimeoutError); !isTimeout {
		select {
		case <-timeout.Chan():
			primary = &processTimeoutError{path: p.path, timeout: p.limits.timeout}
		default:
		}
	}
	if _, isTimeout := primary.(*processTimeoutError); !isTimeout {
		if p.stdout.Exceeded() {
			primary = &processOverflowError{path: p.path, stream: processStdout, limit: p.limits.stdoutLimit}
		} else if p.stderr.Exceeded() {
			primary = &processOverflowError{path: p.path, stream: processStderr, limit: p.limits.stderrLimit}
		}
	}
	return waitError, primary, waited
}

func (r execRunner) cleanup(
	process activeProcess,
	primary error,
) error {
	termError := process.process.SignalGroup(syscall.SIGTERM)
	if termError != nil && !errors.Is(termError, syscall.ESRCH) {
		return r.killAndWait(process, primary)
	}
	grace := r.clock.NewTimer(process.limits.termGrace)
	select {
	case waitError := <-process.process.Wait():
		stopProcessTimer(grace)
		if !validCleanupWait(waitError) {
			return &processWaitError{path: process.path, kind: waitFailed, err: waitError, primary: primary}
		}
		setProcessCleanup(primary, cleanupTerminated)
		return primary
	case <-grace.Chan():
	}
	return r.killAndWait(process, primary)
}

func (r execRunner) killAndWait(
	process activeProcess,
	primary error,
) error {
	killError := process.process.SignalGroup(syscall.SIGKILL)
	if errors.Is(killError, syscall.ESRCH) {
		killError = nil
	} else if killError != nil {
		directError := process.process.KillDirect()
		killError = errors.Join(killError, directError)
	}
	waitBound := r.clock.NewTimer(process.limits.waitBound)
	select {
	case waitError := <-process.process.Wait():
		stopProcessTimer(waitBound)
		if killError != nil {
			return &processWaitError{path: process.path, kind: waitFailed, err: killError, primary: primary}
		}
		if !validCleanupWait(waitError) {
			return &processWaitError{path: process.path, kind: waitFailed, err: waitError, primary: primary}
		}
		setProcessCleanup(primary, cleanupKilled)
		return primary
	case <-waitBound.Chan():
		return &processWaitError{
			path: process.path, kind: waitBoundExceeded, bound: process.limits.waitBound, err: killError, primary: primary,
		}
	}
}

func validCleanupWait(err error) bool {
	if err == nil {
		return true
	}
	var exitError *exec.ExitError
	if !errors.As(err, &exitError) {
		return false
	}
	status, ok := exitError.Sys().(syscall.WaitStatus)
	return ok && status.Signaled()
}

func setProcessCleanup(primary error, cleanup processCleanup) {
	var timeout *processTimeoutError
	if errors.As(primary, &timeout) {
		timeout.cleanup = cleanup
		return
	}
	var overflow *processOverflowError
	if errors.As(primary, &overflow) {
		overflow.cleanup = cleanup
	}
}

func collectProcessResult(stdout, stderr *cappedProcessOutput, waitError error) processResult {
	exitCode := 0
	var exitError *exec.ExitError
	if errors.As(waitError, &exitError) {
		exitCode = exitError.ExitCode()
	}
	return processResult{stdout: stdout.Bytes(), stderr: stderr.Bytes(), exitCode: exitCode}
}

func classifyProcessWait(path string, err error) error {
	if err == nil {
		return nil
	}
	var exitError *exec.ExitError
	if !errors.As(err, &exitError) {
		return &processWaitError{path: path, kind: waitFailed, err: err}
	}
	status, ok := exitError.Sys().(syscall.WaitStatus)
	if ok && status.Signaled() {
		return &processSignalError{path: path, signal: status.Signal()}
	}
	return &processExitError{path: path, code: exitError.ExitCode()}
}

func stopProcessTimer(timer processTimer) {
	if timer.Stop() {
		return
	}
	select {
	case <-timer.Chan():
	default:
	}
}

var unixRunner = newExecRunner()
