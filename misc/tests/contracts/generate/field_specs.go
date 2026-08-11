package main

import (
	"bytes"
	"fmt"
)

type fieldSpec struct {
	required []string
	optional []string
}

type platformFieldSpecs struct {
	goos    string
	methods []string
	specs   map[string]fieldSpec
}

var freeBSDFieldSpecs = map[string]fieldSpec{
	"AccountLocked":                  {required: []string{"User"}},
	"BasePackageInstalled":           {required: []string{"Name"}},
	"BaseUpdateChecksEnabled":        {},
	"Command":                        {required: []string{"Cmd"}},
	"CommandContains":                {required: []string{"Cmd", "Value"}},
	"CommandOutput":                  {required: []string{"Cmd", "Value"}},
	"DirContains":                    {required: []string{"Path", "Value"}},
	"FileACL":                        {required: []string{"Path", "Value"}},
	"FileContains":                   {required: []string{"Path", "Value"}},
	"FileEquals":                     {required: []string{"Path", "Value"}},
	"FileFlagSet":                    {required: []string{"Name", "Path"}},
	"FileOwner":                      {required: []string{"Group", "Path", "User"}},
	"FirewallAvailable":              {optional: []string{"Name"}},
	"FirewallEnabled":                {optional: []string{"Name"}},
	"FirewallRulesLoaded":            {optional: []string{"Name"}},
	"FirewallUp":                     {optional: []string{"Name"}},
	"JailRunning":                    {required: []string{"Name"}},
	"KernelVersion":                  {required: []string{"Value"}},
	"PackageUpdateChecksEnabled":     {},
	"PasswordAuthenticationDisabled": {required: []string{"User"}},
	"PasswordChanged":                {required: []string{"User", "Value"}},
	"PathExists":                     {required: []string{"Path"}},
	"PermissionIs":                   {required: []string{"Path", "Value"}},
	"PkgAuditClean":                  {},
	"ProgramInstalled":               {required: []string{"Name"}},
	"ProgramVersion":                 {required: []string{"Name", "Value"}},
	"RunningInJail":                  {},
	"ServiceEnabled":                 {required: []string{"Name"}},
	"ServiceUp":                      {required: []string{"Name"}},
	"SysctlValue":                    {required: []string{"Name", "Value"}},
	"UserExists":                     {required: []string{"User"}},
	"UserInGroup":                    {required: []string{"Group", "User"}},
}

var conditionFieldIdentifiers = map[string]string{
	"After": "conditionFieldAfter",
	"Cmd":   "conditionFieldCmd",
	"Group": "conditionFieldGroup",
	"Key":   "conditionFieldKey",
	"Name":  "conditionFieldName",
	"Path":  "conditionFieldPath",
	"User":  "conditionFieldUser",
	"Value": "conditionFieldValue",
}

func writeConditionFieldSpecs(source *bytes.Buffer, contract availabilityContract) {
	source.WriteString("\nvar conditionFieldSpecsByGOOS = map[string]map[string]conditionFieldSpec{\n")
	writePlatformFieldSpecs(source, platformFieldSpecs{goos: "freebsd", methods: freeBSDConditionMethods, specs: freeBSDFieldSpecs})
	writePlatformFieldSpecs(source, platformFieldSpecs{goos: "linux", methods: contract.Linux, specs: requiredFieldSpecs(contract.linuxRequired)})
	writePlatformFieldSpecs(source, platformFieldSpecs{goos: "windows", methods: contract.Windows, specs: requiredFieldSpecs(contract.windowsRequired)})
	source.WriteString("}\n")
}

func requiredFieldSpecs(required map[string][]string) map[string]fieldSpec {
	specs := make(map[string]fieldSpec, len(required))
	for method, fields := range required {
		specs[method] = fieldSpec{required: fields}
	}
	return specs
}

func writePlatformFieldSpecs(source *bytes.Buffer, platform platformFieldSpecs) {
	fmt.Fprintf(source, "\t%q: {\n", platform.goos)
	for _, method := range platform.methods {
		spec, exists := platform.specs[method]
		if !exists {
			panic(fmt.Sprintf("missing %s field specification for %s", platform.goos, method))
		}
		fmt.Fprintf(source, "\t\t%q: {", method)
		writeFields(source, "Required", spec.required)
		writeFields(source, "Optional", spec.optional)
		source.WriteString("},\n")
	}
	source.WriteString("\t},\n")
}

func writeFields(source *bytes.Buffer, name string, fields []string) {
	if len(fields) == 0 {
		return
	}
	fmt.Fprintf(source, "%s: []conditionField{", name)
	for index, field := range fields {
		identifier, exists := conditionFieldIdentifiers[field]
		if !exists {
			panic(fmt.Sprintf("unknown condition field %q", field))
		}
		if index > 0 {
			source.WriteString(", ")
		}
		source.WriteString(identifier)
	}
	source.WriteString("}")
}
