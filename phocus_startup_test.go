package main

import (
	"errors"
	"testing"
)

func TestPhocusStartupRejectsInvalidFreeBSDShellBeforeLoopAndShell(t *testing.T) {
	oldConf := conf
	t.Cleanup(func() { conf = oldConf })

	// Given: published state and a FreeBSD config forbidden from enabling shell.
	conf = &config{Name: "sentinel"}
	wantConfPointer := conf
	loopCalls := 0
	scoreCalls := 0
	shellCalls := 0

	// When: the Phocus startup boundary validates through the production parser.
	err := startPhocus(
		func() error {
			return parseConfigWithCapabilities("version='2.1.1'\nplatform='freebsd'\nshell=true\nos='display only'\n", "freebsd", resolveFreeBSDCapabilitiesForTest)
		},
		func(shellLauncher func()) {
			loopCalls++
			scoreCalls++
			shellLauncher()
		},
		func() { shellCalls++ },
	)

	// Then: the typed error returns before publication, loop entry, scoring, or shell launch.
	var capabilityErr *CapabilityError
	if !errors.As(err, &capabilityErr) {
		t.Errorf("startup error = %v, want *CapabilityError", err)
	}
	if capabilityErr.Field != "shell" || capabilityErr.Value != "true" || capabilityErr.GOOS != "freebsd" {
		t.Errorf("startup error = %+v", capabilityErr)
	}
	if conf != wantConfPointer {
		t.Errorf("config pointer changed: got %p want %p", conf, wantConfPointer)
	}
	if loopCalls != 0 || scoreCalls != 0 || shellCalls != 0 {
		t.Fatalf("side-effect calls: loop=%d score=%d shell=%d, want loop=0 score=0 shell=0", loopCalls, scoreCalls, shellCalls)
	}
}

func TestPhocusStartupRunsLoopAfterValidFreeBSDConfig(t *testing.T) {
	oldConf := conf
	t.Cleanup(func() { conf = oldConf })
	loopCalls := 0
	scoreCalls := 0
	shellCalls := 0

	// Given: a valid legacy-compatible FreeBSD config.
	loadConfig := func() error {
		return parseConfigWithCapabilities("version='2.1.1'\nplatform='freebsd'\nshell=false\nos='arbitrary display metadata'\n", "freebsd", resolveFreeBSDCapabilitiesForTest)
	}

	// When: the Phocus startup boundary loads and validates it.
	err := startPhocus(
		loadConfig,
		func(shellLauncher func()) {
			loopCalls++
			if conf.Platform != "freebsd" || conf.Desktop != "headless" || conf.Autologin != "none" {
				t.Fatalf("loop observed unvalidated config: %+v", conf)
			}
			scoreCalls++
		},
		func() { shellCalls++ },
	)

	// Then: validation succeeds before one loop and score invocation with no shell launch.
	if err != nil {
		t.Fatalf("startPhocus() error = %v", err)
	}
	if loopCalls != 1 || scoreCalls != 1 || shellCalls != 0 {
		t.Fatalf("side-effect calls: loop=%d score=%d shell=%d, want loop=1 score=1 shell=0", loopCalls, scoreCalls, shellCalls)
	}
}

func resolveFreeBSDCapabilitiesForTest(candidate config) (config, error) {
	return resolveCapabilities(candidate, "freebsd")
}
