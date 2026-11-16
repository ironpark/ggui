//go:build (unix && !ios && !android) || windows

package main

import (
	"bytes"
	"encoding/json"
	"image"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/ironpark/ggwd/remote"
)

func TestParseCommand(t *testing.T) {
	good := []struct {
		line  string
		debug bool
		want  command
	}{
		{`{"type":"building"}`, false, command{Type: cmdBuilding}},
		{`{"type":"build-failed","output":"./a.go:1:1: x\n"}`, false, command{Type: cmdBuildFailed, Output: "./a.go:1:1: x\n"}},
		{`{"type":"app-exited","status":"exit status 2","output":"panic: x"}`, false, command{Type: cmdAppExited, Status: "exit status 2", Output: "panic: x"}},
		{`{"type":"clear","extra":1}`, false, command{Type: cmdClear}},
		{`{"type":"quit"}`, false, command{Type: cmdQuit}},
		{`{"type":"debug-dump","path":"/tmp/a.png"}`, true, command{Type: cmdDebugDump, Path: "/tmp/a.png"}},
		{`{"type":"debug-window","action":"resize","width":300,"height":200}`, true, command{Type: cmdDebugWindow, Action: "resize", Width: 300, Height: 200}},
		{`{"type":"debug-input","input":{"kind":7,"text":"한","start":0,"end":3}}`, true,
			command{Type: cmdDebugInput, Input: &remote.Input{Kind: remote.InputComposition, Text: "한", End: 3}}},
	}
	for _, c := range good {
		got, err := parseCommand([]byte(c.line), c.debug)
		if err != nil || !reflect.DeepEqual(got, c.want) {
			t.Errorf("parseCommand(%s) = %+v, %v; want %+v", c.line, got, err, c.want)
		}
	}
	bad := []struct {
		line  string
		debug bool
	}{
		{`building`, true},
		{`{}`, true},
		{`{"type":"rebuild"}`, true},
		{`{"type":"debug-dump","path":"/tmp/a.png"}`, false},
		{`{"type":"debug-dump"}`, true},
		{`{"type":"debug-window","action":"explode"}`, true},
		{`{"type":"debug-window","action":"resize"}`, true},
		{`{"type":"debug-input"}`, true},
	}
	for _, c := range bad {
		if got, err := parseCommand([]byte(c.line), c.debug); err == nil {
			t.Errorf("parseCommand(%s) = %+v, want an error", c.line, got)
		}
	}
}

func TestEvents(t *testing.T) {
	var b bytes.Buffer
	e := &events{w: &b}
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Go(func() { e.send(event{Type: evAppAttached, PID: i + 1}) })
	}
	wg.Wait()
	e.send(event{Type: evReady, Socket: "/tmp/s"})
	lines := strings.Split(strings.TrimSuffix(b.String(), "\n"), "\n")
	if len(lines) != 51 {
		t.Fatalf("%d lines", len(lines))
	}
	for _, l := range lines {
		var ev map[string]any
		if err := json.Unmarshal([]byte(l), &ev); err != nil {
			t.Errorf("%q: %v", l, err)
		}
	}
	if want := `{"type":"ready","socket":"/tmp/s"}`; lines[50] != want {
		t.Errorf("got %s, want %s", lines[50], want)
	}
}

func TestState(t *testing.T) {
	dir := t.TempDir()
	p := statePath(dir, "/home/me/my app")
	if !strings.HasPrefix(p, filepath.Join(dir, "ggui", "ggviewer", "my_app-")) || !strings.HasSuffix(p, ".json") {
		t.Errorf("statePath: %s", p)
	}
	if q := statePath(dir, "/elsewhere/my app"); q == p {
		t.Error("two projects of one base name share a state file")
	}
	if _, ok := loadState(p, "/home/me/my app"); ok {
		t.Error("a state loaded from no file")
	}
	g := geometry{X: 10, Y: -20.5, Width: 640, Height: 480}
	if err := saveState(p, "/home/me/my app", g); err != nil {
		t.Fatal(err)
	}
	if got, ok := loadState(p, "/home/me/my app"); !ok || got != g {
		t.Errorf("loaded %+v, %t; want %+v", got, ok, g)
	}
	// A file of another project, one that is broken, or a size of nothing,
	// is not used.
	if _, ok := loadState(p, "/other"); ok {
		t.Error("a state loaded for another project")
	}
	for _, s := range []string{`{`, `{"project":"/home/me/my app","window":{"Width":0,"Height":10}}`} {
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, ok := loadState(p, "/home/me/my app"); ok {
			t.Errorf("a state loaded from %s", s)
		}
	}
	entries, _ := os.ReadDir(filepath.Dir(p))
	if len(entries) != 1 {
		t.Errorf("the state's directory has %d files, want 1", len(entries))
	}
}

type fakeWindow struct{ calls []string }

