//go:build unix && !ios && !android

package main

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ironpark/ggfx"
	ggfxremote "github.com/ironpark/ggfx/remote"
	"github.com/ironpark/ggwd"
	"github.com/ironpark/ggwd/remote"
	"github.com/ironpark/ggwd/remote/viewer"
)

// handshakeTimeout is how long an app that connected has to say hello
// and describe its window.
const handshakeTimeout = 5 * time.Second

// placeholder are the options of a window opened before any app came, to
// show a failure in.
var placeholder = remote.OpenWindow{Title: "ggviewer", Width: 800, Height: 600, Resizable: true,
	MinWidth: -1, MinHeight: -1, MaxWidth: -1, MaxHeight: -1, Decorated: true}

var background = color.RGBA{0x1f, 0x1f, 0x23, 0xff}

// app is an app process attached to the viewer.
type app struct {
	id   int
	pid  int
	conn *remote.Conn
	opts remote.OpenWindow

	mu      sync.Mutex
	shm     []byte
	oldShm  [][]byte // replaced, to unmap on the main thread
	pending []*remote.Batch

	// shown reports that a frame of the app's was shown. Main thread only.
	shown bool
}

// requests are what the other goroutines ask of the main thread, which
// owns the window.
type requests struct {
	options   *remote.OpenWindow
	title     *string
	cursor    *remote.SetCursor
	textInput *remote.TextInput
	window    []uint8
	actions   []command
	inputs    []remote.Input
	dumps     []string
	close     bool
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

	mu       sync.Mutex
	cur      *app
	apps     []*app // attached, for detachAll
	nextID   int
	gone     []*app
	req      requests
	ov       overlay
	ovSerial int
	cfg      remote.Config
	haveCfg  bool
	geom     geometry
	geomSet  bool

	// The main thread's.
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

// handshake welcomes an app and reads its hello and its window.
func (v *session) handshake(c *net.UnixConn) (*app, error) {
	_ = c.SetDeadline(time.Now().Add(handshakeTimeout))
	conn := remote.NewConn(c)
	if err := conn.Send(&remote.Welcome{Version: remote.Version, Driver: v.info}); err != nil {
		return nil, err
	}
	m, err := conn.Receive()
	if err != nil {
		return nil, err
	}
	h, ok := m.(*remote.Hello)
	if !ok {
		return nil, fmt.Errorf("an app said %T before hello", m)
	}
	if h.Version != remote.Version {
		// The app refuses the viewer too, and says why.
		return nil, fmt.Errorf("app %d speaks protocol version %d, the viewer %d: build the viewer and the app with the same ggwd", h.PID, h.Version, remote.Version)
	}
	if m, err = conn.Receive(); err != nil {
		return nil, err
	}
	o, ok := m.(*remote.OpenWindow)
	if !ok {
		return nil, fmt.Errorf("app %d said %T before its window", h.PID, m)
	}
	_ = c.SetDeadline(time.Time{})
	return &app{pid: h.PID, conn: conn, opts: *o}, nil
}

// attach makes a the app shown, in place of the one before.
func (v *session) attach(a *app) {
	v.mu.Lock()
	v.nextID++
	a.id = v.nextID
	old := v.cur
	v.cur = a
	v.apps = append(v.apps, a)
	// What the app before asked of the window is not this one's.
	v.resetRequests()
	cfg, have := v.cfg, v.haveCfg
	v.mu.Unlock()
	if old != nil {
		_ = old.conn.Close()
	}
	if have {
		_ = a.conn.Send(&cfg)
	}
	v.events.send(event{Type: evAppAttached, PID: a.pid})
	go v.read(a)
}

// resetRequests puts the text input and the cursor back as they are with
// no app. v.mu is held.
func (v *session) resetRequests() {
	v.req.textInput = &remote.TextInput{}
	v.req.cursor = &remote.SetCursor{Shape: uint8(ggwd.CursorShapeDefault), Mode: uint8(ggwd.CursorModeVisible)}
}

// accept attaches the apps that connect after the first, until the
// listener closes.
func (v *session) accept() {
	for c := range v.conns {
		a, err := v.handshake(c)
		if err != nil {
			log.Print(err)
			_ = c.Close()
			continue
		}
		v.attach(a)
		v.mu.Lock()
		o := a.opts
		v.req.options = &o
		v.mu.Unlock()
		v.wake()
	}
}

// detachAll closes every app's connection, which ends the apps.
func (v *session) detachAll() {
	v.mu.Lock()
	apps := v.apps
	v.mu.Unlock()
	for _, a := range apps {
		_ = a.conn.Close()
	}
}

// read reads a's messages until it goes away.
func (v *session) read(a *app) {
	for {
		m, err := a.conn.Receive()
		if err != nil {
			v.mu.Lock()
			if v.cur == a {
				v.cur = nil
				v.resetRequests()
			}
			v.gone = append(v.gone, a)
			v.mu.Unlock()
			v.events.send(event{Type: evAppDetached, PID: a.pid})
			v.kick()
			return
		}
		v.mu.Lock()
		current := v.cur == a
		v.mu.Unlock()
		switch m := m.(type) {
		case *remote.SharedMemory:
			b, err := m.Map()
			if err != nil {
				log.Printf("app %d: mapping its shared memory: %v", a.pid, err)
				_ = a.conn.Close()
				continue
			}
			a.mu.Lock()
			if a.shm != nil {
				a.oldShm = append(a.oldShm, a.shm)
			}
			a.shm = b
			a.mu.Unlock()
		case *remote.Batch:
			a.mu.Lock()
			a.pending = append(a.pending, m)
			a.mu.Unlock()
			v.kick()
			continue
		case *remote.Idle:
			continue
		case *remote.Closed:
			if current {
				v.mu.Lock()
				v.req.close = true
				v.mu.Unlock()
			}
		default:
			if current {
				v.request(m)
			}
		}
		v.wake()
	}
}

// request queues an app's request of the window for the main thread.
func (v *session) request(m remote.Message) {
	v.mu.Lock()
	defer v.mu.Unlock()
	switch m := m.(type) {
	case *remote.OpenWindow:
		v.req.options = m
	case *remote.SetTitle:
		v.req.title = &m.Title
	case *remote.SetCursor:
		v.req.cursor = m
	case *remote.TextInput:
		v.req.textInput = m
	case *remote.WindowRequest:
		v.req.window = append(v.req.window, m.Request)
	}
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
	case ggwd.WindowStateEvent:
		v.state = ev.State
	case ggwd.FocusEvent:
		v.focused = ev.Focused
		if v.opts.debug {
			log.Printf("focused: %t", ev.Focused)
		}
		v.reconfigure()
	case ggwd.SystemColorModeEvent, ggwd.MonitorEvent:
		v.reconfigure()
	case ggwd.CloseEvent:
		// The app decides, and says Closed when it closes; with no app the
		// window closes, and the viewer ends.
		v.mu.Lock()
		a := v.cur
		v.mu.Unlock()
		if a != nil {
			ev.KeepOpen()
			_ = a.conn.Send(&remote.Close{})
		}
	case ggwd.ClosedEvent:
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
	if v.first != nil {
		if a, err = v.handshake(v.first); err != nil {
			log.Print(err)
			_ = v.first.Close()
		}
		v.first = nil
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
	if a != nil {
		v.lastOpts = &a.opts
		v.attach(a)
	}
	go v.accept()
	return nil
}

// pump does what the other goroutines asked of the window.
func (v *session) pump() {
	w := v.win.Load()
	if w == nil {
		return
	}
	v.mu.Lock()
	r := v.req
	v.req = requests{}
	v.mu.Unlock()
	if r.options != nil {
		c := planOptions(v.lastOpts, r.options, v.title, v.restored)
		c.apply(w)
		if c.title != nil {
			v.title = *c.title
		}
		v.lastOpts = r.options
	}
	if r.title != nil && *r.title != v.title {
		w.SetTitle(*r.title)
		v.title = *r.title
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
	}
	for _, a := range r.actions {
		switch a.Action {
		case "resize":
			w.SetSize(a.Width, a.Height)
		case "hide":
			w.Hide()
		case "show":
			w.Show()
		case "minimize":
			w.Minimize()
		case "restore":
			w.Restore()
		case "focus":
			w.Focus()
		}
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
	v.configure(viewer.Config(w, cfg.Width, cfg.Height, cfg.Scale, v.focused))
}

// service frees the apps that went away and replays the shown app's
// batches. It runs inside a frame: a background frame, which comes
// whatever the window is doing, or the window's.
func (v *session) service(windowFrame bool) {
	v.mu.Lock()
	gone := v.gone
	v.gone = nil
	a := v.cur
	v.mu.Unlock()
	for _, g := range gone {
		if r := v.replayers[g]; r != nil {
			images, shaders := r.Live()
			if err := r.Dispose(); err != nil {
				log.Printf("freeing app %d: %v", g.pid, err)
			}
			if v.opts.debug {
				li, ls := r.Live()
				log.Printf("app %d freed: %d images and %d shaders, %d and %d left", g.pid, images, shaders, li, ls)
			}
			delete(v.replayers, g)
		}
		g.mu.Lock()
		for _, b := range append(g.oldShm, g.shm) {
			if b != nil {
				_ = remote.Unmap(b)
			}
		}
		g.shm, g.oldShm, g.pending = nil, nil, nil
		g.mu.Unlock()
	}
	if a != nil && (windowFrame || !v.opts.windowFramesOnly) {
		a.mu.Lock()
		pending, shm, old := a.pending, a.shm, a.oldShm
		a.pending, a.oldShm = nil, nil
		a.mu.Unlock()
		for _, b := range old {
			_ = remote.Unmap(b)
		}
		r := v.replayers[a]
		if r == nil && len(pending) > 0 {
			r = ggfxremote.NewReplayer()
			v.replayers[a] = r
		}
		for _, b := range pending {
			if !v.replay(a, r, shm, b) {
				break
			}
		}
	}
	for _, p := range v.dumps {
		if err := v.dump(p); err != nil {
			log.Printf("dumping the window: %v", err)
		}
		v.events.send(event{Type: evDumped, Path: p})
	}
	v.dumps = nil
}

// replay replays one batch of a's into the screen image and acks it. It
// reports false when the app is dropped for a bad batch.
func (v *session) replay(a *app, r *ggfxremote.Replayer, shm []byte, b *remote.Batch) bool {
	drop := func(err error) bool {
		log.Printf("app %d's batch %d: %v; dropping the app", a.pid, b.Seq, err)
		_ = a.conn.Close()
		return false
	}
	if b.Offset < 0 || b.Length < 0 || b.Offset+b.Length > int64(len(shm)) {
		return drop(errors.New("outside its shared memory"))
	}
	batch := shm[b.Offset : b.Offset+b.Length]
	w, h, err := ggfxremote.ScreenSize(batch)
	if err != nil {
		return drop(err)
	}
	var screen *ggfx.Image
	if w > 0 {
		if v.screen == nil || v.screen.Bounds().Size() != image.Pt(w, h) {
			if v.screen != nil {
				v.screen.Deallocate()
			}
			v.screen = ggfx.NewImageWithOptions(image.Rect(0, 0, w, h), &ggfx.NewImageOptions{Unmanaged: true})
		}
		screen = v.screen
	}
	if b.Read != nil {
		dsts := make([][]byte, len(b.Read.Regions))
		for k, rect := range b.Read.Regions {
			dsts[k] = make([]byte, 4*max(rect.Dx(), 0)*max(rect.Dy(), 0))
		}
		reply := &remote.Pixels{Seq: b.Seq, Pixels: dsts}
		if err := r.ReadPixels(batch, screen, b.Read.Image, b.Read.Regions, dsts); err != nil {
			reply.Err = err.Error()
		}
		_ = a.conn.Send(reply)
		return true
	}
	res, err := r.Replay(batch, screen)
	if err != nil {
		return drop(err)
	}
	_ = a.conn.Send(&remote.Ack{Seq: b.Seq})
	if res.Presented && !a.shown {
		// The app's first frame takes the place of what the overlay said
		// while it was coming.
		a.shown = true
		v.mu.Lock()
		v.ov = overlay{}
		v.ovSerial++
		v.mu.Unlock()
		v.events.send(event{Type: evFirstFrame, PID: a.pid})
	}
	return true
}

// compose draws the app's last frame and the overlay to dst.
func (v *session) compose(dst *ggfx.Image) {
	db := dst.Bounds()
	if v.screen == nil {
		dst.Fill(background)
	} else if sb := v.screen.Bounds(); sb.Size() == db.Size() {
		dst.DrawImage(v.screen, &ggfx.DrawImageOptions{Blend: ggfx.BlendCopy})
	} else {
		// The app has not caught up with a resize: letterbox its last frame.
		dst.Fill(background)
		s := min(float64(db.Dx())/float64(sb.Dx()), float64(db.Dy())/float64(sb.Dy()))
		op := &ggfx.DrawImageOptions{Filter: ggfx.FilterLinear}
		op.GeoM.Scale(s, s)
		op.GeoM.Translate((float64(db.Dx())-s*float64(sb.Dx()))/2, (float64(db.Dy())-s*float64(sb.Dy()))/2)
		dst.DrawImage(v.screen, op)
	}
	v.updateOverlay(db.Dx(), db.Dy())
	if v.ovImage != nil {
		op := &ggfx.DrawImageOptions{}
		op.GeoM.Translate(float64(v.ovLayout.rect.Min.X), float64(v.ovLayout.rect.Min.Y))
		dst.DrawImage(v.ovImage, op)
	}
}

// updateOverlay draws the overlay again if it or the window's size
// changed since it was drawn.
func (v *session) updateOverlay(w, h int) {
	v.mu.Lock()
	ov, serial := v.ov, v.ovSerial
	v.mu.Unlock()
	key := [4]float64{float64(serial), float64(w), float64(h), v.scale}
	if key == v.ovKey {
		return
	}
	v.ovKey = key
	img, layout := ov.render(w, h, v.scale)
	v.mu.Lock()
	if v.ovSerial == serial {
		v.ov.scroll = ov.scroll
	}
	v.mu.Unlock()
	v.ovLayout = layout
	if img == nil {
		if v.ovImage != nil {
			v.ovImage.Deallocate()
			v.ovImage = nil
		}
		return
	}
	if v.ovImage == nil || v.ovImage.Bounds().Size() != img.Rect.Size() {
		if v.ovImage != nil {
			v.ovImage.Deallocate()
		}
		v.ovImage = ggfx.NewImage(img.Rect.Dx(), img.Rect.Dy())
	}
	v.ovImage.WritePixels(img.Pix)
}

// scrollOverlay scrolls the overlay's output when the pointer is over it,
// and reports whether it did.
func (v *session) scrollOverlay(ev ggwd.ScrollEvent) bool {
	l := v.ovLayout
	p := image.Pt(int(v.mouse[0]*v.scale), int(v.mouse[1]*v.scale))
	if !l.scrollable() || !p.In(l.rect) || l.lineHeight == 0 {
		return false
	}
	v.scrolled -= ev.Y * ev.Line * v.scale / float64(l.lineHeight)
	n := int(v.scrolled)
	if n != 0 {
		v.scrolled -= float64(n)
		v.mu.Lock()
		v.ov.scroll = max(v.ov.scroll+n, 0)
		v.ovSerial++
		v.mu.Unlock()
		if w := v.win.Load(); w != nil {
			w.RequestFrame()
		}
	}
	return true
}

// dump writes what the window shows to a PNG at path.
func (v *session) dump(path string) error {
	size := v.pixels
	if size.X <= 0 || size.Y <= 0 {
		if v.screen == nil {
			return errors.New("the window has no size yet")
		}
		size = v.screen.Bounds().Size()
	}
	img := ggfx.NewImage(size.X, size.Y)
	defer img.Deallocate()
	v.compose(img)
	rgba := image.NewRGBA(image.Rectangle{Max: size})
	img.ReadPixels(rgba.Pix)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, rgba); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// remember notes the window's geometry, and keeps it a moment later, once
// the window stopped moving.
func (v *session) remember(f func(*geometry)) {
	if v.statePath == "" {
		return
	}
	v.mu.Lock()
	f(&v.geom)
	first := !v.geomSet
	v.geomSet = true
	v.mu.Unlock()
	if first {
		go func() {
			for {
				select {
				case <-v.quitting:
					return
				case <-time.After(time.Second):
					v.saveState()
				}
			}
		}()
	}
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
