package main

import (
	"context"
	"crypto/rand"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/ironpark/ggui/cmd/ggui/internal/config"
	"github.com/ironpark/ggui/cmd/ggui/internal/gotool"
	"github.com/ironpark/ggui/cmd/ggui/internal/linux"
	"github.com/ironpark/ggui/cmd/ggui/internal/project"
	"github.com/ironpark/ggui/cmd/ggui/internal/release"
	"github.com/ironpark/ggui/cmd/ggui/internal/release/choco"
	"github.com/ironpark/ggui/cmd/ggui/internal/release/github"
	"github.com/ironpark/ggui/cmd/ggui/internal/release/homebrew"
	"github.com/ironpark/ggui/cmd/ggui/internal/ui"
	"github.com/ironpark/ggui/cmd/ggui/internal/windows"
	"github.com/ironpark/ggui/cmd/ggui/internal/ziglib"
)

// reportOut is where report prints the path of what it built: standard
// output, where a script takes it, except during ggui release, whose
// standard output is the table of what it publishes.
var reportOut io.Writer = os.Stdout

var releaseCmd = &command{
	name:    "release",
	args:    "[dir]",
	summary: "build the release files and publish them to GitHub, Homebrew and Chocolatey",
	help: `Release builds app.version of the application for each target and
architecture, collects the files in build/release/VERSION, and publishes
them.

  macos    Name.dmg when [macos.dmg] asks for one, else a zip of Name.app;
           one universal build when both amd64 and arm64 are built.
           Signed and notarized as [macos] says.
  windows  a zip of Name.exe for each architecture.
  linux    the tarball and packages linux.formats names, for each
           architecture.

SHA256SUMS lists every file's checksum. Then, by --publish or
release.publish:

  github    a release at the tag v<version>, which GitHub makes at this
            commit, with the files: made as a draft, the files uploaded,
            and only then published. The token is $GITHUB_TOKEN,
            $GH_TOKEN or gh auth token's.
  homebrew  the cask Casks/<token>.rb, committed to release.homebrew.tap.
  choco     the Chocolatey package, pushed with $CHOCOLATEY_API_KEY.

The work tree must be clean, and the tag, if it exists, at this commit.
--dry-run builds and writes everything, the cask and the Chocolatey
package too, and prints the requests it would send instead of sending
them. docs/releasing.md has the details.`,
	flags: func(fs *flag.FlagSet) func(context.Context, []string, []string) error {
		o := releaseOptions{}
		fs.StringVar(&o.configPath, "config", "", "read `file` instead of the ggui.toml found from the project directory")
		fs.StringVar(&o.version, "version", "", "release `version` X.Y.Z, replacing app.version")
		targets := fs.String("targets", "", "the `targets` to build, comma-separated from macos, windows and linux, replacing release.targets")
		arches := fs.String("arches", "", "the `architectures` to build for, comma-separated, replacing release.arches")
		publish := fs.String("publish", "", "where to publish: `channels` from github, homebrew and choco, comma-separated, replacing release.publish; \"none\" publishes nothing")
		fs.BoolVar(&o.draft, "draft", false, "leave the GitHub release a draft")
		fs.BoolVar(&o.prerelease, "prerelease", false, "mark the GitHub release as a prerelease")
		fs.StringVar(&o.notes, "notes", "", "the release's description, from the Markdown `file`, replacing release.notes")
		fs.BoolVar(&o.dryRun, "dry-run", false, "build and write everything, publish nothing, and print the requests it would send")
		fs.BoolVar(&o.allowDirty, "allow-dirty", false, "release from a work tree with changes not committed")
		fs.BoolVar(&o.replaceAssets, "replace-assets", false, "upload to a GitHub release already published, replacing its files")
		fs.StringVar(&o.out, "out", "", "the output `directory`, replacing build.output; the files go in its release/VERSION")
		renderer := fs.String("renderer", "", "graphics driver implementation: `go` (default) or zig (native Metal/DX12/Vulkan), replacing build.renderer")
		return func(ctx context.Context, pos, rest []string) error {
			if len(pos) > 1 || len(rest) > 0 {
				fmt.Fprintln(os.Stderr, "ggui release: takes one directory")
				return errUsage
			}
			if len(pos) == 1 {
				o.dir = pos[0]
			}
			o.targetsSet, o.archesSet, o.publishSet = isSet(fs, "targets"), isSet(fs, "arches"), isSet(fs, "publish")
			o.targetsFlag, o.archesFlag, o.publishFlag = *targets, *arches, *publish
			o.rendererSet, o.rendererFlag = isSet(fs, "renderer"), *renderer
			return runRelease(ctx, o, os.Stdout, os.Getenv)
		}
	},
}

