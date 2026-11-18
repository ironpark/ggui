package ggui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// testScreens are a laptop's display, the primary, and two external
// displays of one model beside it, which share a name: the left one above
// the laptop's top edge, so its corner is negative.
var testScreens = []Screen{
	{Name: "Built-in", ID: "laptop", Size: Sz(1440, 900), WorkArea: Rct(Pt(0, 25), Sz(1440, 875)), Scale: 2},
	{Name: "S22", ID: "s22-right", Position: Pt(1440, 0), Size: Sz(1080, 1920), WorkArea: Rct(Pt(0, 0), Sz(1080, 1920)), Scale: 1},
	{Name: "S22", ID: "s22-left", Position: Pt(-1080, -300), Size: Sz(1080, 1920), WorkArea: Rct(Pt(0, 0), Sz(1080, 1920)), Scale: 1},
}

// on is a placement of 800x500 at pos on the display id.
func on(id string, pos Point) placement {
	return placement{Position: &pos, Width: 800, Height: 500, Screen: &placementScreen{ID: id}}
}

func TestPlacedKeepsAReachableFrame(t *testing.T) {
	t.Parallel()
	got := placed(Config{Width: 800, Height: 600}, placement{
		Position: &Point{X: 100, Y: 80}, Width: 1000, Height: 700,
		Screen: &placementScreen{ID: "laptop", Name: "Built-in"},
	}, testScreens)
	if got.Screen != "laptop" || got.Position == nil || *got.Position != Pt(100, 80) || got.Width != 1000 || got.Height != 700 || got.Maximized {
		t.Fatalf("placed = screen %q, position %v, %dx%d, maximized %t; want laptop, (100,80), 1000x700, not maximized", got.Screen, got.Position, got.Width, got.Height, got.Maximized)
	}
}

func TestPlacedReopensOnASecondaryDisplay(t *testing.T) {
	t.Parallel()
	got := placed(Config{Width: 640, Height: 480}, on("s22-right", Pt(200, 1000)), testScreens)
	if got.Screen != "s22-right" || got.Position == nil || *got.Position != Pt(200, 1000) || got.Width != 800 || got.Height != 500 {
		t.Fatalf("placed = screen %q at %v %dx%d; want s22-right at (200,1000) 800x500", got.Screen, got.Position, got.Width, got.Height)
	}
}

func TestPlacedTellsDisplaysOfOneNameApart(t *testing.T) {
	t.Parallel()
	for _, id := range []string{"s22-left", "s22-right"} {
		if got := placed(Config{}, on(id, Pt(100, 100)), testScreens); got.Screen != id {
			t.Errorf("a placement on %s reopened on %q", id, got.Screen)
		}
	}
}

func TestPlacedFollowsADisplayThatWasRearranged(t *testing.T) {
	t.Parallel()
	// The right display was moved above the laptop, and listed first.
	moved := []Screen{testScreens[1], testScreens[0], testScreens[2]}
	moved[0].Position = Pt(0, -1920)
	got := placed(Config{}, on("s22-right", Pt(200, 1000)), moved)
	if got.Screen != "s22-right" || got.Position == nil || *got.Position != Pt(200, 1000) {
		t.Fatalf("placed = screen %q at %v; want s22-right at (200,1000), on the display wherever it is", got.Screen, got.Position)
	}
}

func TestPlacedLeavesThePlaceToTheConfigWhenTheDisplayIsGone(t *testing.T) {
	t.Parallel()
	at := Pt(30, 40)
	unplugged := testScreens[:1]
	for _, p := range []placement{
		on("s22-right", Pt(200, 1000)),
		{Position: &Point{X: 100, Y: 100}, Width: 800, Height: 500}, // no display named
	} {
		for _, cfg := range []Config{{Width: 640, Height: 480}, {Width: 640, Height: 480, Position: &at, Screen: "laptop"}} {
			got := placed(cfg, p, unplugged)
			if got.Position != cfg.Position || got.Screen != cfg.Screen {
				t.Errorf("display %v gone, Config at %v on %q: reopened at %v on %q; want the Config's", p.Screen, cfg.Position, cfg.Screen, got.Position, got.Screen)
			}
			if got.Width != 800 || got.Height != 500 {
				t.Errorf("display %v gone: size %dx%d; want the saved 800x500 kept", p.Screen, got.Width, got.Height)
			}
		}
	}
	// The size is fit to the display the Config opens on.
	got := placed(Config{Screen: "laptop"}, placement{Width: 1080, Height: 1900, Screen: &placementScreen{ID: "gone"}}, unplugged)
	if got.Width != 1080 || got.Height != 875 {
		t.Fatalf("a tall window from a display gone is %dx%d; want 1080x875, the laptop's work area", got.Width, got.Height)
	}
}

func TestPlacedBringsAnUnreachablePositionIntoTheWorkArea(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		at   Point
		want Point
	}{
		{"past the right edge", Pt(5000, 100), Pt(640, 100)},
		{"below the bottom", Pt(100, 5000), Pt(100, 400)},
		{"title bar under the menu bar", Pt(100, -200), Pt(100, 25)},
		{"all but a sliver off the left", Pt(-790, 100), Pt(0, 100)},
	} {
		got := placed(Config{}, on("laptop", c.at), testScreens)
		if got.Screen != "laptop" || got.Position == nil || *got.Position != c.want || got.Width != 800 || got.Height != 500 {
			t.Errorf("%s: reopened on %q at %v %dx%d; want laptop at %v 800x500", c.name, got.Screen, got.Position, got.Width, got.Height, c.want)
		}
	}
	// Partly off the edge but within reach, it stays where the user left it.
	if got := placed(Config{}, on("laptop", Pt(1000, 100)), testScreens); got.Position == nil || *got.Position != Pt(1000, 100) {
		t.Errorf("a reachable window partly off the edge moved to %v", got.Position)
	}
}

