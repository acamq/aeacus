//go:build windows

package main

func shellCommand(commandGiven string) error {
	err := rawCmd(powerShellReleaseScript(commandGiven)).Run()
	if err != nil && verboseEnabled {
		printShellCommandError(commandGiven, err)
	}
	return err
}

func shellCommandOutput(commandGiven string) (string, error) {
	output, err := rawCmd(powerShellReleaseScript(commandGiven)).Output()
	debugShellCommand(commandGiven, output, err)
	if err != nil {
		if verboseEnabled {
			printShellCommandError(commandGiven, err)
		}
		return "", err
	}
	return string(output), nil
}

func powerShellReleaseScript(commandGiven string) string {
	return `trap { exit 1 }
$ErrorActionPreference = 'Stop'
$global:LASTEXITCODE = 0
& { ` + commandGiven + ` }
$nativeExitCode = $LASTEXITCODE
if ($nativeExitCode -ne 0) { exit $nativeExitCode }`
}
