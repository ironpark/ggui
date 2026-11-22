//go:build (unix && !ios && !android) || windows

package main

import (
	"bytes"
	"encoding/json"
	"image"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ironpark/ggui/internal/platform/nativemenu"
	"github.com/ironpark/ggwd"
	"github.com/ironpark/ggwd/remote"
)

func TestParseCommand(t *testing.T) {
	good := []struct {
		line  string
		debug bool
		want  command
	}{
		{`{"type":"building"}`, false, command{Type: cmdBuilding}},
		{`{"type":"build-failed","output":"./a.go:1:1: x\n"}`, false, command{Type: cmdBuildFailed, Output: "./a.go:1:1: x\n"}},
		{`{"type":"app-exited","status":"exit status 2","output":"panic: x"}`, false, command{Type: cmdAppExited, Status: "exit status 2", Output: "panic: x"}},
		{`{"type":"clear","extra":1}`, false, command{Type: cmdClear}},
		{`{"type":"quit"}`, false, command{Type: cmdQuit}},
		{`{"type":"debug-dump","path":"/tmp/a.png"}`, true, command{Type: cmdDebugDump, Path: "/tmp/a.png"}},
		{`{"type":"debug-window","action":"resize","width":300,"height":200}`, true, command{Type: cmdDebugWindow, Action: "resize", Width: 300, Height: 200}},
		{`{"type":"debug-input","input":{"kind":7,"text":"한","start":0,"end":3}}`, true,
			command{Type: cmdDebugInput, Input: &remote.Input{Kind: remote.InputComposition, Text: "한", End: 3}}},
	}
	for _, c := range good {
		got, err := parseCommand([]byte(c.line), c.debug)
		if err != nil || !reflect.DeepEqual(got, c.want) {
			t.Errorf("parseCommand(%s) = %+v, %v; want %+v", c.line, got, err, c.want)
		}
	}
	bad := []struct {
		line  string
		debug bool
	}{
		{`building`, true},
		{`{}`, true},
		{`{"type":"rebuild"}`, true},
		{`{"type":"debug-dump","path":"/tmp/a.png"}`, false},
		{`{"type":"debug-dump"}`, true},
		{`{"type":"debug-window","action":"explode"}`, true},
		{`{"type":"debug-window","action":"resize"}`, true},
		{`{"type":"debug-input"}`, true},
	}
	for _, c := range bad {
		if got, err := parseCommand([]byte(c.line), c.debug); err == nil {
			t.Errorf("parseCommand(%s) = %+v, want an error", c.line, got)
		}
	}
}

func TestEvents(t *testing.T) {
	var b bytes.Buffer
	e := &events{w: &b}
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Go(func() { e.send(event{Type: evAppAttached, PID: i + 1}) })
	}
	wg.Wait()
	e.send(event{Type: evReady, Socket: "/tmp/s"})
	lines := strings.Split(strings.TrimSuffix(b.String(), "\n"), "\n")
	if len(lines) != 51 {
		t.Fatalf("%d lines", len(lines))
	}
	for _, l := range lines {
		var ev map[string]any
		if err := json.Unmarshal([]byte(l), &ev); err != nil {
			t.Errorf("%q: %v", l, err)
		}
	}
	if want := `{"type":"ready","socket":"/tmp/s"}`; lines[50] != want {
		t.Errorf("got %s, want %s", lines[50], want)
	}
}

