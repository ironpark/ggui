// Package ui is how the ggui command talks to a person: a few styles and
// symbols, and steps that show a spinner while they work.
//
// Everything is decided per writer. One that is not a terminal gets plain
// text, a line per event, without carriage returns or escape sequences,
// as a log, a pipe or a test wants. A terminal gets color, unless NO_COLOR
// is set, and steps that redraw a line of their own about twelve times a
// second at the bottom of the screen: the live region. Whatever else is
// written through an Output while steps are live is printed above them,
// so that the two never share a line.
package ui

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// Output writes to one writer, styled as that writer can show.
type Output struct {
	w     io.Writer
	color bool
	// live is a terminal that takes carriage returns and escape
	// sequences: steps animate on it, and writes keep below the region.
	live bool
	// ascii is a console that cannot be trusted with ✓ and the like.
	ascii  bool
	width  func() int
	prefix string
}

// Stdout and Stderr are the program's standard output and error. Use them,
// or For, rather than New on os.Stdout or os.Stderr, so that every write
// to the terminal goes through the same place.
var (
	Stdout = New(os.Stdout)
	Stderr = New(os.Stderr)
)

// For returns the Output for w: Stdout or Stderr for the program's own,
// w itself when it is an Output, and a new one otherwise.
func For(w io.Writer) *Output {
	switch w {
	case io.Writer(os.Stdout):
		return Stdout
	case io.Writer(os.Stderr):
		return Stderr
	}
	if o, ok := w.(*Output); ok {
		return o
	}
	return New(w)
}

// New returns an Output for w, deciding from w and the environment
// whether it is a terminal and whether to color.
func New(w io.Writer) *Output { return newOutput(w, os.Getenv) }

func newOutput(w io.Writer, getenv func(string) string) *Output {
	o := &Output{w: w, width: func() int { return 80 }}
	terminal, vt := false, false
	if f, ok := w.(*os.File); ok && f != nil {
		fd := f.Fd()
		terminal = isTerminal(fd)
		vt = terminal && enableVT(fd)
		o.width = func() int {
			if n := termWidth(fd); n > 0 {
				return n
			}
			return 80
		}
	}
	o.ascii = terminal && !vt
	o.live = vt && getenv("TERM") != "dumb"
	o.color = colorOn(vt, getenv)
	return o
}

// colorOn decides color for a writer, terminal or not: NO_COLOR turns it
// off, then CLICOLOR_FORCE or FORCE_COLOR on, then TERM=dumb off.
func colorOn(terminal bool, getenv func(string) string) bool {
	if getenv("NO_COLOR") != "" {
		return false
	}
	if forced(getenv("CLICOLOR_FORCE")) || forced(getenv("FORCE_COLOR")) {
		return true
	}
	if getenv("TERM") == "dumb" {
		return false
	}
	return terminal
}

func forced(v string) bool { return v != "" && v != "0" && v != "false" }

// Prefixed returns an Output on the same writer whose messages begin with
// p, the program's name: plain, "p: message", as a log line; on a
// terminal, p dimmed before the message's symbol. It is for output
// interleaved with another program's, as ggui dev's is with the app's.
func (o *Output) Prefixed(p string) *Output {
	c := *o
	c.prefix = p
	return &c
}

// Terminal reports whether o animates: whether it is a terminal that takes
// escape sequences.
func (o *Output) Terminal() bool { return o.live }

// Color reports whether o colors what it writes.
func (o *Output) Color() bool { return o.color }

