package main

import (
	"errors"
	"runtime"
	"strings"
	"testing"
)

func TestValidateConfigConditionsRejectsInvalidFields(t *testing.T) {
	tests := []struct {
		name      string
		condition cond
		list      string
		field     string
	}{
		{name: "missing required", condition: cond{Type: "PathExists"}, list: "Pass", field: "Path"},
		{name: "unused known", condition: cond{Type: "PathExists", Path: "/tmp/example", Cmd: "true"}, list: "Fail", field: "Cmd"},
		{name: "missing regex operand", condition: cond{Type: "FileContainsRegex", Path: "/tmp/example"}, list: "PassOverride", field: "Value"},
		{name: "unused zero-field operand", condition: cond{Type: "AutoCheckUpdatesEnabled", Name: "unexpected"}, list: "Pass", field: "Name"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: one condition in the selected scoring list.
			configuredCheck := check{}
			switch tt.list {
			case "Pass":
				configuredCheck.Pass = []cond{tt.condition}
			case "Fail":
				configuredCheck.Fail = []cond{tt.condition}
			case "PassOverride":
				configuredCheck.PassOverride = []cond{tt.condition}
			default:
				t.Fatalf("unhandled list %q", tt.list)
			}

			// When: semantic condition fields are validated.
			err := validateConfigConditions(&config{Check: []check{configuredCheck}}, "linux")

			// Then: the original type, platform, and offending field are reported.
			if err == nil {
				t.Fatal("validateConfigConditions() error = nil")
			}
			var validationErr *conditionValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("error %T is not *conditionValidationError", err)
			}
			for _, expected := range []string{tt.condition.Type, "linux", tt.field} {
				if !strings.Contains(err.Error(), expected) {
					t.Errorf("error %q does not contain %q", err, expected)
				}
			}
		})
	}
}

func TestValidateConfigConditionsAcceptsOptionalFields(t *testing.T) {
	// Given: globally optional Hint and FreeBSD's optional firewall Name.
	linuxConfig := &config{Check: []check{{Pass: []cond{{Type: "PathExists", Path: "/tmp/example", Hint: "operator context"}}}}}
	freeBSDConfig := &config{Check: []check{{Pass: []cond{{Type: "FirewallUp", Name: "pf"}}}}}

	// When/Then: both optional-field forms validate for their target GOOS.
	if err := validateConfigConditions(linuxConfig, "linux"); err != nil {
		t.Fatalf("Linux optional Hint rejected: %v", err)
	}
	if err := validateConfigConditions(freeBSDConfig, "freebsd"); err != nil {
		t.Fatalf("FreeBSD optional Name rejected: %v", err)
	}
}

func TestParseConfigRejectsUndecodedConditionField(t *testing.T) {
	oldConf := conf
	t.Cleanup(func() { conf = oldConf })
	conf = &config{Name: "unchanged"}
	input := "[[check]]\n[[check.pass]]\ntype='PathExists'\npath='/tmp/example'\nextra='rejected'\n"

	// When: an unknown key appears inside a condition table.
	err := parseConfig(input)

	// Then: parsing fails with type, GOOS, and key before publishing the candidate.
	if err == nil {
		t.Fatal("parseConfig() error = nil")
	}
	for _, expected := range []string{"PathExists", runtime.GOOS, "extra"} {
		if !strings.Contains(err.Error(), expected) {
			t.Errorf("error %q does not contain %q", err, expected)
		}
	}
	if conf.Name != "unchanged" {
		t.Fatalf("parseConfig() published invalid config: %+v", conf)
	}
}

func TestParseConfigRejectsWrongConditionFieldType(t *testing.T) {
	oldConf := conf
	t.Cleanup(func() { conf = oldConf })
	conf = &config{Name: "unchanged"}
	input := "[[check]]\n[[check.pass]]\ntype='PathExists'\npath=7\n"

	// When: a known condition field has a non-string TOML value.
	err := parseConfig(input)

	// Then: parsing fails with the original condition type, GOOS, and field.
	if err == nil {
		t.Fatal("parseConfig() error = nil")
	}
	for _, expected := range []string{"PathExists", runtime.GOOS, "Path"} {
		if !strings.Contains(err.Error(), expected) {
			t.Errorf("error %q does not contain %q", err, expected)
		}
	}
	if conf.Name != "unchanged" {
		t.Fatalf("parseConfig() published invalid config: %+v", conf)
	}
}
