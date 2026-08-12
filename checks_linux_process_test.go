//go:build linux

package main

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
)

type recordedProcessCall struct {
	path    string
	args    []string
	profile processProfile
}

type scriptedProcessOutcome struct {
	result processResult
	err    error
}

type scriptedProcessRunner struct {
	calls    []recordedProcessCall
	outcomes []scriptedProcessOutcome
}

func (r *scriptedProcessRunner) Run(path string, args []string, profile processProfile) (processResult, error) {
	r.calls = append(r.calls, recordedProcessCall{path: path, args: append([]string(nil), args...), profile: profile})
	if len(r.outcomes) == 0 {
		return processResult{}, errors.New("unexpected process call")
	}
	outcome := r.outcomes[0]
	r.outcomes = r.outcomes[1:]
	return outcome.result, outcome.err
}

func processExit(path string, code int) error {
	return &processExitError{path: path, code: code}
}

func TestLinuxBuiltInsRejectUnsafeOperandsBeforeRunner(t *testing.T) {
	tests := []struct {
		name string
		call func(*scriptedProcessRunner) (bool, error)
	}{
		{"package shell syntax", func(r *scriptedProcessRunner) (bool, error) {
			return programInstalledWithRunner(cond{Name: "x; touch sentinel"}, r)
		}},
		{"package leading dash", func(r *scriptedProcessRunner) (bool, error) {
			return programVersionWithRunner(cond{Name: "-unsafe", Value: "1"}, r)
		}},
		{"service slash", func(r *scriptedProcessRunner) (bool, error) {
			return serviceUpWithRunner(cond{Name: "../unsafe"}, r)
		}},
		{"package over 128 bytes", func(r *scriptedProcessRunner) (bool, error) {
			return programInstalledWithRunner(cond{Name: strings.Repeat("p", 129)}, r)
		}},
		{"service over 64 bytes", func(r *scriptedProcessRunner) (bool, error) {
			return serviceUpWithRunner(cond{Name: strings.Repeat("s", 65)}, r)
		}},
		{"package NUL", func(r *scriptedProcessRunner) (bool, error) {
			return programInstalledWithRunner(cond{Name: "safe\x00name"}, r)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &scriptedProcessRunner{}
			got, err := tt.call(runner)
			var operandError *processOperandError
			if got || !errors.As(err, &operandError) {
				t.Fatalf("got (%v, %T), want (false, operand error)", got, err)
			}
			if len(runner.calls) != 0 {
				t.Fatalf("unsafe operand reached runner: %#v", runner.calls)
			}
		})
	}
}

func TestProgramInstalledUsesLiteralDpkgArgvAndClassifiesAbsence(t *testing.T) {
	runner := &scriptedProcessRunner{outcomes: []scriptedProcessOutcome{{
		result: processResult{stdout: []byte("install ok installed\t1.2-3\n")},
	}}}
	got, err := programInstalledWithRunner(cond{Name: "safe+name.x86_64"}, runner)
	if err != nil || !got {
		t.Fatalf("installed got (%v, %v)", got, err)
	}
	wantCall := recordedProcessCall{
		path:    "/usr/bin/dpkg-query",
		args:    []string{"-W", "-f=${Status}\t${Version}\n", "safe+name.x86_64"},
		profile: builtInQuery,
	}
	if !reflect.DeepEqual(runner.calls, []recordedProcessCall{wantCall}) {
		t.Fatalf("calls got %#v, want %#v", runner.calls, wantCall)
	}

	runner = &scriptedProcessRunner{outcomes: []scriptedProcessOutcome{{err: processExit("/usr/bin/dpkg-query", 1)}}}
	got, err = programInstalledWithRunner(cond{Name: "missing"}, runner)
	if err != nil || got || len(runner.calls) != 1 {
		t.Fatalf("absence got (%v, %v), calls=%d", got, err, len(runner.calls))
	}
}

