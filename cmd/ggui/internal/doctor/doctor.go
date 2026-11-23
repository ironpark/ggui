// Package doctor checks that a machine and a project have what building
// and running a ggui application takes, and says what to do about what is
// missing.
//
// What it looks for on each operating system is what ggwd and ggfx load
// at run time. They are cgo-free: they open the system's libraries with
// purego (dlopen on Unix, LoadLibrary on Windows) when the program starts,
// so a missing library is not a build error but a program that fails to
// open its window. The lists here follow their sources; see platform.go.
package doctor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/ironpark/ggui/cmd/ggui/internal/ui"
)

// Status is a check's verdict.
type Status string

const (
	OK   Status = "ok"
	Warn Status = "warn" // works, but something is missing or worth knowing
	Fail Status = "fail" // building or running will not work until fixed
)

// Check is one line of the report.
type Check struct {
	Name   string `json:"name"`
	Status Status `json:"status"`
	Detail string `json:"detail"`
	// Fix says what to do, for a check that is not ok.
	Fix string `json:"fix,omitempty"`
}

// Report is every check made.
type Report struct {
	Checks []Check `json:"checks"`
}

func (r *Report) add(name string, s Status, detail, fix string) {
	r.Checks = append(r.Checks, Check{Name: name, Status: s, Detail: detail, Fix: fix})
}

func (r *Report) ok(name, detail string)        { r.add(name, OK, detail, "") }
func (r *Report) warn(name, detail, fix string) { r.add(name, Warn, detail, fix) }
func (r *Report) fail(name, detail, fix string) { r.add(name, Fail, detail, fix) }

// Failed reports whether any check failed.
func (r *Report) Failed() bool {
	for _, c := range r.Checks {
		if c.Status == Fail {
			return true
		}
	}
	return false
}

// Options says where to look.
type Options struct {
	Dir    string // the directory to check as a project
	Config string // an explicit ggui.toml, or ""
}

// Run makes every check.
func Run(ctx context.Context, o Options) *Report {
	r := &Report{}
	env, ok := checkGo(ctx, r, o)
	if !ok {
		return r // nothing else can be asked of a missing go command
	}
	// The Go version is judged against the ggui the project resolves, but
	// reads best first.
	pr := &Report{}
	proj := checkProject(ctx, pr, o, env)
	checkGoVersion(r, env, proj)
	checkTarget(r, env)
	r.Checks = append(r.Checks, pr.Checks...)
	checkRemote(ctx, r, o, proj)
	cfg := checkConfig(r, o)
	if cfg != nil {
		checkTranslations(r, cfg.Root, cfg.Config.I18n)
	}
	checkPlatform(ctx, r, env)
	checkTools(ctx, r)
	checkSigning(ctx, r, cfg)
	return r
}

// Print writes the report for a person: a line per check, its fix
// indented below it. On a terminal the status is colored and marked ✓, !
// or ✗.
func (r *Report) Print(w io.Writer) {
	o := ui.For(w)
	statusWidth := 4
	status := func(s Status) string { return fmt.Sprintf("%-4s", s) }
	if o.Terminal() {
		statusWidth = 6
		status = func(s Status) string {
			text := fmt.Sprintf("%-4s", s)
			switch s {
			case OK:
				return o.Green("✓ " + text)
			case Warn:
				return o.Yellow("! " + text)
			default:
				return o.Red("✗ " + text)
			}
		}
	}
	width := 0
	for _, c := range r.Checks {
		width = max(width, len(c.Name))
	}
	for _, c := range r.Checks {
		// A detail of several lines, such as a list of errors, keeps to
		// its column.
		detail := strings.ReplaceAll(c.Detail, "\n", "\n"+strings.Repeat(" ", statusWidth+2+width+2))
		fmt.Fprintf(w, "%s  %-*s  %s\n", status(c.Status), width, c.Name, detail)
		if c.Fix != "" {
			for line := range strings.SplitSeq(c.Fix, "\n") {
				fmt.Fprintf(w, "%-*s  %-*s  %s %s\n", statusWidth, "", width, "", o.Cyan("→"), line)
			}
		}
	}
	var fails, warns int
	for _, c := range r.Checks {
		switch c.Status {
		case Fail:
			fails++
		case Warn:
			warns++
		}
	}
	fmt.Fprintln(w)
	switch {
	case fails > 0:
		fmt.Fprintln(w, o.Red(fmt.Sprintf("%d failed, %d warnings.", fails, warns)))
	case warns > 0:
		fmt.Fprintln(w, o.Yellow(fmt.Sprintf("No problems that stop a build; %d warnings.", warns)))
	default:
		fmt.Fprintln(w, o.Green("Everything checked is in order."))
	}
}

// PrintJSON writes the report for a program.
func (r *Report) PrintJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(struct {
		OK     bool    `json:"ok"`
		Checks []Check `json:"checks"`
	}{!r.Failed(), r.Checks})
}
