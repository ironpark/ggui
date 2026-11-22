//go:build (unix && !ios && !android) || windows

package main

import (
	"image"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ironpark/ggfx"
	ggfxremote "github.com/ironpark/ggfx/remote"
	"github.com/ironpark/ggui/internal/platform/nativemenu"
	"github.com/ironpark/ggwd"
	"github.com/ironpark/ggwd/remote"
	"github.com/ironpark/ggwd/remote/viewer"
)

// placeholder are the options of a window opened before any app came, to
// show a failure in.
var placeholder = remote.OpenWindow{Title: "ggviewer", Width: 800, Height: 600, Resizable: true,
	MinWidth: -1, MinHeight: -1, MaxWidth: -1, MaxHeight: -1, Decorated: true}

type trayOp struct {
	set    *remote.SetTray
	remove int
}

// requests are what the other goroutines ask of the main thread, which
// owns the window.
type requests struct {
	reset       bool
	options     *remote.OpenWindow
	title       *string
	size        *remote.SetSize
	position    *remote.SetPosition
	icon        *remote.SetIcon
	colorMode   *remote.SetColorMode
	dockIcon    *remote.SetDockIcon
	menu        *remote.SetMenu
	menuUpdates []*remote.UpdateMenuItem
	trays       []trayOp
	cursor      *remote.SetCursor
	textInput   *remote.TextInput
	window      []uint8
	actions     []command
	inputs      []remote.Input
	dumps       []string
	close       bool
}

// session is the viewer: its window, the app it shows, and what the
// launcher told it.
type session struct {
	opts      options
	events    *events
	statePath string
	saved     geometry
	haveSaved bool

	// first is the first app's connection, which Run starts with; conns are
	// the ones after it.
	first *net.UnixConn
	conns chan *net.UnixConn
	info  []byte

	needWindow chan struct{}
	needOnce   sync.Once
	quitting   chan struct{}
	quitOnce   sync.Once
	running    atomic.Bool
	win        atomic.Pointer[ggwd.Window]

	mu  sync.Mutex
	cur *app
	// closeAsked is set once the window's close was handed to the app, so
	// that the app's Closed closes the window too; an app that closes on
	// its own, as App.Quit does, leaves the window to the next app.
	closeAsked bool
	apps       map[*app]struct{} // attached, for detachAll
	nextID     int
	gone       []*app
	req        requests
	ov         overlay
	ovSerial   int
	cfg        remote.Config
	haveCfg    bool
	geom       geometry
	geomSet    bool
	saveTimer  *time.Timer
	saveDelay  time.Duration // zero uses saveDebounce

	// The main thread's.
	menu      menuBar
	trayKeys  map[int]struct{}
	appHid    bool
	lastOpts  *remote.OpenWindow
	restored  bool
	title     string
	focused   bool
	state     ggwd.WindowState
	scale     float64
	pixels    image.Point
	screen    *ggfx.Image
	replayers map[*app]*ggfxremote.Replayer
	ovImage   *ggfx.Image
	ovLayout  overlayLayout
	ovKey     [4]float64
	dumps     []string
	mouse     [2]float64
	scrolled  float64
	seq       uint64
	// scripted reports that debug-input commands came, after which the
	// window's own input is not forwarded.
	scripted bool
}

func newSession(o options, ev *events) *session {
	return &session{
		opts:       o,
		events:     ev,
		needWindow: make(chan struct{}),
		quitting:   make(chan struct{}),
		apps:       map[*app]struct{}{},
		menu:       nativeMenuBar{},
		trayKeys:   map[int]struct{}{},
		replayers:  map[*app]*ggfxremote.Replayer{},
		scale:      1,
		ovKey:      [4]float64{-1},
	}
}

// quit ends the viewer: it closes the window, or, before there is one,
// ends at once.
func (v *session) quit() {
	v.quitOnce.Do(func() { close(v.quitting) })
	v.mu.Lock()
	v.req.close = true
	if v.saveTimer != nil {
		v.saveTimer.Stop()
	}
	v.mu.Unlock()
	v.wake()
}

func (v *session) wake() {
	if v.running.Load() {
		ggwd.Wake()
	}
}

// kick asks for the frames that replay an app's batches and show them.
func (v *session) kick() {
	if w := v.win.Load(); w != nil {
		w.RequestFrame()
	}
	if v.running.Load() && !v.opts.windowFramesOnly {
		ggwd.RequestBackgroundFrame()
	}
}

