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

// library is the comparable Set Set returns; the role map is built the
// first time a role is resolved, so a program that never asks for the
// set links none of these icons.
type library struct{}

var roles = sync.OnceValue(func() icons.Map {
	return icons.Map{
		icons.Check:          Check.SVG(),
		icons.Close:          XMark.SVG(),
		icons.ChevronDown:    ChevronDown.SVG(),
		icons.ChevronUp:      ChevronUp.SVG(),
		icons.ChevronLeft:    ChevronLeft.SVG(),
		icons.ChevronRight:   ChevronRight.SVG(),
		icons.Search:         MagnifyingGlass.SVG(),
		icons.Alert:          ExclamationCircle.SVG(),
		icons.Info:           InformationCircle.SVG(),
		icons.Plus:           Plus.SVG(),
		icons.Minus:          Minus.SVG(),
		icons.Download:       ArrowDownTray.SVG(),
		icons.File:           Document.SVG(),
		icons.Sun:            Sun.SVG(),
		icons.Moon:           Moon.SVG(),
		icons.Grip:           Bars2.SVG(),
		icons.Loader:         ArrowPath.SVG(),
		icons.Undo:           ArrowUturnLeft.SVG(),
		icons.Redo:           ArrowUturnRight.SVG(),
		icons.MoreHorizontal: EllipsisHorizontal.SVG(),
		icons.Copy:           DocumentDuplicate.SVG(),
		icons.ArrowDown:      ArrowDown.SVG(),
		icons.PanelLeft:      Bars3.SVG(),
	}
})

func (library) Resolve(role icons.Role) *icons.SVG { return roles()[role] }

// Set maps the roles the controls draw to Heroicons's icons, for
// icons.SetKey in a theme or an Env.
func Set() icons.Set { return library{} }
