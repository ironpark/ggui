package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ironpark/ggui/cmd/ggui/internal/gotool"
	"github.com/ironpark/ggui/cmd/ggui/internal/macos"
	"github.com/ironpark/ggui/cmd/ggui/internal/project"
	"github.com/ironpark/ggui/cmd/ggui/internal/web"
)

var buildCmd = &command{
	name:    "build",
	args:    "[package]",
	summary: "build a release: a macOS .app, a web bundle or a program",
	help: `Build makes a release build of the application, without the development
tags: -trimpath, and -ldflags "-s -w" followed by build.ldflags.

Targets:

  macos   Name.app in the output directory: the program, Info.plist from
          [app] and [macos], the icon made from app.icon, signed with
          macos.signing_identity (ad hoc by default). The default on macOS.
          Signing needs macOS; elsewhere the bundle is left unsigned.
  web     a directory for any static host: index.html, main.wasm and the
          Go toolchain's wasm_exec.js, each with a precompressed .gz copy,
          the binary run through wasm-opt (web.wasm_opt) when installed.
  native  the program for the go command's GOOS and GOARCH. The default
          elsewhere.

The output directory is build.output, build/ by default; the web bundle
goes in its web/ subdirectory. --out names the directory instead.`,
	flags: func(fs *flag.FlagSet) func(context.Context, []string, []string) error {
		cfgPath := fs.String("config", "", "read `file` instead of the ggui.toml found from the current directory")
		tgt := fs.String("target", defaultTarget(), "what to build: `macos`, web or native")
		out := fs.String("out", "", "the output `directory`, replacing build.output")
		universal := fs.Bool("universal", false, "with --target macos, build for Apple silicon and Intel and join them with lipo (macOS only)")
		tags := fs.String("tags", "", "build `tags`, comma-separated, replacing build.tags")
		ldflags := fs.String("ldflags", "", "linker `flags` after -s -w, replacing build.ldflags")
		return func(ctx context.Context, pos, rest []string) error {
			if len(pos) > 1 || len(rest) > 0 {
				fmt.Fprintln(os.Stderr, "ggui build: takes one package")
				return errUsage
			}
			arg := ""
			if len(pos) == 1 {
				arg = pos[0]
			}
			proj, err := project.Open(".", arg, *cfgPath)
			if err != nil {
				return err
			}
			cfg := proj.Config
			b := &builder{proj: proj, out: *out, tags: cfg.Build.Tags, ldflags: cfg.Build.LDFlags}
			if isSet(fs, "tags") {
				b.tags = splitTags(*tags)
			}
			if isSet(fs, "ldflags") {
				b.ldflags = *ldflags
			}
			b.universal = cfg.MacOS.Universal || *universal

			lo := gotool.ListOptions{Tags: b.tags}
			switch *tgt {
			case "macos":
				lo.GOOS = "darwin"
			case "web":
				lo.GOOS, lo.GOARCH = "js", "wasm"
			case "native":
			default:
				return fmt.Errorf("unknown --target %q: use macos, web or native", *tgt)
			}
			if *universal && *tgt != "macos" {
				return errors.New("--universal makes a macOS bundle: add --target macos")
			}
			if b.main, err = proj.ResolveMain(ctx, arg, lo); err != nil {
				return err
			}
			switch *tgt {
			case "macos":
				return b.macos(ctx)
			case "web":
				return b.web(ctx)
			default:
				return b.native(ctx)
			}
		}
	},
}

// defaultTarget is the bundle the host makes most use of.
func defaultTarget() string {
	if runtime.GOOS == "darwin" {
		return "macos"
	}
	return "native"
}

type builder struct {
	proj      *project.Project
	main      *project.Main
	out       string
	tags      []string
	ldflags   string
	universal bool
}

// release returns the options every release build shares.
func (b *builder) release(output string) gotool.BuildOptions {
	ld := "-s -w"
	if b.ldflags != "" {
		ld += " " + b.ldflags
	}
	return gotool.BuildOptions{
		Dir: b.proj.Root, Package: b.main.Package, Output: output,
		Tags: b.tags, LDFlags: ld, Trimpath: true,
	}
}

func (b *builder) build(ctx context.Context, o gotool.BuildOptions) error {
	fmt.Fprintln(os.Stderr, o)
	return gotool.Build(ctx, o)
}

func (b *builder) native(ctx context.Context) error {
	dir, err := b.proj.OutDir(b.out)
	if err != nil {
		return err
	}
	env, err := gotool.Env(ctx, b.proj.Root, nil, "GOOS", "GOARCH")
	if err != nil {
		return err
	}
	name := b.main.Executable
	if env["GOOS"] == "windows" {
		name += ".exe"
	}
	out := filepath.Join(dir, name)
	if err := b.build(ctx, b.release(out)); err != nil {
		return err
	}
	return report(out, env["GOOS"]+"/"+env["GOARCH"])
}

