package ui

import (
	"time"

	"github.com/ironpark/ggui"
)

// HoverCardWidget shows a panel of content near its anchor once the cursor
// has rested on it, and keeps it up while the cursor is on either one.
// Build one with HoverCard.
//
// It is Tooltip's larger relative: a preview of what the anchor points at,
// rather than a line of help. The content is painted through an overlay,
// so widgets inside it stay interactive, but nothing about it takes focus:
// a hover card is an aside, and the keyboard never has to visit it.
type HoverCardWidget struct {
	anchor, content ggui.Widget
	delay           time.Duration
	width           float64
	name            string

	panel *ggui.BoxWidget
	env   ggui.Env
	theme ggui.Theme
	size  ggui.Size
}

// hoverCardState is the open timer and the panel's last place, retained on
// the Canvas so that a card rebuilt every frame neither forgets how long
// the cursor has rested nor loses the cursor as it crosses onto the panel.
type hoverCardState struct {
	since time.Time
	panel ggui.Rect
}

var hoverCardSlot = ggui.NewSlot[hoverCardState]("hover card")

// HoverCard wraps anchor and shows content below it after half a second.
//
//	ui.HoverCard(ui.Button("@ada", nil).Ghost(), ggui.Column(
//		ggui.Title("Ada Lovelace"), ggui.Caption("Joined 1843"),
//	).Gap(4))
func HoverCard(anchor, content ggui.Widget) *HoverCardWidget {
	return &HoverCardWidget{anchor: anchor, content: content, delay: 500 * time.Millisecond, width: 260}
}

// Delay sets how long the cursor must rest before the card appears.
func (h *HoverCardWidget) Delay(d time.Duration) *HoverCardWidget { h.delay = d; return h }

// Width sets the card's width in logical pixels.
func (h *HoverCardWidget) Width(w float64) *HoverCardWidget { h.width = w; return h }

// Named sets the accessible name of the card.
func (h *HoverCardWidget) Named(s string) *HoverCardWidget { h.name = s; return h }

// Layout implements ggui.Widget: the card takes no room, so the anchor's
// size is the widget's. The card itself is not measured here -- it is out
// of sight on all but a few frames, and measuring it would mean a layout
// pass over the whole content every frame to answer a question nobody is
// asking yet. Paint measures it once it is about to show.
func (h *HoverCardWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	h.env, h.theme = env, env.Theme()
	if h.panel == nil {
		h.panel = ggui.Box(h.content)
	}
	panelBox(h.panel, h.theme).Radius(h.theme.RadiusLg)
	return h.anchor.Layout(c, env)
}

// Paint implements ggui.Widget.
func (h *HoverCardWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	dst.Paint(h.anchor, r)
	at := ggui.Anchor{Rect: r}
	state, _ := dst.Retained(at, hoverCardSlot)
	p, ok := dst.Pointer()
	// The panel counts as the card too, so moving onto it keeps it open;
	// its place is known only from the frame that painted it.
	if !ok || !(r.Contains(p) || state.panel.Contains(p)) {
		dst.Retain(at, hoverCardSlot, hoverCardState{})
		return
	}
	now := ggui.Now()
	if state.since.IsZero() {
		state.since = now
	}
	if now.Sub(state.since) < h.delay {
		dst.Retain(at, hoverCardSlot, hoverCardState{since: state.since})
		return
	}
	w := max(h.width, 0)
	h.size = h.panel.Layout(ggui.Constraints{MinW: w, MaxW: w, MaxH: ggui.Unbounded}, h.env)
	panel := h.place(dst.Size(), r)
	dst.Retain(at, hoverCardSlot, hoverCardState{since: state.since, panel: panel})
	dst.Overlay(func(dst *ggui.Canvas) {
		dst.DescribeNode(panel, hoverCardPanel{h}, func(dst *ggui.Canvas) {
			dst.Paint(h.panel, panel)
		})
	})
}

// place puts the card under the anchor, or above it when there is no room,
// kept inside the window either way.
func (h *HoverCardWidget) place(screen ggui.Size, anchor ggui.Rect) ggui.Rect {
	gap := h.theme.Space / 2
	at := ggui.Pt(anchor.Origin.X+(anchor.Size.W-h.size.W)/2, anchor.Origin.Y+anchor.Size.H+gap)
	if screen != (ggui.Size{}) {
		at.X = clamp(at.X, 0, max(screen.W-h.size.W, 0))
		if at.Y+h.size.H > screen.H {
			at.Y = max(anchor.Origin.Y-h.size.H-gap, 0)
		}
	}
	return ggui.Rct(at, h.size)
}

// hoverCardPanel names the panel in the accessibility tree without giving
// it any action: there is nothing to press, and nothing to focus.
type hoverCardPanel struct{ h *HoverCardWidget }

func (p hoverCardPanel) Semantics() (ggui.Role, string) { return ggui.RoleGroup, p.h.name }
