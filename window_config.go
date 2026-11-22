package ggui

import (
	"image"
	"image/color"

	"github.com/ironpark/ggwd"
)

// Config describes a window: the one New opens, or one App.OpenWindow
// adds. Only Title, Width and Height are commonly set; the zero value of
// every other field is an ordinary decorated window centered on the screen.
type Config struct {
	Title  string
	Width  int
	Height int

	// The size limits in logical pixels. Zero means no limit.
	MinWidth  int
	MinHeight int
	MaxWidth  int
	MaxHeight int

	// Position is where the window opens on its screen, in logical pixels
	// from the screen's top-left corner; nil centers it.
	Position *Point
	// Screen is the ID of the display the window opens on (see Screen.ID),
	// which Position is on. Empty, or a display not connected, is the one
	// windows open on by default.
	Screen string

	// Remember names the window's placement, such as "main", so that it
	// reopens where it was last closed: on the same display, at the same
	// position and size, and maximized if it was, kept for the user under
	// os.UserConfigDir. A saved placement overrides Screen, Position,
	// Width, Height and Maximized, within the size limits and the display's
	// work area; one whose display is gone, or that would leave the window
	// out of reach, keeps the size and leaves the place to Screen and
	// Position. Each window of an app needs a name of its own. Empty, the
	// default, keeps nothing. A browser, and the viewer of ggui dev, keep
	// nothing either.
	Remember string

	Resizable bool
	// Frameless opens the window without its title bar and border.
	Frameless bool
	// TitlebarHidden runs the window's content under its title bar on
	// macOS, keeping the frame and the traffic lights: the bar is clear and
	// has no title, the app draws its own header there, and moves the
	// window by it with Window.StartDrag. TrafficLights, when set, is where
	// the close button's top-left corner goes, from the window's, in
	// logical pixels. Elsewhere the window has its title bar as usual.
	TitlebarHidden bool
	TrafficLights  *Point
	// AlwaysOnTop keeps the window above ordinary windows.
	AlwaysOnTop bool
	// Transparent lets what is behind the window show through wherever
	// nothing is painted. The window's background is clear unless
	// Background says otherwise. DirectX cannot present such a window, so
	// on a Windows machine that chose it the window fails to open.
	Transparent bool
	// Hidden opens the window without showing it; Show shows it.
	Hidden bool
	// Maximized opens the window maximized.
	Maximized bool
	// Unfocused opens the window without taking the keyboard focus.
	Unfocused bool
	// Icon is the window's icon in several sizes, where the platform shows
	// one; the platform picks the size closest to what it needs.
	Icon []image.Image

	Background color.Color // nil follows the root environment background
	Inspector  string      // a chord that toggles the widget inspector, such as "f1"; empty for none. Import ggui/inspect/panel for the panel.

	// Accessibility says when the window talks to the platform's
	// accessibility API. The zero value waits for an assistive technology
	// to attach and costs nothing while none has; see AccessibilityMode.
	Accessibility AccessibilityMode
}

func (c Config) withDefaults() Config {
	if c.Title == "" {
		c.Title = "ggui"
	}
	if c.Width == 0 {
		c.Width = 800
	}
	if c.Height == 0 {
		c.Height = 600
	}
	return c
}

// options is the ggfx form of c.
func (c Config) options() *ggwd.WindowOptions {
	o := &ggwd.WindowOptions{
		Title:       c.Title,
		Width:       float64(c.Width),
		Height:      float64(c.Height),
		Resizable:   c.Resizable,
		Undecorated: c.Frameless,
		Floating:    c.AlwaysOnTop,
		Transparent: c.Transparent,
		Hidden:      c.Hidden,
		Maximized:   c.Maximized,
		Unfocused:   c.Unfocused,
		MinWidth:    float64(c.MinWidth),
		MinHeight:   float64(c.MinHeight),
		MaxWidth:    float64(c.MaxWidth),
		MaxHeight:   float64(c.MaxHeight),
	}
	if c.Position != nil {
		o.Position = &ggwd.Point{X: c.Position.X, Y: c.Position.Y}
	}
	o.TitlebarHidden = c.TitlebarHidden
	if c.TrafficLights != nil {
		o.TrafficLights = &ggwd.Point{X: c.TrafficLights.X, Y: c.TrafficLights.Y}
	}
	return o
}

// WindowState is whether a window is shown normally, minimized, maximized
// or fullscreen.
type WindowState uint8

// The states a window can be in.
const (
	WindowNormal WindowState = iota
	WindowMinimized
	WindowMaximized
	WindowFullscreen
)
