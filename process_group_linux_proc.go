//go:build linux

package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type linuxProcessEntry struct {
	pid   int
	group int
	state byte
}

func readLinuxProcesses() ([]linuxProcessEntry, error) {
	directories, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	entries := make([]linuxProcessEntry, 0, len(directories))
	for _, directory := range directories {
		pid, err := strconv.Atoi(directory.Name())
		if err != nil || !directory.IsDir() {
			continue
		}
		entry, err := readLinuxProcess(pid)
		if err == nil {
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

func readLinuxProcess(pid int) (linuxProcessEntry, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return linuxProcessEntry{}, err
	}
	closing := strings.LastIndexByte(string(data), ')')
	if closing < 0 {
		return linuxProcessEntry{}, fmt.Errorf("malformed process stat for %d", pid)
	}
	fields := strings.Fields(string(data[closing+1:]))
	if len(fields) < 3 || len(fields[0]) != 1 {
		return linuxProcessEntry{}, fmt.Errorf("malformed process stat for %d", pid)
	}
	group, err := strconv.Atoi(fields[2])
	if err != nil {
		return linuxProcessEntry{}, err
	}
	return linuxProcessEntry{pid: pid, group: group, state: fields[0][0]}, nil
}
