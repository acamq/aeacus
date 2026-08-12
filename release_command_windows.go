//go:build windows

package main

import "os/exec"

func runPlatformReleaseCommand(command string) error {
	err := releasePowerShellCommand(command).Run()
	if err != nil && verboseEnabled {
		printShellCommandError(command, err)
	}
	return err
}

func releasePowerShellCommand(command string) *exec.Cmd {
	return rawCmd(windowsReleaseScript(command))
}

// windowsReleaseScript accepts only trusted release commands containing at
// most one native executable so its status is captured at that boundary.
func windowsReleaseScript(command string) string {
	return `trap { exit 1 }
$ErrorActionPreference = 'Stop'
$global:LASTEXITCODE = 0
& { ` + command + ` }
$nativeExitCode = $LASTEXITCODE
if ($nativeExitCode -ne 0) { exit $nativeExitCode }`
}
