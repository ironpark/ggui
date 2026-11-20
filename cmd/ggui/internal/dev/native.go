// Package dev is ggui dev's loop: watch the sources, rebuild on a change
// and replace the running app with the new build.
//
// Three pieces do the work and know nothing of each other: the watcher
// (package watch) says that something changed, the builder (package
// gotool, or package web for the browser) turns the sources into a
// program, and the runner (package proc) starts and stops it. A fourth,
// the Display, shows the app's window in a viewer process that outlives
// the app's restarts (package viewer). This package only sequences them,
// and Native.restart is the one place where a running instance and its
// replacement meet.
package dev

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ironpark/ggui/cmd/ggui/internal/gosrc"
	"github.com/ironpark/ggui/cmd/ggui/internal/gotool"
	"github.com/ironpark/ggui/cmd/ggui/internal/proc"
	"github.com/ironpark/ggui/cmd/ggui/internal/ui"
	"github.com/ironpark/ggui/cmd/ggui/internal/viewer"
	"github.com/ironpark/ggui/cmd/ggui/internal/watch"
)

// Native runs a desktop app under development.
type Native struct {
	Dir     string // where the go command runs
	Package string
	Tags    []string
	Args    []string
	Env     map[string]string
	// Extra are paths to watch besides the Go sources: ggui.toml and
	// dev.watch, absolute.
	Extra []string
	// OutDir is where ggui build writes, whose changes never wake the
	// watcher.
	OutDir string
	// Watch rebuilds on change; without it the loop builds and runs once.
	Watch bool
	// StopTimeout is how long the app has to exit when asked before it is
	// killed.
	StopTimeout time.Duration
	// Force, when closed, kills the app at once instead of waiting out
	// StopTimeout: a second Ctrl-C.
	Force <-chan struct{}
	// Config is ggui.toml's path, and Reload reads it again: when the file
	// changes while Run watches, the settings Reload returns replace Tags,
	// Args, Env and Extra, for the build the change starts and the app
	// that build is run as. Notes are said as they are: what in the file
	// changed that only starting ggui dev again takes up. A file that
	// cannot be read is said, and the settings in use stay. Without
	// Reload the settings are those Run started with.
	Config string
	Reload func() (s Settings, notes []string, err error)
	// Display, when set, shows the app in a viewer's window rather than
	// in its own: remote display. When there is no viewer to be had, or
	// it goes away, the app opens its own window for the rest of the
	// session, and ggui says why.
	Display Display
	// Bundle, when set (on macOS), packages the app into <Name>.dev.app
	// when it runs in its own window, and signs the standalone app binary
	// when a viewer holds <Name>.dev.app. Its directory is also ignored by
	// the file watcher.
	Bundle Bundler

	Log    io.Writer // ggui's own messages
	Stdout io.Writer // the app's
	Stderr io.Writer
	// Tracef, when set, is told each decision the watcher and the loop
	// make: ggui dev --verbose.
	Tracef func(format string, args ...any)

	tmp string
	seq int
	app *proc.Process
	exe string // the build app is running, or the last that built
	w   *watch.Watcher
	// set guards list and Extra, which the watcher lists the sources with
	// on a goroutine of its own while a reload replaces them.
	set  sync.Mutex
	list gotool.ListOptions

	plan    plan
	results chan buildResult
	cancel  context.CancelFunc // the running build's
	goVer   chan string        // go env GOVERSION, once known
	parse   *bool              // whether this program parses the project's Go

	viewer   Viewer
	keep     *keeper       // the state the app keeps across restarts, with Watch
	handWait time.Duration // handTimeout, or a test's
	out      *ui.Output    // Log, styled
	step     *ui.Step      // the build of stepID, while it runs
	stepID   int
	tail     *tail     // the end of what the running app wrote to stderr
	changed  time.Time // when the change being built was seen
	cause    string    // "the change", or "ggui dev started"
	built    time.Duration
	started  time.Time
	shown    bool      // the running app's first frame is up
	noticed  noticeSet // the viewer's notices said this session
}

