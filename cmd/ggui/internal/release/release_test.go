package release

import (
	"archive/zip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNames(t *testing.T) {
	for in, want := range map[string]string{
		"Hello World": "hello-world",
		"Tiny App!":   "tiny-app",
		"ggui":        "ggui",
		"  A & B  ":   "a-b",
		"日本":          "",
	} {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
	if got := MacName("hello", "1.2.0", "universal", "dmg"); got != "hello-1.2.0-macos-universal.dmg" {
		t.Errorf("MacName = %s", got)
	}
	if got := WindowsName("hello", "1.2.0", "arm64"); got != "hello-1.2.0-windows-arm64.zip" {
		t.Errorf("WindowsName = %s", got)
	}
}

func TestSums(t *testing.T) {
	dir := t.TempDir()
	var assets []Asset
	// Fake outputs of a build, named as the release names them.
	for _, name := range []string{"x-1.0.0-windows-amd64.zip", "x-1.0.0-linux-amd64.tar.gz", "x_1.0.0_amd64.deb"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
		a := Asset{Name: name, Path: path}
		if err := a.Hash(); err != nil {
			t.Fatal(err)
		}
		assets = append(assets, a)
	}
	if assets[0].Size != int64(len(assets[0].Name)) {
		t.Errorf("size %d", assets[0].Size)
	}
	// Sorted by name, in sha256sum's format.
	sums, err := WriteSums(dir, assets)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(sums.Path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	want := []string{"x-1.0.0-linux-amd64.tar.gz", "x-1.0.0-windows-amd64.zip", "x_1.0.0_amd64.deb"}
	if len(lines) != len(want) {
		t.Fatalf("SHA256SUMS:\n%s", data)
	}
	for i, l := range lines {
		sum, name, ok := strings.Cut(l, "  ")
		if !ok || name != want[i] || len(sum) != 64 {
			t.Errorf("line %d = %q", i, l)
		}
	}
	if !strings.Contains(string(data), assets[0].SHA256+"  "+assets[0].Name+"\n") {
		t.Errorf("SHA256SUMS lacks %s's sum:\n%s", assets[0].Name, data)
	}
	if sums.Name != SumsName || sums.SHA256 == "" {
		t.Errorf("sums asset %+v", sums)
	}
	if _, err := WriteSums(dir, []Asset{{Name: "unhashed"}}); err == nil {
		t.Error("an asset without a hash was summed")
	}

	var b strings.Builder
	Summary(&b, append(assets, sums))
	if !strings.Contains(b.String(), assets[1].SHA256[:12]) || !strings.HasPrefix(b.String(), "asset") {
		t.Errorf("summary:\n%s", b.String())
	}
}

func TestHumanSize(t *testing.T) {
	for n, want := range map[int64]string{0: "0 B", 999: "999 B", 1500: "1.5 kB", 4_200_000: "4.2 MB"} {
		if got := HumanSize(n); got != want {
			t.Errorf("HumanSize(%d) = %s, want %s", n, got, want)
		}
	}
}

// readZip returns the entries of a zip by name.
func readZip(t *testing.T, path string) map[string]*zip.File {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { zr.Close() })
	m := map[string]*zip.File{}
	for _, f := range zr.File {
		m[f.Name] = f
	}
	return m
}

func contents(t *testing.T, f *zip.File) string {
	t.Helper()
	r, err := f.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestZipDirKeepsParentModesAndLinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symbolic links and modes are Unix's")
	}
	root := t.TempDir()
	app := filepath.Join(root, "Hello World.app")
	bin := filepath.Join(app, "Contents", "MacOS")
	fw := filepath.Join(app, "Contents", "Frameworks", "X.framework")
	for _, d := range []string{bin, filepath.Join(fw, "Versions", "A")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(bin, "hello"), []byte("program"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), []byte("plist"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("Versions/A", filepath.Join(fw, "Current")); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "app.zip")
	if err := ZipDir(app, out); err != nil {
		t.Fatal(err)
	}
	files := readZip(t, out)
	for name := range files {
		if !strings.HasPrefix(name, "Hello World.app/") {
			t.Errorf("entry %q is not below the bundle's own name", name)
		}
	}
	exe := files["Hello World.app/Contents/MacOS/hello"]
	if exe == nil || exe.Mode().Perm() != 0o755 || contents(t, exe) != "program" {
		t.Fatalf("program entry %+v", exe)
	}
	if p := files["Hello World.app/Contents/Info.plist"]; p == nil || p.Mode().Perm() != 0o644 {
		t.Errorf("Info.plist entry %+v", p)
	}
	link := files["Hello World.app/Contents/Frameworks/X.framework/Current"]
	if link == nil || link.Mode()&fs.ModeSymlink == 0 || contents(t, link) != "Versions/A" {
		t.Errorf("link entry %+v", link)
	}
	if d := files["Hello World.app/Contents/"]; d == nil || !d.Mode().IsDir() {
		t.Errorf("no directory entry for Contents")
	}

	// The same files make the same zip.
	again := filepath.Join(root, "again.zip")
	if err := ZipDir(app, again); err != nil {
		t.Fatal(err)
	}
	a, b := Asset{Path: out}, Asset{Path: again}
	if a.Hash() != nil || b.Hash() != nil || a.SHA256 != b.SHA256 {
		t.Error("zipping twice made two different files")
	}
}

func TestZipFiles(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "Hello World.exe")
	if err := os.WriteFile(exe, []byte("MZ"), 0o755); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "hello-world-1.0.0-windows-amd64.zip")
	if err := ZipFiles(out, exe); err != nil {
		t.Fatal(err)
	}
	files := readZip(t, out)
	if len(files) != 1 || files["Hello World.exe"] == nil || contents(t, files["Hello World.exe"]) != "MZ" {
		t.Errorf("zip has %v", files)
	}
	if err := ZipFiles(filepath.Join(dir, "bad.zip"), filepath.Join(dir, "missing.exe")); err == nil {
		t.Error("zipped a missing file")
	}
	if _, err := os.Stat(filepath.Join(dir, "bad.zip")); err == nil {
		t.Error("a failed zip was left behind")
	}
}

