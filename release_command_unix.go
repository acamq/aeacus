//go:build linux || freebsd

package main

func runPlatformReleaseCommand(command string) error {
	return shellCommand(command)
}
