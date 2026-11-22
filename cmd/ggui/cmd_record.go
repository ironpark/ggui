package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/ironpark/ggui/cmd/ggui/internal/gotool"
	"github.com/ironpark/ggui/cmd/ggui/internal/proc"
	"github.com/ironpark/ggui/cmd/ggui/internal/project"
	"github.com/ironpark/ggui/cmd/ggui/internal/ui"
)

var recordCmd = &command{
	name:    "record",
	args:    "[name]",
	summary: "draw a tour of the application into an animated GIF",
	help: `Record draws a tour the application registered with package record
(github.com/ironpark/ggui/record) into an animated GIF, or into numbered
PNG frames with --frames. The tour is a script in the app: what it
shows, the steps it takes and how long each is held. Nothing is
captured from the screen and no window opens: the app draws each frame
offscreen, on a clock of its own, so a recording takes about as long as
drawing its frames and comes out the same every time.

Record builds the app with the release tags (build.tags in ggui.toml)
and runs it with GGUI_RECORD set, which ggui.Run reads: it draws the
tour asked for and exits instead of opening its window. The app runs in
the project's directory with dev.env added to its environment. What
it prints goes to the terminal.

The name selects the tour; with none, the tour registered with no name,
or the only one. --list prints the names. The GIF is name.gif, or
tour.gif, in the project's directory, unless -o names another file.

The GIF has one palette for the whole recording, chosen from its
colors, and each frame after the first holds only the pixels that
changed, so a tour that mostly holds still makes a small file. --fps
keeps fewer frames than the tour draws, --width scales them down,
--colors makes the palette smaller, --dither mixes its colors where
gradients band, and --lossy trades exact colors for a smaller file.
docs/recording.md has the details.

--frame mac draws each frame as a macOS window, with a title bar,
rounded corners and a shadow, on a backdrop that --backdrop sets, in
place of the frame the tour asks for; --frame none draws the content
alone. The other mac frames are the same window with another title bar:
mac-titleless (no title), mac-unified (a 52pt unified toolbar, the
title after the buttons), mac-compact (a 38pt compact toolbar),
mac-expanded (the title row over a 38pt toolbar row) and mac-hidden (no
bar, the buttons over the content). --cursor=false leaves out the
cursor the tour's pointer steps draw.`,
	flags: func(fs *flag.FlagSet) func(context.Context, []string, []string) error {
		f := newRecordFlags(fs)
		return f.run
	},
}

// recordFlags are the flags of ggui record.
type recordFlags struct {
	cfgPath, pkg, tags, out, frames *string
	fps, width, colors              *int
	dither                          *float64
	loop, lossy                     *int
	frame, backdrop                 *string
	list, cursor                    *bool
	isSet                           func(string) bool
}

func newRecordFlags(fs *flag.FlagSet) *recordFlags {
	f := &recordFlags{isSet: func(name string) bool { return isSet(fs, name) }}
	f.cfgPath = fs.String("config", "", "read `file` instead of the ggui.toml found from the current directory")
	f.pkg = fs.String("package", "", "the main `package` to build, replacing app.package")
	f.tags = fs.String("tags", "", "build `tags`, comma-separated, replacing build.tags")
	f.out = fs.String("o", "", "write the GIF to `file` instead of name.gif in the project's directory")
	f.frames = fs.String("frames", "", "write PNG frames, 0000.png and on, to `directory` instead of a GIF")
	f.list = fs.Bool("list", false, "print the names of the tours the app registered")
	f.fps = fs.Int("fps", 0, "keep `n` frames a second; 0 keeps every frame the tour draws")
	f.width = fs.Int("width", 0, "scale the frames down to `px` pixels wide; 0 keeps the size drawn")
	f.colors = fs.Int("colors", 256, "the GIF palette's `n` colors, 2 to 256")
	f.dither = fs.Float64("dither", 0, "ordered dithering's `strength`, 0 (none) to 1; 0.3 smooths gradients that band")
	f.loop = fs.Int("loop", 0, "play the GIF `n` times; 0 loops for ever")
	f.lossy = fs.Int("lossy", 0, "let the GIF's compression write colors up to `n`/1000 off in Oklab, 0 (exact) to 200; 20 to 40 is hard to see")
	f.frame = fs.String("frame", "", "draw the frames in a `style` of window frame, in place of the tour's: "+strings.Join(recordFrames, ", "))
	f.backdrop = fs.String("backdrop", "", "the `color` a framed window is drawn on, #RRGGBB or transparent")
	f.cursor = fs.Bool("cursor", true, "draw the cursor the tour's pointer steps move")
	return f
}

