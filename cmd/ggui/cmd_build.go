package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/ironpark/ggui/cmd/ggui/internal/gotool"
	"github.com/ironpark/ggui/cmd/ggui/internal/i18n"
	"github.com/ironpark/ggui/cmd/ggui/internal/installer"
	"github.com/ironpark/ggui/cmd/ggui/internal/linux"
	"github.com/ironpark/ggui/cmd/ggui/internal/macos"
	"github.com/ironpark/ggui/cmd/ggui/internal/project"
	"github.com/ironpark/ggui/cmd/ggui/internal/svgicon"
	"github.com/ironpark/ggui/cmd/ggui/internal/ui"
	"github.com/ironpark/ggui/cmd/ggui/internal/web"
	"github.com/ironpark/ggui/cmd/ggui/internal/windows"
	"github.com/ironpark/ggui/cmd/ggui/internal/ziglib"
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
           [macos.dmg] also Name.dmg, and with --installer, --pkg, a Go
           installer package (./cmd/installer, ./installer or
           build.installer) or [macos.pkg] Name.pkg; with --notarize each
           is notarized and stapled. A build signed with an identity is
           verified at the end. The default on macOS.
  windows  Name.exe in the output directory, for the go command's GOARCH
           (amd64, arm64 or 386), with the icon made from app.icon,
           version information from [app] and [windows], and a manifest,
           linked as a GUI program that opens no console (--console keeps
           it). With --installer or a Go installer package (./cmd/installer,
           ./installer or build.installer) also Name-Setup.exe, a ggui-based
           installer and uninstaller. The default on Windows.
  linux    an install tree, name-linux-arch, in the output directory: the
           program in usr/bin, built for the go command's GOARCH (amd64,
           arm64 or riscv64) without cgo, a desktop entry, the icon made
           from app.icon in the hicolor sizes, and AppStream metadata,
           from [app] and [linux]. Beside it what --format or
           linux.formats names: tar, the tarball
           name-version-linux-arch.tar.gz (the default); deb, a Debian
           package; rpm, an RPM package.
  web      a directory for any static host: index.html, main.wasm and the
           Go toolchain's wasm_exec.js, each with a precompressed .gz copy,
           the binary run through wasm-opt (web.wasm_opt) when installed.
  native   the program for the go command's GOOS and GOARCH. The default
           elsewhere.

The output directory is build.output, build/ by default; the web bundle
goes in its web/ subdirectory. --out names the directory instead.

