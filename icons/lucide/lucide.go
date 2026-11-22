// Package lucide is the whole of the Lucide icon library, a variable for
// each icon:
//
//	lucide.Download.Icon().Size(20)
//
// A program is linked with only the icons it names, so importing the
// package costs a binary nothing by itself, and an icon is kept already
// parsed, so drawing it reads no SVG. To look an icon up by a name known
// only at run time, which takes every icon, import the all package beside
// this one.
//
// The ui package draws its controls with a small Lucide set of its own,
// github.com/ironpark/ggui/ui/icons/lucide; Set here is the same roles from
// this package's icons.
package lucide

//go:generate go run github.com/ironpark/ggui/icons/internal/gen -lib lucide -out .

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
		icons.Close:          X.SVG(),
		icons.ChevronDown:    ChevronDown.SVG(),
		icons.ChevronUp:      ChevronUp.SVG(),
		icons.ChevronLeft:    ChevronLeft.SVG(),
		icons.ChevronRight:   ChevronRight.SVG(),
		icons.Search:         Search.SVG(),
		icons.Alert:          CircleExclamationPoint.SVG(), // Lucide 1.73 renamed circle-alert to circle-exclamation-point.
		icons.Info:           Info.SVG(),
		icons.Plus:           Plus.SVG(),
		icons.Minus:          Minus.SVG(),
		icons.Download:       Download.SVG(),
		icons.File:           File.SVG(),
		icons.Sun:            Sun.SVG(),
		icons.Moon:           Moon.SVG(),
		icons.Grip:           GripVertical.SVG(),
		icons.Loader:         LoaderCircle.SVG(),
		icons.Undo:           Undo2.SVG(),
		icons.Redo:           Redo2.SVG(),
		icons.MoreHorizontal: Ellipsis.SVG(),
		icons.Copy:           Copy.SVG(),
		icons.ArrowDown:      ArrowDown.SVG(),
		icons.PanelLeft:      PanelLeft.SVG(),
	}
})

func (library) Resolve(role icons.Role) *icons.SVG { return roles()[role] }

// Set maps the roles the controls draw to Lucide's icons, for
// icons.SetKey in a theme or an Env.
func Set() icons.Set { return library{} }
