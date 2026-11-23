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

	"github.com/ironpark/ggui/internal/devstate"
	"github.com/ironpark/ggui/internal/platform/remotedisplay"
	"github.com/ironpark/ggui/internal/robustio"
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
	// Screen is the display the window was on, which Position is on; nil
	// when the window could not tell.
	Screen *placementScreen `json:"screen,omitempty"`
	// Asked is what the Config asked for when the window that left this
	// placement opened, for ggui dev to tell a Config edited since; see
	// placementAsk.
	Asked *placementAsk `json:"asked,omitempty"`
}

// placementAsk is the part of a Config a placement overrides, as the code
// had it. An app run by ggui dev compares it with its Config now: where
// they differ the code was edited to place the window otherwise, and the
// window opens as the code says, which is what was just asked for, rather
// than where the last run left it. An app run by itself never looks: its
// user's placement outlives a release that changes the defaults.
type placementAsk struct {
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Position  *Point `json:"position,omitempty"`
	Screen    string `json:"screen,omitempty"`
	Maximized bool   `json:"maximized,omitempty"`
}

// askOf returns what cfg asks for of a window's placement.
func askOf(cfg Config) placementAsk {
	a := placementAsk{Width: cfg.Width, Height: cfg.Height, Screen: cfg.Screen, Maximized: cfg.Maximized}
	if cfg.Position != nil {
		at := *cfg.Position
		a.Position = &at
	}
	return a
}

// same reports whether a and b ask for the same placement.
func (a placementAsk) same(b placementAsk) bool {
	if (a.Position == nil) != (b.Position == nil) || a.Position != nil && *a.Position != *b.Position {
		return false
	}
	return a.Width == b.Width && a.Height == b.Height && a.Screen == b.Screen && a.Maximized == b.Maximized
}

// placementScreen names the display a placement was taken on by its ID,
// which tells it from another of the same model and which it keeps across
// restarts. The position is kept on the display rather than in the
// desktop, so that a display moved in the arrangement takes its windows
// with it. The name is for whoever reads the file.
type placementScreen struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

// placementReach is how much of a window, in logical pixels, must lie in
// the work area for its saved position to be kept as it is: enough of its
// top edge to take hold of it and drag it.
const placementReach = 40

// placed returns cfg with p applied, among the displays screens: the
// display, its size, kept within cfg's limits and the display's work area,
// its maximized state, and its position. Where the display is still
// connected the window opens on it, at its saved position, or moved just
// far enough into the work area when the display shrank and the position
// would leave it out of reach. Where the display is gone, or the placement
// does not say which it was, the size is kept for the display cfg opens on
// and the place left to cfg. With no screens to measure against, or a
// placement without a size, cfg is returned as it is.
func placed(cfg Config, p placement, screens []Screen) Config {
	if len(screens) == 0 || !validLength(p.Width) || !validLength(p.Height) {
		return cfg
	}
	saved, connected := p.Screen.among(screens)
	target := saved
	if !connected {
		target = screenWithID(cfg.Screen, screens)
	}
	area := target.usable()
	size := Sz(fitLength(p.Width, area.Size.W, cfg.MinWidth, cfg.MaxWidth), fitLength(p.Height, area.Size.H, cfg.MinHeight, cfg.MaxHeight))
	cfg.Width, cfg.Height = int(math.Round(size.W)), int(math.Round(size.H))
	cfg.Maximized = p.Maximized
	if !connected {
		return cfg
	}
	cfg.Screen = saved.ID
	if pos := p.Position; pos != nil && !math.IsNaN(pos.X) && !math.IsNaN(pos.Y) {
		at := *pos
		if !reachable(at, size, area) {
			at = into(at, size, area)
		}
		cfg.Position = &at
	}
	return cfg
}

// validLength reports whether v is a size a window can have.
func validLength(v float64) bool { return v >= 1 && !math.IsInf(v, 0) }

// fitLength bounds v, a saved width or height, by the work area's and then
// by the window's limits, of which a zero one is none: the minimum wins, as
// the window could not be made smaller anyway.
func fitLength(v, area float64, least, most int) float64 {
	if area >= 1 {
		v = min(v, area)
	}
	if most > 0 {
		v = min(v, float64(most))
	}
	return max(v, float64(least), 1)
}

// among returns the display of screens that s names, and whether it is
// connected: by ID alone, as two displays of one model share a name and a
// rearranged one has moved.
func (s *placementScreen) among(screens []Screen) (Screen, bool) {
	if s == nil || s.ID == "" {
		return Screen{}, false
	}
	for _, c := range screens {
		if c.ID == s.ID {
			return c, true
		}
	}
	return Screen{}, false
}

