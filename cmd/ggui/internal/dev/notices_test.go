package dev

import (
	"bytes"
	"slices"
	"strings"
	"testing"
)

// The viewer's notices are taken out of the app's stderr and said once a
// session, however the app's writes split its lines; the rest passes as
// written.
func TestNotices(t *testing.T) {
	const tray = "ggui: the tray icon does not show while ggui dev's viewer shows the app; ggui dev --display=window or a release build does\n"
	const window = "ggui: window \"Prefs\" does not open while ggui dev's viewer shows the app; ggui dev --display=window or a release build does\n"
	var seen noticeSet
	var said []string
	run := func(writes ...string) string {
		var out bytes.Buffer
		n := &notices{out: &out, seen: &seen, say: func(what string) { said = append(said, what) }}
		for _, w := range writes {
			if k, err := n.Write([]byte(w)); err != nil || k != len(w) {
				t.Fatalf("Write(%q) = %d, %v", w, k, err)
			}
		}
		return out.String()
	}

	got := run("starting\n", tray[:3], tray[3:20], tray[20:]+"ggui: an app's own line\nprogress: 5", "0%\n", window+"gg", "ui: done\n")
	if want := "starting\nggui: an app's own line\nprogress: 50%\nggui: done\n"; got != want {
		t.Errorf("passed on %q, want %q", got, want)
	}
	if want := []string{"the tray icon does not show", `window "Prefs" does not open`}; !slices.Equal(said, want) {
		t.Errorf("said %q, want %q", said, want)
	}

	// The next run of the app says the same again: ggui does not.
	said = nil
	if got := run(tray, "x ggui: not at the start\n"); got != "x ggui: not at the start\n" || len(said) != 0 {
		t.Errorf("a second run: passed on %q, said %q", got, said)
	}

	// A line that is not a notice is not held past its end, nor one that
	// stops looking like one.
	var out bytes.Buffer
	n := &notices{out: &out, seen: &seen, say: func(string) {}}
	n.Write([]byte("ggx"))
	if out.String() != "ggx" {
		t.Errorf("held %q", "ggx")
	}
	if !strings.Contains(restate("the tray icon does not show"), "--display=window") {
		t.Error("the tray's restatement lacks the flag")
	}
}
