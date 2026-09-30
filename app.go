// Package ggui is a cross-platform GUI framework for Go: Flutter's widget tree
// and layout model, driven by Svelte-style reactivity, rendered by ggfx.
package ggui

import (
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui/a11y"
	"github.com/ironpark/ggui/internal/nativemenu"
	"github.com/ironpark/ggui/internal/reactive"
	"github.com/ironpark/ggui/runtime"
)

// App is the process's GUI: the event loop and the windows it drives. New
// makes one with its main window, which App embeds, so that app.Shortcut
// and app.Post reach that window; OpenWindow adds more. Every window runs
// on the one UI thread and shares the one reactive graph.
//
// Run ends when the last window closes, or when Close or Quit is called.
type App struct {
	*Window // the main window, the one New opened

	mu         sync.Mutex // guards windows, which Quit reads from any goroutine
	windows    []*Window  // open or waiting for Run, in the order they were made
	running    bool       // the engine is running, so native windows may be made
	quit       atomic.Bool
	focus      atomic.Pointer[Window] // the window that last had the focus; read by native menus
	menu       *menuSet               // the menu bar's
	trays      []*Tray
	nextTray   int
	actions    map[int]MenuItem // every menu action installed, by id
	nextAction int

	releaseInstance func() // gives up SingleInstance's claim

	wakeQueued atomic.Bool // a WakeEvent is on its way
	draining   atomic.Bool // posted work is running outside a frame
}

// New creates an App whose main window renders the tree returned by build.
func New(cfg Config, build Builder) *App {
	a := &App{}
	a.Window = newWindow(a, cfg, build)
	a.windows = []*Window{a.Window}
	return a
}

// Setup registers fn to run under the main window's root owner before the
// first build, so derived values and watchers have an owner and are
// disposed by Close. Call it before Run.
//
//	app.Setup(func() {
//	    ggui.Watch(background, func(c color.Color) {
//	        ggui.SetEnv(ggui.Untrack(ggui.UseEnv).With(ggui.BackgroundKey, c))
//	    })
//	})
func (a *App) Setup(fn func()) *App {
	a.Window.Setup(fn)
	return a
}

// SetDialogs replaces the main window's file dialogs; see Window.SetDialogs.
func (a *App) SetDialogs(p runtime.Dialogs) *App {
	a.Window.SetDialogs(p)
	return a
}

// SetClipboard replaces the main window's clipboard; see
// Window.SetClipboard.
func (a *App) SetClipboard(c runtime.Clipboard) *App {
	a.Window.SetClipboard(c)
	return a
}

// OpenWindow adds a window that renders the tree returned by build. Before
// Run the window waits and opens with the main one; while the app runs it
// opens now, and build runs before OpenWindow returns. Call it from the UI
// thread: from Setup, a handler, a shortcut or posted work.
//
// The window has a root owner of its own, disposed when it closes, and its
// own shortcuts, close handlers, dialogs and inspector. State it shares
// with other windows is ordinary signals: a write in one repaints every
// window that reads it.
//
//	ggui.UseWindow().App().OpenWindow(ggui.Config{Title: "Preferences"}, prefs)
func (a *App) OpenWindow(cfg Config, build Builder) (*Window, error) {
	w := newWindow(a, cfg, build)
	if !NativeMenu() {
		a.addMenuShortcuts(w)
	}
	if a.running {
		if err := w.open(); err != nil {
			return nil, err
		}
	}
	a.mu.Lock()
	a.windows = append(a.windows, w)
	a.mu.Unlock()
	return w, nil
}

// Windows returns the windows that are open, or waiting for Run to open
// them, in the order they were made.
func (a *App) Windows() []*Window {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.DeleteFunc(slices.Clone(a.windows), (*Window).Closed)
}

// Close closes every window, disposing everything they built, and ends Run.
// Call it from the UI thread; from a goroutine, call Quit.
func (a *App) Close() {
	a.quit.Store(true)
	for _, w := range a.Windows() {
		w.Close()
	}
	a.release(a.menu)
	a.menu = nil
	for _, t := range slices.Clone(a.trays) {
		t.Remove()
	}
	if a.releaseInstance != nil {
		a.releaseInstance()
		a.releaseInstance = nil
	}
	reactive.UnmarkUIThread()
}

// Quit ends Run as Close does, from any goroutine: the windows close at the
// start of the next event, on the UI thread. It does not ask the windows'
// OnCloseRequest handlers.
func (a *App) Quit() {
	a.quit.Store(true)
	for _, w := range a.Windows() {
		w.requestFrame()
	}
}

// Run opens the windows and blocks until the last one closes, disposing
// owned resources on return, including when the engine returns an error.
func Run(cfg Config, build Builder) error {
	return New(cfg, build).Run()
}

// Run opens the windows and blocks until the last one closes, disposing
// owned resources on return, including when the engine returns an error.
func (a *App) Run() error {
	defer func() {
		a.running = false
		a.Close()
		appRunning.Store(false)
	}()
	appRunning.Store(true)
	// Holding a key on macOS pops up the accent menu, as it does in every
	// text field on the platform; text editing relies on it.
	return ggfx.Run(ggfx.HandlerFunc(a.handleEvent), &ggfx.RunOptions{ApplePressAndHoldEnabled: true})
}

