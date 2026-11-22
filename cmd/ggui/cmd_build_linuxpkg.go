package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/ironpark/ggui/cmd/ggui/internal/config"
	"github.com/ironpark/ggui/cmd/ggui/internal/linux"
	"github.com/ironpark/ggui/cmd/ggui/internal/project"
	"github.com/ironpark/ggui/cmd/ggui/internal/svgicon"
	"github.com/ironpark/ggui/cmd/ggui/internal/ui"
)

// linuxPackage is the package name, and the program's name in /usr/bin:
// made from the app's name, else the main package's, else the last part
// of the ID.
func linuxPackage(m *project.Main) string {
	for _, s := range []string{m.Name, m.Executable, m.ID[strings.LastIndexByte(m.ID, '.')+1:]} {
		if p := linux.PackageName(s); p != "" {
			return p
		}
	}
	return "app"
}

// parseFormats reads --format: Linux formats, comma-separated, each once.
// An empty value makes the install tree alone.
func parseFormats(s string) ([]string, error) {
	var out []string
	for f := range strings.SplitSeq(s, ",") {
		f = strings.ToLower(strings.TrimSpace(f))
		switch {
		case f == "":
		case !slices.Contains(config.LinuxFormats, f):
			return nil, fmt.Errorf("--format %q: the formats are %s", f, strings.Join(config.LinuxFormats, ", "))
		case !slices.Contains(out, f):
			out = append(out, f)
		}
	}
	return out, nil
}

// linuxApp is what the Linux files say about the app.
func (b *builder) linuxApp() linux.App {
	cfg := b.proj.Config
	l := cfg.Linux
	return linux.App{
		Name:        b.main.Name,
		GenericName: l.GenericName,
		Summary:     l.Description,
		ID:          b.main.ID,
		Package:     linuxPackage(b.main),
		Version:     cfg.App.Version,
		Arch:        b.arch,
		Categories:  l.Categories,
		MIMETypes:   l.MIMETypes,
		Homepage:    l.Homepage,
		License:     l.License,
		Maintainer:  l.Maintainer,
	}
}

// depends returns what a package in format, deb or rpm, depends on.
func (b *builder) depends(format string) []string {
	if format == "deb" {
		return b.proj.Config.Linux.DependsDeb
	}
	return b.proj.Config.Linux.DependsRPM
}

// linux builds the install tree Package-linux-Arch: the program for
// linux/arch without cgo, as ggwd and ggfx load the system's libraries
// at run time, with its desktop entry, icons and AppStream metadata; and
// from the tree the archives asked for.
func (b *builder) linux(ctx context.Context) error {
	if err := b.proj.CheckIcon(); err != nil {
		return err
	}
	cfg := b.proj.Config
	dir, err := b.proj.OutDir(b.out)
	if err != nil {
		return err
	}
	app := b.linuxApp()

	tmp, err := os.MkdirTemp("", "ggui-build-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	icon, err := b.icon(ctx, tmp)
	if err != nil {
		return err
	}
	svg := ""
	if p := b.proj.Path(cfg.App.Icon); p != "" && svgicon.Is(p) {
		svg = p
	}

	bin := filepath.Join(tmp, app.Package)
	o := b.release(bin)
	o.GOOS, o.GOARCH = "linux", b.arch
	o.Env = append(o.Env, "CGO_ENABLED=0")
	if err := b.build(ctx, o, "linux/"+b.arch); err != nil {
		return err
	}

	tree, err := b.linuxTree(ctx, dir, app, bin, icon, svg)
	if err != nil {
		return err
	}
	archives, err := b.linuxArchives(ctx, app, tree, dir)
	if err != nil {
		return err
	}
	return b.reportLinux(tree, archives)
}

// reportLinux says what was built: the install tree and the archives,
// and notes when the architecture is this machine's but not amd64.
func (b *builder) reportLinux(tree string, archives []string) error {
	log := ui.Stderr
	if runtime.GOOS != "linux" && b.arch == runtime.GOARCH && b.arch != "amd64" && os.Getenv("GOARCH") == "" {
		log.Note("%s", log.Dim("for "+b.arch+", this machine's architecture; most PCs need GOARCH=amd64"))
	}
	for _, a := range append([]string{tree}, archives...) {
		if err := report(a, "linux/"+b.arch); err != nil {
			return err
		}
	}
	return nil
}

// linuxTree assembles the install tree in dir and says what the files
// leave out, returning the tree's path.
func (b *builder) linuxTree(ctx context.Context, dir string, app linux.App, bin, icon, svg string) (string, error) {
	cfg := b.proj.Config
	log := ui.Stderr
	name := linux.TreeName(app)
	var (
		tree     string
		warnings []string
	)
	err := step(ctx, "Assembling "+name, "Assembled "+name, func() error {
		var err error
		tree, warnings, err = linux.Assemble(linux.Tree{
			App:      app,
			Binary:   bin,
			Icon:     icon,
			IconName: b.proj.Path(cfg.App.Icon),
			SVG:      svg,
			OutDir:   dir,
		})
		return err
	})
	if err != nil {
		return "", err
	}
	for _, w := range warnings {
		log.Warn("%s", w)
	}
	if icon == "" {
		log.Warn("app.icon is not set, so the desktop entry has no icon and desktops show a generic one")
	}
	if b.main.ID != cfg.App.ID {
		log.Warn("app.id is not set, so the desktop entry is %s.desktop; set it in ggui.toml before shipping, since desktops match windows to entries by it", b.main.ID)
	}
	if cfg.Linux.Maintainer == "" && slices.Contains(b.formats, "deb") {
		log.Warn("linux.maintainer is not set, so the Debian package names a placeholder; set it to \"Name <email>\" before shipping")
	}
	return tree, nil
}

// linuxArchives makes each format of b.formats from the tree, returning
// the files made.
func (b *builder) linuxArchives(ctx context.Context, app linux.App, tree, dir string) ([]string, error) {
	var artifacts []string
	for _, f := range b.formats {
		var (
			out   string
			write func() error
		)
		switch f {
		case "tar":
			out = filepath.Join(dir, linux.TarName(app))
			write = func() error { return linux.Tar(tree, out) }
		default:
			p := linux.Package{App: app, Tree: tree, Depends: b.depends(f), OutDir: dir}
			name, err := p.FileName(f)
			if err != nil {
				return nil, err
			}
			out = filepath.Join(dir, name)
			write = func() error {
				_, err := p.Make(f)
				return err
			}
		}
		if err := step(ctx, "Making "+filepath.Base(out), "Made "+filepath.Base(out), write); err != nil {
			return nil, err
		}
		artifacts = append(artifacts, out)
	}
	return artifacts, nil
}
