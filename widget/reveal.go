package widget

import (
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/property"
)

// RevealWidget shows its child while a condition holds and folds it away
// while it does not: its height eases between nothing and the child's, so
// what lies below slides down to make room and back up after. Build one
// with Reveal.
//
// The child is laid out at its own height and clipped to the share of it
// showing. While it folds away it is painted inert, taking no input, and
// once it is gone it is neither laid out nor painted, so it registers no
// hit region, focus or semantics node. Under reduced motion it opens and
// closes at once.
type RevealWidget struct {
	props      property.Owner
	open       ggui.Readable[bool]
	child      ggui.Widget
	duration   time.Duration
	ease       ggui.Easing
	horizontal bool

	motion   ggui.Motion
	shown    bool      // open, as the last layout read it
	progress float64   // how far open the last layout left it
	size     ggui.Size // the child's own, as laid out; zero while it is gone
}

// Reveal shows child while open is true, easing it open and closed over
// 200ms with EaseOut until Duration and Easing say otherwise. open is read
// as the Reveal is laid out, so pass a signal or something reading one.
// The first layout shows it as open is, without easing.
//
//	more := ggui.State(false)
//	widget.Column(
//		widget.Tap(widget.Text("Details"), func() { ggui.Toggle(more) }),
//		widget.Reveal(more, details),
//	)
func Reveal(open ggui.Readable[bool], child ggui.Widget) *RevealWidget {
	return &RevealWidget{open: open, child: child, duration: 200 * time.Millisecond, ease: ggui.EaseOut}
}

// Duration sets how long opening or closing all the way takes.
func (r *RevealWidget) Duration(d time.Duration) *RevealWidget {
	defer property.Watch(&r.props, &r.duration)()
	r.duration = d
	return r
}

// Easing sets the curve; see ggui.EaseLinear, EaseIn, EaseOut, EaseInOut.
func (r *RevealWidget) Easing(e ggui.Easing) *RevealWidget {
	r.ease = e
	return r
}

// Horizontal eases the width instead of the height: a side panel that
// slides out from its edge and pushes what is beside it along.
func (r *RevealWidget) Horizontal() *RevealWidget {
	defer property.Watch(&r.props, &r.horizontal)()
	r.horizontal = true
	return r
}

// Progress returns how far open the last layout left it, from zero for
// gone to one for all there, for a look that moves with it, such as a
// chevron turning. An overshooting Easing may take it a little past either
// end on the way.
func (r *RevealWidget) Progress() float64 { return r.progress }

// Layout implements ggui.Widget. The child is offered the constraints
// given, less any minimum along the axis it folds on, and is laid out while
// it is open or still on its way closed.
func (r *RevealWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer r.props.Layout().EndLayout()
	r.shown = r.open.Get()
	r.progress = r.motion.Easing(r.ease).LayoutToggle(env, r.shown, r.duration)
	r.size = ggui.Size{}
	if !r.shown && r.progress <= 0 {
		return c.Constrain(ggui.Size{})
	}
	inner, size := c, ggui.Size{}
	if r.horizontal {
		inner.MinW = 0
		r.size = r.child.Layout(inner, env)
		size = ggui.Sz(r.size.W*max(r.progress, 0), r.size.H)
	} else {
		inner.MinH = 0
		r.size = r.child.Layout(inner, env)
		size = ggui.Sz(r.size.W, r.size.H*max(r.progress, 0))
	}
	return c.Constrain(size)
}

// Paint implements ggui.Widget: the child at its own size, clipped to the
// Reveal's Rect, and inert while it folds away.
func (r *RevealWidget) Paint(dst *ggui.Canvas, rect ggui.Rect) {
	if r.progress <= 0 {
		return
	}
	body := dst.Clip(rect)
	if !r.shown {
		body = body.Inert()
	}
	body.Paint(r.child, ggui.Rct(rect.Origin, r.size))
}

// Baseline implements ggui.Baseliner: the child's, once it is all the way
// open.
func (r *RevealWidget) Baseline() (float64, bool) {
	if r.progress < 1 {
		return 0, false
	}
	if b, ok := r.child.(ggui.Baseliner); ok {
		return b.Baseline()
	}
	return 0, false
}
