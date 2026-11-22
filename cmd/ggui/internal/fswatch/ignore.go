package fswatch

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// DefaultPatterns are the names an [Ignore] leaves out: version
// control directories, node_modules, Finder's .DS_Store, and the
// temporary and backup files editors write beside the file they save:
// Emacs's backups, lock files and auto-saves, Vim's swap files and the
// file 4913 it writes to test a directory, and JetBrains' safe-write
// copies.
var DefaultPatterns = []string{
	".git", ".hg", ".svn", "node_modules", ".DS_Store",
	"*~", ".#*", "#*#",
	"*.swp", "*.swo", "*.swx", "4913",
	"*___jb_tmp___", "*___jb_old___",
}

// IgnoreConfig says what an [Ignore] leaves out.
type IgnoreConfig struct {
	// Patterns are matched, with path.Match, against each element of a
	// path: a match leaves out the path and all below it, wherever the
	// element is.
	Patterns []string
	// Dirs are directories left out with all below them, such as a
	// build's output.
	Dirs []string
	// Git also leaves out what git ignores, by its .gitignore files,
	// .git/info/exclude and core.excludesFile, for a path in a git work
	// tree. Git is asked, with git check-ignore, once for each work tree
	// in a call to Filter, and once for each directory in a call to
	// Match, which asks about the directory's other entries meanwhile;
	// answers are kept until [Ignore.Reset].
	Git bool
	// Fallback are patterns, as Patterns, that apply where git's rules
	// do not: without Git, outside a work tree, or where git is not
	// installed or fails.
	Fallback []string
}

// Ignore leaves out paths by their names, by the directories they are
// in, and optionally by what git ignores. Its Match method is an
// [Options.Ignore]. A nil *Ignore leaves out nothing. It is safe for
// concurrent use.
type Ignore struct {
	c   IgnoreConfig
	git string // the git program, or "" when not asked or not found

	mu    sync.Mutex
	cache map[string]bool   // git's answers
	tops  map[string]string // a directory's git work tree, or ""
}

// ignoreCacheMax bounds the answers kept; past it they are forgotten.
const ignoreCacheMax = 50000

// NewIgnore returns the rules c describes.
func NewIgnore(c IgnoreConfig) *Ignore {
	ig := &Ignore{cache: map[string]bool{}, tops: map[string]string{}}
	ig.c.Patterns = append([]string(nil), c.Patterns...)
	ig.c.Fallback = append([]string(nil), c.Fallback...)
	for _, d := range c.Dirs {
		if d != "" {
			ig.c.Dirs = append(ig.c.Dirs, filepath.Clean(d))
		}
	}
	if c.Git {
		ig.c.Git = true
		ig.git, _ = exec.LookPath("git")
	}
	return ig
}

// Reset forgets git's answers, for when a .gitignore changed.
func (ig *Ignore) Reset() {
	if ig == nil {
		return
	}
	ig.mu.Lock()
	ig.cache = map[string]bool{}
	ig.tops = map[string]string{}
	ig.mu.Unlock()
}

// Match reports whether path is left out. isDir is not needed by the
// rules; it is there so that Match is an [Options.Ignore].
func (ig *Ignore) Match(path string, isDir bool) bool {
	return ig.filter([]string{path}, true)[0]
}

// Ignored reports whether path is left out.
func (ig *Ignore) Ignored(path string) bool {
	return ig.filter([]string{path}, true)[0]
}

// Filter reports, for each of paths, whether it is left out, asking git
// at most once for each work tree.
func (ig *Ignore) Filter(paths []string) []bool {
	return ig.filter(paths, false)
}

