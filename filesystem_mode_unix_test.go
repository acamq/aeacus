//go:build linux || freebsd

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestUnixModeCoversEveryOrdinaryPermissionCombination(t *testing.T) {
	for raw := 0; raw <= 0o777; raw++ {
		mode := os.FileMode(raw)
		value := ordinaryUnixMode(mode)
		expected, err := parseUnixMode(value)
		if err != nil {
			t.Fatalf("parseUnixMode(%q): %v", value, err)
		}
		if !unixModeMatches(mode, expected) {
			t.Fatalf("mode %03o did not match %q", raw, value)
		}
	}
}

func TestUnixModeCoversEverySpecialAndExecuteCombination(t *testing.T) {
	for executeBits := 0; executeBits < 8; executeBits++ {
		for specialBits := 0; specialBits < 8; specialBits++ {
			mode, value := specialUnixMode(executeBits, specialBits)
			expected, err := parseUnixMode(value)
			if err != nil {
				t.Fatalf("parseUnixMode(%q): %v", value, err)
			}
			if !unixModeMatches(mode, expected) {
				t.Fatalf("mode %v did not match %q", mode, value)
			}
		}
	}
}

func TestUnixModeMatchesWildcardsAndIgnoresFileType(t *testing.T) {
	tests := []struct {
		name   string
		actual os.FileMode
		value  string
		want   bool
	}{
		{name: "nine character exact", actual: 0o640, value: "rw-r-----", want: true},
		{name: "regular type ignored", actual: 0o640, value: "-rw-r-----", want: true},
		{name: "directory type ignored", actual: os.ModeDir | 0o640, value: "drw-r-----", want: true},
		{name: "configured type ignored", actual: os.ModeDir | 0o640, value: "-rw-r-----", want: true},
		{name: "wildcards ignore selected bits", actual: 0o647, value: "rw-r--???", want: true},
		{name: "ordinary mismatch", actual: 0o640, value: "rw-r----x", want: false},
		{name: "special mismatch", actual: 0o640 | os.ModeSetuid, value: "rw-r-----", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expected, err := parseUnixMode(tt.value)
			if err != nil {
				t.Fatal(err)
			}
			if got := unixModeMatches(tt.actual, expected); got != tt.want {
				t.Fatalf("unixModeMatches(%v, %q) = %v, want %v", tt.actual, tt.value, got, tt.want)
			}
		})
	}
}

func TestUnixModeTrimsSurroundingUnicodeWhitespace(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{name: "reported ASCII whitespace", value: " rw-------\n"},
		{name: "Unicode whitespace", value: "\u2003?rw-------\u00a0"},
		{name: "special bits", value: "\t rws--S--t \r\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expected, err := parseUnixMode(tt.value)
			if err != nil {
				t.Fatal(err)
			}
			actual := os.FileMode(0o600)
			if tt.name == "special bits" {
				actual = 0o701 | os.ModeSetuid | os.ModeSetgid | os.ModeSticky
			}
			if !unixModeMatches(actual, expected) {
				t.Fatalf("trimmed mode %q did not match %v", tt.value, actual)
			}
		})
	}
}

func TestUnixModeRejectsMalformedSymbolicModes(t *testing.T) {
	for _, value := range []string{
		"rwx", "rwxrwxrwxq", "rwsrwxrwz", "srwxrwxrw", "xrwxrwxrwx", "rwtrwxrwx", "rwxrwt rwx",
		"rwxrwxrwé", "!rwxrwxrwx", "xrwxrwxrwx", "rwt------", "-----T---", "--------s",
		"", " \t\r\n", " rwx ", "rwx rwxrwx", "rw\x00r-----", string([]byte{0xff, 'r', 'w', '-', 'r', '-', '-', '-', '-', '-'}),
	} {
		t.Run(fmt.Sprintf("%q", value), func(t *testing.T) {
			_, err := parseUnixMode(value)
			if !errors.Is(err, ErrUnixModeSyntax) {
				t.Fatalf("parseUnixMode(%q) error = %v, want ErrUnixModeSyntax", value, err)
			}
			var modeErr *unixModeError
			if !errors.As(err, &modeErr) {
				t.Fatalf("parseUnixMode(%q) error type = %T, want *unixModeError", value, err)
			}
		})
	}
}

func TestUnixModeRejectsInvalidSymbolAtEveryPermissionPosition(t *testing.T) {
	for index := range 9 {
		value := []byte("rwxrwxrwx")
		value[index] = '!'
		if _, err := parseUnixMode(string(value)); !errors.Is(err, ErrUnixModeSyntax) {
			t.Fatalf("permission index %d error = %v, want ErrUnixModeSyntax", index, err)
		}
	}
}

func TestUnixPermissionIsUsesRealSpecialModeMatrix(t *testing.T) {
	path := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := 0
	for executeBits := 0; executeBits < 8; executeBits++ {
		for specialBits := 0; specialBits < 8; specialBits++ {
			mode, value := specialUnixMode(executeBits, specialBits)
			if err := os.Chmod(path, mode); err != nil {
				t.Fatal(err)
			}
			got, err := unixPermissionIs(path, value)
			if err != nil || !got {
				t.Fatalf("actual mode %v against %q = (%v, %v), want (true, nil)", mode, value, got, err)
			}
			t.Logf("mode=%s actual=%t expected=true error_class=nil", value, got)
			cases++
		}
	}
	t.Logf("special-mode-matrix cases=%d actual=true expected=true error_class=nil", cases)
}

func ordinaryUnixMode(mode os.FileMode) string {
	positions := [...]os.FileMode{0o400, 0o200, 0o100, 0o040, 0o020, 0o010, 0o004, 0o002, 0o001}
	present := "rwxrwxrwx"
	value := []byte("---------")
	for index, bit := range positions {
		if mode&bit != 0 {
			value[index] = present[index]
		}
	}
	return string(value)
}

func specialUnixMode(executeBits, specialBits int) (os.FileMode, string) {
	mode := os.FileMode(0o600)
	value := []byte("rw-------")
	executeModes := [...]os.FileMode{0o100, 0o010, 0o001}
	specialModes := [...]os.FileMode{os.ModeSetuid, os.ModeSetgid, os.ModeSticky}
	positions := [...]int{2, 5, 8}
	lower := [...]byte{'s', 's', 't'}
	upper := [...]byte{'S', 'S', 'T'}
	for index := range positions {
		executable := executeBits&(1<<index) != 0
		special := specialBits&(1<<index) != 0
		if executable {
			mode |= executeModes[index]
			value[positions[index]] = 'x'
		}
		if special {
			mode |= specialModes[index]
			if executable {
				value[positions[index]] = lower[index]
			} else {
				value[positions[index]] = upper[index]
			}
		}
	}
	return mode, string(value)
}
