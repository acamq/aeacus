//go:build linux || freebsd

package main

import (
	"errors"
	"os/exec"
	"syscall"
	"time"
)

const unixGroupPollInterval = 10 * time.Millisecond

type processReapRequest struct {
	primary      error
	cleanup      processCleanup
	cleanupError error
	bound        processTimer
}

func (r execRunner) cleanup(process *activeProcess, primary error, priorError error) error {
	termError := labelProcessError("signal process group with SIGTERM", process.group.Signal(syscall.SIGTERM))
	grace := r.clock.NewTimer(process.limits.termGrace)
	cleanupError := errors.Join(priorError, termError)
	for {
		if processTimerFired(grace) {
			return r.killAndReap(process, primary, cleanupError)
		}
		alive, aliveError := process.group.Alive()
		if aliveError != nil {
			stopProcessTimer(grace)
			return r.killAndReap(process, primary, errors.Join(
				cleanupError,
				labelProcessError("check process group after SIGTERM", aliveError),
			))
		}
		if !alive {
			stopProcessTimer(grace)
			return r.reap(process, processReapRequest{
				primary: primary, cleanup: cleanupTerminated, cleanupError: cleanupError,
			})
		}
		poll := r.clock.NewTimer(unixGroupPollInterval)
		select {
		case <-grace.Chan():
			stopProcessTimer(poll)
			return r.killAndReap(process, primary, cleanupError)
		case <-poll.Chan():
		}
	}
}

func (r execRunner) killAndReap(process *activeProcess, primary error, cleanupError error) error {
	killError := process.group.Signal(syscall.SIGKILL)
	if killError != nil {
		killError = errors.Join(
			labelProcessError("signal process group with SIGKILL", killError),
			labelProcessError("kill direct child after group SIGKILL failure", process.process.KillDirect()),
		)
	}
	request := processReapRequest{
		primary: primary, cleanup: cleanupKilled,
		cleanupError: errors.Join(cleanupError, killError),
		bound:        r.clock.NewTimer(process.limits.waitBound),
	}
	for {
		if processTimerFired(request.bound) {
			return process.reapExpired(request)
		}
		alive, aliveError := process.group.Alive()
		if aliveError != nil {
			request.cleanup = cleanupNone
			request.cleanupError = errors.Join(
				request.cleanupError,
				labelProcessError("check process group after SIGKILL", aliveError),
			)
			return r.reap(process, request)
		}
		if !alive {
			return process.reapWithin(request)
		}
		poll := r.clock.NewTimer(unixGroupPollInterval)
		select {
		case <-request.bound.Chan():
			stopProcessTimer(poll)
			return process.reapExpired(request)
		case <-poll.Chan():
		}
	}
}

func (r execRunner) reap(process *activeProcess, request processReapRequest) error {
	if request.bound == nil {
		request.bound = r.clock.NewTimer(process.limits.waitBound)
	}
	return process.reapWithin(request)
}

func (p *activeProcess) reapWithin(request processReapRequest) error {
	wait := p.process.Wait()
	select {
	case waitError := <-wait:
		stopProcessTimer(request.bound)
		return p.finishReap(waitError, request)
	case <-request.bound.Chan():
		return p.reapExpiredFrom(wait, request)
	}
}

func (p *activeProcess) reapExpired(request processReapRequest) error {
	return p.reapExpiredFrom(p.process.Wait(), request)
}

func (p *activeProcess) reapExpiredFrom(wait <-chan error, request processReapRequest) error {
	select {
	case p.waitError = <-wait:
		request.cleanupError = errors.Join(
			request.cleanupError,
			labelProcessError("wait for direct child at final deadline", p.waitError),
		)
	default:
	}
	return &processWaitError{
		path: p.path, kind: waitBoundExceeded, bound: p.limits.waitBound,
		err: request.cleanupError, primary: request.primary,
	}
}

func (p *activeProcess) finishReap(waitError error, request processReapRequest) error {
	p.waitError = waitError
	if request.cleanupError != nil || request.primary != nil && !validCleanupWait(waitError) {
		return &processWaitError{
			path: p.path, kind: waitFailed,
			err: errors.Join(
				request.cleanupError,
				labelProcessError("wait for direct child", waitError),
			),
			primary: request.primary,
		}
	}
	setProcessCleanup(request.primary, request.cleanup)
	return request.primary
}

func (r execRunner) reapAndClassify(process *activeProcess, observationError error) error {
	request := processReapRequest{
		cleanupError: labelProcessError("observe leader exit", observationError),
		bound:        r.clock.NewTimer(process.limits.waitBound),
	}
	wait := process.process.Wait()
	select {
	case waitError := <-wait:
		stopProcessTimer(request.bound)
		process.waitError = waitError
		if request.cleanupError != nil {
			return &processWaitError{
				path: process.path, kind: waitFailed,
				err: errors.Join(
					request.cleanupError,
					labelProcessError("wait for direct child", waitError),
				),
			}
		}
		return classifyProcessWait(process.path, waitError)
	case <-request.bound.Chan():
		return process.reapExpiredFrom(wait, request)
	}
}

func processTimerFired(timer processTimer) bool {
	select {
	case <-timer.Chan():
		return true
	default:
		return false
	}
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
