package config

import (
	"fmt"
	"math"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/BurntSushi/toml"
	"github.com/ironpark/ggui/cmd/ggui/internal/i18n"
	"github.com/ironpark/zapp/pkg/dmg"
	"github.com/ironpark/zapp/pkg/udif"
)

type problem struct {
	key toml.Key
	msg string
}

var (
	bundleID   = regexp.MustCompile(`^[A-Za-z0-9-]+(\.[A-Za-z0-9-]+)+$`)
	versionRE  = regexp.MustCompile(`^[0-9]+(\.[0-9]+){0,2}$`)
	macVersion = regexp.MustCompile(`^[0-9]+(\.[0-9]+){0,2}$`)
	tagRE      = regexp.MustCompile(`^[A-Za-z0-9_.]+$`)
)

func validAppName(s string) bool {
	if s == "." || strings.ContainsAny(s, `/\`) || strings.Contains(s, "..") {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// validate checks the values the decoder accepted as types but that the
// build would trip over later, further from where they were written.
func (c *Config) validate() []problem {
	var ps []problem
	add := func(msg string, key ...string) { ps = append(ps, problem{toml.Key(key), msg}) }

	if c.App.Name != "" && !validAppName(c.App.Name) {
		add(fmt.Sprintf("app.name %q must not contain path separators, \"..\", or control characters", c.App.Name), "app", "name")
	}
	if c.App.ID != "" && !bundleID.MatchString(c.App.ID) {
		add(fmt.Sprintf("app.id %q is not a bundle identifier: use reverse-DNS letters, digits, hyphens and dots, such as com.example.hello", c.App.ID), "app", "id")
	}
	if !versionRE.MatchString(c.App.Version) {
		add(fmt.Sprintf("app.version %q must be one to three numbers separated by dots, such as 1.2.0: macOS reads it as CFBundleShortVersionString", c.App.Version), "app", "version")
	}
	switch ext := strings.ToLower(filepath.Ext(c.App.Icon)); {
	case c.App.Icon == "" || ext == ".png" || ext == ".svg":
		// An SVG is drawn at build time by the project's own ggfx.
	case ext == ".svgz":
		base := filepath.Base(c.App.Icon)
		add(fmt.Sprintf("app.icon %q is a compressed SVG; name the SVG it holds, as gunzip -c %s > %s writes it",
			c.App.Icon, base, strings.TrimSuffix(base, filepath.Ext(base))+".svg"), "app", "icon")
	default:
		add(fmt.Sprintf("app.icon %q must be a PNG file, square, ideally 1024×1024, or an SVG with a square viewBox", c.App.Icon), "app", "icon")
	}
	if c.App.Package == "" {
		add(`app.package must name the main package's directory; "." is the directory of this file`, "app", "package")
	}
	if c.Build.Output == "" {
		add(`build.output must name a directory, such as "build"`, "build", "output")
	}
	for _, list := range []struct {
		tags []string
		key  string
	}{{c.Build.Tags, "build"}, {c.Dev.Tags, "dev"}} {
		for _, t := range list.tags {
			if !tagRE.MatchString(t) {
				add(fmt.Sprintf("%s.tags: %q is not a build tag; give each tag as its own string", list.key, t), list.key, "tags")
			}
		}
	}
	switch c.Dev.Display {
	case "", "remote", "window":
	default:
		add(fmt.Sprintf(`dev.display %q must be "remote" or "window"`, c.Dev.Display), "dev", "display")
	}
	for k := range c.Dev.Env {
		if k == "" || strings.ContainsAny(k, "= \t") {
			add(fmt.Sprintf("dev.env: %q is not an environment variable name", k), "dev", "env")
		}
	}
	if c.MacOS.MinimumSystemVersion != "" && !macVersion.MatchString(c.MacOS.MinimumSystemVersion) {
		add(fmt.Sprintf("macos.minimum_system_version %q must be a version such as 11.0", c.MacOS.MinimumSystemVersion), "macos", "minimum_system_version")
	}
	if c.MacOS.Category != "" && !strings.HasPrefix(c.MacOS.Category, "public.app-category.") {
		add(fmt.Sprintf("macos.category %q must be one of Apple's public.app-category.* types, such as public.app-category.productivity", c.MacOS.Category), "macos", "category")
	}
	if c.MacOS.SigningIdentity == "" {
		add(`macos.signing_identity must not be empty: "-" signs ad hoc`, "macos", "signing_identity")
	}
	ps = append(ps, c.MacOS.validateSigning()...)
	for k, v := range c.MacOS.Plist {
		if err := plistValue(v); err != nil {
			add(fmt.Sprintf("macos.plist.%s: %v", k, err), "macos", "plist", k)
		}
	}
	ps = append(ps, c.Linux.validate()...)
	ps = append(ps, c.Release.validate()...)
	if !strings.HasPrefix(c.Web.Base, "/") {
		add(fmt.Sprintf("web.base %q must be an absolute path such as / or /app/", c.Web.Base), "web", "base")
	}
	for f := range strings.FieldsSeq(c.Web.WasmOpt) {
		if !strings.HasPrefix(f, "-") {
			add(fmt.Sprintf("web.wasm_opt %q must be wasm-opt flags such as -O2 or -Oz, or empty to skip it", c.Web.WasmOpt), "web", "wasm_opt")
			break
		}
	}
	for i, key := range []string{"funcs", "plurals"} {
		list := c.I18n.Funcs
		if i == 1 {
			list = c.I18n.Plurals
		}
		for _, spec := range list {
			if _, err := i18n.ParseFunc(spec, i == 1); err != nil {
				add(fmt.Sprintf("i18n.%s: %v", key, err), "i18n", key)
			}
		}
	}
	return ps
}

