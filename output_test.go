package main

import (
	"strings"
	"testing"

	"github.com/fatih/color"
)

// TestPrinterFormatsLabelAndMessage characterizes the normal-case output of
// printer: the label is wrapped in brackets and followed by the message.
// It passes both before and after the format-string fix and documents the
// structural contract every caller (pass/fail/warn/info/debug/red/green/blue)
// depends on.
func TestPrinterFormatsLabelAndMessage(t *testing.T) {
	color.NoColor = true
	got := printer(color.FgGreen, "PASS", "all checks done")
	if !strings.Contains(got, "[PASS]") || !strings.Contains(got, "] all checks done") {
		t.Errorf("printer output unexpected: got %q", got)
	}
}

// TestPrinterLabelNotInterpretedAsFormat guards against regressing the
// non-constant format string bug: messageType must be rendered literally and
// never interpreted as a format string, so a label containing a percent verb
// (e.g. "%d") must survive unchanged. Before the fix, printer.Sprintf was
// called with messageType as the format, turning "%d" into "%!d(MISSING)".
func TestPrinterLabelNotInterpretedAsFormat(t *testing.T) {
	color.NoColor = true
	got := printer(color.FgRed, "%d", "msg")
	if !strings.Contains(got, "[%d]") {
		t.Errorf("printer interpreted label as a format verb; got %q, want literal [%%d]", got)
	}
}