func TestProgramInstalledFallsBackOnlyWhenDpkgExecutableIsAbsent(t *testing.T) {
	missingTool := &processStartError{path: "/usr/bin/dpkg-query", err: os.ErrNotExist}
	runner := &scriptedProcessRunner{outcomes: []scriptedProcessOutcome{
		{err: missingTool},
		{result: processResult{stdout: []byte("4.19.0-1\n")}},
	}}
	got, err := programInstalledWithRunner(cond{Name: "bash"}, runner)
	if err != nil || !got {
		t.Fatalf("rpm fallback got (%v, %v)", got, err)
	}
	wantRPM := recordedProcessCall{
		path:    "/usr/bin/rpm",
		args:    []string{"-q", "--qf", "%{VERSION}-%{RELEASE}\n", "bash"},
		profile: builtInQuery,
	}
	if len(runner.calls) != 2 || !reflect.DeepEqual(runner.calls[1], wantRPM) {
		t.Fatalf("rpm call got %#v", runner.calls)
	}

	runner = &scriptedProcessRunner{outcomes: []scriptedProcessOutcome{{err: processExit("/usr/bin/dpkg-query", 2)}}}
	got, err = programInstalledWithRunner(cond{Name: "bash"}, runner)
	if err == nil || got || len(runner.calls) != 1 {
		t.Fatalf("operational failure got (%v, %v), calls=%d", got, err, len(runner.calls))
	}
}

func TestProgramVersionParsesExactVersionAndRejectsMalformedOutput(t *testing.T) {
	runner := &scriptedProcessRunner{outcomes: []scriptedProcessOutcome{{
		result: processResult{stdout: []byte("install ok installed\t1.2-3\n")},
	}}}
	got, err := programVersionWithRunner(cond{Name: "safe", Value: "1.2-3"}, runner)
	if err != nil || !got {
		t.Fatalf("version match got (%v, %v)", got, err)
	}

	runner = &scriptedProcessRunner{outcomes: []scriptedProcessOutcome{{
		result: processResult{stdout: []byte("install ok installed 1.2-3\n")},
	}}}
	got, err = programVersionWithRunner(cond{Name: "safe", Value: "1.2-3"}, runner)
	var outputError *processOutputError
	if got || !errors.As(err, &outputError) {
		t.Fatalf("malformed output got (%v, %T)", got, err)
	}

	runner = &scriptedProcessRunner{outcomes: []scriptedProcessOutcome{{
		result: processResult{stdout: []byte("deinstall ok config-files\t1.2-3\n")},
	}}}
	got, err = programVersionWithRunner(cond{Name: "safe", Value: "1.2-3"}, runner)
	if got || !errors.As(err, &outputError) {
		t.Fatalf("unexpected status got (%v, %T)", got, err)
	}
}

func TestServiceUpUsesLiteralArgvAndExplicitStatusClassification(t *testing.T) {
	tests := []struct {
		name    string
		outcome scriptedProcessOutcome
		want    bool
		wantErr bool
	}{
		{"active", scriptedProcessOutcome{result: processResult{stdout: []byte("active\n")}}, true, false},
		{"inactive", scriptedProcessOutcome{result: processResult{stdout: []byte("inactive\n")}, err: processExit("/usr/bin/systemctl", 3)}, false, false},
		{"unknown", scriptedProcessOutcome{result: processResult{stdout: []byte("unknown\n")}, err: processExit("/usr/bin/systemctl", 4)}, false, false},
		{"malformed", scriptedProcessOutcome{result: processResult{stdout: []byte("surprise\n")}, err: processExit("/usr/bin/systemctl", 3)}, false, true},
		{"operational", scriptedProcessOutcome{err: processExit("/usr/bin/systemctl", 1)}, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &scriptedProcessRunner{outcomes: []scriptedProcessOutcome{tt.outcome}}
			got, err := serviceUpWithRunner(cond{Name: "sshd.service"}, runner)
			if got != tt.want || (err != nil) != tt.wantErr {
				t.Fatalf("got (%v, %v), want (%v, err=%v)", got, err, tt.want, tt.wantErr)
			}
			want := recordedProcessCall{path: "/usr/bin/systemctl", args: []string{"is-active", "sshd.service"}, profile: builtInQuery}
			if len(runner.calls) != 1 || !reflect.DeepEqual(runner.calls[0], want) {
				t.Fatalf("call got %#v", runner.calls)
			}
		})
	}
}