var (
	envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	teamID  = regexp.MustCompile(`^[A-Z0-9]{10}$`)
)

// validateSigning checks the keys that sign, notarize and package.
// Whether there is a certificate or a password to use depends on the
// environment of the build, so that is checked there.
func (m *MacOS) validateSigning() []problem {
	var ps []problem
	add := func(msg string, key ...string) { ps = append(ps, problem{toml.Key(key), msg}) }

	for _, e := range []struct {
		value string
		key   []string
	}{
		{m.P12Base64Env, []string{"macos", "p12_base64_env"}},
		{m.P12PasswordEnv, []string{"macos", "p12_password_env"}},
		{m.Notary.PasswordEnv, []string{"macos", "notary", "password_env"}},
	} {
		if e.value != "" && !envName.MatchString(e.value) {
			add(fmt.Sprintf("%s %q must name an environment variable, such as GGUI_P12_PASSWORD; the secret itself never goes in this file",
				strings.Join(e.key, "."), e.value), e.key...)
		}
	}

	n := m.Notary
	if n.AppleID != "" && n.TeamID == "" {
		add("macos.notary.apple_id needs macos.notary.team_id, the ten-character ID of the team the app is signed for", "macos", "notary", "apple_id")
	}
	if n.TeamID != "" && !teamID.MatchString(n.TeamID) {
		add(fmt.Sprintf("macos.notary.team_id %q must be ten capital letters and digits, as in the signing identity's parentheses", n.TeamID), "macos", "notary", "team_id")
	}
	if d, err := time.ParseDuration(n.Timeout); err != nil || d <= 0 {
		add(fmt.Sprintf("macos.notary.timeout %q must be a duration such as 30m or 2h", n.Timeout), "macos", "notary", "timeout")
	}

	d := m.DMG
	if d.Icon != "" && !hasExt(d.Icon, ".png", ".icns") {
		add(fmt.Sprintf("macos.dmg.icon %q must be a PNG or an .icns file", d.Icon), "macos", "dmg", "icon")
	}
	if d.Background != "" && !hasExt(d.Background, ".png", ".jpg", ".jpeg", ".tif", ".tiff") {
		add(fmt.Sprintf("macos.dmg.background %q must be a PNG, JPEG or TIFF image", d.Background), "macos", "dmg", "background")
	}
	if d.Window.Width <= 0 || d.Window.Height <= 0 {
		add(fmt.Sprintf("macos.dmg.window is %d×%d; give a width and height in points, such as { width = 640, height = 480 }", d.Window.Width, d.Window.Height), "macos", "dmg", "window")
	}
	if d.IconSize < dmg.MinIconSize || d.IconSize > dmg.MaxIconSize {
		add(fmt.Sprintf("macos.dmg.icon_size %d must be from %d to %d", d.IconSize, dmg.MinIconSize, dmg.MaxIconSize), "macos", "dmg", "icon_size")
	}
	if d.LabelSize < dmg.MinLabelSize || d.LabelSize > dmg.MaxLabelSize {
		add(fmt.Sprintf("macos.dmg.label_size %d must be from %d to %d", d.LabelSize, dmg.MinLabelSize, dmg.MaxLabelSize), "macos", "dmg", "label_size")
	}
	for _, p := range []struct {
		pos []int
		key string
	}{{d.AppPosition, "app_position"}, {d.ApplicationsPosition, "applications_position"}} {
		if p.pos != nil && (len(p.pos) != 2 || p.pos[0] < 0 || p.pos[1] < 0) {
			add(fmt.Sprintf("macos.dmg.%s must be [x, y], the icon's centre in points from the window's top left", p.key), "macos", "dmg", p.key)
		}
	}
	if _, err := dmg.ParseFileSystem(d.FS); err != nil {
		add(fmt.Sprintf("macos.dmg.fs %q must be one of %s", d.FS, strings.Join(dmg.FileSystemNames, ", ")), "macos", "dmg", "fs")
	}
	if _, err := udif.ParseFormat(d.Format); err != nil {
		add(fmt.Sprintf(`macos.dmg.format %q must be "udzo" (zlib, which every macOS reads) or "ulfo" (lzfse, smaller, macOS 10.11 and later)`, d.Format), "macos", "dmg", "format")
	}

	if !strings.HasPrefix(m.PKG.InstallLocation, "/") {
		add(fmt.Sprintf("macos.pkg.install_location %q must be an absolute path, such as /Applications", m.PKG.InstallLocation), "macos", "pkg", "install_location")
	}
	return ps
}

