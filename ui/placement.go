package ui

import (
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/widget"
)

// floatBeside places a floating panel of the given size on side of anchor,
// gap away from it, as widget.Popup places its content: for the tooltip and
// the hover card, which open on hover and so are not popups. The panel goes
// to the opposite side when it does not fit on side and that one has more
// room, is lined up along the side by align and offset, and is kept
// widget.PopupCollisionPadding inside screen, as a Base UI positioner keeps
// its popup, unless screen is the zero Size.
//
// It returns the panel's place, the side it ended on -- one of the four
// physical ones -- and how far along that side's edge the panel's entrance
// grows from, for widget.TransitionWidget.PopFrom.
func floatBeside(screen ggui.Size, anchor ggui.Rect, size ggui.Size, side widget.PopupSide, align widget.PopupAlignment, offset, gap float64, rtl bool) (ggui.Rect, widget.PopupSide, float64) {
	switch side {
	case widget.PopupInlineStart:
		side = pick(rtl, widget.PopupRight, widget.PopupLeft)
	case widget.PopupInlineEnd:
		side = pick(rtl, widget.PopupLeft, widget.PopupRight)
	}
	const edge = widget.PopupCollisionPadding
	bounded := screen != (ggui.Size{})
	before := anchor.Origin.Y - gap - edge                             // room above
	after := screen.H - edge - (anchor.Origin.Y + anchor.Size.H + gap) // and below
	need := size.H
	horizontal := side == widget.PopupLeft || side == widget.PopupRight
	if horizontal {
		before = anchor.Origin.X - gap - edge
		after = screen.W - edge - (anchor.Origin.X + anchor.Size.W + gap)
		need = size.W
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
		at.X = anchor.Origin.X + anchor.Size.W + gap
		if side == widget.PopupLeft {
			at.X = anchor.Origin.X - gap - size.W
		}
		at.Y = anchor.Origin.Y
		switch align {
		case widget.PopupCenter:
			at.Y += (anchor.Size.H-size.H)/2 + offset
			along = .5
		case widget.PopupEnd:
			at.Y += anchor.Size.H - size.H - offset
			along = 1
		default:
			at.Y += offset
		}
	} else {
		at.Y = anchor.Origin.Y + anchor.Size.H + gap
		if side == widget.PopupTop {
			at.Y = anchor.Origin.Y - gap - size.H
		}
		// The alignment is logical: its start is the right edge under RTL.
		at.X = anchor.Origin.X
		switch {
		case align == widget.PopupCenter:
			at.X += (anchor.Size.W-size.W)/2 + pick(rtl, -offset, offset)
			along = .5
		case (align == widget.PopupEnd) != rtl:
			at.X += anchor.Size.W - size.W - offset
			along = 1
		default:
			at.X += offset
		}
	}
	if bounded {
		at.X = clamp(at.X, edge, max(screen.W-edge-size.W, edge))
		at.Y = clamp(at.Y, edge, max(screen.H-edge-size.H, edge))
	}
	return ggui.Rct(at, size), side, along
}