type releaseOptions struct {
	dir, configPath, version, notes, out string
	draft, prerelease, dryRun            bool
	allowDirty, replaceAssets            bool

	targetsSet, archesSet, publishSet, rendererSet     bool
	targetsFlag, archesFlag, publishFlag, rendererFlag string
}

// list reads a comma-separated flag of names from allowed, each once.
func list(flagName, s string, allowed []string) ([]string, error) {
	var out []string
	for v := range strings.SplitSeq(s, ",") {
		v = strings.ToLower(strings.TrimSpace(v))
		switch {
		case v == "":
		case !slices.Contains(allowed, v):
			return nil, fmt.Errorf("--%s %q: choose from %s", flagName, v, strings.Join(allowed, ", "))
		case !slices.Contains(out, v):
			out = append(out, v)
		}
	}
	return out, nil
}

var versionRE = regexp.MustCompile(`^[0-9]+(\.[0-9]+){0,2}$`)

// releasePlan is a release worked out from the configuration and flags,
// before anything is built.
type releasePlan struct {
	proj      *project.Project
	arg       string
	version   string
	tag       string
	targets   []string
	arches    []string
	publish   []string
	tags      []string
	zig       bool
	repo      string // owner/repo, or "" when unknown in a dry run
	api, web  string
	head      string // the commit released, or "" outside git in a dry run
	dir       string // build/release/VERSION
	slug      string // the app's name in file names
	main      *project.Main
	assets    []release.Asset
	sums      release.Asset
	cask      string // the cask's path, when one was written
	caskToken string
	nupkg     string // the .nupkg's path, when one was made
	chocoID   string
	notesFile string // the release's description, from --notes or release.notes
}

func (p *releasePlan) publishes(ch string) bool { return slices.Contains(p.publish, ch) }

