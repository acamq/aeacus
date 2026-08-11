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
	shellCalls := 0

	// When: the Phocus startup boundary validates through the production parser.
	err := startPhocus(
		func() error {
			return parseConfigWithCapabilities("version='2.1.1'\nplatform='freebsd'\nshell=true\nos='display only'\n", "freebsd", resolveFreeBSDCapabilitiesForTest)
		},
		func(shellLauncher func()) {
			loopCalls++
			shellLauncher()
		},
		func() { shellCalls++ },
	)

	// Then: the typed error returns before publication, loop entry, or shell launch.
	var capabilityErr *CapabilityError
	if !errors.As(err, &capabilityErr) {
		t.Fatalf("startup error = %v, want *CapabilityError", err)
	}
	if capabilityErr.Field != "shell" || capabilityErr.Value != "true" || capabilityErr.GOOS != "freebsd" {
		t.Fatalf("startup error = %+v", capabilityErr)
	}
	if conf != wantConfPointer {
		t.Fatalf("config pointer changed: got %p want %p", conf, wantConfPointer)
	}
	if loopCalls != 0 || shellCalls != 0 {
		t.Fatalf("side-effect calls: loop=%d shell=%d, want zero", loopCalls, shellCalls)
	}
}

func TestPhocusStartupRunsLoopAfterValidFreeBSDConfig(t *testing.T) {
	oldConf := conf
	t.Cleanup(func() { conf = oldConf })
	loopCalls := 0
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
		},
		func() { shellCalls++ },
	)

	// Then: validation succeeds before exactly one loop entry and no shell launch.
	if err != nil {
		t.Fatalf("startPhocus() error = %v", err)
	}
	if loopCalls != 1 || shellCalls != 0 {
		t.Fatalf("side-effect calls: loop=%d shell=%d, want loop=1 shell=0", loopCalls, shellCalls)
	}
}

func resolveFreeBSDCapabilitiesForTest(candidate config) (config, error) {
	return resolveCapabilities(candidate, "freebsd")
}
