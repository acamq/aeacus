package main

import (
	"bytes"
	"errors"
	"os/exec"
	"reflect"
	"runtime"
	"testing"
)

func TestCLIInvalidReleaseExitsNonzeroWithoutArtifacts(t *testing.T) {
	root := writeCLIFixture(t, "[[check]]\nmessage='invalid release'\npoints=10\n[[check.pass]]\ntype='PathExists'\n")
	before := directoryInventory(t, root)

	// When: the production CLI receives global flags before the invalid release command.
	output, err := runProductionCLI(t, "-y", "--dir", root, "release")

	// Then: validation exits nonzero before permission, confirmation, scoring, release, or cleanup mutations.
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() == 0 {
		t.Fatalf("release exit = %v, output:\n%s", err, output)
	}
	for _, expected := range []string{"PathExists", "Path", "GOOS " + runtime.GOOS} {
		if !bytes.Contains(output, []byte(expected)) {
			t.Errorf("output lacks %q:\n%s", expected, output)
		}
	}
	after := directoryInventory(t, root)
	want := []string{scoringConf}
	if !reflect.DeepEqual(before, want) || !reflect.DeepEqual(after, want) {
		t.Fatalf("inventory changed: before=%v after=%v want=%v", before, after, want)
	}
}
