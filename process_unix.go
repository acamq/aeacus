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
	groups  processGroupFactory
}

type activeProcess struct {
	process       runningProcess
	group         processGroup
	path          string
	limits        processLimits
	stdout        *cappedProcessOutput
	stderr        *cappedProcessOutput
	waitError     error
	cutReady      chan<- struct{}
	releaseCut    <-chan struct{}
	eventAdmitted chan<- processEventKind
}

func newExecRunner() execRunner {
	return execRunner{clock: realProcessClock{}, starter: osProcessStarter{}, groups: osProcessGroupFactory{}}
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
	group, err := r.groups.Open(process.PID())
	if err != nil {
		return collectProcessResult(stdout, stderr, nil), r.abortStart(process, path, limits, err)
	}
	defer group.Close()
	active := &activeProcess{process: process, group: group, path: path, limits: limits, stdout: stdout, stderr: stderr}
	timeout := r.clock.NewTimer(limits.timeout)
	first := active.firstEvent(timeout)
	if first.primary == nil {
		alive, aliveError := active.group.Alive()
		if aliveError != nil {
			stopProcessTimer(timeout)
			classification := r.reap(active, processReapRequest{
				cleanup: cleanupNone,
				cleanupError: errors.Join(
					labelProcessError("observe leader exit", first.observationError),
					labelProcessError("check process group after leader exit", aliveError),
				),
			})
			return collectProcessResult(stdout, stderr, active.waitError), classification
		}
		if alive {
			stopProcessTimer(timeout)
			if cleanup := r.cleanup(active, nil, labelProcessError("observe leader exit", first.observationError)); cleanup != nil {
				return collectProcessResult(stdout, stderr, active.waitError), cleanup
			}
			return collectProcessResult(stdout, stderr, active.waitError), classifyProcessWait(path, active.waitError)
		}
	}
	stopProcessTimer(timeout)
	if first.primary != nil {
		cleanup := r.cleanup(active, first.primary, labelProcessError("observe leader exit", first.observationError))
		return collectProcessResult(stdout, stderr, active.waitError), cleanup
	}
	classification := r.reapAndClassify(active, first.observationError)
	return collectProcessResult(stdout, stderr, active.waitError), classification
}

func (r execRunner) abortStart(process runningProcess, path string, limits processLimits, openError error) error {
	killError := labelProcessError("kill direct child after group identity open failure", process.KillDirect())
	waitBound := r.clock.NewTimer(limits.waitBound)
	wait := process.Wait()
	select {
	case waitError := <-wait:
		stopProcessTimer(waitBound)
		return &processStartError{path: path, err: errors.Join(
			labelProcessError("open process group identity", openError), killError,
			labelProcessError("wait for direct child after identity open failure", waitError),
		)}
	case <-waitBound.Chan():
		primary := &processStartError{path: path, err: errors.Join(
			labelProcessError("open process group identity", openError), killError,
		)}
		var waitError error
		select {
		case waitError = <-wait:
		default:
		}
		return &processWaitError{
			path: path, kind: waitBoundExceeded, bound: limits.waitBound,
			err: waitError, primary: primary,
		}
	}
}

type processEvent struct {
	primary          error
	observationError error
}

func (p *activeProcess) firstEvent(timeout processTimer) processEvent {
	return newProcessEventCoordinator(p, timeout).first(p)
}

func (p *activeProcess) waitForEventCut() {
	if p.cutReady != nil {
		p.cutReady <- struct{}{}
		<-p.releaseCut
	}
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
