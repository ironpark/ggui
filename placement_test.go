package ggui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// testScreens are a laptop's display, which windows open on, and an
// external one beside it.
var testScreens = []Screen{
	{Name: "Built-in", Size: Sz(1440, 900), Scale: 2},
	{Name: "External", Size: Sz(2560, 1440), Scale: 1},
}

func TestPlacedKeepsAReachableFrame(t *testing.T) {
	t.Parallel()
	got := placed(Config{Width: 800, Height: 600}, placement{
		Position: &Point{X: 100, Y: 80}, Width: 1000, Height: 700,
		Screen: &placementScreen{Name: "Built-in"},
	}, testScreens)
	if got.Position == nil || *got.Position != Pt(100, 80) || got.Width != 1000 || got.Height != 700 || got.Maximized {
		t.Fatalf("placed = position %v, %dx%d, maximized %t; want (100,80), 1000x700, not maximized", got.Position, got.Width, got.Height, got.Maximized)
	}
}

func TestPlacedLeavesAnUnreachablePositionToTheConfig(t *testing.T) {
	t.Parallel()
	at := Pt(30, 40)
	for _, c := range []struct {
		name string
		p    placement
	}{
		{"past the right edge", placement{Position: &Point{X: 5000, Y: 100}}},
		{"below the bottom", placement{Position: &Point{X: 100, Y: 5000}}},
		{"title bar above the top", placement{Position: &Point{X: 100, Y: -200}}},
		{"all but a sliver off the left", placement{Position: &Point{X: -790, Y: 100}}},
		{"on a display since unplugged", placement{Position: &Point{X: 100, Y: 100}, Screen: &placementScreen{Name: "Projector"}}},
		{"on a display windows do not open on", placement{Position: &Point{X: 100, Y: 100}, Screen: &placementScreen{Name: "External"}}},
	} {
		c.p.Width, c.p.Height = 800, 500
		for _, cfg := range []Config{{Width: 640, Height: 480}, {Width: 640, Height: 480, Position: &at}} {
			got := placed(cfg, c.p, testScreens)
			if got.Position != cfg.Position {
				t.Errorf("%s, Config.Position %v: position %v; want the Config's", c.name, cfg.Position, got.Position)
			}
			if got.Width != 800 || got.Height != 500 {
				t.Errorf("%s: size %dx%d; want the saved 800x500 kept", c.name, got.Width, got.Height)
			}
		}
	}
}

func TestPlacedKeepsTheSizeWithinTheLimitsAndTheScreen(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name         string
		cfg          Config
		w, h         float64
		wantW, wantH int
	}{
		{"larger than the screen", Config{}, 3000, 2000, 1440, 900},
		{"beyond the maximum", Config{MaxWidth: 1000, MaxHeight: 500}, 1200, 800, 1000, 500},
		{"below the minimum", Config{MinWidth: 300, MinHeight: 200}, 100, 50, 300, 200},
		{"a minimum larger than the screen", Config{MinWidth: 2000}, 3000, 600, 2000, 600},
		{"a fraction", Config{}, 640.6, 480.4, 641, 480},
	} {
		got := placed(c.cfg, placement{Width: c.w, Height: c.h}, testScreens)
		if got.Width != c.wantW || got.Height != c.wantH {
			t.Errorf("%s: %dx%d; want %dx%d", c.name, got.Width, got.Height, c.wantW, c.wantH)
		}
	}
}

func TestPlacedReopensMaximizedOverTheNormalFrame(t *testing.T) {
	t.Parallel()
	got := placed(Config{Width: 800, Height: 600}, placement{
		Position: &Point{X: 50, Y: 60}, Width: 900, Height: 650, Maximized: true,
	}, testScreens)
	if !got.Maximized {
		t.Fatal("a placement saved maximized reopened normal")
	}
	if got.Position == nil || *got.Position != Pt(50, 60) || got.Width != 900 || got.Height != 650 {
		t.Fatalf("the frame Restore returns to is %v %dx%d; want (50,60) 900x650", got.Position, got.Width, got.Height)
	}
	// What the user left wins over what the app asks for.
	if placed(Config{Maximized: true}, placement{Width: 900, Height: 650}, testScreens).Maximized {
		t.Fatal("a window closed normal reopened maximized because its Config says so")
	}
}

