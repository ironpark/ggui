package dev

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/ironpark/ggui/cmd/ggui/internal/gotool"
	"github.com/ironpark/ggui/cmd/ggui/internal/viewer"
)

// RemoteTag is the build tag that links ggwd's remote backend into the
// app: with it, and the viewer's socket in its environment, the app shows
// its window in the viewer; without the socket it opens its own.
const RemoteTag = "ggwd_remote"

// A Display shows the app's window in a viewer, a process of its own
// whose window stays while the app is rebuilt and restarted: remote
// display. It is the fourth piece of the loop, beside the watcher, the
// builder and the runner, and knows nothing of them either.
type Display interface {
	// Start starts a viewer, building it first if it must, and returns
	// once it is ready for an app. An error means there is none to have
	// and the app opens its own window.
	Start(ctx context.Context) (Viewer, error)
	// Stale reports whether changes to paths, which built, call for
	// another viewer: one built from what they changed.
	Stale(ctx context.Context, paths []string) bool
}

// A Viewer is a running viewer.
type Viewer interface {
	// Socket is what the app finds in its environment to connect to.
	Socket() string
	Send(viewer.Command) error
	// Events delivers what the viewer says, and is closed once it exited.
	Events() <-chan viewer.Event
	// Exited describes how a viewer whose Events closed exited.
	Exited() string
	// Stop ends the viewer, killing it if it has not after timeout.
	Stop(timeout time.Duration)
}

// RemoteDisplay is the Display of ggviewer, built from the project's
// module graph and kept in a cache.
type RemoteDisplay struct {
	Dir      string // the project's directory, where the go command runs
	CacheDir string // where the viewer's binaries are kept
	Stderr   io.Writer
	Log      io.Writer // ggui's own messages
	// Exe, when set, is the viewer to run instead of the project's: one
	// being worked on, from GGUI_VIEWER.
	Exe string

	bin  *viewer.Binary
	next *viewer.Binary // a binary Stale found, to start next
	proj string
}

func (d *RemoteDisplay) logf(format string, args ...any) {
	fmt.Fprintf(d.Log, "ggui: "+format+"\n", args...)
}

// Start resolves the viewer of the project's module graph, builds it if
// the cache does not have it and starts it.
func (d *RemoteDisplay) Start(ctx context.Context) (Viewer, error) {
	if d.proj == "" {
		d.proj = d.Dir
		if env, err := gotool.Env(ctx, d.Dir, nil, "GOMOD"); err == nil {
			if m := env["GOMOD"]; filepath.IsAbs(m) {
				d.proj = filepath.Dir(m)
			}
		}
	}
	if d.Exe != "" {
		p, err := viewer.Start(ctx, d.Exe, viewer.Options{Project: d.proj, Stderr: d.Stderr})
		if err != nil {
			return nil, fmt.Errorf("the viewer %s did not start: %w", d.Exe, err)
		}
		return p, nil
	}
	b := d.next
	d.next = nil
	if b == nil {
		var err error
		if b, err = viewer.Resolve(ctx, d.Dir, d.CacheDir); err != nil {
			return nil, err
		}
	}
	if _, err := os.Stat(b.Path); err != nil {
		d.logf("building the viewer, once for these versions of ggui, ggwd and ggfx")
	}
	start := time.Now()
	built, err := b.Ensure(ctx)
	if err != nil {
		return nil, err
	}
	if built {
		d.logf("built the viewer in %s; later sessions find it in %s", time.Since(start).Round(time.Millisecond), filepath.Dir(b.Path))
	}
	p, err := viewer.Start(ctx, b.Path, viewer.Options{Project: d.proj, Stderr: d.Stderr})
	if err != nil {
		return nil, fmt.Errorf("the viewer did not start: %w", err)
	}
	d.bin = b
	return p, nil
}

// Stale resolves the viewer again when paths may change it, and reports
// whether that makes another binary.
func (d *RemoteDisplay) Stale(ctx context.Context, paths []string) bool {
	if d.bin == nil || !d.bin.Affects(paths) {
		return false
	}
	b, err := viewer.Resolve(ctx, d.Dir, d.CacheDir)
	if err != nil || b.Key == d.bin.Key {
		// A graph that lost its viewer keeps the one running.
		return false
	}
	d.next = b
	return true
}
