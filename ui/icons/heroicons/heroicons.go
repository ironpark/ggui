// Package heroicons embeds a curated Heroicons SVG set for GGUI controls.
package heroicons

import (
	"embed"
	"fmt"
	"sync"

	"github.com/ironpark/ggui/ui/icons"
)

//go:embed svg/*.svg
var files embed.FS
var assets sync.Map
var loadMu sync.Mutex

// Asset loads a named SVG once. Names omit the .svg suffix (for example "x").
func Asset(name string) (*icons.SVG, error) {
	if a, ok := assets.Load(name); ok {
		return a.(*icons.SVG), nil
	}
	loadMu.Lock()
	defer loadMu.Unlock()
	if a, ok := assets.Load(name); ok {
		return a.(*icons.SVG), nil
	}
	a, err := icons.Load(files, "svg/"+name+".svg")
	if err != nil {
		return nil, fmt.Errorf("heroicons %q: %w", name, err)
	}
	assets.Store(name, a)
	return a, nil
}

// Icon creates a widget for a named bundled icon. Unknown names panic; use
// Asset when the name comes from external input and an error is preferable.
func Icon(name string) *icons.Widget {
	a, err := Asset(name)
	if err != nil {
		panic(err)
	}
	return icons.New(a)
}

type library struct{}

// Set returns the immutable mapping from common roles to Heroicons icons.
func Set() icons.Set { return library{} }
func (library) Resolve(role icons.Role) *icons.SVG {
	name, ok := roles[role]
	if !ok {
		return nil
	}
	a, err := Asset(name)
	if err != nil {
		panic(err)
	}
	return a
}

var roles = map[icons.Role]string{
	icons.Check:        "check",
	icons.Close:        "x-mark",
	icons.ChevronDown:  "chevron-down",
	icons.ChevronUp:    "chevron-up",
	icons.ChevronLeft:  "chevron-left",
	icons.ChevronRight: "chevron-right",
	icons.Search:       "magnifying-glass",
	icons.Alert:        "exclamation-circle",
	icons.Info:         "information-circle",
	icons.Plus:         "plus",
	icons.Minus:        "minus",
	icons.Download:     "arrow-down-tray",
	icons.File:         "document",
	icons.Sun:          "sun",
	icons.Moon:         "moon",
	icons.Grip:         "bars-2",
	icons.Loader:       "arrow-path",
}