func (o *Output) style(code, s string) string {
	if !o.color || s == "" {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

// Bold, Dim, Red, Green, Yellow and Cyan return s styled for o: unchanged
// when o does not color.
func (o *Output) Bold(s string) string   { return o.style("1", s) }
func (o *Output) Dim(s string) string    { return o.style("2", s) }
func (o *Output) Red(s string) string    { return o.style("31", s) }
func (o *Output) Green(s string) string  { return o.style("32", s) }
func (o *Output) Yellow(s string) string { return o.style("33", s) }
func (o *Output) Cyan(s string) string   { return o.style("36", s) }

// kind is what a line says, which its symbol shows.
type kind int

const (
	plain kind = iota // indented, under the line before
	info
	success
	warning
	failure
)

func (o *Output) symbol(k kind) string {
	switch k {
	case info:
		return o.Dim(o.pick("•", "-"))
	case success:
		return o.Green(o.pick("✓", "+"))
	case warning:
		return o.Yellow("!")
	case failure:
		return o.Red(o.pick("✗", "x"))
	}
	return " "
}

func (o *Output) pick(unicode, ascii string) string {
	if o.ascii {
		return ascii
	}
	return unicode
}

// line formats a message of kind k.
func (o *Output) line(k kind, text string) string {
	switch {
	case o.prefix != "" && !o.live:
		return o.prefix + ": " + text
	case o.prefix != "":
		return o.Dim(o.prefix) + " " + o.symbol(k) + " " + text
	default:
		return o.symbol(k) + " " + text
	}
}

// Info, Success, Warn and Error print a line marked •, ✓, ! or ✗.
func (o *Output) Info(format string, args ...any)    { o.say(info, format, args) }
func (o *Output) Success(format string, args ...any) { o.say(success, format, args) }
func (o *Output) Warn(format string, args ...any)    { o.say(warning, format, args) }
func (o *Output) Error(format string, args ...any)   { o.say(failure, format, args) }

// Note prints a line indented to the text of the lines above, unmarked.
func (o *Output) Note(format string, args ...any) { o.say(plain, format, args) }

func (o *Output) say(k kind, format string, args []any) {
	mu.Lock()
	defer mu.Unlock()
	o.printLocked(o.line(k, fmt.Sprintf(format, args...)))
}

// Println writes its operands as fmt.Println does, through o.
func (o *Output) Println(a ...any) { o.Write(fmt.Appendln(nil, a...)) }

// Printf writes as fmt.Printf does, through o.
func (o *Output) Printf(format string, args ...any) { o.Write(fmt.Appendf(nil, format, args...)) }

// Write writes p as it is, above the live region when steps are live on
// the same terminal. A write that leaves a line unfinished holds the
// region off the screen until a later write finishes it.
func (o *Output) Write(p []byte) (int, error) {
	mu.Lock()
	defer mu.Unlock()
	return o.writeLocked(p)
}

// The live region: the steps that animate, drawn at the bottom of one
// terminal, and the state of the screen. One lock covers it and every
// write through an Output, so that a frame never lands inside a line.
var (
	mu       sync.Mutex
	steps    []*Step // every step running, live or not
	region   []*Step // the live ones, in the order they started
	regionTo *Output // where the region is drawn; nil when nothing is
	drawn    int     // lines of the region on the screen now
	held     bool    // a line was left unfinished above the region
	ticking  bool
	frame    int

	// now and autoTick let tests take the clock.
	now      = time.Now
	autoTick = true
)

// frameInterval is the time between frames, about 12 a second.
const frameInterval = 80 * time.Millisecond

func (o *Output) writeLocked(p []byte) (int, error) {
	if !o.live || regionTo == nil {
		return o.w.Write(p)
	}
	to := regionTo
	clear := clearSeq()
	if len(p) > 0 {
		held = p[len(p)-1] != '\n'
	}
	draw := drawSeq()
	if len(region) == 0 {
		regionTo = nil // erased, and nothing left to draw
	}
	if to.w == o.w {
		b := append(append(clear, p...), draw...)
		if _, err := o.w.Write(b); err != nil {
			return 0, err
		}
		return len(p), nil
	}
	to.w.Write(clear)
	n, err := o.w.Write(p)
	to.w.Write(draw)
	return n, err
}

// printLocked writes a line of ggui's own, on a line of its own.
func (o *Output) printLocked(line string) {
	if held && o.live && regionTo != nil {
		line = "\n" + line
	}
	o.writeLocked([]byte(line + "\n"))
}

// clearSeq returns what erases the region from the screen, leaving the
// cursor where its first line began.
func clearSeq() []byte {
	if drawn == 0 {
		return nil
	}
	b := []byte("\r\x1b[K")
	for i := 1; i < drawn; i++ {
		b = append(b, "\x1b[A\x1b[K"...)
	}
	drawn = 0
	return b
}

// drawSeq returns the region's lines, each cut to fit the terminal so
// that none wraps, with the cursor left at the end of the last.
func drawSeq() []byte {
	if held || len(region) == 0 || regionTo == nil {
		return nil
	}
	width := regionTo.width()
	t := now()
	var lines []string
	for _, s := range region {
		lines = append(lines, s.render(t, width)...)
	}
	drawn = len(lines)
	return []byte(strings.Join(lines, "\n"))
}

// refreshLocked draws the region again, a new frame.
func refreshLocked() {
	if regionTo != nil {
		regionTo.writeLocked(nil)
	}
}

func tickLoop() {
	t := time.NewTicker(frameInterval)
	defer t.Stop()
	for range t.C {
		mu.Lock()
		if len(steps) == 0 {
			ticking = false
			mu.Unlock()
			return
		}
		tickLocked(now())
		mu.Unlock()
	}
}

// tickLocked advances the spinners and says, under a step that is not
// live, that it has gone quiet for long.
func tickLocked(t time.Time) {
	frame++
	for _, s := range steps {
		if !s.animated && s.stall > 0 && !s.stalled && t.Sub(s.heard) >= s.stall {
			s.stalled = true
			s.o.printLocked(s.o.line(plain, s.hint))
		}
	}
	refreshLocked()
}

// Reset ends every step still running, erasing the live region, as the
// program exits.
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	for _, s := range steps {
		s.ended = true
	}
	steps, region = nil, nil
	refreshLocked()
	regionTo = nil
}
