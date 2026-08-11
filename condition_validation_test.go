package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

func TestParseConditionType(t *testing.T) {
	tests := []struct {
		name    string
		want    parsedConditionType
		wantErr bool
	}{
		{"FileContains", parsedConditionType{Base: "FileContains"}, false},
		{"FileContainsNot", parsedConditionType{Base: "FileContains", Negated: true}, false},
		{"FileContainsRegex", parsedConditionType{Base: "FileContains", Regex: true}, false},
		{"FileContainsRegexNot", parsedConditionType{Base: "FileContains", Regex: true, Negated: true}, false},
		{"", parsedConditionType{}, true},
		{"A", parsedConditionType{}, true},
		{"Regex", parsedConditionType{}, true},
		{"Not", parsedConditionType{}, true},
		{"FileContainsNotRegex", parsedConditionType{}, true},
		{"FileContainsNotNot", parsedConditionType{}, true},
		{"FileContainsRegexRegex", parsedConditionType{}, true},
		{"FileContainsRegexNotNot", parsedConditionType{}, true},
		{"fileContains", parsedConditionType{}, true},
		{"File-Contains", parsedConditionType{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseConditionType(tt.name)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseConditionType(%q) error = %v, wantErr %v", tt.name, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("parseConditionType(%q) = %+v, want %+v", tt.name, got, tt.want)
			}
		})
	}
}

func TestValidateConfigConditionsRejectsInvalidTypes(t *testing.T) {
	tests := []struct {
		name     string
		goos     string
		typeName string
		list     string
	}{
		{"empty", "linux", "", "Pass"},
		{"short", "linux", "A", "Pass"},
		{"unknown", "linux", "UnknownCondition", "Pass"},
		{"negated_unknown", "linux", "UnknownConditionNot", "Fail"},
		{"malformed_suffix", "linux", "FileContainsNotRegex", "PassOverride"},
		{"unsupported_suffix", "linux", "PathExistsRegex", "Pass"},
		{"platform_inapplicable", "linux", "RegistryKey", "Pass"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			condition := cond{Type: tt.typeName}
			configuredCheck := check{}
			switch tt.list {
			case "Pass":
				configuredCheck.Pass = []cond{condition}
			case "Fail":
				configuredCheck.Fail = []cond{condition}
			case "PassOverride":
				configuredCheck.PassOverride = []cond{condition}
			default:
				t.Fatalf("unhandled list %q", tt.list)
			}

			err := validateConfigConditions(&config{Check: []check{configuredCheck}}, tt.goos)
			if err == nil {
				t.Fatal("validateConfigConditions() error = nil")
			}
			var validationErr *conditionValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("error %T is not *conditionValidationError", err)
			}
			if validationErr.Type != tt.typeName || validationErr.GOOS != tt.goos {
				t.Errorf("validation error = %+v, want type %q and GOOS %q", validationErr, tt.typeName, tt.goos)
			}
			if !strings.Contains(err.Error(), tt.goos) || !strings.Contains(err.Error(), strconv.Quote(tt.typeName)) {
				t.Errorf("error %q does not include type %q and GOOS %q", err, tt.typeName, tt.goos)
			}
		})
	}
}

func TestValidateConfigConditionsAcceptsLegacyForms(t *testing.T) {
	legacy := &config{Check: []check{{
		Pass: []cond{
			{Type: "FileContains", Path: "/tmp/example", Value: "literal"},
			{Type: "FileContainsNot", Path: "/tmp/example", Value: "literal"},
			{Type: "FileContainsRegex", Path: "/tmp/example", Value: "^regex$"},
			{Type: "FileContainsRegexNot", Path: "/tmp/example", Value: "^regex$"},
			{Type: "Command", Cmd: "true"},
		},
		Fail:         []cond{{Type: "PathExistsNot", Path: "/tmp/example"}},
		PassOverride: []cond{{Type: "UserExists", User: "example"}},
	}}}
	if err := validateConfigConditions(legacy, "linux"); err != nil {
		t.Fatalf("validateConfigConditions() = %v", err)
	}
}

func TestConditionAvailabilityUsesCompiledPlatformSet(t *testing.T) {
	if !conditionMethodAvailable(runtime.GOOS, "FileContains") {
		t.Fatal("FileContains must be available on the current platform")
	}
	if runtime.GOOS == "linux" && conditionMethodAvailable(runtime.GOOS, "RegistryKey") {
		t.Fatal("RegistryKey must not be available on Linux")
	}
	if !conditionMethodDeclaredForGOOS("freebsd", "SysctlValue") {
		t.Fatal("FreeBSD planned availability must include SysctlValue")
	}
	parsed, err := parseConditionType("ProgramVersionRegex")
	if err != nil {
		t.Fatal(err)
	}
	if err := validateConditionCapability("freebsd", parsed); err != nil {
		t.Fatalf("planned FreeBSD ProgramVersionRegex rejected: %v", err)
	}
}

func TestLegacyExampleConditionTypesRemainAvailable(t *testing.T) {
	tests := []struct {
		path string
		goos string
	}{
		{path: "docs/examples/linux-remote.conf", goos: "linux"},
		{path: "docs/examples/windows.conf", goos: "windows"},
	}
	for _, tt := range tests {
		t.Run(tt.goos, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Clean(tt.path))
			if err != nil {
				t.Fatal(err)
			}
			candidate := &config{}
			if _, err := toml.Decode(string(data), candidate); err != nil {
				t.Fatal(err)
			}
			if err := validateConfigConditions(candidate, tt.goos); err != nil {
				t.Fatalf("legacy %s example rejected: %v", tt.goos, err)
			}
		})
	}
}
