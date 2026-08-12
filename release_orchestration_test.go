//go:build !phocus

package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

var errInjectedReleaseFailure = errors.New("injected release failure")

func TestRunReleasePipelineStopsAtFirstStageFailure(t *testing.T) {
	stageNames := requiredReleaseStageNames()
	for failureIndex, failureStage := range stageNames {
		t.Run(string(failureStage), func(t *testing.T) {
			var trace []releaseStage
			cleanupCalls := 0
			stages := tracedReleaseStages(&trace, &cleanupCalls, true)
			stages.steps[failureIndex].run = func() error {
				trace = append(trace, failureStage)
				return errInjectedReleaseFailure
			}

			err := runReleasePipeline(stages)

			var stageErr *releaseStageError
			if !errors.As(err, &stageErr) || stageErr.Stage != failureStage {
				t.Fatalf("error = %v, want %q stage error", err, failureStage)
			}
			if !errors.Is(err, errInjectedReleaseFailure) {
				t.Fatalf("error %v does not retain cause", err)
			}
			if want := stageNames[:failureIndex+1]; !reflect.DeepEqual(trace, want) {
				t.Fatalf("trace = %v, want %v", trace, want)
			}
			if cleanupCalls != 0 {
				t.Fatalf("cleanup calls = %d, want 0", cleanupCalls)
			}
		})
	}
}

func TestRunReleasePipelineConfirmationOutcomes(t *testing.T) {
	tests := []struct {
		name            string
		initial         bool
		cleanup         bool
		confirmationErr error
		wantErrStage    releaseStage
		wantCleanup     int
		wantStages      int
	}{
		{name: "initial decline returns nonzero error", initial: false, cleanup: true, wantErrStage: releaseStageInitialConfirmation, wantStages: 3},
		{name: "initial input error stops", initial: true, cleanup: true, confirmationErr: errInjectedReleaseFailure, wantErrStage: releaseStageInitialConfirmation, wantStages: 3},
		{name: "cleanup decline preserves files", initial: true, cleanup: false, wantStages: 9},
		{name: "cleanup input error stops", initial: true, cleanup: true, confirmationErr: errInjectedReleaseFailure, wantErrStage: releaseStageCleanupConfirmation, wantStages: 9},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			protected := filepath.Join(t.TempDir(), "protected")
			if err := os.WriteFile(protected, []byte("keep"), 0o600); err != nil {
				t.Fatal(err)
			}
			var trace []releaseStage
			cleanupCalls := 0
			stages := tracedReleaseStages(&trace, &cleanupCalls, tt.cleanup)
			stages.steps[2].run = func() error {
				trace = append(trace, releaseStageInitialConfirmation)
				if tt.wantErrStage == releaseStageInitialConfirmation && tt.confirmationErr != nil {
					return tt.confirmationErr
				}
				if !tt.initial {
					return errReleaseDeclined
				}
				return nil
			}
			if tt.wantErrStage == releaseStageCleanupConfirmation {
				stages.confirmCleanup = func() (bool, error) {
					trace = append(trace, releaseStageCleanupConfirmation)
					return false, tt.confirmationErr
				}
			}
			stages.cleanup = func() error {
				trace = append(trace, releaseStageCleanup)
				cleanupCalls++
				return os.Remove(protected)
			}

			err := runReleasePipeline(stages)

			if tt.wantErrStage == "" {
				if err != nil {
					t.Fatalf("error = %v", err)
				}
			} else {
				var stageErr *releaseStageError
				if !errors.As(err, &stageErr) || stageErr.Stage != tt.wantErrStage {
					t.Fatalf("error = %v, want %q stage", err, tt.wantErrStage)
				}
			}
			if cleanupCalls != tt.wantCleanup {
				t.Fatalf("cleanup calls = %d, want %d", cleanupCalls, tt.wantCleanup)
			}
			if _, statErr := os.Stat(protected); statErr != nil {
				t.Fatalf("protected file changed: %v", statErr)
			}
			if got := countRequiredStages(trace); got != tt.wantStages {
				t.Fatalf("required stage count = %d, want %d; trace=%v", got, tt.wantStages, trace)
			}
		})
	}
}

