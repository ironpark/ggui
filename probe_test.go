package ggui

import "testing"

// The Probe's tests through the public surface are in internal/apitest;
// this one stays for the fake IME a test editor needs, which only the
// package can install.
func TestEachProbeHasItsOwnClipboard(t *testing.T) {
	t.Parallel()
	useFakeIME(t)
	a := NewProbe(TextInput(State("secret")), Sz(200, 20))
	defer a.Close()
	b := NewProbe(Text("other"), Sz(100, 20))
	defer b.Close()
	b.Frame()
	a.Click(Pt(190, 10))
	a.Key("cmd+a", "cmd+c")
	if got := a.Clipboard().Read(); got != "secret" {
		t.Fatalf("copy put %q on the probe's clipboard", got)
	}
	if got := b.Clipboard().Read(); got != "" {
		t.Fatalf("copy in one probe reached another's clipboard: %q", got)
	}
}
