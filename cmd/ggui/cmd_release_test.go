package main

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ironpark/ggui/cmd/ggui/internal/config"
	"github.com/ironpark/ggui/cmd/ggui/internal/project"
	"github.com/ironpark/ggui/cmd/ggui/internal/release"
	"github.com/ironpark/ggui/cmd/ggui/internal/release/appcast"
	"github.com/ironpark/ggui/cmd/ggui/internal/release/github"
	"github.com/ironpark/ggui/cmd/ggui/internal/ui"
)

// quiet sends what ggui says to a buffer for the test.
func quiet(t *testing.T) *bytes.Buffer {
	var said bytes.Buffer
	saved := ui.Stderr
	ui.Stderr = ui.New(&said)
	t.Cleanup(func() { ui.Stderr = saved })
	return &said
}

func TestReleaseBuilds(t *testing.T) {
	quiet(t)
	p := &releasePlan{targets: []string{"macos", "windows", "linux"}, arches: []string{"amd64", "arm64", "386", "riscv64"}}
	var got []string
	for _, b := range p.builds() {
		got = append(got, b.target+"/"+b.arch)
	}
	want := []string{"macos/universal", "windows/amd64", "windows/arm64", "windows/386", "linux/amd64", "linux/arm64", "linux/riscv64"}
	if !slices.Equal(got, want) {
		t.Errorf("builds %v, want %v", got, want)
	}
}