// command does a command of the launcher's.
func (v *session) command(c command) {
	v.mu.Lock()
	defer v.mu.Unlock()
	setOverlay := func(o overlay) {
		v.ov = o
		v.ovSerial++
		if w := v.win.Load(); w != nil {
			w.RequestFrame()
		}
	}
	switch c.Type {
	case cmdBuilding:
		setOverlay(overlay{kind: overlayBuilding})
	case cmdBuildFailed:
		setOverlay(overlay{kind: overlayBuildFailed, output: c.Output})
		v.needOnce.Do(func() { close(v.needWindow) })
	case cmdAppExited:
		setOverlay(overlay{kind: overlayAppExited, status: c.Status, output: c.Output})
		v.needOnce.Do(func() { close(v.needWindow) })
	case cmdClear:
		setOverlay(overlay{})
	case cmdQuit:
		go v.quit()
	case cmdDebugDump:
		v.req.dumps = append(v.req.dumps, c.Path)
	case cmdDebugWindow:
		v.req.actions = append(v.req.actions, c)
	case cmdDebugInput:
		v.req.inputs = append(v.req.inputs, *c.Input)
	}
	v.wake()
}

// menuSelected forwards a native menu bar or tray action to the attached app.
func (v *session) menuSelected(id int) {
	v.mu.Lock()
	a := v.cur
	v.mu.Unlock()
	if a == nil {
		return
	}
	allowForeground(a.pid)
	_ = a.conn.Send(&remote.MenuSelected{ID: id})
}

// clearTrays removes all system tray icons installed for the previous app.
// Main thread only.
func (v *session) clearTrays() {
	for k := range v.trayKeys {
		v.menu.RemoveTray(k)
	}
	clear(v.trayKeys)
}

// applyTrays executes queued tray additions/updates/removals on the main thread.
func (v *session) applyTrays(ops []trayOp) {
	for _, op := range ops {
		if op.set != nil {
			v.menu.SetTray(op.set.Key, remoteToNativeTray(op.set))
			v.trayKeys[op.set.Key] = struct{}{}
		} else {
			v.menu.RemoveTray(op.remove)
			delete(v.trayKeys, op.remove)
		}
	}
}

// sendAppEvent forwards a process-level macOS event (quit / reopen) to the
// attached app.
func (v *session) sendAppEvent(ae remote.AppEvent) {
	v.mu.Lock()
	a := v.cur
	v.mu.Unlock()
	if a == nil {
		return
	}
	_ = a.conn.Send(&ae)
}

// askClose hands the window's close to the app, and reports whether
// there is an app to ask.
func (v *session) askClose() bool {
	v.mu.Lock()
	a := v.cur
	if a != nil {
		v.closeAsked = true
	}
	v.mu.Unlock()
	if a == nil {
		return false
	}
	_ = a.conn.Send(&remote.Close{})
	return true
}

// HandleEvent handles the viewer's window's events: it shows the app's
// frames and forwards the window's input to the app.
func (v *session) HandleEvent(ev ggwd.Event) error {
	switch ev := ev.(type) {
	case ggwd.StartEvent:
		return v.start()
	case ggwd.WakeEvent:
		v.pump()
	case ggwd.BackgroundFrameEvent:
		v.pump()
		v.service(false)
	case ggwd.FrameEvent:
		if ev.Scale > 0 {
			v.scale = ev.Scale
		}
		v.pump()
		v.service(true)
		v.compose(ev.Screen)
	case ggwd.ResizeEvent:
		v.scale = ev.Scale
		cfg := viewer.Config(ev.Window, ev.Width, ev.Height, ev.Scale, v.focused)
		v.pixels = image.Pt(cfg.PixelWidth, cfg.PixelHeight)
		v.configure(cfg)
		if v.state == ggwd.WindowStateNormal {
			v.remember(func(g *geometry) { g.Width, g.Height = ev.Width, ev.Height })
		}
	case ggwd.MoveEvent:
		if v.state == ggwd.WindowStateNormal {
			v.remember(func(g *geometry) { g.X, g.Y = ev.X, ev.Y })
		}
		v.reconfigure()
	case ggwd.WindowStateEvent:
		v.state = ev.State
		v.reconfigure()
	case ggwd.FocusEvent:
		v.focused = ev.Focused
		if v.opts.debug {
			log.Printf("focused: %t", ev.Focused)
		}
		v.reconfigure()
	case ggwd.SystemColorModeEvent, ggwd.MonitorEvent:
		v.reconfigure()
	case ggwd.QuitEvent, ggwd.ReopenEvent:
		if ae, ok := viewer.AppEvent(ev); ok {
			v.sendAppEvent(ae)
		}
	case ggwd.CloseEvent:
		// The app decides, and says Closed when it closes; with no app the
		// window closes, and the viewer ends.
		if v.askClose() {
			ev.KeepOpen()
		}
	case ggwd.ClosedEvent:
		v.menu.Set(nil)
		v.clearTrays()
		v.saveState()
		v.events.send(event{Type: evWindowClosed})
	case ggwd.MouseMoveEvent:
		v.mouse = [2]float64{ev.X, ev.Y}
		v.forwardEvent(ev)
	case ggwd.ScrollEvent:
		if !v.scrollOverlay(ev) {
			v.forwardEvent(ev)
		}
	default:
		v.forwardEvent(ev)
	}
	return nil
}

