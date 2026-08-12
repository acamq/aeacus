//go:build linux || freebsd

package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestUnixFileOwnerComparesExactUIDAndGID(t *testing.T) {
	path, uid, gid := ownedUnixFixture(t)
	tests := []struct {
		name      string
		passwdUID uint32
		passwdGID uint32
		groupGID  uint32
		groupName string
		want      bool
	}{
		{name: "legacy name derives primary GID", passwdUID: uid, passwdGID: gid, groupGID: gid, groupName: "nonstandard-primary", want: true},
		{name: "explicit unrelated group name resolves exact GID", passwdUID: uid, passwdGID: nextUnixID(gid), groupGID: gid, groupName: "release-engineering", want: true},
		{name: "UID mismatch", passwdUID: nextUnixID(uid), passwdGID: gid, groupGID: gid, groupName: "nonstandard-primary", want: false},
		{name: "derived primary GID mismatch", passwdUID: uid, passwdGID: nextUnixID(gid), groupGID: nextUnixID(gid), groupName: "nonstandard-primary", want: false},
		{name: "explicit group GID mismatch", passwdUID: uid, passwdGID: gid, groupGID: nextUnixID(gid), groupName: "release-engineering", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setUnixIdentityFixture(t,
				fmt.Sprintf("task-owner:x:%d:%d::/:/bin/sh\n", tt.passwdUID, tt.passwdGID),
				fmt.Sprintf("%s:x:%d:\n", tt.groupName, tt.groupGID),
			)
			groupName := ""
			if tt.name == "explicit unrelated group name resolves exact GID" || tt.name == "explicit group GID mismatch" {
				groupName = tt.groupName
			}
			got, err := unixFileOwner(path, "task-owner", groupName)
			if err != nil || got != tt.want {
				t.Fatalf("unixFileOwner = (%v, %v), want (%v, nil)", got, err, tt.want)
			}
			t.Logf("owner-case=%s actual=%t expected=%t error_class=nil", tt.name, got, tt.want)
		})
	}
}

func TestUnixFileOwnerFailsClosedOnIdentityErrors(t *testing.T) {
	path, _, _ := ownedUnixFixture(t)
	tests := []struct {
		name    string
		reader  fixtureIdentityReader
		group   string
		wantErr error
	}{
		{name: "unknown user", reader: identityReader("other:x:1:1::/:/bin/sh\n", "staff:x:1:\n"), wantErr: ErrUnixUserNotFound},
		{name: "unknown explicit group", reader: identityReader("task-owner:x:1:1::/:/bin/sh\n", "staff:x:1:\n"), group: "missing", wantErr: ErrUnixGroupNotFound},
		{name: "malformed passwd", reader: identityReader("task-owner:x:nope:1::/:/bin/sh\n", "staff:x:1:\n"), wantErr: ErrUnixIdentityMalformed},
		{name: "duplicate passwd", reader: identityReader("task-owner:x:1:1::/:/bin/sh\ntask-owner:x:2:2::/:/bin/sh\n", "staff:x:1:\n"), wantErr: ErrUnixIdentityDuplicate},
		{name: "duplicate explicit group", reader: identityReader("task-owner:x:1:1::/:/bin/sh\n", "staff:x:1:\nstaff:x:2:\n"), group: "staff", wantErr: ErrUnixIdentityDuplicate},
		{name: "CRLF passwd", reader: identityReader("task-owner:x:1:1::/:/bin/sh\r\n", "staff:x:1:\n"), wantErr: ErrUnixIdentityMalformed},
		{name: "CRLF explicit group", reader: identityReader("task-owner:x:1:1::/:/bin/sh\n", "staff:x:1:\r\n"), group: "staff", wantErr: ErrUnixIdentityMalformed},
		{name: "malformed group", reader: identityReader("task-owner:x:1:1::/:/bin/sh\n", "staff:x:nope:\n"), group: "staff", wantErr: ErrUnixIdentityMalformed},
		{name: "duplicate group", reader: identityReader("task-owner:x:1:1::/:/bin/sh\n", "staff:x:1:\nstaff:x:2:\n"), group: "staff", wantErr: ErrUnixIdentityDuplicate},
		{name: "CRLF group", reader: identityReader("task-owner:x:1:1::/:/bin/sh\n", "staff:x:1:\r\n"), group: "staff", wantErr: ErrUnixIdentityMalformed},
		{name: "unreadable passwd", reader: fixtureIdentityReader{errs: map[string]error{unixPasswdPath: fs.ErrPermission}}, wantErr: fs.ErrPermission},
		{name: "unreadable group", reader: fixtureIdentityReader{files: map[string][]byte{unixPasswdPath: []byte("task-owner:x:1:1::/:/bin/sh\n")}, errs: map[string]error{unixGroupPath: fs.ErrPermission}}, group: "staff", wantErr: fs.ErrPermission},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setUnixIdentityReader(t, tt.reader)
			got, err := unixFileOwner(path, "task-owner", tt.group)
			if got || !errors.Is(err, tt.wantErr) || !errors.Is(err, ErrUnixFileIdentity) {
				t.Fatalf("unixFileOwner identity failure = (%v, %v), want wrapped %v", got, err, tt.wantErr)
			}
		})
	}
}