func TestRunReleasePipelineCleanupFailure(t *testing.T) {
	var trace []releaseStage
	cleanupCalls := 0
	stages := tracedReleaseStages(&trace, &cleanupCalls, true)
	stages.cleanup = func() error {
		trace = append(trace, releaseStageCleanup)
		cleanupCalls++
		return errInjectedReleaseFailure
	}

	err := runReleasePipeline(stages)

	var stageErr *releaseStageError
	if !errors.As(err, &stageErr) || stageErr.Stage != releaseStageCleanup {
		t.Fatalf("error = %v, want cleanup stage", err)
	}
	if !errors.Is(err, errInjectedReleaseFailure) || cleanupCalls != 1 {
		t.Fatalf("error = %v, cleanup calls = %d", err, cleanupCalls)
	}
}

func TestRunReleasePipelineAcceptedCleanupRemovesProtectedFixture(t *testing.T) {
	protected := filepath.Join(t.TempDir(), "protected")
	if err := os.WriteFile(protected, []byte("delete"), 0o600); err != nil {
		t.Fatal(err)
	}
	var trace []releaseStage
	cleanupCalls := 0
	stages := tracedReleaseStages(&trace, &cleanupCalls, true)
	stages.cleanup = func() error {
		trace = append(trace, releaseStageCleanup)
		cleanupCalls++
		return os.Remove(protected)
	}

	err := runReleasePipeline(stages)

	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if cleanupCalls != 1 {
		t.Fatalf("cleanup calls = %d, want 1", cleanupCalls)
	}
	if _, statErr := os.Stat(protected); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("protected fixture stat error = %v, want not exist", statErr)
	}
}

func TestRunReleasePipelineSuccessfulOrderAndRepeatedInvocation(t *testing.T) {
	var trace []releaseStage
	cleanupCalls := 0
	stages := tracedReleaseStages(&trace, &cleanupCalls, true)
	want := append(requiredReleaseStageNames(), releaseStageCleanupConfirmation, releaseStageCleanup)

	for invocation := 1; invocation <= 2; invocation++ {
		trace = nil
		if err := runReleasePipeline(stages); err != nil {
			t.Fatalf("invocation %d: %v", invocation, err)
		}
		if !reflect.DeepEqual(trace, want) {
			t.Fatalf("invocation %d trace = %v, want %v", invocation, trace, want)
		}
	}
	if cleanupCalls != 2 {
		t.Fatalf("cleanup calls = %d, want 2", cleanupCalls)
	}
}

func TestRunReleasePipelineFailedInvocationDoesNotLeakState(t *testing.T) {
	var failedTrace []releaseStage
	failedCleanupCalls := 0
	failedStages := tracedReleaseStages(&failedTrace, &failedCleanupCalls, true)
	failedStages.steps[7].run = func() error {
		failedTrace = append(failedTrace, releaseStageFont)
		return errInjectedReleaseFailure
	}
	if err := runReleasePipeline(failedStages); !errors.Is(err, errInjectedReleaseFailure) {
		t.Fatalf("failed invocation error = %v", err)
	}

	var successfulTrace []releaseStage
	successfulCleanupCalls := 0
	if err := runReleasePipeline(tracedReleaseStages(&successfulTrace, &successfulCleanupCalls, true)); err != nil {
		t.Fatalf("successful invocation error = %v", err)
	}

	want := append(requiredReleaseStageNames(), releaseStageCleanupConfirmation, releaseStageCleanup)
	if !reflect.DeepEqual(successfulTrace, want) {
		t.Fatalf("successful trace = %v, want %v", successfulTrace, want)
	}
	if failedCleanupCalls != 0 || successfulCleanupCalls != 1 {
		t.Fatalf("cleanup calls after failed/successful runs = %d/%d, want 0/1", failedCleanupCalls, successfulCleanupCalls)
	}
}

