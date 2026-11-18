package config

import (
	"slices"
	"strings"
	"testing"
)

func TestReleaseValidation(t *testing.T) {
	src := `[release]
targets = ["macos", "web"]
arches = ["amd64", "amd64"]
github = "not a repo"
github_api = "api.github.com"
tag_prefix = "-v"
publish = ["github", "brew", "homebrew"]

[release.homebrew]
cask = "My App"
homepage = "example.com"

[release.chocolatey]
id = "My.App"
icon_url = "ftp://x"
tags = ["two words"]
source = ""
`
	want := []string{
		`release.targets: "web" must be one of "macos", "windows" or "linux"`,
		`release.arches names "amd64" twice`,
		`release.github "not a repo" must be a repository, "owner/repo"`,
		`release.github_api "api.github.com" must be an http or https URL`,
		`release.tag_prefix "-v" cannot begin a git tag`,
		`release.publish: "brew" must be one of "github", "homebrew" or "choco"`,
		`release.publish names "homebrew", which needs release.homebrew.tap`,
		`release.homebrew.cask "My App" is not a cask token`,
		`release.homebrew.homepage "example.com" must be an http or https URL`,
		`release.chocolatey.id "My.App" is not a package ID`,
		`release.chocolatey.icon_url "ftp://x" must be an http or https URL`,
		`release.chocolatey.tags: "two words" must be one tag`,
		`release.chocolatey.source "" must be an http or https URL`,
	}
	es := errorsOf(t, src)
	if len(es) != len(want) {
		t.Errorf("got %d errors, want %d:\n%v", len(es), len(want), es)
	}
	for _, w := range want {
		found := false
		for _, e := range es {
			if strings.HasPrefix(e.Msg, w) {
				found = true
				if e.Line == 0 {
					t.Errorf("%q has no line", e.Msg)
				}
			}
		}
		if !found {
			t.Errorf("no error starting %q in:\n%v", w, es)
		}
	}

	cfg, err := Parse("ggui.toml", []byte(`[release]
targets = ["windows"]
publish = ["github", "homebrew", "choco"]
github = "alice/hello"
[release.homebrew]
tap = "alice/homebrew-tap"
[release.chocolatey]
id = "hello-app"
tags = ["gui", "go"]
`))
	if err != nil {
		t.Fatal(err)
	}
	r := cfg.Release
	if r.TagPrefix != "v" || r.Chocolatey.Source != "https://push.chocolatey.org/" || !slices.Equal(r.Arches, []string{"amd64", "arm64"}) {
		t.Errorf("defaults lost: %+v", r)
	}
	if r.Homebrew.Tap != "alice/homebrew-tap" || !slices.Equal(r.Chocolatey.Tags, []string{"gui", "go"}) {
		t.Errorf("values not read: %+v", r)
	}
}
