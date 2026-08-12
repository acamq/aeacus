//go:build linux

package main

import (
	"errors"
	"io/fs"
	"testing"
)

func TestLinuxIdentityChecksUseInjectedLocalDatabases(t *testing.T) {
	previousReader := unixIdentityFiles
	t.Cleanup(func() { unixIdentityFiles = previousReader })
	unixIdentityFiles = fixtureIdentityReader{files: map[string][]byte{
		unixPasswdPath: []byte("alice:x:1000:2000::/:/bin/sh\n"),
		unixGroupPath:  []byte("primary-team:x:2000:\noperators:x:3000:alice\n"),
	}}

	tests := []struct {
		name string
		call func() (bool, error)
	}{
		{name: "user exists", call: func() (bool, error) { return (cond{User: "alice"}).UserExists() }},
		{name: "primary group", call: func() (bool, error) { return (cond{User: "alice", Group: "primary-team"}).UserInGroup() }},
		{name: "supplementary group", call: func() (bool, error) { return (cond{User: "alice", Group: "operators"}).UserInGroup() }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.call()
			if err != nil || !got {
				t.Fatalf("identity check = (%v, %v), want (true, nil)", got, err)
			}
		})
	}
}

func TestLinuxUserInGroupNotCannotPassOnUnreadableDatabase(t *testing.T) {
	tests := []struct {
		name   string
		reader fixtureIdentityReader
	}{
		{name: "unreadable group", reader: fixtureIdentityReader{files: map[string][]byte{unixPasswdPath: []byte("alice:x:1000:2000::/:/bin/sh\n")}, errs: map[string]error{unixGroupPath: fs.ErrPermission}}},
		{name: "missing user", reader: fixtureIdentityReader{files: map[string][]byte{unixPasswdPath: []byte("bob:x:1001:2001::/:/bin/sh\n"), unixGroupPath: []byte("staff:x:2000:\n")}}},
		{name: "missing group", reader: fixtureIdentityReader{files: map[string][]byte{unixPasswdPath: []byte("alice:x:1000:2000::/:/bin/sh\n"), unixGroupPath: []byte("wheel:x:2000:\n")}}},
		{name: "malformed passwd", reader: fixtureIdentityReader{files: map[string][]byte{unixPasswdPath: []byte("alice:x:bad:2000::/:/bin/sh\n"), unixGroupPath: []byte("staff:x:2000:\n")}}},
		{name: "duplicate group", reader: fixtureIdentityReader{files: map[string][]byte{unixPasswdPath: []byte("alice:x:1000:2000::/:/bin/sh\n"), unixGroupPath: []byte("staff:x:2000:\nstaff:x:3000:alice\n")}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			previousReader := unixIdentityFiles
			t.Cleanup(func() { unixIdentityFiles = previousReader })
			unixIdentityFiles = tt.reader
			c := cond{Type: "UserInGroupNot", User: "alice", Group: "staff"}
			if err := obfuscateCond(&c); err != nil {
				t.Fatalf("obfuscate condition: %v", err)
			}
			if runCheck(c) {
				t.Fatal("UserInGroupNot passed despite indeterminate identity state")
			}
		})
	}
}

func TestLinuxUserExistsNotCannotPassOnDatabaseError(t *testing.T) {
	previousReader := unixIdentityFiles
	t.Cleanup(func() { unixIdentityFiles = previousReader })
	unixIdentityFiles = fixtureIdentityReader{errs: map[string]error{unixPasswdPath: fs.ErrPermission}}
	c := cond{Type: "UserExistsNot", User: "alice"}
	if err := obfuscateCond(&c); err != nil {
		t.Fatalf("obfuscate condition: %v", err)
	}
	if runCheck(c) {
		t.Fatal("UserExistsNot passed despite an unreadable passwd database")
	}
	_, err := (cond{User: "alice"}).UserExists()
	if !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("UserExists error = %v, want permission in error chain", err)
	}
}

func TestLinuxIdentityNegationPreservesDeterminateAbsence(t *testing.T) {
	previousReader := unixIdentityFiles
	t.Cleanup(func() { unixIdentityFiles = previousReader })
	unixIdentityFiles = fixtureIdentityReader{files: map[string][]byte{
		unixPasswdPath: []byte("alice:x:1000:2000::/:/bin/sh\n"),
		unixGroupPath:  []byte("staff:x:3000:bob\n"),
	}}
	for _, condition := range []cond{
		{Type: "UserExistsNot", User: "missing"},
		{Type: "UserInGroupNot", User: "alice", Group: "staff"},
	} {
		if err := obfuscateCond(&condition); err != nil {
			t.Fatalf("obfuscate condition: %v", err)
		}
		if !runCheck(condition) {
			t.Fatalf("%s failed on determinate absence", condition.Type)
		}
	}
}
