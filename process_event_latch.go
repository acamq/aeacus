//go:build linux || freebsd

package main

import (
	"sync"
	"time"
)

type processEventKind uint8

const (
	processTimeoutEvent processEventKind = iota
	processStdoutEvent
	processStderrEvent
	processLeaderEvent
)

type processEventCoordinator struct {
	mu       sync.Mutex
	wake     chan struct{}
	snapshot chan struct{}
	ready    sync.WaitGroup
	done     sync.WaitGroup

	path      string
	limits    processLimits
	cut       bool
	timeout   bool
	stdout    bool
	stderr    bool
	leader    bool
	leaderErr error

	admitted chan<- processEventKind
}

func newProcessEventCoordinator(process *activeProcess, timeout processTimer) *processEventCoordinator {
	coordinator := &processEventCoordinator{
		wake: make(chan struct{}, 1), snapshot: make(chan struct{}), path: process.path,
		limits: process.limits, admitted: process.eventAdmitted,
	}
	coordinator.ready.Add(4)
	coordinator.done.Add(4)
	go coordinator.watchTimeout(timeout.Chan())
	go coordinator.watchOverflow(process.stdout.overflow, processStdoutEvent)
	go coordinator.watchOverflow(process.stderr.overflow, processStderrEvent)
	go coordinator.watchLeader(process.group.LeaderExited())
	coordinator.ready.Wait()
	return coordinator
}

func (c *processEventCoordinator) first(process *activeProcess) processEvent {
	<-c.wake
	process.waitForEventCut()
	event := c.closeAdjudicationEpoch()
	close(c.snapshot)
	c.done.Wait()
	return event
}

func (c *processEventCoordinator) closeAdjudicationEpoch() processEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cut = true
	return c.eventLocked()
}

func (c *processEventCoordinator) watchTimeout(source <-chan time.Time) {
	defer c.done.Done()
	c.ready.Done()
	select {
	case <-source:
		c.admit(processTimeoutEvent, nil)
		<-c.snapshot
	case <-c.snapshot:
	}
}

func (c *processEventCoordinator) watchOverflow(source <-chan processOutputStream, kind processEventKind) {
	defer c.done.Done()
	c.ready.Done()
	select {
	case <-source:
		c.admit(kind, nil)
		<-c.snapshot
	case <-c.snapshot:
	}
}

func (c *processEventCoordinator) watchLeader(source <-chan error) {
	defer c.done.Done()
	c.ready.Done()
	select {
	case err := <-source:
		c.admit(processLeaderEvent, err)
		<-c.snapshot
	case <-c.snapshot:
	}
}

func (c *processEventCoordinator) admit(kind processEventKind, err error) {
	c.mu.Lock()
	if c.cut {
		c.mu.Unlock()
		return
	}
	switch kind {
	case processTimeoutEvent:
		c.timeout = true
	case processStdoutEvent:
		c.stdout = true
	case processStderrEvent:
		c.stderr = true
	case processLeaderEvent:
		c.leader = true
		c.leaderErr = err
	}
	c.mu.Unlock()
	if c.admitted != nil {
		c.admitted <- kind
	}
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

func (c *processEventCoordinator) eventLocked() processEvent {
	event := processEvent{}
	if c.leader {
		event.observationError = c.leaderErr
	}
	switch {
	case c.timeout:
		event.primary = &processTimeoutError{path: c.path, timeout: c.limits.timeout}
	case c.stdout:
		event.primary = &processOverflowError{path: c.path, stream: processStdout, limit: c.limits.stdoutLimit}
	case c.stderr:
		event.primary = &processOverflowError{path: c.path, stream: processStderr, limit: c.limits.stderrLimit}
	}
	return event
}
