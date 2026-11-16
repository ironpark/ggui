//go:build (unix && !ios && !android) || windows

package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ironpark/ggwd"
	"github.com/ironpark/ggwd/remote"
	"github.com/ironpark/ggwd/remote/viewer"
)

// TestEndToEnd runs the viewer and examples/counter, built with the
// remote backend, as ggui dev does, on the system's own window and GPU
// (DirectX on Windows, which GGFX_DIRECTX=warp makes software). It is
// slow and opens a window, so it runs only with GGUI_E2E set; on Linux it
// needs an X display, and a window manager for the minimize. The pictures
// of the window it takes go to GGUI_E2E_DIR when that is set.
//
// It checks the events the launcher waits for, that the app's frames
// show and take input, that a build failure shows over them, that a
// resize reaches the app, that a restarted app takes the same window,
// and that one started while the window is hidden or minimized still
// gets its frames through.
func TestEndToEnd(t *testing.T) {
	if os.Getenv("GGUI_E2E") == "" {
		t.Skip("GGUI_E2E is not set")
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" && os.Getenv("DISPLAY") == "" {
		t.Skip("no X display (DISPLAY is not set)")
	}
	bin := t.TempDir()
	exe := func(name string) string {
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		return filepath.Join(bin, name)
	}
	viewerExe, appExe := exe("ggviewer"), exe("counter")
	build(t, "-o", viewerExe, ".")
	build(t, "-tags", "ggwd_remote", "-o", appExe, "../../examples/counter")
	// The window's pictures go to GGUI_E2E_DIR when it is set, to be looked at.
	shots := bin
	if d := os.Getenv("GGUI_E2E_DIR"); d != "" {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		shots = d
	}

	// The socket's directory is short: a unix socket's path is.
	dir, err := os.MkdirTemp("", "ggv")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	v := startViewer(t, viewerExe, filepath.Join(dir, "v.sock"))
	ready := v.expect("ready", 0)
	if ready.Socket == "" {
		t.Fatal("ready named no socket")
	}

	app := v.startApp(appExe, ready.Socket)
	first := v.dump(filepath.Join(shots, "1.png"))
	checkDrawn(t, "the first frame", first)

	// T flips the counter's theme: input reaches the app, and its new
	// frame shows.
	v.input(ggwd.KeyEvent{Key: ggwd.KeyT, Pressed: true})
	v.input(ggwd.KeyEvent{Key: ggwd.KeyT})
	flipped := v.dumpUntil(filepath.Join(shots, "2.png"), "the theme to flip", func(m image.Image) bool {
		return changed(first, m) > 0.2
	})
	checkDrawn(t, "the flipped theme", flipped)

	// A build failure shows over the frame, and the next frame clears it.
	v.send(command{Type: cmdBuildFailed, Output: "./main.go:1:1: expected 'package', found 'oops'"})
	v.dumpUntil(filepath.Join(shots, "3.png"), "the build failure's overlay", func(m image.Image) bool {
		return changed(flipped, m) > 0.01
	})
	v.send(command{Type: cmdClear})

	// A resize reaches the app, whose frame takes the new size.
	before := first.Bounds().Size()
	v.send(command{Type: cmdDebugWindow, Action: "resize", Width: 520, Height: 420})
	resized := v.dumpUntil(filepath.Join(shots, "4.png"), "the frame to take the new size", func(m image.Image) bool {
		return m.Bounds().Size() != before
	})
	checkDrawn(t, "the resized frame", resized)

	// A restarted app takes the same window, at its size.
	v.restart(app, appExe, ready.Socket)
	app = v.cur
	restarted := v.dump(filepath.Join(shots, "5.png"))
	if restarted.Bounds().Size() != resized.Bounds().Size() {
		t.Errorf("the restarted app's frame is %v, the window's was %v", restarted.Bounds().Size(), resized.Bounds().Size())
	}
	checkDrawn(t, "the restarted app's first frame", restarted)

	// An app started while the window is away gets its frames through:
	// the viewer replays them in background frames.
	for _, away := range []string{"hide", "minimize"} {
		v.send(command{Type: cmdDebugWindow, Action: away})
		time.Sleep(500 * time.Millisecond)
		v.restart(app, appExe, ready.Socket)
		app = v.cur
		back := "show"
		if away == "minimize" {
			back = "restore"
		}
		v.send(command{Type: cmdDebugWindow, Action: back})
		checkDrawn(t, "the frame after "+away, v.dump(filepath.Join(shots, away+".png")))
	}

	v.send(command{Type: cmdQuit})
	v.wait()
}

