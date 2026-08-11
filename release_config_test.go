package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseImageReturnsConfigValidationError(t *testing.T) {
	oldDir := dirPath
	t.Cleanup(func() { dirPath = oldDir })
	tempDir := t.TempDir()
	dirPath = tempDir + string(os.PathSeparator)
	invalid := "[[check]]\n[[check.pass]]\ntype='PathExists'\n"
	if err := os.WriteFile(filepath.Join(tempDir, scoringConf), []byte(invalid), 0o600); err != nil {
		t.Fatal(err)
	}

	// When: release starts with an invalid condition configuration.
	err := releaseImage()

	// Then: the configuration error is returned to the CLI boundary.
	if err == nil {
		t.Fatal("releaseImage() error = nil")
	}
	for _, expected := range []string{"PathExists", "Path"} {
		if !strings.Contains(err.Error(), expected) {
			t.Errorf("error %q does not contain %q", err, expected)
		}
	}
}
