//go:build linux || freebsd

package main

import "time"

type processLimits struct {
	timeout     time.Duration
	stdoutLimit int
	stderrLimit int
	termGrace   time.Duration
	waitBound   time.Duration
}

const (
	builtInQueryTimeout       = 10 * time.Second
	pkgInventoryTimeout       = 30 * time.Second
	trustedCommandTimeout     = 30 * time.Second
	builtInQueryStdoutLimit   = 1 << 20
	pkgInventoryStdoutLimit   = 16 << 20
	trustedCommandStdoutLimit = 1 << 20
	unixStderrLimit           = 1 << 20
	unixTermGrace             = 2 * time.Second
	unixWaitBound             = 2 * time.Second
)

func builtInQueryLimits() processLimits {
	return processLimits{builtInQueryTimeout, builtInQueryStdoutLimit, unixStderrLimit, unixTermGrace, unixWaitBound}
}

func pkgInventoryLimits() processLimits {
	return processLimits{pkgInventoryTimeout, pkgInventoryStdoutLimit, unixStderrLimit, unixTermGrace, unixWaitBound}
}

func trustedCommandLimits() processLimits {
	return processLimits{trustedCommandTimeout, trustedCommandStdoutLimit, unixStderrLimit, unixTermGrace, unixWaitBound}
}