var (
	categoryRE   = regexp.MustCompile(`^[A-Za-z0-9-]+$`)
	mimeTypeRE   = regexp.MustCompile(`^[a-z0-9][a-z0-9.+-]*/[A-Za-z0-9][A-Za-z0-9.+_-]*$`)
	maintainerRE = regexp.MustCompile(`^[^<>\n]*[^<>\s] <[^<>@\s]+@[^<>@\s]+>$`)
	licenseRE    = regexp.MustCompile(`^[A-Za-z0-9.+:() -]+$`)
)

// LinuxFormats are what [linux] formats may name.
var LinuxFormats = []string{"tar", "deb", "rpm"}

// validate checks [linux]: values that would make a desktop entry or a
// package that its readers refuse.
func (l *Linux) validate() []problem {
	var ps []problem
	add := func(msg string, key ...string) { ps = append(ps, problem{toml.Key(key), msg}) }

	for i, f := range l.Formats {
		switch {
		case !slices.Contains(LinuxFormats, f):
			add(fmt.Sprintf(`linux.formats: %q must be "tar", "deb" or "rpm"`, f), "linux", "formats")
		case slices.Index(l.Formats, f) != i:
			add(fmt.Sprintf("linux.formats names %q twice", f), "linux", "formats")
		}
	}
	for _, k := range []struct {
		value, key string
	}{{l.Description, "description"}, {l.GenericName, "generic_name"}} {
		if strings.ContainsAny(k.value, "\n\r") {
			add(fmt.Sprintf("linux.%s must be one line", k.key), "linux", k.key)
		}
	}
	for _, c := range l.Categories {
		if !categoryRE.MatchString(c) {
			add(fmt.Sprintf("linux.categories: %q is not a menu category; use the registered names, such as Utility, Graphics or Development, each as its own string", c), "linux", "categories")
		}
	}
	for _, m := range l.MIMETypes {
		if !mimeTypeRE.MatchString(m) {
			add(fmt.Sprintf("linux.mime_types: %q is not a MIME type such as image/png or x-scheme-handler/myapp", m), "linux", "mime_types")
		}
	}
	if l.Maintainer != "" && !maintainerRE.MatchString(l.Maintainer) {
		add(fmt.Sprintf(`linux.maintainer %q must be a name and an email address, such as "Jane Doe <jane@example.com>"`, l.Maintainer), "linux", "maintainer")
	}
	if l.Homepage != "" && !strings.HasPrefix(l.Homepage, "https://") && !strings.HasPrefix(l.Homepage, "http://") {
		add(fmt.Sprintf("linux.homepage %q must be an http or https URL", l.Homepage), "linux", "homepage")
	}
	if l.License != "" && !licenseRE.MatchString(l.License) {
		add(fmt.Sprintf("linux.license %q must be an SPDX license expression, such as MIT or Apache-2.0", l.License), "linux", "license")
	}
	for _, d := range []struct {
		list []string
		key  string
	}{{l.DependsDeb, "depends_deb"}, {l.DependsRPM, "depends_rpm"}} {
		for _, p := range d.list {
			if strings.TrimSpace(p) == "" || strings.ContainsAny(p, ",\n") {
				add(fmt.Sprintf("linux.%s: %q must name one dependency; give each as its own string", d.key, p), "linux", d.key)
			}
		}
	}
	return ps
}

