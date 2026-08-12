//go:build linux || freebsd

package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	unixPasswdPath = "/etc/passwd"
	unixGroupPath  = "/etc/group"
)

var (
	ErrUnixIdentityDatabase  = errors.New("unix identity database error")
	ErrUnixIdentityMalformed = errors.New("malformed unix identity record")
	ErrUnixIdentityDuplicate = errors.New("duplicate unix identity record")
	ErrUnixUserNotFound      = errors.New("unix user not found")
	ErrUnixGroupNotFound     = errors.New("unix group not found")
)

type unixUID uint32
type unixGID uint32

type unixUserRecord struct {
	name       string
	uid        unixUID
	primaryGID unixGID
}

type unixGroupRecord struct {
	name    string
	gid     unixGID
	members map[string]struct{}
}

type unixIdentityError struct {
	Kind  error
	Cause error
	Path  string
	Line  int
	Name  string
}

func (e *unixIdentityError) Error() string {
	detail := e.Kind.Error()
	if e.Name != "" {
		detail += fmt.Sprintf(" for %q", e.Name)
	}
	if e.Line > 0 {
		detail += fmt.Sprintf(" at %s:%d", e.Path, e.Line)
	} else {
		detail += " at " + e.Path
	}
	if e.Cause != nil {
		detail += ": " + e.Cause.Error()
	}
	return detail
}

func (e *unixIdentityError) Unwrap() error {
	return errors.Join(ErrUnixIdentityDatabase, e.Kind, e.Cause)
}

type unixIdentityReader interface {
	ReadPasswd() ([]byte, error)
	ReadGroup() ([]byte, error)
}

// localUnixIdentityReader is limited to local account files and never consults NSS.
type localUnixIdentityReader struct{}

func (localUnixIdentityReader) ReadPasswd() ([]byte, error) {
	return os.ReadFile(unixPasswdPath)
}

func (localUnixIdentityReader) ReadGroup() ([]byte, error) {
	return os.ReadFile(unixGroupPath)
}

var unixIdentityFiles unixIdentityReader = localUnixIdentityReader{}

type unixIdentityDatabase struct {
	reader unixIdentityReader
}

func (d unixIdentityDatabase) UserExists(name string) (bool, error) {
	data, err := d.readPasswd()
	if err != nil {
		return false, err
	}
	users, err := parseUnixPasswd(data)
	if err != nil {
		return false, err
	}
	_, found := users[name]
	return found, nil
}

func (d unixIdentityDatabase) UserInGroup(userName, groupName string) (bool, error) {
	passwdData, err := d.readPasswd()
	if err != nil {
		return false, err
	}
	users, err := parseUnixPasswd(passwdData)
	if err != nil {
		return false, err
	}
	groupData, err := d.readGroup()
	if err != nil {
		return false, err
	}
	groups, err := parseUnixGroup(groupData)
	if err != nil {
		return false, err
	}
	user, found := users[userName]
	if !found {
		return false, &unixIdentityError{Kind: ErrUnixUserNotFound, Path: unixPasswdPath, Name: userName}
	}
	group, found := groups[groupName]
	if !found {
		return false, &unixIdentityError{Kind: ErrUnixGroupNotFound, Path: unixGroupPath, Name: groupName}
	}
	if user.primaryGID == group.gid {
		return true, nil
	}
	_, found = group.members[user.name]
	return found, nil
}

func (d unixIdentityDatabase) readPasswd() ([]byte, error) {
	data, err := d.reader.ReadPasswd()
	if err != nil {
		return nil, &unixIdentityError{Kind: ErrUnixIdentityDatabase, Cause: err, Path: unixPasswdPath}
	}
	return data, nil
}

func (d unixIdentityDatabase) readGroup() ([]byte, error) {
	data, err := d.reader.ReadGroup()
	if err != nil {
		return nil, &unixIdentityError{Kind: ErrUnixIdentityDatabase, Cause: err, Path: unixGroupPath}
	}
	return data, nil
}

