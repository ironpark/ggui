package ggui

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/ironpark/ggui/internal/platform/remotedisplay"
)

// A window whose Config names a Remember key reopens where it was last
// closed. What it keeps is a placement: the frame the window had while it
// was last shown normally, which is what Restore returns a maximized window
// to, and whether it was maximized. A minimized or fullscreen window's frame
// is the system's, not the user's, so it is never taken.
//
// The placements of an app live in one JSON file under os.UserConfigDir, in
// a directory named for the app (see App.identity), keyed by the Remember
// names. Nothing about them may fail the app: a file that cannot be read or
// written is said once on stderr and otherwise ignored.

// placement is what Config.Remember keeps of one window.
type placement struct {
	// Position is where the window's content stood, in logical pixels from
	// the top-left corner of Screen, as Window.Position reports it; nil
	// leaves Config's, centered unless it says otherwise.
	Position *Point `json:"position,omitempty"`
	// Width and Height are the size of the content, in logical pixels.
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
	// Maximized reopens the window maximized, over the frame above.
	Maximized bool `json:"maximized,omitempty"`
	// Screen is the display Position is measured on; nil for the one a
	// window opens on.
	Screen *placementScreen `json:"screen,omitempty"`
}

// placementScreen names the display a placement was taken on, so that a
// position measured on another display is not taken for one on the display
// the window opens on. Its size is not compared: a display whose resolution
// changed is still the same one, and reachable decides what to keep there.
type placementScreen struct {
	Name string `json:"name"`
}

// placementReach is how much of a window, in logical pixels, must lie on
// the screen for its saved position to be kept: enough of its top edge to
// take hold of it and drag it back.
const placementReach = 40

// placed returns cfg with p applied, for a window that opens on screens[0],
// the display ggwd places every new window on: its size, kept within cfg's
// limits and the screen, and its maximized state. The saved position is
// kept only when it was taken on that display and the window's top edge is
// still reachable there; otherwise cfg's own placement stands, so a window
// last closed on a display since unplugged, or one that was made smaller,
// opens where the app would put it rather than out of reach. With no
// screens to measure against, or a placement without a size, cfg is
// returned as it is.
func placed(cfg Config, p placement, screens []Screen) Config {
	if len(screens) == 0 || !validLength(p.Width) || !validLength(p.Height) {
		return cfg
	}
	screen := screens[0].Size
	cfg.Width = int(math.Round(fitLength(p.Width, screen.W, cfg.MinWidth, cfg.MaxWidth)))
	cfg.Height = int(math.Round(fitLength(p.Height, screen.H, cfg.MinHeight, cfg.MaxHeight)))
	cfg.Maximized = p.Maximized
	if pos := p.Position; pos != nil && p.Screen.is(screens[0]) && reachable(*pos, Sz(float64(cfg.Width), float64(cfg.Height)), screen) {
		at := *pos
		cfg.Position = &at
	}
	return cfg
}

// validLength reports whether v is a size a window can have.
func validLength(v float64) bool { return v >= 1 && !math.IsInf(v, 0) }

// fitLength bounds v, a saved width or height, by the screen's and then by
// the window's limits, of which a zero one is none: the minimum wins, as the
// window could not be made smaller anyway.
func fitLength(v, screen float64, least, most int) float64 {
	if screen >= 1 {
		v = min(v, screen)
	}
	if most > 0 {
		v = min(v, float64(most))
	}
	return max(v, float64(least), 1)
}

// is reports whether the display s names is screen, or whether s names none,
// as a placement taken before the window knew its display does not.
func (s *placementScreen) is(screen Screen) bool {
	return s == nil || s.Name == screen.Name
}

// reachable reports whether a window of size at pos keeps enough of its top
// edge on a screen of size screen to be dragged.
func reachable(pos Point, size, screen Size) bool {
	if math.IsNaN(pos.X) || math.IsNaN(pos.Y) {
		return false
	}
	return pos.Y >= 0 && pos.Y <= screen.H-placementReach &&
		pos.X+size.W >= placementReach && pos.X <= screen.W-placementReach
}

// placementStore is the file an app's remembered placements live in. A
// store with no path keeps nothing. It is used on the UI thread only.
type placementStore struct {
	path string
}

// placementFile returns the store the app's windows remember their
// placements in, or nil where none is kept: in a browser, and in an app ggui
// dev shows in its viewer, whose one window is the viewer's.
func (a *App) placementFile() *placementStore {
	if a.placements == nil {
		a.placements = &placementStore{path: placementPath(a.identity())}
	}
	if a.placements.path == "" {
		return nil
	}
	return a.placements
}

// placementPath is where the placements of the app named id are kept, or ""
// where there is nowhere to keep them.
func placementPath(id string) string {
	if runtime.GOOS == "js" {
		return ""
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		placementTrouble(err)
		return ""
	}
	return filepath.Join(dir, placementDirName(id), "windows.json")
}

