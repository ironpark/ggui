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

import "github.com/ironpark/ggui/ui/icons"

type library struct{}

// Set maps the roles the controls draw to Heroicons's icons, for
// icons.SetKey in a theme or an Env.
func Set() icons.Set { return library{} }

func (library) Resolve(role icons.Role) *icons.SVG {
	switch role {
	case icons.Check:
		return Check.SVG()
	case icons.Close:
		return XMark.SVG()
	case icons.ChevronDown:
		return ChevronDown.SVG()
	case icons.ChevronUp:
		return ChevronUp.SVG()
	case icons.ChevronLeft:
		return ChevronLeft.SVG()
	case icons.ChevronRight:
		return ChevronRight.SVG()
	case icons.Search:
		return MagnifyingGlass.SVG()
	case icons.Alert:
		return ExclamationCircle.SVG()
	case icons.Info:
		return InformationCircle.SVG()
	case icons.Plus:
		return Plus.SVG()
	case icons.Minus:
		return Minus.SVG()
	case icons.Download:
		return ArrowDownTray.SVG()
	case icons.File:
		return Document.SVG()
	case icons.Sun:
		return Sun.SVG()
	case icons.Moon:
		return Moon.SVG()
	case icons.Grip:
		return Bars2.SVG()
	case icons.Loader:
		return ArrowPath.SVG()
	case icons.Undo:
		return ArrowUturnLeft.SVG()
	case icons.Redo:
		return ArrowUturnRight.SVG()
	case icons.MoreHorizontal:
		return EllipsisHorizontal.SVG()
	}
	return nil
}
