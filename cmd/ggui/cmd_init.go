package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ironpark/ggui/cmd/ggui/internal/gotool"
	"github.com/ironpark/ggui/cmd/ggui/internal/scaffold"
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
			r, err := scaffold.Init(ctx, scaffold.Options{
				Dir: dir, Module: *module, Name: *name, Replace: *replace,
				Offline: *offline, Force: *force, Log: os.Stderr,
			})
			if r == nil {
				return err
			}
			fmt.Printf("Created %s (module %s) in %s:\n  %s\n", r.Name, r.Module, r.Dir, strings.Join(r.Files, "  "))
			if err != nil {
				return err
			}
			// A go.work in a directory above belongs to another project
			// and does not list the new module, and the go command
			// refuses to build a module outside the workspace in effect.
			if env, err := gotool.Env(ctx, r.Dir, nil, "GOWORK"); err == nil {
				if w := env["GOWORK"]; w != "" && w != "off" {
					fmt.Printf("\nNote: %s is in effect here. Unless it lists this module, go and ggui will\n"+
						"refuse to build it: add the module (go work use %s), or set GOWORK=off.\n", w, r.Dir)
				}
			}
			fmt.Println("\nNext:")
			if wd, _ := os.Getwd(); wd != r.Dir {
				rel, err := filepath.Rel(wd, r.Dir)
				if err != nil || strings.HasPrefix(rel, "..") {
					rel = r.Dir
				}
				fmt.Printf("  cd %s\n", rel)
			}
			switch {
			case r.Fetched:
			case *replace != "":
				fmt.Println("  go mod tidy")
			default:
				fmt.Printf("  go get %s@latest && go mod tidy\n", scaffold.GGUI)
			}
			fmt.Println("  ggui doctor     check this machine has what ggui apps need")
			fmt.Println("  ggui dev        run the app, rebuilding and restarting it as you edit")
			fmt.Println("  ggui build      bundle it: a .app on macOS, a program elsewhere, --target web for a browser")
			return nil
		}
	},
}