func (b *builder) macos(ctx context.Context) error {
	if err := b.proj.CheckIcon(); err != nil {
		return err
	}
	cfg := b.proj.Config
	dir, err := b.proj.OutDir(b.out)
	if err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "ggui-build-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	bin := filepath.Join(tmp, b.main.Executable)
	arch := ""
	if b.universal {
		if runtime.GOOS != "darwin" {
			return errors.New("--universal joins the two builds with lipo, which only macOS has: build on a Mac, or drop it and set GOARCH")
		}
		var halves []string
		for _, a := range []string{"arm64", "amd64"} {
			o := b.release(bin + "-" + a)
			o.GOOS, o.GOARCH = "darwin", a
			if err := b.build(ctx, o); err != nil {
				return err
			}
			halves = append(halves, o.Output)
		}
		if err := macos.Lipo(ctx, bin, halves...); err != nil {
			return err
		}
		arch = "arm64 + amd64"
	} else {
		o := b.release(bin)
		o.GOOS = "darwin"
		if err := b.build(ctx, o); err != nil {
			return err
		}
		env, err := gotool.Env(ctx, b.proj.Root, []string{"GOOS=darwin"}, "GOARCH")
		if err != nil {
			return err
		}
		arch = env["GOARCH"]
	}

	app, warnings, err := macos.Assemble(macos.Bundle{
		Info: macos.Info{
			Name: b.main.Name, Executable: b.main.Executable, ID: b.main.ID,
			Version:              cfg.App.Version,
			MinimumSystemVersion: cfg.MacOS.MinimumSystemVersion,
			Category:             cfg.MacOS.Category,
			Extra:                cfg.MacOS.Plist,
		},
		Binary: bin,
		Icon:   b.proj.Path(cfg.App.Icon),
		OutDir: dir,
	})
	if err != nil {
		return err
	}
	for _, w := range warnings {
		fmt.Fprintln(os.Stderr, "ggui: warning:", w)
	}
	if b.main.ID != cfg.App.ID {
		fmt.Fprintf(os.Stderr, "ggui: app.id is not set, so the bundle is %s; set it in ggui.toml before shipping, since macOS keys settings and permissions to it\n", b.main.ID)
	}
	if macos.CanSign() {
		id := cfg.MacOS.SigningIdentity
		if err := macos.Sign(ctx, app, id, b.proj.Path(cfg.MacOS.Entitlements)); err != nil {
			return err
		}
		if id == "-" {
			fmt.Fprintln(os.Stderr, "ggui: signed ad hoc, which runs on this Mac; set macos.signing_identity to ship it to others")
		} else {
			fmt.Fprintf(os.Stderr, "ggui: signed as %s\n", id)
		}
	} else {
		fmt.Fprintln(os.Stderr, "ggui: the bundle is not signed, since codesign needs macOS; sign it on a Mac before shipping it")
	}
	return report(app, "macOS "+arch)
}

func (b *builder) web(ctx context.Context) error {
	cfg := b.proj.Config
	dir := filepath.Join(b.proj.Path(cfg.Build.Output), "web")
	if b.out != "" {
		var err error
		if dir, err = filepath.Abs(b.out); err != nil {
			return err
		}
	}
	title := cfg.Web.Title
	if title == "" {
		title = b.main.Name
	}
	o := b.release("")
	r, err := web.WriteBundle(ctx, web.BundleOptions{
		BuildOptions: web.BuildOptions{
			Dir: o.Dir, Package: o.Package, Tags: o.Tags, LDFlags: o.LDFlags, Trimpath: true,
			WasmOpt: cfg.Web.WasmOpt,
			Logf: func(format string, args ...any) {
				fmt.Fprintf(os.Stderr, "ggui: "+format+"\n", args...)
			},
		},
		OutDir: dir,
		Title:  title,
		Base:   cfg.Web.Base,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "ggui: %s, %s gzipped; serve %s at %s\n", web.MB(r.Size), web.MB(r.GzSize), dir, web.CleanBase(cfg.Web.Base))
	fmt.Println(dir)
	return nil
}

// report says what was built, on standard error, and prints its path on
// standard output, where a script can take it.
func report(path, what string) error {
	size := ""
	if fi, err := os.Stat(path); err == nil && !fi.IsDir() {
		size = ", " + web.MB(fi.Size())
	}
	rel := path
	if wd, err := os.Getwd(); err == nil {
		if r, err := filepath.Rel(wd, path); err == nil && !strings.HasPrefix(r, "..") {
			rel = r
		}
	}
	fmt.Fprintf(os.Stderr, "ggui: built %s (%s%s)\n", rel, what, size)
	fmt.Println(path)
	return nil
}
