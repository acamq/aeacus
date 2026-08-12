//go:build windows && !phocus

package main

import (
	"strings"
	"testing"
)

func TestPowerShellReleaseScriptPropagatesStatementAndNativeFailures(t *testing.T) {
	script := powerShellReleaseScript("Write-Output 'stage'")

	for _, required := range []string{"trap { exit 1 }", "$ErrorActionPreference = 'Stop'", "$global:LASTEXITCODE = 0", "$nativeExitCode = $LASTEXITCODE", "if ($nativeExitCode -ne 0)", "exit $nativeExitCode"} {
		if !strings.Contains(script, required) {
			t.Fatalf("script = %q, want %q", script, required)
		}
	}
}
