// Package chat provides conversation and AI chat widgets for ggui:
// Message, Bubble, Marker, Attachment, AttachmentGroup, MessageScroller,
// Questionnaire, and Trace.
//
// Controls inherit design tokens from ui/theme (including Theme.Chat geometry)
// and localized strings from ui.ProvideStrings / ui.BindStrings.
package chat

import (
	"cmp"
	"image/color"
	"maps"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/gem"
	"github.com/ironpark/ggui/ui"
	"github.com/ironpark/ggui/ui/icons"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// loopStep is how often an endless ambient animation (such as text shimmer)
// steps, matching ui's spinner/skeleton loop cadence so concurrent loops share
// the same frames.
const loopStep = time.Second / 30

func pick[T any](cond bool, a, b T) T { return gem.Pick(cond, a, b) }

func bounded(v, fallback float64) float64 { return gem.Bounded(v, fallback) }

func clamp[T cmp.Ordered](v, lo, hi T) T { return gem.Clamp(v, lo, hi) }

func fade(c color.Color, f float64) color.Color { return uitheme.Fade(c, f) }

func mix(a, b color.Color, amount float64) color.Color { return uitheme.Mix(a, b, amount) }

// isDark reports whether c is closer to black than to white.
func isDark(c color.Color) bool {
	r, g, b, _ := c.RGBA()
	return r+g+b < 3*32768
}

func colorOr(value, fallback color.Color) color.Color {
	if value != nil {
		return value
	}
	return fallback
}

// squareGlyph is the box a checkbox or radio draws itself in.
func squareGlyph(t *uitheme.Theme) ggui.Size { return ggui.Sz(t.ControlSize, t.ControlSize) }

// ease is Canvas.EaseWith along the theme's Ease curve. A nil t uses the
// default theme curve.
func ease(dst *ggui.Canvas, t *uitheme.Theme, at ggui.Anchor, s ggui.Slot[*ggui.Motion], target float64, d time.Duration) float64 {
	var c uitheme.Curve
	if t != nil {
		c = t.Ease
	}
	return dst.EaseWith(at, s, target, d, c.Easing())
}

// haloStroke draws the three-pixel halo just outside r in col.
func haloStroke(dst *ggui.Canvas, r ggui.Rect, radius float64, col color.Color) {
	dst.StrokeRoundRect(ggui.Rct(r.Origin.Add(ggui.Pt(-2, -2)), ggui.Sz(r.Size.W+4, r.Size.H+4)), radius+2, 3, col)
}

// fieldHalo draws the focus ring just outside r at the theme's halo opacity.
func fieldHalo(dst *ggui.Canvas, t *uitheme.Theme, r ggui.Rect, radius float64, ring color.Color) {
	haloStroke(dst, r, radius, t.Halo(ring))
}

// paintIconAt paints a size-square icon centered on center.
func paintIconAt(dst *ggui.Canvas, env ggui.Env, role icons.Role, center ggui.Point, size float64, col color.Color, rotation float64) {
	ui.PaintIcon(dst, env, role, ggui.Rct(center.Add(ggui.Pt(-size/2, -size/2)), ggui.Sz(size, size)), col, rotation)
}

// paintDisclosure paints content folding open below at: the body is clipped to
// progress of its height and inert while closing.
func paintDisclosure(dst *ggui.Canvas, content ggui.Widget, at ggui.Point, width float64, size ggui.Size, progress float64, open bool) {
	if progress <= 0 {
		return
	}
	body := dst.Clip(ggui.Rct(at, ggui.Sz(width, size.H*progress)))
	if !open {
		body = body.Inert()
	}
	body.Paint(content, ggui.Rct(at, size))
}

// withKey returns a copy of the set m with k added when on and removed
// otherwise, leaving m unchanged.
func withKey[K comparable](m map[K]bool, k K, on bool) map[K]bool {
	next := make(map[K]bool, len(m)+1)
	maps.Copy(next, m)
	if on {
		next[k] = true
	} else {
		delete(next, k)
	}
	return next
}

// beyond reports whether ev is a move a header hears only because the pointer
// is over a region painted inside its own but outside at, the header's Rect.
func beyond(ev ggui.PointerEvent, at ggui.Rect) bool {
	return ev.Kind == ggui.PointerMove && ev.Inner && !at.Contains(ev.Pos)
}

// stepIndex moves cur by dir through n items, wrapping around and skipping
// items enabled rejects (nil rejects none); from -1 it starts at the near end.
func stepIndex(cur, dir, n int, enabled func(int) bool) int {
	i := cur
	for range n {
		if i < 0 {
			i = pick(dir < 0, n-1, 0)
		} else {
			i = (i + dir + n) % n
		}
		if enabled == nil || enabled(i) {
			return i
		}
	}
	return cur
}

// directionalKey maps physical horizontal movement to logical item order.
func directionalKey(ev ggui.KeyEvent, rtl bool) ggui.KeyEvent {
	if rtl {
		switch ev.Key {
		case ggui.KeyArrowLeft:
			ev.Key = ggui.KeyArrowRight
		case ggui.KeyArrowRight:
			ev.Key = ggui.KeyArrowLeft
		}
	}
	return ev
}

// childKey identifies a control a composite builds for itself by the
// composite's identity and the part it plays.
type childKey struct {
	owner, part any
}

// ownerOf is the identity a composite's parts are keyed under.
func ownerOf(i *ggui.Interactive, self any) any {
	if id := i.HitID(); id != nil {
		return id
	}
	return self
}

// traceStatus is how s words the status of a Trace step.
func traceStatus(s *ui.Strings, status TraceStatus) string {
	s = s.OrDefault()
	switch status {
	case TracePending:
		return s.TraceStatusPending
	case TraceRunning:
		return s.TraceStatusRunning
	case TraceCompleted:
		return s.TraceStatusCompleted
	case TraceFailed:
		return s.TraceStatusFailed
	case TraceError:
		return s.TraceStatusError
	case TraceSkipped:
		return s.TraceStatusSkipped
	}
	return string(status)
}

// attachmentState is how s words the upload state of an Attachment.
func attachmentState(s *ui.Strings, state AttachmentState) string {
	s = s.OrDefault()
	switch state {
	case AttachmentIdle:
		return s.AttachmentStatusIdle
	case AttachmentUploading:
		return s.AttachmentStatusUploading
	case AttachmentProcessing:
		return s.AttachmentStatusProcessing
	case AttachmentError:
		return s.AttachmentStatusError
	case AttachmentDone:
		return s.AttachmentStatusDone
	}
	return string(state)
}

// localized lays its child out after apply has worded it in the Strings of the
// Env it is laid out in.
type localized struct {
	child ggui.Widget
	apply func(*ui.Strings)
}

// localize returns child, worded by apply at each layout.
func localize(child ggui.Widget, apply func(*ui.Strings)) *localized {
	return &localized{child: child, apply: apply}
}

func (l *localized) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	l.apply(ui.StringsOf(env))
	return l.child.Layout(c, env)
}

func (l *localized) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(l.child, r) }

// Baseline implements ggui.Baseliner: the child's.
func (l *localized) Baseline() (float64, bool) { return ggui.BaselineOf(l.child) }
