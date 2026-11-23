package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"time"

	"github.com/ironpark/ggui/cmd/ggui/internal/config"
	"github.com/ironpark/ggui/cmd/ggui/internal/dev"
	"github.com/ironpark/ggui/cmd/ggui/internal/gotool"
	"github.com/ironpark/ggui/cmd/ggui/internal/macos"
	"github.com/ironpark/ggui/cmd/ggui/internal/project"
	"github.com/ironpark/ggui/cmd/ggui/internal/svgicon"
	"github.com/ironpark/ggui/cmd/ggui/internal/ui"
	"github.com/ironpark/ggui/cmd/ggui/internal/viewer"
	"github.com/ironpark/ggui/cmd/ggui/internal/ziglib"
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
starts the new one. A change to comments or layout alone, a test file
and what git ignores rebuild nothing. A failed build, or a Go file that
does not parse, prints the errors and leaves the running app alone. An app that exits is started again by
the next change. Ctrl-C stops the app and ggui; a second Ctrl-C kills
the app at once.

State the app marks with StateValue.Keep, and a router's location with
Router.Keep, is asked of the app before it is stopped and handed to the
new build as it starts, so that a save does not lose the screen being
worked on; with --web the page carries it across its reload.

On macOS, Linux and Windows the app's window is shown by a viewer, a
process of its own that keeps the window open, where it was and as large
as it was, while the app is rebuilt and restarts: the window shows the
build's progress and a failed build's errors, and says when the app
exits. The app is built with the ggwd_remote tag for it and draws
through the viewer. With --display=window, or dev.display = "window",
each run of the app opens a window of its own, as a release build does.
When there is no viewer to be had, because the project's ggui is too old
for one or it does not build or start, the app opens its own window and
ggui says why; if the viewer goes away, the app is started again in its
own window. Closing the viewer's window ends ggui dev. The viewer shows
one window along with the app's native menu bar and system tray icons;
when the app adds what does not show (such as additional windows), ggui
says so, once, with --display=window to see it.

On macOS ggui dev creates a Name.dev.app bundle in build.output (or
--bundle-dir / dev.bundle_dir) with CFBundleIdentifier <app.id>.dev,
Info.plist and AppIcon.icns, in both --display=remote (where the viewer
runs inside the bundle for the Dock icon, menu bar name and Info.plist)
and --display=window (where the app runs inside it). The bundle is
signed with --sign or dev.identity, else macos.signing_identity, else an
Apple Development identity from the keychain, else ad hoc ("-"). Signing
with a keychain identity preserves macOS TCC permissions across
rebuilds; Info.plist and AppIcon.icns are only rebuilt when they change.

The package is a directory such as ./cmd/app, app.package in ggui.toml
by default. Arguments after -- go to the app, after dev.args.

