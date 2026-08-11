package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"syscall"
	"testing"
)

const cliHelperEnvironment = "AEACUS_PRODUCTION_CLI_HELPER"

func TestProductionCLIHelperProcess(t *testing.T) {
	if os.Getenv(cliHelperEnvironment) != "1" {
		return
	}
	if os.Geteuid() == 0 {
		if err := syscall.Setgroups([]int{}); err != nil {
			t.Fatal(err)
		}
		if err := syscall.Setgid(65534); err != nil {
			t.Fatal(err)
		}
		if err := syscall.Setuid(65534); err != nil {
			t.Fatal(err)
		}
	}
	var arguments []string
	if err := json.Unmarshal([]byte(os.Getenv("AEACUS_PRODUCTION_CLI_ARGS")), &arguments); err != nil {
		t.Fatal(err)
	}
	os.Args = append([]string{"aeacus"}, arguments...)
	main()
}

func runProductionCLI(t *testing.T, arguments ...string) ([]byte, error) {
	t.Helper()
	payload, err := json.Marshal(arguments)
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(os.Args[0], "-test.run=^TestProductionCLIHelperProcess$")
	command.Env = append(os.Environ(), cliHelperEnvironment+"=1", "AEACUS_PRODUCTION_CLI_ARGS="+string(payload))
	return command.CombinedOutput()
}

func writeCLIFixture(t *testing.T, config string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, scoringConf), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	return root + string(os.PathSeparator)
}

func directoryInventory(t *testing.T, root string) []string {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	inventory := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}
		inventory = append(inventory, name)
	}
	slices.Sort(inventory)
	return inventory
}
