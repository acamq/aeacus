//go:build windows

package main

func resolveRuntimeCapabilities(candidate config) (config, error) {
	return resolveCapabilities(candidate, "windows")
}