Signing with an identity uses the keychain on macOS, or a certificate:
--p12-file, macos.p12_file, or base64 text in $GGUI_P12_BASE64, its
password in --p12-password-file or $GGUI_P12_PASSWORD. Notarizing uses
a notarytool profile, an App Store Connect API key, or an Apple ID with
its app-specific password in $GGUI_NOTARY_PASSWORD. Signing ad hoc, and
--universal, work on any host. On Linux and Windows a build of ggui with
zapp's rcodesign library, as the release binaries are, signs with a
certificate and notarizes with an API key; elsewhere signing with an
identity takes a Mac. docs/cli.md has the details.`,
	flags: func(fs *flag.FlagSet) func(context.Context, []string, []string) error {
		f := newBuildFlags(fs)
		return f.run
	},
}

// buildFlags are the flags of ggui build: the ones that pick the target
// and the ones macFlags gathers for signing a macOS build.
type buildFlags struct {
	cfgPath, tgt, out                             *string
	universal, console, installer                 *bool
	installerPkg, format, tags, ldflags, renderer *string
	mac                                           macFlags
	isSet                                         func(string) bool
}

func newBuildFlags(fs *flag.FlagSet) *buildFlags {
	f := &buildFlags{isSet: func(name string) bool { return isSet(fs, name) }}
	f.cfgPath = fs.String("config", "", "read `file` instead of the ggui.toml found from the current directory")
	f.tgt = fs.String("target", defaultTarget(), "what to build: `macos`, windows, linux, web or native")
	f.out = fs.String("out", "", "the output `directory`, replacing build.output")
	f.universal = fs.Bool("universal", false, "with --target macos, build for Apple silicon and Intel and join them into one universal program")
	f.console = fs.Bool("console", false, "with --target windows, link for the console, which Windows opens a window of, replacing windows.console")
	f.installer = fs.Bool("installer", false, "also make a platform installer (Name.pkg on macOS, Name-Setup.exe on Windows); --installer=false skips any Go installer package or [macos.pkg]")
	f.installerPkg = fs.String("installer-pkg", "", "Go main `package` defining the installer with github.com/ironpark/ggui/installer, replacing build.installer and auto-discovery")
	f.format = fs.String("format", "", "with --target linux, what to make of the install tree: `formats` from tar, deb and rpm, comma-separated, replacing linux.formats")
	f.tags = fs.String("tags", "", "build `tags`, comma-separated, replacing build.tags")
	f.ldflags = fs.String("ldflags", "", "linker `flags` after -s -w, replacing build.ldflags")
	f.renderer = fs.String("renderer", "", "graphics driver implementation: `go` (default) or zig (native Metal/DX12/Vulkan), replacing build.renderer")
	f.mac = macFlags{
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
		isSet:        f.isSet,
		macOnlyFlags: []string{"identity", "p12-file", "p12-password-file", "notarize", "notary-profile", "api-key-file", "apple-id", "team-id", "dmg", "pkg"},
	}
	return f
}

// run is the build command once its flags are parsed.
func (f *buildFlags) run(ctx context.Context, pos, rest []string) error {
	if len(pos) > 1 || len(rest) > 0 {
		fmt.Fprintln(os.Stderr, "ggui build: takes one package")
		return errUsage
	}
	arg := ""
	if len(pos) == 1 {
		arg = pos[0]
	}
	proj, err := project.Open(".", arg, *f.cfgPath)
	if err != nil {
		return err
	}
	cfg := proj.Config
	b := &builder{proj: proj, out: *f.out, tags: cfg.Build.Tags, ldflags: cfg.Build.LDFlags}
	if f.isSet("tags") {
		b.tags = splitTags(*f.tags)
	}
	if f.isSet("ldflags") {
		b.ldflags = *f.ldflags
	}
	b.zig, b.tags, err = ziglib.ResolveRenderer(cfg.Build.Renderer, *f.renderer, f.isSet("renderer"), b.tags)
	if err != nil {
		return err
	}
	b.rendererFlagSet = f.isSet("renderer")
	b.universal = cfg.MacOS.Universal || *f.universal
	b.console = cfg.Windows.Console
	if f.isSet("console") {
		b.console = *f.console
	}
	b.installerSet = f.isSet("installer")
	b.installerVal = *f.installer
	b.installerPkgSet = f.isSet("installer-pkg")
	b.installerPkgFlag = *f.installerPkg
	lo, err := b.planTarget(ctx, *f.tgt)
	if err != nil {
		return err
	}
	if err := f.checkTarget(b, *f.tgt); err != nil {
		return err
	}
	b.mac = f.mac
	if b.main, err = resolveMain(ctx, ui.Stderr, proj, arg, lo); err != nil {
		return err
	}
	return b.produce(ctx, *f.tgt)
}

// produce runs the build for the target.
func (b *builder) produce(ctx context.Context, tgt string) error {
	switch tgt {
	case "macos":
		return b.macos(ctx)
	case "windows":
		return b.windows(ctx)
	case "linux":
		return b.linux(ctx)
	case "web":
		return b.web(ctx)
	default:
		return b.native(ctx)
	}
}

// planTarget works out the go command's environment for the target, and
// the architecture a Windows or Linux build is for.
func (b *builder) planTarget(ctx context.Context, tgt string) (gotool.ListOptions, error) {
	lo := gotool.ListOptions{Tags: b.tags}
	switch tgt {
	case "macos":
		lo.GOOS = "darwin"
	case "windows":
		// The architecture is resolved first, as the files the
		// package is built from, its .syso files among them,
		// depend on it.
		env, err := gotool.Env(ctx, b.proj.Root, []string{"GOOS=windows"}, "GOARCH")
		if err != nil {
			return lo, err
		}
		b.arch = env["GOARCH"]
		if !slices.Contains(windows.Arches, b.arch) {
			return lo, fmt.Errorf("GOARCH=%s: ggui builds Windows programs for %s; set GOARCH to one of them", b.arch, strings.Join(windows.Arches, ", "))
		}
		lo.GOOS, lo.GOARCH = "windows", b.arch
	case "linux":
		env, err := gotool.Env(ctx, b.proj.Root, []string{"GOOS=linux"}, "GOARCH")
		if err != nil {
			return lo, err
		}
		b.arch = env["GOARCH"]
		if !slices.Contains(linux.Arches, b.arch) {
			return lo, fmt.Errorf("GOARCH=%s: ggui packages Linux programs for %s; set GOARCH to one of them", b.arch, strings.Join(linux.Arches, ", "))
		}
		lo.GOOS, lo.GOARCH = "linux", b.arch
	case "web":
		lo.GOOS, lo.GOARCH = "js", "wasm"
	case "native":
	default:
		return lo, fmt.Errorf("unknown --target %q: use macos, windows, linux, web or native", tgt)
	}
	return lo, nil
}

// checkTarget refuses the flags that do not go with the target, and
// plans what a Linux build makes of its install tree.
func (f *buildFlags) checkTarget(b *builder, tgt string) error {
	if *f.universal && tgt != "macos" {
		return errors.New("--universal makes a macOS bundle: add --target macos")
	}
	if tgt != "macos" {
		for _, name := range f.mac.macOnlyFlags {
			if f.mac.isSet(name) {
				return fmt.Errorf("--%s is for a macOS build: add --target macos", name)
			}
		}
	}
	if tgt != "windows" && f.isSet("console") {
		return errors.New("--console is for a Windows build: add --target windows")
	}
	if (f.isSet("installer") || f.isSet("installer-pkg")) && tgt != "macos" && tgt != "windows" {
		return errors.New("--installer is for a macOS or Windows build: add --target macos or --target windows")
	}
	if f.isSet("installer-pkg") && f.isSet("installer") && !*f.installer {
		return errors.New("--installer-pkg and --installer=false disagree: pass one of them")
	}
	if tgt == "macos" && f.isSet("installer-pkg") && f.mac.isSet("pkg") && !*f.mac.pkg {
		return errors.New("--installer-pkg and --pkg=false disagree: pass one of them")
	}
	if tgt == "macos" && f.isSet("installer") && f.mac.isSet("pkg") && *f.installer != *f.mac.pkg {
		return errors.New("--installer and --pkg disagree: pass one of them")
	}
	b.formats = b.proj.Config.Linux.Formats
	if f.isSet("format") {
		if tgt != "linux" {
			return errors.New("--format is for a Linux build: add --target linux")
		}
		formats, err := parseFormats(*f.format)
		if err != nil {
			return err
		}
		b.formats = formats
	}
	return nil
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
	proj             *project.Project
	main             *project.Main
	out              string
	tags             []string
	ldflags          string
	zig              bool // build with ggfx's Zig native graphics driver
	rendererFlagSet  bool // --renderer was passed on the command line
	universal        bool
	mac              macFlags
	installerSet     bool   // --installer was passed on the command line
	installerVal     bool   // value of --installer
	installerPkgSet  bool   // --installer-pkg was passed on the command line
	installerPkgFlag string // value of --installer-pkg
	instResolved     bool
	instPkg          *installer.Package
	instDesc         *installer.Description
	console          bool     // link a Windows program for the console
	arch             string   // the GOARCH of a Windows or Linux build
	formats          []string // what a Linux build makes of its tree
}

// resolveInstaller discovers any Go installer package (from --installer-pkg,
// build.installer, or conventional ./cmd/installer or ./installer) and reads
// its build-time Description via GGUI_INSTALLER_DESCRIBE.
func (b *builder) resolveInstaller(ctx context.Context, tgt string) error {
	if b.instResolved {
		return nil
	}
	b.instResolved = true
	if b.installerSet && !b.installerVal {
		return nil
	}
	if tgt == "macos" && b.mac.isSet != nil && b.mac.isSet("pkg") && !*b.mac.pkg {
		return nil
	}
	mainDir := ""
	if b.main != nil {
		mainDir = b.main.Dir
	}
	pkg, err := installer.Discover(ctx, b.proj.Root, mainDir, b.proj.Config.Build.Installer, b.installerPkgFlag)
	if err != nil {
		return err
	}
	if pkg == nil {
		return nil
	}
	b.instPkg = pkg
	label := shortPath(pkg.Dir)
	if label == "" {
		label = pkg.Arg
	}
	var desc *installer.Description
	err = step(ctx, "Reading installer "+label, "Read installer "+label, func() error {
		var err error
		desc, err = installer.Describe(ctx, b.proj.Root, pkg, func(format string, args ...any) {
			ui.Stderr.Println(ui.Stderr.Dim(fmt.Sprintf(format, args...)))
		})
		return err
	})
	if err != nil {
		return err
	}
	b.instDesc = desc
	return nil
}

func (b *builder) wantMacOSInstaller() bool {
	if b.mac.isSet != nil && b.mac.isSet("pkg") {
		return *b.mac.pkg
	}
	if b.installerSet {
		return b.installerVal
	}
	if b.installerPkgSet {
		return true
	}
	if b.instDesc != nil && b.instDesc.MacOS.Enabled {
		return true
	}
	return b.proj.Config.MacOS.PKG.Make
}

func (b *builder) wantWindowsInstaller() bool {
	if b.installerSet {
		return b.installerVal
	}
	if b.installerPkgSet {
		return true
	}
	if b.instDesc != nil {
		return b.instDesc.Windows.Enabled
	}
	return false
}

// buildInstaller runs an installer.Builder as a step and logs any warnings.
func (b *builder) buildInstaller(ctx context.Context, ib installer.Builder, spec installer.Spec) (string, error) {
	out := ib.OutputPath(spec)
	var res installer.Result
	err := step(ctx, "Making "+filepath.Base(out), "Made "+filepath.Base(out), func() error {
		var err error
		res, err = ib.Build(ctx, spec)
		return err
	})
	if err != nil {
		return "", err
	}
	for _, w := range res.Warnings {
		ui.Stderr.Warn("%s", w)
	}
	return res.Path, nil
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
	pkgIdentity := m.PKG.SigningIdentity
	if b.instDesc != nil && b.instDesc.MacOS.SigningIdentity != "" {
		pkgIdentity = b.instDesc.MacOS.SigningIdentity
	}
	s = macos.Signing{
		Identity:     f.str("identity", f.identity, m.SigningIdentity),
		Entitlements: b.proj.Path(m.Entitlements),
		PKGIdentity:  pkgIdentity,
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

// buildIDVar is the variable of the ggui package the linker sets to the
// app's ID, which the program then knows itself by: on X11 its window
// class, which a desktop matches to the desktop entry of that name.
const buildIDVar = "github.com/ironpark/ggui.buildID"

// release returns the options every release build shares: stripped, the
// app's ID set in the ggui package, after build.ldflags so that it is
// the ID the files of a bundle are named for.
func (b *builder) release(output string) gotool.BuildOptions {
	ld := "-s -w"
	if b.ldflags != "" {
		ld += " " + b.ldflags
	}
	ld += " -X " + buildIDVar + "=" + b.main.ID
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
	goos, goarch := env["GOOS"], env["GOARCH"]
	name := b.main.Executable
	if goos == "windows" {
		name += ".exe"
	}
	out := filepath.Join(dir, name)
	o := b.release(out)
	if b.zig {
		if ziglib.Supported(goos, goarch) {
			if !slices.Contains(o.Tags, ziglib.NoEmbedTag) {
				if _, err := ziglib.Prebuilt(ctx, b.proj.Root, goos, goarch); err != nil {
					return err
				}
			}
			o.Tags = ziglib.WithTags(o.Tags, true)
		} else {
			ui.Stderr.Warn("the Zig renderer is not available for %s/%s; building with the Go renderer", goos, goarch)
			o.Tags = ziglib.WithoutTags(o.Tags)
		}
	}
	if err := b.build(ctx, o, goos+"/"+goarch); err != nil {
		return err
	}
	return report(out, goos+"/"+goarch)
}

func (b *builder) macos(ctx context.Context) error {
	if err := b.proj.CheckIcon(); err != nil {
		return err
	}
	dir, err := b.proj.OutDir(b.out)
	if err != nil {
		return err
	}
	if err := b.resolveInstaller(ctx, "macos"); err != nil {
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
	if b.wantMacOSInstaller() {
		if err := sg.CheckInstaller(ctx); err != nil {
			return err
		}
	}

	tmp, err := os.MkdirTemp("", "ggui-build-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	arch, err := b.macosArch(ctx)
	if err != nil {
		return err
	}
	var frameworks []string
	var useZig bool
	if b.zig {
		dylib, ok, err := b.macosZigDriver(ctx, tmp, arch)
		if err != nil {
			return err
		}
		if ok {
			useZig = true
			frameworks = append(frameworks, dylib)
		}
	}
	icon, err := b.icon(ctx, tmp)
	if err != nil {
		return err
	}
	bin, archLabel, err := b.macosBinary(ctx, tmp, arch, useZig)
	if err != nil {
		return err
	}
	app, err := b.assembleBundle(dir, icon, bin, frameworks)
	if err != nil {
		return err
	}
	artifacts, err := b.macosArtifacts(ctx, sg, app, dir, notarize)
	if err != nil {
		return err
	}
	return b.reportMacOS(ctx, sg, app, artifacts, notarize, archLabel)
}

func (b *builder) macosArch(ctx context.Context) (string, error) {
	if b.universal {
		return "universal", nil
	}
	env, err := gotool.Env(ctx, b.proj.Root, []string{"GOOS=darwin"}, "GOARCH")
	if err != nil {
		return "", err
	}
	return env["GOARCH"], nil
}

// macosZigDriver unpacks ggfx's prebuilt libggfx_driver.dylib for arch
// (joining arm64 and amd64 into a universal dylib when universal) into tmp.
func (b *builder) macosZigDriver(ctx context.Context, tmp, arch string) (string, bool, error) {
	dst := filepath.Join(tmp, ziglib.FileName("darwin"))
	if b.universal {
		var halves []string
		for _, a := range []string{"arm64", "amd64"} {
			h := filepath.Join(tmp, "libggfx_driver-"+a+".dylib")
			if err := ziglib.Extract(ctx, b.proj.Root, "darwin", a, h); err != nil {
				return "", false, err
			}
			halves = append(halves, h)
		}
		err := step(ctx, "Joining arm64 and amd64 Zig driver libraries", "Joined arm64 and amd64 Zig driver libraries", func() error {
			return macos.Lipo(dst, halves...)
		})
		if err != nil {
			return "", false, err
		}
		return dst, true, nil
	}
	if !ziglib.Supported("darwin", arch) {
		ui.Stderr.Warn("the Zig renderer is not available for darwin/%s; building with the Go renderer", arch)
		return "", false, nil
	}
	if err := ziglib.Extract(ctx, b.proj.Root, "darwin", arch, dst); err != nil {
		return "", false, err
	}
	return dst, true, nil
}

// macosBinary builds the program the bundle holds: for arm64 and amd64
// joined by lipo when the build is universal, else for arch. It returns
// the program's path and the architecture label built for.
func (b *builder) macosBinary(ctx context.Context, tmp, arch string, useZig bool) (bin, archLabel string, err error) {
	bin = filepath.Join(tmp, b.main.Executable)
	releaseOpts := func(out string) gotool.BuildOptions {
		o := b.release(out)
		if useZig {
			o.Tags = ziglib.WithTags(o.Tags, false)
		} else if b.zig {
			o.Tags = ziglib.WithoutTags(o.Tags)
		}
		return o
	}
	if b.universal {
		var halves []string
		for _, a := range []string{"arm64", "amd64"} {
			o := releaseOpts(bin + "-" + a)
			o.GOOS, o.GOARCH = "darwin", a
			if err := b.build(ctx, o, "darwin/"+a); err != nil {
				return "", "", err
			}
			halves = append(halves, o.Output)
		}
		err := step(ctx, "Joining arm64 and amd64 into a universal program", "Joined arm64 and amd64 into a universal program", func() error {
			return macos.Lipo(bin, halves...)
		})
		if err != nil {
			return "", "", err
		}
		return bin, "arm64 + amd64", nil
	}
	o := releaseOpts(bin)
	o.GOOS = "darwin"
	if err := b.build(ctx, o, "darwin/"+arch); err != nil {
		return "", "", err
	}
	return bin, arch, nil
}

// assembleBundle puts the program in Name.app in dir, with the icon,
// any bundled framework libraries, and the Info.plist keys [macos.plist]
// and the translation catalogs give, and returns the bundle's path.
func (b *builder) assembleBundle(dir, icon, bin string, frameworks []string) (string, error) {
	cfg := b.proj.Config
	appName := b.main.Name + ".app"
	extra, langs := plistExtra(b.proj.Root, cfg.MacOS.Plist)
	if langs != nil {
		logf("Info.plist names the localizations %s, from the translation catalogs", strings.Join(langs, ", "))
	}
	assemble := ui.Stderr.Step("Assembling " + appName)
	app, warnings, err := macos.Assemble(macos.Bundle{
		Name: b.main.Name, Executable: b.main.Executable, ID: b.main.ID,
		Version:              cfg.App.Version,
		MinimumSystemVersion: cfg.MacOS.MinimumSystemVersion,
		Category:             cfg.MacOS.Category,
		Extra:                extra,
		Binary:               bin,
		Frameworks:           frameworks,
		Icon:                 icon,
		IconName:             b.proj.Path(cfg.App.Icon),
		IconInset:            cfg.MacOS.IconInset,
		OutDir:               dir,
	})
	if err != nil {
		assemble.Fail("Assembling " + appName + " failed")
		return "", err
	}
	assemble.Done("Assembled " + appName)
	for _, w := range warnings {
		ui.Stderr.Warn("%s", w)
	}
	if b.main.ID != cfg.App.ID {
		ui.Stderr.Warn("app.id is not set, so the bundle is %s; set it in ggui.toml before shipping, since macOS keys settings and permissions to it", b.main.ID)
	}
	return app, nil
}

// macosArtifacts signs the bundle, notarizes it when asked, and makes
// the disk image and the installer package when their flags or the
// configuration ask for them. It returns the bundle and what was made
// beside it, in order.
func (b *builder) macosArtifacts(ctx context.Context, sg *macos.Signer, app, dir string, notarize bool) ([]string, error) {
	if _, err := sign(ctx, sg, app); err != nil {
		return nil, err
	}
	if sg.AdHoc() {
		ui.Stderr.Note("%s", ui.Stderr.Dim("which macOS runs as it is, but opens once downloaded only after Open Anyway in System Settings; set macos.signing_identity and notarize to ship it to others"))
	}
	artifacts := []string{app}
	if notarize {
		if err := b.notarize(ctx, sg, app); err != nil {
			return nil, err
		}
	}
	cfg := b.proj.Config
	if b.mac.boolean("dmg", b.mac.dmg, cfg.MacOS.DMG.Make) {
		out, err := b.macosDMG(ctx, sg, app, dir, notarize)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, out)
	}
	if b.wantMacOSInstaller() {
		out, err := b.macosPKG(ctx, sg, app, dir, notarize)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, out)
	}
	return artifacts, nil
}

// macosDMG makes Name.dmg holding the bundle, signed and notarized as
// the bundle was unless the signature is ad hoc.
func (b *builder) macosDMG(ctx context.Context, sg *macos.Signer, app, dir string, notarize bool) (string, error) {
	d := b.proj.Config.MacOS.DMG
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
		return "", err
	}
	// An ad hoc signature on a disk image means nothing to another
	// Mac, and the app inside is signed already.
	if !sg.AdHoc() {
		if _, err := sign(ctx, sg, out); err != nil {
			return "", err
		}
		if notarize {
			if err := b.notarize(ctx, sg, out); err != nil {
				return "", err
			}
		}
	}
	return out, nil
}

// macosPKG makes the installer Name.pkg, signed and notarized as the
// bundle was; an ad hoc app leaves it unsigned, which macOS opens only
// after a warning.
func (b *builder) macosPKG(ctx context.Context, sg *macos.Signer, app, dir string, notarize bool) (string, error) {
	cfg := b.proj.Config
	m := cfg.MacOS
	installLoc := m.PKG.InstallLocation
	scripts := m.PKG.Scripts
	licPath := b.proj.Path(m.PKG.License)
	licText := ""
	if b.instDesc != nil {
		if b.instDesc.MacOS.InstallLocation != "" {
			installLoc = b.instDesc.MacOS.InstallLocation
		}
		if b.instDesc.MacOS.Scripts != "" {
			scripts = b.instDesc.MacOS.Scripts
		}
		if b.instDesc.MacOS.License != "" {
			licText = b.instDesc.MacOS.License
			licPath = ""
		} else if b.instDesc.MacOS.LicenseFile != "" {
			licPath = b.proj.Path(b.instDesc.MacOS.LicenseFile)
		}
	}
	spec := installer.Spec{
		Name:        b.main.Name,
		Executable:  b.main.Executable,
		ID:          b.main.ID,
		Version:     cfg.App.Version,
		ProjectDir:  b.proj.Root,
		Artifact:    app,
		OutDir:      dir,
		License:     licPath,
		LicenseText: licText,
		MacOS: installer.MacOSOptions{
			MinOS:           m.MinimumSystemVersion,
			InstallLocation: installLoc,
			Scripts:         b.proj.Path(scripts),
		},
	}
	out, err := b.buildInstaller(ctx, macos.PKGBuilder{}, spec)
	if err != nil {
		return "", err
	}
	signed, err := sign(ctx, sg, out)
	if err != nil {
		return "", err
	}
	if !signed {
		ui.Stderr.Warn("The installer is not signed, since the app is signed ad hoc; macOS opens an unsigned installer only after a warning")
	} else if notarize {
		if err := b.notarize(ctx, sg, out); err != nil {
			return "", err
		}
	}
	return out, nil
}

// reportMacOS verifies what was built when the signature is not ad hoc,
// and prints the bundle and what was made beside it.
func (b *builder) reportMacOS(ctx context.Context, sg *macos.Signer, app string, artifacts []string, notarize bool, arch string) error {
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

// icon returns the PNG the icons are made from, "" for none: app.icon
// itself, or for an SVG the PNG drawn from it in dir by the project's
// ggfx.
func (b *builder) icon(ctx context.Context, dir string) (string, error) {
	path := b.proj.Path(b.proj.Config.App.Icon)
	if path == "" || !svgicon.Is(path) {
		return path, nil
	}
	out := filepath.Join(dir, "icon.png")
	log := ui.Stderr
	name := filepath.Base(path)
	size := fmt.Sprintf("%d×%d", svgicon.Size, svgicon.Size)
	return out, step(ctx, "Drawing "+name+" at "+size+" with the project's ggfx", "Drew "+name+" at "+size, func() error {
		return svgicon.Draw(ctx, svgicon.Options{
			Dir: b.proj.Root, SVG: path, Name: b.proj.Config.App.Icon, Out: out,
			Logf: func(format string, args ...any) { log.Println(log.Dim(fmt.Sprintf(format, args...))) },
		})
	})
}

// plistExtra returns the Info.plist keys [macos.plist] sets, with
// CFBundleLocalizations, when it does not set that, made from the
// languages of the translation catalogs below root, found as ggui i18n
// check finds them: the development language, en, first, then the rest
// sorted. macOS shows the app's menus and dialogs, the ones it draws, in
// a language only when the bundle names it. It also returns the languages
// it named, or nil when it named none.
func plistExtra(root string, plist map[string]any) (map[string]any, []string) {
	if _, set := plist["CFBundleLocalizations"]; set {
		return plist, nil
	}
	// A Go file that does not parse fails the build; the catalogs found
	// before it are as good as any.
	src, _ := i18n.Find(root, i18n.DefaultGlobs)
	if src == nil {
		return plist, nil
	}
	found := i18n.Langs(src.Catalogs)
	if len(found) == 0 {
		return plist, nil
	}
	found = slices.DeleteFunc(found, func(l string) bool { return l == "en" })
	slices.Sort(found)
	langs := append([]string{"en"}, found...)
	extra := maps.Clone(plist)
	if extra == nil {
		extra = map[string]any{}
	}
	list := make([]any, len(langs))
	for i, l := range langs {
		list[i] = l
	}
	extra["CFBundleLocalizations"] = list
	return extra, langs
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
	if err := windows.CheckFileName(b.main.Name); err != nil {
		return err
	}
	dir, err := b.proj.OutDir(b.out)
	if err != nil {
		return err
	}
	if err := b.resolveInstaller(ctx, "windows"); err != nil {
		return err
	}
	exe := b.main.Name + ".exe"
	log := ui.Stderr
	useZig := false
	if b.zig {
		if ziglib.Supported("windows", b.arch) {
			if _, err := ziglib.Prebuilt(ctx, b.proj.Root, "windows", b.arch); err != nil {
				return err
			}
			useZig = true
		} else {
			log.Warn("the Zig renderer is not available for windows/%s; building with the Go renderer", b.arch)
		}
	}
	tmp, err := os.MkdirTemp("", "ggui-build-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	icon, err := b.icon(ctx, tmp)
	if err != nil {
		return err
	}

	cleanup, err := b.windowsResources(ctx, exe, icon)
	if err != nil {
		return err
	}
	defer cleanup()

	o := b.release(filepath.Join(dir, exe))
	o.GOOS, o.GOARCH = "windows", b.arch
	if useZig {
		o.Tags = ziglib.WithTags(o.Tags, false)
	} else if b.zig {
		o.Tags = ziglib.WithoutTags(o.Tags)
	}
	if !b.console {
		// The GUI subsystem: Windows opens no console for the program,
		// and its standard streams go nowhere unless redirected.
		o.LDFlags += " -H=windowsgui"
	}
	if err := b.build(ctx, o, "windows/"+b.arch); err != nil {
		return err
	}
	var extraFiles []string
	if useZig {
		dll := filepath.Join(dir, ziglib.FileName("windows"))
		if err := ziglib.Extract(ctx, b.proj.Root, "windows", b.arch, dll); err != nil {
			return err
		}
		extraFiles = append(extraFiles, dll)
		log.Note("%s", log.Dim("bundled Zig driver library "+shortPath(dll)+" beside "+exe))
	}
	if runtime.GOOS != "windows" && b.arch == runtime.GOARCH && b.arch != "amd64" && os.Getenv("GOARCH") == "" {
		log.Note("%s", log.Dim("for "+b.arch+", this machine's architecture; most PCs need GOARCH=amd64"))
	}
	var setupPath string
	if b.wantWindowsInstaller() {
		cfg := b.proj.Config
		desc := cmpOr(cfg.Windows.Description, cfg.Linux.Description)
		publisher := cfg.Windows.Company
		copyright := cfg.Windows.Copyright
		homepage := cfg.Linux.Homepage
		licPath, licText := "", ""
		winOpts := installer.WindowsOptions{
			Scope:             installer.ScopeUser,
			StartMenuShortcut: true,
			DesktopShortcut:   false,
			LaunchAfter:       true,
		}
		if b.instDesc != nil {
			if b.instDesc.Description != "" {
				desc = b.instDesc.Description
			}
			if b.instDesc.Copyright != "" {
				copyright = b.instDesc.Copyright
			}
			if b.instDesc.Windows.Publisher != "" {
				publisher = b.instDesc.Windows.Publisher
			}
			if b.instDesc.Windows.URL != "" {
				homepage = b.instDesc.Windows.URL
			}
			if b.instDesc.Windows.Scope != "" {
				winOpts.Scope = b.instDesc.Windows.Scope
			}
			winOpts.InstallLocation = b.instDesc.Windows.InstallDir
			winOpts.StartMenuShortcut = b.instDesc.Windows.StartMenuShortcut
			winOpts.DesktopShortcut = b.instDesc.Windows.DesktopShortcut
			winOpts.LaunchAfter = b.instDesc.Windows.LaunchAfter
			if b.instDesc.Windows.License != "" {
				licText = b.instDesc.Windows.License
			} else if b.instDesc.Windows.LicenseFile != "" {
				licPath = b.proj.Path(b.instDesc.Windows.LicenseFile)
			}
		}
		spec := installer.Spec{
			Name:        b.main.Name,
			Executable:  b.main.Executable,
			ID:          b.main.ID,
			Version:     cfg.App.Version,
			Arch:        b.arch,
			ProjectDir:  b.proj.Root,
			Artifact:    o.Output,
			ExtraFiles:  extraFiles,
			OutDir:      dir,
			Package:     b.instPkg,
			Description: desc,
			Publisher:   publisher,
			Copyright:   copyright,
			Homepage:    homepage,
			Icon:        icon,
			License:     licPath,
			LicenseText: licText,
			Windows:     winOpts,
			Logf:        func(format string, args ...any) { log.Println(log.Dim(fmt.Sprintf(format, args...))) },
		}
		setupPath, err = b.buildInstaller(ctx, windows.SetupBuilder{}, spec)
		if err != nil {
			return err
		}
	}
	if err := report(o.Output, "windows/"+b.arch); err != nil {
		return err
	}
	if setupPath != "" {
		return report(setupPath, "windows/"+b.arch)
	}
	return nil
}

// windowsResources writes the .syso the go command links the program's
// icon, version information and manifest from, in the main package's
// directory, for the build only; a package that links resources of its
// own keeps them.
func (b *builder) windowsResources(ctx context.Context, exe, icon string) (func(), error) {
	cfg := b.proj.Config
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
		return func() {}, nil
	}
	w := cfg.Windows
	r := windows.Resources{
		Arch: b.arch, Name: b.main.Name, Executable: b.main.Executable, ID: b.main.ID,
		Version:     cfg.App.Version,
		Description: w.Description, Company: w.Company, Copyright: w.Copyright,
		Icon:     icon,
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
		return nil, err
	}
	for _, w := range warnings {
		log.Warn("%s", w)
	}
	if leftover {
		log.Note("%s", log.Dim("replacing the "+filepath.Base(path)+" a build that was stopped left behind"))
	}
	log.Note("%s", log.Dim("linked from "+shortPath(path)+", which is removed after the build"))
	// Removed however the build ends, failed or interrupted too: the
	// file belongs to this build, not to the sources.
	return func() {
		if err := remove(); err != nil {
			log.Warn("removing %s: %v; delete it before building for another target", path, err)
		}
	}, nil
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
	if b.zig {
		if b.rendererFlagSet {
			ui.Stderr.Warn("web builds always use WebGL; ignoring --renderer zig")
		}
		o.Tags = ziglib.WithoutTags(o.Tags)
	}
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
	_, err = fmt.Println(dir)
	return err
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
	_, err := fmt.Fprintln(reportOut, path)
	return err
}
