package main

import (
	"errors"
	"reflect"
	"testing"
)

var errInjectedReleaseCommand = errors.New("injected release command failure")

func TestRunReleaseCommandsDispatchesThroughReleaseRunnerAndStops(t *testing.T) {
	oldRunner := releaseCommandRunner
	t.Cleanup(func() { releaseCommandRunner = oldRunner })
	var commands []string
	releaseCommandRunner = func(command string) error {
		commands = append(commands, command)
		if command == "second" {
			return errInjectedReleaseCommand
		}
		return nil
	}

	err := runReleaseCommands("first", "second", "third")

	if !errors.Is(err, errInjectedReleaseCommand) {
		t.Fatalf("error = %v, want injected cause", err)
	}
	if want := []string{"first", "second"}; !reflect.DeepEqual(commands, want) {
		t.Fatalf("commands = %v, want %v", commands, want)
	}
}