// Settings are what ggui.toml gives a running session that it can take up
// without starting again.
type Settings struct {
	Tags  []string
	Args  []string
	Env   map[string]string
	Extra []string
}

// say returns ggui's own messages' Output: Log, each line marked ggui.
func (n *Native) say() *ui.Output {
	if n.out == nil {
		n.out = ui.For(n.Log).Prefixed("ggui")
	}
	return n.out
}

func (n *Native) logf(format string, args ...any) { n.say().Info(format, args...) }

func (n *Native) warnf(format string, args ...any) { n.say().Warn(format, args...) }

func (n *Native) tracef(format string, args ...any) {
	if n.Tracef != nil {
		n.Tracef(format, args...)
	}
}

// startResult is a viewer that started, or why none did.
type startResult struct {
	v   Viewer
	err error
}

// buildResult is a build that ended.
type buildResult struct {
	id     int
	batch  []string // the changes it is of
	exe    string
	err    error
	syntax bool // err is from parsing, before the build ran
	took   time.Duration
}

// Run builds and runs the app, and with Watch rebuilds and restarts it on
// every change until ctx is done. A failed build leaves the app that is
// running alone; an app that exits by itself is started again by the next
// change that builds. Without Watch it returns when the app exits. With a
// Display, it also returns when the viewer's window is closed.
func (n *Native) Run(ctx context.Context) error {
	ctx, stopAll := context.WithCancel(ctx)
	defer stopAll() // the watcher, and a build in flight
	tmp, err := os.MkdirTemp("", "ggui-dev-")
	if err != nil {
		return err
	}
	n.tmp = tmp
	defer os.RemoveAll(tmp)
	// The app stops first, then its viewer.
	defer n.dropViewer()
	defer n.stop()
	n.results = make(chan buildResult)
	if n.Watch {
		k, err := newKeeper()
		if err != nil {
			n.warnf("state the app keeps is not kept across restarts: %v", err)
		} else {
			n.keep = k
			defer k.close()
		}
	}

	// The viewer starts, which may take a build of it, while the app
	// builds.
	var viewerReady chan startResult
	if n.Display != nil {
		n.Tags = append(slices.Clone(n.Tags), RemoteTag)
		viewerReady = make(chan startResult, 1)
		go func() {
			v, err := n.Display.Start(ctx)
			viewerReady <- startResult{v, err}
		}()
	}
	n.list = gotool.ListOptions{Dir: n.Dir, Tags: n.Tags}

	var changes chan watch.Change
	if n.Watch {
		ignoreDirs := []string{n.OutDir}
		if n.Bundle != nil {
			if d := n.Bundle.Dir(); d != "" && d != n.OutDir {
				ignoreDirs = append(ignoreDirs, d)
			}
		}
		ig := watch.NewIgnore(ignoreDirs...)
		list := func(ctx context.Context) ([]string, error) {
			n.set.Lock()
			lo, extra := n.list, n.Extra
			n.set.Unlock()
			return WatchSet(ctx, lo, n.Package, extra, ig)
		}
		// go list may have modules to download first.
		s := n.say().Step("finding the files to watch", ui.QuietStart())
		set, err := list(ctx)
		s.Stop()
		if err != nil {
			if viewerReady != nil {
				n.useViewer(<-viewerReady)
			}
			return err
		}
		n.w = watch.New(set, watch.Options{
			List: list, Ignore: ig,
			Settle: settleTime, Poll: pollInterval, SafetyNet: safetyNetInterval,
			Logf: n.warnf, Tracef: n.Tracef,
		})
		changes = make(chan watch.Change)
		go n.w.Run(ctx, changes)
		n.logf("watching %d files", n.w.Len())

		n.goVer = make(chan string, 1)
		go func() {
			env, _ := gotool.Env(ctx, n.Dir, nil, "GOVERSION")
			n.goVer <- env["GOVERSION"]
		}()
	}

	id, _, _ := n.plan.start()
	n.changed, n.cause = time.Now(), "ggui dev started"
	n.launch(ctx, id, nil)
	if viewerReady != nil {
		n.useViewer(<-viewerReady)
	}
	due := time.NewTimer(time.Hour)
	defer due.Stop()
	for {
		var exited <-chan struct{}
		if n.app != nil {
			exited = n.app.Done()
		}
		var events <-chan viewer.Event
		if n.viewer != nil {
			events = n.viewer.Events()
		}
		var dueC <-chan time.Time
		if t, ok := n.plan.due(); ok {
			due.Reset(time.Until(t))
			dueC = due.C
		}
		select {
		case <-ctx.Done():
			return nil
		case c := <-changes:
			n.change(c)
		case <-dueC:
			id, batch, since := n.plan.start()
			n.changed, n.cause = since, "the change"
			n.logf("%s changed, rebuilding", n.say().Cyan(describe(n.Dir, batch)))
			if n.Config != "" && slices.Contains(batch, n.Config) {
				n.reload()
			}
			n.send(viewer.Command{Type: viewer.Building})
			n.launch(ctx, id, batch)
		case r := <-n.results:
			if err := n.finish(ctx, r); err != nil {
				return err
			}
		case <-exited:
			msg := n.app.Exited()
			err := n.app.Err()
			n.app.Stop(0) // releases the group
			n.app = nil
			if err == nil && n.windowClosing() {
				n.logf("the window was closed")
				return nil
			}
			if !n.Watch {
				if err != nil {
					return fmt.Errorf("the app %s", msg)
				}
				return nil
			}
			if err != nil {
				n.say().Error("the app %s; waiting for a change to start it again", msg)
			} else {
				n.warnf("the app %s; waiting for a change to start it again", msg)
			}
			n.send(viewer.Command{Type: viewer.AppExited, Status: msg, Output: n.tail.String()})
		case ev, ok := <-events:
			if !ok {
				n.viewerLost(ctx)
				continue
			}
			switch ev.Type {
			case viewer.FirstFrame:
				if n.app != nil && ev.PID == n.app.Pid() && !n.shown {
					n.shown = true
					now := time.Now()
					n.logf("on screen %s after %s (build %s, start to first frame %s)",
						now.Sub(n.changed).Round(time.Millisecond), n.cause, n.built.Round(time.Millisecond),
						now.Sub(n.started).Round(time.Millisecond))
				}
			case viewer.WindowClosed:
				n.logf("the window was closed")
				return nil
			}
		}
	}
}

