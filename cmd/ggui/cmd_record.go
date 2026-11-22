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
		cfgPath := fs.String("config", "", "read `file` instead of the ggui.toml found from the current directory")
		pkg := fs.String("package", "", "the main `package` to build, replacing app.package")
		tags := fs.String("tags", "", "build `tags`, comma-separated, replacing build.tags")
		out := fs.String("o", "", "write the GIF to `file` instead of name.gif in the project's directory")
		frames := fs.String("frames", "", "write PNG frames, 0000.png and on, to `directory` instead of a GIF")
		list := fs.Bool("list", false, "print the names of the tours the app registered")
		fps := fs.Int("fps", 0, "keep `n` frames a second; 0 keeps every frame the tour draws")
		width := fs.Int("width", 0, "scale the frames down to `px` pixels wide; 0 keeps the size drawn")
		colors := fs.Int("colors", 256, "the GIF palette's `n` colors, 2 to 256")
		dither := fs.Float64("dither", 0, "ordered dithering's `strength`, 0 (none) to 1; 0.3 smooths gradients that band")
		loop := fs.Int("loop", 0, "play the GIF `n` times; 0 loops for ever")
		lossy := fs.Int("lossy", 0, "let the GIF's compression write colors up to `n`/1000 off in Oklab, 0 (exact) to 200; 20 to 40 is hard to see")
		frame := fs.String("frame", "", "draw the frames in a `style` of window frame, in place of the tour's: "+strings.Join(recordFrames, ", "))
		backdrop := fs.String("backdrop", "", "the `color` a framed window is drawn on, #RRGGBB or transparent")
		cursor := fs.Bool("cursor", true, "draw the cursor the tour's pointer steps move")
		return func(ctx context.Context, pos, rest []string) error {
			if len(pos) > 1 || len(rest) > 0 {
				fmt.Fprintln(os.Stderr, "ggui record: takes one tour name")
				return errUsage
			}
			name := ""
			if len(pos) == 1 {
				name = pos[0]
			}
			switch {
			case len(pos) == 1 && !validTourName(name):
				return fmt.Errorf("tour name %q must not be empty, a dot name, or contain path separators or \"..\"", name)
			case *out != "" && *frames != "":
				return errors.New("-o writes a GIF and --frames PNG frames: give one")
			case *colors < 2 || *colors > 256:
				return fmt.Errorf("--colors %d: a GIF has 2 to 256 colors", *colors)
			case *dither < 0 || *dither > 1:
				return fmt.Errorf("--dither %g: the strength is 0 to 1", *dither)
			case *fps < 0 || *width < 0 || *loop < 0:
				return errors.New("--fps, --width and --loop are not negative")
			case *lossy < 0 || *lossy > 200:
				return fmt.Errorf("--lossy %d: it is 0 to 200", *lossy)
			case *frame != "" && !slices.Contains(recordFrames, *frame):
				return fmt.Errorf("--frame %s: the frames are %s", *frame, strings.Join(recordFrames, ", "))
			case *backdrop != "" && !validBackdrop(*backdrop):
				return fmt.Errorf("--backdrop %s: give #RRGGBB or transparent", *backdrop)
			}
			proj, err := project.Open(".", *pkg, *cfgPath)
			if err != nil {
				return err
			}
			cfg := proj.Config
			buildTags := cfg.Build.Tags
			if isSet(fs, "tags") {
				buildTags = splitTags(*tags)
			}
			say := ui.Stderr.Prefixed("ggui")
			main, err := resolveMain(ctx, say, proj, *pkg, gotool.ListOptions{Tags: buildTags})
			if err != nil {
				return err
			}

			tmp, err := os.MkdirTemp("", "ggui-record-")
			if err != nil {
				return err
			}
			defer os.RemoveAll(tmp)
			exe := filepath.Join(tmp, main.Executable)
			if runtime.GOOS == "windows" {
				exe += ".exe"
			}
			bo := gotool.BuildOptions{Dir: proj.Root, Package: main.Package, Output: exe, Tags: buildTags, TmpDir: tmp}
			ui.Stderr.Println(ui.Stderr.Dim(bo.String()))
			if err := step(ctx, "Building "+main.Name, "Built "+main.Name, func() error { return gotool.Build(ctx, bo) }); err != nil {
				return err
			}

			req := recordRequest{V: recordProtocol, Tour: name, Report: filepath.Join(tmp, "report.json")}
			switch {
			case *list:
				req.Format = "list"
			case *frames != "":
				req.Format, req.Out = "png", *frames
			default:
				req.Format, req.Out = "gif", *out
				if req.Out == "" {
					base := name
					if base == "" {
						base = "tour"
					}
					req.Out = filepath.Join(proj.Root, base+".gif")
				}
			}
			if req.Out != "" {
				if req.Out, err = filepath.Abs(req.Out); err != nil {
					return err
				}
			}
			if req.Format != "list" {
				req.FPS, req.Width, req.Loop, req.Dither = *fps, *width, *loop, *dither
				req.Lossy = *lossy
				if *colors != 256 {
					req.Colors = *colors
				}
				req.Frame, req.Backdrop, req.NoCursor = *frame, *backdrop, !*cursor
			}
			data, err := json.Marshal(req)
			if err != nil {
				return err
			}
			env := append(os.Environ(), proc.Environ(cfg.Dev.Env)...)
			env = append(env, recordEnv+"="+string(data))

			title, done := "Recording "+tourName(name), "Recorded "+tourName(name)
			if *list {
				title, done = "Listing the tours", "Listed the tours"
			}
			var rep recordReport
			err = step(ctx, title, done, func() error {
				if err := runRecorder(ctx, exe, proj.Root, env); err != nil {
					return err
				}
				return readRecordReport(req.Report, &rep)
			})
			if err != nil {
				return err
			}
			if *list {
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
	},
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
