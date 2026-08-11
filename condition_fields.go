package main

import "fmt"

type conditionField uint8

const (
	conditionFieldPath conditionField = iota
	conditionFieldCmd
	conditionFieldUser
	conditionFieldGroup
	conditionFieldName
	conditionFieldKey
	conditionFieldValue
	conditionFieldAfter
)

var conditionFields = [...]conditionField{
	conditionFieldPath,
	conditionFieldCmd,
	conditionFieldUser,
	conditionFieldGroup,
	conditionFieldName,
	conditionFieldKey,
	conditionFieldValue,
	conditionFieldAfter,
}

type conditionFieldSpec struct {
	Required []conditionField
	Optional []conditionField
}

type conditionFieldIssue uint8

const (
	conditionFieldRequired conditionFieldIssue = iota
	conditionFieldUnused
	conditionFieldUnknown
)

type conditionFieldError struct {
	Field string
	Issue conditionFieldIssue
}

func (e *conditionFieldError) Error() string {
	return fmt.Sprintf("condition field %q is %s", e.Field, e.Issue)
}

func validateConditionFields(goos, method string, condition *cond) error {
	platformSpecs, knownGOOS := conditionFieldSpecsByGOOS[goos]
	if !knownGOOS {
		return fmt.Errorf("condition field specification is unavailable for GOOS %s", goos)
	}
	spec, knownMethod := platformSpecs[method]
	if !knownMethod {
		return fmt.Errorf("condition field specification is unavailable for method %q", method)
	}
	for _, field := range spec.Required {
		if condition.fieldValue(field) == "" {
			return &conditionFieldError{Field: field.String(), Issue: conditionFieldRequired}
		}
	}
	for _, field := range conditionFields {
		if condition.fieldValue(field) != "" && !spec.allows(field) {
			return &conditionFieldError{Field: field.String(), Issue: conditionFieldUnused}
		}
	}
	if len(condition.unknownFields) > 0 {
		return &conditionFieldError{Field: condition.unknownFields[0], Issue: conditionFieldUnknown}
	}
	return nil
}

func (issue conditionFieldIssue) String() string {
	switch issue {
	case conditionFieldRequired:
		return "required"
	case conditionFieldUnused:
		return "unused"
	case conditionFieldUnknown:
		return "unknown"
	}
	return "invalid"
}

func (spec conditionFieldSpec) allows(field conditionField) bool {
	for _, required := range spec.Required {
		if required == field {
			return true
		}
	}
	for _, optional := range spec.Optional {
		if optional == field {
			return true
		}
	}
	return false
}

func (c cond) fieldValue(field conditionField) string {
	switch field {
	case conditionFieldPath:
		return c.Path
	case conditionFieldCmd:
		return c.Cmd
	case conditionFieldUser:
		return c.User
	case conditionFieldGroup:
		return c.Group
	case conditionFieldName:
		return c.Name
	case conditionFieldKey:
		return c.Key
	case conditionFieldValue:
		return c.Value
	case conditionFieldAfter:
		return c.After
	}
	return ""
}

func (field conditionField) String() string {
	switch field {
	case conditionFieldPath:
		return "Path"
	case conditionFieldCmd:
		return "Cmd"
	case conditionFieldUser:
		return "User"
	case conditionFieldGroup:
		return "Group"
	case conditionFieldName:
		return "Name"
	case conditionFieldKey:
		return "Key"
	case conditionFieldValue:
		return "Value"
	case conditionFieldAfter:
		return "After"
	}
	return "Unknown"
}
