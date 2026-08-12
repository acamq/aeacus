//go:build !phocus

package main

import (
	"errors"
	"fmt"
)

type releaseStage string

const (
	releaseStageConfig              releaseStage = "config"
	releaseStagePermissions         releaseStage = "permissions"
	releaseStageInitialConfirmation releaseStage = "initial confirmation"
	releaseStageEncryption          releaseStage = "encryption"
	releaseStageReadMe              releaseStage = "README"
	releaseStageDesktop             releaseStage = "desktop files"
	releaseStageAutologin           releaseStage = "autologin"
	releaseStageFont                releaseStage = "font"
	releaseStageService             releaseStage = "service"
	releaseStageCleanupConfirmation releaseStage = "cleanup confirmation"
	releaseStageCleanup             releaseStage = "cleanup"
)

var errReleaseDeclined = errors.New("release declined")

var errReleaseAdministratorRequired = errors.New("administrator permissions are required")

type releaseStageError struct {
	Stage releaseStage
	Err   error
}

func (e *releaseStageError) Error() string {
	return fmt.Sprintf("release stage %q: %v", e.Stage, e.Err)
}

func (e *releaseStageError) Unwrap() error {
	return e.Err
}

type releaseStep struct {
	stage releaseStage
	run   func() error
}

type releaseStages struct {
	steps          []releaseStep
	confirmCleanup func() (bool, error)
	cleanup        func() error
}

func runReleasePipeline(stages releaseStages) error {
	for _, step := range stages.steps {
		if err := step.run(); err != nil {
			return &releaseStageError{Stage: step.stage, Err: err}
		}
	}

	confirmed, err := stages.confirmCleanup()
	if err != nil {
		return &releaseStageError{Stage: releaseStageCleanupConfirmation, Err: err}
	}
	if !confirmed {
		return nil
	}
	if err := stages.cleanup(); err != nil {
		return &releaseStageError{Stage: releaseStageCleanup, Err: err}
	}
	return nil
}

func checkReleasePermissions() error {
	if !adminCheck() {
		return errReleaseAdministratorRequired
	}
	return nil
}
