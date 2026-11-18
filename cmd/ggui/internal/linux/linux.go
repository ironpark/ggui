// Package linux makes what ships a ggui application to Linux desktops: an
// install tree laid out as under /usr, with the program, its desktop
// entry, its icons in the hicolor theme and its AppStream metadata; and
// the tarball, Debian package and RPM package made from that tree.
//
// Everything is written in pure Go, the packages through nfpm's deb and
// rpm packagers, so a Linux release is built the same from macOS,
// Windows or Linux.
package linux

import (
	"strings"
)

// Arches are the GOARCH values a Linux build is packaged for: those
// ggui's CI builds and tests on Linux, which nfpm names for both package
// formats.
var Arches = []string{"amd64", "arm64", "riscv64"}

// App is what the files of the tree say about the application.
type App struct {
	// Name is the name people see, the desktop entry's Name.
	Name string
	// GenericName is what kind of program it is, such as "Web Browser";
	// left out when empty.
	GenericName string
	// Summary is one line on what it does: the desktop entry's Comment,
	// the AppStream summary and the packages' description. Empty is Name.
	Summary string
	// ID is the reverse-DNS application ID: the desktop file's name, the
	// icons' name, the AppStream component ID, and the X11 window class
	// the program sets, which is how a desktop matches its windows to the
	// entry.
	ID string
	// Package is the package name, which is also the program's file name
	// in /usr/bin: PackageName of the app's name.
	Package string
	// Version is app.version, as the files and the packages write it.
	Version string
	// Arch is the GOARCH the program is built for.
	Arch       string
	Categories []string // freedesktop.org menu categories
	// MIMETypes are the types the program opens. With any, the program
	// is given the files or URLs it is asked to open (Exec ... %U).
	MIMETypes  []string
	Homepage   string
	License    string // an SPDX expression
	Maintainer string // "Name <email>"
}

func (a App) summary() string {
	if a.Summary != "" {
		return a.Summary
	}
	return a.Name
}

// PackageName makes a Debian and RPM package name from an app's name:
// lowercase letters, digits and the separators those allow, a run of
// anything else one hyphen, starting with a letter or digit, so
// "Hello World" is hello-world. It returns "" when fewer than two
// characters remain, as for a name in another script, for the caller to
// fall back on another source.
func PackageName(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '+', r == '.':
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			dash = false
			b.WriteRune(r)
		default:
			dash = true
		}
	}
	s := strings.TrimLeft(b.String(), "+.-")
	s = strings.TrimRight(s, ".")
	if len(s) < 2 {
		return ""
	}
	return s
}

// PackageVersion makes a version both dpkg and rpm accept, and order as
// intended, from the app's version. app.version is one to three numbers,
// which pass unchanged. Anything else, as a version given some other way,
// is made valid rather than refused:
//
//   - a leading v before a digit is dropped: v1.2.0 is 1.2.0;
//   - a pre-release after a hyphen becomes a tilde, which both sort
//     before the release: 1.2.0-rc.1 is 1.2.0~rc.1;
//   - build metadata after + is kept: 1.2.0+git5 stays;
//   - any other character neither allows becomes a dot;
//   - a version that does not start with a digit, such as dev, is put
//     before every release as 0~dev, and an empty one is 0.
func PackageVersion(v string) string {
	v = strings.TrimSpace(v)
	if len(v) > 1 && (v[0] == 'v' || v[0] == 'V') && v[1] >= '0' && v[1] <= '9' {
		v = v[1:]
	}
	meta := ""
	if i := strings.IndexByte(v, '+'); i >= 0 {
		v, meta = v[:i], v[i+1:]
	}
	clean := func(s string, allowTilde bool) string {
		var b strings.Builder
		for _, r := range s {
			switch {
			case r >= '0' && r <= '9', r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '.':
				b.WriteRune(r)
			case r == '~' && allowTilde:
				b.WriteRune(r)
			case r == '-' && allowTilde:
				b.WriteRune('~')
			default:
				b.WriteRune('.')
			}
		}
		return b.String()
	}
	v = clean(v, true)
	meta = strings.Trim(clean(meta, false), ".")
	if v == "" {
		v = "0"
	} else if v[0] < '0' || v[0] > '9' {
		v = "0~" + v
	}
	if meta != "" {
		v += "+" + meta
	}
	return v
}