// filter is Filter; siblings has git asked about the other entries of
// each directory too, so that the next of them is answered from the
// cache.
func (ig *Ignore) filter(paths []string, siblings bool) []bool {
	out := make([]bool, len(paths))
	if ig == nil {
		return out
	}
	clean := make([]string, len(paths))
	ask := map[string][]int{} // work tree → indexes into paths
	var needTop []int
	ig.mu.Lock()
	for i, p := range paths {
		p = filepath.Clean(p)
		clean[i] = p
		if ig.fixed(p) {
			out[i] = true
			continue
		}
		if v, ok := ig.cache[p]; ok {
			out[i] = v
			continue
		}
		if ig.git == "" {
			out[i] = matchAny(ig.c.Fallback, p)
			continue
		}
		if top, ok := ig.tops[filepath.Dir(p)]; ok {
			if top == "" {
				out[i] = matchAny(ig.c.Fallback, p)
			} else {
				ask[top] = append(ask[top], i)
			}
			continue
		}
		needTop = append(needTop, i)
	}
	ig.mu.Unlock()

	for _, i := range needTop {
		top := ig.top(filepath.Dir(clean[i]))
		if top == "" {
			out[i] = matchAny(ig.c.Fallback, clean[i])
			continue
		}
		ask[top] = append(ask[top], i)
	}

	for top, idx := range ask {
		names := make([]string, 0, len(idx))
		for _, i := range idx {
			names = append(names, clean[i])
		}
		if siblings {
			names = append(names, ig.uncachedSiblings(names)...)
		}
		ignored, ok := ig.checkIgnore(top, names)
		ig.mu.Lock()
		if len(ig.cache) > ignoreCacheMax {
			ig.cache = map[string]bool{}
		}
		if ok {
			for _, n := range names {
				ig.cache[n] = ignored[n]
			}
		}
		for _, i := range idx {
			if ok {
				out[i] = ignored[clean[i]]
			} else {
				out[i] = matchAny(ig.c.Fallback, clean[i])
			}
		}
		ig.mu.Unlock()
	}
	return out
}

// fixed reports whether the patterns or the directories leave out p.
func (ig *Ignore) fixed(p string) bool {
	if matchAny(ig.c.Patterns, p) {
		return true
	}
	for _, d := range ig.c.Dirs {
		if within(d, p) {
			return true
		}
	}
	return false
}

// uncachedSiblings returns the entries of the directories of paths that
// have no answer yet.
func (ig *Ignore) uncachedSiblings(paths []string) []string {
	seen := map[string]bool{}
	for _, p := range paths {
		seen[p] = true
	}
	var out []string
	for _, p := range paths {
		dir := filepath.Dir(p)
		if seen[dir+"/"] {
			continue
		}
		seen[dir+"/"] = true
		des, _ := os.ReadDir(dir)
		ig.mu.Lock()
		for _, de := range des {
			s := filepath.Join(dir, de.Name())
			if _, ok := ig.cache[s]; !ok && !seen[s] && !ig.fixed(s) {
				seen[s] = true
				out = append(out, s)
			}
		}
		ig.mu.Unlock()
	}
	return out
}

// matchAny reports whether a pattern matches an element of p.
func matchAny(patterns []string, p string) bool {
	if len(patterns) == 0 {
		return false
	}
	for el := range strings.SplitSeq(filepath.ToSlash(p), "/") {
		if el == "" {
			continue
		}
		for _, pat := range patterns {
			if ok, _ := path.Match(pat, el); ok {
				return true
			}
		}
	}
	return false
}

// top returns the git work tree dir is in, or "". ig.mu must not be held.
func (ig *Ignore) top(dir string) string {
	var walked []string
	top := ""
	for d := dir; ; d = filepath.Dir(d) {
		ig.mu.Lock()
		t, ok := ig.tops[d]
		ig.mu.Unlock()
		if ok {
			top = t
			break
		}
		walked = append(walked, d)
		// A directory or, in a linked work tree or a submodule, a file.
		if _, err := os.Lstat(filepath.Join(d, ".git")); err == nil {
			top = d
			break
		}
		if d == filepath.Dir(d) {
			break
		}
	}
	if len(walked) > 0 {
		ig.mu.Lock()
		if len(ig.tops) > ignoreCacheMax {
			ig.tops = map[string]string{}
		}
		for _, d := range walked {
			ig.tops[d] = top
		}
		ig.mu.Unlock()
	}
	return top
}

// checkIgnore asks git which of paths it ignores, by their names relative
// to top, which survive symbolic links in top's own path. ok is false
// when git could not answer.
func (ig *Ignore) checkIgnore(top string, paths []string) (ignored map[string]bool, ok bool) {
	var in bytes.Buffer
	byName := map[string]string{}
	for _, p := range paths {
		rel, err := filepath.Rel(top, p)
		if err != nil {
			return nil, false
		}
		rel = filepath.ToSlash(rel)
		byName[rel] = p
		in.WriteString(rel)
		in.WriteByte(0)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, ig.git, "-C", top, "check-ignore", "--stdin", "-z")
	cmd.Stdin = &in
	out, err := cmd.Output()
	if err != nil {
		// Status 1 is git's "none of them".
		if ee, isExit := err.(*exec.ExitError); !isExit || ee.ExitCode() != 1 {
			return nil, false
		}
	}
	ignored = map[string]bool{}
	for name := range bytes.SplitSeq(out, []byte{0}) {
		if p, found := byName[string(name)]; found {
			ignored[p] = true
		}
	}
	return ignored, true
}