// fakePlan is a release of fake build outputs in a temporary directory.
func fakePlan(t *testing.T, cfg config.Config, files map[string][2]string) *releasePlan {
	t.Helper()
	root := t.TempDir()
	p := &releasePlan{
		proj: &project.Project{Root: root, Config: cfg}, version: "1.2.0", tag: "v1.2.0",
		repo: "alice/hello", api: "https://api.github.com", web: "https://github.com",
		dir:  filepath.Join(root, "release"),
		main: &project.Main{Name: "Hello World", ID: "com.example.hello", Executable: "hello"},
		slug: "hello-world", publish: cfg.Release.Publish,
	}
	if err := os.MkdirAll(p.dir, 0o755); err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	slices.Sort(names)
	for _, name := range names {
		src := filepath.Join(work, "built-"+name)
		if err := os.WriteFile(src, []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
		ta := files[name]
		if err := p.add(src, name, ta[0], ta[1]); err != nil {
			t.Fatal(err)
		}
	}
	for i := range p.assets {
		if err := p.assets[i].Hash(); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

func TestReleaseCollectCaskAndChoco(t *testing.T) {
	quiet(t)
	cfg := config.Defaults()
	cfg.Release.Publish = []string{"github", "homebrew", "choco"}
	cfg.Release.Homebrew.Tap = "alice/homebrew-tap"
	cfg.Linux.Description = "Says hello."
	p := fakePlan(t, cfg, map[string][2]string{
		release.MacName("hello-world", "1.2.0", "arm64", "zip"): {"macos", "arm64"},
		release.MacName("hello-world", "1.2.0", "arm64", "dmg"): {"macos", "arm64"},
		release.MacName("hello-world", "1.2.0", "amd64", "zip"): {"macos", "amd64"},
		release.MacName("hello-world", "1.2.0", "amd64", "pkg"): {"macos", "amd64"},
		release.WindowsName("hello-world", "1.2.0", "amd64"):    {"windows", "amd64"},
		release.WindowsName("hello-world", "1.2.0", "arm64"):    {"windows", "arm64"},
		"hello-world-1.2.0-linux-amd64.tar.gz":                  {"linux", "amd64"},
		"hello-world_1.2.0_amd64.deb":                           {"linux", "amd64"},
	})
	sums, err := release.WriteSums(p.dir, p.assets)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(p.dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(p.assets)+1 || sums.Name != "SHA256SUMS" {
		t.Errorf("release directory has %d files for %d assets", len(entries), len(p.assets))
	}

	if err := p.writeCask(); err != nil {
		t.Fatal(err)
	}
	cask, err := os.ReadFile(p.cask)
	if err != nil {
		t.Fatal(err)
	}
	// The disk image for arm64, where there is one; the zip for Intel;
	// never the installer.
	for _, want := range []string{
		`cask "hello-world" do`,
		`url "https://github.com/alice/hello/releases/download/v#{version}/hello-world-#{version}-macos-arm64.dmg"`,
		`hello-world-#{version}-macos-amd64.zip"`,
		`desc "Says hello"`,
		`app "Hello World.app"`,
		`"~/Library/Application Support/com.example.hello"`,
	} {
		if !strings.Contains(string(cask), want) {
			t.Errorf("cask lacks %s:\n%s", want, cask)
		}
	}
	if strings.Contains(string(cask), ".pkg") {
		t.Errorf("cask downloads the installer:\n%s", cask)
	}
	if filepath.Base(filepath.Dir(p.cask)) != "Casks" || filepath.Base(p.cask) != "hello-world.rb" {
		t.Errorf("cask at %s", p.cask)
	}

	if err := p.writeChoco(); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.OpenReader(p.nupkg)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	var install string
	for _, f := range zr.File {
		if f.Name == "tools/chocolateyinstall.ps1" {
			r, _ := f.Open()
			var b bytes.Buffer
			b.ReadFrom(r)
			r.Close()
			install = b.String()
		}
	}
	if !strings.Contains(install, "hello-world-1.2.0-windows-amd64.zip") || strings.Contains(install, "arm64") ||
		!strings.Contains(install, "'Hello World.exe'") {
		t.Errorf("install script:\n%s", install)
	}
	if filepath.Base(p.nupkg) != "hello-world.1.2.0.nupkg" {
		t.Errorf("nupkg %s", p.nupkg)
	}
}

func TestReleaseChannelChecks(t *testing.T) {
	quiet(t)
	cfg := config.Defaults()
	for _, c := range []struct {
		publish, targets []string
		draft            bool
		want             string
	}{
		{[]string{"homebrew"}, []string{"macos"}, false, "release.homebrew.tap"},
		{[]string{"choco"}, []string{"linux"}, false, "needs the windows target"},
		{[]string{"github", "choco"}, []string{"windows"}, true, "--draft"},
	} {
		p := &releasePlan{proj: &project.Project{Config: cfg}, publish: c.publish, targets: c.targets, arches: []string{"amd64"}}
		err := p.checkChannels(releaseOptions{draft: c.draft})
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%v %v: err = %v, want %q", c.publish, c.targets, err, c.want)
		}
	}
}

// git runs git in dir for a test.
func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=T", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false", "-c", "init.defaultBranch=main"}, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestReleaseGitChecks(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	quiet(t)
	remote := t.TempDir()
	git(t, remote, "init", "--bare", "-q")
	dir := t.TempDir()
	git(t, dir, "init", "-q")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "one")
	first := git(t, dir, "rev-parse", "HEAD")
	git(t, dir, "remote", "add", "origin", "git@github.com:alice/hello.git")
	git(t, dir, "remote", "set-url", "--push", "origin", remote)

	github := ""
	plan := func() *releasePlan {
		cfg := config.Defaults()
		cfg.Release.GitHub = github
		return &releasePlan{proj: &project.Project{Root: dir, Config: cfg}, tag: "v1.0.0", publish: []string{"github"}}
	}

	// The repository is read from origin's URL. ls-remote reads origin,
	// so then point origin at the bare repository, and name the GitHub
	// repository in the configuration.
	p := plan()
	if err := p.checkGit(t.Context(), releaseOptions{dryRun: true}); err != nil {
		t.Fatal(err)
	}
	if p.repo != "alice/hello" || p.head != first {
		t.Errorf("repo %q head %q", p.repo, p.head)
	}
	git(t, dir, "remote", "set-url", "origin", remote)
	github = "alice/hello"
	if err := plan().checkGit(t.Context(), releaseOptions{}); err != nil {
		t.Fatalf("clean tree, no tag: %v", err)
	}

	// A change not committed stops a release, unless allowed.
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := plan().checkGit(t.Context(), releaseOptions{})
	if err == nil || !strings.Contains(err.Error(), "b.txt") || !strings.Contains(err.Error(), "--allow-dirty") {
		t.Errorf("dirty: %v", err)
	}
	if err := plan().checkGit(t.Context(), releaseOptions{allowDirty: true}); err != nil {
		t.Errorf("--allow-dirty: %v", err)
	}
	if err := plan().checkGit(t.Context(), releaseOptions{dryRun: true}); err != nil {
		t.Errorf("a dry run stopped at a dirty tree: %v", err)
	}
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "two")

	// The tag on origin at another commit stops it; at this one, not.
	git(t, dir, "tag", "-a", "-m", "v1.0.0", "v1.0.0", first)
	git(t, dir, "push", "-q", "origin", "v1.0.0")
	git(t, dir, "tag", "-d", "v1.0.0")
	err = plan().checkGit(t.Context(), releaseOptions{})
	if err == nil || !strings.Contains(err.Error(), "on origin") || !strings.Contains(err.Error(), first[:12]) {
		t.Errorf("remote tag elsewhere: %v", err)
	}
	git(t, dir, "push", "-q", "origin", ":refs/tags/v1.0.0")
	git(t, dir, "tag", "v1.0.0")
	git(t, dir, "push", "-q", "origin", "v1.0.0")
	if err := plan().checkGit(t.Context(), releaseOptions{}); err != nil {
		t.Errorf("tag at HEAD: %v", err)
	}
	// A local tag elsewhere too.
	git(t, dir, "tag", "-f", "v1.0.0", first)
	if err := plan().checkGit(t.Context(), releaseOptions{}); err == nil || !strings.Contains(err.Error(), "here") {
		t.Errorf("local tag elsewhere: %v", err)
	}
}

