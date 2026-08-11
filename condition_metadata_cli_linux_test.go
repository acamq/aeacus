package main

import (
	"bytes"
	"errors"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestCLIValidConditionDoesNotWarnUndecoded(t *testing.T) {
	config := "version='" + version + "'\n[[check]]\nmessage='valid'\npoints=10\n[[check.pass]]\ntype='PathExists'\npath='/tmp/example'\nhint='context'\n"
	root := writeCLIFixture(t, config)

	// When: the production check command decodes a valid condition in verbose mode.
	output, err := runProductionCLI(t, "--verbose", "--dir", root, "check")

	// Then: consumed condition keys do not produce misleading undecoded warnings.
	if err != nil {
		t.Fatalf("check failed: %v\n%s", err, output)
	}
	if bytes.Contains(output, []byte("Undecoded scoring configuration key")) {
		t.Fatalf("valid condition emitted undecoded warning:\n%s", output)
	}
}

func TestCLIUnknownTopLevelKeyStillWarns(t *testing.T) {
	config := "version='" + version + "'\nmystery='value'\n[[check]]\nmessage='valid'\npoints=10\n[[check.pass]]\ntype='PathExists'\npath='/tmp/example'\n"
	root := writeCLIFixture(t, config)

	// When: a genuinely unknown top-level key is decoded in verbose mode.
	output, err := runProductionCLI(t, "--verbose", "--dir", root, "check")

	// Then: the existing warning remains visible.
	if err != nil {
		t.Fatalf("check failed: %v\n%s", err, output)
	}
	want := `Undecoded scoring configuration key "mystery" will not be used.`
	if !strings.Contains(string(output), want) {
		t.Fatalf("output does not contain %q:\n%s", want, output)
	}
	if count := strings.Count(string(output), "Undecoded scoring configuration key"); count != 1 {
		t.Fatalf("undecoded warning count = %d, want 1:\n%s", count, output)
	}
}

func TestCLICaseVariantDiagnosticsAreDeterministic(t *testing.T) {
	tests := []struct {
		name     string
		config   string
		expected []string
	}{
		{
			name:     "path duplicate",
			config:   "[[check]]\n[[check.pass]]\nType='PathExists'\nPath='/tmp/one'\npath='/tmp/two'\n",
			expected: []string{`condition type "PathExists" repeats field "Path"`, "GOOS " + runtime.GOOS},
		},
		{
			name:     "type duplicate",
			config:   "[[check]]\n[[check.pass]]\nType='PathExists'\ntype='FileContains'\nPath='/tmp/one'\n",
			expected: []string{`condition type "PathExists" repeats field "Type"`, "GOOS " + runtime.GOOS},
		},
		{
			name:     "wrong type",
			config:   "[[check]]\n[[check.pass]]\nType='PathExists'\nPATH=7\n",
			expected: []string{`condition type "PathExists" field "Path" must be a string`, "GOOS " + runtime.GOOS},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := writeCLIFixture(t, tt.config)
			var first []byte
			for run := 0; run < 20; run++ {
				output, err := runProductionCLI(t, "--dir", root, "check")
				var exitErr *exec.ExitError
				if !errors.As(err, &exitErr) || exitErr.ExitCode() == 0 {
					t.Fatalf("run %d exit = %v, output:\n%s", run, err, output)
				}
				for _, expected := range tt.expected {
					if !bytes.Contains(output, []byte(expected)) {
						t.Fatalf("run %d output lacks %q:\n%s", run, expected, output)
					}
				}
				if run == 0 {
					first = slices.Clone(output)
				} else if !bytes.Equal(output, first) {
					t.Fatalf("run %d output differs:\nfirst: %q\ncurrent: %q", run, first, output)
				}
			}
		})
	}
}

func TestCLIExactDuplicateIsDeterministicSyntaxError(t *testing.T) {
	config := "[[check]]\n[[check.pass]]\ntype='PathExists'\npath='/tmp/one'\npath='/tmp/two'\n"
	root := writeCLIFixture(t, config)
	var first []byte
	for run := 0; run < 10; run++ {
		// Exact duplicate TOML keys are rejected by the parser before condition decoding, so Type is intentionally unavailable without a bespoke scanner.
		output, err := runProductionCLI(t, "--dir", root, "check")
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() == 0 {
			t.Fatalf("run %d exit = %v, output:\n%s", run, err, output)
		}
		for _, expected := range []string{"GOOS " + runtime.GOOS, "check.pass.path"} {
			if !bytes.Contains(output, []byte(expected)) {
				t.Fatalf("run %d output lacks %q:\n%s", run, expected, output)
			}
		}
		if bytes.Contains(output, []byte(`condition type "PathExists"`)) {
			t.Fatalf("syntax-boundary error unexpectedly claims a decoded condition type:\n%s", output)
		}
		if run == 0 {
			first = slices.Clone(output)
		} else if !bytes.Equal(output, first) {
			t.Fatalf("run %d output differs:\nfirst: %q\ncurrent: %q", run, first, output)
		}
	}
}
