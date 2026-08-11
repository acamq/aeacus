package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
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
			{Type: "FileContains"},
			{Type: "FileContainsNot"},
			{Type: "FileContainsRegex"},
			{Type: "FileContainsRegexNot"},
			{Type: "Command"},
		},
		Fail:         []cond{{Type: "PathExistsNot"}},
		PassOverride: []cond{{Type: "UserExists"}},
	}}}
	if err := validateConfigConditions(legacy, "linux"); err != nil {
		t.Fatalf("validateConfigConditions() = %v", err)
	}
}

func TestParseConfigValidationFailurePreservesState(t *testing.T) {
	oldConf, oldImage, oldConn, oldCount := conf, image, conn, checkCount
	t.Cleanup(func() { conf, image, conn, checkCount = oldConf, oldImage, oldConn, oldCount })

	conf = &config{Name: "sentinel", Check: []check{{Points: 17, Message: "unchanged"}}}
	image = &imageData{Score: 23, TotalPoints: 42, Points: []scoreItem{{Message: "unchanged"}}}
	conn = &connData{OverallStatus: "unchanged", Status: true}
	checkCount = 9
	wantConf := *conf
	wantImage := *image
	wantConn := *conn

	input := "[[check]]\npoints = 50\n[[check.pass]]\ntype = 'UnknownConditionNot'\n"
	if err := parseConfig(input); err == nil {
		t.Fatal("parseConfig() error = nil")
	}
	if !reflect.DeepEqual(*conf, wantConf) || !reflect.DeepEqual(*image, wantImage) || !reflect.DeepEqual(*conn, wantConn) || checkCount != 9 {
		t.Fatalf("validation failure mutated state: conf=%+v image=%+v conn=%+v checkCount=%d", conf, image, conn, checkCount)
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