var (
	// RepoRE matches a GitHub repository, owner/repo.
	RepoRE = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,38})/[A-Za-z0-9._-]+$`)
	// CaskTokenRE matches a Homebrew cask token.
	CaskTokenRE = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*(?:@[a-z0-9.-]+)?$`)
	// ChocoIDRE matches a Chocolatey package ID, which the community
	// repository wants lowercase.
	ChocoIDRE = regexp.MustCompile(`^[a-z0-9]+(?:[.-][a-z0-9]+)*$`)
	// tagPrefixRE is what may go before the version in a git tag.
	tagPrefixRE = regexp.MustCompile(`^[A-Za-z0-9._/-]*$`)
)

// validate checks [release] and its tables.
func (r *Release) validate() []problem {
	var ps []problem
	add := func(msg string, key ...string) { ps = append(ps, problem{toml.Key(key), msg}) }
	list := func(values, allowed []string, key string) {
		if len(values) == 0 {
			add(fmt.Sprintf("release.%s must name at least one of %s", key, quoteList(allowed)), "release", key)
		}
		for i, v := range values {
			switch {
			case !slices.Contains(allowed, v):
				add(fmt.Sprintf("release.%s: %q must be one of %s", key, v, quoteList(allowed)), "release", key)
			case slices.Index(values, v) != i:
				add(fmt.Sprintf("release.%s names %q twice", key, v), "release", key)
			}
		}
	}
	list(r.Targets, ReleaseTargets, "targets")
	list(r.Arches, ReleaseArches, "arches")
	if r.GitHub != "" && !RepoRE.MatchString(r.GitHub) {
		add(fmt.Sprintf(`release.github %q must be a repository, "owner/repo"`, r.GitHub), "release", "github")
	}
	if !httpURL(r.GitHubAPI) {
		add(fmt.Sprintf("release.github_api %q must be an http or https URL, such as https://api.github.com or https://github.example.com/api/v3", r.GitHubAPI), "release", "github_api")
	}
	if !tagPrefixRE.MatchString(r.TagPrefix) || strings.Contains(r.TagPrefix, "..") || strings.HasPrefix(r.TagPrefix, "/") || strings.HasPrefix(r.TagPrefix, "-") {
		add(fmt.Sprintf(`release.tag_prefix %q cannot begin a git tag; use letters, digits, ".", "_", "-" and "/", such as "v"`, r.TagPrefix), "release", "tag_prefix")
	}
	for i, v := range r.Publish {
		switch {
		case !slices.Contains(ReleaseChannels, v):
			add(fmt.Sprintf("release.publish: %q must be one of %s", v, quoteList(ReleaseChannels)), "release", "publish")
		case slices.Index(r.Publish, v) != i:
			add(fmt.Sprintf("release.publish names %q twice", v), "release", "publish")
		}
	}
	if strings.ContainsAny(r.Notes, "\n\r") {
		add("release.notes must name one file", "release", "notes")
	}

	h := r.Homebrew
	if h.Tap != "" && !RepoRE.MatchString(h.Tap) {
		add(fmt.Sprintf(`release.homebrew.tap %q must be the tap's repository, "owner/homebrew-name"`, h.Tap), "release", "homebrew", "tap")
	}
	if h.Tap == "" && slices.Contains(r.Publish, "homebrew") {
		add(`release.publish names "homebrew", which needs release.homebrew.tap, "owner/homebrew-name"`, "release", "publish")
	}
	if h.Cask != "" && !CaskTokenRE.MatchString(h.Cask) {
		add(fmt.Sprintf("release.homebrew.cask %q is not a cask token: lowercase letters, digits and single hyphens, such as my-app", h.Cask), "release", "homebrew", "cask")
	}
	if h.Homepage != "" && !httpURL(h.Homepage) {
		add(fmt.Sprintf("release.homebrew.homepage %q must be an http or https URL", h.Homepage), "release", "homebrew", "homepage")
	}
	if strings.ContainsAny(h.Desc, "\n\r") {
		add("release.homebrew.desc must be one line", "release", "homebrew", "desc")
	}

	c := r.Chocolatey
	if c.ID != "" && !ChocoIDRE.MatchString(c.ID) {
		add(fmt.Sprintf("release.chocolatey.id %q is not a package ID: lowercase letters and digits, joined by single dots or hyphens, such as my-app", c.ID), "release", "chocolatey", "id")
	}
	for _, u := range []struct{ value, key string }{
		{c.ProjectURL, "project_url"}, {c.LicenseURL, "license_url"}, {c.IconURL, "icon_url"},
	} {
		if u.value != "" && !httpURL(u.value) {
			add(fmt.Sprintf("release.chocolatey.%s %q must be an http or https URL", u.key, u.value), "release", "chocolatey", u.key)
		}
	}
	if !httpURL(c.Source) {
		add(fmt.Sprintf("release.chocolatey.source %q must be an http or https URL, such as https://push.chocolatey.org/", c.Source), "release", "chocolatey", "source")
	}
	for _, t := range c.Tags {
		if t == "" || strings.ContainsAny(t, " \t,\n") {
			add(fmt.Sprintf("release.chocolatey.tags: %q must be one tag without spaces or commas; give each as its own string", t), "release", "chocolatey", "tags")
		}
	}
	if strings.ContainsAny(c.Summary, "\n\r") {
		add("release.chocolatey.summary must be one line", "release", "chocolatey", "summary")
	}
	return ps
}