func (w *fakeWindow) SetTitle(s string)    { w.calls = append(w.calls, "title "+s) }
func (w *fakeWindow) SetSize(x, y float64) { w.calls = append(w.calls, sprint("size", x, y)) }
func (w *fakeWindow) SetSizeLimits(a, b, c, d float64) {
	w.calls = append(w.calls, sprint("limits", a, b, c, d))
}
func (w *fakeWindow) SetResizable(v bool) { w.calls = append(w.calls, sprint("resizable", v)) }
func (w *fakeWindow) SetDecorated(v bool) { w.calls = append(w.calls, sprint("decorated", v)) }
func (w *fakeWindow) SetFloating(v bool)  { w.calls = append(w.calls, sprint("floating", v)) }
func sprint(name string, args ...any) string {
	b, _ := json.Marshal(args)
	return name + " " + string(b)
}
func (w *fakeWindow) reset() []string { c := w.calls; w.calls = nil; return c }
func appOptions(f func(*remote.OpenWindow)) remote.OpenWindow {
	o := placeholder
	o.Title = "app"
	f(&o)
	return o
}

func TestPlanOptions(t *testing.T) {
	first := appOptions(func(o *remote.OpenWindow) { o.Width, o.Height = 400, 300 })
	for _, c := range []struct {
		name     string
		prev     *remote.OpenWindow
		next     remote.OpenWindow
		title    string
		restored bool
		want     []string
	}{
		{"the same app again", &first, first, "app", false, nil},
		{"the same app again, whose old one set its title", &first, first, "count 3", false, []string{"title app"}},
		{"a new size", &first, appOptions(func(o *remote.OpenWindow) { o.Width = 500 }), "app", false, []string{`size [500,600]`}},
		{"new limits", &first, appOptions(func(o *remote.OpenWindow) { o.Width, o.Height, o.MinWidth, o.MaxHeight = 400, 300, 200, 900 }), "app", false,
			[]string{`limits [200,0,0,900]`}},
		{"flags", &first, appOptions(func(o *remote.OpenWindow) {
			o.Width, o.Height, o.Resizable, o.Decorated, o.Floating, o.TitlebarHidden, o.Transparent = 400, 300, false, false, true, true, true
		}), "app", false, []string{"resizable [false]", "decorated [false]", "floating [true]"}},
		{"the first app in a window opened for a failure", nil, first, "ggviewer", false,
			[]string{"title app", "limits [0,0,0,0]", "resizable [true]", "decorated [true]", "floating [false]", "size [400,300]"}},
		{"the first app in a window of a remembered size", nil, first, "ggviewer", true,
			[]string{"title app", "limits [0,0,0,0]", "resizable [true]", "decorated [true]", "floating [false]"}},
	} {
		var w fakeWindow
		planOptions(c.prev, &c.next, c.title, c.restored).apply(&w)
		if got := w.reset(); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

func TestOverlay(t *testing.T) {
	const w, h = 1000, 800
	if img, _ := (&overlay{}).render(w, h, 2); img != nil {
		t.Error("no overlay drew something")
	}

	// Building is a small note in the top right corner.
	img, l := (&overlay{kind: overlayBuilding}).render(w, h, 2)
	if img == nil || !l.rect.In(image.Rect(w/2, 0, w, h/8)) || img.Rect.Size() != l.rect.Size() {
		t.Errorf("building: %v at %v", img.Rect, l.rect)
	}
	if !hasInk(img) {
		t.Error("building: nothing drawn")
	}

	// A failure's panel is at the bottom, no higher than its share of the
	// window, and scrolls when the output is longer.
	o := &overlay{kind: overlayBuildFailed, output: strings.Repeat("./main.go:1:2: undefined: x\n", 200), scroll: 1000}
	img, l = o.render(w, h, 2)
	if l.rect.Max.Y > h || l.rect.Dy() > int(h*panelMaxHeight) || l.rect.Dx() > w {
		t.Errorf("build-failed: the panel is at %v", l.rect)
	}
	if !l.scrollable() || l.lines != 200 || o.scroll != l.lines-l.rows {
		t.Errorf("build-failed: %d of %d lines from %d", l.rows, l.lines, o.scroll)
	}
	if !hasInk(img) {
		t.Error("build-failed: nothing drawn")
	}

	// A short one fits.
	o = &overlay{kind: overlayAppExited, status: "exit status 2", output: "panic: x\n"}
	_, l = o.render(w, h, 1)
	if l.scrollable() || l.lines != 1 || l.rect.Dy() > h/4 {
		t.Errorf("app-exited: %+v", l)
	}
}

func hasInk(img *image.RGBA) bool {
	if img == nil {
		return false
	}
	for i := 3; i < len(img.Pix); i += 4 {
		if img.Pix[i] != 0 {
			return true
		}
	}
	return false
}

func TestCleanAndWrapText(t *testing.T) {
	if got, want := cleanText("\x1b[31merror\x1b[0m:\r\n\tx\x00y\n\n"), "error:\n    xy"; got != want {
		t.Errorf("cleanText: %q, want %q", got, want)
	}
	if got, want := wrapText("abcdefg\n한글한글한", 3), []string{"abc", "def", "g", "한글한", "글한"}; !reflect.DeepEqual(got, want) {
		t.Errorf("wrapText: %q, want %q", got, want)
	}
	if got := wrapText("", 3); got != nil {
		t.Errorf("wrapText of nothing: %q", got)
	}
}
