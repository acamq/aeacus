package main

import (
	"fmt"
	"sort"
	"strings"
)

type conditionFieldDecodeError struct {
	Type      string
	Field     string
	ValueType string
}

func (e *conditionFieldDecodeError) Error() string {
	return fmt.Sprintf("condition type %q field %q must be a string, got %s", e.Type, e.Field, e.ValueType)
}

func (c *cond) UnmarshalTOML(value interface{}) error {
	table, ok := value.(map[string]interface{})
	if !ok {
		return fmt.Errorf("condition must be a TOML table, got %T", value)
	}

	keys := make([]string, 0, len(table))
	for key := range table {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(left, right int) bool {
		leftCanonical, _ := canonicalConditionField(keys[left])
		rightCanonical, _ := canonicalConditionField(keys[right])
		if leftCanonical != rightCanonical {
			return leftCanonical < rightCanonical
		}
		return keys[left] < keys[right]
	})

	typeSeen := false
	for _, key := range keys {
		canonical, _ := canonicalConditionField(key)
		if canonical != "Type" {
			continue
		}
		if typeSeen {
			return fmt.Errorf("condition type %q repeats field %q", c.Type, canonical)
		}
		typeSeen = true
		raw := table[key]
		text, ok := raw.(string)
		if !ok {
			return &conditionFieldDecodeError{Field: canonical, ValueType: fmt.Sprintf("%T", raw)}
		}
		c.Type = text
	}

	seen := make(map[string]struct{}, len(table))
	for _, key := range keys {
		canonical, known := canonicalConditionField(key)
		if canonical == "Type" {
			continue
		}
		if _, duplicate := seen[canonical]; duplicate {
			return fmt.Errorf("condition type %q repeats field %q", c.Type, canonical)
		}
		seen[canonical] = struct{}{}

		raw := table[key]
		text, isString := raw.(string)
		if known && !isString {
			return &conditionFieldDecodeError{Type: c.Type, Field: canonical, ValueType: fmt.Sprintf("%T", raw)}
		}
		switch canonical {
		case "Hint":
			c.Hint = text
		case "Path":
			c.Path = text
		case "Cmd":
			c.Cmd = text
		case "User":
			c.User = text
		case "Group":
			c.Group = text
		case "Name":
			c.Name = text
		case "Key":
			c.Key = text
		case "Value":
			c.Value = text
		case "After":
			c.After = text
		default:
			c.unknownFields = append(c.unknownFields, canonical)
		}
	}
	sort.Strings(c.unknownFields)
	return nil
}

func canonicalConditionField(field string) (string, bool) {
	switch strings.ToLower(field) {
	case "type":
		return "Type", true
	case "hint":
		return "Hint", true
	case "path":
		return "Path", true
	case "cmd":
		return "Cmd", true
	case "user":
		return "User", true
	case "group":
		return "Group", true
	case "name":
		return "Name", true
	case "key":
		return "Key", true
	case "value":
		return "Value", true
	case "after":
		return "After", true
	default:
		return strings.ToLower(field), false
	}
}