func TestState(t *testing.T) {
	dir := t.TempDir()
	p := statePath(dir, "/home/me/my app")
	if !strings.HasPrefix(p, filepath.Join(dir, "ggui", "ggviewer", "my_app-")) || !strings.HasSuffix(p, ".json") {
		t.Errorf("statePath: %s", p)
	}
	if q := statePath(dir, "/elsewhere/my app"); q == p {
		t.Error("two projects of one base name share a state file")
	}
	if _, ok := loadState(p, "/home/me/my app"); ok {
		t.Error("a state loaded from no file")
	}
	g := geometry{X: 10, Y: -20.5, Width: 640, Height: 480}
	if err := saveState(p, "/home/me/my app", g); err != nil {
		t.Fatal(err)
	}
	if got, ok := loadState(p, "/home/me/my app"); !ok || got != g {
		t.Errorf("loaded %+v, %t; want %+v", got, ok, g)
	}
	// A file of another project, one that is broken, or a size of nothing,
	// is not used.
	if _, ok := loadState(p, "/other"); ok {
		t.Error("a state loaded for another project")
	}
	for _, s := range []string{`{`, `{"project":"/home/me/my app","window":{"Width":0,"Height":10}}`} {
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, ok := loadState(p, "/home/me/my app"); ok {
			t.Errorf("a state loaded from %s", s)
		}
	}
	entries, _ := os.ReadDir(filepath.Dir(p))
	if len(entries) != 1 {
		t.Errorf("the state's directory has %d files, want 1", len(entries))
	}
}

type fakeWindow struct{ calls []string }

func (w *fakeWindow) SetTitle(s string)        { w.calls = append(w.calls, "title "+s) }
func (w *fakeWindow) SetSize(x, y float64)     { w.calls = append(w.calls, sprint("size", x, y)) }
func (w *fakeWindow) SetPosition(x, y float64) { w.calls = append(w.calls, sprint("position", x, y)) }
func (w *fakeWindow) SetSizeLimits(a, b, c, d float64) {
	w.calls = append(w.calls, sprint("limits", a, b, c, d))
}
func (w *fakeWindow) SetResizable(v bool) { w.calls = append(w.calls, sprint("resizable", v)) }
func (w *fakeWindow) SetDecorated(v bool) { w.calls = append(w.calls, sprint("decorated", v)) }
func (w *fakeWindow) SetTitlebarHidden(v bool) {
	w.calls = append(w.calls, sprint("titlebar-hidden", v))
}
func (w *fakeWindow) SetTrafficLights(p *ggwd.Point) {
	w.calls = append(w.calls, sprint("traffic-lights", p))
}
func (w *fakeWindow) SetFloating(v bool)         { w.calls = append(w.calls, sprint("floating", v)) }
func (w *fakeWindow) SetMousePassthrough(v bool) { w.calls = append(w.calls, sprint("passthrough", v)) }
func (w *fakeWindow) Maximize()                  { w.calls = append(w.calls, "maximize") }
func (w *fakeWindow) Restore()                   { w.calls = append(w.calls, "restore") }
func (w *fakeWindow) Show()                      { w.calls = append(w.calls, "show") }
func (w *fakeWindow) Hide()                      { w.calls = append(w.calls, "hide") }
func sprint(name string, args ...any) string {
	b, _ := json.Marshal(args)
	return name + " " + string(b)
}
func (w *fakeWindow) reset() []string { c := w.calls; w.calls = nil; return c }
func appOptions(f func(*remote.OpenWindow)) remote.OpenWindow {
	o := placeholder
	o.Title = "app"
	f(&o)
	return o
}

