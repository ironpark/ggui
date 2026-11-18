package linux

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/goreleaser/nfpm/v2"
	"github.com/goreleaser/nfpm/v2/deb"
	"github.com/goreleaser/nfpm/v2/files"
	"github.com/goreleaser/nfpm/v2/rpm"
)

// Package says what package to make of an install tree.
type Package struct {
	App
	Tree string // the tree Assemble made
	// Depends are the package's dependencies, in its format's syntax.
	// ggwd and ggfx open the system's libraries at run time, which a
	// package manager cannot see, so they are named here; ggui.toml's
	// defaults, config.DefaultDependsDeb and DefaultDependsRPM, list them.
	Depends []string
	OutDir  string // the directory the package is written to
}

// refreshScript runs after the package is installed or removed: it
// updates the caches of desktop entries and icons, where the tools that
// keep them are installed, and never fails the installation.
const refreshScript = `#!/bin/sh
if command -v update-desktop-database >/dev/null 2>&1; then
	update-desktop-database -q /usr/share/applications || true
fi
if command -v gtk-update-icon-cache >/dev/null 2>&1; then
	gtk-update-icon-cache -q -t -f /usr/share/icons/hicolor || true
fi
exit 0
`

// info is the nfpm description of the package.
func (p Package) info(scripts string) (*nfpm.Info, error) {
	entries, err := walk(p.Tree)
	if err != nil {
		return nil, err
	}
	var contents files.Contents
	for _, e := range entries {
		if e.dir {
			// The directories come with the files; the shared ones,
			// such as /usr/share/icons/hicolor, belong to other
			// packages, and an RPM must not claim them.
			continue
		}
		contents = append(contents, &files.Content{
			Source:      e.src,
			Destination: "/" + e.rel,
			Type:        files.TypeFile,
			FileInfo:    &files.ContentFileInfo{Mode: e.mode, Owner: "root", Group: "root"},
		})
	}
	summary := p.summary()
	maintainer := p.Maintainer
	if maintainer == "" {
		// What nfpm would write, set here so that it does not warn of it
		// on standard error; the build warns in its own words.
		maintainer = "Unset Maintainer <unset@localhost>"
	}
	in := &nfpm.Info{
		Name:     p.Package,
		Arch:     p.Arch,
		Platform: "linux",
		Version:  PackageVersion(p.Version),
		// The version is made valid for both formats here, and nfpm is
		// to take it as it is rather than read it as semver.
		VersionSchema:   "none",
		Section:         "misc",
		Maintainer:      maintainer,
		Description:     summary,
		Homepage:        p.Homepage,
		License:         p.License,
		DisableGlobbing: true,
		Overridables: nfpm.Overridables{
			Depends:  p.Depends,
			Contents: contents,
			Scripts: nfpm.Scripts{
				PostInstall: scripts,
				PostRemove:  scripts,
			},
			RPM: nfpm.RPM{Summary: summary},
		},
	}
	return nfpm.WithDefaults(in), nil
}

// FileName is the package's conventional file name for format, deb or
// rpm: name_version_arch.deb or name-version-1.arch.rpm, in the format's
// own names of the architecture.
func (p Package) FileName(format string) (string, error) {
	pk, err := packager(format)
	if err != nil {
		return "", err
	}
	in, err := p.info("")
	if err != nil {
		return "", err
	}
	return pk.ConventionalFileName(in), nil
}

type packagerWithName interface {
	nfpm.Packager
	ConventionalFileName(*nfpm.Info) string
}

func packager(format string) (packagerWithName, error) {
	switch format {
	case "deb":
		return deb.Default, nil
	case "rpm":
		return rpm.DefaultRPM, nil
	}
	return nil, fmt.Errorf("linux: no packager for %q", format)
}

// Make writes the package in format, deb or rpm, to OutDir under its
// conventional name, replacing one there, and returns its path.
func (p Package) Make(format string) (path string, err error) {
	pk, err := packager(format)
	if err != nil {
		return "", err
	}
	tmp, err := os.MkdirTemp("", "ggui-package-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	script := filepath.Join(tmp, "refresh.sh")
	if err := os.WriteFile(script, []byte(refreshScript), 0o755); err != nil {
		return "", err
	}
	in, err := p.info(script)
	if err != nil {
		return "", err
	}
	path = filepath.Join(p.OutDir, pk.ConventionalFileName(in))
	// The packager changes what it is given, as the architecture's name;
	// the name above was made from the same values.
	in, err = p.info(script)
	if err != nil {
		return "", err
	}
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			os.Remove(path)
		}
	}()
	if err := pk.Package(in, f); err != nil {
		return "", fmt.Errorf("%s package: %w", format, err)
	}
	return path, nil
}