// ggui release --dry-run of a small app for Windows and Linux, end to
// end: built, collected, summed, packaged for Chocolatey, and nothing
// sent.
func TestReleaseDryRun(t *testing.T) {
	if testing.Short() {
		t.Skip("cross-compiles a program for Windows and Linux")
	}
	buildEnv(t)
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
	dir := tiny(t)
	toml := filepath.Join(dir, config.FileName)
	data, err := os.ReadFile(toml)
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, []byte("version = \"1.2.0\"\n[linux]\nformats = [\"tar\", \"deb\"]\n[release]\ngithub = \"alice/tiny\"\npublish = [\"github\", \"choco\"]\n")...)
	if err := os.WriteFile(toml, data, 0o644); err != nil {
		t.Fatal(err)
	}
	// The module must resolve for the build to run at all.
	if out, err := exec.Command("go", "list", "-C", dir, ".").CombinedOutput(); err != nil {
		t.Skipf("the test app does not resolve: %v\n%s", err, out)
	}
	out := t.TempDir()
	said := quiet(t)
	// The command prints to the process's standard output.
	capture, err := os.Create(filepath.Join(t.TempDir(), "stdout"))
	if err != nil {
		t.Fatal(err)
	}
	defer capture.Close()
	saved := os.Stdout
	os.Stdout = capture
	var discard, stderr bytes.Buffer
	code := run([]string{"release", dir, "--dry-run", "--targets", "windows,linux", "--arches", "amd64", "--out", out}, &discard, &stderr)
	os.Stdout = saved
	if code != 0 {
		t.Fatalf("exit %d:\n%s%s", code, said, stderr.String())
	}
	stdout, err := os.ReadFile(capture.Name())
	if err != nil {
		t.Fatal(err)
	}
	rel := filepath.Join(out, "release", "1.2.0")
	want := []string{"SHA256SUMS", "chocolatey", "tiny-1.2.0-linux-amd64.tar.gz", "tiny-1.2.0-windows-amd64.zip", "tiny_1.2.0_amd64.deb"}
	entries, err := os.ReadDir(rel)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range entries {
		got = append(got, e.Name())
	}
	if !slices.Equal(got, want) {
		t.Errorf("release directory has %v, want %v", got, want)
	}
	// Nothing of the build's work is left beside it.
	if left, _ := filepath.Glob(filepath.Join(out, ".release-*")); len(left) > 0 {
		t.Errorf("left behind %v", left)
	}
	zr, err := zip.OpenReader(filepath.Join(rel, "tiny-1.2.0-windows-amd64.zip"))
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	if len(zr.File) != 1 || zr.File[0].Name != "Tiny.exe" {
		t.Errorf("the Windows zip has %v", zr.File)
	}
	if _, err := os.Stat(filepath.Join(rel, "chocolatey", "tiny.1.2.0.nupkg")); err != nil {
		t.Error(err)
	}

	sums, err := os.ReadFile(filepath.Join(rel, "SHA256SUMS"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(sums), "\n") != 3 {
		t.Errorf("SHA256SUMS:\n%s", sums)
	}
	s := string(stdout)
	for _, w := range []string{
		"tiny-1.2.0-windows-amd64.zip", "SHA256SUMS",
		"Dry run: nothing was published",
		"POST   https://api.github.com/repos/alice/tiny/releases",
		"POST   https://uploads.github.com/repos/alice/tiny/releases/{id}/assets?name=tiny_1.2.0_amd64.deb",
		"PATCH  https://api.github.com/repos/alice/tiny/releases/{id}",
		"PUT    https://push.chocolatey.org/api/v2/package/",
	} {
		if !strings.Contains(s, w) {
			t.Errorf("standard output lacks %q:\n%s", w, s)
		}
	}
	// The paths of the builds, which go to standard output after a
	// build, are not among the table.
	if strings.Contains(s, ".release-") {
		t.Errorf("a build's path reached standard output:\n%s", s)
	}
	if !strings.Contains(said.String(), "not in a git repository") {
		t.Errorf("no word of the missing repository:\n%s", said)
	}
}