func httpURL(s string) bool {
	return strings.HasPrefix(s, "https://") && len(s) > len("https://") ||
		strings.HasPrefix(s, "http://") && len(s) > len("http://")
}

// quoteList writes a, b and c as "a", "b" or "c".
func quoteList(vs []string) string {
	q := make([]string, len(vs))
	for i, v := range vs {
		q[i] = strconv.Quote(v)
	}
	if len(q) == 1 {
		return q[0]
	}
	return strings.Join(q[:len(q)-1], ", ") + " or " + q[len(q)-1]
}

func hasExt(path string, exts ...string) bool {
	return slices.Contains(exts, strings.ToLower(filepath.Ext(path)))
}

// plistValue reports whether v can go in an Info.plist. Every TOML value
// can but dates and the floats a plist has no form for: a table inside an
// array of tables is fine, and a value mixing kinds is still a single
// plist type per element.
func plistValue(v any) error {
	switch v := v.(type) {
	case string, bool, int64:
		return nil
	case float64:
		if math.IsInf(v, 0) || math.IsNaN(v) {
			return fmt.Errorf("%v has no Info.plist form", v)
		}
		return nil
	case []any:
		for _, e := range v {
			if err := plistValue(e); err != nil {
				return err
			}
		}
		return nil
	case []map[string]any:
		for _, e := range v {
			if err := plistValue(e); err != nil {
				return err
			}
		}
		return nil
	case map[string]any:
		for _, e := range v {
			if err := plistValue(e); err != nil {
				return err
			}
		}
		return nil
	default:
		// TOML's dates and times; Info.plist keys never take them.
		return fmt.Errorf("a %T cannot be written to Info.plist", v)
	}
}
