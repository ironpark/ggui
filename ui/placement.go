package ui

import (
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/widget"
)

// floatSpec is a floating panel to place: of size, on side of anchor and
// gap away from it, lined up along that side by align and offset, in a
// window that reads right to left when rtl is set.
type floatSpec struct {
	anchor      ggui.Rect
	size        ggui.Size
	side        widget.PopupSide
	align       widget.PopupAlignment
	offset, gap float64
	rtl         bool
}

// floatBeside places the floating panel f beside its anchor, as
// widget.Popup places its content: for the tooltip and the hover card,
// which open on hover and so are not popups. The panel goes to the
// opposite side when it does not fit on its side and that one has more
// room, is lined up along the side by its align and offset, and is kept
// widget.PopupCollisionPadding inside screen, as a Base UI positioner
// keeps its popup, unless screen is the zero Size.
//
// It returns the panel's place, the side it ended on -- one of the four
// physical ones -- and how far along that side's edge the panel's entrance
// grows from, for widget.TransitionWidget.PopFrom.
func floatBeside(screen ggui.Size, f floatSpec) (ggui.Rect, widget.PopupSide, float64) {
	side := f.side
	switch side {
	case widget.PopupInlineStart:
		side = pick(f.rtl, widget.PopupRight, widget.PopupLeft)
	case widget.PopupInlineEnd:
		side = pick(f.rtl, widget.PopupLeft, widget.PopupRight)
	}
	const edge = widget.PopupCollisionPadding
	bounded := screen != (ggui.Size{})
	before := f.anchor.Origin.Y - f.gap - edge                               // room above
	after := screen.H - edge - (f.anchor.Origin.Y + f.anchor.Size.H + f.gap) // and below
	need := f.size.H
	horizontal := side == widget.PopupLeft || side == widget.PopupRight
	if horizontal {
		before = f.anchor.Origin.X - f.gap - edge
		after = screen.W - edge - (f.anchor.Origin.X + f.anchor.Size.W + f.gap)
		need = f.size.W
	}
	if bounded {
		leading := side == widget.PopupTop || side == widget.PopupLeft
		room, other := after, before
		if leading {
			room, other = before, after
		}
		if need > room && other > room {
			switch side {
			case widget.PopupTop:
				side = widget.PopupBottom
			case widget.PopupBottom:
				side = widget.PopupTop
			case widget.PopupLeft:
				side = widget.PopupRight
			default:
				side = widget.PopupLeft
			}
		}
	}
	var at ggui.Point
	along := 0.0
	if horizontal {
		at.X = f.anchor.Origin.X + f.anchor.Size.W + f.gap
		if side == widget.PopupLeft {
			at.X = f.anchor.Origin.X - f.gap - f.size.W
		}
		at.Y = f.anchor.Origin.Y
		switch f.align {
		case widget.PopupCenter:
			at.Y += (f.anchor.Size.H-f.size.H)/2 + f.offset
			along = .5
		case widget.PopupEnd:
			at.Y += f.anchor.Size.H - f.size.H - f.offset
			along = 1
		default:
			at.Y += f.offset
		}
	} else {
		at.Y = f.anchor.Origin.Y + f.anchor.Size.H + f.gap
		if side == widget.PopupTop {
			at.Y = f.anchor.Origin.Y - f.gap - f.size.H
		}
		// The alignment is logical: its start is the right edge under RTL.
		at.X = f.anchor.Origin.X
		switch {
		case f.align == widget.PopupCenter:
			at.X += (f.anchor.Size.W-f.size.W)/2 + pick(f.rtl, -f.offset, f.offset)
			along = .5
		case (f.align == widget.PopupEnd) != f.rtl:
			at.X += f.anchor.Size.W - f.size.W - f.offset
			along = 1
		default:
			at.X += f.offset
		}
	}
	if bounded {
		at.X = clamp(at.X, edge, max(screen.W-edge-f.size.W, edge))
		at.Y = clamp(at.Y, edge, max(screen.H-edge-f.size.H, edge))
	}
	return ggui.Rct(at, f.size), side, along
}