func runRelease(ctx context.Context, o releaseOptions, stdout io.Writer, getenv func(string) string) error {
	proj, err := project.Open(".", o.dir, o.configPath)
	if err != nil {
		return err
	}
	p, err := newReleasePlan(proj, o)
	if err != nil {
		return err
	}
	gh, err := p.checkRelease(ctx, o, getenv)
	if err != nil {
		return err
	}

	out, err := proj.OutDir(o.out)
	if err != nil {
		return err
	}
	p.dir = filepath.Join(out, "release", p.version)
	if err := os.RemoveAll(p.dir); err != nil {
		return err
	}
	if err := os.MkdirAll(p.dir, 0o755); err != nil {
		return err
	}
	work, err := os.MkdirTemp(out, ".release-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)

	if err := p.build(ctx, work); err != nil {
		return err
	}
	all, body, err := p.collect(stdout)
	if err != nil {
		return err
	}
	pub := github.Publish{
		Repo: p.repo, Tag: p.tag, Commitish: p.head, Name: p.main.Name + " " + p.version, Body: body,
		Draft: o.draft, Prerelease: o.prerelease, ReplaceAssets: o.replaceAssets, Assets: all,
		Progress: func(format string, args ...any) { logf(format, args...) },
	}
	if o.dryRun {
		p.printPlan(stdout, pub)
		return nil
	}
	return p.publishAll(ctx, gh, pub, getenv)
}

// newReleasePlan works out the release from the configuration and the
// flags: the version, the tag, the targets, architectures and channels
// to build and publish.
func newReleasePlan(proj *project.Project, o releaseOptions) (*releasePlan, error) {
	cfg := &proj.Config
	r := cfg.Release
	p := &releasePlan{proj: proj, arg: o.dir, version: cfg.App.Version, targets: r.Targets, arches: r.Arches, publish: r.Publish}
	var err error
	if o.version != "" {
		p.version = strings.TrimPrefix(o.version, "v")
	}
	if !versionRE.MatchString(p.version) {
		return nil, fmt.Errorf("version %q must be one to three numbers separated by dots, such as 1.2.0, as app.version is", p.version)
	}
	// The builds stamp the version in: Info.plist, the Windows version
	// information, the Linux packages.
	cfg.App.Version = p.version
	p.tag = r.TagPrefix + p.version
	if o.targetsSet {
		if p.targets, err = list("targets", o.targetsFlag, config.ReleaseTargets); err != nil {
			return nil, err
		}
	}
	if o.archesSet {
		if p.arches, err = list("arches", o.archesFlag, config.ReleaseArches); err != nil {
			return nil, err
		}
	}
	if o.publishSet {
		if strings.TrimSpace(o.publishFlag) == "none" {
			p.publish = nil
		} else if p.publish, err = list("publish", o.publishFlag, config.ReleaseChannels); err != nil {
			return nil, err
		}
	}
	if len(p.targets) == 0 || len(p.arches) == 0 {
		return nil, errors.New("nothing to build: name at least one target and one architecture")
	}
	p.zig, p.tags, err = ziglib.ResolveRenderer(cfg.Build.Renderer, o.rendererFlag, o.rendererSet, cfg.Build.Tags)
	if err != nil {
		return nil, err
	}
	p.notesFile = o.notes
	if p.notesFile == "" && r.Notes != "" {
		p.notesFile = proj.Path(r.Notes)
	}
	if err := p.checkChannels(o); err != nil {
		return nil, err
	}
	return p, nil
}

// checkRelease checks what can be checked before the build is, since a
// build takes minutes and a notarized one longer: the repository, the
// commit and the tag, the GitHub token, the Chocolatey key.
func (p *releasePlan) checkRelease(ctx context.Context, o releaseOptions, getenv func(string) string) (*github.Client, error) {
	p.api = strings.TrimRight(p.proj.Config.Release.GitHubAPI, "/")
	p.web = github.WebURL(p.api)
	if err := p.checkGit(ctx, o); err != nil {
		return nil, err
	}
	var gh *github.Client
	var err error
	if !o.dryRun && (p.publishes("github") || p.publishes("homebrew")) {
		if gh, err = p.preflight(ctx, o, getenv); err != nil {
			return nil, err
		}
	}
	if !o.dryRun && p.publishes("choco") && getenv("CHOCOLATEY_API_KEY") == "" {
		return nil, errors.New("publishing to Chocolatey needs $CHOCOLATEY_API_KEY, your chocolatey.org account's API key")
	}
	return gh, nil
}

// collect checks the builds made something, hashes and lists the files,
// writes the cask and the Chocolatey package when asked, and returns the
// assets and the release's description.
func (p *releasePlan) collect(stdout io.Writer) ([]release.Asset, string, error) {
	log := ui.Stderr
	if len(p.assets) == 0 {
		return nil, "", errors.New("the builds made nothing to release")
	}
	for i := range p.assets {
		if err := p.assets[i].Hash(); err != nil {
			return nil, "", err
		}
	}
	var err error
	if p.sums, err = release.WriteSums(p.dir, p.assets); err != nil {
		return nil, "", err
	}
	all := append(slices.Clone(p.assets), p.sums)
	log.Success("%s %s: %d files in %s", p.main.Name, p.version, len(all), log.Cyan(shortPath(p.dir)))
	release.Summary(stdout, all)

	if err := p.writeCask(); err != nil {
		return nil, "", err
	}
	if err := p.writeChoco(); err != nil {
		return nil, "", err
	}
	body, err := p.notes(p.notesFile, all)
	if err != nil {
		return nil, "", err
	}
	return all, body, nil
}

// checkChannels checks the channels can be published to with what is
// built.
func (p *releasePlan) checkChannels(o releaseOptions) error {
	r := p.proj.Config.Release
	if p.publishes("homebrew") {
		if r.Homebrew.Tap == "" {
			return errors.New(`publishing to Homebrew needs release.homebrew.tap, "owner/homebrew-name"`)
		}
		if !slices.Contains(p.targets, "macos") {
			return errors.New("publishing to Homebrew needs the macos target")
		}
	}
	if p.publishes("choco") {
		if !slices.Contains(p.targets, "windows") || !slices.Contains(p.arches, "amd64") && !slices.Contains(p.arches, "386") {
			return errors.New("publishing to Chocolatey needs the windows target for amd64 or 386")
		}
	}
	if o.draft && (p.publishes("homebrew") || p.publishes("choco")) {
		return errors.New("--draft leaves the GitHub release's files private, which the cask and the Chocolatey package download: publish to github alone, or drop --draft")
	}
	if (p.publishes("homebrew") || p.publishes("choco")) && !p.publishes("github") {
		ui.Stderr.Warn("the cask and the Chocolatey package download from the GitHub release %s, which this run does not publish: it must exist with these files", p.tag)
	}
	return nil
}

// checkGit finds the repository and the commit, and checks the tree is
// clean and the tag, if made, is at the commit. In a dry run what fails
// is said and passed over.
func (p *releasePlan) checkGit(ctx context.Context, o releaseOptions) error {
	log := ui.Stderr
	soft := func(err error) error {
		if o.dryRun {
			log.Warn("%v (a real run stops here)", err)
			return nil
		}
		return err
	}
	g := release.Git{Dir: p.proj.Root}
	needGitHub := p.publishes("github") || p.publishes("homebrew") || p.publishes("choco")

	p.repo = p.proj.Config.Release.GitHub
	if err := p.gitHead(ctx, g, needGitHub, soft); err != nil {
		return err
	}
	if p.head == "" {
		return nil
	}
	if err := p.gitRepo(ctx, g, needGitHub, soft); err != nil {
		return err
	}
	if err := p.gitDirty(ctx, g, o, soft); err != nil {
		return err
	}
	if err := p.gitLocalTag(ctx, g, soft); err != nil {
		return err
	}
	if !needGitHub {
		return nil
	}
	return p.gitRemoteTag(ctx, g, soft)
}

// gitHead finds the commit released, or says why there is none; p.head
// stays "" outside a repository.
func (p *releasePlan) gitHead(ctx context.Context, g release.Git, needGitHub bool, soft func(error) error) error {
	head, err := g.Head(ctx)
	if errors.Is(err, release.ErrNotRepo) {
		if p.repo == "" && needGitHub {
			if err := soft(errors.New("not in a git repository, and release.github is not set: ggui release publishes a commit of a GitHub repository")); err != nil {
				return err
			}
			p.repo = "OWNER/REPO"
		}
		if needGitHub && p.publishes("github") {
			return soft(errors.New("not in a git repository: a GitHub release is made at a commit"))
		}
		return nil
	} else if err != nil {
		return err
	}
	p.head = head
	return nil
}

// gitRepo finds the repository released to when none is configured.
func (p *releasePlan) gitRepo(ctx context.Context, g release.Git, needGitHub bool, soft func(error) error) error {
	if p.repo != "" || !needGitHub {
		return nil
	}
	origin, err := g.Origin(ctx)
	if err != nil {
		if err := soft(fmt.Errorf("no repository to release to: set release.github, or add an origin remote (%w)", err)); err != nil {
			return err
		}
		p.repo = "OWNER/REPO"
		return nil
	}
	_, repo, err := release.ParseRepo(origin)
	if err != nil {
		if err := soft(fmt.Errorf("%w: set release.github", err)); err != nil {
			return err
		}
		p.repo = "OWNER/REPO"
		return nil
	}
	p.repo = repo
	return nil
}

// gitDirty checks the work tree is clean, or says what is not committed.
func (p *releasePlan) gitDirty(ctx context.Context, g release.Git, o releaseOptions, soft func(error) error) error {
	dirty, err := g.Dirty(ctx)
	if err != nil {
		return err
	}
	if len(dirty) == 0 || o.allowDirty {
		return nil
	}
	shown := dirty[:min(len(dirty), 5)]
	more := ""
	if len(dirty) > len(shown) {
		more = fmt.Sprintf("\n  and %d more", len(dirty)-len(shown))
	}
	return soft(fmt.Errorf("the work tree has changes not committed, which the release would not be the commit of; commit them, or give --allow-dirty:\n  %s%s", strings.Join(shown, "\n  "), more))
}

// gitLocalTag checks a tag already made here is at the commit released.
func (p *releasePlan) gitLocalTag(ctx context.Context, g release.Git, soft func(error) error) error {
	sha, err := g.LocalTag(ctx, p.tag)
	if err != nil {
		return err
	}
	if sha == "" || sha == p.head {
		return nil
	}
	return soft(fmt.Errorf("the tag %s is at %s here, not at this commit %s: release from that commit, or bump the version", p.tag, short(sha), short(p.head)))
}

// gitRemoteTag checks the tag on origin is at the commit released, when
// the release publishes.
func (p *releasePlan) gitRemoteTag(ctx context.Context, g release.Git, soft func(error) error) error {
	sha, err := g.RemoteTag(ctx, p.tag)
	switch {
	case err != nil:
		return soft(fmt.Errorf("checking origin for the tag %s: %w", p.tag, err))
	case sha != "" && sha != p.head:
		return soft(fmt.Errorf("the tag %s is at %s on origin, not at this commit %s: release from that commit, or bump the version", p.tag, short(sha), short(p.head)))
	}
	return nil
}

func short(sha string) string { return sha[:min(len(sha), 12)] }

// preflight checks GitHub before the build: the token, that the commit
// is pushed, that the release is not published already.
func (p *releasePlan) preflight(ctx context.Context, o releaseOptions, getenv func(string) string) (*github.Client, error) {
	if t := strings.TrimSpace(getenv("HOMEBREW_TAP_TOKEN")); t != "" && !p.publishes("github") {
		return github.New(p.api, release.NewToken(t, "$HOMEBREW_TAP_TOKEN")), nil
	}
	tok, err := release.GitHubToken(ctx, getenv, github.Host(p.api))
	if err != nil {
		return nil, err
	}
	gh := github.New(p.api, tok)
	gh.Logf = func(format string, args ...any) { ui.Stderr.Warn(format, args...) }
	if !p.publishes("github") {
		return gh, nil
	}
	ok, err := gh.CommitExists(ctx, p.repo, p.head)
	if err != nil {
		return nil, fmt.Errorf("checking %s on GitHub (%v): %w", p.repo, tok, err)
	}
	if !ok {
		return nil, fmt.Errorf("%s has no commit %s, which the release is made at: push it first, git push origin HEAD", p.repo, short(p.head))
	}
	rel, err := gh.FindRelease(ctx, p.repo, p.tag)
	if err != nil {
		return nil, err
	}
	if rel != nil && !rel.Draft && !o.replaceAssets {
		return nil, fmt.Errorf("the release %s of %s is published already (%s): bump app.version, or give --replace-assets to replace its files", p.tag, p.repo, rel.HTMLURL)
	}
	return gh, nil
}

// releaseArch is one build of a target.
type releaseArch struct {
	target, arch string // arch is "universal" for a macOS build of both
}

// builds lists the builds to make, saying which combinations a target
// has no build for.
func (p *releasePlan) builds() []releaseArch {
	var out []releaseArch
	for _, t := range p.targets {
		var have []string
		switch t {
		case "macos":
			have = []string{"amd64", "arm64"}
		case "windows":
			have = windows.Arches
		case "linux":
			have = linux.Arches
		}
		var arches []string
		for _, a := range p.arches {
			if slices.Contains(have, a) {
				arches = append(arches, a)
			} else {
				ui.Stderr.Note("%s", ui.Stderr.Dim("no "+t+" build for "+a))
			}
		}
		// Both macOS architectures join into one universal binary via pure-Go lipo.
		if t == "macos" && slices.Contains(arches, "amd64") && slices.Contains(arches, "arm64") {
			arches = []string{"universal"}
		}
		for _, a := range arches {
			out = append(out, releaseArch{t, a})
		}
	}
	return out
}

// build makes each build in its own directory under work and moves what
// it publishes to p.dir under its release name.
func (p *releasePlan) build(ctx context.Context, work string) error {
	saved := reportOut
	reportOut = io.Discard
	defer func() { reportOut = saved }()

	for _, ra := range p.builds() {
		if err := p.buildArch(ctx, work, ra); err != nil {
			return err
		}
	}
	return nil
}

// buildArch makes one build of the release in its own directory under
// work and moves what it publishes to p.dir under its release name.
func (p *releasePlan) buildArch(ctx context.Context, work string, ra releaseArch) error {
	log := ui.Stderr
	log.Info("%s", log.Bold("Building "+ra.target+" "+ra.arch))
	cfg := p.proj.Config
	dir := filepath.Join(work, ra.target+"-"+ra.arch)
	b := &builder{
		proj: p.proj, out: dir, tags: p.tags, ldflags: cfg.Build.LDFlags, zig: p.zig,
		console: cfg.Windows.Console, formats: cfg.Linux.Formats,
		mac: macFlags{isSet: func(string) bool { return false }},
	}
	lo := gotool.ListOptions{Tags: b.tags}
	switch ra.target {
	case "macos":
		lo.GOOS = "darwin"
		b.universal = ra.arch == "universal"
	case "windows":
		lo.GOOS, lo.GOARCH, b.arch = "windows", ra.arch, ra.arch
	case "linux":
		lo.GOOS, lo.GOARCH, b.arch = "linux", ra.arch, ra.arch
	}
	var err error
	if b.main, err = resolveMain(ctx, log, p.proj, p.arg, lo); err != nil {
		return err
	}
	if p.main == nil {
		p.main = b.main
		p.slug = release.Slug(b.main.Name)
		if p.slug == "" {
			p.slug = linuxPackage(b.main)
		}
	}
	build := func() error {
		switch ra.target {
		case "macos":
			return p.buildMac(ctx, b, ra.arch, dir)
		case "windows":
			return p.buildWindows(ctx, b, ra.arch, dir)
		}
		return p.buildLinux(ctx, b, ra.arch, dir)
	}
	if ra.arch == "universal" {
		return build()
	}
	return withGOARCH(ra.arch, build)
}

// withGOARCH runs fn with $GOARCH set to arch: the macOS build takes the
// architecture of a single build from the go command's environment, and
// the others then know the architecture was asked for.
func withGOARCH(arch string, fn func() error) error {
	old, had := os.LookupEnv("GOARCH")
	if err := os.Setenv("GOARCH", arch); err != nil {
		return err
	}
	defer func() {
		var err error
		if had {
			err = os.Setenv("GOARCH", old)
		} else {
			err = os.Unsetenv("GOARCH")
		}
		if err != nil {
			ui.Stderr.Warn("restoring $GOARCH: %v", err)
		}
	}()
	return fn()
}

func (p *releasePlan) add(path, name, target, arch string) error {
	dst, err := release.Move(path, p.dir, name)
	if err != nil {
		return err
	}
	p.assets = append(p.assets, release.Asset{Name: name, Path: dst, Target: target, Arch: arch})
	return nil
}

func (p *releasePlan) buildMac(ctx context.Context, b *builder, arch, dir string) error {
	if err := b.macos(ctx); err != nil {
		return err
	}
	name := b.main.Name
	if dmg := filepath.Join(dir, name+".dmg"); exists(dmg) {
		if err := p.add(dmg, release.MacName(p.slug, p.version, arch, "dmg"), "macos", arch); err != nil {
			return err
		}
	} else {
		zip := filepath.Join(dir, release.MacName(p.slug, p.version, arch, "zip"))
		err := step(ctx, "Zipping "+name+".app", "Zipped "+name+".app", func() error {
			return release.ZipDir(filepath.Join(dir, name+".app"), zip)
		})
		if err != nil {
			return err
		}
		if err := p.add(zip, filepath.Base(zip), "macos", arch); err != nil {
			return err
		}
	}
	if pkg := filepath.Join(dir, name+".pkg"); exists(pkg) {
		return p.add(pkg, release.MacName(p.slug, p.version, arch, "pkg"), "macos", arch)
	}
	return nil
}

func (p *releasePlan) buildWindows(ctx context.Context, b *builder, arch, dir string) error {
	if err := b.windows(ctx); err != nil {
		return err
	}
	zip := filepath.Join(dir, release.WindowsName(p.slug, p.version, arch))
	exe := b.main.Name + ".exe"
	files := []string{filepath.Join(dir, exe)}
	if dll := filepath.Join(dir, ziglib.FileName("windows")); exists(dll) {
		files = append(files, dll)
	}
	err := step(ctx, "Zipping "+exe, "Zipped "+exe, func() error {
		return release.ZipFiles(zip, files...)
	})
	if err != nil {
		return err
	}
	return p.add(zip, filepath.Base(zip), "windows", arch)
}

func (p *releasePlan) buildLinux(ctx context.Context, b *builder, arch, dir string) error {
	if len(b.formats) == 0 {
		ui.Stderr.Warn("linux.formats is empty, so the Linux build makes no file to release; add \"tar\", \"deb\" or \"rpm\"")
	}
	if err := b.linux(ctx); err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		// The install tree is a directory; the tarball and the packages
		// beside it are what is released.
		if e.Type().IsRegular() {
			if err := p.add(filepath.Join(dir, e.Name()), e.Name(), "linux", arch); err != nil {
				return err
			}
		}
	}
	return nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// download is where an asset will be downloaded from.
func (p *releasePlan) download(a release.Asset) string {
	return github.DownloadURL(p.web, p.repo, p.tag, a.Name)
}

// homepage is the app's: the one given, else [linux]'s, else the
// repository's.
func (p *releasePlan) homepage(given string) string {
	switch {
	case given != "":
		return given
	case p.proj.Config.Linux.Homepage != "":
		return p.proj.Config.Linux.Homepage
	}
	return p.web + "/" + p.repo
}

// writeCask writes the cask to homebrew/Casks in the release directory,
// when publishing to Homebrew.
func (p *releasePlan) writeCask() error {
	if !p.publishes("homebrew") {
		return nil
	}
	cfg := p.proj.Config
	h := cfg.Release.Homebrew
	c := homebrew.Cask{
		Token: h.Cask, Name: p.main.Name, Version: p.version, ID: p.main.ID,
		Desc: h.Desc, Homepage: p.homepage(h.Homepage), MinMacOS: cfg.MacOS.MinimumSystemVersion,
	}
	if c.Token == "" {
		c.Token = homebrew.Token(p.main.Name)
	}
	if c.Desc == "" {
		c.Desc = cfg.Linux.Description
	}
	if v, ok := cfg.MacOS.Plist["LSMinimumSystemVersion"].(string); ok {
		c.MinMacOS = v
	}
	// The disk image if there is one, else the zip: the cask installs
	// the app from either, but not from the installer package.
	pick := map[string]release.Asset{}
	for _, a := range p.assets {
		if a.Target != "macos" || strings.HasSuffix(a.Name, ".pkg") {
			continue
		}
		if cur, ok := pick[a.Arch]; !ok || strings.HasSuffix(a.Name, ".dmg") && !strings.HasSuffix(cur.Name, ".dmg") {
			pick[a.Arch] = a
		}
	}
	file := func(arch string) *homebrew.File {
		if a, ok := pick[arch]; ok {
			return &homebrew.File{URL: p.download(a), SHA256: a.SHA256}
		}
		return nil
	}
	c.Universal, c.Arm, c.Intel = file("universal"), file("arm64"), file("amd64")
	ruby, err := c.Ruby()
	if err != nil {
		return err
	}
	path := filepath.Join(p.dir, "homebrew", filepath.FromSlash(homebrew.Path(c.Token)))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(ruby), 0o644); err != nil {
		return err
	}
	p.cask, p.caskToken = path, c.Token
	ui.Stderr.Success("Wrote the cask %s", ui.Stderr.Cyan(shortPath(path)))
	if !p.notarized() {
		ui.Stderr.Warn("the app is not notarized, so macOS blocks its first launch from the cask; set macos.notarize, as docs/releasing.md says")
	}
	return nil
}