// screenWithID returns the display of screens whose ID is id, or the first,
// the primary, which stands for the one a window opens on by default.
func screenWithID(id string, screens []Screen) Screen {
	if id != "" {
		for _, s := range screens {
			if s.ID == id {
				return s
			}
		}
	}
	return screens[0]
}

// usable returns the part of s windows are placed in, from its top-left
// corner: its work area, or all of it where that is not known.
func (s Screen) usable() Rect {
	if s.WorkArea.Size.W >= 1 && s.WorkArea.Size.H >= 1 {
		return s.WorkArea
	}
	return Rct(Point{}, s.Size)
}

// reachable reports whether a window of size at pos keeps enough of its top
// edge in area to be dragged.
func reachable(pos Point, size Size, area Rect) bool {
	top, left := area.Origin.Y, area.Origin.X
	return pos.Y >= top && pos.Y <= top+area.Size.H-placementReach &&
		pos.X+size.W >= left+placementReach && pos.X <= left+area.Size.W-placementReach
}

// into moves a window of size at pos the least distance that puts all of it
// in area, which it fits.
func into(pos Point, size Size, area Rect) Point {
	pos.X = max(min(pos.X, area.Origin.X+area.Size.W-size.W), area.Origin.X)
	pos.Y = max(min(pos.Y, area.Origin.Y+area.Size.H-size.H), area.Origin.Y)
	return pos
}

// placementStore is the file an app's remembered placements live in. A
// store with no path keeps nothing. It is used on the UI thread only.
type placementStore struct {
	path string
	// dev is set in an app ggui dev runs, where a Config edited since a
	// placement was kept wins over it; see placementAsk.
	dev bool
}

// placementFile returns the store the app's windows remember their
// placements in, or nil where none is kept: in a browser, and in an app ggui
// dev shows in its viewer, whose one window is the viewer's.
func (a *App) placementFile() *placementStore {
	if a.placements == nil {
		a.placements = &placementStore{path: placementPath(a.identity()), dev: os.Getenv(devstate.Env) != ""}
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

// identity is the name the app's placements are kept under: the app's id
// where it was given one, as AppID orders them (the bundle identifier, the
// id ggui build linked in, the id given to this App's SingleInstance), or
// else the main window's title as New was given it, or else the
// executable's name.
func (a *App) identity() string {
	if id, src := chooseAppID(bundleID(), buildID, a.instanceID, ""); src > idExecutable {
		return id
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
		err = replaceFile(s.path, data, 0o600)
	}
	if err != nil {
		placementTrouble(err)
	}
}

// replaceFile writes data to path through a file beside it, renamed over
// path once complete, so that a reader never sees half of it, and gives it
// mode. It makes the directory if it is missing.
func replaceFile(path string, data []byte, mode fs.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(f.Name(), mode)
	}
	if err == nil {
		err = robustio.Rename(f.Name(), path)
	}
	if err != nil {
		if rerr := os.Remove(f.Name()); rerr != nil && !errors.Is(rerr, fs.ErrNotExist) {
			err = errors.Join(err, rerr)
		}
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

// placementEdited says once that a Config edited under ggui dev won over a
// kept placement.
var placementEdited sync.Once

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
	w.asked = askOf(w.cfg)
	if p, ok := store.load(w.cfg.Remember); ok {
		if store.dev && p.Asked != nil && !p.Asked.same(w.asked) {
			placementEdited.Do(func() {
				fmt.Fprintf(os.Stderr, "ggui: the window %q opens as its Config now says, not where it was last closed: the Config was edited\n", w.cfg.Remember)
			})
		} else {
			w.cfg = placed(w.cfg, p, screens)
		}
	}
	return store
}

// trackPlacement starts keeping the window's placement in store, from the
// Config it opened with on screen, the display it opened on; the window's
// events follow it from there.
func (w *Window) trackPlacement(store *placementStore, screen Screen) {
	if store == nil {
		return
	}
	w.remember = store
	w.remembered = placement{Width: float64(w.cfg.Width), Height: float64(w.cfg.Height), Maximized: w.cfg.Maximized}
	if w.cfg.Position != nil {
		at := *w.cfg.Position
		w.remembered.Position = &at
	}
	w.remembered.Screen = placementScreenOf(screen)
}

// placementScreenOf names screen in a placement, or is nil for a display
// with no ID.
func placementScreenOf(screen Screen) *placementScreen {
	if screen.ID == "" {
		return nil
	}
	return &placementScreen{ID: screen.ID, Name: screen.Name}
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
		p.Screen = placementScreenOf(screenOf(m))
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
	asked := w.asked
	w.remembered.Asked = &asked
	w.remember.save(w.cfg.Remember, w.remembered)
	w.remember = nil
}
