//go:build linux || freebsd

package main

import (
	"errors"
	"os/exec"
	"syscall"
)

type execRunner struct {
	clock        processClock
	starter      processStarter
	groupSignals groupSignalFactory
}

type activeProcess struct {
	process runningProcess
	group   groupSignalTarget
	path    string
	limits  processLimits
	stdout  *cappedProcessOutput
	stderr  *cappedProcessOutput
}

func newExecRunner() execRunner {
	return execRunner{clock: realProcessClock{}, starter: osProcessStarter{}, groupSignals: osGroupSignalFactory{}}
}

func (r execRunner) RunBuiltIn(path string, args []string) (processResult, error) {
	return r.run(path, args, builtInQueryLimits())
}

func (r execRunner) RunInventory(path string, args []string) (processResult, error) {
	return r.run(path, args, pkgInventoryLimits())
}

func (r execRunner) RunTrusted(path string, args []string) (processResult, error) {
	return r.run(path, args, trustedCommandLimits())
}

func (r execRunner) run(path string, args []string, limits processLimits) (processResult, error) {
	stdout := newCappedProcessOutput(limits.stdoutLimit, processStdout)
	stderr := newCappedProcessOutput(limits.stderrLimit, processStderr)
	process, err := r.starter.Start(processInvocation{
		path: path, args: append([]string(nil), args...), stdout: stdout, stderr: stderr, waitDelay: limits.waitBound,
	})
	if err != nil {
		return processResult{exitCode: -1}, &processStartError{path: path, err: err}
	}
	group, err := r.groupSignals.Open(process.PID())
	if err != nil {
		return collectProcessResult(stdout, stderr, nil), r.abortStart(process, path, limits, err)
	}
	defer group.Close()
	active := activeProcess{process: process, group: group, path: path, limits: limits, stdout: stdout, stderr: stderr}
	timeout := r.clock.NewTimer(limits.timeout)
	events := active.processEvents(timeout)
	first := <-events
	stopProcessTimer(timeout)
	if first.primary != nil {
		if first.waited {
			if err := active.group.Signal(syscall.SIGTERM); err != nil {
				return collectProcessResult(stdout, stderr, first.waitError), &processWaitError{
					path: path, kind: waitFailed, err: err, primary: first.primary,
				}
			}
			setProcessCleanup(first.primary, cleanupTerminated)
			return collectProcessResult(stdout, stderr, first.waitError), first.primary
		}
		cleanup := r.cleanup(active, first.primary)
		return collectProcessResult(stdout, stderr, first.waitError), cleanup
	}
	result := collectProcessResult(stdout, stderr, first.waitError)
	return result, classifyProcessWait(path, first.waitError)
}

func (r execRunner) abortStart(process runningProcess, path string, limits processLimits, openError error) error {
	killError := process.KillDirect()
	waitBound := r.clock.NewTimer(limits.waitBound)
	select {
	case waitError := <-process.Wait():
		stopProcessTimer(waitBound)
		return &processStartError{path: path, err: errors.Join(openError, killError, waitError)}
	case <-waitBound.Chan():
		primary := &processStartError{path: path, err: errors.Join(openError, killError)}
		return &processWaitError{path: path, kind: waitBoundExceeded, bound: limits.waitBound, primary: primary}
	}
}

type processEvent struct {
	waitError error
	primary   error
	waited    bool
}

func (p activeProcess) processEvents(timeout processTimer) <-chan processEvent {
	events := make(chan processEvent, 1)
	go func() {
		select {
		case waitError := <-p.process.Wait():
			events <- processEvent{waitError: waitError, waited: true}
		case <-timeout.Chan():
			events <- processEvent{primary: &processTimeoutError{path: p.path, timeout: p.limits.timeout}}
		case <-p.stdout.overflow:
			events <- processEvent{primary: &processOverflowError{
				path: p.path, stream: processStdout, limit: p.limits.stdoutLimit,
			}}
		case <-p.stderr.overflow:
			events <- processEvent{primary: &processOverflowError{
				path: p.path, stream: processStderr, limit: p.limits.stderrLimit,
			}}
		}
	}()
	return events
}

func (r execRunner) cleanup(
	process activeProcess,
	primary error,
) error {
	termError := process.group.Signal(syscall.SIGTERM)
	if termError != nil {
		return r.killAndWait(process, primary, &processCause{operation: "signal process group with SIGTERM", err: termError})
	}
	grace := r.clock.NewTimer(process.limits.termGrace)
	select {
	case waitError := <-process.process.Wait():
		if !validCleanupWait(waitError) {
			stopProcessTimer(grace)
			return &processWaitError{path: process.path, kind: waitFailed, err: waitError, primary: primary}
		}
		stopProcessTimer(grace)
		setProcessCleanup(primary, cleanupTerminated)
		return primary
	case <-grace.Chan():
	}
	return r.killAndWait(process, primary, nil)
}

func (r execRunner) killAndWait(
	process activeProcess,
	primary error,
	termError error,
) error {
	killError := process.group.Signal(syscall.SIGKILL)
	if killError != nil {
		directError := process.process.KillDirect()
		killError = &processCause{operation: "signal process group with SIGKILL", err: killError}
		if directError != nil {
			killError = errors.Join(killError, &processCause{operation: "kill direct child", err: directError})
		}
	}
	cleanupError := errors.Join(termError, killError)
	waitBound := r.clock.NewTimer(process.limits.waitBound)
	select {
	case waitError := <-process.process.Wait():
		if cleanupError != nil {
			stopProcessTimer(waitBound)
			return &processWaitError{path: process.path, kind: waitFailed, err: cleanupError, primary: primary}
		}
		if !validCleanupWait(waitError) {
			stopProcessTimer(waitBound)
			return &processWaitError{path: process.path, kind: waitFailed, err: waitError, primary: primary}
		}
		stopProcessTimer(waitBound)
		setProcessCleanup(primary, cleanupKilled)
		return primary
	case <-waitBound.Chan():
		return &processWaitError{
			path: process.path, kind: waitBoundExceeded, bound: process.limits.waitBound, err: cleanupError, primary: primary,
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
