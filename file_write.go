package main

import "os"

func writeFile(fileName, fileContent string) {
	if err := os.WriteFile(fileName, []byte(fileContent), 0o644); err != nil {
		fail("Error writing file: " + err.Error())
	}
}
