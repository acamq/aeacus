//go:build linux || freebsd

package main

import "sync"

type processEventLatch struct {
	mu         sync.Mutex
	wake       chan struct{}
	path       string
	limits     processLimits
	held       bool
	timeoutSet bool
	stdoutSet  bool
	stderrSet  bool
	leaderSet  bool
	leaderErr  error
}

func newProcessEventLatch(path string, limits processLimits) *processEventLatch {
	return &processEventLatch{wake: make(chan struct{}, 1), path: path, limits: limits}
}

func (l *processEventLatch) hold() {
	l.mu.Lock()
	l.held = true
	l.mu.Unlock()
}

func (l *processEventLatch) release() {
	l.mu.Lock()
	l.held = false
	l.notifyLocked()
	l.mu.Unlock()
}

func (l *processEventLatch) timeout() {
	l.mu.Lock()
	l.timeoutSet = true
	l.notifyLocked()
	l.mu.Unlock()
}

func (l *processEventLatch) overflow(stream processOutputStream) {
	l.mu.Lock()
	if stream == processStdout {
		l.stdoutSet = true
	} else {
		l.stderrSet = true
	}
	l.notifyLocked()
	l.mu.Unlock()
}

func (l *processEventLatch) leader(err error) {
	l.mu.Lock()
	l.leaderSet = true
	l.leaderErr = err
	l.notifyLocked()
	l.mu.Unlock()
}

func (l *processEventLatch) wait() processEvent {
	<-l.wake
	event, _ := l.event()
	return event
}

func (l *processEventLatch) event() (processEvent, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	switch {
	case l.timeoutSet:
		return processEvent{primary: &processTimeoutError{path: l.path, timeout: l.limits.timeout}}, true
	case l.stdoutSet:
		return processEvent{primary: &processOverflowError{path: l.path, stream: processStdout, limit: l.limits.stdoutLimit}}, true
	case l.stderrSet:
		return processEvent{primary: &processOverflowError{path: l.path, stream: processStderr, limit: l.limits.stderrLimit}}, true
	case l.leaderSet:
		return processEvent{observationError: l.leaderErr}, true
	}
	return processEvent{}, false
}

func (l *processEventLatch) notifyLocked() {
	if l.held || (!l.timeoutSet && !l.stdoutSet && !l.stderrSet && !l.leaderSet) {
		return
	}
	select {
	case l.wake <- struct{}{}:
	default:
	}
}