func TestUnixFilesystemFollowsValidSymlinksAndErrorsOnDanglingLinks(t *testing.T) {
	path, uid, gid := ownedUnixFixture(t)
	if err := os.Chmod(path, 0o711|os.ModeSetuid|os.ModeSetgid|os.ModeSticky); err != nil {
		t.Fatal(err)
	}
	setUnixIdentityFixture(t, fmt.Sprintf("task-owner:x:%d:%d::/:/bin/sh\n", uid, gid), "")
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if got, err := unixFileOwner(link, "task-owner", ""); err != nil || !got {
		t.Fatalf("owner through valid symlink = (%v, %v), want (true, nil)", got, err)
	}
	if got, err := unixPermissionIs(link, "rws--s--t"); err != nil || !got {
		t.Fatalf("mode through valid symlink = (%v, %v), want (true, nil)", got, err)
	}
	t.Log("valid-link actual=true expected=true error_class=nil")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	for _, call := range []func() (bool, error){
		func() (bool, error) { return unixFileOwner(link, "task-owner", "") },
		func() (bool, error) { return unixPermissionIs(link, "rws--s--t") },
	} {
		got, err := call()
		if got || !errors.Is(err, ErrUnixFileStat) || !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("dangling symlink = (%v, %v), want wrapped stat/not-exist error", got, err)
		}
	}
	t.Log("dangling-link actual=false expected=false error_class=stat")
}

func TestUnixManualFilesystemMatrix(t *testing.T) {
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
				t.Fatalf("manual mode case = (%v, %v), want (true, nil)", got, err)
			}
			cases++
		}
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	got, err := unixPermissionIs(link, "rw-------")
	t.Logf("manual-mode cases=%d actual=true expected=true error_class=nil", cases)
	t.Logf("manual-valid-link actual=%v expected=true error_class=%s", got, unixErrorClass(err))
	if err != nil || !got {
		t.FailNow()
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	got, err = unixPermissionIs(link, "rw-------")
	t.Logf("manual-dangling-link actual=%v expected=false error_class=%s", got, unixErrorClass(err))
	if got || !errors.Is(err, ErrUnixFileStat) {
		t.FailNow()
	}
}

func unixErrorClass(err error) string {
	if err == nil {
		return "nil"
	}
	if errors.Is(err, ErrUnixFileStat) {
		return "stat"
	}
	return "unexpected"
}

func TestUnixFilesystemReturnsTypedStatErrors(t *testing.T) {
	previousStat := unixStat
	t.Cleanup(func() { unixStat = previousStat })
	unixStat = func(string) (os.FileInfo, error) { return nil, fs.ErrPermission }
	setUnixIdentityFixture(t, "task-owner:x:1:1::/:/bin/sh\n", "")
	for _, call := range []func() (bool, error){
		func() (bool, error) { return unixFileOwner("redacted", "task-owner", "") },
		func() (bool, error) { return unixPermissionIs("redacted", "rw-------") },
	} {
		got, err := call()
		if got || !errors.Is(err, ErrUnixFileStat) || !errors.Is(err, fs.ErrPermission) {
			t.Fatalf("injected stat error = (%v, %v), want wrapped permission error", got, err)
		}
		var fileErr *unixFileError
		if !errors.As(err, &fileErr) {
			t.Fatalf("stat error type = %T, want *unixFileError", err)
		}
	}
}

func TestUnixFileOwnerRejectsNonUnixStatIdentity(t *testing.T) {
	previousStat := unixStat
	t.Cleanup(func() { unixStat = previousStat })
	unixStat = func(string) (os.FileInfo, error) { return syntheticUnixFileInfo{}, nil }
	setUnixIdentityFixture(t, "task-owner:x:1:1::/:/bin/sh\n", "")
	got, err := unixFileOwner("redacted", "task-owner", "")
	if got || !errors.Is(err, ErrUnixFileIdentity) {
		t.Fatalf("non-Unix stat identity = (%v, %v), want identity error", got, err)
	}
}

func TestUnixPermissionIsEvaluatesRegularFilesAndDirectories(t *testing.T) {
	directory := t.TempDir()
	file := filepath.Join(directory, "target")
	if err := os.WriteFile(file, []byte("fixture"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(directory, 0o750); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []struct {
		path  string
		value string
	}{
		{path: file, value: "-rw-r-----"},
		{path: directory, value: "drwxr-x---"},
	} {
		got, err := unixPermissionIs(fixture.path, fixture.value)
		if err != nil || !got {
			t.Fatalf("unixPermissionIs(%q) = (%v, %v), want (true, nil)", fixture.value, got, err)
		}
		t.Logf("filesystem-kind actual=true expected=true error_class=nil")
	}
}

type syntheticUnixFileInfo struct{}

func (syntheticUnixFileInfo) Name() string       { return "redacted" }
func (syntheticUnixFileInfo) Size() int64        { return 0 }
func (syntheticUnixFileInfo) Mode() os.FileMode  { return 0o600 }
func (syntheticUnixFileInfo) ModTime() time.Time { return time.Time{} }
func (syntheticUnixFileInfo) IsDir() bool        { return false }
func (syntheticUnixFileInfo) Sys() interface{}   { return nil }

func ownedUnixFixture(t *testing.T) (string, uint32, uint32) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatalf("stat identity type = %T, want *syscall.Stat_t", info.Sys())
	}
	return path, stat.Uid, stat.Gid
}

func nextUnixID(value uint32) uint32 {
	if value == ^uint32(0) {
		return value - 1
	}
	return value + 1
}

func identityReader(passwd, group string) fixtureIdentityReader {
	return fixtureIdentityReader{files: map[string][]byte{
		unixPasswdPath: []byte(passwd),
		unixGroupPath:  []byte(group),
	}}
}

func setUnixIdentityFixture(t *testing.T, passwd, group string) {
	t.Helper()
	setUnixIdentityReader(t, identityReader(passwd, group))
}

func setUnixIdentityReader(t *testing.T, reader unixIdentityReader) {
	t.Helper()
	previousReader := unixIdentityFiles
	t.Cleanup(func() { unixIdentityFiles = previousReader })
	unixIdentityFiles = reader
}