// notarized reports whether the macOS build is notarized, which a build
// asked to notarize is or fails.
func (p *releasePlan) notarized() bool { return p.proj.Config.MacOS.Notarize }

// writeChoco writes the Chocolatey package's source to chocolatey/ in the
// release directory and packs it, when publishing to Chocolatey.
func (p *releasePlan) writeChoco() error {
	if !p.publishes("choco") {
		return nil
	}
	cfg := p.proj.Config
	c := cfg.Release.Chocolatey
	owner, _, _ := strings.Cut(p.repo, "/")
	pkg := choco.Package{
		ID: c.ID, Version: p.version, Title: p.main.Name, Authors: c.Authors,
		ProjectURL: p.homepage(c.ProjectURL), LicenseURL: c.LicenseURL, IconURL: c.IconURL,
		PackageSourceURL: p.web + "/" + p.repo,
		Tags:             c.Tags, Summary: c.Summary, Description: c.Description,
		Exe: p.main.Name + ".exe",
	}
	if pkg.ID == "" {
		pkg.ID = release.Slug(p.main.Name)
	}
	if pkg.Authors == "" {
		pkg.Authors = cmpOr(cfg.Windows.Company, owner)
	}
	if pkg.Summary == "" {
		pkg.Summary = cfg.Linux.Description
	}
	if pkg.Description == "" {
		pkg.Description = cmpOr(pkg.Summary, p.main.Name+" "+p.version)
	}
	for _, a := range p.assets {
		if a.Target != "windows" {
			continue
		}
		switch a.Arch {
		case "amd64":
			pkg.URL64, pkg.Checksum64 = p.download(a), a.SHA256
		case "386":
			pkg.URL32, pkg.Checksum32 = p.download(a), a.SHA256
		}
	}
	if err := pkg.Check(); err != nil {
		return err
	}
	dir := filepath.Join(p.dir, "chocolatey")
	if err := pkg.Write(dir); err != nil {
		return err
	}
	p.nupkg = filepath.Join(dir, pkg.FileName())
	if err := pkg.Pack(p.nupkg, rand.Reader); err != nil {
		return err
	}
	p.chocoID = pkg.ID
	ui.Stderr.Success("Made the Chocolatey package %s", ui.Stderr.Cyan(shortPath(p.nupkg)))
	return nil
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// notes returns the release's description: the file's text, or a list of
// the files.
func (p *releasePlan) notes(file string, all []release.Asset) (string, error) {
	if file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Errorf("--notes: %w", err)
		}
		return string(data), nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s\n\n| File | Size | SHA-256 |\n| --- | --- | --- |\n", p.main.Name, p.version)
	for _, a := range all {
		fmt.Fprintf(&b, "| %s | %s | `%s` |\n", a.Name, release.HumanSize(a.Size), a.SHA256)
	}
	return b.String(), nil
}

