package main

import (
	"fmt"
	"strings"
)

const (
	conditionNotSuffix   = "Not"
	conditionRegexSuffix = "Regex"
)

type parsedConditionType struct {
	Base    string
	Negated bool
	Regex   bool
}

type conditionTypeError struct {
	Type   string
	Reason string
}

func (e *conditionTypeError) Error() string {
	return fmt.Sprintf("invalid condition type %q: %s", e.Type, e.Reason)
}

func parseConditionType(typeName string) (parsedConditionType, error) {
	parsed := parsedConditionType{Base: typeName}
	if len(typeName) <= len(conditionRegexSuffix) {
		return parsedConditionType{}, newConditionTypeError(typeName, "empty or too short")
	}

	if strings.HasSuffix(parsed.Base, conditionNotSuffix) {
		parsed.Negated = true
		parsed.Base = strings.TrimSuffix(parsed.Base, conditionNotSuffix)
		if strings.HasSuffix(parsed.Base, conditionNotSuffix) {
			return parsedConditionType{}, newConditionTypeError(typeName, "repeated Not suffix")
		}
	}

	if strings.HasSuffix(parsed.Base, conditionRegexSuffix) {
		parsed.Regex = true
		parsed.Base = strings.TrimSuffix(parsed.Base, conditionRegexSuffix)
		if strings.HasSuffix(parsed.Base, conditionRegexSuffix) {
			return parsedConditionType{}, newConditionTypeError(typeName, "repeated Regex suffix")
		}
	}

	if strings.HasSuffix(parsed.Base, conditionNotSuffix) {
		return parsedConditionType{}, newConditionTypeError(typeName, "Not must follow Regex")
	}
	if len(parsed.Base) <= len(conditionRegexSuffix) {
		return parsedConditionType{}, newConditionTypeError(typeName, "base name is too short")
	}
	if !validConditionIdentifier(parsed.Base) {
		return parsedConditionType{}, newConditionTypeError(typeName, "base name is not an exported Go identifier")
	}
	return parsed, nil
}

func newConditionTypeError(typeName, reason string) error {
	return &conditionTypeError{Type: typeName, Reason: reason}
}

func validConditionIdentifier(name string) bool {
	for index, char := range name {
		if index == 0 {
			if char < 'A' || char > 'Z' {
				return false
			}
			continue
		}
		if char != '_' && (char < 'A' || char > 'Z') && (char < 'a' || char > 'z') && (char < '0' || char > '9') {
			return false
		}
	}
	return true
}
