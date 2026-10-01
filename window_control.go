package ggui

import (
	"image"

	"github.com/ironpark/ggfx"
)

// The methods here change the native window. Called before the window
// opens, they change the Config it opens with instead, where there is a
// field for it; the rest wait for nothing and do nothing. Call them from
// the UI thread.

// Title returns the window's title.
func (w *Window) Title() string { return w.cfg.Title }

// SetTitle changes the window's title.
func (w *Window) SetTitle(title string) {
	w.cfg.Title = title
	if nw := w.window.Load(); nw != nil {
		nw.SetTitle(title)
	}
}

// Size returns the size of the window's content, in logical pixels.
func (w *Window) Size() Size {
	if nw := w.window.Load(); nw != nil {
		width, height := nw.Size()
		return Sz(float64(width), float64(height))
	}
	return Sz(float64(w.cfg.Width), float64(w.cfg.Height))
}

// SetSize resizes the window's content to size, in logical pixels.
func (w *Window) SetSize(size Size) {
	w.cfg.Width, w.cfg.Height = int(size.W), int(size.H)
	w.native(func(nw *ggfx.Window) { nw.SetSize(w.cfg.Width, w.cfg.Height) })
}

// Position returns where the window is on the screen, in logical pixels.
func (w *Window) Position() Point {
	if nw := w.window.Load(); nw != nil {
		x, y := nw.Position()
		return Pt(float64(x), float64(y))
	}
	if w.cfg.Position != nil {
		return *w.cfg.Position
	}
	return Point{}
}

// SetPosition moves the window to p on the screen, in logical pixels.
func (w *Window) SetPosition(p Point) {
	w.cfg.Position = &p
	if nw := w.window.Load(); nw != nil {
		nw.SetPosition(int(p.X), int(p.Y))
	}
}

// SetSizeLimits bounds the size the user can resize the window's content
// to, in logical pixels. A zero width or height means no limit.
func (w *Window) SetSizeLimits(minSize, maxSize Size) {
	w.cfg.MinWidth, w.cfg.MinHeight = int(minSize.W), int(minSize.H)
	w.cfg.MaxWidth, w.cfg.MaxHeight = int(maxSize.W), int(maxSize.H)
	limit := func(v int) int {
		if v <= 0 {
			return -1
		}
		return v
	}
	w.native(func(nw *ggfx.Window) {
		nw.SetSizeLimits(limit(w.cfg.MinWidth), limit(w.cfg.MinHeight), limit(w.cfg.MaxWidth), limit(w.cfg.MaxHeight))
	})
}

// SetResizable sets whether the user can resize the window.
func (w *Window) SetResizable(resizable bool) {
	w.cfg.Resizable = resizable
	if nw := w.window.Load(); nw != nil {
		nw.SetResizable(resizable)
	}
}

// SetFrameless shows or hides the window's title bar and border.
func (w *Window) SetFrameless(frameless bool) {
	w.cfg.Frameless = frameless
	w.native(func(nw *ggfx.Window) { nw.SetDecorated(!frameless) })
}

// SetAlwaysOnTop sets whether the window stays above ordinary windows.
func (w *Window) SetAlwaysOnTop(on bool) {
	w.cfg.AlwaysOnTop = on
	if nw := w.window.Load(); nw != nil {
		nw.SetFloating(on)
	}
}

// SetIcon sets the window's icon, in several sizes; see Config.Icon.
func (w *Window) SetIcon(icon []image.Image) {
	w.cfg.Icon = icon
	if nw := w.window.Load(); nw != nil {
		nw.SetIcon(icon)
	}
}

// Minimize minimizes the window.
func (w *Window) Minimize() { w.native((*ggfx.Window).Minimize) }

// Maximize maximizes the window.
func (w *Window) Maximize() {
	w.cfg.Maximized = true
	w.native((*ggfx.Window).Maximize)
}

// Restore brings the window back from being minimized or maximized.
func (w *Window) Restore() {
	w.cfg.Maximized = false
	w.native((*ggfx.Window).Restore)
}

// SetFullscreen puts the window into fullscreen or takes it out.
func (w *Window) SetFullscreen(on bool) {
	w.native(func(nw *ggfx.Window) { nw.SetFullscreen(on) })
}

// Show shows a hidden window.
func (w *Window) Show() {
	w.cfg.Hidden = false
	w.native((*ggfx.Window).Show)
}

// Hide hides the window without closing it.
func (w *Window) Hide() {
	w.cfg.Hidden = true
	w.native((*ggfx.Window).Hide)
}

// Visible reports whether the window is shown.
func (w *Window) Visible() bool {
	if nw := w.window.Load(); nw != nil {
		return nw.IsVisible()
	}
	return !w.cfg.Hidden
}

// Focus brings the window to the front and gives it the keyboard focus.
func (w *Window) Focus() { w.native((*ggfx.Window).Focus) }

// RequestAttention asks for the user's attention, such as by bouncing the
// dock icon or flashing the taskbar button, without taking the focus.
func (w *Window) RequestAttention() { w.native((*ggfx.Window).RequestAttention) }

// Focused follows whether the window has the keyboard focus.
func (w *Window) Focused() Readable[bool] { return w.focused }

// Viewport follows the size of the window's content, in logical pixels.
func (w *Window) Viewport() Readable[Size] { return w.viewport }

// State follows whether the window is minimized, maximized or fullscreen.
func (w *Window) State() Readable[WindowState] { return w.state }

// native calls fn with the native window, when there is one. What fn
// changes comes back as events: a ResizeEvent for the size and a
// WindowStateEvent for the state, once the platform has animated it.
func (w *Window) native(fn func(*ggfx.Window)) {
	if nw := w.window.Load(); nw != nil {
		fn(nw)
	}
}

// windowState converts the engine's window state.
func windowState(s ggfx.WindowState) WindowState {
	switch s {
	case ggfx.WindowStateMinimized:
		return WindowMinimized
	case ggfx.WindowStateMaximized:
		return WindowMaximized
	case ggfx.WindowStateFullscreen:
		return WindowFullscreen
	}
	return WindowNormal
}

// Screen describes a display.
type Screen struct {
	Name string
	// Size is the display's size in logical pixels.
	Size Size
	// Scale is the display's physical pixels per logical pixel.
	Scale float64
}

func screenOf(m *ggfx.MonitorType) Screen {
	width, height := m.Size()
	return Screen{Name: m.Name(), Size: Sz(float64(width), float64(height)), Scale: m.DeviceScaleFactor()}
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
	for _, m := range ggfx.AppendMonitors(nil) {
		out = append(out, screenOf(m))
	}
	return out
}
