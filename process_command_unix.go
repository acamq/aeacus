//go:build linux || freebsd

package main

// shellCommand is the trusted administrator-configured exception. Built-in
// checks use fixed executable paths and argv through execRunner instead.
func shellCommand(commandGiven string) error {
	_, err := unixRunner.RunTrusted("/bin/sh", []string{"-c", commandGiven})
	if err != nil && verboseEnabled {
		printShellCommandError(commandGiven, err)
	}
	return err
}

func shellCommandOutput(commandGiven string) (string, error) {
	result, err := unixRunner.RunTrusted("/bin/sh", []string{"-c", commandGiven})
	debugShellCommand(commandGiven, result.stdout, err)
	if err != nil {
		if verboseEnabled {
			printShellCommandError(commandGiven, err)
		}
		return "", err
	}
	return string(result.stdout), nil
}

func (c cond) Command() (bool, error) {
	c.requireArgs("Cmd")
	return shellCommand(c.Cmd) == nil, nil
}
