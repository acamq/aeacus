//go:build linux

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestLinuxFileOwnerPreservesLegacyNameSyntax(t *testing.T) {
	path, uid, gid := ownedUnixFixture(t)
	setUnixIdentityFixture(t, fmt.Sprintf("task-owner:x:%d:%d::/:/bin/sh\n", uid, gid), "nonstandard-primary:x:9999:\n")
	got, err := (cond{Path: path, Name: "task-owner"}).FileOwner()
	if err != nil || !got {
		t.Fatalf("legacy FileOwner = (%v, %v), want (true, nil)", got, err)
	}
}

func TestLinuxPermissionIsPreservesSymbolicSyntax(t *testing.T) {
	path, _, _ := ownedUnixFixture(t)
	got, err := (cond{Path: path, Value: "?rw-------"}).PermissionIs()
	if err != nil || !got {
		t.Fatalf("legacy PermissionIs = (%v, %v), want (true, nil)", got, err)
	}
}

func TestLinuxPermissionIsTrimsSurroundingWhitespace(t *testing.T) {
	path, _, _ := ownedUnixFixture(t)
	got, err := (cond{Path: path, Value: "\t rw------- \r\n"}).PermissionIs()
	t.Logf("trimmed-adapter actual=%t expected=true error_class=%s", got, unixErrorClass(err))
	if err != nil || !got {
		t.Fatalf("trimmed PermissionIs = (%v, %v), want (true, nil)", got, err)
	}
}

func TestLinuxPermissionWhitespaceRunCheckIsDeterminate(t *testing.T) {
	path, _, _ := ownedUnixFixture(t)
	tests := []struct {
		name      string
		condition cond
		want      bool
	}{
		{name: "trimmed valid passes", condition: cond{Type: "PermissionIs", Path: path, Value: "\t rw------- \r\n"}, want: true},
		{name: "trimmed valid negation fails", condition: cond{Type: "PermissionIsNot", Path: path, Value: "\u2003rw-------\u00a0"}, want: false},
		{name: "internal whitespace negation fails closed", condition: cond{Type: "PermissionIsNot", Path: path, Value: "rw- --- ---"}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := obfuscateCond(&tt.condition); err != nil {
				t.Fatal(err)
			}
			if got := runCheck(tt.condition); got != tt.want {
				t.Fatalf("runCheck = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLinuxPermissionWhitespaceScoringIsFailClosed(t *testing.T) {
	path, _, _ := ownedUnixFixture(t)
	previousImage := image
	t.Cleanup(func() { image = previousImage })
	tests := []struct {
		name      string
		condition cond
		wantScore int
	}{
		{name: "trimmed valid scores", condition: cond{Type: "PermissionIs", Path: path, Value: "\t rw------- \r\n"}, wantScore: 7},
		{name: "internal whitespace does not score", condition: cond{Type: "PermissionIsNot", Path: path, Value: "rw- --- ---"}, wantScore: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			image = &imageData{}
			if err := obfuscateCond(&tt.condition); err != nil {
				t.Fatal(err)
			}
			scoreCheck(check{Points: 7, Pass: []cond{tt.condition}})
			t.Logf("score-case=%s actual=%d expected=%d", tt.name, image.Score, tt.wantScore)
			if image.Score != tt.wantScore {
				t.Fatalf("score = %d, want %d", image.Score, tt.wantScore)
			}
		})
	}
}

func TestLinuxFilesystemNegationDoesNotRewardErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing")
	tests := []cond{
		{Type: "PermissionIsNot", Path: path, Value: "rw-------"},
		{Type: "PermissionIsNot", Path: path, Value: "malformed"},
		{Type: "FileOwnerNot", Path: path, Name: "missing-user"},
	}
	setUnixIdentityFixture(t, "other:x:1:1::/:/bin/sh\n", "")
	for _, condition := range tests {
		if err := obfuscateCond(&condition); err != nil {
			t.Fatal(err)
		}
		if runCheck(condition) {
			t.Fatalf("%s passed despite indeterminate error", condition.Type)
		}
	}
}

func TestLinuxFilesystemNegationPassesOnlyOnDeterminateMismatch(t *testing.T) {
	path, uid, gid := ownedUnixFixture(t)
	setUnixIdentityFixture(t, fmt.Sprintf("task-owner:x:%d:%d::/:/bin/sh\n", nextUnixID(uid), gid), "")
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, condition := range []cond{
		{Type: "PermissionIsNot", Path: path, Value: "rwx------"},
		{Type: "FileOwnerNot", Path: path, Name: "task-owner"},
	} {
		if err := obfuscateCond(&condition); err != nil {
			t.Fatal(err)
		}
		if !runCheck(condition) {
			t.Fatalf("%s failed despite determinate mismatch", condition.Type)
		}
	}
}