func TestReadReleaseConfirmation(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		automatic bool
		want      bool
		wantErr   error
	}{
		{name: "yes", input: "y\n", want: true},
		{name: "uppercase yes", input: "YES\n", want: true},
		{name: "empty defaults yes", input: "\n", want: true},
		{name: "no", input: "n\n", want: false},
		{name: "uppercase no", input: "NO\n", want: false},
		{name: "yes without newline", input: "yes", want: true},
		{name: "automatic yes", automatic: true, want: true},
		{name: "malformed", input: "later\n", wantErr: errInvalidReleaseConfirmation},
		{name: "EOF", wantErr: io.EOF},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer

			got, err := readReleaseConfirmation(bytes.NewBufferString(tt.input), &output, "prompt", tt.automatic)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("result = %v, error = %v, want %v, nil", got, err, tt.want)
			}
		})
	}
}

func TestNewReleaseCommandReturnsPipelineError(t *testing.T) {
	command := newReleaseCommand(func() error { return errInjectedReleaseFailure })

	err := command.Action(nil)

	if !errors.Is(err, errInjectedReleaseFailure) {
		t.Fatalf("error = %v, want pipeline cause", err)
	}
}

func TestWriteConfigReturnsPersistenceError(t *testing.T) {
	oldDir := dirPath
	oldConf := conf
	t.Cleanup(func() {
		dirPath = oldDir
		conf = oldConf
	})
	dirPath = filepath.Join(t.TempDir(), "missing") + string(os.PathSeparator)
	conf = &config{Version: version}

	err := writeConfig()

	if err == nil || !strings.Contains(err.Error(), "persist encrypted scoring configuration") {
		t.Fatalf("error = %v, want persistence stage error", err)
	}
}

func TestGenReadMeReturnsInputAndOutputErrors(t *testing.T) {
	oldDir := dirPath
	oldConf := conf
	t.Cleanup(func() {
		dirPath = oldDir
		conf = oldConf
	})
	conf = &config{}

	t.Run("missing input", func(t *testing.T) {
		tempDir := t.TempDir()
		dirPath = tempDir + string(os.PathSeparator)
		withWorkingDirectory(t, tempDir)

		err := genReadMe()

		if err == nil || !strings.Contains(err.Error(), "read README configuration") {
			t.Fatalf("error = %v, want README input error", err)
		}
	})

	t.Run("unwritable output path", func(t *testing.T) {
		tempDir := t.TempDir()
		if err := os.WriteFile(filepath.Join(tempDir, "ReadMe.conf"), []byte("fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
		dirPath = filepath.Join(tempDir, "missing") + string(os.PathSeparator)
		withWorkingDirectory(t, tempDir)

		err := genReadMe()

		if err == nil || !strings.Contains(err.Error(), "write generated README") {
			t.Fatalf("error = %v, want README output error", err)
		}
	})
}

func withWorkingDirectory(t *testing.T, path string) {
	t.Helper()
	oldWorkingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWorkingDirectory); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})
}

func requiredReleaseStageNames() []releaseStage {
	return []releaseStage{
		releaseStageConfig,
		releaseStagePermissions,
		releaseStageInitialConfirmation,
		releaseStageEncryption,
		releaseStageReadMe,
		releaseStageDesktop,
		releaseStageAutologin,
		releaseStageFont,
		releaseStageService,
	}
}

func tracedReleaseStages(trace *[]releaseStage, cleanupCalls *int, cleanupConfirmed bool) releaseStages {
	steps := make([]releaseStep, 0, len(requiredReleaseStageNames()))
	for _, name := range requiredReleaseStageNames() {
		stage := name
		steps = append(steps, releaseStep{stage: stage, run: func() error {
			*trace = append(*trace, stage)
			return nil
		}})
	}
	return releaseStages{
		steps: steps,
		confirmCleanup: func() (bool, error) {
			*trace = append(*trace, releaseStageCleanupConfirmation)
			return cleanupConfirmed, nil
		},
		cleanup: func() error {
			*trace = append(*trace, releaseStageCleanup)
			*cleanupCalls++
			return nil
		},
	}
}

func countRequiredStages(trace []releaseStage) int {
	required := 0
	for _, stage := range trace {
		if stage != releaseStageCleanupConfirmation && stage != releaseStageCleanup {
			required++
		}
	}
	return required
}
