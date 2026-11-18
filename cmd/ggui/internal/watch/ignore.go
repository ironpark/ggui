package watch

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// Ignore says which paths are not worth waking the scan for, and which
// new directories a source need not watch: .git, the directories given,
// such as ggui build's output, and what git ignores in a git work tree, or,
// outside one or without git, node_modules. It never takes a path out of
// the set the app is built from: a generated file that git ignores but
// the app embeds still rebuilds it.
type Ignore struct {
	dirs []string // always ignored, with all below them
	git  string   // the git program, or "" when there is none

	mu    sync.Mutex
	cache map[string]bool
	tops  map[string]string // a directory's git work tree, or ""
}

// fallbackIgnored are directory names ignored wherever git's rules are not
// to be had.
var fallbackIgnored = []string{"node_modules"}

// ignoreCacheMax bounds the answers kept; past it they are forgotten.
const ignoreCacheMax = 50000

// NewIgnore returns the rules, with dirs and all below them ignored.
func NewIgnore(dirs ...string) *Ignore {
	ig := &Ignore{cache: map[string]bool{}, tops: map[string]string{}}
	for _, d := range dirs {
		if d != "" {
			ig.dirs = append(ig.dirs, filepath.Clean(d))
		}
	}
	ig.git, _ = exec.LookPath("git")
	return ig
}

// Reset forgets the answers, for when a .gitignore changed.
func (ig *Ignore) Reset() {
	if ig == nil {
		return
	}
	ig.mu.Lock()
	ig.cache = map[string]bool{}
	ig.mu.Unlock()
}

// Ignored reports whether path is ignored.
func (ig *Ignore) Ignored(path string) bool {
	return ig.Filter([]string{path})[0]
}

// Filter reports, for each of paths, whether it is ignored, asking git
// once for each work tree.
func (ig *Ignore) Filter(paths []string) []bool {
	out := make([]bool, len(paths))
	if ig == nil {
		return out
	}
	paths = slices.Clone(paths)
	ask := map[string][]int{} // work tree → indexes into paths
	ig.mu.Lock()
	for i, p := range paths {
		p = filepath.Clean(p)
		paths[i] = p
		if slices.Contains(strings.Split(filepath.ToSlash(p), "/"), ".git") ||
			slices.ContainsFunc(ig.dirs, func(d string) bool { return within(d, p) }) {
			out[i] = true
			continue
		}
		if v, ok := ig.cache[p]; ok {
			out[i] = v
			continue
		}
		top := ""
		if ig.git != "" {
			top = ig.top(filepath.Dir(p))
		}
		if top == "" {
			out[i] = fallback(p)
			continue
		}
		ask[top] = append(ask[top], i)
	}
	ig.mu.Unlock()

	for top, idx := range ask {
		ignored, ok := ig.checkIgnore(top, paths, idx)
		ig.mu.Lock()
		if len(ig.cache) > ignoreCacheMax {
			ig.cache = map[string]bool{}
		}
		for _, i := range idx {
			if ok {
				out[i] = ignored[i]
				ig.cache[paths[i]] = out[i]
			} else {
				out[i] = fallback(paths[i])
			}
		}
		ig.mu.Unlock()
	}
	return out
}

// fallback is the rule without git: a path in node_modules is ignored.
func fallback(path string) bool {
	for part := range strings.SplitSeq(filepath.ToSlash(path), "/") {
		if slices.Contains(fallbackIgnored, part) {
			return true
		}
	}
	return false
}

// top returns the git work tree dir is in, or "". ig.mu is held.
func (ig *Ignore) top(dir string) string {
	var walked []string
	top := ""
	for d := dir; ; d = filepath.Dir(d) {
		if t, ok := ig.tops[d]; ok {
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
	for _, d := range walked {
		ig.tops[d] = top
	}
	return top
}

// checkIgnore asks git which of paths[idx] it ignores, by their names
// relative to top, which survive symbolic links in top's own path. ok is
// false when git could not answer.
func (ig *Ignore) checkIgnore(top string, paths []string, idx []int) (ignored map[int]bool, ok bool) {
	var in bytes.Buffer
	byName := map[string]int{}
	for _, i := range idx {
		rel, err := filepath.Rel(top, paths[i])
		if err != nil {
			return nil, false
		}
		rel = filepath.ToSlash(rel)
		byName[rel] = i
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
	ignored = map[int]bool{}
	for name := range bytes.SplitSeq(out, []byte{0}) {
		if i, found := byName[string(name)]; found {
			ignored[i] = true
		}
	}
	return ignored, true
}