func build(t *testing.T, args ...string) {
	t.Helper()
	cmd := exec.Command("go", append([]string{"build"}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

type e2eViewer struct {
	t      *testing.T
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	events chan event
	stderr *bytes.Buffer // the viewer's and the apps' output, under logMu
	logMu  sync.Mutex
	exited chan struct{}
	cur    *exec.Cmd // the app started last
	apps   []*exec.Cmd
}

func startViewer(t *testing.T, exe, socket string) *e2eViewer {
	t.Helper()
	cmd := exec.Command(exe, "-socket", socket, "-debug")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	v := &e2eViewer{t: t, cmd: cmd, stdin: stdin, events: make(chan event, 64), stderr: &bytes.Buffer{}, exited: make(chan struct{})}
	cmd.Stderr = &lockedWriter{mu: &v.logMu, w: v.stderr}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() {
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			var ev event
			if err := json.Unmarshal(sc.Bytes(), &ev); err == nil {
				v.events <- ev
			}
		}
		_ = cmd.Wait()
		close(v.exited)
	}()
	t.Cleanup(func() {
		_ = stdin.Close()
		select {
		case <-v.exited:
		case <-time.After(10 * time.Second):
			_ = cmd.Process.Kill()
			<-v.exited
		}
		for _, a := range v.apps {
			_ = a.Process.Kill()
			_, _ = a.Process.Wait()
		}
		if t.Failed() {
			v.logMu.Lock()
			t.Logf("the viewer's and the apps' output:\n%s", v.stderr)
			v.logMu.Unlock()
		}
	})
	return v
}

// lockedWriter is a writer that the processes' output shares.
type lockedWriter struct {
	mu *sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(b []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(b)
}

const e2eTimeout = 90 * time.Second

// expect waits for an event of type typ, of pid when it is not 0,
// skipping the others.
func (v *e2eViewer) expect(typ string, pid int) event {
	v.t.Helper()
	deadline := time.After(e2eTimeout)
	for {
		select {
		case ev := <-v.events:
			if ev.Type == typ && (pid == 0 || ev.PID == pid) {
				return ev
			}
		case <-v.exited:
			v.t.Fatalf("the viewer exited waiting for %s", typ)
		case <-deadline:
			v.t.Fatalf("no %s in %v", typ, e2eTimeout)
		}
	}
}

func (v *e2eViewer) send(c command) {
	v.t.Helper()
	b, err := json.Marshal(c)
	if err != nil {
		v.t.Fatal(err)
	}
	if _, err := v.stdin.Write(append(b, '\n')); err != nil {
		v.t.Fatal(err)
	}
}

func (v *e2eViewer) input(ev ggwd.Event) {
	v.t.Helper()
	in, ok := viewer.Input(ev)
	if !ok {
		v.t.Fatalf("%T is not input", ev)
	}
	v.send(command{Type: cmdDebugInput, Input: &in})
}

// startApp starts the app and waits for its first frame.
func (v *e2eViewer) startApp(exe, socket string) *exec.Cmd {
	v.t.Helper()
	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(), remote.EnvSocket+"="+socket)
	log := &lockedWriter{mu: &v.logMu, w: v.stderr}
	cmd.Stdout, cmd.Stderr = log, log
	if err := cmd.Start(); err != nil {
		v.t.Fatal(err)
	}
	v.apps = append(v.apps, cmd)
	v.cur = cmd
	v.expect("app-attached", cmd.Process.Pid)
	v.expect("first-frame", cmd.Process.Pid)
	return cmd
}

// restart ends app, as ggui dev does on a change, and starts another.
func (v *e2eViewer) restart(app *exec.Cmd, exe, socket string) {
	v.t.Helper()
	_ = app.Process.Kill()
	_, _ = app.Process.Wait()
	v.expect("app-detached", app.Process.Pid)
	v.startApp(exe, socket)
}

func (v *e2eViewer) dump(path string) image.Image {
	v.t.Helper()
	v.send(command{Type: cmdDebugDump, Path: path})
	for {
		if ev := v.expect("dumped", 0); ev.Path == path {
			break
		}
	}
	f, err := os.Open(path)
	if err != nil {
		v.t.Fatal(err)
	}
	defer f.Close()
	m, err := png.Decode(f)
	if err != nil {
		v.t.Fatal(err)
	}
	return m
}

// dumpUntil dumps the window until ok says the picture is what it waits for.
func (v *e2eViewer) dumpUntil(path, what string, ok func(image.Image) bool) image.Image {
	v.t.Helper()
	deadline := time.Now().Add(e2eTimeout)
	for {
		m := v.dump(path)
		if ok(m) {
			return m
		}
		if time.Now().After(deadline) {
			v.t.Fatalf("no dump showed %s in %v (the last is %s)", what, e2eTimeout, path)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func (v *e2eViewer) wait() {
	v.t.Helper()
	select {
	case <-v.exited:
	case <-time.After(e2eTimeout):
		v.t.Fatal("the viewer did not quit")
	}
}

// checkDrawn checks that m is a frame of the counter rather than an empty
// window: it has a background and a card on it, with text.
func checkDrawn(t *testing.T, what string, m image.Image) {
	t.Helper()
	b := m.Bounds()
	if b.Dx() < 100 || b.Dy() < 100 {
		t.Errorf("%s is %v", what, b.Size())
		return
	}
	counts := map[[4]uint32]int{}
	for y := b.Min.Y; y < b.Max.Y; y += 2 {
		for x := b.Min.X; x < b.Max.X; x += 2 {
			r, g, bl, a := m.At(x, y).RGBA()
			counts[[4]uint32{r >> 8, g >> 8, bl >> 8, a >> 8}]++
		}
	}
	total, top := 0, 0
	for _, n := range counts {
		total += n
		top = max(top, n)
	}
	if len(counts) < 8 || top > total*98/100 {
		t.Errorf("%s looks blank: %d colours, the commonest on %d of %d pixels", what, len(counts), top, total)
	}
}

// changed returns the share of the pixels of b that differ from a's, 1
// when their sizes differ.
func changed(a, b image.Image) float64 {
	if a.Bounds() != b.Bounds() {
		return 1
	}
	r := a.Bounds()
	n, diff := 0, 0
	for y := r.Min.Y; y < r.Max.Y; y += 2 {
		for x := r.Min.X; x < r.Max.X; x += 2 {
			n++
			r1, g1, b1, a1 := a.At(x, y).RGBA()
			r2, g2, b2, a2 := b.At(x, y).RGBA()
			if r1 != r2 || g1 != g2 || b1 != b2 || a1 != a2 {
				diff++
			}
		}
	}
	return float64(diff) / float64(n)
}