func TestPlanOptions(t *testing.T) {
	first := appOptions(func(o *remote.OpenWindow) { o.Width, o.Height = 400, 300 })
	hiddenBar := appOptions(func(o *remote.OpenWindow) {
		o.Width, o.Height = 400, 300
		o.TitlebarHidden = true
		o.TrafficLightsSet, o.TrafficLightsX, o.TrafficLightsY = true, 16, 14
	})
	undecoratedHidden := appOptions(func(o *remote.OpenWindow) {
		o.Width, o.Height = 400, 300
		o.Decorated = false
		o.TitlebarHidden = true
	})
	for _, c := range []struct {
		name     string
		prev     *remote.OpenWindow
		next     remote.OpenWindow
		title    string
		restored bool
		want     []string
	}{
		{"the same app again", &first, first, "app", false, nil},
		{"the same app again, whose old one set its title", &first, first, "count 3", false, []string{"title app"}},
		{"a new size", &first, appOptions(func(o *remote.OpenWindow) { o.Width = 500 }), "app", false, []string{`size [500,600]`}},
		{"new limits", &first, appOptions(func(o *remote.OpenWindow) { o.Width, o.Height, o.MinWidth, o.MaxHeight = 400, 300, 200, 900 }), "app", false,
			[]string{`limits [200,0,0,900]`}},
		{"flags", &first, appOptions(func(o *remote.OpenWindow) {
			o.Width, o.Height, o.Resizable, o.Decorated, o.Floating, o.TitlebarHidden, o.Transparent = 400, 300, false, false, true, true, true
		}), "app", false, []string{"resizable [false]", "decorated [false]", "titlebar-hidden [true]", "traffic-lights [null]", "floating [true]"}},
		{"the first app in a window opened for a failure", nil, first, "ggviewer", false,
			[]string{"title app", "limits [0,0,0,0]", "resizable [true]", "decorated [true]", "floating [false]", "size [400,300]"}},
		{"the first app in a window of a remembered size", nil, first, "ggviewer", true,
			[]string{"title app", "limits [0,0,0,0]", "resizable [true]", "decorated [true]", "floating [false]"}},
		{"first app with hidden titlebar and traffic lights in failure window", nil, hiddenBar, "ggviewer", false,
			[]string{"title app", "limits [0,0,0,0]", "resizable [true]", "decorated [true]", "titlebar-hidden [true]", `traffic-lights [{"X":16,"Y":14}]`, "floating [false]", "size [400,300]"}},
		{"rebuilt app clears hidden titlebar and traffic lights", &hiddenBar, first, "app", false,
			[]string{"titlebar-hidden [false]", "traffic-lights [null]"}},
		{"rebuilt app moves traffic lights", &hiddenBar, appOptions(func(o *remote.OpenWindow) {
			o.Width, o.Height = 400, 300
			o.TitlebarHidden = true
			o.TrafficLightsSet, o.TrafficLightsX, o.TrafficLightsY = true, 24, 18
		}), "app", false, []string{"titlebar-hidden [true]", `traffic-lights [{"X":24,"Y":18}]`}},
		{"re-enabling decoration re-applies hidden titlebar", &undecoratedHidden, appOptions(func(o *remote.OpenWindow) {
			o.Width, o.Height = 400, 300
			o.Decorated = true
			o.TitlebarHidden = true
		}), "app", false, []string{"decorated [true]", "titlebar-hidden [true]", "traffic-lights [null]"}},
		{"position maximized hidden passthrough", &first, appOptions(func(o *remote.OpenWindow) {
			o.Width, o.Height = 400, 300
			o.PositionSet, o.X, o.Y = true, 120, 80
			o.MousePassthrough = true
			o.Maximized = true
			o.Hidden = true
		}), "app", false, []string{"passthrough [true]", "position [120,80]", "maximize", "hide"}},
		{"restore and show when flags clear", &remote.OpenWindow{
			Title: "app", Width: 400, Height: 300, Resizable: true, Decorated: true,
			MinWidth: -1, MinHeight: -1, MaxWidth: -1, MaxHeight: -1,
			Maximized: true, Hidden: true, MousePassthrough: true,
		}, first, "app", false, []string{"passthrough [false]", "restore", "show"}},
	} {
		var w fakeWindow
		planOptions(c.prev, &c.next, c.title, c.restored).apply(&w)
		if got := w.reset(); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

type fakeMenuBar struct {
	mu           sync.Mutex
	sets         [][]nativemenu.Item
	enabled      map[int]bool
	checked      map[int]bool
	trays        map[int]nativemenu.Tray
	removedTrays []int
}

func newFakeMenuBar() *fakeMenuBar {
	return &fakeMenuBar{
		enabled: map[int]bool{},
		checked: map[int]bool{},
		trays:   map[int]nativemenu.Tray{},
	}
}

func (m *fakeMenuBar) Set(menus []nativemenu.Item) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sets = append(m.sets, menus)
}

func (m *fakeMenuBar) SetEnabled(id int, on bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.enabled[id] = on
}

func (m *fakeMenuBar) SetChecked(id int, on bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.checked[id] = on
}

func (m *fakeMenuBar) SetTray(key int, t nativemenu.Tray) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.trays[key] = t
}