// printPlan prints what a real run would send.
func (p *releasePlan) printPlan(w io.Writer, pub github.Publish) {
	if len(p.publish) == 0 {
		fmt.Fprintln(w, "\nDry run: nothing to publish.")
		return
	}
	fmt.Fprintln(w, "\nDry run: nothing was published. A real run would send:")
	if p.publishes("github") {
		fmt.Fprintf(w, "\n# GitHub release %s of %s\n", p.tag, p.repo)
		for _, l := range github.Plan(p.api, pub) {
			fmt.Fprintln(w, l)
		}
	}
	if p.publishes("homebrew") {
		tap := p.proj.Config.Release.Homebrew.Tap
		fmt.Fprintf(w, "\n# Homebrew cask %s in %s, from %s\n", p.caskToken, tap, shortPath(p.cask))
		for _, l := range homebrew.Plan(p.api, tap, p.caskToken, p.version) {
			fmt.Fprintln(w, l)
		}
	}
	if p.publishes("choco") {
		fmt.Fprintf(w, "\n# Chocolatey package %s %s\n", p.chocoID, p.version)
		fmt.Fprintf(w, "PUT    %s  (multipart/form-data, %s, X-NuGet-ApiKey from $CHOCOLATEY_API_KEY)\n",
			choco.PushURL(p.proj.Config.Release.Chocolatey.Source), filepath.Base(p.nupkg))
	}
}

