//go:build windows

package main

func shellCommand(commandGiven string) error {
	err := rawCmd(commandGiven).Run()
	if err != nil && verboseEnabled {
		printShellCommandError(commandGiven, err)
	}
	return err
}

func shellCommandOutput(commandGiven string) (string, error) {
	output, err := rawCmd(commandGiven).Output()
	debugShellCommand(commandGiven, output, err)
	if err != nil {
		if verboseEnabled {
			printShellCommandError(commandGiven, err)
		}
		return "", err
	}
	return string(output), nil
}