func TestPlacedWithNothingToGoOnLeavesTheConfig(t *testing.T) {
	t.Parallel()
	at := Pt(10, 10)
	cfg := Config{Width: 640, Height: 480, Position: &at, Maximized: true}
	for name, c := range map[string]struct {
		p       placement
		screens []Screen
	}{
		"no screens":     {placement{Position: &Point{X: 100, Y: 100}, Width: 900, Height: 700}, nil},
		"no size":        {placement{Position: &Point{X: 100, Y: 100}}, testScreens},
		"negative width": {placement{Width: -5, Height: 700}, testScreens},
	} {
		got := placed(cfg, c.p, c.screens)
		if got.Position != cfg.Position || got.Width != 640 || got.Height != 480 || !got.Maximized {
			t.Errorf("%s: the Config changed to %v %dx%d maximized %t", name, got.Position, got.Width, got.Height, got.Maximized)
		}
	}
}

// rememberingApp makes an app whose main window remembers its placement as
// "main", in a file under dir.
func rememberingApp(t *testing.T, dir string) *App {
	t.Helper()
	a := New(Config{Title: "Notes", Width: 640, Height: 480, Remember: "main"}, func() Widget { return sized(0, 0) })
	a.placements = &placementStore{path: filepath.Join(dir, "windows.json")}
	t.Cleanup(a.Close)
	return a
}

// openRemembering does what Window.open does with the window's placement,
// without the native window: restores it and starts keeping it.
func openRemembering(w *Window) {
	w.trackPlacement(w.restorePlacement(testScreens))
}

func TestARememberedPlacementComesBackAfterClose(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := rememberingApp(t, dir)
	openRemembering(a.Window)
	// What notePlacement takes from a window moved and resized, then
	// maximized.
	a.remembered = placement{Position: &Point{X: 120, Y: 90}, Width: 1000, Height: 700, Screen: &placementScreen{Name: "Built-in"}}
	a.remembered.Maximized = true
	a.Close()

	b := rememberingApp(t, dir)
	openRemembering(b.Window)
	if b.cfg.Position == nil || *b.cfg.Position != Pt(120, 90) || b.cfg.Width != 1000 || b.cfg.Height != 700 || !b.cfg.Maximized {
		t.Fatalf("reopened at %v %dx%d maximized %t; want (120,90) 1000x700 maximized", b.cfg.Position, b.cfg.Width, b.cfg.Height, b.cfg.Maximized)
	}
	// Closed again untouched, it keeps what it opened with.
	b.Close()
	c := rememberingApp(t, dir)
	openRemembering(c.Window)
	if c.cfg.Position == nil || *c.cfg.Position != Pt(120, 90) || c.cfg.Width != 1000 || !c.cfg.Maximized {
		t.Fatalf("a window closed as it opened lost its placement: %v %dx%d maximized %t", c.cfg.Position, c.cfg.Width, c.cfg.Height, c.cfg.Maximized)
	}
}

func TestARememberedPlacementOffEveryScreenOpensWhereTheConfigSays(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := rememberingApp(t, dir)
	openRemembering(a.Window)
	a.remembered = placement{Position: &Point{X: 4000, Y: 3000}, Width: 900, Height: 600}
	a.Close()

	b := rememberingApp(t, dir)
	openRemembering(b.Window)
	if b.cfg.Position != nil || b.cfg.Width != 900 || b.cfg.Height != 600 {
		t.Fatalf("reopened at %v %dx%d; want centered at the saved 900x600", b.cfg.Position, b.cfg.Width, b.cfg.Height)
	}
}

