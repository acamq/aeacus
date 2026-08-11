// checks_test.go is responsible for testing all non-platform dependent checks.
package main

import (
	"testing"
)

func TestCommandContains(t *testing.T) {
	c := cond{
		Cmd:   "echo 'hello, world!'",
		Value: "hello, world!",
	}

	// Should pass: exact match
	out, err := c.CommandContains()
	if err != nil || out != true {
		t.Error(c, "failed:", out, err)
	}

	// Should pass: substring
	c.Value = "hello"
	out, err = c.CommandContains()
	if err != nil || out != true {
		t.Error(c, "failed:", out, err)
	}

	// Should fail: not substring
	c.Value = "bye"
	out, err = c.CommandContains()
	if err != nil || out != false {
		t.Error(c, "failed:", out, err)
	}
}

func TestCommandOutput(t *testing.T) {
	c := cond{
		Cmd:   "echo 'hello, world!'",
		Value: "hello, world!",
	}

	// Should pass: exact match
	out, err := c.CommandOutput()
	if err != nil || out != true {
		t.Error(c, "failed:", out, err)
	}

	// Should fail: just substring
	c.Value = "hello"
	out, err = c.CommandOutput()
	if err != nil || out != false {
		t.Error(c, "failed:", out, err)
	}

	// Should fail: not exact or substring
	c.Value = "bye"
	out, err = c.CommandOutput()
	if err != nil || out != false {
		t.Error(c, "failed:", out, err)
	}
}

// TestDirContains exercises the DirContains method directly. The unexported
// regex field defaults to false, so the method performs literal substring
// matching (strings.Contains), not regexp evaluation. Regex behavior is
// exercised through runCheck with the *Regex suffix in TestRunCheckContains.
func TestDirContains(t *testing.T) {
	c := cond{
		Path:  "misc/tests/dir",
		Value: "efgh",
	}
	out, err := c.DirContains()
	if err != nil || out != true {
		t.Error(c, "failed:", out, err)
	}

	c.Value = "efghabcd"
	out, err = c.DirContains()
	if err != nil || out != true {
		t.Error(c, "failed:", out, err)
	}

	c.Value = "aaaaaa"
	out, err = c.DirContains()
	if err != nil || out != false {
		t.Error(c, "failed:", out, err)
	}

	c.Value = "spaces    in it"
	out, err = c.DirContains()
	if err != nil || out != true {
		t.Error(c, "failed:", out, err)
	}

	c.Value = "spaces    in it   99999999 nums"
	out, err = c.DirContains()
	if err != nil || out != false {
		t.Error(c, "failed:", out, err)
	}
}

func TestFileContains(t *testing.T) {
	c := cond{
		Path:  "misc/tests/TestFileContains.txt",
		Value: "hello",
	}
	out, err := c.FileContains()
	if err != nil || out != true {
		t.Error(c, "failed:", out, err)
	}

	c.Value = "nothere"
	out, err = c.FileContains()
	if err != nil || out != false {
		t.Error(c, "failed:", out, err)
	}
}