func (m *fakeMenuBar) RemoveTray(key int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.trays, key)
	m.removedTrays = append(m.removedTrays, key)
}

func TestRemoteToNativeMenu(t *testing.T) {
	if got := remoteToNativeMenu(nil); got != nil {
		t.Fatalf("remoteToNativeMenu(nil) = %v, want nil", got)
	}
	in := []remote.MenuItem{
		{
			Title: "File",
			Items: []remote.MenuItem{
				{Title: "Save", ID: 1, Key: "s", Cmd: true, Enabled: true},
				{Separator: true},
				{
					Title: "Recent",
					Items: []remote.MenuItem{
						{Title: "a.go", ID: 2, Enabled: false, Checked: true, Shift: true, Alt: true, Ctrl: true},
					},
				},
			},
		},
	}
	want := []nativemenu.Item{
		{
			Title: "File",
			Items: []nativemenu.Item{
				{Title: "Save", ID: 1, Key: "s", Cmd: true, Enabled: true},
				{Separator: true},
				{
					Title: "Recent",
					Items: []nativemenu.Item{
						{Title: "a.go", ID: 2, Enabled: false, Checked: true, Shift: true, Alt: true, Ctrl: true},
					},
				},
			},
		},
	}
	if got := remoteToNativeMenu(in); !reflect.DeepEqual(got, want) {
		t.Fatalf("remoteToNativeMenu =\n%+v\nwant\n%+v", got, want)
	}
}

