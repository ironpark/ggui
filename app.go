// Package ggui is a cross-platform GUI framework for Go: Flutter's widget tree
// and layout model, driven by Svelte-style reactivity, rendered by ggfx.
package ggui

import (
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ironpark/ggui/a11y"
	"github.com/ironpark/ggui/internal/nativemenu"
	"github.com/ironpark/ggui/internal/reactive"
	"github.com/ironpark/ggui/runtime"
	"github.com/ironpark/ggwd"
)

// App is the process's GUI: the event loop and the windows it drives. New
// makes one with its main window, which App embeds, so that app.Shortcut
// and app.Post reach that window; OpenWindow adds more. Every window runs
// on the one UI thread and in the process's reactive runtime, which a
// Probe does not share.
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
	quitAsked  []func()         // OnQuitRequest's
	reopened   []func()         // OnReopen's
	actions    map[int]MenuItem // every menu action installed, by id
	nextAction int

	releaseInstance func() // gives up SingleInstance's claim

	wakeQueued atomic.Bool // a WakeEvent is on its way
	draining   atomic.Bool // posted work is running outside a frame
}

// New creates an App whose main window renders the tree returned by build.
func New(cfg Config, build Builder) *App {
	// Every window of the app builds in one reactive runtime, made beside
	// the Base of the goroutine that called New, so that state and widgets
	// main built before Run are flushed by the app's frames.
	rt := reactive.NewRuntime()
	worldOf(rt).nativeFonts = true // made here, before the engine's goroutine can ask for it
	// The interface and fallback fonts take a fifth of a second to read and
	// parse. Begun now, they load while the window opens rather than in its
	// first frame, which waits only for what is left.
	go func() {
		systemUIFonts()
		SystemFonts()
	}()
	a := &App{}
	a.Window = newWindow(a, rt, cfg, build)
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
	w := newWindow(a, a.rt, cfg, build)
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
		runningApp.CompareAndSwap(a, nil)
	}()
	appRunning.Store(true)
	runningApp.Store(a)
	// Holding a key on macOS pops up the accent menu, as it does in every
	// text field on the platform; text editing relies on it.
	return ggwd.Run(ggwd.HandlerFunc(a.handleEvent), &ggwd.RunOptions{MacOS: ggwd.MacOSOptions{PressAndHold: true}})
}

// handleEvent is the App's ggwd.Handler. It opens the windows made before
// Run when the engine starts and hands every other event to the window it
// is for.
func (a *App) handleEvent(ev ggwd.Event) error {
	if a.quit.Load() {
		a.Close()
		return ggwd.Termination
	}
	if _, ok := ev.(ggwd.WakeEvent); ok {
		return a.drain()
	}
	if _, ok := ev.(ggwd.StartEvent); ok {
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
	switch ev.(type) {
	case ggwd.QuitEvent:
		a.postAll(a.quitAsked)
		return nil
	case ggwd.ReopenEvent:
		a.postAll(a.reopened)
		return nil
	}
	w := a.windowFor(eventWindow(ev))
	if w == nil {
		return nil
	}
	if f, ok := ev.(ggwd.FocusEvent); ok && f.Focused {
		a.focus.Store(w)
	}
	if err := w.handle(ev); err != nil {
		return err
	}
	if _, ok := ev.(ggwd.FrameEvent); ok {
		if err := a.follow(w); err != nil {
			return err
		}
	}
	a.forgetClosed()
	if a.quit.Load() {
		a.Close()
		return ggwd.Termination
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
		time.AfterFunc(wakeDelay, ggwd.Wake)
		return
	}
	ggwd.Wake()
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
		setRunning(&w.frameLoop)
		state, layout := reactive.StateGen(), layoutGen()
		w.runPosted()
		if err := w.catchUp(); err != nil {
			return err
		}
		// catchUp settles the layout, so the frame cannot tell it changed:
		// work that changed it is painted whole. Work that changed nothing
		// leaves the frame free to be painted in part.
		stateChanged := reactive.StateGen() != state
		layoutChanged := layoutGen() != layout
		if stateChanged || layoutChanged {
			var reason dirtyReason
			if stateChanged {
				reason |= dirtySignal
			}
			if layoutChanged {
				reason |= dirtyLayout
			}
			w.markDirty(reason)
		} else {
			w.wakeFrame()
		}
	}
	a.forgetClosed()
	if a.quit.Load() {
		a.Close()
		return ggwd.Termination
	}
	return nil
}