// publishAll publishes to each channel in turn: GitHub first, since the
// others download from it.
func (p *releasePlan) publishAll(ctx context.Context, gh *github.Client, pub github.Publish, getenv func(string) string) error {
	log := ui.Stderr
	if p.publishes("github") {
		rel, err := gh.Run(ctx, pub)
		if err != nil {
			return err
		}
		if rel.Draft {
			log.Success("Uploaded to the draft release %s: %s", p.tag, log.Cyan(rel.HTMLURL))
		} else {
			log.Success("Published %s: %s", p.tag, log.Cyan(rel.HTMLURL))
		}
	}
	if p.publishes("homebrew") {
		tap := p.proj.Config.Release.Homebrew.Tap
		c := gh
		if t := strings.TrimSpace(getenv("HOMEBREW_TAP_TOKEN")); t != "" {
			c = github.New(p.api, release.NewToken(t, "$HOMEBREW_TAP_TOKEN"))
		}
		ruby, err := os.ReadFile(p.cask)
		if err != nil {
			return err
		}
		err = step(ctx, "Committing "+homebrew.Path(p.caskToken)+" to "+tap, "Committed "+homebrew.Path(p.caskToken)+" to "+tap, func() error {
			return homebrew.Publish(ctx, c, tap, p.caskToken, p.version, ruby)
		})
		if err != nil {
			return fmt.Errorf("the tap %s (with the %v, which must be able to write it): %w", tap, c.Token, err)
		}
	}
	if p.publishes("choco") {
		src := p.proj.Config.Release.Chocolatey.Source
		err := step(ctx, "Pushing "+filepath.Base(p.nupkg)+" to "+src, "Pushed "+filepath.Base(p.nupkg)+" to "+src, func() error {
			return choco.Push(ctx, nil, src, getenv("CHOCOLATEY_API_KEY"), p.nupkg)
		})
		if err != nil {
			return err
		}
		log.Note("%s", log.Dim("a community package is checked by Chocolatey's moderators before it is listed"))
	}
	return nil
}