// handleEvent is the App's ggfx.Handler. It opens the windows made before
// Run when the engine starts and hands every other event to the window it
// is for.
func (a *App) handleEvent(ev ggfx.Event) error {
	if a.quit.Load() {
		a.Close()
		return ggfx.Termination
	}
	if _, ok := ev.(ggfx.WakeEvent); ok {
		return a.drain()
	}
	if _, ok := ev.(ggfx.StartEvent); ok {
		a.running = true
		if Appearance(appearance.Load()) != AppearanceSystem {
			applyAppearance()
		}
		for _, w := range a.Windows() {
			if err := w.open(); err != nil {
				return err
			}
		}
		// The menu bar and the status bar exist once the engine has a
		// window.
		nativemenu.Handle(a.menuSelected)
		if a.menu != nil && nativemenu.Supported() {
			nativemenu.Set(a.menu.native)
		}
		for _, t := range a.trays {
			t.install()
		}
		return nil
	}
	w := a.windowFor(eventWindow(ev))
	if w == nil {
		return nil
	}
	if f, ok := ev.(ggfx.FocusEvent); ok && f.Focused {
		a.focus.Store(w)
	}
	if err := w.handle(ev); err != nil {
		return err
	}
	if _, ok := ev.(ggfx.FrameEvent); ok {
		if err := a.follow(w); err != nil {
			return err
		}
	}
	a.forgetClosed()
	if a.quit.Load() {
		a.Close()
		return ggfx.Termination
	}
	return nil
}

// wakeDelay is how long a post made by posted work waits to be run: a
// frame, so that work that keeps posting itself cannot hold the loop.
const wakeDelay = 16 * time.Millisecond

// wakeUp makes the app run posted work soon, whichever window it was
// posted to and whether or not that window draws. It is safe from any
// goroutine.
func (a *App) wakeUp() {
	if a.wakeQueued.Swap(true) {
		return
	}
	if a.draining.Load() {
		time.AfterFunc(wakeDelay, ggfx.Wake)
		return
	}
	ggfx.Wake()
}

// drain runs every window's posted work, settles what it changed and asks
// the windows for frames to paint it. It is what a WakeEvent does, so that
// work posted to a window the platform sends no frames, such as a hidden
// or covered one, still runs.
func (a *App) drain() error {
	a.wakeQueued.Store(false)
	if !a.running {
		// The windows run their work from their first frames.
		return nil
	}
	a.draining.Store(true)
	defer a.draining.Store(false)
	for _, w := range a.Windows() {
		if !w.hasPosted() {
			continue
		}
		running.Store(&w.frameLoop)
		w.runPosted()
		if err := w.catchUp(); err != nil {
			return err
		}
		w.requestFrame()
	}
	a.forgetClosed()
	if a.quit.Load() {
		a.Close()
		return ggfx.Termination
	}
	return nil
}

// windowFor is the open window whose native window is nw.
func (a *App) windowFor(nw *ggfx.Window) *Window {
	if nw == nil {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, w := range a.windows {
		if w.window.Load() == nw && !w.closed {
			return w
		}
	}
	return nil
}

// follow brings every other window up to date with what w's frame wrote,
// since state is shared and one of them may read it. Each settles now, so
// its effects run even while the platform sends it no frames, as it does
// for a window that is covered or minimized, and paints at its next frame.
func (a *App) follow(w *Window) error {
	for _, o := range a.Windows() {
		if o == w || o.seenGen == reactive.LayoutGen() {
			continue
		}
		if err := o.catchUp(); err != nil {
			return err
		}
		o.requestFrame()
	}
	running.Store(&w.frameLoop)
	return nil
}

// forgetClosed drops the windows that have closed.
func (a *App) forgetClosed() {
	a.mu.Lock()
	a.windows = slices.DeleteFunc(a.windows, (*Window).Closed)
	a.mu.Unlock()
}

// eventWindow is the window ev is for, or nil for an event that has none.
func eventWindow(ev ggfx.Event) *ggfx.Window {
	switch ev := ev.(type) {
	case ggfx.FrameEvent:
		return ev.Window
	case ggfx.ResizeEvent:
		return ev.Window
	case ggfx.FocusEvent:
		return ev.Window
	case ggfx.CloseEvent:
		return ev.Window
	case ggfx.KeyEvent:
		return ev.Window
	case ggfx.TextEvent:
		return ev.Window
	case ggfx.CompositionEvent:
		return ev.Window
	case ggfx.MouseMoveEvent:
		return ev.Window
	case ggfx.MouseButtonEvent:
		return ev.Window
	case ggfx.ScrollEvent:
		return ev.Window
	case ggfx.TouchEvent:
		return ev.Window
	case ggfx.DragEvent:
		return ev.Window
	case ggfx.DropEvent:
		return ev.Window
	}
	return nil
}

// AccessibilityMode says when an App talks to the platform's accessibility
// API. It is defined by the bridge in the a11y package; these are the same
// values under the names ggui has always used for them.
type AccessibilityMode = a11y.Mode

// When the bridge is active.
const (
	// AccessibilityAuto turns the bridge on while an assistive technology
	// is attached, and off again when the last one leaves.
	AccessibilityAuto = a11y.Auto

	// AccessibilityAlways keeps the bridge active, which is what a tool
	// such as Accessibility Inspector or a screenshot run needs.
	AccessibilityAlways = a11y.Always

	// AccessibilityOff disables the native bridge entirely.
	AccessibilityOff = a11y.Off
)

// appRunning reports whether Run has started, which is when platform
// services such as the IME may be used.
var appRunning atomic.Bool

var mouseButtons = []MouseButton{MouseButtonLeft, MouseButtonRight, MouseButtonMiddle}
