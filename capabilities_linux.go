//go:build linux

package main

func resolveRuntimeCapabilities(candidate config) (config, error) {
	return resolveCapabilities(candidate, "linux")
}
