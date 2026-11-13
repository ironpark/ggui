package ui

import "github.com/ironpark/ggui"

// floatBeside places a floating panel of the given size on side of anchor,
// gap away from it, as ggui.Popup places its content: for the tooltip and
// the hover card, which open on hover and so are not popups. The panel goes
// to the opposite side when it does not fit on side and that one has more
// room, is lined up along the side by align and offset, and is kept inside
// screen, unless screen is the zero Size.
//
// It returns the panel's place, the side it ended on -- one of the four
// physical ones -- and how far along that side's edge the panel's entrance
// grows from, for ggui.TransitionWidget.PopFrom.
func floatBeside(screen ggui.Size, anchor ggui.Rect, size ggui.Size, side ggui.PopupSide, align ggui.PopupAlignment, offset, gap float64, rtl bool) (ggui.Rect, ggui.PopupSide, float64) {
	switch side {
	case ggui.PopupInlineStart:
		side = pick(rtl, ggui.PopupRight, ggui.PopupLeft)
	case ggui.PopupInlineEnd:
		side = pick(rtl, ggui.PopupLeft, ggui.PopupRight)
	}
	bounded := screen != (ggui.Size{})
	before := anchor.Origin.Y - gap                             // room above
	after := screen.H - (anchor.Origin.Y + anchor.Size.H + gap) // and below
	need := size.H
	horizontal := side == ggui.PopupLeft || side == ggui.PopupRight
	if horizontal {
		before = anchor.Origin.X - gap
		after = screen.W - (anchor.Origin.X + anchor.Size.W + gap)
		need = size.W
	}
	if bounded {
		leading := side == ggui.PopupTop || side == ggui.PopupLeft
		room, other := after, before
		if leading {
			room, other = before, after
		}
		if need > room && other > room {
			switch side {
			case ggui.PopupTop:
				side = ggui.PopupBottom
			case ggui.PopupBottom:
				side = ggui.PopupTop
			case ggui.PopupLeft:
				side = ggui.PopupRight
			default:
				side = ggui.PopupLeft
			}
		}
	}
	var at ggui.Point
	along := 0.0
	if horizontal {
		at.X = anchor.Origin.X + anchor.Size.W + gap
		if side == ggui.PopupLeft {
			at.X = anchor.Origin.X - gap - size.W
		}
		at.Y = anchor.Origin.Y
		switch align {
		case ggui.PopupCenter:
			at.Y += (anchor.Size.H-size.H)/2 + offset
			along = .5
		case ggui.PopupEnd:
			at.Y += anchor.Size.H - size.H - offset
			along = 1
		default:
			at.Y += offset
		}
	} else {
		at.Y = anchor.Origin.Y + anchor.Size.H + gap
		if side == ggui.PopupTop {
			at.Y = anchor.Origin.Y - gap - size.H
		}
		// The alignment is logical: its start is the right edge under RTL.
		at.X = anchor.Origin.X
		switch {
		case align == ggui.PopupCenter:
			at.X += (anchor.Size.W-size.W)/2 + pick(rtl, -offset, offset)
			along = .5
		case (align == ggui.PopupEnd) != rtl:
			at.X += anchor.Size.W - size.W - offset
			along = 1
		default:
			at.X += offset
		}
	}
	if bounded {
		at.X = clamp(at.X, 0, max(screen.W-size.W, 0))
		at.Y = clamp(at.Y, 0, max(screen.H-size.H, 0))
	}
	return ggui.Rct(at, size), side, along
}