func TestPlacedKeepsTheSizeWithinTheLimitsAndTheWorkArea(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name         string
		cfg          Config
		w, h         float64
		wantW, wantH int
	}{
		{"larger than the work area", Config{}, 3000, 2000, 1440, 875},
		{"beyond the maximum", Config{MaxWidth: 1000, MaxHeight: 500}, 1200, 800, 1000, 500},
		{"below the minimum", Config{MinWidth: 300, MinHeight: 200}, 100, 50, 300, 200},
		{"a minimum larger than the display", Config{MinWidth: 2000}, 3000, 600, 2000, 600},
		{"a fraction", Config{}, 640.6, 480.4, 641, 480},
	} {
		got := placed(c.cfg, placement{Width: c.w, Height: c.h, Screen: &placementScreen{ID: "laptop"}}, testScreens)
		if got.Width != c.wantW || got.Height != c.wantH {
			t.Errorf("%s: %dx%d; want %dx%d", c.name, got.Width, got.Height, c.wantW, c.wantH)
		}
	}
}

func TestPlacedReopensMaximizedOverTheNormalFrame(t *testing.T) {
	t.Parallel()
	p := on("s22-left", Pt(50, 60))
	p.Maximized = true
	got := placed(Config{Width: 800, Height: 600}, p, testScreens)
	if !got.Maximized {
		t.Fatal("a placement saved maximized reopened normal")
	}
	if got.Screen != "s22-left" || got.Position == nil || *got.Position != Pt(50, 60) || got.Width != 800 || got.Height != 500 {
		t.Fatalf("the frame Restore returns to is %v %dx%d on %q; want (50,60) 800x500 on s22-left", got.Position, got.Width, got.Height, got.Screen)
	}
	// What the user left wins over what the app asks for.
	if placed(Config{Maximized: true}, on("laptop", Pt(10, 30)), testScreens).Maximized {
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
		"no screens":     {on("laptop", Pt(100, 100)), nil},
		"no size":        {placement{Position: &Point{X: 100, Y: 100}, Screen: &placementScreen{ID: "laptop"}}, testScreens},
		"negative width": {placement{Width: -5, Height: 700, Screen: &placementScreen{ID: "laptop"}}, testScreens},
	} {
		got := placed(cfg, c.p, c.screens)
		if got.Position != cfg.Position || got.Screen != "" || got.Width != 640 || got.Height != 480 || !got.Maximized {
			t.Errorf("%s: the Config changed to %v on %q %dx%d maximized %t", name, got.Position, got.Screen, got.Width, got.Height, got.Maximized)
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
	w.trackPlacement(w.restorePlacement(testScreens), screenWithID(w.cfg.Screen, testScreens))
}

func TestARememberedPlacementComesBackAfterClose(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := rememberingApp(t, dir)
	openRemembering(a.Window)
	// What notePlacement takes from a window moved onto the left external
	// display and resized, then maximized there.
	a.remembered = placement{Position: &Point{X: 120, Y: 90}, Width: 1000, Height: 700, Screen: &placementScreen{ID: "s22-left", Name: "S22"}}
	a.remembered.Maximized = true
	a.Close()

	b := rememberingApp(t, dir)
	openRemembering(b.Window)
	if b.cfg.Screen != "s22-left" || b.cfg.Position == nil || *b.cfg.Position != Pt(120, 90) || b.cfg.Width != 1000 || b.cfg.Height != 700 || !b.cfg.Maximized {
		t.Fatalf("reopened on %q at %v %dx%d maximized %t; want s22-left (120,90) 1000x700 maximized", b.cfg.Screen, b.cfg.Position, b.cfg.Width, b.cfg.Height, b.cfg.Maximized)
	}
	// Closed again untouched, it keeps what it opened with.
	b.Close()
	c := rememberingApp(t, dir)
	openRemembering(c.Window)
	if c.cfg.Screen != "s22-left" || c.cfg.Position == nil || *c.cfg.Position != Pt(120, 90) || c.cfg.Width != 1000 || !c.cfg.Maximized {
		t.Fatalf("a window closed as it opened lost its placement: %q %v %dx%d maximized %t", c.cfg.Screen, c.cfg.Position, c.cfg.Width, c.cfg.Height, c.cfg.Maximized)
	}
}

func TestARememberedPlacementOnADisplaySinceUnpluggedOpensWhereTheConfigSays(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := rememberingApp(t, dir)
	openRemembering(a.Window)
	a.remembered = placement{Position: &Point{X: 400, Y: 300}, Width: 900, Height: 600, Screen: &placementScreen{ID: "projector"}}
	a.Close()

	b := rememberingApp(t, dir)
	openRemembering(b.Window)
	if b.cfg.Screen != "" || b.cfg.Position != nil || b.cfg.Width != 900 || b.cfg.Height != 600 {
		t.Fatalf("reopened on %q at %v %dx%d; want centered on the default display at the saved 900x600", b.cfg.Screen, b.cfg.Position, b.cfg.Width, b.cfg.Height)
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
