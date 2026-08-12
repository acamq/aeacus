//go:build linux || freebsd

package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"
)

var (
	ErrUnixFileStat     = errors.New("unix file stat error")
	ErrUnixFileIdentity = errors.New("unix file identity error")
	ErrUnixModeSyntax   = errors.New("invalid unix symbolic mode")
)

const unixFileTypeSymbols = "?-dalTLDpSugct"

type unixOwnerExpectation struct {
	uid unixUID
	gid unixGID
}

type unixFileError struct {
	Kind  error
	Cause error
	Path  string
}

func (e *unixFileError) Error() string {
	if e.Cause == nil {
		return fmt.Sprintf("%s for %q", e.Kind, e.Path)
	}
	return fmt.Sprintf("%s for %q: %v", e.Kind, e.Path, e.Cause)
}

func (e *unixFileError) Unwrap() error {
	return errors.Join(e.Kind, e.Cause)
}

type unixModeError struct {
	Value string
	Index int
}

func (e *unixModeError) Error() string {
	if e.Index < 0 {
		return fmt.Sprintf("%s %q: expected 9 or 10 characters", ErrUnixModeSyntax, e.Value)
	}
	return fmt.Sprintf("%s %q at index %d", ErrUnixModeSyntax, e.Value, e.Index)
}

func (e *unixModeError) Unwrap() error {
	return ErrUnixModeSyntax
}

type unixModeExpectation struct {
	mask os.FileMode
	bits os.FileMode
}

type unixModePosition struct {
	present byte
	bit     os.FileMode
	special os.FileMode
	lower   byte
	upper   byte
}

var unixModePositions = [...]unixModePosition{
	{present: 'r', bit: 0o400},
	{present: 'w', bit: 0o200},
	{present: 'x', bit: 0o100, special: os.ModeSetuid, lower: 's', upper: 'S'},
	{present: 'r', bit: 0o040},
	{present: 'w', bit: 0o020},
	{present: 'x', bit: 0o010, special: os.ModeSetgid, lower: 's', upper: 'S'},
	{present: 'r', bit: 0o004},
	{present: 'w', bit: 0o002},
	{present: 'x', bit: 0o001, special: os.ModeSticky, lower: 't', upper: 'T'},
}

var unixStat = os.Stat

func unixFileOwner(path, userName, groupName string) (bool, error) {
	expected, err := resolveUnixOwnerExpectation(userName, groupName)
	if err != nil {
		return false, &unixFileError{Kind: ErrUnixFileIdentity, Cause: err, Path: path}
	}
	info, err := statUnixFile(path)
	if err != nil {
		return false, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return false, &unixFileError{Kind: ErrUnixFileIdentity, Path: path}
	}
	return unixUID(stat.Uid) == expected.uid && unixGID(stat.Gid) == expected.gid, nil
}

func unixPermissionIs(path, value string) (bool, error) {
	expected, err := parseUnixMode(value)
	if err != nil {
		return false, err
	}
	info, err := statUnixFile(path)
	if err != nil {
		return false, err
	}
	actual := info.Mode() & (os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky)
	return unixModeMatches(actual, expected), nil
}

func resolveUnixOwnerExpectation(userName, groupName string) (unixOwnerExpectation, error) {
	database := unixIdentityDatabase{reader: unixIdentityFiles}
	user, err := database.lookupUser(userName)
	if err != nil {
		return unixOwnerExpectation{}, err
	}
	if groupName == "" {
		return unixOwnerExpectation{uid: user.uid, gid: user.primaryGID}, nil
	}
	group, err := database.lookupGroup(groupName)
	if err != nil {
		return unixOwnerExpectation{}, err
	}
	return unixOwnerExpectation{uid: user.uid, gid: group.gid}, nil
}

func statUnixFile(path string) (os.FileInfo, error) {
	info, err := unixStat(path)
	if err != nil {
		return nil, &unixFileError{Kind: ErrUnixFileStat, Cause: err, Path: path}
	}
	return info, nil
}

func parseUnixMode(raw string) (unixModeExpectation, error) {
	value := strings.TrimSpace(raw)
	if len(value) == 10 {
		if strings.IndexByte(unixFileTypeSymbols, value[0]) < 0 {
			return unixModeExpectation{}, &unixModeError{Value: value, Index: 0}
		}
		value = value[1:]
	} else if len(value) != 9 {
		return unixModeExpectation{}, &unixModeError{Value: value, Index: -1}
	}
	expected := unixModeExpectation{}
	for index, symbol := range []byte(value) {
		position := unixModePositions[index]
		if symbol == '?' {
			continue
		}
		expected.mask |= position.bit | position.special
		switch symbol {
		case '-':
		case position.present:
			expected.bits |= position.bit
		case position.lower:
			if position.special == 0 {
				return unixModeExpectation{}, &unixModeError{Value: value, Index: index}
			}
			expected.bits |= position.bit | position.special
		case position.upper:
			if position.special == 0 {
				return unixModeExpectation{}, &unixModeError{Value: value, Index: index}
			}
			expected.bits |= position.special
		default:
			return unixModeExpectation{}, &unixModeError{Value: value, Index: index}
		}
	}
	return expected, nil
}

func unixModeMatches(actual os.FileMode, expected unixModeExpectation) bool {
	return actual&expected.mask == expected.bits
}
