package main

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
)

type reflectionConditionFixture struct{}

func (reflectionConditionFixture) PublicZero() (bool, error) { return true, nil }
func (reflectionConditionFixture) helper() (bool, error)     { return true, nil }

func TestDiscoverMethodsFiltersToReflectionPublicConditions(t *testing.T) {
	root := t.TempDir()
	files := []struct {
		name    string
		content string
	}{
		{name: "conditions.go", content: `package fixture
type cond struct{}
type other struct{}
func (c cond) requireArgs(fields ...interface{}) {}
func (c cond) Common() (bool, error) { c.requireArgs("Path"); return true, nil }
func (c cond) PublicZero() (bool, error) { return true, nil }
func (c cond) helper() (bool, error) { return true, nil }
func (c other) WrongReceiver() (bool, error) { return true, nil }
func (c *cond) PointerReceiver() (bool, error) { return true, nil }
func (c cond) HasParameters(value string) (bool, error) { return true, nil }
func (c cond) WrongResultCount() bool { return true }
func (c cond) WrongFirstResult() (string, error) { return "", nil }
func (c cond) WrongSecondResult() (bool, string) { return true, "" }
`},
		{name: "conditions_linux.go", content: `package fixture
func (c cond) LinuxOnly() (bool, error) { c.requireArgs("Name"); return true, nil }
`},
		{name: "conditions_windows.go", content: `package fixture
func (c cond) WindowsOnly() (bool, error) { c.requireArgs("Key", "Value"); return true, nil }
`},
		{name: "conditions_test.go", content: `package fixture
func (c cond) TestOnly() (bool, error) { return true, nil }
`},
	}
	for _, fixture := range files {
		if err := os.WriteFile(filepath.Join(root, fixture.name), []byte(fixture.content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	// When: real GOOS-aware package selection and method discovery run.
	linux, linuxFields, err := discoverMethods(root, "linux")
	if err != nil {
		t.Fatal(err)
	}
	windows, windowsFields, err := discoverMethods(root, "windows")
	if err != nil {
		t.Fatal(err)
	}

	// Then: only exported value-receiver methods with the dispatch signature remain.
	if want := []string{"Common", "LinuxOnly", "PublicZero"}; !slices.Equal(linux, want) {
		t.Errorf("Linux methods = %v, want %v", linux, want)
	}
	if want := []string{"Common", "PublicZero", "WindowsOnly"}; !slices.Equal(windows, want) {
		t.Errorf("Windows methods = %v, want %v", windows, want)
	}
	wantLinuxFields := map[string][]string{"Common": {"Path"}, "LinuxOnly": {"Name"}, "PublicZero": {}}
	wantWindowsFields := map[string][]string{"Common": {"Path"}, "PublicZero": {}, "WindowsOnly": {"Key", "Value"}}
	if !reflect.DeepEqual(linuxFields, wantLinuxFields) {
		t.Errorf("Linux required fields = %v, want %v", linuxFields, wantLinuxFields)
	}
	if !reflect.DeepEqual(windowsFields, wantWindowsFields) {
		t.Errorf("Windows required fields = %v, want %v", windowsFields, wantWindowsFields)
	}
}

func TestExportedMatchingMethodDefinesPublicConditionSurface(t *testing.T) {
	// Given: exported and unexported methods with otherwise identical dispatch signatures.
	conditionType := reflect.TypeOf(reflectionConditionFixture{})

	// When/Then: reflection exposes the exported method and hides the helper.
	if _, exists := conditionType.MethodByName("PublicZero"); !exists {
		t.Fatal("exported matching method is absent from reflection's public method set")
	}
	if _, exists := conditionType.MethodByName("helper"); exists {
		t.Fatal("unexported helper is present in reflection's public method set")
	}
}
