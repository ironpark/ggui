package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/ironpark/ggui/cmd/ggui/internal/gotool"
	"github.com/ironpark/ggui/cmd/ggui/internal/scaffold"
	"github.com/ironpark/ggui/cmd/ggui/internal/ui"
)

var initCmd = &command{
	name:    "init",
	args:    "[dir]",
	summary: "create a new application",
	help: `Init writes a new ggui application into dir, the current directory by
default, creating it if needed: go.mod, a small counter in main.go,
ggui.toml with every setting documented, and .gitignore. It then adds
the latest ggui to go.mod with go get and go mod tidy.

It refuses to overwrite any of those files unless given --force, and
then writes none of them.`,
	flags: func(fs *flag.FlagSet) func(context.Context, []string, []string) error {
		module := fs.String("module", "", "the module `path`, such as example.com/hello (default: the directory's name)")
		name := fs.String("name", "", "the app's `name`, as people see it (default: the directory's name)")
		replace := fs.String("replace", "", "use the ggui checkout in `dir` instead of a published version, through a replace directive")
		offline := fs.Bool("offline", false, "write the files only; run go get and go mod tidy yourself")
		force := fs.Bool("force", false, "overwrite files that exist")
		return func(ctx context.Context, pos, rest []string) error {
			if len(pos) > 1 || len(rest) > 0 {
				fmt.Fprintln(os.Stderr, "ggui init: takes one directory")
				return errUsage
			}
			dir := "."
			if len(pos) == 1 {
				dir = pos[0]
			}
			r, err := scaffold.Write(scaffold.Options{
				Dir: dir, Module: *module, Name: *name, Replace: *replace, Force: *force,
			})
			if err != nil {
				return err
			}
			log, out := ui.Stderr, ui.Stdout
			log.Success("Created %s (module %s) in %s", log.Bold(r.Name), log.Bold(r.Module), log.Cyan(shortPath(r.Dir)))
			log.Note("%s", log.Dim(strings.Join(r.Files, "  ")))

			cd := ""
			if wd, _ := os.Getwd(); wd != r.Dir {
				cd = shortPath(r.Dir)
			}
			if !*offline {
				if err := fetch(ctx, log, r); err != nil {
					finish(log, r, cd, err, ctx.Err() != nil)
					return errFailed
				}
				r.Fetched = true
			}

			// A go.work in a directory above belongs to another project
			// and does not list the new module, and the go command
			// refuses to build a module outside the workspace in effect.
			if env, err := gotool.Env(ctx, r.Dir, nil, "GOWORK"); err == nil {
				if w := env["GOWORK"]; w != "" && w != "off" {
					out.Printf("\n%s\n", out.Yellow(fmt.Sprintf("Note: %s is in effect here. Unless it lists this module, go and ggui will\n"+
						"refuse to build it: add the module (go work use %s), or set GOWORK=off.", w, r.Dir)))
				}
			}
			out.Printf("\n%s\n", out.Bold("Next:"))
			if cd != "" {
				out.Printf("  %s\n", out.Cyan("cd "+cd))
			}
			if !r.Fetched {
				out.Printf("  %s\n", out.Cyan(r.Finish()))
			}
			out.Printf("  %s     check this machine has what ggui apps need\n", out.Cyan("ggui doctor"))
			out.Printf("  %s        run the app, rebuilding and restarting it as you edit\n", out.Cyan("ggui dev"))
			out.Printf("  %s      bundle it: a .app on macOS, an .exe on Windows, a program elsewhere, --target web for a browser\n", out.Cyan("ggui build"))
			return nil
		}
	},
}

// stallTime is how long the go command may print nothing before init
// says that it is still waiting, so that a slow download does not read
// as a hang.
const stallTime = 20 * time.Second

// fetch runs the go commands that add ggui to the new module, each a step
// showing what the go command prints last.
func fetch(ctx context.Context, log *ui.Output, r *scaffold.Result) error {
	for _, args := range r.Commands() {
		cmdline := "go " + strings.Join(args, " ")
		title := "Resolving dependencies"
		if args[0] == "get" {
			title = "Fetching ggui"
		}
		hint := ui.StallHint(stallTime, "still waiting on the go command — a slow network, or GOPROXY?")
		var s *ui.Step
		if log.Terminal() {
			// The command shows as the detail until it prints something,
			// leaving the line's room to what it prints.
			s = log.Step(title, hint)
			s.Detail(cmdline)
		} else {
			s = log.Step(title+" ("+cmdline+")", hint)
		}
		err := scaffold.Go(ctx, r.Dir, s.Writer(), args...)
		switch {
		case err != nil && ctx.Err() != nil:
			s.Warn("Interrupted: " + cmdline)
			return ctx.Err()
		case err != nil:
			s.Fail(cmdline + " failed")
			// What the go command said, in full: the detail line showed
			// only the last of it.
			if out := strings.TrimRight(s.Output(), "\n"); out != "" {
				log.Println(log.Dim(indent(out, "    ")))
			}
			if errors.Is(err, gotool.ErrNoGo) || s.Output() == "" {
				log.Note("%v", err)
			}
			return err
		case args[0] == "get":
			s.Done(added(s.Output()))
		default:
			s.Done("Tidied go.mod and go.sum")
		}
	}
	return nil
}

// addedRE finds the version go get added in what it printed.
var addedRE = regexp.MustCompile(`go: (?:added|upgraded) ` + regexp.QuoteMeta(scaffold.GGUI) + ` (?:\S+ => )?(v\S+)`)

// added says what go get added, with the version when it said which.
func added(output string) string {
	if m := addedRE.FindStringSubmatch(output); m != nil {
		return "Added " + scaffold.GGUI + " " + m[1]
	}
	return "Added the latest " + scaffold.GGUI
}

// finish says, after go get or go mod tidy did not finish, that the files
// are written and what finishes the job.
func finish(log *ui.Output, r *scaffold.Result, cd string, err error, interrupted bool) {
	fmt.Fprintln(log)
	if interrupted {
		log.Println("The files are written. To finish, run:")
	} else {
		log.Println("The files are written. Fix the cause, then finish with:")
	}
	if cd != "" {
		log.Printf("  %s\n", log.Cyan("cd "+cd))
	}
	log.Printf("  %s\n", log.Cyan(r.Finish()))
	if !interrupted && !errors.Is(err, gotool.ErrNoGo) {
		log.Printf("Or skip this step next time with %s.\n", log.Cyan("ggui init --offline"))
	}
}

// shortPath names dir relative to the working directory when it is below
// it, as a person would type it to cd there.
func shortPath(dir string) string {
	wd, err := os.Getwd()
	if err != nil {
		return dir
	}
	rel, err := filepath.Rel(wd, dir)
	if err != nil || strings.HasPrefix(rel, "..") {
		return dir
	}
	return rel
}

// indent puts prefix before every line of s.
func indent(s, prefix string) string {
	return prefix + strings.ReplaceAll(s, "\n", "\n"+prefix)
}
