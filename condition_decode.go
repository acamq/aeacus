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

	for key, raw := range table {
		if strings.EqualFold(key, "Type") {
			text, ok := raw.(string)
			if !ok {
				return &conditionFieldDecodeError{Field: key, ValueType: fmt.Sprintf("%T", raw)}
			}
			c.Type = text
		}
	}

	seen := make(map[string]struct{}, len(table))
	for key, raw := range table {
		normalized := strings.ToLower(key)
		if _, duplicate := seen[normalized]; duplicate {
			return fmt.Errorf("condition type %q repeats field %q", c.Type, key)
		}
		seen[normalized] = struct{}{}
		if normalized == "type" {
			continue
		}

		text, isString := raw.(string)
		switch normalized {
		case "hint", "path", "cmd", "user", "group", "name", "key", "value", "after":
			if !isString {
				return &conditionFieldDecodeError{Type: c.Type, Field: key, ValueType: fmt.Sprintf("%T", raw)}
			}
		}
		switch normalized {
		case "hint":
			c.Hint = text
		case "path":
			c.Path = text
		case "cmd":
			c.Cmd = text
		case "user":
			c.User = text
		case "group":
			c.Group = text
		case "name":
			c.Name = text
		case "key":
			c.Key = text
		case "value":
			c.Value = text
		case "after":
			c.After = text
		default:
			c.unknownFields = append(c.unknownFields, key)
		}
	}
	sort.Strings(c.unknownFields)
	return nil
}