// reload takes ggui.toml's settings up again after it changed: the build
// about to start is of them, and the app it is run as has their arguments
// and environment. The sources are listed again, since the tags and
// dev.watch decide which files are watched.
func (n *Native) reload() {
	if n.Reload == nil {
		return
	}
	s, notes, err := n.Reload()
	if err != nil {
		n.say().Error("%s: %v; the settings it had stay in use", displayPath(n.Config), err)
		return
	}
	tags := slices.Clone(s.Tags)
	if n.Display != nil {
		tags = append(tags, RemoteTag)
	}
	n.set.Lock()
	n.Tags, n.Args, n.Env, n.Extra = tags, s.Args, s.Env, s.Extra
	n.list.Tags = tags
	n.set.Unlock()
	n.logf("%s read again", displayPath(n.Config))
	for _, note := range notes {
		n.warnf("%s", note)
	}
	if n.w != nil {
		n.w.Relist()
	}
}

// change takes what the watcher found. A real change waits for the plan,
// and cancels a build that runs; a change to comments or layout alone
// builds nothing, which is said when nothing else is under way.
func (n *Native) change(c watch.Change) {
	if len(c.Real) > 0 {
		if n.plan.change(c.At, c.Real) {
			n.tracef("%s changed during build %d: cancelling it", describe(n.Dir, c.Real), n.plan.id)
			n.cancel()
			n.step.Stop()
		}
		return
	}
	if len(c.Cosmetic) > 0 && n.plan.idle() {
		n.logf("%s: comments only; not restarted — line numbers in traces now lag the source", describe(n.Dir, c.Cosmetic))
	}
}