// start opens the window, with the first app's options if one came.
func (v *session) start() error {
	first := v.first
	v.first = nil
	attached := false
	defer func() {
		if !attached && first != nil {
			_ = first.Close()
		}
	}()
	info, err := ggfxremote.LocalInfo()
	if err != nil {
		return err
	}
	if v.info, err = info.MarshalBinary(); err != nil {
		return err
	}
	v.running.Store(true)
	select {
	case <-v.quitting:
		return ggwd.Termination
	default:
	}
	var a *app
	if first != nil {
		if a, err = v.handshake(first); err != nil {
			log.Print(err)
			_ = first.Close()
			first = nil
		}
	}
	o := placeholder
	if a != nil {
		o = a.opts
	}
	wo := viewer.WindowOptions(&o)
	if v.haveSaved {
		wo.Width, wo.Height = v.saved.Width, v.saved.Height
		wo.Position = &ggwd.Point{X: v.saved.X, Y: v.saved.Y}
		v.restored = true
	}
	w, err := ggwd.NewWindow(wo)
	if err != nil {
		return err
	}
	v.win.Store(w)
	v.title = o.Title
	nativemenu.OnSelect(v.menuSelected)
	if a != nil {
		if a.opts.MousePassthrough {
			w.SetMousePassthrough(true)
		}
		last := a.opts
		v.lastOpts = &last
		v.appHid = a.opts.Hidden
		v.attach(a, false)
		attached = true
	}
	go v.accept()
	return nil
}

// setAppHidden records whether the app currently hides the viewer window and
// keeps v.lastOpts.Hidden in sync so subsequent planOptions calls see the
// window's real visibility. Main thread only.
func (v *session) setAppHidden(hidden bool) {
	v.appHid = hidden
	if v.lastOpts != nil {
		v.lastOpts.Hidden = hidden
	}
}

// pump does what the other goroutines asked of the window.
func (v *session) pump() {
	w := v.win.Load()
	if w == nil {
		return
	}
	v.mu.Lock()
	r := v.req
	ovKind := v.ov.kind
	v.req = requests{}
	v.mu.Unlock()
	reconfig := false
	if r.reset {
		viewer.Reset(w)
		if v.lastOpts != nil {
			v.lastOpts.MousePassthrough = false
		}
		if r.menu == nil {
			v.menu.Set(nil)
		}
		v.clearTrays()
		if v.appHid && (r.options == nil || !r.options.Hidden) {
			w.Show()
			v.setAppHidden(false)
			reconfig = true
		}
	}
	if r.options != nil {
		c := planOptions(v.lastOpts, r.options, v.title, v.restored)
		c.apply(w)
		if c.title != nil {
			v.title = *c.title
		}
		if c.hidden != nil {
			v.appHid = *c.hidden
			reconfig = true
		}
		last := *r.options
		v.lastOpts = &last
	}
	if r.title != nil && *r.title != v.title {
		w.SetTitle(*r.title)
		v.title = *r.title
	}
	if r.size != nil {
		viewer.SetSize(w, r.size)
	}
	if r.position != nil {
		viewer.SetPosition(w, r.position)
	}
	if r.icon != nil {
		viewer.SetIcon(w, r.icon)
	}
	if r.colorMode != nil {
		viewer.SetColorMode(r.colorMode)
	}
	if r.dockIcon != nil {
		viewer.SetDockIcon(r.dockIcon)
	}
	if r.menu != nil {
		v.menu.Set(remoteToNativeMenu(r.menu.Items))
	}
	v.applyTrays(r.trays)
	for _, u := range r.menuUpdates {
		if u.SetEnabled {
			v.menu.SetEnabled(u.ID, u.Enabled)
		}
		if u.SetChecked {
			v.menu.SetChecked(u.ID, u.Checked)
		}
	}
	if r.cursor != nil {
		viewer.SetCursor(w, r.cursor)
	}
	if r.textInput != nil {
		if v.opts.debug {
			log.Printf("text input: %+v", *r.textInput)
		}
		viewer.SetTextInput(w, r.textInput)
	}
	for _, q := range r.window {
		viewer.Do(w, &remote.WindowRequest{Request: q})
		switch q {
		case remote.RequestHide:
			v.setAppHidden(true)
			reconfig = true
		case remote.RequestShow:
			v.setAppHidden(false)
			reconfig = true
		}
	}
	if v.appHid && (ovKind == overlayBuildFailed || ovKind == overlayAppExited) {
		w.Show()
		v.setAppHidden(false)
		reconfig = true
	}
	for _, a := range r.actions {
		switch a.Action {
		case "resize":
			w.SetSize(a.Width, a.Height)
		case "hide":
			w.Hide()
			reconfig = true
		case "show":
			w.Show()
			reconfig = true
		case "minimize":
			w.Minimize()
		case "restore":
			w.Restore()
		case "focus":
			w.Focus()
		case "close":
			// As the window's close button: the app is asked.
			if !v.askClose() {
				w.Close()
			}
		}
	}
	if reconfig {
		v.reconfigure()
	}
	for _, in := range r.inputs {
		v.scripted = true
		v.forward(in)
	}
	if len(r.dumps) > 0 {
		v.dumps = append(v.dumps, r.dumps...)
		ggwd.RequestBackgroundFrame()
	}
	if r.close {
		w.Close()
	}
}

