package main

import (
	"slices"
	"testing"
)

type normativeFieldSpec struct {
	required []string
	optional []string
}

func TestFreeBSDFieldSpecsMatchNormativeTable(t *testing.T) {
	// Given: an independent transcription of all 32 certified Task 4 field rows.
	expected := map[string]normativeFieldSpec{
		"AccountLocked":                  {required: []string{"User"}},
		"BasePackageInstalled":           {required: []string{"Name"}},
		"BaseUpdateChecksEnabled":        {},
		"Command":                        {required: []string{"Cmd"}},
		"CommandContains":                {required: []string{"Cmd", "Value"}},
		"CommandOutput":                  {required: []string{"Cmd", "Value"}},
		"DirContains":                    {required: []string{"Path", "Value"}},
		"FileACL":                        {required: []string{"Path", "Value"}},
		"FileContains":                   {required: []string{"Path", "Value"}},
		"FileEquals":                     {required: []string{"Path", "Value"}},
		"FileFlagSet":                    {required: []string{"Name", "Path"}},
		"FileOwner":                      {required: []string{"Group", "Path", "User"}},
		"FirewallAvailable":              {optional: []string{"Name"}},
		"FirewallEnabled":                {optional: []string{"Name"}},
		"FirewallRulesLoaded":            {optional: []string{"Name"}},
		"FirewallUp":                     {optional: []string{"Name"}},
		"JailRunning":                    {required: []string{"Name"}},
		"KernelVersion":                  {required: []string{"Value"}},
		"PackageUpdateChecksEnabled":     {},
		"PasswordAuthenticationDisabled": {required: []string{"User"}},
		"PasswordChanged":                {required: []string{"User", "Value"}},
		"PathExists":                     {required: []string{"Path"}},
		"PermissionIs":                   {required: []string{"Path", "Value"}},
		"PkgAuditClean":                  {},
		"ProgramInstalled":               {required: []string{"Name"}},
		"ProgramVersion":                 {required: []string{"Name", "Value"}},
		"RunningInJail":                  {},
		"ServiceEnabled":                 {required: []string{"Name"}},
		"ServiceUp":                      {required: []string{"Name"}},
		"SysctlValue":                    {required: []string{"Name", "Value"}},
		"UserExists":                     {required: []string{"User"}},
		"UserInGroup":                    {required: []string{"Group", "User"}},
	}

	// When/Then: production and independent tables match bidirectionally with sorted exact fields.
	if len(expected) != 32 || len(freeBSDFieldSpecs) != len(expected) {
		t.Fatalf("method counts: expected=%d production=%d", len(expected), len(freeBSDFieldSpecs))
	}
	for method, want := range expected {
		actual, exists := freeBSDFieldSpecs[method]
		if !exists {
			t.Errorf("production field specs omit %q", method)
			continue
		}
		if !slices.IsSorted(actual.required) || !slices.IsSorted(actual.optional) {
			t.Errorf("production fields for %q are not sorted: required=%v optional=%v", method, actual.required, actual.optional)
		}
		if !slices.Equal(actual.required, want.required) || !slices.Equal(actual.optional, want.optional) {
			t.Errorf("%s fields: required=%v optional=%v, want required=%v optional=%v", method, actual.required, actual.optional, want.required, want.optional)
		}
	}
	for method := range freeBSDFieldSpecs {
		if _, exists := expected[method]; !exists {
			t.Errorf("production field specs contain unexpected method %q", method)
		}
	}
}
