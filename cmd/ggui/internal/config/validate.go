package config

import (
	"fmt"
	"math"
	"net/url"
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

// validator holds the configuration under check and the problems found so
// far; each rule appends its own.
type validator struct {
	cfg *Config
	ps  []problem
}

func (v *validator) add(msg string, key ...string) {
	v.ps = append(v.ps, problem{toml.Key(key), msg})
}

// validate checks the values the decoder accepted as types but that the
// build would trip over later, further from where they were written.
func (c *Config) validate() []problem {
	v := validator{cfg: c}
	v.validateApp()
	v.validateBuild()
	v.validateDevDisplay()
	v.validateDevEnv()
	v.validateMacOS()
	v.ps = append(v.ps, v.cfg.MacOS.validateSigning()...)
	v.validatePlist()
	v.ps = append(v.ps, v.cfg.Linux.validate()...)
	v.ps = append(v.ps, v.cfg.Release.validate()...)
	v.validateWebBase()
	v.validateWebWasmOpt()
	v.validateI18nFuncs()
	return v.ps
}

func (v *validator) validateApp() {
	if v.cfg.App.Name != "" && !validAppName(v.cfg.App.Name) {
		v.add(fmt.Sprintf("app.name %q must not contain path separators, \"..\", or control characters", v.cfg.App.Name), "app", "name")
	}
	if v.cfg.App.ID != "" && !bundleID.MatchString(v.cfg.App.ID) {
		v.add(fmt.Sprintf("app.id %q is not a bundle identifier: use reverse-DNS letters, digits, hyphens and dots, such as com.example.hello", v.cfg.App.ID), "app", "id")
	}
	if !versionRE.MatchString(v.cfg.App.Version) {
		v.add(fmt.Sprintf("app.version %q must be one to three numbers separated by dots, such as 1.2.0: macOS reads it as CFBundleShortVersionString", v.cfg.App.Version), "app", "version")
	}
	v.validateAppIcon()
	if v.cfg.App.Package == "" {
		v.add(`app.package must name the main package's directory; "." is the directory of this file`, "app", "package")
	}
}

func (v *validator) validateAppIcon() {
	switch ext := strings.ToLower(filepath.Ext(v.cfg.App.Icon)); {
	case v.cfg.App.Icon == "" || ext == ".png" || ext == ".svg":
		// An SVG is drawn at build time by the project's own ggfx.
	case ext == ".svgz":
		base := filepath.Base(v.cfg.App.Icon)
		v.add(fmt.Sprintf("app.icon %q is a compressed SVG; name the SVG it holds, as gunzip -c %s > %s writes it",
			v.cfg.App.Icon, base, strings.TrimSuffix(base, filepath.Ext(base))+".svg"), "app", "icon")
	default:
		v.add(fmt.Sprintf("app.icon %q must be a PNG file, square, ideally 1024×1024, or an SVG with a square viewBox", v.cfg.App.Icon), "app", "icon")
	}
}

func (v *validator) validateBuild() {
	if v.cfg.Build.Output == "" {
		v.add(`build.output must name a directory, such as "build"`, "build", "output")
	}
	switch v.cfg.Build.Renderer {
	case RendererGo, RendererZig:
	default:
		v.add(fmt.Sprintf(`build.renderer %q must be "go" or "zig"`, v.cfg.Build.Renderer), "build", "renderer")
	}
	for _, list := range []struct {
		tags []string
		key  string
	}{{v.cfg.Build.Tags, "build"}, {v.cfg.Dev.Tags, "dev"}} {
		for _, t := range list.tags {
			if !tagRE.MatchString(t) {
				v.add(fmt.Sprintf("%s.tags: %q is not a build tag; give each tag as its own string", list.key, t), list.key, "tags")
			}
		}
	}
}

func (v *validator) validateDevDisplay() {
	switch v.cfg.Dev.Display {
	case "", "remote", "window":
	default:
		v.add(fmt.Sprintf(`dev.display %q must be "remote" or "window"`, v.cfg.Dev.Display), "dev", "display")
	}
}

func (v *validator) validateDevEnv() {
	for k := range v.cfg.Dev.Env {
		if k == "" || strings.ContainsAny(k, "= \t") {
			v.add(fmt.Sprintf("dev.env: %q is not an environment variable name", k), "dev", "env")
		}
	}
}

func (v *validator) validateMacOS() {
	if v.cfg.MacOS.MinimumSystemVersion != "" && !macVersion.MatchString(v.cfg.MacOS.MinimumSystemVersion) {
		v.add(fmt.Sprintf("macos.minimum_system_version %q must be a version such as 11.0", v.cfg.MacOS.MinimumSystemVersion), "macos", "minimum_system_version")
	}
	if v.cfg.MacOS.Category != "" && !strings.HasPrefix(v.cfg.MacOS.Category, "public.app-category.") {
		v.add(fmt.Sprintf("macos.category %q must be one of Apple's public.app-category.* types, such as public.app-category.productivity", v.cfg.MacOS.Category), "macos", "category")
	}
	if v.cfg.MacOS.SigningIdentity == "" {
		v.add(`macos.signing_identity must not be empty: "-" signs ad hoc`, "macos", "signing_identity")
	}
}

func (v *validator) validatePlist() {
	for k, val := range v.cfg.MacOS.Plist {
		if err := plistValue(val); err != nil {
			v.add(fmt.Sprintf("macos.plist.%s: %v", k, err), "macos", "plist", k)
		}
	}
}

func (v *validator) validateWebBase() {
	if !strings.HasPrefix(v.cfg.Web.Base, "/") {
		v.add(fmt.Sprintf("web.base %q must be an absolute path such as / or /app/", v.cfg.Web.Base), "web", "base")
	}
}

func (v *validator) validateWebWasmOpt() {
	for f := range strings.FieldsSeq(v.cfg.Web.WasmOpt) {
		if !strings.HasPrefix(f, "-") {
			v.add(fmt.Sprintf("web.wasm_opt %q must be wasm-opt flags such as -O2 or -Oz, or empty to skip it", v.cfg.Web.WasmOpt), "web", "wasm_opt")
			break
		}
	}
}

func (v *validator) validateI18nFuncs() {
	for i, key := range []string{"funcs", "plurals"} {
		list := v.cfg.I18n.Funcs
		if i == 1 {
			list = v.cfg.I18n.Plurals
		}
		for _, spec := range list {
			if _, err := i18n.ParseFunc(spec, i == 1); err != nil {
				v.add(fmt.Sprintf("i18n.%s: %v", key, err), "i18n", key)
			}
		}
	}
}

var (
	envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	teamID  = regexp.MustCompile(`^[A-Z0-9]{10}$`)
)

// validateSigning checks the keys that sign, notarize and package.
// Whether there is a certificate or a password to use depends on the
// environment of the build, so that is checked there.
func (m *MacOS) validateSigning() []problem {
	v := validator{cfg: &Config{MacOS: *m}}
	v.validateEnvVars()
	v.validateNotary()
	v.validateDmg()
	v.validatePkg()
	return v.ps
}

func (v *validator) validateEnvVars() {
	for _, e := range []struct {
		value string
		key   []string
	}{
		{v.cfg.MacOS.P12Base64Env, []string{"macos", "p12_base64_env"}},
		{v.cfg.MacOS.P12PasswordEnv, []string{"macos", "p12_password_env"}},
		{v.cfg.MacOS.Notary.PasswordEnv, []string{"macos", "notary", "password_env"}},
	} {
		if e.value != "" && !envName.MatchString(e.value) {
			v.add(fmt.Sprintf("%s %q must name an environment variable, such as GGUI_P12_PASSWORD; the secret itself never goes in this file",
				strings.Join(e.key, "."), e.value), e.key...)
		}
	}
}

func (v *validator) validateNotary() {
	n := v.cfg.MacOS.Notary
	if n.AppleID != "" && n.TeamID == "" {
		v.add("macos.notary.apple_id needs macos.notary.team_id, the ten-character ID of the team the app is signed for", "macos", "notary", "apple_id")
	}
	if n.TeamID != "" && !teamID.MatchString(n.TeamID) {
		v.add(fmt.Sprintf("macos.notary.team_id %q must be ten capital letters and digits, as in the signing identity's parentheses", n.TeamID), "macos", "notary", "team_id")
	}
	if d, err := time.ParseDuration(n.Timeout); err != nil || d <= 0 {
		v.add(fmt.Sprintf("macos.notary.timeout %q must be a duration such as 30m or 2h", n.Timeout), "macos", "notary", "timeout")
	}
}

func (v *validator) validateDmg() {
	d := v.cfg.MacOS.DMG
	if d.Icon != "" && !hasExt(d.Icon, ".png", ".icns") {
		v.add(fmt.Sprintf("macos.dmg.icon %q must be a PNG or an .icns file", d.Icon), "macos", "dmg", "icon")
	}
	if d.Background != "" && !hasExt(d.Background, ".png", ".jpg", ".jpeg", ".tif", ".tiff") {
		v.add(fmt.Sprintf("macos.dmg.background %q must be a PNG, JPEG or TIFF image", d.Background), "macos", "dmg", "background")
	}
	if d.Window.Width <= 0 || d.Window.Height <= 0 {
		v.add(fmt.Sprintf("macos.dmg.window is %d×%d; give a width and height in points, such as { width = 640, height = 480 }", d.Window.Width, d.Window.Height), "macos", "dmg", "window")
	}
	if d.IconSize < dmg.MinIconSize || d.IconSize > dmg.MaxIconSize {
		v.add(fmt.Sprintf("macos.dmg.icon_size %d must be from %d to %d", d.IconSize, dmg.MinIconSize, dmg.MaxIconSize), "macos", "dmg", "icon_size")
	}
	if d.LabelSize < dmg.MinLabelSize || d.LabelSize > dmg.MaxLabelSize {
		v.add(fmt.Sprintf("macos.dmg.label_size %d must be from %d to %d", d.LabelSize, dmg.MinLabelSize, dmg.MaxLabelSize), "macos", "dmg", "label_size")
	}
	v.validateDmgPositions()
	if _, err := dmg.ParseFileSystem(d.FS); err != nil {
		v.add(fmt.Sprintf("macos.dmg.fs %q must be one of %s", d.FS, strings.Join(dmg.FileSystemNames, ", ")), "macos", "dmg", "fs")
	}
	if _, err := udif.ParseFormat(d.Format); err != nil {
		v.add(fmt.Sprintf(`macos.dmg.format %q must be "udzo" (zlib, which every macOS reads) or "ulfo" (lzfse, smaller, macOS 10.11 and later)`, d.Format), "macos", "dmg", "format")
	}
}

func (v *validator) validateDmgPositions() {
	d := v.cfg.MacOS.DMG
	for _, p := range []struct {
		pos []int
		key string
	}{{d.AppPosition, "app_position"}, {d.ApplicationsPosition, "applications_position"}} {
		if p.pos != nil && (len(p.pos) != 2 || p.pos[0] < 0 || p.pos[1] < 0) {
			v.add(fmt.Sprintf("macos.dmg.%s must be [x, y], the icon's centre in points from the window's top left", p.key), "macos", "dmg", p.key)
		}
	}
}

func (v *validator) validatePkg() {
	if !strings.HasPrefix(v.cfg.MacOS.PKG.InstallLocation, "/") {
		v.add(fmt.Sprintf("macos.pkg.install_location %q must be an absolute path, such as /Applications", v.cfg.MacOS.PKG.InstallLocation), "macos", "pkg", "install_location")
	}
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
	v := validator{cfg: &Config{Linux: *l}}
	v.validateLinuxFormats()
	v.validateLinuxDescription()
	v.validateLinuxGenericName()
	v.validateLinuxCategories()
	v.validateLinuxMIMETypes()
	v.validateLinuxMaintainer()
	v.validateLinuxHomepage()
	v.validateLinuxLicense()
	v.validateLinuxDepends()
	return v.ps
}

func (v *validator) validateLinuxFormats() {
	l := &v.cfg.Linux
	for i, f := range l.Formats {
		switch {
		case !slices.Contains(LinuxFormats, f):
			v.add(fmt.Sprintf(`linux.formats: %q must be "tar", "deb" or "rpm"`, f), "linux", "formats")
		case slices.Index(l.Formats, f) != i:
			v.add(fmt.Sprintf("linux.formats names %q twice", f), "linux", "formats")
		}
	}
}

func (v *validator) validateLinuxDescription() {
	if strings.ContainsAny(v.cfg.Linux.Description, "\n\r") {
		v.add("linux.description must be one line", "linux", "description")
	}
}

func (v *validator) validateLinuxGenericName() {
	if strings.ContainsAny(v.cfg.Linux.GenericName, "\n\r") {
		v.add("linux.generic_name must be one line", "linux", "generic_name")
	}
}

func (v *validator) validateLinuxCategories() {
	for _, c := range v.cfg.Linux.Categories {
		if !categoryRE.MatchString(c) {
			v.add(fmt.Sprintf("linux.categories: %q is not a menu category; use the registered names, such as Utility, Graphics or Development, each as its own string", c), "linux", "categories")
		}
	}
}

func (v *validator) validateLinuxMIMETypes() {
	for _, m := range v.cfg.Linux.MIMETypes {
		if !mimeTypeRE.MatchString(m) {
			v.add(fmt.Sprintf("linux.mime_types: %q is not a MIME type such as image/png or x-scheme-handler/myapp", m), "linux", "mime_types")
		}
	}
}

func (v *validator) validateLinuxMaintainer() {
	if v.cfg.Linux.Maintainer != "" && !maintainerRE.MatchString(v.cfg.Linux.Maintainer) {
		v.add(fmt.Sprintf(`linux.maintainer %q must be a name and an email address, such as "Jane Doe <jane@example.com>"`, v.cfg.Linux.Maintainer), "linux", "maintainer")
	}
}

func (v *validator) validateLinuxHomepage() {
	if v.cfg.Linux.Homepage != "" && !httpURL(v.cfg.Linux.Homepage) {
		v.add(fmt.Sprintf("linux.homepage %q must be an http or https URL", v.cfg.Linux.Homepage), "linux", "homepage")
	}
}

func (v *validator) validateLinuxLicense() {
	if v.cfg.Linux.License != "" && !licenseRE.MatchString(v.cfg.Linux.License) {
		v.add(fmt.Sprintf("linux.license %q must be an SPDX license expression, such as MIT or Apache-2.0", v.cfg.Linux.License), "linux", "license")
	}
}

func (v *validator) validateLinuxDepends() {
	for _, d := range []struct {
		list []string
		key  string
	}{{v.cfg.Linux.DependsDeb, "depends_deb"}, {v.cfg.Linux.DependsRPM, "depends_rpm"}} {
		for _, p := range d.list {
			if strings.TrimSpace(p) == "" || strings.ContainsAny(p, ",\n") {
				v.add(fmt.Sprintf("linux.%s: %q must name one dependency; give each as its own string", d.key, p), "linux", d.key)
			}
		}
	}
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
	v := validator{cfg: &Config{Release: *r}}
	v.validateReleaseList(r.Targets, ReleaseTargets, "targets")
	v.validateReleaseList(r.Arches, ReleaseArches, "arches")
	v.validateReleaseGitHub()
	v.validateReleaseGitHubAPI()
	v.validateReleaseTagPrefix()
	v.validateReleaseEntries(r.Publish, ReleaseChannels, "publish")
	v.validateReleaseNotes()
	v.validateReleaseHomebrew()
	v.validateReleaseChocolatey()
	v.validateReleaseAppcast()
	return v.ps
}

// validateReleaseList checks a [release] list that must not be empty: at
// least one entry, each one of allowed, none twice.
func (v *validator) validateReleaseList(values, allowed []string, key string) {
	if len(values) == 0 {
		v.add(fmt.Sprintf("release.%s must name at least one of %s", key, quoteList(allowed)), "release", key)
	}
	v.validateReleaseEntries(values, allowed, key)
}

// validateReleaseEntries checks the entries of a [release] list, which may
// be empty: each one of allowed, none twice.
func (v *validator) validateReleaseEntries(values, allowed []string, key string) {
	for i, val := range values {
		switch {
		case !slices.Contains(allowed, val):
			v.add(fmt.Sprintf("release.%s: %q must be one of %s", key, val, quoteList(allowed)), "release", key)
		case slices.Index(values, val) != i:
			v.add(fmt.Sprintf("release.%s names %q twice", key, val), "release", key)
		}
	}
}

func (v *validator) validateReleaseGitHub() {
	if v.cfg.Release.GitHub != "" && !RepoRE.MatchString(v.cfg.Release.GitHub) {
		v.add(fmt.Sprintf(`release.github %q must be a repository, "owner/repo"`, v.cfg.Release.GitHub), "release", "github")
	}
}

func (v *validator) validateReleaseGitHubAPI() {
	if !httpURL(v.cfg.Release.GitHubAPI) {
		v.add(fmt.Sprintf("release.github_api %q must be an http or https URL, such as https://api.github.com or https://github.example.com/api/v3", v.cfg.Release.GitHubAPI), "release", "github_api")
	}
}

func (v *validator) validateReleaseTagPrefix() {
	if !tagPrefixRE.MatchString(v.cfg.Release.TagPrefix) || strings.Contains(v.cfg.Release.TagPrefix, "..") || strings.HasPrefix(v.cfg.Release.TagPrefix, "/") || strings.HasPrefix(v.cfg.Release.TagPrefix, "-") {
		v.add(fmt.Sprintf(`release.tag_prefix %q cannot begin a git tag; use letters, digits, ".", "_", "-" and "/", such as "v"`, v.cfg.Release.TagPrefix), "release", "tag_prefix")
	}
}

func (v *validator) validateReleaseNotes() {
	if strings.ContainsAny(v.cfg.Release.Notes, "\n\r") {
		v.add("release.notes must name one file", "release", "notes")
	}
}

func (v *validator) validateReleaseHomebrew() {
	h := v.cfg.Release.Homebrew
	if h.Tap != "" && !RepoRE.MatchString(h.Tap) {
		v.add(fmt.Sprintf(`release.homebrew.tap %q must be the tap's repository, "owner/homebrew-name"`, h.Tap), "release", "homebrew", "tap")
	}
	if h.Tap == "" && slices.Contains(v.cfg.Release.Publish, "homebrew") {
		v.add(`release.publish names "homebrew", which needs release.homebrew.tap, "owner/homebrew-name"`, "release", "publish")
	}
	if h.Cask != "" && !CaskTokenRE.MatchString(h.Cask) {
		v.add(fmt.Sprintf("release.homebrew.cask %q is not a cask token: lowercase letters, digits and single hyphens, such as my-app", h.Cask), "release", "homebrew", "cask")
	}
	if h.Homepage != "" && !httpURL(h.Homepage) {
		v.add(fmt.Sprintf("release.homebrew.homepage %q must be an http or https URL", h.Homepage), "release", "homebrew", "homepage")
	}
	if strings.ContainsAny(h.Desc, "\n\r") {
		v.add("release.homebrew.desc must be one line", "release", "homebrew", "desc")
	}
}

func (v *validator) validateReleaseChocolatey() {
	c := v.cfg.Release.Chocolatey
	if c.ID != "" && !ChocoIDRE.MatchString(c.ID) {
		v.add(fmt.Sprintf("release.chocolatey.id %q is not a package ID: lowercase letters and digits, joined by single dots or hyphens, such as my-app", c.ID), "release", "chocolatey", "id")
	}
	for _, u := range []struct{ value, key string }{
		{c.ProjectURL, "project_url"}, {c.LicenseURL, "license_url"}, {c.IconURL, "icon_url"},
	} {
		if u.value != "" && !httpURL(u.value) {
			v.add(fmt.Sprintf("release.chocolatey.%s %q must be an http or https URL", u.key, u.value), "release", "chocolatey", u.key)
		}
	}
	if !httpURL(c.Source) {
		v.add(fmt.Sprintf("release.chocolatey.source %q must be an http or https URL, such as https://push.chocolatey.org/", c.Source), "release", "chocolatey", "source")
	}
	for _, t := range c.Tags {
		if t == "" || strings.ContainsAny(t, " \t,\n") {
			v.add(fmt.Sprintf("release.chocolatey.tags: %q must be one tag without spaces or commas; give each as its own string", t), "release", "chocolatey", "tags")
		}
	}
	if strings.ContainsAny(c.Summary, "\n\r") {
		v.add("release.chocolatey.summary must be one line", "release", "chocolatey", "summary")
	}
}

func (v *validator) validateReleaseAppcast() {
	a := v.cfg.Release.Appcast
	if a.File == "" || a.File == "." || a.File == ".." || strings.ContainsAny(a.File, `/\`) || strings.Contains(a.File, "..") || strings.ContainsAny(a.File, "\n\r\x00") {
		v.add(fmt.Sprintf(`release.appcast.file %q must be a simple file name without path separators or "..", such as "appcast.xml"`, a.File), "release", "appcast", "file")
	}
	if !envName.MatchString(a.PrivateKeyEnv) {
		v.add(fmt.Sprintf(`release.appcast.private_key_env %q must name an environment variable, such as GGUI_UPDATE_PRIVATE_KEY; the private key itself never goes in this file`, a.PrivateKeyEnv), "release", "appcast", "private_key_env")
	}
	if a.MaxItems <= 0 {
		v.add(fmt.Sprintf("release.appcast.max_items %d must be greater than zero", a.MaxItems), "release", "appcast", "max_items")
	}
	if a.PhasedRollout != "" {
		if d, err := time.ParseDuration(a.PhasedRollout); err != nil || d < time.Second {
			v.add(fmt.Sprintf(`release.appcast.phased_rollout %q must be a duration of at least 1s, such as "24h" or "168h"`, a.PhasedRollout), "release", "appcast", "phased_rollout")
		}
	}
	if a.ReleaseNotesLink != "" && !httpURL(a.ReleaseNotesLink) {
		v.add(fmt.Sprintf("release.appcast.release_notes_link %q must be an http or https URL", a.ReleaseNotesLink), "release", "appcast", "release_notes_link")
	}
	if a.Channel != "" && strings.ContainsAny(a.Channel, " \t\n\r") {
		v.add(fmt.Sprintf("release.appcast.channel %q must not contain spaces or newlines", a.Channel), "release", "appcast", "channel")
	}
	if a.MergeFrom != "" {
		if strings.ContainsAny(a.MergeFrom, "\r\n\x00") {
			v.add("release.appcast.merge_from must be one path or URL without control characters", "release", "appcast", "merge_from")
		} else if strings.Contains(a.MergeFrom, "://") && !httpURL(a.MergeFrom) {
			v.add(fmt.Sprintf("release.appcast.merge_from %q must be a local path or an http or https URL", a.MergeFrom), "release", "appcast", "merge_from")
		}
	}
}

func httpURL(s string) bool {
	if s == "" || strings.ContainsAny(s, "\r\n\x00") {
		return false
	}
	u, err := url.Parse(s)
	if err != nil {
		return false
	}
	scheme := strings.ToLower(u.Scheme)
	return (scheme == "http" || scheme == "https") && u.Host != ""
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
	case []any, []map[string]any:
		return plistValues(v)
	case map[string]any:
		return plistValues(v)
	default:
		// TOML's dates and times; Info.plist keys never take them.
		return fmt.Errorf("a %T cannot be written to Info.plist", v)
	}
}

// plistValues reports whether every element of a plist array or table
// can go in an Info.plist.
func plistValues(v any) error {
	switch v := v.(type) {
	case []any:
		for _, e := range v {
			if err := plistValue(e); err != nil {
				return err
			}
		}
	case []map[string]any:
		for _, e := range v {
			if err := plistValue(e); err != nil {
				return err
			}
		}
	case map[string]any:
		for _, e := range v {
			if err := plistValue(e); err != nil {
				return err
			}
		}
	}
	return nil
}
