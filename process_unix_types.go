//go:build linux || freebsd

package main

import (
	"bytes"
	"fmt"
	"io"
	"sync"
	"syscall"
	"time"
)

type processProfile uint8

const (
	builtInQuery processProfile = iota
	pkgInventory
	trustedCommand
)

type processLimits struct {
	timeout     time.Duration
	stdoutLimit int
	stderrLimit int
	termGrace   time.Duration
	waitBound   time.Duration
}

func (p processProfile) Limits() processLimits {
	switch p {
	case builtInQuery:
		return processLimits{10 * time.Second, 1 << 20, 1 << 20, 2 * time.Second, 2 * time.Second}
	case pkgInventory:
		return processLimits{30 * time.Second, 16 << 20, 1 << 20, 2 * time.Second, 2 * time.Second}
	case trustedCommand:
		return processLimits{30 * time.Second, 1 << 20, 1 << 20, 2 * time.Second, 2 * time.Second}
	default:
		return processLimits{}
	}
}

type processCleanup uint8

const (
	cleanupNone processCleanup = iota
	cleanupTerminated
	cleanupKilled
)

type processOutputStream uint8

const (
	processStdout processOutputStream = iota
	processStderr
)

func (s processOutputStream) String() string {
	if s == processStderr {
		return "stderr"
	}
	return "stdout"
}

type processWaitFailure uint8

const (
	waitFailed processWaitFailure = iota
	waitBoundExceeded
)

type processResult struct {
	stdout   []byte
	stderr   []byte
	exitCode int
}

type processStartError struct {
	path string
	err  error
}

func (e *processStartError) Error() string { return fmt.Sprintf("start %s: %v", e.path, e.err) }
func (e *processStartError) Unwrap() error { return e.err }

type processExitError struct {
	path string
	code int
}

func (e *processExitError) Error() string {
	return fmt.Sprintf("%s exited with status %d", e.path, e.code)
}

type processSignalError struct {
	path   string
	signal syscall.Signal
}

func (e *processSignalError) Error() string {
	return fmt.Sprintf("%s terminated by signal %s", e.path, e.signal)
}

type processTimeoutError struct {
	path    string
	timeout time.Duration
	cleanup processCleanup
}

func (e *processTimeoutError) Error() string {
	return fmt.Sprintf("%s exceeded timeout %s", e.path, e.timeout)
}

type processOverflowError struct {
	path    string
	stream  processOutputStream
	limit   int
	cleanup processCleanup
}

func (e *processOverflowError) Error() string {
	return fmt.Sprintf("%s exceeded %s limit %d", e.path, e.stream, e.limit)
}

type processWaitError struct {
	path    string
	kind    processWaitFailure
	bound   time.Duration
	err     error
	primary error
}

func (e *processWaitError) Error() string {
	if e.kind == waitBoundExceeded {
		return fmt.Sprintf("%s was not reaped within %s", e.path, e.bound)
	}
	return fmt.Sprintf("wait for %s: %v", e.path, e.err)
}

func (e *processWaitError) Unwrap() error {
	if e.primary != nil {
		return e.primary
	}
	return e.err
}

type processTimer interface {
	Chan() <-chan time.Time
	Stop() bool
}

type processClock interface {
	NewTimer(time.Duration) processTimer
}

type processInvocation struct {
	path      string
	args      []string
	stdout    io.Writer
	stderr    io.Writer
	waitDelay time.Duration
}

type runningProcess interface {
	Wait() <-chan error
	SignalGroup(syscall.Signal) error
	KillDirect() error
	GroupAlive() (bool, error)
}

type processStarter interface {
	Start(processInvocation) (runningProcess, error)
}

type cappedProcessOutput struct {
	mu       sync.Mutex
	buffer   bytes.Buffer
	limit    int
	stream   processOutputStream
	overflow chan processOutputStream
	exceeded bool
}

func newCappedProcessOutput(limit int, stream processOutputStream) *cappedProcessOutput {
	return &cappedProcessOutput{limit: limit, stream: stream, overflow: make(chan processOutputStream, 1)}
}

func (o *cappedProcessOutput) Write(data []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	remaining := o.limit - o.buffer.Len()
	if remaining > len(data) {
		remaining = len(data)
	}
	if remaining > 0 {
		o.buffer.Write(data[:remaining])
	}
	if remaining < len(data) && !o.exceeded {
		o.exceeded = true
		o.overflow <- o.stream
	}
	return len(data), nil
}

func (o *cappedProcessOutput) Bytes() []byte {
	o.mu.Lock()
	defer o.mu.Unlock()
	return bytes.Clone(o.buffer.Bytes())
}

func (o *cappedProcessOutput) Exceeded() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.exceeded
}