func unixPair(t *testing.T) (*net.UnixConn, *net.UnixConn) {
	t.Helper()
	dir, err := os.MkdirTemp("", "ggvt")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	addr := &net.UnixAddr{Name: filepath.Join(dir, "s"), Net: "unix"}
	ln, err := net.ListenUnix("unix", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	accepted := make(chan *net.UnixConn, 1)
	go func() {
		c, err := ln.AcceptUnix()
		if err == nil {
			accepted <- c
		}
	}()
	client, err := net.DialUnix("unix", nil, addr)
	if err != nil {
		t.Fatal(err)
	}
	server := <-accepted
	t.Cleanup(func() {
		_ = server.Close()
		_ = client.Close()
	})
	return server, client
}

func TestSessionMenuAndRequestsRoundTrip(t *testing.T) {
	var evBuf bytes.Buffer
	v := newSession(options{}, &events{w: &evBuf})
	fb := newFakeMenuBar()
	v.menu = fb

	c1Viewer, c1App := unixPair(t)

	a1 := &app{
		pid:  101,
		conn: remote.NewConn(c1Viewer),
		opts: appOptions(func(o *remote.OpenWindow) {
			o.TitlebarHidden = true
			o.TrafficLightsSet, o.TrafficLightsX, o.TrafficLightsY = true, 16, 14
		}),
	}
	v.attach(a1, true)
	app1Conn := remote.NewConn(c1App)

	// Send menu + dynamic item state updates + window requests from app 1.
	menuMsg := &remote.SetMenu{
		Items: []remote.MenuItem{
			{
				Title: "File",
				Items: []remote.MenuItem{
					{Title: "Save", ID: 7, Key: "s", Cmd: true, Enabled: true},
					{Title: "Dark", ID: 8, Checked: false, Enabled: true},
				},
			},
		},
	}
	if err := app1Conn.Send(menuMsg); err != nil {
		t.Fatal(err)
	}
	if err := app1Conn.Send(&remote.UpdateMenuItem{ID: 7, SetEnabled: true, Enabled: false}); err != nil {
		t.Fatal(err)
	}
	if err := app1Conn.Send(&remote.UpdateMenuItem{ID: 8, SetChecked: true, Checked: true}); err != nil {
		t.Fatal(err)
	}
	if err := app1Conn.Send(&remote.SetTray{
		Key:      1,
		Icon:     []byte{0x89, 'P', 'N', 'G'},
		Template: true,
		Title:    "Tray1",
		Tooltip:  "Tip1",
		Menu:     []remote.MenuItem{{Title: "Show", ID: 9, Enabled: true}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := app1Conn.Send(&remote.SetTray{Key: 2, Title: "Tray2", ClickID: 10}); err != nil {
		t.Fatal(err)
	}
	if err := app1Conn.Send(&remote.RemoveTray{Key: 2}); err != nil {
		t.Fatal(err)
	}
	if err := app1Conn.Send(&remote.SetSize{Width: 640, Height: 480}); err != nil {
		t.Fatal(err)
	}
	if err := app1Conn.Send(&remote.SetPosition{X: 10, Y: 20}); err != nil {
		t.Fatal(err)
	}
	if err := app1Conn.Send(&remote.SetColorMode{Mode: uint8(ggwd.ColorModeDark)}); err != nil {
		t.Fatal(err)
	}
	if err := app1Conn.Send(&remote.SetDockIcon{Visible: false}); err != nil {
		t.Fatal(err)
	}
	if err := app1Conn.Send(&remote.WindowRequest{Request: remote.RequestStartDrag}); err != nil {
		t.Fatal(err)
	}
	if err := app1Conn.Send(&remote.WindowRequest{Request: remote.RequestTitlebarDoubleClick}); err != nil {
		t.Fatal(err)
	}

	// Wait until the reader goroutine has processed the last request.
	deadline := time.Now().Add(2 * time.Second)
	for {
		v.mu.Lock()
		got := len(v.req.window)
		v.mu.Unlock()
		if got == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for requests; got %d window requests", got)
		}
		time.Sleep(5 * time.Millisecond)
	}

	v.mu.Lock()
	r := v.req
	v.req = requests{}
	v.mu.Unlock()

	if r.options == nil || !r.options.TitlebarHidden || !r.options.TrafficLightsSet || r.options.TrafficLightsX != 16 || r.options.TrafficLightsY != 14 {
		t.Fatalf("options = %+v, want TitlebarHidden + TrafficLights(16,14)", r.options)
	}
	if r.size == nil || r.size.Width != 640 || r.size.Height != 480 {
		t.Fatalf("size = %+v", r.size)
	}
	if r.position == nil || r.position.X != 10 || r.position.Y != 20 {
		t.Fatalf("position = %+v", r.position)
	}
	if r.colorMode == nil || r.colorMode.Mode != uint8(ggwd.ColorModeDark) {
		t.Fatalf("colorMode = %+v", r.colorMode)
	}
	if r.dockIcon == nil || r.dockIcon.Visible {
		t.Fatalf("dockIcon = %+v", r.dockIcon)
	}
	if r.menu == nil || len(r.menuUpdates) != 2 {
		t.Fatalf("menu = %+v, updates = %d", r.menu, len(r.menuUpdates))
	}
	if !reflect.DeepEqual(r.window, []uint8{remote.RequestStartDrag, remote.RequestTitlebarDoubleClick}) {
		t.Fatalf("window requests = %v", r.window)
	}

	// Apply menu, trays & updates against fakeMenuBar as pump does.
	v.menu.Set(remoteToNativeMenu(r.menu.Items))
	v.applyTrays(r.trays)
	for _, u := range r.menuUpdates {
		if u.SetEnabled {
			v.menu.SetEnabled(u.ID, u.Enabled)
		}
		if u.SetChecked {
			v.menu.SetChecked(u.ID, u.Checked)
		}
	}
	if len(fb.sets) != 1 || len(fb.sets[0]) != 1 || fb.sets[0][0].Title != "File" {
		t.Fatalf("menu sets = %+v", fb.sets)
	}
	if fb.enabled[7] != false || fb.checked[8] != true {
		t.Fatalf("enabled=%v checked=%v", fb.enabled, fb.checked)
	}
	if len(fb.trays) != 1 || fb.trays[1].Title != "Tray1" || !fb.trays[1].Template || len(fb.trays[1].Menu) != 1 || fb.trays[1].Menu[0].ID != 9 {
		t.Fatalf("trays = %+v", fb.trays)
	}
	if !reflect.DeepEqual(fb.removedTrays, []int{2}) {
		t.Fatalf("removedTrays = %v, want [2]", fb.removedTrays)
	}
	// Reset clears remaining installed trays cleanly.
	v.clearTrays()
	if len(fb.trays) != 0 || !reflect.DeepEqual(fb.removedTrays, []int{2, 1}) {
		t.Fatalf("after clearTrays: trays=%+v removed=%v", fb.trays, fb.removedTrays)
	}

	// Menu click and AppEvent travel back to the attached app.
	received := make(chan remote.Message, 2)
	go func() {
		for range 2 {
			m, err := app1Conn.Receive()
			if err != nil {
				return
			}
			received <- m
		}
	}()
	v.menuSelected(7)
	v.sendAppEvent(remote.AppEvent{Kind: remote.AppEventReopen})

	m1 := <-received
	sel, ok := m1.(*remote.MenuSelected)
	if !ok || sel.ID != 7 {
		t.Fatalf("got %T (%+v), want *remote.MenuSelected{ID:7}", m1, m1)
	}
	m2 := <-received
	ae, ok := m2.(*remote.AppEvent)
	if !ok || ae.Kind != remote.AppEventReopen {
		t.Fatalf("got %T (%+v), want *remote.AppEvent{Kind:AppEventReopen}", m2, m2)
	}

	// Replacing app 1 with app 2 queues a reset and drops stale app 1 updates.
	v.request(a1, &remote.UpdateMenuItem{ID: 7, SetEnabled: true, Enabled: true})
	c2Viewer, c2App := unixPair(t)
	a2 := &app{pid: 202, conn: remote.NewConn(c2Viewer), opts: appOptions(func(o *remote.OpenWindow) {})}
	v.attach(a2, true)
	// A late request from the superseded app 1 must be ignored once app 2 is active.
	v.request(a1, &remote.SetTitle{Title: "stale"})

	v.mu.Lock()
	r2 := v.req
	v.req = requests{}
	v.mu.Unlock()
	if !r2.reset {
		t.Fatal("replacing app did not set reset")
	}
	if len(r2.menuUpdates) != 0 {
		t.Fatalf("replacing app kept stale menuUpdates: %+v", r2.menuUpdates)
	}
	if r2.title != nil {
		t.Fatalf("stale app poisoned active requests with title %q", *r2.title)
	}

	// Closing app 2 detaches it and queues a reset.
	_ = c2App.Close()
	deadline = time.Now().Add(2 * time.Second)
	for {
		v.mu.Lock()
		detached := v.cur == nil && v.req.reset
		v.mu.Unlock()
		if detached {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for app 2 detach reset")
		}
		time.Sleep(5 * time.Millisecond)
	}
	// Clicking a menu item with no attached app is a safe no-op.
	v.menuSelected(7)
}

func TestSetAppHiddenSyncsLastOpts(t *testing.T) {
	hiddenOpts := appOptions(func(o *remote.OpenWindow) {
		o.Width, o.Height = 400, 300
		o.Hidden = true
	})
	last := hiddenOpts
	v := newSession(options{}, &events{w: &bytes.Buffer{}})
	v.lastOpts = &last
	v.appHid = true

	// Detach reset or error overlay shows the window and syncs lastOpts.Hidden.
	v.setAppHidden(false)
	if v.appHid || v.lastOpts.Hidden {
		t.Fatalf("after setAppHidden(false): appHid=%v lastOpts.Hidden=%v, want both false", v.appHid, v.lastOpts.Hidden)
	}
	// A rebuilt app attaching with Hidden: true must hide the window again.
	var w fakeWindow
	planOptions(v.lastOpts, &hiddenOpts, "app", false).apply(&w)
	if got := w.reset(); !reflect.DeepEqual(got, []string{"hide"}) {
		t.Fatalf("rebuilt Hidden:true after reveal = %q, want [hide]", got)
	}

	// Runtime RequestHide syncs lastOpts.Hidden = true so a visible re-attach shows it.
	visibleOpts := appOptions(func(o *remote.OpenWindow) {
		o.Width, o.Height = 400, 300
		o.Hidden = false
	})
	last2 := visibleOpts
	v.lastOpts = &last2
	v.setAppHidden(true)
	if !v.appHid || !v.lastOpts.Hidden {
		t.Fatalf("after setAppHidden(true): appHid=%v lastOpts.Hidden=%v, want both true", v.appHid, v.lastOpts.Hidden)
	}
	planOptions(v.lastOpts, &visibleOpts, "app", false).apply(&w)
	if got := w.reset(); !reflect.DeepEqual(got, []string{"show"}) {
		t.Fatalf("rebuilt Hidden:false after RequestHide = %q, want [show]", got)
	}
}

func TestOverlay(t *testing.T) {
	const w, h = 1000, 800
	if img, _ := (&overlay{}).render(w, h, 2); img != nil {
		t.Error("no overlay drew something")
	}

	// Building is a small note in the top right corner.
	img, l := (&overlay{kind: overlayBuilding}).render(w, h, 2)
	if img == nil || !l.rect.In(image.Rect(w/2, 0, w, h/8)) || img.Rect.Size() != l.rect.Size() {
		t.Errorf("building: %v at %v", img.Rect, l.rect)
	}
	if !hasInk(img) {
		t.Error("building: nothing drawn")
	}

	// A failure's panel is at the bottom, no higher than its share of the
	// window, and scrolls when the output is longer.
	o := &overlay{kind: overlayBuildFailed, output: strings.Repeat("./main.go:1:2: undefined: x\n", 200), scroll: 1000}
	img, l = o.render(w, h, 2)
	if l.rect.Max.Y > h || l.rect.Dy() > int(h*panelMaxHeight) || l.rect.Dx() > w {
		t.Errorf("build-failed: the panel is at %v", l.rect)
	}
	if !l.scrollable() || l.lines != 200 || o.scroll != l.lines-l.rows {
		t.Errorf("build-failed: %d of %d lines from %d", l.rows, l.lines, o.scroll)
	}
	if !hasInk(img) {
		t.Error("build-failed: nothing drawn")
	}

	// A short one fits.
	o = &overlay{kind: overlayAppExited, status: "exit status 2", output: "panic: x\n"}
	_, l = o.render(w, h, 1)
	if l.scrollable() || l.lines != 1 || l.rect.Dy() > h/4 {
		t.Errorf("app-exited: %+v", l)
	}
}

func hasInk(img *image.RGBA) bool {
	if img == nil {
		return false
	}
	for i := 3; i < len(img.Pix); i += 4 {
		if img.Pix[i] != 0 {
			return true
		}
	}
	return false
}

func TestCleanAndWrapText(t *testing.T) {
	if got, want := cleanText("\x1b[31merror\x1b[0m:\r\n\tx\x00y\n\n"), "error:\n    xy"; got != want {
		t.Errorf("cleanText: %q, want %q", got, want)
	}
	if got, want := wrapText("abcdefg\n한글한글한", 3), []string{"abc", "def", "g", "한글한", "글한"}; !reflect.DeepEqual(got, want) {
		t.Errorf("wrapText: %q, want %q", got, want)
	}
	if got := wrapText("", 3); got != nil {
		t.Errorf("wrapText of nothing: %q", got)
	}
}

func TestSessionAppsMapCleansUpOnDetach(t *testing.T) {
	v := newSession(options{}, &events{w: &bytes.Buffer{}})
	for i := range 5 {
		cViewer, cApp := unixPair(t)
		a := &app{pid: 1000 + i, conn: remote.NewConn(cViewer), opts: placeholder}
		v.attach(a, false)
		_ = cApp.Close()
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		v.mu.Lock()
		n := len(v.apps)
		v.mu.Unlock()
		if n == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("v.apps still has %d leaked entries after all apps detached", n)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestAcceptHandshakesConcurrently(t *testing.T) {
	v := newSession(options{}, &events{w: &bytes.Buffer{}})
	v.conns = make(chan *net.UnixConn, 2)
	go v.accept()
	t.Cleanup(func() {
		close(v.conns)
		v.detachAll()
	})

	// First connection stalls without sending Hello.
	stalledViewer, stalledApp := unixPair(t)
	defer stalledApp.Close()
	v.conns <- stalledViewer

	// Second connection completes the handshake immediately and must attach
	// without waiting for handshakeTimeout on the first.
	goodViewer, goodApp := unixPair(t)
	v.conns <- goodViewer

	conn := remote.NewConn(goodApp)
	if _, err := conn.Receive(); err != nil { // Welcome
		t.Fatalf("receiving Welcome: %v", err)
	}
	if err := conn.Send(&remote.Hello{Version: remote.Version, PID: 4242}); err != nil {
		t.Fatal(err)
	}
	if err := conn.Send(&placeholder); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		v.mu.Lock()
		cur := v.cur
		v.mu.Unlock()
		if cur != nil && cur.pid == 4242 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("stalled connection blocked subsequent app handshake")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestPendingBatchCapturesShmSnapshot(t *testing.T) {
	v := newSession(options{}, &events{w: &bytes.Buffer{}})
	cViewer, cApp := unixPair(t)
	a := &app{pid: 555, conn: remote.NewConn(cViewer), opts: placeholder}
	shm1 := []byte("first-shm")
	shm2 := []byte("second-shm")
	a.shm = shm1
	v.attach(a, false)

	appConn := remote.NewConn(cApp)
	if err := appConn.Send(&remote.Batch{Seq: 1, Offset: 0, Length: 5}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		a.mu.Lock()
		n := len(a.pending)
		a.mu.Unlock()
		if n == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for batch 1")
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Simulate shm replacement before replay drains pending.
	a.mu.Lock()
	a.oldShm = append(a.oldShm, a.shm)
	a.shm = shm2
	a.mu.Unlock()

	if err := appConn.Send(&remote.Batch{Seq: 2, Offset: 0, Length: 6}); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(2 * time.Second)
	for {
		a.mu.Lock()
		n := len(a.pending)
		a.mu.Unlock()
		if n == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for batch 2")
		}
		time.Sleep(5 * time.Millisecond)
	}

	a.mu.Lock()
	p0, p1 := a.pending[0], a.pending[1]
	a.mu.Unlock()
	if string(p0.shm) != "first-shm" || string(p1.shm) != "second-shm" {
		t.Fatalf("pending shm snapshots = (%q, %q), want (first-shm, second-shm)", p0.shm, p1.shm)
	}
	_ = cApp.Close()
}

func TestRememberDebouncesStateSaves(t *testing.T) {
	dir := t.TempDir()
	v := newSession(options{project: "/proj"}, &events{w: &bytes.Buffer{}})
	v.statePath = statePath(dir, "/proj")
	v.saveDelay = 30 * time.Millisecond
	defer v.quit()

	v.remember(func(g *geometry) { *g = geometry{X: 1, Y: 2, Width: 300, Height: 200} })
	v.remember(func(g *geometry) { *g = geometry{X: 10, Y: 20, Width: 640, Height: 480} })

	deadline := time.Now().Add(2 * time.Second)
	for {
		if got, ok := loadState(v.statePath, "/proj"); ok && got == (geometry{X: 10, Y: 20, Width: 640, Height: 480}) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("debounced remember did not persist final geometry")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
