package main

import (
	"testing"

	"github.com/fatih/color"
)

// TestPrinterFormatsLabelAndMessage characterizes the exact normal-case output
// of printer: the label is wrapped in brackets and followed by the message. It
// passes both before and after the format-string fix and documents the
// structural contract every caller (pass/fail/warn/info/debug/red/green/blue)
// depends on.
//
// color.NoColor is a process-global switch; the prior value is saved, forced
// true for deterministic plain output, and restored via t.Cleanup so the
// suite's global state is untouched regardless of outcome or ordering. This
// test must not run in parallel while it mutates that global.
func TestPrinterFormatsLabelAndMessage(t *testing.T) {
	prev := color.NoColor
	color.NoColor = true
	t.Cleanup(func() { color.NoColor = prev })

	const want = "[PASS] all checks done"
	got := printer(color.FgGreen, "PASS", "all checks done")
	if got != want {
		t.Errorf("printer output mismatch:\n got: %q\nwant: %q", got, want)
	}
}

// TestPrinterLabelNotInterpretedAsFormat guards against regressing the
// non-constant format string bug: messageType must be rendered literally and
// never interpreted as a format string, so a label containing a percent verb
// (e.g. "%d") must survive unchanged. Before the fix, printer.Sprintf was
// called with messageType as the format, turning "%d" into "%!d(MISSING)".
//
// color.NoColor is handled with the same save/set/restore discipline as the
// test above; this test must likewise stay sequential.
func TestPrinterLabelNotInterpretedAsFormat(t *testing.T) {
	prev := color.NoColor
	color.NoColor = true
	t.Cleanup(func() { color.NoColor = prev })

	const want = "[%d] msg"
	got := printer(color.FgRed, "%d", "msg")
	if got != want {
		t.Errorf("printer output mismatch:\n got: %q\nwant: %q", got, want)
	}
}
