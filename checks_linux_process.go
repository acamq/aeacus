//go:build linux

package main

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	dpkgQueryPath = "/usr/bin/dpkg-query"
	rpmQueryPath  = "/usr/bin/rpm"
	systemctlPath = "/usr/bin/systemctl"
)

var (
	packageOperandPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9+_.-]{0,127}$`)
	serviceOperandPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)
)

type processCommandRunner interface {
	RunBuiltIn(string, []string) (processResult, error)
}

type processOperandError struct {
	kind  string
	value string
}

func (e *processOperandError) Error() string {
	return fmt.Sprintf("invalid %s operand %q", e.kind, e.value)
}

type processOutputError struct {
	path   string
	detail string
}

func (e *processOutputError) Error() string {
	return fmt.Sprintf("malformed output from %s: %s", e.path, e.detail)
}

type packageQuery struct {
	installed bool
	version   string
}

func (c cond) ProgramInstalled() (bool, error) {
	c.requireArgs("Name")
	return programInstalledWithRunner(c, unixRunner)
}

func programInstalledWithRunner(c cond, runner processCommandRunner) (bool, error) {
	if !packageOperandPattern.MatchString(c.Name) {
		return false, &processOperandError{kind: "package", value: c.Name}
	}
	query, fallback, err := queryDpkg(runner, c.Name)
	if err != nil {
		return false, err
	}
	if fallback {
		query, err = queryRPM(runner, c.Name)
		if err != nil {
			return false, err
		}
	}
	return query.installed, nil
}

func (c cond) ProgramVersion() (bool, error) {
	c.requireArgs("Name", "Value")
	return programVersionWithRunner(c, unixRunner)
}

func programVersionWithRunner(c cond, runner processCommandRunner) (bool, error) {
	if !packageOperandPattern.MatchString(c.Name) {
		return false, &processOperandError{kind: "package", value: c.Name}
	}
	query, fallback, err := queryDpkg(runner, c.Name)
	if err != nil {
		return false, err
	}
	if fallback {
		query, err = queryRPM(runner, c.Name)
		if err != nil {
			return false, err
		}
	}
	return query.installed && query.version == c.Value, nil
}

func queryDpkg(runner processCommandRunner, name string) (packageQuery, bool, error) {
	result, err := runner.RunBuiltIn(
		dpkgQueryPath,
		[]string{"-W", "-f=${Status}\t${Version}\n", name},
	)
	if err != nil {
		var startError *processStartError
		if errors.As(err, &startError) && errors.Is(err, os.ErrNotExist) {
			return packageQuery{}, true, nil
		}
		if code, ok := processErrorExitCode(err); ok && code == 1 {
			return packageQuery{}, false, nil
		}
		return packageQuery{}, false, err
	}
	query, err := parseDpkgQuery(result.stdout)
	return query, false, err
}

func queryRPM(runner processCommandRunner, name string) (packageQuery, error) {
	result, err := runner.RunBuiltIn(
		rpmQueryPath,
		[]string{"-q", "--qf", "%{VERSION}-%{RELEASE}\n", name},
	)
	if err != nil {
		if code, ok := processErrorExitCode(err); ok && code == 1 {
			return packageQuery{}, nil
		}
		return packageQuery{}, err
	}
	version, err := parseSingleLine(rpmQueryPath, result.stdout)
	if err != nil {
		return packageQuery{}, err
	}
	return packageQuery{installed: true, version: version}, nil
}

func parseDpkgQuery(output []byte) (packageQuery, error) {
	line, err := parseSingleLine(dpkgQueryPath, output)
	if err != nil {
		return packageQuery{}, err
	}
	fields := strings.Split(line, "\t")
	if len(fields) != 2 || fields[1] == "" {
		return packageQuery{}, &processOutputError{path: dpkgQueryPath, detail: "expected status and version"}
	}
	status := strings.Fields(fields[0])
	if len(status) != 3 {
		return packageQuery{}, &processOutputError{path: dpkgQueryPath, detail: "invalid package status"}
	}
	if status[1] != "ok" || status[2] != "installed" {
		return packageQuery{}, &processOutputError{path: dpkgQueryPath, detail: "unexpected package status"}
	}
	return packageQuery{installed: true, version: fields[1]}, nil
}

func parseSingleLine(path string, output []byte) (string, error) {
	if !utf8.Valid(output) || len(output) < 2 || output[len(output)-1] != '\n' || strings.Count(string(output), "\n") != 1 {
		return "", &processOutputError{path: path, detail: "expected one UTF-8 line"}
	}
	line := string(output[:len(output)-1])
	for _, character := range line {
		if character < ' ' && character != '\t' {
			return "", &processOutputError{path: path, detail: "control character"}
		}
	}
	return line, nil
}

func processErrorExitCode(err error) (int, bool) {
	var exitError *processExitError
	if !errors.As(err, &exitError) {
		return 0, false
	}
	return exitError.code, true
}

func (c cond) ServiceUp() (bool, error) {
	c.requireArgs("Name")
	return serviceUpWithRunner(c, unixRunner)
}

func serviceUpWithRunner(c cond, runner processCommandRunner) (bool, error) {
	if !serviceOperandPattern.MatchString(c.Name) {
		return false, &processOperandError{kind: "service", value: c.Name}
	}
	result, err := runner.RunBuiltIn(systemctlPath, []string{"is-active", c.Name})
	state := strings.TrimSuffix(string(result.stdout), "\n")
	if err == nil {
		if state == "active" || state == "reloading" {
			return true, nil
		}
		return false, &processOutputError{path: systemctlPath, detail: "unexpected active state"}
	}
	code, isExit := processErrorExitCode(err)
	if !isExit {
		return false, err
	}
	if code == 3 && (state == "inactive" || state == "failed" || state == "deactivating") {
		return false, nil
	}
	if code == 4 && (state == "unknown" || state == "inactive") {
		return false, nil
	}
	if code == 3 || code == 4 {
		return false, &processOutputError{path: systemctlPath, detail: "status does not match exit code"}
	}
	return false, err
}
