package main

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestCommand(t *testing.T) {
	c := cond{
		Cmd: "echo 'hello, world!'",
	}

	// Should pass: command ran
	out, err := c.Command()
	if err != nil || out != true {
		t.Error(c, "failed:", out, err)
	}

	// Should fail: command return false
	c.Cmd = "commanddoesntexist"
	out, err = c.Command()
	if err != nil || out != false {
		t.Error(c, "failed:", out, err)
	}

	// Should fail: command return false
	c.Cmd = "cat /etc/file/doesnt/exist"
	out, err = c.Command()
	if err != nil || out != false {
		t.Error(c, "failed:", out, err)
	}
}

func TestTrustedCommandFamilyCharacterization(t *testing.T) {
	tests := []struct {
		name string
		cond cond
		call func(cond) (bool, error)
		want bool
	}{
		{"command success", cond{Cmd: "exit 0"}, func(c cond) (bool, error) { return c.Command() }, true},
		{"command nonzero", cond{Cmd: "exit 7"}, func(c cond) (bool, error) { return c.Command() }, false},
		{"contains match", cond{Cmd: "printf 'alpha beta\\n'", Value: "ha be"}, func(c cond) (bool, error) { return c.CommandContains() }, true},
		{"contains successful nonmatch", cond{Cmd: "printf 'alpha\\n'", Value: "omega"}, func(c cond) (bool, error) { return c.CommandContains() }, false},
		{"output trims", cond{Cmd: "printf 'value\\n'", Value: "value"}, func(c cond) (bool, error) { return c.CommandOutput() }, true},
		{"output mismatch", cond{Cmd: "printf 'value\\n'", Value: "other"}, func(c cond) (bool, error) { return c.CommandOutput() }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.call(tt.cond)
			if err != nil || got != tt.want {
				t.Fatalf("got (%v, %v), want (%v, nil)", got, err, tt.want)
			}
		})
	}

	for _, c := range []cond{
		{Cmd: "printf anything; exit 9", Value: "anything"},
		{Cmd: "kill -TERM $$", Value: "anything"},
	} {
		if got, err := c.CommandContains(); err == nil || got {
			t.Fatalf("CommandContains operational failure got (%v, %v), want (false, error)", got, err)
		}
		if got, err := c.CommandOutput(); err == nil || got {
			t.Fatalf("CommandOutput operational failure got (%v, %v), want (false, error)", got, err)
		}
	}
}

func TestLinuxBuiltInCommandCharacterization(t *testing.T) {
	query := exec.Command("/usr/bin/dpkg-query", "-W", "-f=${Version}", "dash")
	version, err := query.Output()
	if err != nil {
		t.Skipf("dpkg-query dash fixture unavailable: %v", err)
	}

	installed, err := cond{Name: "dash"}.ProgramInstalled()
	if err != nil || !installed {
		t.Fatalf("installed package got (%v, %v), want (true, nil)", installed, err)
	}
	missing, err := cond{Name: "aeacus-characterization-package-does-not-exist"}.ProgramInstalled()
	if err != nil || missing {
		t.Fatalf("missing package got (%v, %v), want (false, nil)", missing, err)
	}
	matched, err := cond{Name: "dash", Value: strings.TrimSpace(string(version))}.ProgramVersion()
	if err != nil || !matched {
		t.Fatalf("package version got (%v, %v), want (true, nil)", matched, err)
	}
	service := "aeacus-characterization-service-does-not-exist"
	probe := exec.Command("/usr/bin/systemctl", "is-active", service)
	probeOutput, probeErr := probe.Output()
	stopped, err := cond{Name: service}.ServiceUp()
	var probeExit *exec.ExitError
	if errors.As(probeErr, &probeExit) && (probeExit.ExitCode() == 3 || probeExit.ExitCode() == 4) &&
		(strings.TrimSpace(string(probeOutput)) == "inactive" || strings.TrimSpace(string(probeOutput)) == "unknown") {
		if err != nil || stopped {
			t.Fatalf("absent service got (%v, %v), want (false, nil)", stopped, err)
		}
	} else if err == nil || stopped {
		t.Fatalf("indeterminate service manager got (%v, %v), want (false, error)", stopped, err)
	}
}

func TestTrustedCommandFamilyOperationalFailureContract(t *testing.T) {
	previousRunner := unixRunner
	t.Cleanup(func() { unixRunner = previousRunner })
	unixRunner = execRunner{
		clock: newFakeProcessClock(),
		starter: &fakeProcessStarter{
			startError: os.ErrPermission,
			started:    make(chan processInvocation, 1),
		},
	}
	if got, err := (cond{Cmd: "ignored"}).Command(); err != nil || got {
		t.Fatalf("Command start failure got (%v, %v), want (false, nil)", got, err)
	}
	if got, err := (cond{Cmd: "ignored", Value: "x"}).CommandContains(); err == nil || got {
		t.Fatalf("CommandContains start failure got (%v, %v), want (false, error)", got, err)
	}
	if got, err := (cond{Cmd: "ignored", Value: "x"}).CommandOutput(); err == nil || got {
		t.Fatalf("CommandOutput start failure got (%v, %v), want (false, error)", got, err)
	}
	unixRunner = previousRunner
	if got, err := (cond{Cmd: "printf x", Value: "[", regex: true}).CommandContains(); err == nil || got {
		t.Fatalf("CommandContains invalid regex got (%v, %v), want (false, error)", got, err)
	}
}

func TestCommandMapsTimeoutToFalseWithoutError(t *testing.T) {
	previousRunner := unixRunner
	t.Cleanup(func() { unixRunner = previousRunner })
	runner, clock, starter, group := newFakeExecRunner()
	unixRunner = runner
	done := make(chan struct {
		result bool
		err    error
	}, 1)
	go func() {
		result, err := (cond{Cmd: "ignored"}).Command()
		done <- struct {
			result bool
			err    error
		}{result: result, err: err}
	}()
	invocation := <-starter.started
	if invocation.path != "/bin/sh" || len(invocation.args) != 2 || invocation.args[0] != "-c" || invocation.args[1] != "ignored" {
		t.Fatalf("trusted shell invocation got %#v", invocation)
	}
	timeout := <-clock.timers
	if timeout.duration != 30*time.Second {
		t.Fatalf("trusted timeout got %v", timeout.duration)
	}
	timeout.fire()
	<-group.signals
	<-clock.timers
	starter.process.wait <- nil
	outcome := <-done
	if outcome.err != nil || outcome.result {
		t.Fatalf("timeout got (%v, %v), want (false, nil)", outcome.result, outcome.err)
	}
}