// launch starts build id of the changes in batch. The Go files among them
// are parsed first, when this program can parse the project's Go, so that
// a syntax error is shown at once rather than after the go command has
// loaded the packages. The result comes on n.results.
func (n *Native) launch(ctx context.Context, id int, batch []string) {
	n.seq++
	exe := filepath.Join(n.tmp, fmt.Sprintf("app-%d", n.seq))
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	work := filepath.Join(n.tmp, fmt.Sprintf("work-%d", n.seq))
	var check []string
	for _, p := range batch {
		if strings.HasSuffix(p, ".go") {
			check = append(check, p)
		}
	}
	if len(check) > 0 && !n.canParse() {
		check = nil
	}
	ctx, cancel := context.WithCancel(ctx)
	n.cancel = cancel
	n.tracef("build %d starts", id)
	// The spinner turns only while no app runs: the app writes to the
	// terminal unseen, and a frame would land in its output.
	opts := []ui.StepOption{ui.QuietStart(), ui.NoElapsed()}
	if n.app != nil {
		opts = append(opts, ui.NoSpin())
	}
	n.step, n.stepID = n.say().Step("building the app", opts...), id
	tags := n.Tags // a reload may replace them while this build runs
	go func() {
		defer cancel()
		r := buildResult{id: id, batch: batch}
		if len(check) > 0 {
			if err := gosrc.CheckFiles(ctx, check, displayPath, syntaxRetry); err != nil {
				r.err, r.syntax = err, true
			}
		}
		if r.err == nil {
			start := time.Now()
			r.err = os.MkdirAll(work, 0o755)
			if r.err == nil {
				r.err = gotool.Build(ctx, gotool.BuildOptions{
					Dir: n.Dir, Package: n.Package, Output: exe, Tags: tags, TmpDir: work,
				})
				// A cancelled build is killed, and leaves the go command's
				// work directory behind.
				os.RemoveAll(work)
			}
			r.took = time.Since(start)
			if r.err == nil {
				r.exe = exe
			}
		}
		select {
		case n.results <- r:
		case <-ctx.Done():
			// Cancelled: nobody waits for this build any more.
			os.Remove(exe)
		}
	}()
}

// finish takes a build that ended. One cancelled, or of sources that
// changed since it started, is thrown away; a failure is shown; a success
// restarts the app on it, after a last look at the sources. Without Watch
// a failed build is Run's error.
func (n *Native) finish(ctx context.Context, r buildResult) error {
	if r.err == nil && n.w != nil && n.plan.building && n.plan.id == r.id {
		// A save in the last moments of the build may not have been
		// reported yet: scan now, so that the app does not restart into a
		// build older than the sources.
		n.change(n.w.Sync(ctx))
	}
	if !n.plan.done(r.id) {
		n.tracef("build %d is of sources that changed since; thrown away", r.id)
		if r.id == n.stepID {
			n.step.Stop()
		}
		if r.exe != "" {
			os.Remove(r.exe)
		}
		return nil
	}
	if ctx.Err() != nil {
		n.step.Stop()
		return nil
	}
	if r.err != nil {
		what := "build failed"
		if r.syntax {
			what = "syntax error"
		}
		if n.app != nil {
			n.step.Fail(what + "; the app keeps running the last build that succeeded:")
		} else {
			n.step.Fail(what + ":")
		}
		// As the compiler wrote it, in the file:line:col form terminals
		// and editors make links of; indenting would break them.
		fmt.Fprintln(n.say(), r.err.Error())
		n.send(viewer.Command{Type: viewer.BuildFailed, Output: r.err.Error()})
		if !n.Watch {
			return r.err
		}
		return nil
	}
	n.built = r.took
	n.step.Done(fmt.Sprintf("built in %s", n.built.Round(time.Millisecond)))
	if len(r.batch) > 0 && n.viewer != nil && n.Display.Stale(ctx, r.batch) {
		n.logf("the viewer or its bundle changed; restarting it, and its window with it")
		n.keepState()
		n.stop()
		n.dropViewer()
		v, err := n.Display.Start(ctx)
		n.useViewer(startResult{v, err})
	}
	n.restart(ctx, r.exe)
	return nil
}