func parseUnixPasswd(data []byte) (map[string]unixUserRecord, error) {
	users := make(map[string]unixUserRecord)
	for index, line := range strings.Split(string(data), "\n") {
		if strings.ContainsRune(line, '\r') {
			return nil, &unixIdentityError{Kind: ErrUnixIdentityMalformed, Path: unixPasswdPath, Line: index + 1}
		}
		if ignoredUnixIdentityLine(line) {
			continue
		}
		fields := strings.Split(line, ":")
		if len(fields) != 7 || fields[0] == "" {
			return nil, &unixIdentityError{Kind: ErrUnixIdentityMalformed, Path: unixPasswdPath, Line: index + 1}
		}
		uid, err := parseUnixUID(fields[2])
		if err != nil {
			return nil, &unixIdentityError{Kind: ErrUnixIdentityMalformed, Cause: err, Path: unixPasswdPath, Line: index + 1, Name: fields[0]}
		}
		gid, err := parseUnixGID(fields[3])
		if err != nil {
			return nil, &unixIdentityError{Kind: ErrUnixIdentityMalformed, Cause: err, Path: unixPasswdPath, Line: index + 1, Name: fields[0]}
		}
		if _, duplicate := users[fields[0]]; duplicate {
			return nil, &unixIdentityError{Kind: ErrUnixIdentityDuplicate, Path: unixPasswdPath, Line: index + 1, Name: fields[0]}
		}
		users[fields[0]] = unixUserRecord{name: fields[0], uid: uid, primaryGID: gid}
	}
	return users, nil
}

func parseUnixGroup(data []byte) (map[string]unixGroupRecord, error) {
	groups := make(map[string]unixGroupRecord)
	for index, line := range strings.Split(string(data), "\n") {
		if strings.ContainsRune(line, '\r') {
			return nil, &unixIdentityError{Kind: ErrUnixIdentityMalformed, Path: unixGroupPath, Line: index + 1}
		}
		if ignoredUnixIdentityLine(line) {
			continue
		}
		fields := strings.Split(line, ":")
		if len(fields) != 4 || fields[0] == "" {
			return nil, &unixIdentityError{Kind: ErrUnixIdentityMalformed, Path: unixGroupPath, Line: index + 1}
		}
		gid, err := parseUnixGID(fields[2])
		if err != nil {
			return nil, &unixIdentityError{Kind: ErrUnixIdentityMalformed, Cause: err, Path: unixGroupPath, Line: index + 1, Name: fields[0]}
		}
		members, err := parseUnixGroupMembers(fields[3])
		if err != nil {
			return nil, &unixIdentityError{Kind: ErrUnixIdentityMalformed, Cause: err, Path: unixGroupPath, Line: index + 1, Name: fields[0]}
		}
		if _, duplicate := groups[fields[0]]; duplicate {
			return nil, &unixIdentityError{Kind: ErrUnixIdentityDuplicate, Path: unixGroupPath, Line: index + 1, Name: fields[0]}
		}
		groups[fields[0]] = unixGroupRecord{name: fields[0], gid: gid, members: members}
	}
	return groups, nil
}

func parseUnixUID(raw string) (unixUID, error) {
	value, err := parseUnixID(raw)
	return unixUID(value), err
}

func parseUnixGID(raw string) (unixGID, error) {
	value, err := parseUnixID(raw)
	return unixGID(value), err
}

func parseUnixID(raw string) (uint32, error) {
	if raw == "" {
		return 0, errors.New("empty numeric identifier")
	}
	for _, value := range []byte(raw) {
		if value < '0' || value > '9' {
			return 0, errors.New("non-decimal numeric identifier")
		}
	}
	parsed, err := strconv.ParseUint(raw, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("numeric identifier: %w", err)
	}
	return uint32(parsed), nil
}

func parseUnixGroupMembers(raw string) (map[string]struct{}, error) {
	members := make(map[string]struct{})
	if raw == "" {
		return members, nil
	}
	for _, member := range strings.Split(raw, ",") {
		if member == "" {
			return nil, errors.New("empty group member")
		}
		members[member] = struct{}{}
	}
	return members, nil
}

func ignoredUnixIdentityLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	return trimmed == "" || strings.HasPrefix(trimmed, "#")
}