func (v *session) forwardEvent(ev ggwd.Event) {
	if v.scripted {
		// The debug inputs have the app to themselves.
		return
	}
	if in, ok := viewer.Input(ev); ok {
		v.forward(in)
	}
}

// forward sends input to the app, numbered.
func (v *session) forward(in remote.Input) {
	v.mu.Lock()
	a := v.cur
	v.mu.Unlock()
	if a == nil {
		return
	}
	v.seq++
	in.Seq = v.seq
	if (in.Kind == remote.InputMouseButton || in.Kind == remote.InputKey) && in.Pressed {
		// What the user does in the window may open a dialog of the app's.
		allowForeground(a.pid)
	}
	_ = a.conn.Send(&in)
}

// configure keeps the window's Config and tells the app.
func (v *session) configure(cfg remote.Config) {
	v.mu.Lock()
	v.cfg, v.haveCfg = cfg, true
	a := v.cur
	v.mu.Unlock()
	if a != nil {
		_ = a.conn.Send(&cfg)
	}
}

// reconfigure tells the app the window's Config after something other
// than its size changed.
func (v *session) reconfigure() {
	w := v.win.Load()
	v.mu.Lock()
	cfg, have := v.cfg, v.haveCfg
	v.mu.Unlock()
	if w == nil || !have {
		return
	}
	scale := cfg.Scale
	if s := w.DeviceScaleFactor(); s > 0 {
		scale = s
		v.scale = s
	}
	next := viewer.Config(w, cfg.Width, cfg.Height, scale, v.focused)
	v.pixels = image.Pt(next.PixelWidth, next.PixelHeight)
	v.configure(next)
}

const saveDebounce = time.Second

// remember notes the window's geometry, and keeps it a moment later, once
// the window stops moving.
func (v *session) remember(f func(*geometry)) {
	if v.statePath == "" {
		return
	}
	v.mu.Lock()
	f(&v.geom)
	v.geomSet = true
	delay := v.saveDelay
	if delay <= 0 {
		delay = saveDebounce
	}
	if v.saveTimer != nil {
		v.saveTimer.Reset(delay)
	} else {
		v.saveTimer = time.AfterFunc(delay, func() {
			select {
			case <-v.quitting:
				return
			default:
			}
			v.saveState()
		})
	}
	v.mu.Unlock()
}

var saveMu sync.Mutex

// saveState keeps the window's geometry for the project, if it changed.
func (v *session) saveState() {
	v.mu.Lock()
	g, set := v.geom, v.geomSet
	v.mu.Unlock()
	if !set || !g.valid() {
		return
	}
	saveMu.Lock()
	defer saveMu.Unlock()
	if v.haveSaved && v.saved == g {
		return
	}
	if err := saveState(v.statePath, v.opts.project, g); err != nil {
		log.Printf("remembering the window: %v", err)
		return
	}
	v.saved, v.haveSaved = g, true
}
