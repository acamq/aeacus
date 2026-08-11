package main

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestGeneratedAvailabilityIsCurrent(t *testing.T) {
	root, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	availability, err := discoverAvailability(root)
	if err != nil {
		t.Fatal(err)
	}

	contract, err := os.ReadFile(filepath.Join(root, "misc/tests/contracts/condition-availability-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if want := renderContract(availability); !bytes.Equal(contract, want) {
		t.Fatal("condition availability contract is stale; run go run ./misc/tests/contracts/generate -root .")
	}

	generated, err := os.ReadFile(filepath.Join(root, "condition_availability_generated.go"))
	if err != nil {
		t.Fatal(err)
	}
	if want := renderGo(availability); !bytes.Equal(generated, want) {
		t.Fatal("generated Go availability is stale; run go run ./misc/tests/contracts/generate -root .")
	}
}

func TestAvailabilityArraysAreSortedAndUnique(t *testing.T) {
	root, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	availability, err := discoverAvailability(root)
	if err != nil {
		t.Fatal(err)
	}
	for goos, methods := range map[string][]string{
		"freebsd":       freeBSDConditionMethods,
		"freebsd-regex": freeBSDRegexConditionMethods,
		"legacy-regex":  legacyRegexConditionMethods,
		"linux":         availability.Linux,
		"windows":       availability.Windows,
	} {
		t.Run(goos, func(t *testing.T) {
			if !slices.IsSorted(methods) {
				t.Fatalf("%s methods are not sorted: %v", goos, methods)
			}
			for i := 1; i < len(methods); i++ {
				if methods[i-1] == methods[i] {
					t.Fatalf("%s method %q is duplicated", goos, methods[i])
				}
			}
		})
	}
}

func TestFreeBSDAdditionsDoNotLeakToLegacyPlatforms(t *testing.T) {
	root, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	availability, err := discoverAvailability(root)
	if err != nil {
		t.Fatal(err)
	}
	additions := []string{
		"AccountLocked", "BasePackageInstalled", "BaseUpdateChecksEnabled",
		"FileACL", "FileFlagSet", "FirewallAvailable", "FirewallEnabled",
		"FirewallRulesLoaded", "JailRunning", "PackageUpdateChecksEnabled",
		"PasswordAuthenticationDisabled", "PkgAuditClean", "RunningInJail",
		"ServiceEnabled", "SysctlValue",
	}
	for _, method := range additions {
		if slices.Contains(availability.Linux, method) || slices.Contains(availability.Windows, method) {
			t.Errorf("FreeBSD-only method %q leaked into a legacy platform", method)
		}
	}
}
