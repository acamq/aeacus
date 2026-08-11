package main

import (
	"fmt"
	"reflect"
	"runtime"
)

type conditionValidationError struct {
	Type      string
	GOOS      string
	Check     int
	List      string
	Condition int
	Cause     error
}

func (e *conditionValidationError) Error() string {
	return fmt.Sprintf(
		"condition type %q is invalid for GOOS %s at check %d %s condition %d: %v",
		e.Type,
		e.GOOS,
		e.Check,
		e.List,
		e.Condition,
		e.Cause,
	)
}

func (e *conditionValidationError) Unwrap() error {
	return e.Cause
}

func validateConfigConditions(candidate *config, goos string) error {
	for checkIndex := range candidate.Check {
		configuredCheck := &candidate.Check[checkIndex]
		lists := []struct {
			name       string
			conditions []cond
		}{
			{name: "Pass", conditions: configuredCheck.Pass},
			{name: "Fail", conditions: configuredCheck.Fail},
			{name: "PassOverride", conditions: configuredCheck.PassOverride},
		}
		for _, list := range lists {
			for conditionIndex := range list.conditions {
				condition := &list.conditions[conditionIndex]
				parsed, err := parseConditionType(condition.Type)
				if err == nil {
					err = validateConditionCapability(goos, parsed)
				}
				if err != nil {
					return &conditionValidationError{
						Type:      condition.Type,
						GOOS:      goos,
						Check:     checkIndex + 1,
						List:      list.name,
						Condition: conditionIndex + 1,
						Cause:     err,
					}
				}
			}
		}
	}
	return nil
}

func validateConditionCapability(goos string, parsed parsedConditionType) error {
	if !conditionMethodAvailable(goos, parsed.Base) {
		return fmt.Errorf("compiled method %q is unavailable", parsed.Base)
	}
	if parsed.Regex && !conditionRegexAvailable(goos, parsed.Base) {
		return fmt.Errorf("Regex suffix is unsupported for compiled method %q", parsed.Base)
	}
	return nil
}

func conditionMethodAvailable(goos, method string) bool {
	if !conditionMethodDeclaredForGOOS(goos, method) {
		return false
	}
	if goos != runtime.GOOS {
		return true
	}
	_, compiled := reflect.TypeOf(cond{}).MethodByName(method)
	return compiled
}

func conditionMethodDeclaredForGOOS(goos, method string) bool {
	methods, knownGOOS := conditionMethodsByGOOS[goos]
	if !knownGOOS {
		return false
	}
	_, declared := methods[method]
	return declared
}

func conditionRegexAvailable(goos, method string) bool {
	methods, knownGOOS := conditionRegexMethodsByGOOS[goos]
	if !knownGOOS {
		return false
	}
	_, available := methods[method]
	return available
}