// canParse reports whether the go/parser this program was built with
// knows the Go of the toolchain that builds the app. One older may reject
// what the newer added, which the build would accept; then the syntax is
// left to the build.
func (n *Native) canParse() bool {
	if n.parse == nil {
		v := <-n.goVer
		ok := gosrc.CanParse(v)
		n.parse = &ok
		if !ok {
			n.tracef("ggui was built with %s and the app is built with %q: no syntax check before a build", runtime.Version(), v)
		}
	}
	return *n.parse
}

// displayPath names a file in a message as the go command does: relative
// to the working directory, with ./, when it is below it.
func displayPath(path string) string {
	if wd, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(wd, path); err == nil && filepath.IsLocal(rel) {
			return "." + string(filepath.Separator) + rel
		}
	}
	return path
}

// closeGrace is how long an app that exited cleanly is given for its
// viewer to say that its window closed: an app exits when its window is
// closed, a moment before the viewer says so.
const closeGrace = 300 * time.Millisecond

// windowClosing reports whether the viewer says, within closeGrace, that
// its window closed. What else it says meanwhile is about the app that
// exited, and is dropped.
func (n *Native) windowClosing() bool {
	if n.viewer == nil {
		return false
	}
	timer := time.NewTimer(closeGrace)
	defer timer.Stop()
	for {
		select {
		case ev, ok := <-n.viewer.Events():
			if !ok {
				return false // gone: the loop hears of it next
			}
			if ev.Type == viewer.WindowClosed {
				return true
			}
		case <-timer.C:
			return false
		}
	}
}

// useViewer takes the viewer that started, or says why the app opens its
// own window.
func (n *Native) useViewer(r startResult) {
	if r.err != nil {
		if !errors.Is(r.err, context.Canceled) {
			n.warnf("the app opens its own window: %v", r.err)
		}
		return
	}
	n.viewer = r.v
	n.logf("the viewer shows the app: one window, with its native menu bar and tray icons; --display=window opens additional windows")
}

// viewerLost is the viewer's end when nobody closed its window: the app
// opens its own window from then on, running the last build that
// succeeded, as the one that showed in the viewer exits with it.
func (n *Native) viewerLost(ctx context.Context) {
	n.warnf("the viewer ended (%s); the app opens its own window for the rest of this session", n.viewer.Exited())
	n.dropViewer()
	if n.exe == "" {
		return
	}
	n.keepState()
	n.stop()
	n.startPrepared(ctx, n.exe)
}

// dropViewer stops the viewer, if there is one, and forgets it.
func (n *Native) dropViewer() {
	if n.viewer == nil {
		return
	}
	n.viewer.Stop(n.StopTimeout)
	n.viewer = nil
}

// send sends the viewer a command, if there is a viewer. One that went
// away is noticed by its events.
func (n *Native) send(c viewer.Command) {
	if n.viewer != nil {
		_ = n.viewer.Send(c)
	}
}

// restart replaces the running app, if any, with one running exe. The
// running one is asked for the state it keeps before it is stopped, and
// the new one is handed it as it starts: see keeper.
func (n *Native) restart(ctx context.Context, exe string) {
	if n.app != nil {
		n.keepState()
		n.stop()
	}
	if n.exe != "" && n.exe != exe {
		os.Remove(n.exe) // the previous build, no longer running
	}
	n.exe = exe
	n.startPrepared(ctx, exe)
}

// startPrepared stages exe into the dev bundle (when the app owns its own
// window) or signs the standalone binary (when a running viewer owns the
// bundle), then starts it directly via proc.Start.
func (n *Native) startPrepared(ctx context.Context, exe string) {
	runExe := exe
	if n.Bundle != nil {
		if n.viewer == nil {
			staged, warnings, err := n.Bundle.Stage(ctx, exe)
			for _, w := range warnings {
				n.warnf("%s", w)
			}
			if err != nil {
				n.say().Error("preparing the app bundle: %v", err)
				return
			}
			runExe = staged
		} else {
			if err := n.Bundle.SignBinary(ctx, exe); err != nil {
				n.say().Error("signing the app binary: %v", err)
				n.send(viewer.Command{Type: viewer.AppExited, Status: "signing failed", Output: err.Error()})
				return
			}
		}
	}
	n.start(runExe)
}

