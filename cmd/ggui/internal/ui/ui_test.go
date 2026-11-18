package ui

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"
)

// clock is a time a test moves by hand.
type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }

// setup takes the package's clock and its frames for the test.
func setup(t *testing.T) *clock {
	t.Helper()
	c := &clock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	oldNow, oldTick := now, autoTick
	now, autoTick = c.now, false
	t.Cleanup(func() {
		Reset()
		now, autoTick = oldNow, oldTick
		frame, held, drawn = 0, false, 0
	})
	return c
}

// terminal returns an Output that takes w for a terminal width columns
// wide, without color unless asked.
func terminal(w *bytes.Buffer, width int, color bool) *Output {
	return &Output{w: w, live: true, color: color, width: func() int { return width }}
}

func tick() {
	mu.Lock()
	defer mu.Unlock()
	tickLocked(now())
}

func TestColorOn(t *testing.T) {
	for _, tc := range []struct {
		env      map[string]string
		terminal bool
		want     bool
	}{
		{nil, true, true},
		{nil, false, false},
		{map[string]string{"NO_COLOR": "1"}, true, false},
		{map[string]string{"NO_COLOR": ""}, true, true}, // set but empty counts as unset
		{map[string]string{"CLICOLOR_FORCE": "1"}, false, true},
		{map[string]string{"FORCE_COLOR": "1"}, false, true},
		{map[string]string{"FORCE_COLOR": "0"}, false, false},
		{map[string]string{"NO_COLOR": "1", "CLICOLOR_FORCE": "1"}, true, false},
		{map[string]string{"TERM": "dumb"}, true, false},
		{map[string]string{"TERM": "dumb", "CLICOLOR_FORCE": "1"}, true, true},
	} {
		getenv := func(k string) string { return tc.env[k] }
		if got := colorOn(tc.terminal, getenv); got != tc.want {
			t.Errorf("colorOn(%v, %v) = %v, want %v", tc.terminal, tc.env, got, tc.want)
		}
	}
}

// A writer that is not a terminal gets neither color nor animation, even
// when color is forced: no carriage return in a log.
func TestNotATerminal(t *testing.T) {
	var buf bytes.Buffer
	o := newOutput(&buf, func(string) string { return "" })
	if o.Terminal() || o.Color() {
		t.Errorf("a buffer is a terminal (%v) or colored (%v)", o.Terminal(), o.Color())
	}
	forced := newOutput(&buf, func(k string) string {
		if k == "CLICOLOR_FORCE" {
			return "1"
		}
		return ""
	})
	if forced.Terminal() || !forced.Color() {
		t.Errorf("CLICOLOR_FORCE: terminal %v, color %v; want false, true", forced.Terminal(), forced.Color())
	}
	if got := forced.Green("ok"); got != "\x1b[32mok\x1b[0m" {
		t.Errorf("Green = %q", got)
	}
	if For(os.Stderr) != Stderr || For(os.Stdout) != Stdout || For(o) != o {
		t.Error("For did not return the shared Output for the standard streams, or o for o")
	}
}

func TestPlainStep(t *testing.T) {
	c := setup(t)
	var buf bytes.Buffer
	o := New(&buf)
	s := o.Step("Fetching ggui", StallHint(20*time.Second, "still waiting"))
	io := s.Writer()
	io.Write([]byte("go: downloading a v1\ngo: downl"))
	io.Write([]byte("oading b v2\n"))
	c.advance(5 * time.Second)
	tick()
	c.advance(21 * time.Second)
	tick()
	tick() // said once
	s.Done("Fetched ggui")
	s.Done("twice") // ends once

	want := "• Fetching ggui\n" +
		"  still waiting\n" +
		"✓ Fetched ggui  26.0s\n"
	if got := buf.String(); got != want {
		t.Errorf("output:\n%q\nwant\n%q", got, want)
	}
	if got := s.Output(); got != "go: downloading a v1\ngo: downloading b v2\n" {
		t.Errorf("Output = %q", got)
	}
	if s.detail != "go: downloading b v2" {
		t.Errorf("detail = %q", s.detail)
	}
}

func TestPlainOptions(t *testing.T) {
	setup(t)
	var buf bytes.Buffer
	o := New(&buf).Prefixed("ggui")
	o.Step("building", QuietStart(), NoElapsed()).Done("built in 1.2s")
	o.Info("watching %d files", 3)
	o.Step("rebuilding").Fail("")
	New(&buf).Warn("careful")
	want := "ggui: built in 1.2s\n" +
		"ggui: watching 3 files\n" +
		"ggui: rebuilding\n" +
		"ggui: rebuilding\n" +
		"! careful\n"
	if got := buf.String(); got != want {
		t.Errorf("output:\n%q\nwant\n%q", got, want)
	}
}

// On a terminal a step is a line redrawn in place, and what else is
// written goes above it, never into it.
func TestLiveStep(t *testing.T) {
	c := setup(t)
	var buf bytes.Buffer
	o := terminal(&buf, 40, false)
	s := o.Step("Fetching")
	if got := buf.String(); got != "⠋ Fetching" {
		t.Fatalf("first frame = %q", got)
	}
	buf.Reset()

	c.advance(1500 * time.Millisecond)
	s.Writer().Write([]byte("go: downloading golang.org/x/sys v0.48.0\n"))
	tick()
	// Erased and redrawn, cut to 39 columns.
	if got, want := buf.String(), "\r\x1b[K⠙ Fetching  1.5s  go: downloading gola…"; got != want {
		t.Fatalf("frame = %q, want %q", got, want)
	}
	if w := visibleWidth(strings.TrimPrefix(buf.String(), "\r\x1b[K")); w != 39 {
		t.Errorf("frame is %d columns, want 39", w)
	}
	buf.Reset()

	o.Info("a message")
	if got, want := buf.String(), "\r\x1b[K• a message\n⠙ Fetching  1.5s  go: downloading gola…"; got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
	buf.Reset()

	s.Done("Fetched")
	if got, want := buf.String(), "\r\x1b[K✓ Fetched  1.5s\n"; got != want {
		t.Fatalf("done = %q, want %q", got, want)
	}
	buf.Reset()
	tick()
	o.Println("after")
	if got := buf.String(); got != "after\n" {
		t.Errorf("after the step = %q: the region was drawn again", got)
	}
}

