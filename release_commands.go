package main

import "fmt"

var releaseCommandRunner = runPlatformReleaseCommand

func runReleaseCommands(commands ...string) error {
	for commandIndex, command := range commands {
		if err := releaseCommandRunner(command); err != nil {
			return fmt.Errorf("release command %d: %w", commandIndex+1, err)
		}
	}
	return nil
}
