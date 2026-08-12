//go:build linux || freebsd

package main

import (
	"errors"
	"os/exec"
	"syscall"
)

func (r execRunner) cleanup(process *activeProcess, primary error) error {
	termError := process.group.Signal(syscall.SIGTERM)
	grace := r.clock.NewTimer(process.limits.termGrace)
	for {
		alive, aliveError := process.group.Alive()
		if aliveError != nil {
			stopProcessTimer(grace)
			return r.killAndReap(process, primary, errors.Join(
				labelProcessError("check process group after SIGTERM", aliveError),
				labelProcessError("signal process group with SIGTERM", termError),
			))
		}
		if !alive {
			stopProcessTimer(grace)
			return process.reap(primary, cleanupTerminated, labelProcessError("signal process group with SIGTERM", termError))
		}
		select {
		case <-grace.Chan():
			return r.killAndReap(process, primary, labelProcessError("signal process group with SIGTERM", termError))
		case <-process.group.Changed():
		}
	}
}

func (r execRunner) killAndReap(process *activeProcess, primary error, cleanupError error) error {
	killError := process.group.Signal(syscall.SIGKILL)
	if killError != nil {
		killError = errors.Join(
			labelProcessError("signal process group with SIGKILL", killError),
			labelProcessError("kill direct child", process.process.KillDirect()),
		)
	}
	wait := process.process.Wait()
	waitBound := r.clock.NewTimer(process.limits.waitBound)
	for {
		alive, aliveError := process.group.Alive()
		if aliveError != nil {
			stopProcessTimer(waitBound)
			return process.reapFrom(wait, primary, cleanupNone, errors.Join(
				cleanupError,
				killError,
				labelProcessError("check process group after SIGKILL", aliveError),
			))
		}
		if !alive {
			stopProcessTimer(waitBound)
			return process.reapFrom(wait, primary, cleanupKilled, errors.Join(cleanupError, killError))
		}
		select {
		case <-waitBound.Chan():
			return process.reapBounded(wait, primary, errors.Join(cleanupError, killError))
		case <-process.group.Changed():
		}
	}
}

func (p *activeProcess) reap(primary error, cleanup processCleanup, cleanupError error) error {
	return p.reapFrom(p.process.Wait(), primary, cleanup, cleanupError)
}

func (p *activeProcess) reapFrom(wait <-chan error, primary error, cleanup processCleanup, cleanupError error) error {
	waitError := <-wait
	p.waitError = waitError
	if cleanupError != nil || primary != nil && !validCleanupWait(waitError) {
		return &processWaitError{path: p.path, kind: waitFailed, err: errors.Join(cleanupError, waitError), primary: primary}
	}
	setProcessCleanup(primary, cleanup)
	return primary
}

func (p *activeProcess) reapBounded(wait <-chan error, primary error, cleanupError error) error {
	select {
	case waitError := <-wait:
		p.waitError = waitError
	default:
	}
	return &processWaitError{path: p.path, kind: waitBoundExceeded, bound: p.limits.waitBound, err: cleanupError, primary: primary}
}

func (p *activeProcess) reapAndClassify(observationError error) error {
	waitError := <-p.process.Wait()
	p.waitError = waitError
	if observationError != nil {
		return &processWaitError{path: p.path, kind: waitFailed, err: observationError}
	}
	return classifyProcessWait(p.path, waitError)
}

func labelProcessError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return &processCause{operation: operation, err: err}
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
