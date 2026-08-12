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
