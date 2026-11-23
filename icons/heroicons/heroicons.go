// Package heroicons is the whole of the Heroicons icon library, a variable for
// each icon:
//
//	heroicons.ArrowDownTray.Icon().Size(20)
//
// A program is linked with only the icons it names, so importing the
// package costs a binary nothing by itself, and an icon is kept already
// parsed, so drawing it reads no SVG. To look an icon up by a name known
// only at run time, which takes every icon, import the all package beside
// this one.
//
// The ui package draws its controls with a small Heroicons set of its own,
// github.com/ironpark/ggui/ui/icons/heroicons; Set here is the same roles from
// this package's icons.
package heroicons

//go:generate go run github.com/ironpark/ggui/icons/internal/gen -lib heroicons -out .

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
			icons.Close:          XMark,
			icons.ChevronDown:    ChevronDown,
			icons.ChevronUp:      ChevronUp,
			icons.ChevronLeft:    ChevronLeft,
			icons.ChevronRight:   ChevronRight,
			icons.Search:         MagnifyingGlass,
			icons.Alert:          ExclamationCircle,
			icons.Info:           InformationCircle,
			icons.Plus:           Plus,
			icons.Minus:          Minus,
			icons.Download:       ArrowDownTray,
			icons.File:           Document,
			icons.Sun:            Sun,
			icons.Moon:           Moon,
			icons.Grip:           Bars2,
			icons.Loader:         ArrowPath,
			icons.Undo:           ArrowUturnLeft,
			icons.Redo:           ArrowUturnRight,
			icons.MoreHorizontal: EllipsisHorizontal,
			icons.Copy:           DocumentDuplicate,
			icons.ArrowDown:      ArrowDown,
			icons.PanelLeft:      Bars3,
		}
	})
	if g := roleGlyphs[role]; g != nil {
		return g.SVG()
	}
	return nil
}

// Set maps the roles the controls draw to Heroicons's icons, for
// icons.SetKey in a theme or an Env.
func Set() icons.Set { return library{} }
