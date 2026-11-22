package ggui

import "github.com/ironpark/ggwd"

// Screen describes a display.
type Screen struct {
	// Name is the display's name, for people. Two displays of one model
	// may share it, and on Windows most are "Generic PnP Monitor".
	Name string
	// ID tells the display from every other connected, and the display
	// keeps it across restarts: what to save to open a window on it again,
	// with Config.Screen. It is empty in a browser.
	ID string
	// Position is the display's top-left corner in the desktop, which
	// every display is placed in, in logical pixels: on macOS from the main
	// display's corner, elsewhere the desktop's pixels divided by this
	// display's Scale. A window's Position is on its display, from here.
	Position Point
	// Size is the display's size in logical pixels.
	Size Size
	// WorkArea is the part of the display the menu bar, the dock and the
	// taskbar leave to windows, from the display's top-left corner.
	WorkArea Rect
	// Scale is the display's physical pixels per logical pixel.
	Scale float64
}

func screenOf(m *ggwd.Monitor) Screen {
	width, height := m.Size()
	x, y := m.Position()
	ax, ay, aw, ah := m.WorkArea()
	return Screen{
		Name: m.Name(), ID: m.ID(), Position: Pt(x, y),
		Size: Sz(width, height), WorkArea: Rct(Pt(ax, ay), Sz(aw, ah)),
		Scale: m.DeviceScaleFactor(),
	}
}

// monitorByID is the connected display whose ID is id, or nil.
func monitorByID(id string) *ggwd.Monitor {
	if id == "" || !appRunning.Load() {
		return nil
	}
	for _, m := range ggwd.AppendMonitors(nil) {
		if m.ID() == id {
			return m
		}
	}
	return nil
}

// Screen returns the display the window is on, or the zero Screen before
// it opens.
func (w *Window) Screen() Screen {
	if nw := w.window.Load(); nw != nil {
		if m := nw.Monitor(); m != nil {
			return screenOf(m)
		}
	}
	return Screen{}
}

// Screens returns the displays attached to the machine, the primary one
// first. It needs a running App: before Run it returns none.
func Screens() []Screen {
	if !appRunning.Load() {
		return nil
	}
	var out []Screen
	for _, m := range ggwd.AppendMonitors(nil) {
		out = append(out, screenOf(m))
	}
	return out
}