func TestReleaseWithAppcast(t *testing.T) {
	if got := github.ContentType("appcast.xml"); got != "application/xml; charset=utf-8" {
		t.Errorf("github.ContentType(appcast.xml) = %q, want application/xml; charset=utf-8", got)
	}

	pub, priv, err := appcast.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}

	// Serve a prior 1.1.0 appcast over HTTP to test merge_from.
	prevXML, err := appcast.Marshal(&appcast.Feed{
		Title:    "Hello World",
		Language: "en",
		Items: []appcast.Item{{
			Title:        "Version 1.1.0",
			Version:      "1.1.0",
			ShortVersion: "1.1.0",
			PubDate:      time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			Enclosure: appcast.Enclosure{
				URL:         "https://github.com/alice/hello/releases/download/v1.1.0/hello-world-1.1.0-macos-universal.zip",
				Length:      100,
				Type:        appcast.DefaultEnclosureType,
				EdSignature: "prev-sig",
				OS:          "macos",
				Arch:        "universal",
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/appcast.xml":
			w.Header().Set("Content-Type", "application/xml")
			w.Write(prevXML)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	// Pre-build key checks: missing key stops real run, warns on dry-run, invalid key never leaks secret.
	{
		said := quiet(t)
		cfg := config.Defaults()
		cfg.Release.Appcast.Make = true
		p := &releasePlan{proj: &project.Project{Config: cfg}}
		emptyEnv := func(string) string { return "" }
		if err := p.checkAppcastKey(releaseOptions{dryRun: false}, emptyEnv); err == nil || !strings.Contains(err.Error(), "GGUI_UPDATE_PRIVATE_KEY") {
			t.Errorf("missing key real run = %v", err)
		}
		if err := p.checkAppcastKey(releaseOptions{dryRun: true}, emptyEnv); err != nil {
			t.Errorf("missing key dry-run failed: %v", err)
		}
		if !strings.Contains(said.String(), "unsigned preview") {
			t.Errorf("dry-run missing key warning absent:\n%s", said.String())
		}
		badSecret := "TOP_SECRET_INVALID_KEY_VALUE"
		if err := p.checkAppcastKey(releaseOptions{dryRun: false}, func(string) string { return badSecret }); err == nil {
			t.Error("invalid key accepted")
		} else if strings.Contains(err.Error(), badSecret) {
			t.Errorf("invalid key error leaked secret: %v", err)
		}
		if err := p.checkAppcastKey(releaseOptions{dryRun: false}, func(string) string { return priv }); err != nil {
			t.Fatalf("valid key rejected: %v", err)
		}
		if strings.Contains(p.appcastKey.String(), priv) || strings.Contains(p.appcastKey.GoString(), priv) {
			t.Error("releasePlan.appcastKey printed secret")
		}
	}

	// End-to-end collect with signed appcast.xml, SHA256SUMS inclusion, and HTTP merge.
	quiet(t)
	cfg := config.Defaults()
	cfg.Release.Publish = []string{"github"}
	cfg.Release.Appcast.Make = true
	cfg.Release.Appcast.MergeFrom = srv.URL + "/appcast.xml"
	cfg.Release.Appcast.PhasedRollout = "24h"
	cfg.Release.Appcast.Critical = true
	cfg.Release.Appcast.ReleaseNotesLink = "https://github.com/alice/hello/releases/tag/v1.2.0"

	p := fakePlan(t, cfg, map[string][2]string{
		release.MacName("hello-world", "1.2.0", "universal", "zip"): {"macos", "universal"},
		release.MacName("hello-world", "1.2.0", "universal", "pkg"): {"macos", "universal"},
		release.WindowsName("hello-world", "1.2.0", "amd64"):        {"windows", "amd64"},
		release.WindowsSetupName("hello-world", "1.2.0", "amd64"):   {"windows", "amd64"},
		"hello-world-1.2.0-linux-amd64.tar.gz":                      {"linux", "amd64"},
		"hello-world_1.2.0_amd64.deb":                               {"linux", "amd64"},
	})
	if err := p.checkAppcastKey(releaseOptions{}, func(k string) string {
		if k == "GGUI_UPDATE_PRIVATE_KEY" {
			return priv
		}
		return ""
	}); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	all, _, err := p.collect(t.Context(), &stdout)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}

	// appcast.xml must be generated, included in returned assets, and listed in SHA256SUMS.
	appcastPath := filepath.Join(p.dir, "appcast.xml")
	xmlBytes, err := os.ReadFile(appcastPath)
	if err != nil {
		t.Fatalf("appcast.xml not written: %v", err)
	}
	var sawAppcastAsset bool
	for _, a := range all {
		if a.Name == "appcast.xml" && a.SHA256 != "" && a.Size == int64(len(xmlBytes)) {
			sawAppcastAsset = true
		}
	}
	if !sawAppcastAsset {
		t.Errorf("appcast.xml missing or unhashed in collect assets: %+v", all)
	}
	sumsData, err := os.ReadFile(filepath.Join(p.dir, release.SumsName))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(sumsData), "  appcast.xml\n") {
		t.Errorf("SHA256SUMS does not include appcast.xml:\n%s", sumsData)
	}

	feed, err := appcast.Parse(xmlBytes)
	if err != nil {
		t.Fatalf("Parse appcast.xml: %v\n%s", err, xmlBytes)
	}
	// 4 eligible from 1.2.0 (mac zip, win zip, win setup.exe, linux tar.gz) + 1 merged from 1.1.0 = 5.
	if len(feed.Items) != 5 {
		t.Fatalf("got %d items, want 5:\n%s", len(feed.Items), xmlBytes)
	}
	var count120 int
	for _, it := range feed.Items {
		if it.Version != "1.2.0" {
			continue
		}
		count120++
		base := filepath.Base(it.Enclosure.URL)
		if err := appcast.VerifyFile(pub, filepath.Join(p.dir, base), it.Enclosure.EdSignature); err != nil {
			t.Errorf("VerifyFile(%s) failed: %v", base, err)
		}
		if it.PhasedRolloutInterval != 86400 || !it.CriticalUpdate || it.ReleaseNotesLink != cfg.Release.Appcast.ReleaseNotesLink {
			t.Errorf("1.2.0 item metadata mismatch: %+v", it)
		}
		if it.Enclosure.OS == "macos" && it.MinimumSystemVersion != "13.0" {
			t.Errorf("macOS min system version = %q, want 13.0", it.MinimumSystemVersion)
		}
		if it.Enclosure.OS != "macos" && it.MinimumSystemVersion != "" {
			t.Errorf("%s min system version = %q, want empty", it.Enclosure.OS, it.MinimumSystemVersion)
		}
		if strings.HasSuffix(base, "-setup.exe") && it.Enclosure.InstallerArguments != "--silent" {
			t.Errorf("setup.exe installerArguments = %q, want --silent", it.Enclosure.InstallerArguments)
		}
	}
	if count120 != 4 {
		t.Errorf("got %d items for 1.2.0, want 4", count120)
	}

	// Dry-run without key + 404 merge_from warns and generates an unsigned preview appcast.xml.
	said := quiet(t)
	cfg2 := config.Defaults()
	cfg2.Release.Publish = []string{"github"}
	cfg2.Release.Appcast.Make = true
	cfg2.Release.Appcast.MergeFrom = srv.URL + "/missing.xml"
	p2 := fakePlan(t, cfg2, map[string][2]string{
		release.MacName("hello-world", "1.2.0", "universal", "zip"): {"macos", "universal"},
	})
	p2.dryRun = true
	if err := p2.checkAppcastKey(releaseOptions{dryRun: true}, func(string) string { return "" }); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	if _, _, err := p2.collect(t.Context(), &stdout); err != nil {
		t.Fatalf("dry-run unsigned collect: %v", err)
	}
	if !strings.Contains(said.String(), "was not found") {
		t.Errorf("expected 404 merge warning:\n%s", said.String())
	}
	unsignedData, err := os.ReadFile(filepath.Join(p2.dir, "appcast.xml"))
	if err != nil {
		t.Fatal(err)
	}
	unsignedFeed, err := appcast.Parse(unsignedData)
	if err != nil {
		t.Fatal(err)
	}
	if len(unsignedFeed.Items) != 1 || unsignedFeed.Items[0].Enclosure.EdSignature != "" {
		t.Errorf("unsigned dry-run feed = %+v", unsignedFeed.Items)
	}

	// writeAppcast returns a clear error when there are 0 eligible updatable artifacts.
	cfgNoItems := config.Defaults()
	cfgNoItems.Release.Appcast.Make = true
	pNoItems := fakePlan(t, cfgNoItems, map[string][2]string{
		release.MacName("hello-world", "1.2.0", "universal", "pkg"): {"macos", "universal"},
		"hello-world_1.2.0_amd64.deb":                               {"linux", "amd64"},
	})
	pNoItems.dryRun = true
	if err := pNoItems.writeAppcast(t.Context()); err == nil || !strings.Contains(err.Error(), "no updatable release artifacts") {
		t.Errorf("writeAppcast with 0 eligible items = %v, want clear error", err)
	}

	// writeAppcast rejects sub-second phased_rollout durations.
	cfgSubSec := config.Defaults()
	cfgSubSec.Release.Appcast.Make = true
	cfgSubSec.Release.Appcast.PhasedRollout = "500ms"
	pSubSec := fakePlan(t, cfgSubSec, map[string][2]string{
		release.MacName("hello-world", "1.2.0", "universal", "zip"): {"macos", "universal"},
	})
	pSubSec.dryRun = true
	if err := pSubSec.writeAppcast(t.Context()); err == nil || !strings.Contains(err.Error(), "at least 1s") {
		t.Errorf("writeAppcast with 500ms rollout = %v, want duration error", err)
	}

	// releaseCmd.help documents [release.appcast] and appcast.xml.
	if !strings.Contains(releaseCmd.help, "[release.appcast]") || !strings.Contains(releaseCmd.help, "appcast.xml") {
		t.Errorf("releaseCmd.help does not document [release.appcast] / appcast.xml")
	}
}