// Two live steps share the region, a line each; the one that ends first
// leaves the other below its result.
func TestLiveRegion(t *testing.T) {
	setup(t)
	var buf bytes.Buffer
	o := terminal(&buf, 80, false)
	a := o.Step("one")
	b := o.Step("two")
	buf.Reset()
	a.Fail("one failed")
	if got, want := buf.String(), "\r\x1b[K\x1b[A\x1b[K✗ one failed\n⠋ two"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	buf.Reset()
	b.Stop()
	if got := buf.String(); got != "\r\x1b[K" {
		t.Errorf("stop = %q, want the line erased", got)
	}
}

// Output that leaves a line unfinished holds the region off the screen
// until the line is finished, and ggui's own lines start on a new one.
func TestHeldLine(t *testing.T) {
	setup(t)
	var buf bytes.Buffer
	o := terminal(&buf, 80, false)
	s := o.Step("work")
	buf.Reset()
	o.Write([]byte("progress 10%"))
	tick()
	if got := buf.String(); got != "\r\x1b[Kprogress 10%" {
		t.Fatalf("got %q: a frame was drawn after an unfinished line", got)
	}
	buf.Reset()
	s.Done("")
	if got := buf.String(); got != "\n✓ work\n" {
		t.Errorf("got %q, want the result on a line of its own", got)
	}
}

// A step on another terminal, or one asked not to, does not animate.
func TestNoSpin(t *testing.T) {
	setup(t)
	var a, b bytes.Buffer
	ta, tb := terminal(&a, 80, true), terminal(&b, 80, true)
	ta.Step("live")
	s := tb.Step("elsewhere")
	if s.animated || !strings.Contains(b.String(), "elsewhere\n") {
		t.Errorf("a second terminal's step animated: %q", b.String())
	}
	n := ta.Step("quiet", NoSpin(), QuietStart())
	if n.animated {
		t.Error("NoSpin animated")
	}
	if got := ta.Dim("x"); got != "\x1b[2mx\x1b[0m" {
		t.Errorf("Dim = %q", got)
	}
}

func TestStatic(t *testing.T) {
	setup(t)
	var buf bytes.Buffer
	o := terminal(&buf, 80, false)
	s := o.Step("building")
	buf.Reset()
	s.Static()
	if got := buf.String(); got != "\r\x1b[K• building\n" {
		t.Errorf("static = %q", got)
	}
	buf.Reset()
	s.Done("built")
	if got := buf.String(); got != "✓ built\n" {
		t.Errorf("done = %q", got)
	}
}

func TestPrefixedTerminal(t *testing.T) {
	setup(t)
	var buf bytes.Buffer
	o := terminal(&buf, 80, false).Prefixed("ggui")
	o.Warn("the app exited")
	if got := buf.String(); got != "ggui ! the app exited\n" {
		t.Errorf("got %q", got)
	}
}

func TestTruncate(t *testing.T) {
	for _, tc := range []struct {
		s    string
		n    int
		want string
	}{
		{"hello", 10, "hello"},
		{"hello world", 8, "hello w…"},
		{"한글한글", 5, "한글…"},
		{"\x1b[2mdimmed text\x1b[0m", 4, "\x1b[2mdim…\x1b[0m"},
		{"abc", 0, ""},
	} {
		if got := truncate(tc.s, tc.n); got != tc.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tc.s, tc.n, got, tc.want)
		}
		if w := visibleWidth(truncate(tc.s, tc.n)); w > tc.n {
			t.Errorf("truncate(%q, %d) is %d wide", tc.s, tc.n, w)
		}
	}
	if got := sanitize("a\tb\x1b[31mc\x07\r"); got != "a bc" {
		t.Errorf("sanitize = %q", got)
	}
}

func TestFormatElapsed(t *testing.T) {
	for d, want := range map[time.Duration]string{
		2300 * time.Millisecond: "2.3s",
		65 * time.Second:        "1m05s",
		0:                       "0.0s",
	} {
		if got := formatElapsed(d); got != want {
			t.Errorf("formatElapsed(%v) = %q, want %q", d, got, want)
		}
	}
}

// A live step that has gone quiet shows the hint on a line below its own,
// and drops it when output comes again.
func TestLiveStall(t *testing.T) {
	c := setup(t)
	var buf bytes.Buffer
	o := terminal(&buf, 80, false)
	s := o.Step("Fetching", StallHint(20*time.Second, "still waiting"))
	s.Detail("go mod tidy")
	c.advance(25 * time.Second)
	buf.Reset()
	tick()
	if got, want := buf.String(), "\r\x1b[K⠙ Fetching  25.0s  go mod tidy\n  still waiting"; got != want {
		t.Fatalf("stalled frame = %q, want %q", got, want)
	}
	buf.Reset()
	s.Writer().Write([]byte("go: downloading y\n"))
	tick()
	if got, want := buf.String(), "\r\x1b[K\x1b[A\x1b[K⠹ Fetching  25.0s  go: downloading y"; got != want {
		t.Fatalf("frame after output = %q, want %q", got, want)
	}
}
