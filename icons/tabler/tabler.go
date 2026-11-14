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

import "github.com/ironpark/ggui/ui/icons"

type library struct{}

// Set maps the roles the controls draw to Tabler's icons, for
// icons.SetKey in a theme or an Env.
func Set() icons.Set { return library{} }

func (library) Resolve(role icons.Role) *icons.SVG {
	switch role {
	case icons.Check:
		return Check.SVG()
	case icons.Close:
		return X.SVG()
	case icons.ChevronDown:
		return ChevronDown.SVG()
	case icons.ChevronUp:
		return ChevronUp.SVG()
	case icons.ChevronLeft:
		return ChevronLeft.SVG()
	case icons.ChevronRight:
		return ChevronRight.SVG()
	case icons.Search:
		return Search.SVG()
	case icons.Alert:
		return AlertCircle.SVG()
	case icons.Info:
		return InfoCircle.SVG()
	case icons.Plus:
		return Plus.SVG()
	case icons.Minus:
		return Minus.SVG()
	case icons.Download:
		return Download.SVG()
	case icons.File:
		return File.SVG()
	case icons.Sun:
		return Sun.SVG()
	case icons.Moon:
		return Moon.SVG()
	case icons.Grip:
		return GripVertical.SVG()
	case icons.Loader:
		return Loader2.SVG()
	case icons.Undo:
		return ArrowBackUp.SVG()
	case icons.Redo:
		return ArrowForwardUp.SVG()
	case icons.MoreHorizontal:
		return Dots.SVG()
	}
	return nil
}
