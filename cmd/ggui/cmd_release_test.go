package main

import (
	"archive/zip"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ironpark/ggui/cmd/ggui/internal/config"
	"github.com/ironpark/ggui/cmd/ggui/internal/project"
	"github.com/ironpark/ggui/cmd/ggui/internal/release"
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