// run is the record command once its flags are parsed.
func (f *recordFlags) run(ctx context.Context, pos, rest []string) error {
	name, err := f.check(pos, rest)
	if err != nil {
		return err
	}
	proj, err := project.Open(".", *f.pkg, *f.cfgPath)
	if err != nil {
		return err
	}
	cfg := proj.Config
	buildTags := cfg.Build.Tags
	if f.isSet("tags") {
		buildTags = splitTags(*f.tags)
	}
	say := ui.Stderr.Prefixed("ggui")
	main, err := resolveMain(ctx, say, proj, *f.pkg, gotool.ListOptions{Tags: buildTags})
	if err != nil {
		return err
	}

	tmp, err := os.MkdirTemp("", "ggui-record-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	exe, err := f.build(ctx, proj, main, buildTags, tmp)
	if err != nil {
		return err
	}
	req, err := f.request(name, proj)
	if err != nil {
		return err
	}
	rep, err := f.record(ctx, proj, exe, req, tmp)
	if err != nil {
		return err
	}
	if *f.list {
		if len(rep.Tours) == 0 {
			say.Warn("%s registered no tours: see docs/recording.md", main.Name)
		}
		for _, t := range rep.Tours {
			if t == "" {
				t = "(default)"
			}
			fmt.Println(t)
		}
		return nil
	}
	return reportRecording(req.Out, req.Format, rep)
}

// check reads the arguments and flags, returning the tour's name.
func (f *recordFlags) check(pos, rest []string) (string, error) {
	if len(pos) > 1 || len(rest) > 0 {
		fmt.Fprintln(os.Stderr, "ggui record: takes one tour name")
		return "", errUsage
	}
	name := ""
	if len(pos) == 1 {
		name = pos[0]
	}
	if len(pos) == 1 && !validTourName(name) {
		return "", fmt.Errorf("tour name %q must not be empty, a dot name, or contain path separators or \"..\"", name)
	}
	if err := f.checkValues(); err != nil {
		return "", err
	}
	return name, f.checkChoices()
}

// checkValues checks the flags' values are in range.
func (f *recordFlags) checkValues() error {
	switch {
	case *f.out != "" && *f.frames != "":
		return errors.New("-o writes a GIF and --frames PNG frames: give one")
	case *f.colors < 2 || *f.colors > 256:
		return fmt.Errorf("--colors %d: a GIF has 2 to 256 colors", *f.colors)
	case *f.dither < 0 || *f.dither > 1:
		return fmt.Errorf("--dither %g: the strength is 0 to 1", *f.dither)
	case *f.fps < 0 || *f.width < 0 || *f.loop < 0:
		return errors.New("--fps, --width and --loop are not negative")
	case *f.lossy < 0 || *f.lossy > 200:
		return fmt.Errorf("--lossy %d: it is 0 to 200", *f.lossy)
	}
	return nil
}

// checkChoices checks the frame style and backdrop name something known.
func (f *recordFlags) checkChoices() error {
	switch {
	case *f.frame != "" && !slices.Contains(recordFrames, *f.frame):
		return fmt.Errorf("--frame %s: the frames are %s", *f.frame, strings.Join(recordFrames, ", "))
	case *f.backdrop != "" && !validBackdrop(*f.backdrop):
		return fmt.Errorf("--backdrop %s: give #RRGGBB or transparent", *f.backdrop)
	}
	return nil
}

// build compiles the app to record into tmp, returning the executable.
func (f *recordFlags) build(ctx context.Context, proj *project.Project, main *project.Main, tags []string, tmp string) (string, error) {
	exe := filepath.Join(tmp, main.Executable)
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	bo := gotool.BuildOptions{Dir: proj.Root, Package: main.Package, Output: exe, Tags: tags, TmpDir: tmp}
	ui.Stderr.Println(ui.Stderr.Dim(bo.String()))
	if err := step(ctx, "Building "+main.Name, "Built "+main.Name, func() error { return gotool.Build(ctx, bo) }); err != nil {
		return "", err
	}
	return exe, nil
}

// request is what the app is asked to record, from the flags.
func (f *recordFlags) request(name string, proj *project.Project) (recordRequest, error) {
	req := recordRequest{V: recordProtocol, Tour: name}
	switch {
	case *f.list:
		req.Format = "list"
	case *f.frames != "":
		req.Format, req.Out = "png", *f.frames
	default:
		req.Format, req.Out = "gif", *f.out
		if req.Out == "" {
			base := name
			if base == "" {
				base = "tour"
			}
			req.Out = filepath.Join(proj.Root, base+".gif")
		}
	}
	if req.Out != "" {
		abs, err := filepath.Abs(req.Out)
		if err != nil {
			return req, err
		}
		req.Out = abs
	}
	if req.Format != "list" {
		req.FPS, req.Width, req.Loop, req.Dither = *f.fps, *f.width, *f.loop, *f.dither
		req.Lossy = *f.lossy
		req.Frame, req.Backdrop, req.NoCursor = *f.frame, *f.backdrop, !*f.cursor
	}
	return req, nil
}

// record runs the app built to record with the request in its
// environment, and reads what it reports it did.
func (f *recordFlags) record(ctx context.Context, proj *project.Project, exe string, req recordRequest, tmp string) (recordReport, error) {
	reportPath := filepath.Join(tmp, "report.json")
	req.Report = reportPath
	data, err := json.Marshal(req)
	if err != nil {
		return recordReport{}, err
	}
	env := append(os.Environ(), proc.Environ(proj.Config.Dev.Env)...)
	env = append(env, recordEnv+"="+string(data))

	title, done := "Recording "+tourName(req.Tour), "Recorded "+tourName(req.Tour)
	if *f.list {
		title, done = "Listing the tours", "Listed the tours"
	}
	var rep recordReport
	err = step(ctx, title, done, func() error {
		if err := runRecorder(ctx, exe, proj.Root, env); err != nil {
			return err
		}
		return readRecordReport(reportPath, &rep)
	})
	if err != nil {
		return rep, err
	}
	return rep, nil
}

// recordEnv is the variable the app reads its request from, which
// package record names Env.
const recordEnv = "GGUI_RECORD"

// recordProtocol is the version of the request and the report.
const recordProtocol = 1

// recordRequest is what recordEnv holds, as package record reads it. The
// two modules share no code; a test on each side checks one golden
// string (record/record_test.go and cmd_record_test.go).
type recordRequest struct {
	V      int     `json:"v"`
	Tour   string  `json:"tour"`
	Format string  `json:"format"`
	Out    string  `json:"out,omitempty"`
	FPS    int     `json:"fps,omitempty"`
	Width  int     `json:"width,omitempty"`
	Colors int     `json:"colors,omitempty"`
	Dither float64 `json:"dither,omitempty"`
	Loop   int     `json:"loop,omitempty"`
	Lossy  int     `json:"lossy,omitempty"`
	// Frame is one of recordFrames, or "" for the tour's; Backdrop is
	// "#RRGGBB" or "transparent".
	Frame    string `json:"frame,omitempty"`
	Backdrop string `json:"backdrop,omitempty"`
	NoCursor bool   `json:"no_cursor,omitempty"`
	Report   string `json:"report,omitempty"`
}

// recordFrames are the frames package record draws, as its FrameStyle
// writes them.
var recordFrames = []string{"none", "mac", "mac-titleless", "mac-unified", "mac-compact", "mac-expanded", "mac-hidden"}

// validTourName reports whether name is safe to use as a tour identifier and
// default output filename stem.
func validTourName(name string) bool {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return false
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

// validBackdrop reports whether s is a backdrop the app reads: #RRGGBB or
// transparent.
func validBackdrop(s string) bool {
	if s == "transparent" {
		return true
	}
	if len(s) != 7 || s[0] != '#' {
		return false
	}
	_, err := strconv.ParseUint(s[1:], 16, 32)
	return err == nil
}

// recordReport is what the app reports it did.
type recordReport struct {
	V          int      `json:"v"`
	Tours      []string `json:"tours"`
	Frames     int      `json:"frames"`
	DurationMS int64    `json:"duration_ms"`
	Width      int      `json:"width"`
	Height     int      `json:"height"`
}

func tourName(name string) string {
	if name == "" {
		return "the tour"
	}
	return fmt.Sprintf("the tour %q", name)
}

// runRecorder runs the app built to record, its output on the terminal,
// until it exits, or until ctx is done, when it is asked to stop.
func runRecorder(ctx context.Context, exe, dir string, env []string) error {
	p, err := proc.Start(exe, nil, proc.Options{Dir: dir, Env: env, Stdout: os.Stdout, Stderr: ui.Stderr})
	if err != nil {
		return err
	}
	select {
	case <-p.Done():
	case <-ctx.Done():
		p.Stop(3 * time.Second)
		return ctx.Err()
	}
	if err := p.Err(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ExitCode() > 0 {
			// The app said why on its standard error, which is the
			// terminal's.
			return errFailed
		}
		return fmt.Errorf("the app %s", p.Exited())
	}
	return nil
}

// readRecordReport reads the report the app wrote; an app that wrote none
// exited before it reached ggui.Run.
func readRecordReport(path string, rep *recordReport) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return errors.New("the app exited without recording: main must reach ggui.Run, or call record.Main, " +
			"before it exits (see docs/recording.md)")
	}
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, rep); err != nil {
		return fmt.Errorf("the app's report: %w", err)
	}
	if rep.V != recordProtocol {
		return fmt.Errorf("the app reports in version %d of the protocol, this ggui reads %d: "+
			"use a ggui command of the same release as the module the app is built with", rep.V, recordProtocol)
	}
	return nil
}

// reportRecording says what was written: the file, its size, and the
// frames in it.
func reportRecording(path, format string, rep recordReport) error {
	rel := path
	if wd, err := os.Getwd(); err == nil {
		if r, err := filepath.Rel(wd, path); err == nil && filepath.IsLocal(r) {
			rel = r
		}
	}
	what := fmt.Sprintf("%d frames, %.1fs, %d×%d", rep.Frames, float64(rep.DurationMS)/1000, rep.Width, rep.Height)
	if format == "gif" {
		fi, err := os.Stat(path)
		if err != nil {
			return err
		}
		what = byteSize(fi.Size()) + ", " + what
	}
	ui.Stderr.Success("Wrote %s (%s)", ui.Stderr.Cyan(rel), what)
	_, err := fmt.Println(path)
	return err
}

// byteSize writes n bytes for a person: 57.3 KB, 2.1 MB.
func byteSize(n int64) string {
	switch {
	case n < 1000:
		return fmt.Sprintf("%d B", n)
	case n < 1000*1000:
		return fmt.Sprintf("%.1f KB", float64(n)/1000)
	}
	return fmt.Sprintf("%.1f MB", float64(n)/1000/1000)
}
