// checks.go contains checks that are identical for both Linux and Windows.

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
)

// requireArgs is a convenience function that prints a warning if any required
// parameters for a given condition are not provided.
func (c cond) requireArgs(args ...interface{}) {
	// Don't process internal calls -- assume the developers know what they're
	// doing. This also prevents extra errors being printed when they don't pass
	// required arguments.
	if c.Type == "" {
		return
	}

	v := reflect.ValueOf(c)
	vType := v.Type()
	for i := 0; i < v.NumField(); i++ {
		if v.Field(i).Kind() != reflect.String || vType.Field(i).Name == "Type" {
			continue
		}

		// Ignore hint fields, they only show up in the scoring report
		if vType.Field(i).Name == "Hint" {
			continue
		}

		required := false
		for _, a := range args {
			if vType.Field(i).Name == a {
				required = true
				break
			}
		}

		if required {
			if v.Field(i).String() == "" {
				fail(c.Type+":", "missing required argument '"+vType.Field(i).Name+"'")
			}
		} else if v.Field(i).String() != "" {
			warn(c.Type+":", "specifying unused argument '"+vType.Field(i).Name+"'")
		}
	}
}

func (c cond) String() string {
	output := ""
	v := reflect.ValueOf(c)
	typeOfS := v.Type()

	for i := 0; i < v.NumField(); i++ {
		if v.Field(i).Kind() != reflect.String {
			continue
		}
		if v.Field(i).String() == "" {
			continue
		}
		output += fmt.Sprintf("\t%s: %v\n", typeOfS.Field(i).Name, v.Field(i).String())
	}
	return output
}

// runCheck executes a single condition check.
func runCheck(cond cond) (passed bool) {
	if err := deobfuscateCond(&cond); err != nil {
		fail(err.Error())
		return false
	}
	defer obfuscateCond(&cond)
	debug("Running condition:\n", cond)

	parsed, err := parseConditionType(cond.Type)
	if err != nil {
		fail(err.Error())
		return false
	}
	if err := validateConditionCapability(runtime.GOOS, parsed); err != nil {
		fail(fmt.Sprintf("Condition type %q is unavailable on GOOS %s: %v", cond.Type, runtime.GOOS, err))
		return false
	}
	cond.regex = parsed.Regex

	methodName := parsed.Base
	originalType := cond.Type
	defer func() {
		if recovered := recover(); recovered != nil {
			fail(fmt.Sprintf("Condition type %q failed on GOOS %s before calling %s: %v", originalType, runtime.GOOS, methodName, recovered))
			passed = false
		}
	}()

	vals := reflect.ValueOf(cond).MethodByName(parsed.Base).Call(nil)
	result := vals[0].Bool()
	conditionErr := vals[1]

	if parsed.Negated {
		debug("Result is", !result, "(was", result, "before negation) and error is", conditionErr)
		return conditionErr.IsNil() && !result
	}

	debug("Result is", result, "and error is", conditionErr)
	if verboseEnabled && !conditionErr.IsNil() {
		warn(parsed.Base, "returned an error:", conditionErr)
	}
	return conditionErr.IsNil() && result
}

// CommandContains checks if a given shell command contains a certain string.
// This check will always fail if the command returns an error.
func (c cond) CommandContains() (bool, error) {
	c.requireArgs("Cmd", "Value")
	out, err := shellCommandOutput(c.Cmd)
	if err != nil {
		return false, err
	}
	if c.regex {
		outTrim := strings.TrimSpace(out)
		return regexp.Match(c.Value, []byte(outTrim))
	}
	return strings.Contains(strings.TrimSpace(out), c.Value), err
}

// CommandOutput checks if a given shell command produces an exact output.
// This check will always fail if the command returns an error.
func (c cond) CommandOutput() (bool, error) {
	c.requireArgs("Cmd", "Value")
	out, err := shellCommandOutput(c.Cmd)
	return strings.TrimSpace(out) == c.Value, err
}

// DirContains returns true if any file in the directory contains the string value provided.
func (c cond) DirContains() (bool, error) {
	c.requireArgs("Path", "Value")
	result, err := cond{
		Path: c.Path,
	}.PathExists()
	if err != nil {
		return false, err
	}
	if !result {
		return false, errors.New("path does not exist")
	}

	var files []string
	err = filepath.Walk(c.Path, func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() {
			files = append(files, path)
		}
		if len(files) > 10000 {
			return errors.New("attempted to index too many files in recursive search")
		}
		return nil
	})

	if err != nil {
		return false, err
	}

	for _, file := range files {
		c.Path = file
		result, err := c.FileContains()
		if err != nil {
			return false, err
		}
		if result {
			return result, nil
		}
	}
	return false, nil
}

// FileContains determines whether a file contains a given regular expression.
//
// Newlines in regex may not work as expected, especially on Windows. It's
// best to not use these (ex. ^ and $).
func (c cond) FileContains() (bool, error) {
	c.requireArgs("Path", "Value")
	fileContent, err := readFile(c.Path)
	if err != nil {
		return false, err
	}
	found := false
	for _, line := range strings.Split(fileContent, "\n") {
		if c.regex {
			found, err = regexp.Match(c.Value, []byte(line))
			if err != nil {
				return false, err
			}
		} else {
			found = strings.Contains(line, c.Value)
		}
		if found {
			break
		}
	}
	return found, err
}

// FileEquals calculates the SHA256 sum of a file and compares it with the hash
// provided in the check.
func (c cond) FileEquals() (bool, error) {
	c.requireArgs("Path", "Value")
	fileContent, err := readFile(c.Path)
	if err != nil {
		return false, err
	}
	hasher := sha256.New()
	_, err = hasher.Write([]byte(fileContent))
	if err != nil {
		return false, err
	}
	hash := hex.EncodeToString(hasher.Sum(nil))
	return hash == c.Value, nil
}

// PathExists is a wrapper around os.Stat and os.IsNotExist, and determines
// whether a file or folder exists.
func (c cond) PathExists() (bool, error) {
	c.requireArgs("Path")
	_, err := os.Stat(c.Path)
	if err != nil && os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return true, nil
}