func TestACorruptPlacementFileIsIgnoredAndReplaced(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "windows.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := rememberingApp(t, dir)
	openRemembering(a.Window)
	if a.cfg.Position != nil || a.cfg.Width != 640 || a.cfg.Height != 480 || a.cfg.Maximized {
		t.Fatalf("a corrupt file changed the Config: %v %dx%d", a.cfg.Position, a.cfg.Width, a.cfg.Height)
	}
	a.remembered = placement{Position: &Point{X: 10, Y: 20}, Width: 700, Height: 500}
	a.Close()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var all map[string]placement
	if err := json.Unmarshal(data, &all); err != nil {
		t.Fatalf("the file was not replaced with one that reads: %v\n%s", err, data)
	}
	if p := all["main"]; p.Position == nil || *p.Position != Pt(10, 20) || p.Width != 700 {
		t.Fatalf("saved %+v; want (10,20) 700x500", p)
	}
}

func TestWindowsOfAnAppKeepTheirPlacementsSideBySide(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := rememberingApp(t, dir)
	openRemembering(a.Window)
	prefs, err := a.OpenWindow(Config{Title: "Preferences", Remember: "prefs"}, func() Widget { return sized(0, 0) })
	if err != nil {
		t.Fatal(err)
	}
	openRemembering(prefs)
	a.remembered = placement{Width: 1000, Height: 700}
	prefs.remembered = placement{Width: 400, Height: 300}
	a.Close()

	b := rememberingApp(t, dir)
	openRemembering(b.Window)
	other, err := b.OpenWindow(Config{Title: "Preferences", Remember: "prefs"}, func() Widget { return sized(0, 0) })
	if err != nil {
		t.Fatal(err)
	}
	openRemembering(other)
	if b.cfg.Width != 1000 || other.cfg.Width != 400 {
		t.Fatalf("widths %d and %d; want 1000 and 400", b.cfg.Width, other.cfg.Width)
	}
}

func TestAWindowThatNeverOpenedKeepsNoPlacement(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := rememberingApp(t, dir)
	a.Close()
	if _, err := os.Stat(filepath.Join(dir, "windows.json")); !os.IsNotExist(err) {
		t.Fatalf("a window that never opened wrote its placement: %v", err)
	}
}

func TestOnlyAnAppsWindowRemembersItsPlacement(t *testing.T) {
	t.Parallel()
	p := NewWindowProbe(func() Widget { return sized(0, 0) }, Sz(100, 100))
	defer p.Close()
	p.Window().cfg.Remember = "main"
	if p.Window().restorePlacement(testScreens) != nil {
		t.Fatal("a WindowProbe's window keeps a placement")
	}
	a := New(Config{}, func() Widget { return sized(0, 0) })
	defer a.Close()
	a.placements = &placementStore{path: filepath.Join(t.TempDir(), "windows.json")}
	if a.restorePlacement(testScreens) != nil {
		t.Fatal("a window without Config.Remember keeps a placement")
	}
}

func TestThePlacementsDirectoryIsNamedForTheApp(t *testing.T) {
	t.Parallel()
	a := New(Config{Title: "Notes"}, func() Widget { return sized(0, 0) })
	defer a.Close()
	a.SetTitle("draft.txt — Notes")
	if got := a.identity(); got != "Notes" {
		t.Errorf("identity %q; want the title New was given, %q", got, "Notes")
	}
	a.instanceID = "com.example.notes"
	if got := a.identity(); got != "com.example.notes" {
		t.Errorf("identity %q; want SingleInstance's id", got)
	}
	for id, want := range map[string]string{
		"com.example.notes": "com.example.notes",
		"Notes: a/b?":       "Notes_ a_b_",
		"..":                "ggui",
		"":                  "ggui",
	} {
		if got := placementDirName(id); got != want {
			t.Errorf("placementDirName(%q) = %q; want %q", id, got, want)
		}
	}
}