// windowFor is the open window whose native window is nw.
func (a *App) windowFor(nw *ggwd.Window) *Window {
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
	gen := layoutGen()
	for _, o := range a.Windows() {
		if o == w || o.seenGen == gen {
			continue
		}
		if err := o.catchUp(); err != nil {
			return err
		}
		o.requestFrame()
	}
	setRunning(&w.frameLoop)
	return nil
}

// forgetClosed drops the windows that have closed.
func (a *App) forgetClosed() {
	a.mu.Lock()
	a.windows = slices.DeleteFunc(a.windows, (*Window).Closed)
	a.mu.Unlock()
}

// eventWindow is the window ev is for, or nil for an event that has none.
func eventWindow(ev ggwd.Event) *ggwd.Window {
	switch ev := ev.(type) {
	case ggwd.FrameEvent:
		return ev.Window
	case ggwd.ResizeEvent:
		return ev.Window
	case ggwd.MoveEvent:
		return ev.Window
	case ggwd.FocusEvent:
		return ev.Window
	case ggwd.WindowStateEvent:
		return ev.Window
	case ggwd.CloseEvent:
		return ev.Window
	case ggwd.ClosedEvent:
		return ev.Window
	case ggwd.KeyEvent:
		return ev.Window
	case ggwd.TextEvent:
		return ev.Window
	case ggwd.CompositionEvent:
		return ev.Window
	case ggwd.MouseMoveEvent:
		return ev.Window
	case ggwd.MouseEnterEvent:
		return ev.Window
	case ggwd.MouseButtonEvent:
		return ev.Window
	case ggwd.ScrollEvent:
		return ev.Window
	case ggwd.TouchEvent:
		return ev.Window
	case ggwd.DragEvent:
		return ev.Window
	case ggwd.DropEvent:
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

// runningApp is the App whose Run is under way, for work from a goroutine
// of the process's own, such as the SystemDark poller, which no frame loop
// runs on.
var runningApp atomic.Pointer[App]

// OnQuitRequest registers fn to run when the user asks the app to quit, as
// with ⌘Q or the Dock's Quit, before every window's OnCloseRequest handlers
// are asked: how an app whose window hides on its close button, to live on
// in the tray, tells a quit from that button. fn runs on the UI thread.
// Only macOS reports a quit.
func (a *App) OnQuitRequest(fn func()) { a.quitAsked = append(a.quitAsked, fn) }

// OnReopen registers fn to run when the user opens the running app again,
// as by clicking its Dock icon: the way back to a window it hid, which the
// system does not show on its own. fn runs on the UI thread. Only macOS
// reports it.
func (a *App) OnReopen(fn func()) { a.reopened = append(a.reopened, fn) }

// postAll runs fns on the UI thread, in the order of the work posted before
// them: a quit's handlers run before the close requests that follow it.
func (a *App) postAll(fns []func()) {
	w := a.activeWindow()
	if w == nil {
		return
	}
	for _, fn := range fns {
		w.post(fn)
	}
}

// SetDockIcon puts the app's icon in the Dock and its menu in the menu bar,
// or takes both out, as an app that lives in the tray does while it shows
// no window. Out of the Dock, the app is not in the ⌘-Tab switcher, and
// its windows have no menu bar: put it back before showing one. Call it
// while the app runs, from any goroutine. Only macOS has a Dock.
func SetDockIcon(visible bool) {
	if appRunning.Load() {
		ggwd.SetDockIcon(visible)
	}
}

// SingleInstance makes the app the only one named id running for the
// user, where id is reverse-DNS by convention: "com.example.editor". Call
// it before Run. When another process already runs the app, that process
// is handed this launch's arguments and working directory, and
// SingleInstance returns runtime.ErrRunning: return from main without
// calling Run.
//
// In the process that runs, fn receives every later launch on the UI
// thread, where UseWindow is the window that last had the focus. It is
// where the app brings a window forward and opens the files it was given:
//
//	err := app.SingleInstance("com.example.editor", func(l runtime.Launch) {
//		app.Focus()
//		m.open(l.Args...)
//	})
//	if errors.Is(err, runtime.ErrRunning) {
//		return
//	}
func (a *App) SingleInstance(id string, fn func(runtime.Launch)) error {
	release, err := runtime.ClaimInstance(id, func(l runtime.Launch) {
		if w := a.activeWindow(); w != nil {
			w.post(func() { fn(l) })
		}
	})
	if err != nil {
		return err
	}
	a.releaseInstance = release
	return nil
}

// Appearance is the light or dark look of what the platform draws for the
// app: title bars, and on some desktops scroll bars and menus.
type Appearance uint8

// The appearances.
const (
	// AppearanceSystem follows the desktop's setting.
	AppearanceSystem Appearance = iota
	AppearanceLight
	AppearanceDark
)

// appearance is what SetAppearance asked for, applied when Run starts if
// it was asked for before.
var appearance atomic.Uint32

// SetAppearance sets the look of the chrome the platform draws around the
// app's windows. It does not change the app's own theme: uitheme.Set does
// that, and calls this to match. It may be called from any goroutine, and
// before Run.
func SetAppearance(a Appearance) {
	if Appearance(appearance.Swap(uint32(a))) == a {
		return
	}
	if appRunning.Load() {
		applyAppearance()
	}
}

// applyAppearance hands the appearance to the engine, which needs a
// running app to change a window.
func applyAppearance() {
	mode := ggwd.ColorModeUnknown
	switch Appearance(appearance.Load()) {
	case AppearanceLight:
		mode = ggwd.ColorModeLight
	case AppearanceDark:
		mode = ggwd.ColorModeDark
	}
	ggwd.SetPreferredColorMode(mode)
}

var (
	systemDark      = State(false)
	systemDarkWatch sync.Once
)

// systemDarkPoll is how often SystemDark asks the platform, which reports
// no change as an event.
const systemDarkPoll = time.Second

// SystemDark follows whether the desktop is set to a dark appearance, for
// an app that follows it:
//
//	uitheme.Bind(ggui.SystemDark(), uitheme.Dark(), uitheme.Default())
//
// It is read while an App runs, about once a second, so a change shows
// within a second. Before Run, under a Probe and where the platform cannot
// tell, it is false.
func SystemDark() Readable[bool] {
	systemDarkWatch.Do(func() { go watchSystemDark() })
	return systemDark
}

// watchSystemDark polls the platform for the life of the process and posts
// each change to whichever window is running.
func watchSystemDark() {
	last := false
	for ; ; time.Sleep(systemDarkPoll) {
		if !appRunning.Load() {
			continue
		}
		dark := ggwd.SystemColorMode() == ggwd.ColorModeDark
		if dark == last {
			continue
		}
		if a := runningApp.Load(); a != nil {
			last = dark
			a.post(func() { systemDark.Set(dark) })
		}
	}
}

// CurrentClipboard is the clipboard of the App or Probe whose frame is
// running, for a widget that cuts, copies or pastes: text fields use it.
// Replace an app's with App.SetClipboard; a Probe has one of its own.
func CurrentClipboard() runtime.Clipboard { return currentClipboard() }

// systemClipboard serves a clipboard use with no frame loop running.
var systemClipboard = runtime.NativeClipboard()

func currentClipboard() runtime.Clipboard {
	if l := runningLoop(); l != nil && l.clipboard != nil {
		return l.clipboard
	}
	return systemClipboard
}
