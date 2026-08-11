//go:build freebsd

package main

func resolveRuntimeCapabilities(candidate config) (config, error) {
	return resolveCapabilities(candidate, "freebsd")
}
