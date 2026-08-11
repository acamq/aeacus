package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	root := flag.String("root", ".", "repository root")
	contractPath := flag.String("contract", "misc/tests/contracts/condition-availability-v1.json", "contract output path")
	goPath := flag.String("go-output", "condition_availability_generated.go", "Go output path")
	flag.Parse()

	availability, err := discoverAvailability(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := writeFile(filepath.Join(*root, *contractPath), renderContract(availability)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := writeFile(filepath.Join(*root, *goPath), renderGo(availability)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func writeFile(path string, data []byte) error {
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
