//go:build linux || freebsd

package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

const processHelperEnvironment = "AEACUS_PROCESS_HELPER"

func helperArgs(mode string, args ...string) []string {
	result := []string{"-test.run=^TestProcessHelper$", "--", mode}
	return append(result, args...)
}

func TestProcessHelper(t *testing.T) {
	if os.Getenv(processHelperEnvironment) != "1" {
		return
	}
	separator := 0
	for i, arg := range os.Args {
		if arg == "--" {
			separator = i
			break
		}
	}
	args := os.Args[separator+1:]
	if len(args) == 0 {
		os.Exit(2)
	}
	switch args[0] {
	case "stdout", "stderr":
		size, err := strconv.Atoi(args[1])
		if err != nil {
			os.Exit(2)
		}
		writer := os.Stdout
		if args[0] == "stderr" {
			writer = os.Stderr
		}
		if _, err := writer.Write([]byte(strings.Repeat("x", size))); err != nil {
			os.Exit(2)
		}
	case "exit":
		code, err := strconv.Atoi(args[1])
		if err != nil {
			os.Exit(2)
		}
		os.Exit(code)
	case "signal":
		if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
			os.Exit(2)
		}
		select {}
	case "term":
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGTERM)
		if err := notifyProcessHelper(args[1], "ready"); err != nil {
			os.Exit(2)
		}
		<-ch
	case "ignore-term":
		signal.Ignore(syscall.SIGTERM)
		if err := notifyProcessHelper(args[1], "ready"); err != nil {
			os.Exit(2)
		}
		select {}
	case "descendant":
		child := exec.Command("/bin/sleep", "60")
		if err := child.Start(); err != nil {
			os.Exit(2)
		}
		if _, err := fmt.Fprintln(os.Stdout, child.Process.Pid); err != nil {
			os.Exit(2)
		}
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGTERM)
		if err := notifyProcessHelper(args[1], strconv.Itoa(child.Process.Pid)); err != nil {
			os.Exit(2)
		}
		<-ch
		if err := child.Wait(); err == nil {
			os.Exit(3)
		}
	case "leader-exits-on-term", "leader-exits-descendant-holds-pipes":
		ch := make(chan os.Signal, 1)
		descendantStarted := make(chan os.Signal, 1)
		signal.Notify(descendantStarted, syscall.SIGUSR1)
		if args[0] == "leader-exits-on-term" {
			signal.Notify(ch, syscall.SIGTERM)
		}
		child := exec.Command(os.Args[0], helperArgs("orphan-ignore-term", args[1])...)
		child.Env = append(os.Environ(), processHelperEnvironment+"=1")
		if args[0] == "leader-exits-descendant-holds-pipes" {
			child.Stdout = os.Stdout
			child.Stderr = os.Stderr
		}
		if err := child.Start(); err != nil {
			os.Exit(2)
		}
		<-descendantStarted
		if err := notifyProcessHelper(args[1], strconv.Itoa(child.Process.Pid), "descendant-ready"); err != nil {
			os.Exit(2)
		}
		if args[0] == "leader-exits-on-term" {
			<-ch
		}
	case "orphan-ignore-term":
		ignoreTERM := make(chan os.Signal, 1)
		signal.Notify(ignoreTERM, syscall.SIGTERM)
		if err := syscall.Kill(os.Getppid(), syscall.SIGUSR1); err != nil {
			os.Exit(2)
		}
		if _, err := fmt.Fprint(os.Stdout, "holding stdout"); err != nil {
			os.Exit(2)
		}
		if _, err := fmt.Fprint(os.Stderr, "holding stderr"); err != nil {
			os.Exit(2)
		}
		select {}
	case "orphan-default-term":
		if err := notifyProcessHelper(args[1], "descendant-ready"); err != nil {
			os.Exit(2)
		}
		if err := syscall.Kill(os.Getppid(), syscall.SIGUSR1); err != nil {
			os.Exit(2)
		}
		select {}
	default:
		os.Exit(2)
	}
	os.Exit(0)
}

func notifyProcessHelper(path string, messages ...string) error {
	connection, err := net.DialUnix("unixgram", nil, &net.UnixAddr{Name: path, Net: "unixgram"})
	if err != nil {
		return err
	}
	for _, message := range messages {
		if _, err := fmt.Fprint(connection, message); err != nil {
			return errors.Join(err, connection.Close())
		}
	}
	return connection.Close()
}
