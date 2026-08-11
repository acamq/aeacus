package main

import "fmt"

type CapabilityError struct {
	Field string
	Value string
	GOOS  string
}

func (e *CapabilityError) Error() string {
	return fmt.Sprintf("capability field %s value %q is invalid for GOOS %s", e.Field, e.Value, e.GOOS)
}

func resolveCapabilities(candidate config, goos string) (config, error) {
	switch goos {
	case "linux", "windows", "freebsd":
	default:
		return config{}, capabilityError("GOOS", goos, goos)
	}

	if candidate.Platform != "" && candidate.Platform != goos {
		return config{}, capabilityError("platform", candidate.Platform, goos)
	}

	switch goos {
	case "linux", "windows":
		if candidate.Desktop != "" {
			return config{}, capabilityError("desktop", candidate.Desktop, goos)
		}
		if candidate.Autologin != "" {
			return config{}, capabilityError("autologin", candidate.Autologin, goos)
		}
	case "freebsd":
		if candidate.Shell {
			return config{}, capabilityError("shell", "true", goos)
		}
		if candidate.Desktop == "" {
			candidate.Desktop = "headless"
		}
		switch candidate.Desktop {
		case "headless", "xfce":
		default:
			return config{}, capabilityError("desktop", candidate.Desktop, goos)
		}
		if candidate.Autologin == "" {
			candidate.Autologin = "none"
		}
		switch candidate.Autologin {
		case "none", "console", "lightdm":
		default:
			return config{}, capabilityError("autologin", candidate.Autologin, goos)
		}
		// Task 20 owns provisioning pair policy; this layer validates each enum only.
	}

	return candidate, nil
}

func capabilityError(field, value, goos string) *CapabilityError {
	return &CapabilityError{Field: field, Value: value, GOOS: goos}
}
