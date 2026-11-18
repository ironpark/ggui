package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/ironpark/ggui/cmd/ggui/internal/gotool"
	"github.com/ironpark/ggui/cmd/ggui/internal/macos"
	"github.com/ironpark/ggui/cmd/ggui/internal/project"
	"github.com/ironpark/ggui/cmd/ggui/internal/ui"
	"github.com/ironpark/ggui/cmd/ggui/internal/web"
	"github.com/ironpark/ggui/cmd/ggui/internal/windows"
)

var buildCmd = &command{
	name:    "build",
	args:    "[package]",
	summary: "build a release: a macOS .app, a Windows .exe, a web bundle or a program",
	help: `Build makes a release build of the application, without the development
tags: -trimpath, and -ldflags "-s -w" followed by build.ldflags.

Targets:

  macos    Name.app in the output directory: the program, Info.plist from
           [app] and [macos], the icon made from app.icon, signed with
           macos.signing_identity (ad hoc by default). With --dmg or
           [macos.dmg] also Name.dmg, and with --pkg or [macos.pkg]
           Name.pkg; with --notarize each is notarized and stapled. A
           build signed with an identity is verified at the end. The
           default on macOS.
  windows  Name.exe in the output directory, for the go command's GOARCH
           (amd64, arm64 or 386), with the icon made from app.icon,
           version information from [app] and [windows], and a manifest,
           linked as a GUI program that opens no console (--console keeps
           it). The default on Windows.
  web      a directory for any static host: index.html, main.wasm and the
           Go toolchain's wasm_exec.js, each with a precompressed .gz copy,
           the binary run through wasm-opt (web.wasm_opt) when installed.
  native   the program for the go command's GOOS and GOARCH. The default
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
		tgt := fs.String("target", defaultTarget(), "what to build: `macos`, windows, web or native")
		out := fs.String("out", "", "the output `directory`, replacing build.output")
		universal := fs.Bool("universal", false, "with --target macos, build for Apple silicon and Intel and join them with lipo (macOS only)")
		console := fs.Bool("console", false, "with --target windows, link for the console, which Windows opens a window of, replacing windows.console")
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
			b.console = cfg.Windows.Console
			if isSet(fs, "console") {
				b.console = *console
			}

			lo := gotool.ListOptions{Tags: b.tags}
			switch *tgt {
			case "macos":
				lo.GOOS = "darwin"
			case "windows":
				// The architecture is resolved first, as the files the
				// package is built from, its .syso files among them,
				// depend on it.
				env, err := gotool.Env(ctx, proj.Root, []string{"GOOS=windows"}, "GOARCH")
				if err != nil {
					return err
				}
				b.arch = env["GOARCH"]
				if !slices.Contains(windows.Arches, b.arch) {
					return fmt.Errorf("GOARCH=%s: ggui builds Windows programs for %s; set GOARCH to one of them", b.arch, strings.Join(windows.Arches, ", "))
				}
				lo.GOOS, lo.GOARCH = "windows", b.arch
			case "web":
				lo.GOOS, lo.GOARCH = "js", "wasm"
			case "native":
			default:
				return fmt.Errorf("unknown --target %q: use macos, windows, web or native", *tgt)
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
			if *tgt != "windows" && isSet(fs, "console") {
				return errors.New("--console is for a Windows build: add --target windows")
			}
			b.mac = mf
			if b.main, err = resolveMain(ctx, ui.Stderr, proj, arg, lo); err != nil {
				return err
			}
			switch *tgt {
			case "macos":
				return b.macos(ctx)
			case "windows":
				return b.windows(ctx)
			case "web":
				return b.web(ctx)
			default:
				return b.native(ctx)
			}
		}
	},
}

// defaultTarget is the bundle the host makes most use of: on macOS and
// Windows the program as people there run it, elsewhere the program
// itself.
func defaultTarget() string {
	switch runtime.GOOS {
	case "darwin":
		return "macos"
	case "windows":
		return "windows"
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
	console   bool   // link a Windows program for the console
	arch      string // the GOARCH of a Windows build
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

// build compiles for target, GOOS/GOARCH, as a step under the go command
// line it runs.
func (b *builder) build(ctx context.Context, o gotool.BuildOptions, target string) error {
	log := ui.Stderr
	log.Println(log.Dim(o.String()))
	return step(ctx, "Compiling for "+target, "Compiled for "+target, func() error {
		return gotool.Build(ctx, o)
	})
}

// step runs fn as a step called title, ending in done or in what went
// wrong, which the caller returns for main to print.
func step(ctx context.Context, title, done string, fn func() error) error {
	s := ui.Stderr.Step(title)
	err := fn()
	switch {
	case err != nil && ctx.Err() != nil:
		s.Warn("Interrupted: " + title)
	case err != nil:
		s.Fail(title + " failed")
	default:
		s.Done(done)
	}
	return err
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
	if err := b.build(ctx, b.release(out), env["GOOS"]+"/"+env["GOARCH"]); err != nil {
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
			ui.Stderr.Warn("cleaning up after signing: %v", err)
		}
	}()
	check := ui.Stderr.Step("Checking how to sign", ui.QuietStart())
	err = sg.Check(ctx)
	check.Stop()
	if err != nil {
		return err
	}
	if !sg.AdHoc() {
		logf("Signing as %s, from %s", sg.Identity, s.Describe())
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
			if err := b.build(ctx, o, "darwin/"+a); err != nil {
				return err
			}
			halves = append(halves, o.Output)
		}
		err := step(ctx, "Joining arm64 and amd64 with lipo", "Joined arm64 and amd64 with lipo", func() error {
			return macos.Lipo(ctx, bin, halves...)
		})
		if err != nil {
			return err
		}
		arch = "arm64 + amd64"
	} else {
		env, err := gotool.Env(ctx, b.proj.Root, []string{"GOOS=darwin"}, "GOARCH")
		if err != nil {
			return err
		}
		arch = env["GOARCH"]
		o := b.release(bin)
		o.GOOS = "darwin"
		if err := b.build(ctx, o, "darwin/"+arch); err != nil {
			return err
		}
	}

	appName := b.main.Name + ".app"
	assemble := ui.Stderr.Step("Assembling " + appName)
	app, warnings, err := macos.Assemble(macos.Bundle{
		Name: b.main.Name, Executable: b.main.Executable, ID: b.main.ID,
		Version:              cfg.App.Version,
		MinimumSystemVersion: cfg.MacOS.MinimumSystemVersion,
		Category:             cfg.MacOS.Category,
		Extra:                cfg.MacOS.Plist,
		Binary:               bin,
		Icon:                 b.proj.Path(cfg.App.Icon),
		IconInset:            cfg.MacOS.IconInset,
		OutDir:               dir,
	})
	if err != nil {
		assemble.Fail("Assembling " + appName + " failed")
		return err
	}
	assemble.Done("Assembled " + appName)
	for _, w := range warnings {
		ui.Stderr.Warn("%s", w)
	}
	if b.main.ID != cfg.App.ID {
		ui.Stderr.Warn("app.id is not set, so the bundle is %s; set it in ggui.toml before shipping, since macOS keys settings and permissions to it", b.main.ID)
	}

	signed, err := sign(ctx, sg, app)
	switch {
	case err != nil:
		return err
	case !signed:
		ui.Stderr.Warn("The bundle is not signed, since codesign needs macOS; sign it on a Mac before shipping it, or set macos.signing_identity and give a certificate")
	case sg.AdHoc():
		ui.Stderr.Note("%s", ui.Stderr.Dim("which runs on this Mac; set macos.signing_identity to ship it to others"))
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
		err := step(ctx, "Making "+filepath.Base(out), "Made "+filepath.Base(out), func() error {
			return macos.MakeDMG(ctx, macos.DMG{
				App: app, Out: out, Title: title,
				Icon: b.proj.Path(d.Icon), Background: b.proj.Path(d.Background),
				Width: d.Window.Width, Height: d.Window.Height,
				IconSize: d.IconSize, LabelSize: d.LabelSize,
				AppPosition: d.AppPosition, ApplicationsPosition: d.ApplicationsPosition,
				FS: d.FS, Format: d.Format,
			})
		})
		if err != nil {
			return err
		}
		// An ad hoc signature on a disk image means nothing to another
		// Mac, and the app inside is signed already.
		if !sg.AdHoc() {
			if _, err := sign(ctx, sg, out); err != nil {
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
		err := step(ctx, "Making "+filepath.Base(out), "Made "+filepath.Base(out), func() error {
			return macos.MakePKG(ctx, macos.PKG{
				App: app, Out: out, ID: b.main.ID, Version: cfg.App.Version,
				MinOS: m.MinimumSystemVersion, InstallLocation: p.InstallLocation,
				Scripts: b.proj.Path(p.Scripts), License: b.proj.Path(p.License),
			})
		})
		if err != nil {
			return err
		}
		signed, err := sign(ctx, sg, out)
		if err != nil {
			return err
		}
		if !signed {
			ui.Stderr.Warn("The installer is not signed, since the app is signed ad hoc; macOS opens an unsigned installer only after a warning")
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
			ui.Stderr.Warn("Not notarized, so Gatekeeper refuses it on other Macs; add --notarize, or macos.notarize = true, to ship it")
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

// sign signs path as a step, reporting whether it was signed: a step that
// signs nothing, as away from a Mac, ends without a word.
func sign(ctx context.Context, sg *macos.Signer, path string) (bool, error) {
	title := "Signing " + filepath.Base(path)
	s := ui.Stderr.Step(title)
	signed, err := sg.Sign(ctx, path)
	switch {
	case err != nil && ctx.Err() != nil:
		s.Warn("Interrupted: " + title)
	case err != nil:
		s.Fail(title + " failed")
	case !signed:
		s.Stop()
	case sg.AdHoc():
		s.Done("Signed " + filepath.Base(path) + " ad hoc")
	default:
		s.Done("Signed " + filepath.Base(path) + " as " + sg.Identity)
	}
	return signed, err
}

// notarize notarizes path, saying so, since it takes minutes.
func (b *builder) notarize(ctx context.Context, sg *macos.Signer, path string) error {
	what := "Notarized"
	if sg.Staples() {
		what += " and stapled"
	}
	name := filepath.Base(path)
	return step(ctx, "Notarizing "+name+"; Apple's notary service usually takes a few minutes", what+" "+name, func() error {
		return sg.Notarize(ctx, path)
	})
}

// verifyAll checks each artifact as a release, printing what was found,
// and fails when a check does.
func verifyAll(ctx context.Context, artifacts []string, notarized bool) error {
	var failed []string
	log := ui.Stderr
	for _, a := range artifacts {
		s := log.Step("Verifying " + filepath.Base(a))
		r, fails := macos.Verify(ctx, a, notarized)
		if len(fails) > 0 {
			s.Fail("Verified " + filepath.Base(a) + ": " + fmt.Sprint(len(fails)) + " failed")
		} else {
			s.Done("Verified " + filepath.Base(a))
		}
		// The report's first line names the artifact; its checks follow.
		if _, checks, ok := strings.Cut(r, "\n"); ok && checks != "" {
			log.Printf("%s", log.Dim(checks))
		}
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
func logf(format string, args ...any) { ui.Stderr.Info(format, args...) }

// windows builds Name.exe: the program for windows/arch with its icon,
// version information and manifest linked in, as a GUI program unless
// asked for the console. The resources go in a .syso file in the main
// package's directory, the one place the go command links them from,
// for the build only.
//
// Signing the program and making an installer come after this, from
// Name.exe as it is, and are not done here.
func (b *builder) windows(ctx context.Context) error {
	if err := b.proj.CheckIcon(); err != nil {
		return err
	}
	cfg := b.proj.Config
	if err := windows.CheckFileName(b.main.Name); err != nil {
		return err
	}
	dir, err := b.proj.OutDir(b.out)
	if err != nil {
		return err
	}
	exe := b.main.Name + ".exe"
	log := ui.Stderr

	// The linker takes one resource section per program, so a package
	// that links resources of its own is built with them.
	var theirs []string
	for _, f := range b.main.SysoFiles {
		if f != windows.SysoName(b.arch) && windows.HasResources(filepath.Join(b.main.Dir, f)) {
			theirs = append(theirs, f)
		}
	}
	if len(theirs) > 0 {
		log.Info("%s links resources of its own from %s, so %s has those rather than ggui's icon, version information and manifest",
			target(b.main), strings.Join(theirs, ", "), exe)
	} else {
		w := cfg.Windows
		r := windows.Resources{
			Arch: b.arch, Name: b.main.Name, Executable: b.main.Executable, ID: b.main.ID,
			Version:     cfg.App.Version,
			Description: w.Description, Company: w.Company, Copyright: w.Copyright,
			Icon:     b.proj.Path(cfg.App.Icon),
			Manifest: b.proj.Path(w.Manifest),
		}
		what := "version and manifest"
		if r.Icon != "" {
			what = "icon, " + what
		}
		var (
			path     string
			leftover bool
			remove   func() error
			warnings []string
		)
		err := step(ctx, "Writing the "+what+" of "+exe, "Wrote the "+what+" of "+exe, func() error {
			obj, ws, err := windows.Object(r)
			if err != nil {
				return err
			}
			warnings = ws
			path, leftover, remove, err = windows.Place(b.main.Dir, b.arch, obj)
			return err
		})
		if err != nil {
			return err
		}
		// Removed however the build ends, failed or interrupted too: the
		// file belongs to this build, not to the sources.
		defer func() {
			if err := remove(); err != nil {
				log.Warn("removing %s: %v; delete it before building for another target", path, err)
			}
		}()
		for _, w := range warnings {
			log.Warn("%s", w)
		}
		if leftover {
			log.Note("%s", log.Dim("replacing the "+filepath.Base(path)+" a build that was stopped left behind"))
		}
		log.Note("%s", log.Dim("linked from "+shortPath(path)+", which is removed after the build"))
	}

	o := b.release(filepath.Join(dir, exe))
	o.GOOS, o.GOARCH = "windows", b.arch
	if !b.console {
		// The GUI subsystem: Windows opens no console for the program,
		// and its standard streams go nowhere unless redirected.
		o.LDFlags += " -H=windowsgui"
	}
	if err := b.build(ctx, o, "windows/"+b.arch); err != nil {
		return err
	}
	if runtime.GOOS != "windows" && b.arch == runtime.GOARCH && b.arch != "amd64" && os.Getenv("GOARCH") == "" {
		log.Note("%s", log.Dim("for "+b.arch+", this machine's architecture; most PCs need GOARCH=amd64"))
	}
	return report(o.Output, "windows/"+b.arch)
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
	log := ui.Stderr
	var r *web.Result
	err := step(ctx, "Building for js/wasm", "Built for js/wasm", func() error {
		var err error
		r, err = web.WriteBundle(ctx, web.BundleOptions{
			Dir: o.Dir, Package: o.Package, Tags: o.Tags, LDFlags: o.LDFlags, Trimpath: true,
			WasmOpt: cfg.Web.WasmOpt,
			Logf: func(format string, args ...any) {
				// The go command line, dimmed as for the other
				// targets; then what wasm-opt and gzip had to say.
				if format == "%s" && len(args) == 1 {
					if bo, ok := args[0].(gotool.BuildOptions); ok {
						log.Println(log.Dim(bo.String()))
						return
					}
				}
				log.Info(format, args...)
			},
			OutDir: dir,
			Title:  title,
			Base:   cfg.Web.Base,
		})
		return err
	})
	if err != nil {
		return err
	}
	log.Success("%s, %s gzipped; serve %s at %s", web.MB(r.Size), web.MB(r.GzSize), log.Cyan(dir), web.CleanBase(cfg.Web.Base))
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
	ui.Stderr.Success("Built %s (%s%s)", ui.Stderr.Cyan(rel), what, size)
	fmt.Println(path)
	return nil
}
