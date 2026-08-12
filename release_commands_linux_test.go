//go:build linux && !phocus

package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRunReleaseCommandsStopsAfterCommandFailure(t *testing.T) {
	sentinel := filepath.Join(t.TempDir(), "later-stage")

	err := runReleaseCommands("exit 23", "touch "+sentinel)

	if err == nil {
		t.Fatal("runReleaseCommands() error = nil")
	}
	if _, statErr := os.Stat(sentinel); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("later command ran: %v", statErr)
	}
}