func TestParseRepo(t *testing.T) {
	for _, c := range []struct{ in, host, repo string }{
		{"https://github.com/alice/hello.git", "github.com", "alice/hello"},
		{"https://github.com/alice/hello", "github.com", "alice/hello"},
		{"https://user:secret@github.com/alice/hello.git/", "github.com", "alice/hello"},
		{"git@github.com:alice/hello.git", "github.com", "alice/hello"},
		{"github.com:alice/hello", "github.com", "alice/hello"},
		{"ssh://git@github.com/alice/hello.git", "github.com", "alice/hello"},
		{"ssh://git@ghe.example.com:2222/team/app", "ghe.example.com", "team/app"},
		{"  git@github.com:alice/hello.git\n", "github.com", "alice/hello"},
	} {
		host, repo, err := ParseRepo(c.in)
		if err != nil || host != c.host || repo != c.repo {
			t.Errorf("ParseRepo(%q) = %q, %q, %v; want %q, %q", c.in, host, repo, err, c.host, c.repo)
		}
	}
	for _, bad := range []string{"", "/srv/git/hello.git", "https://github.com/alice", "https://github.com/a/b/c", "git@github.com:"} {
		if _, repo, err := ParseRepo(bad); err == nil {
			t.Errorf("ParseRepo(%q) = %q, want an error", bad, repo)
		}
	}
}

func TestPeeledTag(t *testing.T) {
	annotated := "1111111111111111111111111111111111111111\trefs/tags/v1.0.0\n2222222222222222222222222222222222222222\trefs/tags/v1.0.0^{}\n"
	if got := PeeledTag(annotated, "v1.0.0"); got != strings.Repeat("2", 40) {
		t.Errorf("annotated: %s", got)
	}
	light := "3333333333333333333333333333333333333333\trefs/tags/v1.0.0\n"
	if got := PeeledTag(light, "v1.0.0"); got != strings.Repeat("3", 40) {
		t.Errorf("lightweight: %s", got)
	}
	if got := PeeledTag("", "v1.0.0"); got != "" {
		t.Errorf("none: %s", got)
	}
}

func TestTokenNeverPrinted(t *testing.T) {
	tok := NewToken("ghp_secret", "$GITHUB_TOKEN")
	for _, s := range []string{fmt.Sprint(tok), fmt.Sprintf("%v %+v %#v %s", tok, tok, tok, tok), fmt.Sprintf("%+v", struct{ T Token }{tok})} {
		if strings.Contains(s, "ghp_secret") {
			t.Errorf("token printed: %s", s)
		}
	}
	if tok.Value() != "ghp_secret" {
		t.Error("Value lost the token")
	}

	env := map[string]string{"GH_TOKEN": "b", "GITHUB_TOKEN": "a"}
	got, err := GitHubToken(t.Context(), func(k string) string { return env[k] }, "github.com")
	if err != nil || got.Value() != "a" || got.Source != "$GITHUB_TOKEN" {
		t.Errorf("GITHUB_TOKEN first: %v %v", got, err)
	}
	delete(env, "GITHUB_TOKEN")
	if got, err := GitHubToken(t.Context(), func(k string) string { return env[k] }, "github.com"); err != nil || got.Value() != "b" {
		t.Errorf("then GH_TOKEN: %v %v", got, err)
	}
}
