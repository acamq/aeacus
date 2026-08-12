//go:build linux || freebsd

package main

import (
	"errors"
	"io/fs"
	"testing"
)

type fixtureIdentityReader struct {
	files map[string][]byte
	errs  map[string]error
}

func (r fixtureIdentityReader) read(path string) ([]byte, error) {
	if err := r.errs[path]; err != nil {
		return nil, err
	}
	data, ok := r.files[path]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return data, nil
}

func (r fixtureIdentityReader) ReadPasswd() ([]byte, error) {
	return r.read(unixPasswdPath)
}

func (r fixtureIdentityReader) ReadGroup() ([]byte, error) {
	return r.read(unixGroupPath)
}

func fixtureIdentity(passwd, group string) unixIdentityDatabase {
	return unixIdentityDatabase{reader: fixtureIdentityReader{files: map[string][]byte{
		unixPasswdPath: []byte(passwd),
		unixGroupPath:  []byte(group),
	}}}
}

func TestUnixIdentityUserExistsReturnsFalseOnlyForWellFormedAbsence(t *testing.T) {
	tests := []struct {
		name    string
		passwd  string
		user    string
		want    bool
		wantErr error
	}{
		{name: "exact present user", passwd: "alice:x:1000:2000::/home/alice:/bin/sh\n", user: "alice", want: true},
		{name: "well formed absent user", passwd: "alice2:x:1001:2001::/home/alice2:/bin/sh\n", user: "alice", want: false},
		{name: "duplicate user", passwd: "alice:x:1000:2000::/:/bin/sh\nalice:x:1001:2001::/:/bin/sh\n", user: "alice", wantErr: ErrUnixIdentityDuplicate},
		{name: "malformed user", passwd: "alice:x:not-a-uid:2000::/:/bin/sh\n", user: "alice", wantErr: ErrUnixIdentityMalformed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := fixtureIdentity(tt.passwd, "").UserExists(tt.user)
			if tt.wantErr != nil {
				if got || !errors.Is(err, tt.wantErr) {
					t.Fatalf("UserExists(%q) = (%v, %v), want (false, %v)", tt.user, got, err, tt.wantErr)
				}
				assertUnixIdentityError(t, err, unixPasswdPath)
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("UserExists(%q) = (%v, %v), want (%v, nil)", tt.user, got, err, tt.want)
			}
		})
	}
}

func TestUnixIdentityUserInGroupMembershipMatrix(t *testing.T) {
	const passwd = "alice:x:1000:2000::/home/alice:/bin/sh\nalice2:x:1001:4000::/home/alice2:/bin/sh\n"
	tests := []struct {
		name      string
		groupData string
		groupName string
		user      string
		want      bool
	}{
		{name: "primary only with unrelated group name", groupData: "primary-team:x:2000:\n", groupName: "primary-team", user: "alice", want: true},
		{name: "supplementary only", groupData: "operators:x:3000:alice\n", groupName: "operators", user: "alice", want: true},
		{name: "primary and supplementary", groupData: "primary-team:x:2000:alice\n", groupName: "primary-team", user: "alice", want: true},
		{name: "neither", groupData: "operators:x:3000:bob\n", groupName: "operators", user: "alice", want: false},
		{name: "similar member is not exact", groupData: "operators:x:3000:alice2\n", groupName: "operators", user: "alice", want: false},
		{name: "matching group name does not imply membership", groupData: "alice:x:3000:\n", groupName: "alice", user: "alice", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := fixtureIdentity(passwd, tt.groupData).UserInGroup(tt.user, tt.groupName)
			if err != nil || got != tt.want {
				t.Fatalf("UserInGroup(%q) = (%v, %v), want (%v, nil)", tt.user, got, err, tt.want)
			}
		})
	}
}

func TestUnixIdentityParsersRejectMalformedAndAmbiguousRecords(t *testing.T) {
	tests := []struct {
		name    string
		passwd  string
		group   string
		wantErr error
		path    string
	}{
		{name: "passwd missing field", passwd: "alice:x:1000:2000::/home/alice\n", group: "staff:x:2000:\n", wantErr: ErrUnixIdentityMalformed, path: unixPasswdPath},
		{name: "passwd empty name", passwd: ":x:1000:2000::/:/bin/sh\n", group: "staff:x:2000:\n", wantErr: ErrUnixIdentityMalformed, path: unixPasswdPath},
		{name: "passwd malformed uid", passwd: "alice:x:not-a-uid:2000::/:/bin/sh\n", group: "staff:x:2000:\n", wantErr: ErrUnixIdentityMalformed, path: unixPasswdPath},
		{name: "passwd overflowing uid", passwd: "alice:x:4294967296:2000::/:/bin/sh\n", group: "staff:x:2000:\n", wantErr: ErrUnixIdentityMalformed, path: unixPasswdPath},
		{name: "passwd malformed gid", passwd: "alice:x:1000:nope::/:/bin/sh\n", group: "staff:x:2000:\n", wantErr: ErrUnixIdentityMalformed, path: unixPasswdPath},
		{name: "group missing field", passwd: "alice:x:1000:2000::/:/bin/sh\n", group: "staff:x:2000\n", wantErr: ErrUnixIdentityMalformed, path: unixGroupPath},
		{name: "group empty name", passwd: "alice:x:1000:2000::/:/bin/sh\n", group: ":x:2000:\n", wantErr: ErrUnixIdentityMalformed, path: unixGroupPath},
		{name: "group malformed gid", passwd: "alice:x:1000:2000::/:/bin/sh\n", group: "staff:x:nope:\n", wantErr: ErrUnixIdentityMalformed, path: unixGroupPath},
		{name: "group empty member", passwd: "alice:x:1000:2000::/:/bin/sh\n", group: "staff:x:3000:alice,,bob\n", wantErr: ErrUnixIdentityMalformed, path: unixGroupPath},
		{name: "duplicate group", passwd: "alice:x:1000:2000::/:/bin/sh\n", group: "staff:x:2000:\nstaff:x:3000:alice\n", wantErr: ErrUnixIdentityDuplicate, path: unixGroupPath},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := fixtureIdentity(tt.passwd, tt.group).UserInGroup("alice", "staff")
			if got || !errors.Is(err, tt.wantErr) {
				t.Fatalf("UserInGroup malformed fixture = (%v, %v), want (false, %v)", got, err, tt.wantErr)
			}
			assertUnixIdentityError(t, err, tt.path)
		})
	}
}

