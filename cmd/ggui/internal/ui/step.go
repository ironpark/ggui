package ui

import (
	"bytes"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"
)

// Step is a piece of work that takes a while: on a terminal a line with a
// spinner, its title, the time it has taken and a dimmed detail, such as
// the last line a command printed, until it ends in a line marked ✓, ! or
// ✗. Elsewhere it prints its title when it starts and the result when it
// ends.
type Step struct {
	o      *Output
	title  string
	start  time.Time
	detail string
	heard  time.Time // when the detail last changed

	stall   time.Duration
	hint    string
	stalled bool // the hint was printed, where nothing animates

	animated              bool
	noSpin, quiet, noTime bool
	ended                 bool

	out     bytes.Buffer // what Writer was given
	partial []byte       // its last line, unfinished
}

// StepOption changes how a step shows.
type StepOption func(*Step)

// NoSpin keeps a step from animating, for when another program writes to
// the same terminal unseen: it prints its title when it starts, as on a
// writer that is not a terminal.
func NoSpin() StepOption { return func(s *Step) { s.noSpin = true } }

// QuietStart has a step that does not animate print nothing when it
// starts, only its result.
func QuietStart() StepOption { return func(s *Step) { s.quiet = true } }

// NoElapsed leaves the time taken off the result's line, for a result
// that says it already.
func NoElapsed() StepOption { return func(s *Step) { s.noTime = true } }

// StallHint shows hint in place of the detail once the step has gone
// after without new detail, so that a slow wait does not read as a hang.
// Where nothing animates it is printed once, as a line of its own.
func StallHint(after time.Duration, hint string) StepOption {
	return func(s *Step) { s.stall, s.hint = after, hint }
}

// Step starts a step called title.
func (o *Output) Step(title string, opts ...StepOption) *Step {
	s := &Step{o: o, title: title}
	for _, opt := range opts {
		opt(s)
	}
	mu.Lock()
	defer mu.Unlock()
	s.start = now()
	s.heard = s.start
	steps = append(steps, s)
	// One region, on one terminal: a step that would start a second
	// shows as a writer that is not a terminal would.
	if o.live && !s.noSpin && (regionTo == nil || regionTo.w == o.w) {
		s.animated = true
		region = append(region, s)
		if regionTo == nil {
			regionTo = o
		}
		refreshLocked()
	} else if !s.quiet {
		o.printLocked(o.line(info, title))
	}
	if !ticking && autoTick {
		ticking = true
		go tickLoop()
	}
	return s
}

// Detail sets the dimmed text after the title, such as what a command
// printed last.
func (s *Step) Detail(text string) {
	mu.Lock()
	defer mu.Unlock()
	s.setDetail(text)
}

func (s *Step) setDetail(text string) {
	s.detail = sanitize(text)
	s.heard = now()
	s.stalled = false
}

// Writer returns a writer whose lines become the step's detail as they
// come, and which keeps all it is given for Output: a command's output.
func (s *Step) Writer() io.Writer { return stepWriter{s} }

type stepWriter struct{ s *Step }

func (w stepWriter) Write(p []byte) (int, error) {
	mu.Lock()
	defer mu.Unlock()
	s := w.s
	s.out.Write(p)
	s.partial = append(s.partial, p...)
	last := ""
	for {
		i := bytes.IndexAny(s.partial, "\r\n")
		if i < 0 {
			break
		}
		if l := strings.TrimSpace(string(s.partial[:i])); l != "" {
			last = l
		}
		s.partial = s.partial[i+1:]
	}
	if len(s.partial) > 4096 {
		s.partial = s.partial[len(s.partial)-4096:]
	}
	if l := strings.TrimSpace(string(s.partial)); l != "" {
		last = l
	}
	if last != "" {
		s.setDetail(last)
	}
	return len(p), nil
}

// Output returns all the step's Writer was given.
func (s *Step) Output() string {
	mu.Lock()
	defer mu.Unlock()
	return s.out.String()
}

// Elapsed returns the time since the step started.
func (s *Step) Elapsed() time.Duration {
	mu.Lock()
	defer mu.Unlock()
	return now().Sub(s.start)
}

// Done, Warn, Fail, Stop and Static do nothing on a nil Step, so that
// one that may not have started can be ended all the same.

// Done ends the step with ✓ and text, its title when text is empty, and
// the time it took.
func (s *Step) Done(text string) { s.end(success, text) }

// Warn ends the step with ! and text.
func (s *Step) Warn(text string) { s.end(warning, text) }

// Fail ends the step with ✗ and text.
func (s *Step) Fail(text string) { s.end(failure, text) }

// Stop ends the step without a word, erasing its line: the caller says
// what happened, or nothing needs saying.
func (s *Step) Stop() { s.end(plain, "") }

func (s *Step) end(k kind, text string) {
	if s == nil {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	if s.ended {
		return
	}
	s.ended = true
	steps = slices.DeleteFunc(steps, func(x *Step) bool { return x == s })
	if s.animated {
		region = slices.DeleteFunc(region, func(x *Step) bool { return x == s })
	}
	if k == plain {
		if s.animated {
			refreshLocked()
		}
		return
	}
	if text == "" {
		text = s.title
	}
	if d := now().Sub(s.start); !s.noTime && d >= 100*time.Millisecond {
		text += "  " + s.o.Dim(formatElapsed(d))
	}
	// A live step's result takes the place of its line: the write erases
	// the region and draws what is left of it below.
	s.o.printLocked(s.o.line(k, text))
}

// Static stops the step animating, for when another program is about to
// write to the terminal unseen: its title is printed as a line, and the
// step ends as a step that never animated does.
func (s *Step) Static() {
	if s == nil {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	if s.ended || !s.animated {
		return
	}
	s.animated = false
	region = slices.DeleteFunc(region, func(x *Step) bool { return x == s })
	s.o.printLocked(s.o.line(info, s.title))
}

// spinner is the frames of a step's spinner.
var (
	spinner      = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	asciiSpinner = []string{"|", "/", "-", `\`}
)

// render returns the step's lines at t, each at most width-1 columns
// wide: its own, and below it the stall hint once it has gone quiet.
func (s *Step) render(t time.Time, width int) []string {
	o := s.o
	frames := spinner
	if o.ascii {
		frames = asciiSpinner
	}
	lead := ""
	if o.prefix != "" {
		lead = o.Dim(o.prefix) + " "
	}
	head := lead + o.Cyan(frames[frame%len(frames)]) + " " + s.title
	if d := t.Sub(s.start); d >= time.Second {
		head += "  " + o.Dim(formatElapsed(d))
	}
	limit := width - 1
	head = truncate(head, limit)
	if s.detail != "" {
		if room := limit - visibleWidth(head) - 2; room >= 12 {
			head += "  " + o.Dim(truncate(s.detail, room))
		}
	}
	if s.stall > 0 && t.Sub(s.heard) >= s.stall {
		hint := lead + "  " + o.Yellow(s.hint)
		return []string{head, truncate(hint, limit)}
	}
	return []string{head}
}

// formatElapsed formats a duration as a person reads it: 2.3s, 1m05s.
func formatElapsed(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return fmt.Sprintf("%dm%02ds", int(d/time.Minute), int(d%time.Minute/time.Second))
}