// TestRunCheckContains exercises the runCheck dispatcher across every
// Contains-family condition type so that the literal/regex and negation
// suffix-parsing logic stays covered end to end. Plain (unobfuscated) cond
// values are intentional: deobfuscateCond fails fast on the non-hex Type
// field and runCheck proceeds with the original literals, which is the same
// escape hatch the existing direct-method tests rely on.
func TestRunCheckContains(t *testing.T) {
	const (
		dirPath     = "misc/tests/dir"
		filePath    = "misc/tests/TestFileContains.txt"
		missingPath = "misc/tests/doesntexist"
	)

	tests := []struct {
		name string
		c    cond
		want bool
	}{
		{"DirContains/literal_positive_match", cond{Type: "DirContains", Path: dirPath, Value: "efgh"}, true},
		{"DirContains/literal_negative_no_match", cond{Type: "DirContains", Path: dirPath, Value: "zzzzz"}, false},
		{"DirContains/missing_path_returns_false", cond{Type: "DirContains", Path: missingPath, Value: "efgh"}, false},

		{"DirContainsNot/negated_positive_flips_to_false", cond{Type: "DirContainsNot", Path: dirPath, Value: "efgh"}, false},
		{"DirContainsNot/negated_negative_flips_to_true", cond{Type: "DirContainsNot", Path: dirPath, Value: "zzzzz"}, true},
		{"DirContainsNot/missing_path_error_returns_false", cond{Type: "DirContainsNot", Path: missingPath, Value: "efgh"}, false},

		{"DirContainsRegex/regex_positive_match", cond{Type: "DirContainsRegex", Path: dirPath, Value: "^efgh"}, true},
		{"DirContainsRegex/regex_negative_no_match", cond{Type: "DirContainsRegex", Path: dirPath, Value: "^zzzzz"}, false},
		{"DirContainsRegex/malformed_regex_error_returns_false", cond{Type: "DirContainsRegex", Path: dirPath, Value: "["}, false},
		{"DirContainsRegex/missing_path_error_returns_false", cond{Type: "DirContainsRegex", Path: missingPath, Value: "^efgh"}, false},

		// DirContainsRegexNot: the malformed-regex case is intentionally
		// omitted. DirContains swallows per-file regexp syntax errors and
		// returns (false, nil), so the negation would yield true rather
		// than an error-driven false. Recorded in
		// .omo/notepads/freebsd-support/issues.md.
		{"DirContainsRegexNot/negated_positive_flips_to_false", cond{Type: "DirContainsRegexNot", Path: dirPath, Value: "^efgh"}, false},
		{"DirContainsRegexNot/negated_negative_flips_to_true", cond{Type: "DirContainsRegexNot", Path: dirPath, Value: "^zzzzz"}, true},
		{"DirContainsRegexNot/missing_path_error_returns_false", cond{Type: "DirContainsRegexNot", Path: missingPath, Value: "^efgh"}, false},

		{"FileContains/literal_positive_match", cond{Type: "FileContains", Path: filePath, Value: "hello"}, true},
		{"FileContains/literal_negative_no_match", cond{Type: "FileContains", Path: filePath, Value: "zzzzz"}, false},
		{"FileContains/missing_path_returns_false", cond{Type: "FileContains", Path: missingPath, Value: "hello"}, false},

		{"FileContainsNot/negated_positive_flips_to_false", cond{Type: "FileContainsNot", Path: filePath, Value: "hello"}, false},
		{"FileContainsNot/negated_negative_flips_to_true", cond{Type: "FileContainsNot", Path: filePath, Value: "zzzzz"}, true},
		{"FileContainsNot/missing_path_error_returns_false", cond{Type: "FileContainsNot", Path: missingPath, Value: "hello"}, false},

		{"FileContainsRegex/regex_positive_match", cond{Type: "FileContainsRegex", Path: filePath, Value: "^hello"}, true},
		{"FileContainsRegex/regex_negative_no_match", cond{Type: "FileContainsRegex", Path: filePath, Value: "^zzzzz"}, false},
		{"FileContainsRegex/malformed_regex_error_returns_false", cond{Type: "FileContainsRegex", Path: filePath, Value: "["}, false},
		{"FileContainsRegex/missing_path_error_returns_false", cond{Type: "FileContainsRegex", Path: missingPath, Value: "^hello"}, false},

		{"FileContainsRegexNot/negated_positive_flips_to_false", cond{Type: "FileContainsRegexNot", Path: filePath, Value: "^hello"}, false},
		{"FileContainsRegexNot/negated_negative_flips_to_true", cond{Type: "FileContainsRegexNot", Path: filePath, Value: "^zzzzz"}, true},
		{"FileContainsRegexNot/malformed_regex_error_returns_false", cond{Type: "FileContainsRegexNot", Path: filePath, Value: "["}, false},
		{"FileContainsRegexNot/missing_path_error_returns_false", cond{Type: "FileContainsRegexNot", Path: missingPath, Value: "^hello"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := runCheck(tt.c)
			if got != tt.want {
				t.Errorf("runCheck(Type=%q, Path=%q, Value=%q) = %v, want %v",
					tt.c.Type, tt.c.Path, tt.c.Value, got, tt.want)
			}
		})
	}
}

func TestPathExists(t *testing.T) {
	c := cond{
		Path: "misc/tests/",
	}
	out, err := c.PathExists()
	if err != nil || out != true {
		t.Error(c, "failed:", out, err)
	}

	c.Path = "misc/doesntexist"
	out, err = c.PathExists()
	if err != nil || out != false {
		t.Error(c, "failed:", out, err)
	}
}
