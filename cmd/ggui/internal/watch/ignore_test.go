package watch

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func mkfiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, data := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

type ignoreCase struct {
	path string
	want bool
}

func checkIgnore(t *testing.T, ig *Ignore, root string, cases []ignoreCase) {
	t.Helper()
	paths := make([]string, len(cases))
	for i, c := range cases {
		paths[i] = filepath.Join(root, filepath.FromSlash(c.path))
	}
	got := ig.Filter(paths)
	for i, c := range cases {
		if got[i] != c.want {
			t.Errorf("%s: ignored = %v, want %v", c.path, got[i], c.want)
		}
		// One at a time, from the cache, the same.
		if one := ig.Ignored(paths[i]); one != c.want {
			t.Errorf("%s alone: ignored = %v, want %v", c.path, one, c.want)
		}
	}
}

func TestIgnoreGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	root := t.TempDir()
	if out, err := exec.Command("git", "-C", root, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	mkfiles(t, root, map[string]string{
		".gitignore":     "*.log\n!keep.log\ngenerated/\n",
		"a/.gitignore":   "local/\n",
		"main.go":        "package main\n",
		"a/a.go":         "package a\n",
		"a/local/x.go":   "package local\n",
		"generated/g.go": "package generated\n",
	})
	out := filepath.Join(root, "build")
	ig := NewIgnore(out)
	checkIgnore(t, ig, root, []ignoreCase{
		{"main.go", false},
		{"a/a.go", false},
		{"debug.log", true},
		{"keep.log", false},    // negated
		{"a/trace.log", true},  // a pattern without a slash, at any depth
		{"a/local", true},      // a nested .gitignore
		{"a/local/x.go", true}, // below an ignored directory
		{"local/x.go", false},  // the nested rule is for a/ only
		{"generated/g.go", true},
		{"generated/new/not-yet.go", true}, // a path that does not exist yet
		{".git/index", true},
		{"build/app", true}, // the output directory, not in .gitignore
		{"build", true},
		{"node_modules/x.js", false}, // git's rules replace the fixed list
	})

	// A changed .gitignore counts once the answers are reset.
	mkfiles(t, root, map[string]string{".gitignore": "*.log\n"})
	if !ig.Ignored(filepath.Join(root, "generated", "g.go")) {
		t.Error("the cached answer was not kept until Reset")
	}
	ig.Reset()
	checkIgnore(t, ig, root, []ignoreCase{
		{"generated/g.go", false},
		{"keep.log", true},
	})
}

func TestIgnoreWithoutGit(t *testing.T) {
	root := t.TempDir()
	mkfiles(t, root, map[string]string{".gitignore": "*.go\n", "a.go": "package a\n"})
	ig := NewIgnore(filepath.Join(root, "out"))
	ig.git = "" // as when git is not on PATH
	checkIgnore(t, ig, root, []ignoreCase{
		{"a.go", false}, // .gitignore is git's, unread without it
		{"node_modules/pkg/index.js", true},
		{"web/node_modules", true},
		{"out/app.exe", true},
		{"output/app", false},
		{".git/HEAD", true},
	})
}

// Outside a git work tree, git is not asked, and the fixed list applies.
func TestIgnoreOutsideWorkTree(t *testing.T) {
	root := t.TempDir()
	ig := NewIgnore()
	checkIgnore(t, ig, root, []ignoreCase{
		{"a.go", false},
		{"node_modules/x", true},
	})
}

func TestIgnoreNil(t *testing.T) {
	var ig *Ignore
	if ig.Ignored("/x/.git/HEAD") {
		t.Error("nil Ignore ignored a path")
	}
	ig.Reset()
}
