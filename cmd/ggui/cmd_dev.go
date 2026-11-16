package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"time"

	"github.com/ironpark/ggui/cmd/ggui/internal/dev"
	"github.com/ironpark/ggui/cmd/ggui/internal/gotool"
	"github.com/ironpark/ggui/cmd/ggui/internal/project"
	"github.com/ironpark/ggui/cmd/ggui/internal/viewer"
)

var devCmd = &command{
	name:    "dev",
	args:    "[package] [-- app arguments]",
	summary: "run an application, rebuilding and restarting it on every change",
	help: `Dev builds the application with the development tags (dev.tags in
ggui.toml, ggui_inspector and ggui_debug by default) and runs it. It
watches the Go files the app is built from in the modules being edited,
the files it embeds, ggui.toml and the paths dev.watch names; on a
change it rebuilds and, when the build succeeds, stops the app and
starts the new one. A failed build prints the compiler's errors and
leaves the running app alone. An app that exits is started again by
the next change. Ctrl-C stops the app and ggui; a second Ctrl-C kills
the app at once.

On macOS the app's window is shown by a viewer, a process of its own
that keeps the window open, where it was and as large as it was, while
the app is rebuilt and restarts: the window shows the build's progress
and a failed build's errors, and says when the app exits. The app is
built with the ggwd_remote tag for it and draws through the viewer. With
--display=window, or dev.display = "window", each run of the app opens
a window of its own, as a release build does. When there is no viewer
to be had, because the project's ggui is too old for one or it does not
build or start, the app opens its own window and ggui says why; if the
viewer goes away, the app is started again in its own window. Closing
the viewer's window ends ggui dev.

The package is a directory such as ./cmd/app, app.package in ggui.toml
by default. Arguments after -- go to the app, after dev.args.

With --web the app is built for the browser and served, and an open page
reloads itself after each build. A failed build is shown over the page,
which keeps running the last build that succeeded.`,
	flags: func(fs *flag.FlagSet) func(context.Context, []string, []string) error {
		cfgPath := fs.String("config", "", "read `file` instead of the ggui.toml found from the current directory")
		tags := fs.String("tags", "", "build `tags`, comma-separated, replacing dev.tags")
		noWatch := fs.Bool("no-watch", false, "build and run once; do not watch for changes")
		stopTimeout := fs.Duration("stop-timeout", 3*time.Second, "how long the app has to exit when asked before it is killed")
		display := fs.String("display", "", "where the app's window shows: `remote`, in a viewer that stays while the app restarts (the default on macOS), or window, its own; replaces dev.display")
		webMode := fs.Bool("web", false, "build for the browser and serve the app instead of running it")
		port := fs.Int("port", 8080, "with --web, the `port` to serve on")
		host := fs.String("host", "localhost", "with --web, the `address` to listen on; 0.0.0.0 reaches the server from other devices, such as a phone")
		wasmOpt := fs.String("wasm-opt", "", "with --web, wasm-opt `flags` such as -O2; empty skips the pass, which takes seconds")
		base := fs.String("base", "", "with --web, the `path` to serve the app below, replacing web.base")
		return func(ctx context.Context, pos, rest []string) error {
			if len(pos) > 1 {
				fmt.Fprintln(os.Stderr, "ggui dev: takes one package")
				return errUsage
			}
			arg := ""
			if len(pos) == 1 {
				arg = pos[0]
			}
			proj, err := project.Open(".", arg, *cfgPath)
			if err != nil {
				return err
			}
			cfg := proj.Config
			devTags := cfg.Dev.Tags
			if isSet(fs, "tags") {
				devTags = splitTags(*tags)
			}
			var extra []string
			if proj.File != "" {
				extra = append(extra, proj.File)
			}
			for _, w := range cfg.Dev.Watch {
				extra = append(extra, proj.Path(w))
			}
			lo := gotool.ListOptions{Tags: devTags}
			if *webMode {
				lo.GOOS, lo.GOARCH = "js", "wasm"
			}
			main, err := proj.ResolveMain(ctx, arg, lo)
			if err != nil {
				return err
			}

			if *webMode {
				if len(rest) > 0 || len(cfg.Dev.Args) > 0 || len(cfg.Dev.Env) > 0 {
					fmt.Fprintln(os.Stderr, "ggui: a page in a browser takes no arguments or environment; dev.args, dev.env and those after -- are not used with --web")
				}
				b := cfg.Web.Base
				if isSet(fs, "base") {
					b = *base
				}
				title := cfg.Web.Title
				if title == "" {
					title = main.Name
				}
				w := &dev.Web{
					Dir: proj.Root, Package: main.Package, PkgDir: main.Dir,
					Target: target(main), Tags: devTags,
					LDFlags: "-s -w", WasmOpt: *wasmOpt,
					Title: title, Base: b,
					Addr:  net.JoinHostPort(*host, strconv.Itoa(*port)),
					Extra: extra, Watch: !*noWatch, Log: os.Stderr,
				}
				return w.Run(ctx)
			}

			// A second signal skips the graceful stop.
			force := make(chan struct{})
			sigs := make(chan os.Signal, 2)
			signal.Notify(sigs, stopSignals...)
			defer signal.Stop(sigs)
			go func() {
				<-sigs
				<-sigs
				close(force)
			}()

			n := &dev.Native{
				Dir: proj.Root, Package: main.Package, Tags: devTags,
				Args:  append(slices.Clone(cfg.Dev.Args), rest...),
				Env:   cfg.Dev.Env,
				Extra: extra, Watch: !*noWatch,
				StopTimeout: *stopTimeout, Force: force,
				Log: os.Stderr, Stdout: os.Stdout, Stderr: os.Stderr,
			}
			mode := cfg.Dev.Display
			if isSet(fs, "display") {
				mode = *display
			}
			d, err := remoteDisplay(mode, proj.Root)
			if err != nil {
				return err
			}
			if d != nil {
				n.Display = d
			}
			return n.Run(ctx)
		}
	},
}

// remoteDisplay returns the Display for mode, dev.display's value, or nil
// for the app's own window. Remote display where it is not available is
// the app's own window too, which is said when it was asked for by name.
func remoteDisplay(mode, dir string) (*dev.RemoteDisplay, error) {
	switch mode {
	case "window":
		return nil, nil
	case "", "remote":
	default:
		fmt.Fprintf(os.Stderr, "ggui dev: --display is remote or window, not %q\n", mode)
		return nil, errUsage
	}
	if err := viewer.Supported(); err != nil {
		if mode == "remote" {
			fmt.Fprintf(os.Stderr, "ggui: the app opens its own window: %v\n", err)
		}
		return nil, nil
	}
	cache, err := viewer.CacheDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ggui: the app opens its own window: no cache directory for the viewer: %v\n", err)
		return nil, nil
	}
	return &dev.RemoteDisplay{Dir: dir, CacheDir: cache, Exe: os.Getenv("GGUI_VIEWER"), Stderr: os.Stderr, Log: os.Stderr}, nil
}

// target names the package for messages: its directory relative to the
// working directory when it is below it, its import path otherwise.
func target(m *project.Main) string {
	if wd, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(wd, m.Dir); err == nil && rel == "." {
			return "."
		} else if err == nil && filepath.IsLocal(rel) {
			return "./" + filepath.ToSlash(rel)
		}
	}
	return m.ImportPath
}
