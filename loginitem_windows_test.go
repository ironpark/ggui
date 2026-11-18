package ggui

import (
	"syscall"
	"testing"
)

// windowsArg is a copy of syscall.EscapeArg, so that every platform can
// test the Run value; here the copy is checked against the original.
func TestWindowsArgIsEscapeArg(t *testing.T) {
	t.Parallel()
	for _, s := range []string{
		"", "plain", "a b", "a\tb", `a"b`, `a\"b`, `a\b`, `a b\`, `a "b" \\`,
		`\\server\share\`, `"`, `\`, `\\"`, "%PATH%", "--hidden",
	} {
		if got, want := windowsArg(s), syscall.EscapeArg(s); got != want {
			t.Errorf("windowsArg(%q) = %s, syscall.EscapeArg = %s", s, got, want)
		}
	}
}
