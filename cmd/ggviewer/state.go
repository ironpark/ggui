//go:build (unix && !ios && !android) || windows

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
)

// geometry is where the window is, in device-independent pixels: its
// position on its monitor and the size of its client area.
type geometry struct {
	X, Y          float64
	Width, Height float64
}

func (g geometry) valid() bool {
	for _, v := range []float64{g.X, g.Y, g.Width, g.Height} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	return g.Width >= 1 && g.Height >= 1
}

// stateFile is what the viewer remembers of a project between sessions.
type stateFile struct {
	Project string   `json:"project"`
	Window  geometry `json:"window"`
}

// statePath returns the file that keeps project's state under the user's
// cache directory dir: a readable name for the project, and a hash of it
// so that projects of the same base name do not share one.
func statePath(dir, project string) string {
	sum := sha256.Sum256([]byte(project))
	name := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			return r
		}
		return '_'
	}, filepath.Base(project))
	if len(name) > 40 {
		name = name[:40]
	}
	return filepath.Join(dir, "ggui", "ggviewer", name+"-"+hex.EncodeToString(sum[:8])+".json")
}

// loadState returns the window's geometry kept at path for project, or
// false when there is none that is usable.
func loadState(path, project string) (geometry, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return geometry{}, false
	}
	var s stateFile
	if err := json.Unmarshal(b, &s); err != nil || s.Project != project || !s.Window.valid() {
		return geometry{}, false
	}
	return s.Window, true
}

// saveState keeps g at path for project. The file is replaced whole, so a
// viewer killed while it writes leaves the old one.
func saveState(path, project string, g geometry) error {
	b, err := json.MarshalIndent(stateFile{Project: project, Window: g}, "", "\t")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".state-*")
	if err != nil {
		return err
	}
	_, werr := f.Write(append(b, '\n'))
	cerr := f.Close()
	if werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Rename(f.Name(), path)
	}
	if werr != nil {
		_ = os.Remove(f.Name())
	}
	return werr
}
