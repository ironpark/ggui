package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/ironpark/ggui/cmd/ggui/internal/doctor"
	"github.com/ironpark/ggui/cmd/ggui/internal/ui"
)

// errFailed reports that doctor found a failure, which it has printed.
var errFailed = errors.New("a check failed")

var doctorCmd = &command{
	name:    "doctor",
	args:    "",
	summary: "check this machine and project for what ggui needs",
	help: `Doctor checks what building and running a ggui application takes and
prints a line for each check: ok, warn (works, but something is missing
or worth knowing) or fail (will not work until fixed), with what to do.
It exits with status 1 when a check failed.

It checks the Go toolchain against the version ggui requires, the
target platform, the project in the current directory (the ggui, ggfx
and ggwd versions it resolves, and whether ggfx and ggwd are the ones
that ggui asks for), ggui.toml and its icon, the libraries and programs
ggwd and ggfx load on this operating system, and the tools ggui build
uses.`,
	flags: func(fs *flag.FlagSet) func(context.Context, []string, []string) error {
		cfgPath := fs.String("config", "", "check `file` instead of the ggui.toml found from the current directory")
		asJSON := fs.Bool("json", false, `print the report as JSON: {"ok": bool, "checks": [{"name", "status", "detail", "fix"}]}`)
		return func(ctx context.Context, pos, rest []string) error {
			if len(pos) > 0 || len(rest) > 0 {
				fmt.Fprintln(os.Stderr, "ggui doctor: takes no arguments")
				return errUsage
			}
			// The checks run the go command, which may have modules to
			// download first.
			s := ui.Stderr.Step("Checking", ui.QuietStart())
			r := doctor.Run(ctx, doctor.Options{Dir: ".", Config: *cfgPath})
			s.Stop()
			if *asJSON {
				if err := r.PrintJSON(os.Stdout); err != nil {
					return err
				}
			} else {
				r.Print(os.Stdout)
			}
			if r.Failed() {
				return errFailed
			}
			return nil
		}
	},
}
