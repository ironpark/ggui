package record

import (
	"encoding/json"
	"fmt"
	"image/color"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/ironpark/ggui/internal/kernel"
)

// Env is the environment variable ggui record starts the app with. Its
// value is a request, JSON, for a tour to be drawn into a file or for the
// tours' names; ggui.Run, and Main, carry it out and exit instead of
// running the app.
const Env = "GGUI_RECORD"

// protocol is the version of the request and the report; ggui record
// and this package each check the other's.
const protocol = 1

// request is what Env holds. ggui record writes it; the two modules
// share no code, so a test on each side checks one golden string.
type request struct {
	V      int     `json:"v"`
	Tour   string  `json:"tour"`
	Format string  `json:"format"` // "gif", "png" (frames in the directory Out) or "list"
	Out    string  `json:"out,omitempty"`
	FPS    int     `json:"fps,omitempty"`
	Width  int     `json:"width,omitempty"`
	Colors int     `json:"colors,omitempty"`
	Dither float64 `json:"dither,omitempty"`
	Loop   int     `json:"loop,omitempty"`
	Lossy  int     `json:"lossy,omitempty"`
	// Frame is a FrameStyle as its String writes it ("mac",
	// "mac-unified", "none" and the rest), in place of the tour's Frame;
	// "" keeps it. Backdrop is "#RRGGBB" or "transparent". NoCursor leaves the
	// cursor out.
	Frame    string `json:"frame,omitempty"`
	Backdrop string `json:"backdrop,omitempty"`
	NoCursor bool   `json:"no_cursor,omitempty"`
	// Report is the file the report is written to, when set.
	Report string `json:"report,omitempty"`
}

// report is what a request did: the tours' names for "list", what was
// written otherwise.
type report struct {
	V          int      `json:"v"`
	Tours      []string `json:"tours"`
	Frames     int      `json:"frames"`
	DurationMS int64    `json:"duration_ms"`
	Width      int      `json:"width"`
	Height     int      `json:"height"`
}

func init() { kernel.Record = serve }

// Main draws the tour Env asks for and exits, when the process was started
// by ggui record, and returns at once otherwise. ggui.Run does the same
// before it opens a window; a program that does work before it calls Run
// that a recording should skip, as opening a database or a network
// connection, calls Main first:
//
//	func main() {
//		record.Main()
//		db := openDatabase()
//		...
//	}
//
// Call it from the main goroutine.
func Main() {
	if req, ok := os.LookupEnv(Env); ok {
		os.Exit(serve(req))
	}
}

// serve carries out req and returns the status the process exits with: 0
// when it was done, 1 when drawing or writing failed, 2 when req cannot be
// carried out, as for a tour the program does not have. What went wrong
// goes to standard error.
func serve(req string) int {
	// The app's own children are not asked to record.
	os.Unsetenv(Env)
	code, err := serveTo(req, os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	return code
}

func serveTo(raw string, stdout io.Writer) (int, error) {
	req, err := parseRequest(raw)
	if err != nil {
		return 2, err
	}
	rep := report{V: protocol}
	if req.Format == "list" {
		return listTours(req, stdout, rep)
	}
	t, ok := Lookup(req.Tour)
	if !ok {
		return 2, noTour(req.Tour)
	}
	if req.Out == "" {
		return 2, fmt.Errorf("record: %s names no file to write", Env)
	}
	o, err := optionsFor(req)
	if err != nil {
		return 2, err
	}
	if req.Format != "gif" && req.Format != "png" {
		return 2, fmt.Errorf("record: %s asks for format %q; this program writes gif, png or list", Env, req.Format)
	}
	res, err := writeTour(t, req, o)
	if err != nil {
		return 1, err
	}
	rep.Frames, rep.DurationMS = res.frames, res.duration.Milliseconds()
	rep.Width, rep.Height = res.w, res.h
	if req.Report == "" {
		return 0, nil
	}
	return writeReport(req.Report, rep)
}

// parseRequest decodes raw and checks its protocol version.
func parseRequest(raw string) (request, error) {
	var req request
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		return req, fmt.Errorf("record: %s is not a request: %w", Env, err)
	}
	if req.V != protocol {
		return req, fmt.Errorf("record: %s is a request of version %d, but this program's ggui reads version %d: "+
			"use a ggui command of the same release as the module the app is built with", Env, req.V, protocol)
	}
	return req, nil
}

// listTours writes the tour names, or a report of them.
func listTours(req request, stdout io.Writer, rep report) (int, error) {
	rep.Tours = Names()
	if req.Report == "" {
		for _, n := range rep.Tours {
			fmt.Fprintln(stdout, n)
		}
		return 0, nil
	}
	return writeReport(req.Report, rep)
}

// optionsFor turns a request into resolved recording options.
func optionsFor(req request) (*settings, error) {
	o := resolve(nil)
	o.fps, o.width = max(req.FPS, 0), max(req.Width, 0)
	o.dither, o.loop = min(max(req.Dither, 0), 1), max(req.Loop, 0)
	if req.Colors != 0 {
		Colors(req.Colors)(o)
	}
	Lossy(req.Lossy)(o)
	o.noCursor = req.NoCursor
	if req.Frame != "" {
		f, err := parseFrame(req.Frame)
		if err != nil {
			return nil, err
		}
		o.frame = &f
	}
	if req.Backdrop != "" {
		c, err := parseBackdrop(req.Backdrop)
		if err != nil {
			return nil, err
		}
		o.backdrop = c
	}
	return o, nil
}

// writeTour draws req's tour in the format it asks for.
func writeTour(t Tour, req request, o *settings) (result, error) {
	switch req.Format {
	case "gif":
		return t.writeGIF(req.Out, o)
	case "png":
		return t.writePNGs(req.Out, o)
	default:
		return result{}, fmt.Errorf("record: %s asks for format %q; this program writes gif, png or list", Env, req.Format)
	}
}

// noTour says that there is no tour called name, and which there are.
func noTour(name string) error {
	names := Names()
	if len(names) == 0 {
		return fmt.Errorf("record: this program registered no tours: call record.Register before ggui.Run")
	}
	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = strconv.Quote(n)
		if n == "" {
			quoted[i] += " (the default)"
		}
	}
	what := "no default tour"
	if name != "" {
		what = fmt.Sprintf("no tour named %q", name)
	}
	return fmt.Errorf("record: this program has %s; it has %s", what, strings.Join(quoted, ", "))
}

func writeReport(path string, rep report) (int, error) {
	data, err := json.Marshal(rep)
	if err != nil {
		return 1, err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return 1, fmt.Errorf("record: writing the report: %w", err)
	}
	return 0, nil
}

// parseBackdrop reads a backdrop as ggui record's --backdrop gives it:
// "#RRGGBB", or "transparent".
func parseBackdrop(s string) (color.Color, error) {
	if s == "transparent" {
		return color.Transparent, nil
	}
	v, err := strconv.ParseUint(strings.TrimPrefix(s, "#"), 16, 32)
	if err != nil || len(s) != 7 || s[0] != '#' {
		return nil, fmt.Errorf("record: the backdrop %q is not #RRGGBB or transparent", s)
	}
	return color.NRGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 0xff}, nil
}
