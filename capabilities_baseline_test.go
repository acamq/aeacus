package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"

	"github.com/BurntSushi/toml"
)

func TestLegacyExamplesRemainValidForTheirPlatforms(t *testing.T) {
	tests := []struct {
		name string
		path string
		goos string
	}{
		{name: "linux", path: "linux-remote.conf", goos: "linux"},
		{name: "windows", path: "windows.conf", goos: "windows"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: an existing platform example config.
			content, err := os.ReadFile(filepath.Join("docs", "examples", tt.path))
			if err != nil {
				t.Fatal(err)
			}
			candidate := &config{}
			if _, err := toml.Decode(string(content), candidate); err != nil {
				t.Fatalf("decode example: %v", err)
			}

			// When: its existing conditions and runtime capabilities are validated.
			err = validateConfigConditions(candidate, tt.goos)
			resolved, capabilityErr := resolveCapabilities(*candidate, tt.goos)

			// Then: the legacy example remains valid and byte-for-byte unchanged.
			if err != nil {
				t.Fatalf("validate example: %v", err)
			}
			if capabilityErr != nil {
				t.Fatalf("resolve example: %v", capabilityErr)
			}
			if !reflect.DeepEqual(resolved, *candidate) {
				t.Fatalf("legacy example changed: got %+v want %+v", resolved, *candidate)
			}
		})
	}
}

func TestCapabilityFieldsUseExactPublicTOMLNames(t *testing.T) {
	tests := []struct {
		field string
		name  string
	}{
		{field: "Platform", name: "platform"},
		{field: "Desktop", name: "desktop"},
		{field: "Autologin", name: "autologin"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: the public config type.
			field, ok := reflect.TypeOf(config{}).FieldByName(tt.field)
			if !ok {
				t.Fatalf("field %s not found", tt.field)
			}

			// When: its TOML name is inspected.
			got := field.Tag.Get("toml")

			// Then: the public key is exact and lowercase.
			if got != tt.name {
				t.Fatalf("TOML tag = %q, want %q", got, tt.name)
			}
		})
	}
}

func TestLegacyShellAndOSMetadataRemainUnrestricted(t *testing.T) {
	oldConf := conf
	t.Cleanup(func() { conf = oldConf })
	tests := []struct {
		name  string
		shell bool
		os    string
	}{
		{name: "shell enabled with FreeBSD display text", shell: true, os: " FreeBSD DISPLAY metadata "},
		{name: "shell disabled with arbitrary display text", shell: false, os: "not-a-runtime-selector"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: a legacy config using shell and arbitrary display-only OS metadata.
			content := "shell = " + strconv.FormatBool(tt.shell) + "\nos = '" + tt.os + "'\n"

			// When: the config is parsed on the current Linux runtime.
			err := parseConfig(content)

			// Then: shell is retained and OS is published byte-for-byte.
			if err != nil {
				t.Fatalf("parseConfig() error = %v", err)
			}
			if conf.Shell != tt.shell || conf.OS != tt.os {
				t.Fatalf("parsed shell/OS = %v/%q, want %v/%q", conf.Shell, conf.OS, tt.shell, tt.os)
			}
		})
	}
}