With --web the app is built for the browser and served, and an open page
reloads itself after each build. A failed build is shown over the page,
which keeps running the last build that succeeded.`,
	flags: func(fs *flag.FlagSet) func(context.Context, []string, []string) error {
		f := newDevFlags(fs)
		return f.run
	},
}

// devFlags are the flags of ggui dev.
type devFlags struct {
	cfgPath, tags, renderer, display, bundleDir, sign *string
	wasmOpt, base, host                               *string
	port                                              *int
	stopTimeout                                       *time.Duration
	noWatch, webMode, verbose                         *bool
	isSet                                             func(string) bool
}

func newDevFlags(fs *flag.FlagSet) *devFlags {
	f := &devFlags{isSet: func(name string) bool { return isSet(fs, name) }}
	f.cfgPath = fs.String("config", "", "read `file` instead of the ggui.toml found from the current directory")
	f.tags = fs.String("tags", "", "build `tags`, comma-separated, replacing dev.tags")
	f.renderer = fs.String("renderer", "", "graphics driver implementation: `go` (default) or zig (native Metal/DX12/Vulkan), replacing build.renderer")
	f.noWatch = fs.Bool("no-watch", false, "build and run once; do not watch for changes")
	f.stopTimeout = fs.Duration("stop-timeout", 3*time.Second, "how long the app has to exit when asked before it is killed")
	f.display = fs.String("display", "", "where the app's window shows: `remote`, in a viewer that stays while the app restarts (the default on macOS, Linux and Windows), or window, its own; replaces dev.display")
	f.bundleDir = fs.String("bundle-dir", "", "on macOS, the `directory` where Name.dev.app is created, replacing dev.bundle_dir (default: build.output)")
	f.sign = fs.String("sign", "", "on macOS, sign Name.dev.app as `identity`, replacing dev.identity; - signs ad hoc")
	f.webMode = fs.Bool("web", false, "build for the browser and serve the app instead of running it")
	f.port = fs.Int("port", 8080, "with --web, the `port` to serve on")
	f.host = fs.String("host", "localhost", "with --web, the `address` to listen on; 0.0.0.0 reaches the server from other devices, such as a phone")
	f.wasmOpt = fs.String("wasm-opt", "", "with --web, wasm-opt `flags` such as -O2; empty skips the pass, which takes seconds")
	f.base = fs.String("base", "", "with --web, the `path` to serve the app below, replacing web.base")
	f.verbose = fs.Bool("verbose", false, "print each decision of the watcher and the rebuild loop, timed: what changed, what was passed over and why, builds started and cancelled; GGUI_VERBOSE=1 does the same")
	return f
}

// devSession is one ggui dev run: the project, its main package and the
// settings the flags and ggui.toml gave it.
type devSession struct {
	f        *devFlags
	proj     *project.Project
	arg      string
	rest     []string
	cfg      config.Config
	main     *project.Main
	say      *ui.Output
	tracef   func(string, ...any)
	tags     []string
	extra    []string
	outDir   string
	first    dev.Settings
	settings func(*project.Project) dev.Settings
}

// run is the dev command once its flags are parsed.
func (f *devFlags) run(ctx context.Context, pos, rest []string) error {
	if len(pos) > 1 {
		fmt.Fprintln(os.Stderr, "ggui dev: takes one package")
		return errUsage
	}
	arg := ""
	if len(pos) == 1 {
		arg = pos[0]
	}
	proj, err := project.Open(".", arg, *f.cfgPath)
	if err != nil {
		return err
	}
	s := &devSession{
		f: f, proj: proj, arg: arg, rest: rest,
		cfg: proj.Config, say: ui.Stderr.Prefixed("ggui"),
	}
	baseTags := proj.Config.Dev.Tags
	if f.isSet("tags") {
		baseTags = splitTags(*f.tags)
	}
	useZig, _, err := ziglib.ResolveRenderer(proj.Config.Build.Renderer, *f.renderer, f.isSet("renderer"), baseTags)
	if err != nil {
		return err
	}
	if useZig {
		switch {
		case *f.webMode:
			if f.isSet("renderer") {
				s.say.Warn("--web always uses WebGL; ignoring --renderer zig")
			}
		case !ziglib.Supported(runtime.GOOS, runtime.GOARCH):
			s.say.Warn("the Zig renderer is not available for %s/%s; using the Go renderer", runtime.GOOS, runtime.GOARCH)
		case !slices.Contains(baseTags, ziglib.NoEmbedTag):
			if _, err := ziglib.Prebuilt(ctx, proj.Root, runtime.GOOS, runtime.GOARCH); err != nil {
				return err
			}
		}
	}
	s.settings = func(p *project.Project) dev.Settings { return f.settings(p, rest) }
	s.first = s.settings(proj)
	s.tags, s.extra = s.first.Tags, s.first.Extra
	s.outDir = proj.Path(s.cfg.Build.Output)
	if *f.verbose || os.Getenv("GGUI_VERBOSE") != "" {
		s.tracef = func(format string, args ...any) {
			s.say.Info("%s", s.say.Dim(fmt.Sprintf("%s "+format, append([]any{time.Now().Format("15:04:05.000")}, args...)...)))
		}
	}
	lo := gotool.ListOptions{Tags: s.tags}
	if *f.webMode {
		lo.GOOS, lo.GOARCH = "js", "wasm"
	}
	if s.main, err = resolveMain(ctx, s.say, proj, arg, lo); err != nil {
		return err
	}
	if *f.webMode {
		return s.runWeb(ctx)
	}
	devBundler, bDir, err := s.bundle(ctx)
	if err != nil {
		return err
	}

	// A second signal skips the graceful stop.
	force, stop := secondSignal()
	defer stop()
	return s.runNative(ctx, force, devBundler, bDir)
}

// settings is what p's file gives the session that it can take up again
// while it runs, the flags replacing what they replace.
func (f *devFlags) settings(p *project.Project, rest []string) dev.Settings {
	s := dev.Settings{
		Tags: p.Config.Dev.Tags,
		Args: append(slices.Clone(p.Config.Dev.Args), rest...),
		Env:  p.Config.Dev.Env,
	}
	if f.isSet("tags") {
		s.Tags = splitTags(*f.tags)
	}
	useZig, cleanedTags, err := ziglib.ResolveRenderer(p.Config.Build.Renderer, *f.renderer, f.isSet("renderer"), s.Tags)
	if err == nil {
		switch {
		case *f.webMode:
			s.Tags = ziglib.WithoutTags(cleanedTags)
		case useZig && ziglib.Supported(runtime.GOOS, runtime.GOARCH):
			s.Tags = ziglib.WithTags(cleanedTags, true)
		case useZig:
			s.Tags = ziglib.WithoutTags(cleanedTags)
		default:
			s.Tags = cleanedTags
		}
	}
	if p.File != "" {
		s.Extra = append(s.Extra, p.File)
	}
	for _, w := range p.Config.Dev.Watch {
		s.Extra = append(s.Extra, p.Path(w))
	}
	if runtime.GOOS == "darwin" && !*f.webMode {
		if p.Config.App.Icon != "" {
			s.Extra = append(s.Extra, p.Path(p.Config.App.Icon))
		}
		if p.Config.MacOS.Entitlements != "" {
			s.Extra = append(s.Extra, p.Path(p.Config.MacOS.Entitlements))
		}
	}
	return s
}

// runWeb serves the app built for js/wasm.
func (s *devSession) runWeb(ctx context.Context) error {
	f, cfg := s.f, s.cfg
	if len(s.rest) > 0 || len(cfg.Dev.Args) > 0 || len(cfg.Dev.Env) > 0 {
		s.say.Warn("a page in a browser takes no arguments or environment; dev.args, dev.env and those after -- are not used with --web")
	}
	if f.isSet("bundle-dir") || f.isSet("sign") {
		s.say.Warn("--bundle-dir and --sign only apply to native macOS dev runs, not --web")
	}
	b := cfg.Web.Base
	if f.isSet("base") {
		b = *f.base
	}
	title := cfg.Web.Title
	if title == "" {
		title = s.main.Name
	}
	w := &dev.Web{
		Dir: s.proj.Root, Package: s.main.Package, PkgDir: s.main.Dir,
		Target: target(s.main), Tags: s.tags,
		LDFlags: "-s -w", WasmOpt: *f.wasmOpt,
		Title: title, Base: b,
		Addr:  net.JoinHostPort(*f.host, strconv.Itoa(*f.port)),
		Extra: s.extra, OutDir: s.outDir, Watch: !*f.noWatch, Log: os.Stderr,
		Config: s.proj.File,
		Tracef: s.tracef,
	}
	return w.Run(ctx)
}

// bundle prepares the macOS dev bundle on darwin, saying how it is
// signed; elsewhere it only warns about the macOS-only flags.
func (s *devSession) bundle(ctx context.Context) (*macos.DevBundle, string, error) {
	f, cfg := s.f, s.cfg
	if runtime.GOOS != "darwin" {
		if f.isSet("bundle-dir") || f.isSet("sign") {
			s.say.Warn("--bundle-dir and --sign only apply on macOS")
		}
		return nil, "", nil
	}
	bDir, err := resolveBundleDir(s.proj, s.outDir, *f.bundleDir, f.isSet("bundle-dir"))
	if err != nil {
		return nil, "", err
	}
	devSign, err := macos.ResolveDevIdentity(ctx, macos.DevSigningRequest{
		FlagIdentity:  *f.sign,
		FlagSet:       f.isSet("sign"),
		DevIdentity:   cfg.Dev.Identity,
		MacOSIdentity: cfg.MacOS.SigningIdentity,
		Entitlements:  s.proj.Path(cfg.MacOS.Entitlements),
	})
	if err != nil {
		return nil, "", err
	}
	for _, w := range devSign.Warnings {
		s.say.Warn("%s", w)
	}
	devBundler := macos.NewDevBundle(devBundleSpec(s.proj, s.main, bDir, s.say), devSign)
	if devSign.AdHoc() {
		s.say.Info("bundle %s (signed ad hoc)", relPath(devBundler.AppPath()))
		s.say.Note("%s", s.say.Dim("ad hoc signature: macOS permissions reset on rebuild; pass --sign, set dev.identity, or install an Apple Development certificate to keep them"))
	} else {
		s.say.Info("bundle %s (signed as %s)", relPath(devBundler.AppPath()), devSign.Display)
	}
	return devBundler, bDir, nil
}

// runNative runs the app in its own process, rebuilt and restarted on
// every change, with the dev bundle and the display asked for.
func (s *devSession) runNative(ctx context.Context, force chan struct{}, devBundler *macos.DevBundle, bDir string) error {
	f, cfg := s.f, s.cfg
	mode := cfg.Dev.Display
	if f.isSet("display") {
		mode = *f.display
	}
	d, err := remoteDisplay(mode, s.proj.Root, viewerTags(s.tags))
	if err != nil {
		return err
	}
	n := &dev.Native{
		Dir: s.proj.Root, Package: s.main.Package, Tags: s.tags,
		Args: s.first.Args, Env: s.first.Env,
		Extra: s.extra, OutDir: s.outDir, Watch: !*f.noWatch,
		StopTimeout: *f.stopTimeout, Force: force,
		Config: s.proj.File,
		// ggui.toml read again when it changes. What the session
		// took from it once and holds for good is named, so that
		// an edit that seems ignored is not a puzzle.
		Reload: func() (dev.Settings, []string, error) {
			p, err := project.Open(".", s.arg, *f.cfgPath)
			if err != nil {
				return dev.Settings{}, nil, err
			}
			var notes []string
			held := func(changed bool, key string) {
				if changed {
					notes = append(notes, key+" changed: start ggui dev again for it to take effect")
				}
			}
			held(p.Config.Dev.Display != cfg.Dev.Display && !f.isSet("display"), "dev.display")
			held(p.Path(p.Config.Build.Output) != s.outDir, "build.output")
			held(p.Config.App.Package != cfg.App.Package && s.arg == "", "app.package")
			if devBundler != nil {
				held(p.Config.Dev.BundleDir != cfg.Dev.BundleDir && !f.isSet("bundle-dir"), "dev.bundle_dir")
				held(p.Config.Dev.Identity != cfg.Dev.Identity && !f.isSet("sign"), "dev.identity")
				held(p.Config.App.Name != cfg.App.Name, "app.name")
				held(p.Config.App.ID != cfg.App.ID, "app.id")
				devBundler.UpdateSpec(devBundleSpec(p, s.main, bDir, s.say), p.Path(p.Config.MacOS.Entitlements))
			}
			next := s.settings(p)
			if d != nil {
				d.Tags = viewerTags(next.Tags)
			}
			return next, notes, nil
		},
		// The app's stdout is the terminal itself; its stderr
		// goes through a pipe for the tail a viewer shows, and on
		// through ui, above any spinner of ggui's.
		Log: os.Stderr, Stdout: os.Stdout, Stderr: ui.Stderr,
		Tracef: s.tracef,
	}
	if devBundler != nil {
		n.Bundle = devBundler
	}
	if d != nil {
		if devBundler != nil {
			d.Bundle = devBundler
		}
		n.Display = d
	}
	return n.Run(ctx)
}

// viewerTags returns the renderer build tags ggviewer needs to match the
// app's resolved dev tags.
func viewerTags(tags []string) []string {
	if slices.Contains(tags, ziglib.Tag) && ziglib.Supported(runtime.GOOS, runtime.GOARCH) {
		return []string{ziglib.Tag}
	}
	return nil
}

// secondSignal returns a channel closed when the interrupt is given a
// second time, and a function that ends the watching.
func secondSignal() (force chan struct{}, stop func()) {
	force = make(chan struct{})
	sigs := make(chan os.Signal, 2)
	signal.Notify(sigs, stopSignals...)
	sigDone := make(chan struct{})
	go func() {
		for range 2 {
			select {
			case <-sigs:
			case <-sigDone:
				return
			}
		}
		close(force)
	}()
	return force, func() {
		close(sigDone)
		signal.Stop(sigs)
	}
}

// remoteDisplay returns the Display for mode, dev.display's value, or nil
// for the app's own window. Remote display where it is not available is
// the app's own window too, which is said when it was asked for by name.
func remoteDisplay(mode, dir string, tags []string) (*dev.RemoteDisplay, error) {
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
			ui.Stderr.Prefixed("ggui").Warn("the app opens its own window: %v", err)
		}
		return nil, nil
	}
	cache, err := viewer.CacheDir()
	if err != nil {
		ui.Stderr.Prefixed("ggui").Warn("the app opens its own window: no cache directory for the viewer: %v", err)
		return nil, nil
	}
	// The viewer's logs go through ui, so that they land above the
	// build's spinner rather than in it.
	return &dev.RemoteDisplay{Dir: dir, CacheDir: cache, Tags: tags, Exe: os.Getenv("GGUI_VIEWER"), Stderr: ui.Stderr, Log: os.Stderr}, nil
}

// resolveMain finds the main package, which takes the go command a moment,
// or longer when it has modules to download first: a spinner says so.
func resolveMain(ctx context.Context, log *ui.Output, proj *project.Project, arg string, lo gotool.ListOptions) (*project.Main, error) {
	s := log.Step("loading the package", ui.QuietStart())
	defer s.Stop()
	return proj.ResolveMain(ctx, arg, lo)
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

// resolveBundleDir returns the directory where macOS Name.dev.app is created:
// --bundle-dir (relative to cwd) when set, else dev.bundle_dir (relative to
// the project root), else outDir (build.output).
func resolveBundleDir(proj *project.Project, outDir, flagVal string, flagSet bool) (string, error) {
	if flagSet {
		if flagVal == "" {
			return outDir, nil
		}
		return filepath.Abs(flagVal)
	}
	if proj.Config.Dev.BundleDir != "" {
		return proj.Path(proj.Config.Dev.BundleDir), nil
	}
	return outDir, nil
}

// devBundleSpec builds a macos.DevBundleSpec from the project configuration
// and resolved main package.
func devBundleSpec(proj *project.Project, main *project.Main, dir string, say *ui.Output) macos.DevBundleSpec {
	cfg := proj.Config
	extra, _ := plistExtra(proj.Root, cfg.MacOS.Plist)
	iconPath := proj.Path(cfg.App.Icon)
	var prepareIcon func(context.Context, string) error
	if iconPath != "" && svgicon.Is(iconPath) {
		root := proj.Root
		name := cfg.App.Icon
		prepareIcon = func(ctx context.Context, dst string) error {
			s := say.Step("drawing " + filepath.Base(iconPath) + " for the dev bundle")
			err := svgicon.Draw(ctx, svgicon.Options{
				Dir:  root,
				SVG:  iconPath,
				Name: name,
				Out:  dst,
			})
			if err != nil {
				s.Fail("drawing " + filepath.Base(iconPath) + " failed")
				return err
			}
			s.Done("drew " + filepath.Base(iconPath))
			return nil
		}
	}
	return macos.DevBundleSpec{
		Name:                 main.Name,
		Executable:           main.Executable,
		ID:                   macos.DevBundleID(main.ID, main.Name),
		Version:              cfg.App.Version,
		MinimumSystemVersion: cfg.MacOS.MinimumSystemVersion,
		Category:             cfg.MacOS.Category,
		Extra:                extra,
		Dir:                  dir,
		Icon:                 iconPath,
		IconName:             iconPath,
		IconInset:            cfg.MacOS.IconInset,
		PrepareIcon:          prepareIcon,
		IconCacheDir:         devIconCacheDir(),
	}
}

func devIconCacheDir() string {
	if d := os.Getenv("GGUI_CACHE_DIR"); d != "" {
		return filepath.Join(d, "dev-icons")
	}
	if d, err := os.UserCacheDir(); err == nil {
		return filepath.Join(d, "ggui", "dev-icons")
	}
	return ""
}

func relPath(path string) string {
	if wd, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(wd, path); err == nil && filepath.IsLocal(rel) {
			return "." + string(filepath.Separator) + rel
		}
	}
	return path
}
