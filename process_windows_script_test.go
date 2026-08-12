//go:build windows && !phocus

package main

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

func TestPowerShellReleaseScriptPropagatesStatementAndNativeFailures(t *testing.T) {
	script := windowsReleaseScript("Write-Output 'stage'")

	for _, required := range []string{"trap { exit 1 }", "$ErrorActionPreference = 'Stop'", "$global:LASTEXITCODE = 0", "$nativeExitCode = $LASTEXITCODE", "if ($nativeExitCode -ne 0)", "exit $nativeExitCode"} {
		if !strings.Contains(script, required) {
			t.Fatalf("script = %q, want %q", script, required)
		}
	}
}

func TestGlobalWindowsShellHelpersDoNotUseReleaseWrapper(t *testing.T) {
	source, err := os.ReadFile("process_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, helper := range []string{"shellCommand", "shellCommandOutput"} {
		pattern := regexp.MustCompile(`(?s)func ` + helper + `\([^}]+\}`)
		function := pattern.FindString(string(source))
		if function == "" {
			t.Fatalf("%s source not found", helper)
		}
		if strings.Contains(function, "windowsReleaseScript") {
			t.Fatalf("%s still invokes strict release wrapper", helper)
		}
	}
}

func TestNonReleaseWindowsCallersDoNotUseReleaseRunner(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if strings.HasPrefix(file, "release_") || file == "process_windows_script_test.go" {
			continue
		}
		source, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(source), "releaseCommandRunner") || strings.Contains(string(source), "windowsReleaseScript") {
			t.Fatalf("non-release source %s references release-only command handling", file)
		}
	}
}

func TestWindowsReleaseCommandsContainAtMostOneNativeExecutable(t *testing.T) {
	nativeExecutable := regexp.MustCompile(`(?i)\b(?:sc|net|secedit|powershell|cmd|reg|shutdown|wevtutil|takeown|icacls|wmic|rundll32|msiexec)\.exe\b`)
	for commandIndex, command := range captureWindowsReleaseCommands(t) {
		if matches := nativeExecutable.FindAllString(command, -1); len(matches) > 1 {
			t.Fatalf("release command %d contains multiple native executables: %v", commandIndex+1, matches)
		}
	}
}

func TestWindowsServiceNativeCommandsUseSeparateReleaseBoundaries(t *testing.T) {
	commands := captureWindowsReleaseCommands(t)
	var serviceCommands []string
	for _, command := range commands {
		if strings.HasPrefix(strings.TrimSpace(command), "sc.exe ") {
			serviceCommands = append(serviceCommands, command)
		}
	}
	want := []string{
		`sc.exe create CSSClient binPath= "C:\aeacus\phocus.exe" start= "auto" DisplayName= "CSSClient"`,
		`sc.exe description CSSClient "This is Aeacus's Competition Scoring System client. Don't stop or mess with this unless you want to not get points, and maybe have your registry deleted."`,
	}
	if !reflect.DeepEqual(serviceCommands, want) {
		t.Fatalf("service commands = %v, want %v", serviceCommands, want)
	}
}

func TestGlobalWindowsRawCommandPreservesUnwrappedScript(t *testing.T) {
	command := rawCmd("Write-Output 'condition'")
	script := command.Args[len(command.Args)-1]

	if script != "{ Write-Output 'condition' }" {
		t.Fatalf("script = %q", script)
	}
	if strings.Contains(script, "$ErrorActionPreference") || strings.Contains(script, "$LASTEXITCODE") {
		t.Fatalf("global command uses strict release handling: %q", script)
	}
}

func TestWindowsReleaseRunnerUsesStrictScript(t *testing.T) {
	command := releasePowerShellCommand("Write-Output 'release'")
	script := command.Args[len(command.Args)-1]

	if script != "{ "+windowsReleaseScript("Write-Output 'release'")+" }" {
		t.Fatalf("script = %q", script)
	}
}

func captureWindowsReleaseCommands(t *testing.T) []string {
	t.Helper()
	oldRunner := releaseCommandRunner
	oldConfig := conf
	oldYes := yesEnabled
	t.Cleanup(func() {
		releaseCommandRunner = oldRunner
		conf = oldConfig
		yesEnabled = oldYes
	})
	var commands []string
	releaseCommandRunner = func(command string) error {
		commands = append(commands, command)
		return nil
	}
	conf = &config{User: "release-user"}
	yesEnabled = true
	for _, hook := range []func() error{writeDesktopFiles, configureAutologin, installFont, installService, cleanUp} {
		if err := hook(); err != nil {
			t.Fatal(err)
		}
	}
	return commands
}
