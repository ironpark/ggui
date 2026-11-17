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
	"time"

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
          macos.signing_identity (ad hoc by default). With --dmg or
          [macos.dmg] also Name.dmg, and with --pkg or [macos.pkg]
          Name.pkg; with --notarize each is notarized and stapled. A
          build signed with an identity is verified at the end. The
          default on macOS.
  web     a directory for any static host: index.html, main.wasm and the
          Go toolchain's wasm_exec.js, each with a precompressed .gz copy,
          the binary run through wasm-opt (web.wasm_opt) when installed.
  native  the program for the go command's GOOS and GOARCH. The default
          elsewhere.

The output directory is build.output, build/ by default; the web bundle
goes in its web/ subdirectory. --out names the directory instead.

Signing with an identity uses the keychain on macOS, or a certificate:
--p12-file, macos.p12_file, or base64 text in $GGUI_P12_BASE64, its
password in --p12-password-file or $GGUI_P12_PASSWORD. Signing and
notarizing take a Mac; elsewhere the bundle is left unsigned. Notarizing
uses a notarytool profile, an App Store Connect API key, or an Apple ID
with its app-specific password in $GGUI_NOTARY_PASSWORD. docs/cli.md
has the details.`,
	flags: func(fs *flag.FlagSet) func(context.Context, []string, []string) error {
		cfgPath := fs.String("config", "", "read `file` instead of the ggui.toml found from the current directory")
		tgt := fs.String("target", defaultTarget(), "what to build: `macos`, web or native")
		out := fs.String("out", "", "the output `directory`, replacing build.output")
		universal := fs.Bool("universal", false, "with --target macos, build for Apple silicon and Intel and join them with lipo (macOS only)")
		tags := fs.String("tags", "", "build `tags`, comma-separated, replacing build.tags")
		ldflags := fs.String("ldflags", "", "linker `flags` after -s -w, replacing build.ldflags")
		mf := macFlags{
			identity:     fs.String("identity", "", "sign the macOS bundle as `identity`, replacing macos.signing_identity; - signs ad hoc"),
			p12File:      fs.String("p12-file", "", "sign with the PKCS#12 certificate in `file`, replacing macos.p12_file"),
			p12Password:  fs.String("p12-password-file", "", "read the certificate's password from `file`, replacing macos.p12_password_file"),
			notarize:     fs.Bool("notarize", false, "notarize and staple what --target macos makes, replacing macos.notarize"),
			notaryProf:   fs.String("notary-profile", "", "notarize with the notarytool keychain `profile`, replacing macos.notary.profile"),
			apiKeyFile:   fs.String("api-key-file", "", "notarize with the App Store Connect API key in `file`, replacing macos.notary.api_key_file"),
			appleID:      fs.String("apple-id", "", "notarize as the Apple `ID`, replacing macos.notary.apple_id"),
			teamID:       fs.String("team-id", "", "the Apple ID's `team`, replacing macos.notary.team_id"),
			dmg:          fs.Bool("dmg", false, "also make Name.dmg; --dmg=false skips the one [macos.dmg] asks for"),
			pkg:          fs.Bool("pkg", false, "also make the installer Name.pkg; --pkg=false skips the one [macos.pkg] asks for"),
			isSet:        func(name string) bool { return isSet(fs, name) },
			macOnlyFlags: []string{"identity", "p12-file", "p12-password-file", "notarize", "notary-profile", "api-key-file", "apple-id", "team-id", "dmg", "pkg"},
		}
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
			if *tgt != "macos" {
				for _, name := range mf.macOnlyFlags {
					if isSet(fs, name) {
						return fmt.Errorf("--%s is for a macOS build: add --target macos", name)
					}
				}
			}
			b.mac = mf
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
	mac       macFlags
}

// macFlags are the flags that sign, notarize and package a macOS build.
type macFlags struct {
	identity, p12File, p12Password          *string
	notarize                                *bool
	notaryProf, apiKeyFile, appleID, teamID *string
	dmg, pkg                                *bool
	isSet                                   func(string) bool
	macOnlyFlags                            []string
}

// str returns a string flag's value when it was given, else the
// configured one.
func (f macFlags) str(name string, flag *string, configured string) string {
	if f.isSet(name) {
		return *flag
	}
	return configured
}

// file returns a path flag's value, made absolute against the working
// directory, when it was given, else the configured path, which the
// caller has resolved against the project.
func (f macFlags) file(name string, flag *string, configured string) string {
	if f.isSet(name) && *flag != "" {
		if abs, err := filepath.Abs(*flag); err == nil {
			return abs
		}
	}
	return f.str(name, flag, configured)
}

// boolean returns a bool flag's value when it was given, else the
// configured one.
func (f macFlags) boolean(name string, flag *bool, configured bool) bool {
	if f.isSet(name) {
		return *flag
	}
	return configured
}

// signing gathers how to sign and notarize from ggui.toml, the flags,
// which take precedence, and the environment, which holds the secrets.
func (b *builder) signing() (s macos.Signing, notarize bool, err error) {
	m, f := b.proj.Config.MacOS, b.mac
	s = macos.Signing{
		Identity:     f.str("identity", f.identity, m.SigningIdentity),
		Entitlements: b.proj.Path(m.Entitlements),
		PKGIdentity:  m.PKG.SigningIdentity,
	}
	if s.Identity == "" {
		return s, false, errors.New(`--identity must name an identity, or "-" to sign ad hoc`)
	}
	if file := f.file("p12-file", f.p12File, b.proj.Path(m.P12File)); file != "" {
		s.P12File, s.P12Source = file, file
	} else if v := os.Getenv(m.P12Base64Env); m.P12Base64Env != "" && v != "" {
		s.P12Base64, s.P12Source = v, "$"+m.P12Base64Env
	}
	if s.HasCertificate() {
		s.P12PasswordFile = f.file("p12-password-file", f.p12Password, b.proj.Path(m.P12PasswordFile))
		if s.P12PasswordFile == "" && m.P12PasswordEnv != "" {
			s.P12Password = os.Getenv(m.P12PasswordEnv)
		}
	} else if f.isSet("p12-password-file") {
		return s, false, errors.New("--p12-password-file opens a certificate: give it with --p12-file, macos.p12_file or $" + m.P12Base64Env)
	}

	n := m.Notary
	timeout, err := time.ParseDuration(n.Timeout)
	if err != nil {
		return s, false, err // validated with the file
	}
	s.Notary = macos.Notary{
		Profile:     f.str("notary-profile", f.notaryProf, n.Profile),
		APIKeyFile:  f.file("api-key-file", f.apiKeyFile, b.proj.Path(n.APIKeyFile)),
		AppleID:     f.str("apple-id", f.appleID, n.AppleID),
		TeamID:      f.str("team-id", f.teamID, n.TeamID),
		PasswordVar: n.PasswordEnv,
		Staple:      n.Staple,
		Timeout:     timeout,
	}
	if n.PasswordEnv != "" {
		s.Notary.Password = os.Getenv(n.PasswordEnv)
	}
	return s, f.boolean("notarize", f.notarize, m.Notarize), nil
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

	// Credentials that cannot sign or notarize fail here, before the
	// build, rather than after it.
	s, notarize, err := b.signing()
	if err != nil {
		return err
	}
	sg, err := macos.NewSigner(s, notarize)
	if err != nil {
		return err
	}
	defer func() {
		if err := sg.Close(); err != nil {
			fmt.Fprintln(os.Stderr, "ggui: warning: cleaning up after signing:", err)
		}
	}()
	if err := sg.Check(ctx); err != nil {
		return err
	}
	if !sg.AdHoc() {
		logf("signing as %s, from %s", sg.Identity, s.Describe())
	}
	makeDMG := b.mac.boolean("dmg", b.mac.dmg, cfg.MacOS.DMG.Make)
	makePKG := b.mac.boolean("pkg", b.mac.pkg, cfg.MacOS.PKG.Make)
	if makePKG {
		if err := sg.CheckInstaller(ctx); err != nil {
			return err
		}
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
		logf("app.id is not set, so the bundle is %s; set it in ggui.toml before shipping, since macOS keys settings and permissions to it", b.main.ID)
	}

	signed, err := sg.Sign(ctx, app)
	switch {
	case err != nil:
		return err
	case !signed:
		logf("the bundle is not signed, since codesign needs macOS; sign it on a Mac before shipping it, or set macos.signing_identity and give a certificate")
	case sg.AdHoc():
		logf("signed ad hoc, which runs on this Mac; set macos.signing_identity to ship it to others")
	}
	artifacts := []string{app}
	if notarize {
		if err := b.notarize(ctx, sg, app); err != nil {
			return err
		}
	}

	m := cfg.MacOS
	if makeDMG {
		d := m.DMG
		out := filepath.Join(dir, b.main.Name+".dmg")
		title := d.Title
		if title == "" {
			title = b.main.Name
		}
		err := macos.MakeDMG(ctx, macos.DMG{
			App: app, Out: out, Title: title,
			Icon: b.proj.Path(d.Icon), Background: b.proj.Path(d.Background),
			Width: d.Window.Width, Height: d.Window.Height,
			IconSize: d.IconSize, LabelSize: d.LabelSize,
			AppPosition: d.AppPosition, ApplicationsPosition: d.ApplicationsPosition,
			FS: d.FS, Format: d.Format,
		})
		if err != nil {
			return err
		}
		// An ad hoc signature on a disk image means nothing to another
		// Mac, and the app inside is signed already.
		if !sg.AdHoc() {
			if _, err := sg.Sign(ctx, out); err != nil {
				return err
			}
			if notarize {
				if err := b.notarize(ctx, sg, out); err != nil {
					return err
				}
			}
		}
		artifacts = append(artifacts, out)
	}
	if makePKG {
		p := m.PKG
		out := filepath.Join(dir, b.main.Name+".pkg")
		err := macos.MakePKG(ctx, macos.PKG{
			App: app, Out: out, ID: b.main.ID, Version: cfg.App.Version,
			MinOS: m.MinimumSystemVersion, InstallLocation: p.InstallLocation,
			Scripts: b.proj.Path(p.Scripts), License: b.proj.Path(p.License),
		})
		if err != nil {
			return err
		}
		signed, err := sg.Sign(ctx, out)
		if err != nil {
			return err
		}
		if !signed {
			logf("the installer is not signed, since the app is signed ad hoc; macOS opens an unsigned installer only after a warning")
		} else if notarize {
			if err := b.notarize(ctx, sg, out); err != nil {
				return err
			}
		}
		artifacts = append(artifacts, out)
	}

	if !sg.AdHoc() {
		if err := verifyAll(ctx, artifacts, notarize); err != nil {
			return err
		}
		if !notarize {
			logf("not notarized, so Gatekeeper refuses it on other Macs; add --notarize, or macos.notarize = true, to ship it")
		}
	}
	if err := report(app, "macOS "+arch); err != nil {
		return err
	}
	for _, a := range artifacts[1:] {
		if err := report(a, "macOS "+arch); err != nil {
			return err
		}
	}
	return nil
}

// notarize notarizes path, saying so, since it takes minutes.
func (b *builder) notarize(ctx context.Context, sg *macos.Signer, path string) error {
	logf("notarizing %s; Apple's notary service usually takes a few minutes", filepath.Base(path))
	start := time.Now()
	if err := sg.Notarize(ctx, path); err != nil {
		return err
	}
	what := "notarized"
	if sg.Staples() {
		what += " and stapled"
	}
	logf("%s %s in %s", what, filepath.Base(path), time.Since(start).Round(time.Second))
	return nil
}

// verifyAll checks each artifact as a release, printing what was found,
// and fails when a check does.
func verifyAll(ctx context.Context, artifacts []string, notarized bool) error {
	var failed []string
	for _, a := range artifacts {
		r, fails := macos.Verify(ctx, a, notarized)
		fmt.Fprint(os.Stderr, "ggui: verified ", r)
		for _, c := range fails {
			failed = append(failed, fmt.Sprintf("%s: %s: %s", filepath.Base(a), c.Name, c.Detail))
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("what was built failed verification:\n  %s", strings.Join(failed, "\n  "))
	}
	return nil
}

// logf says something on standard error, as ggui does.
func logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "ggui: "+format+"\n", args...)
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