func TestUnixIdentityUserInGroupRejectsMissingAndUnreadableDatabases(t *testing.T) {
	validPasswd := []byte("alice:x:1000:2000::/:/bin/sh\n")
	validGroup := []byte("staff:x:2000:\n")
	tests := []struct {
		name    string
		reader  fixtureIdentityReader
		user    string
		group   string
		wantErr error
		path    string
	}{
		{name: "missing user", reader: fixtureIdentityReader{files: map[string][]byte{unixPasswdPath: validPasswd, unixGroupPath: validGroup}}, user: "bob", group: "staff", wantErr: ErrUnixUserNotFound, path: unixPasswdPath},
		{name: "missing group", reader: fixtureIdentityReader{files: map[string][]byte{unixPasswdPath: validPasswd, unixGroupPath: validGroup}}, user: "alice", group: "wheel", wantErr: ErrUnixGroupNotFound, path: unixGroupPath},
		{name: "missing passwd file", reader: fixtureIdentityReader{files: map[string][]byte{unixGroupPath: validGroup}}, user: "alice", group: "staff", wantErr: fs.ErrNotExist, path: unixPasswdPath},
		{name: "missing group file", reader: fixtureIdentityReader{files: map[string][]byte{unixPasswdPath: validPasswd}}, user: "alice", group: "staff", wantErr: fs.ErrNotExist, path: unixGroupPath},
		{name: "unreadable passwd file", reader: fixtureIdentityReader{files: map[string][]byte{unixGroupPath: validGroup}, errs: map[string]error{unixPasswdPath: fs.ErrPermission}}, user: "alice", group: "staff", wantErr: fs.ErrPermission, path: unixPasswdPath},
		{name: "unreadable group file", reader: fixtureIdentityReader{files: map[string][]byte{unixPasswdPath: validPasswd}, errs: map[string]error{unixGroupPath: fs.ErrPermission}}, user: "alice", group: "staff", wantErr: fs.ErrPermission, path: unixGroupPath},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := (unixIdentityDatabase{reader: tt.reader}).UserInGroup(tt.user, tt.group)
			if got || !errors.Is(err, tt.wantErr) || !errors.Is(err, ErrUnixIdentityDatabase) {
				t.Fatalf("UserInGroup(%q, %q) = (%v, %v), want database error wrapping %v", tt.user, tt.group, got, err, tt.wantErr)
			}
			assertUnixIdentityError(t, err, tt.path)
		})
	}
}

func TestUnixIdentityUserExistsRejectsUnreadableDatabase(t *testing.T) {
	identity := unixIdentityDatabase{reader: fixtureIdentityReader{errs: map[string]error{unixPasswdPath: fs.ErrPermission}}}
	got, err := identity.UserExists("alice")
	if got || !errors.Is(err, fs.ErrPermission) || !errors.Is(err, ErrUnixIdentityDatabase) {
		t.Fatalf("UserExists unreadable database = (%v, %v), want database error wrapping permission", got, err)
	}
	assertUnixIdentityError(t, err, unixPasswdPath)
}

func TestUnixIdentityAllowsBlankCommentsAndTrailingNewline(t *testing.T) {
	identity := fixtureIdentity("\n # ignored\nroot:x:0:0::/root:/bin/sh\n\nalice:x:1000:2000::/:/bin/sh\n", "# ignored\n\nprimary-team:x:2000:\n")
	got, err := identity.UserInGroup("alice", "primary-team")
	if err != nil || !got {
		t.Fatalf("UserInGroup blank/comment fixture = (%v, %v), want (true, nil)", got, err)
	}
}

func assertUnixIdentityError(t *testing.T, err error, path string) {
	t.Helper()
	var identityErr *unixIdentityError
	if !errors.As(err, &identityErr) {
		t.Fatalf("error %v has type %T, want *unixIdentityError", err, err)
	}
	if identityErr.Path != path {
		t.Fatalf("identity error path = %q, want %q", identityErr.Path, path)
	}
}
