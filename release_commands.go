package main

import "fmt"

func runReleaseCommands(commands ...string) error {
	for commandIndex, command := range commands {
		if err := shellCommand(command); err != nil {
			return fmt.Errorf("release command %d: %w", commandIndex+1, err)
		}
	}
	return nil
}
