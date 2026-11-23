// Package tabler is the whole of the Tabler icon library, a variable for
// each icon:
//
//	tabler.Download.Icon().Size(20)
//
// A program is linked with only the icons it names, so importing the
// package costs a binary nothing by itself, and an icon is kept already
// parsed, so drawing it reads no SVG. To look an icon up by a name known
// only at run time, which takes every icon, import the all package beside
// this one.
//
// The ui package draws its controls with a small Tabler set of its own,
// github.com/ironpark/ggui/ui/icons/tabler; Set here is the same roles from
// this package's icons.
package tabler

//go:generate go run github.com/ironpark/ggui/icons/internal/gen -lib tabler -out .

import (
	"sync"

	"github.com/ironpark/ggui/ui/icons"
)

// library is the comparable Set Set returns. The role table is built the
// first time a role is resolved, inside Resolve and not at package
// initialization, so a program that never asks for the set links none of
// these icons, and each icon is parsed only when its role is drawn.
type library struct{}

var (
	rolesOnce  sync.Once
	roleGlyphs map[icons.Role]*icons.Glyph
)

func (library) Resolve(role icons.Role) *icons.SVG {
	rolesOnce.Do(func() {
		roleGlyphs = map[icons.Role]*icons.Glyph{
			icons.Check:          Check,
			icons.Close:          X,
			icons.ChevronDown:    ChevronDown,
			icons.ChevronUp:      ChevronUp,
			icons.ChevronLeft:    ChevronLeft,
			icons.ChevronRight:   ChevronRight,
			icons.Search:         Search,
			icons.Alert:          AlertCircle,
			icons.Info:           InfoCircle,
			icons.Plus:           Plus,
			icons.Minus:          Minus,
			icons.Download:       Download,
			icons.File:           File,
			icons.Sun:            Sun,
			icons.Moon:           Moon,
			icons.Grip:           GripVertical,
			icons.Loader:         Loader2,
			icons.Undo:           ArrowBackUp,
			icons.Redo:           ArrowForwardUp,
			icons.MoreHorizontal: Dots,
			icons.Copy:           Copy,
			icons.ArrowDown:      ArrowDown,
			icons.PanelLeft:      LayoutSidebar,
		}
	})
	if g := roleGlyphs[role]; g != nil {
		return g.SVG()
	}
	return nil
}

// Set maps the roles the controls draw to Tabler's icons, for
// icons.SetKey in a theme or an Env.
func Set() icons.Set { return library{} }