// placementDirName makes id a single directory name: whatever a file name
// cannot hold on some system becomes an underscore.
func placementDirName(id string) string {
	name := strings.Map(func(r rune) rune {
		switch {
		case r < ' ', strings.ContainsRune(`/\:*?"<>|`, r):
			return '_'
		}
		return r
	}, id)
	name = strings.Trim(name, ". ")
	if name == "" {
		return "ggui"
	}
	return name
}

// identity is the name the app's placements are kept under: the id given
// to SingleInstance, which is the app's reverse-DNS name, or else the main
// window's title as New was given it, or else the executable's name.
func (a *App) identity() string {
	if a.instanceID != "" {
		return a.instanceID
	}
	if a.title != "" {
		return a.title
	}
	if exe, err := os.Executable(); err == nil {
		return strings.TrimSuffix(filepath.Base(exe), ".exe")
	}
	return "ggui"
}

// read returns every placement in the file, or none when there is no file
// or it cannot be made sense of.
func (s *placementStore) read() map[string]placement {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			placementTrouble(err)
		}
		return nil
	}
	var all map[string]placement
	if err := json.Unmarshal(data, &all); err != nil {
		placementTrouble(fmt.Errorf("%s: %w", s.path, err))
		return nil
	}
	return all
}

// load returns the placement kept under name.
func (s *placementStore) load(name string) (placement, bool) {
	p, ok := s.read()[name]
	return p, ok
}

// save keeps p under name, beside the placements of the app's other
// windows. The file is replaced whole, so that a crash while it is written
// leaves the last one rather than half of this one; one that could not be
// read is replaced by one holding p alone.
func (s *placementStore) save(name string, p placement) {
	all := s.read()
	if all == nil {
		all = map[string]placement{}
	}
	all[name] = p
	data, err := json.MarshalIndent(all, "", "\t")
	if err == nil {
		err = replaceFile(s.path, data)
	}
	if err != nil {
		placementTrouble(err)
	}
}

// replaceFile writes data to path through a file beside it, renamed over
// path once it is complete.
func replaceFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".windows-*.json")
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(f.Name(), path)
	}
	if err != nil {
		os.Remove(f.Name())
	}
	return err
}

// placementTroubled says the first trouble with the placements file once.
var placementTroubled sync.Once

// placementTrouble says on stderr, the first time only, that the windows'
// placements could not be kept: the app runs on as if none were asked for.
func placementTrouble(err error) {
	placementTroubled.Do(func() {
		fmt.Fprintf(os.Stderr, "ggui: window placements are not kept: %v\n", err)
	})
}

// restorePlacement applies the placement kept under the window's Remember
// name to the Config it is about to open with, measured against screens,
// and returns the store to keep its placement in when it closes: nil when
// it keeps none, as under a WindowProbe, which belongs to no app.
func (w *Window) restorePlacement(screens []Screen) *placementStore {
	if w.cfg.Remember == "" || w.app == nil {
		return nil
	}
	if remotedisplay.On() {
		remoteNotice("a window's remembered placement is not kept")
		return nil
	}
	store := w.app.placementFile()
	if store == nil {
		return nil
	}
	if p, ok := store.load(w.cfg.Remember); ok {
		w.cfg = placed(w.cfg, p, screens)
	}
	return store
}

// trackPlacement starts keeping the window's placement in store, from the
// Config it opened with; the window's events follow it from there.
func (w *Window) trackPlacement(store *placementStore) {
	if store == nil {
		return
	}
	w.remember = store
	w.remembered = placement{Width: float64(w.cfg.Width), Height: float64(w.cfg.Height), Maximized: w.cfg.Maximized}
	if w.cfg.Position != nil {
		at := *w.cfg.Position
		w.remembered.Position = &at
	}
}

// notePlacement takes the native window's placement as it is now: its
// frame while it is shown normally, and only that it is maximized while it
// is, keeping the frame Restore returns it to. A minimized or fullscreen
// window, or one already gone, changes nothing.
func (w *Window) notePlacement() {
	if w.remember == nil {
		return
	}
	nw := w.window.Load()
	if nw == nil || nw.IsMinimized() || nw.IsFullscreen() {
		return
	}
	if nw.IsMaximized() {
		w.remembered.Maximized = true
		return
	}
	width, height := nw.Size()
	if width < 1 || height < 1 {
		return
	}
	x, y := nw.Position()
	p := placement{Position: &Point{X: x, Y: y}, Width: width, Height: height}
	if m := nw.Monitor(); m != nil {
		p.Screen = &placementScreen{Name: m.Name()}
	}
	w.remembered = p
}

// savePlacement keeps the window's placement as it closes, once.
func (w *Window) savePlacement() {
	if w.remember == nil {
		return
	}
	// The last move may not have reached the window as an event yet.
	w.notePlacement()
	w.remember.save(w.cfg.Remember, w.remembered)
	w.remember = nil
}
