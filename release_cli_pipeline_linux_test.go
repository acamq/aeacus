//go:build linux && !phocus

package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"testing"
)

const releaseCLIStageFailureEnvironment = "AEACUS_RELEASE_STAGE_FAILURE"

func TestReleaseCLIStageFailureHelperProcess(t *testing.T) {
	if os.Getenv(releaseCLIStageFailureEnvironment) != "1" {
		return
	}
	command := newReleaseCommand(func() error {
		return &releaseStageError{Stage: releaseStageService, Err: errInjectedReleaseFailure}
	})
	if err := command.Action(nil); err != nil {
		fail(err.Error())
		os.Exit(1)
	}
}

func TestReleaseCLIStageFailureExitsNonzero(t *testing.T) {
	command := exec.Command(os.Args[0], "-test.run=^TestReleaseCLIStageFailureHelperProcess$")
	command.Env = append(os.Environ(), releaseCLIStageFailureEnvironment+"=1")

	output, err := command.CombinedOutput()

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() == 0 {
		t.Fatalf("exit error = %v, output = %s", err, output)
	}
	if !bytes.Contains(output, []byte(string(releaseStageService))) || !bytes.Contains(output, []byte(errInjectedReleaseFailure.Error())) {
		t.Fatalf("output does not identify service failure: %s", output)
	}
}