// start starts the app on exe, in the viewer when there is one.
func (n *Native) start(exe string) {
	// A build's spinner stops turning before the app has the terminal.
	n.step.Static()
	env := proc.Environ(n.Env)
	var ours []string
	if n.viewer != nil {
		ours = append(ours, remoteSocketEnv+"="+n.viewer.Socket())
	}
	if n.keep != nil {
		ours = append(ours, n.keep.handTo())
	}
	if len(ours) > 0 {
		if env == nil {
			env = os.Environ()
		}
		env = append(env, ours...)
	}
	n.tail = &tail{}
	stderr := n.Stderr
	if n.viewer != nil {
		// What the viewer does not show, the app says on stderr; ggui
		// says it instead, where it is not lost among the app's lines.
		say := n.say()
		stderr = &notices{out: n.Stderr, seen: &n.noticed, say: func(what string) {
			say.Warn("%s", restate(what))
		}}
	}
	p, err := proc.Start(exe, slices.Clone(n.Args), proc.Options{
		Dir:    n.Dir,
		Env:    env,
		Stdout: n.Stdout,
		Stderr: io.MultiWriter(stderr, n.tail),
	})
	if err != nil {
		n.say().Error("could not start the app: %v", err)
		n.send(viewer.Command{Type: viewer.AppExited, Status: "did not start", Output: err.Error()})
		return
	}
	n.app = p
	n.started, n.shown = time.Now(), false
}

// remoteSocketEnv is ggwd/remote.EnvSocket, which this command, importing
// nothing of ggui's, spells out.
const remoteSocketEnv = "GGWD_REMOTE_SOCKET"

// keepState asks the running app for the state it keeps, before it is
// stopped to be replaced, for the instance that replaces it.
func (n *Native) keepState() {
	if n.keep == nil || n.app == nil {
		return
	}
	wait := n.handWait
	if wait == 0 {
		wait = handTimeout
	}
	kept, err := n.keep.take(wait)
	if err != nil {
		n.warnf("the app did not hand over the state it keeps within %s; the new one starts without it", wait)
		return
	}
	n.tracef("%d kept values for the new app", kept)
}

// stop stops the running app, asking first, killing it if it does not exit
// within StopTimeout or when Force closes.
func (n *Native) stop() {
	if n.app == nil {
		return
	}
	app := n.app
	n.app = nil
	stopped := make(chan struct{})
	defer close(stopped)
	go func() {
		select {
		case <-n.Force:
			app.Kill()
		case <-stopped:
		}
	}()
	if err := app.Stop(n.StopTimeout); errors.Is(err, proc.ErrKilled) {
		n.warnf("the app %s after %s", err, n.StopTimeout)
	}
}

// describe names the changed files briefly, relative to dir.
func describe(dir string, paths []string) string {
	names := make([]string, 0, 3)
	for _, p := range paths {
		if len(names) == 3 {
			break
		}
		if rel, err := filepath.Rel(dir, p); err == nil && !strings.HasPrefix(rel, "..") {
			p = rel
		}
		names = append(names, p)
	}
	s := strings.Join(names, ", ")
	if more := len(paths) - len(names); more > 0 {
		s += fmt.Sprintf(" and %d more", more)
	}
	return s
}

// tailSize is how much of the end of the app's stderr a viewer is shown
// when the app exits.
const tailSize = 8 << 10

// tail keeps the last tailSize bytes written to it.
type tail struct {
	mu  sync.Mutex
	b   []byte
	cut bool // the start was dropped
}

func (t *tail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.b = append(t.b, p...)
	if over := len(t.b) - tailSize; over > 0 {
		t.b = append(t.b[:0], t.b[over:]...)
		t.cut = true
	}
	return len(p), nil
}

// String returns what was kept, from the start of a line.
func (t *tail) String() string {
	if t == nil {
		return ""
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	s := string(t.b)
	if t.cut {
		if i := strings.IndexByte(s, '\n'); i >= 0 {
			s = s[i+1:]
		}
	}
	return s
}
