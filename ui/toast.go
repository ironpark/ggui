package ui

import (
	"slices"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/ui/icons"
	"github.com/ironpark/ggui/ui/icons/lucide"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// ToastType is what a notification reports, which picks the icon it leads
// with.
type ToastType uint8

const (
	// ToastPlain is a notice with no icon, the default.
	ToastPlain ToastType = iota
	// ToastSuccess leads with a check.
	ToastSuccess
	// ToastInfo leads with an information mark.
	ToastInfo
	// ToastWarning leads with a warning mark.
	ToastWarning
	// ToastError leads with an error mark in the theme's Destructive color.
	ToastError
	// ToastLoading leads with a spinner.
	ToastLoading
)

// ToastPosition is the corner or edge of the window a Toaster stacks its
// notices at.
type ToastPosition uint8

const (
	// ToastBottomRight stacks upward from the bottom right, the default.
	ToastBottomRight ToastPosition = iota
	// ToastBottomCenter stacks upward from the middle of the bottom edge.
	ToastBottomCenter
	// ToastBottomLeft stacks upward from the bottom left.
	ToastBottomLeft
	// ToastTopRight stacks downward from the top right.
	ToastTopRight
	// ToastTopCenter stacks downward from the middle of the top edge.
	ToastTopCenter
	// ToastTopLeft stacks downward from the top left.
	ToastTopLeft
)

// top reports whether the position hangs from the window's top edge.
func (p ToastPosition) top() bool { return p >= ToastTopRight }

// alignX is where the stack sits across the window: 0 left, 1 right.
func (p ToastPosition) alignX() float64 {
	switch p {
	case ToastBottomCenter, ToastTopCenter:
		return .5
	case ToastBottomLeft, ToastTopLeft:
		return 0
	}
	return 1
}

const (
	// toastWidth is the most a notice is wide, shadcn's max-w-sm.
	toastWidth = 384.0
	// toastSlide is how long a notice takes to come, go or move within
	// the stack; its content fades in half that.
	toastSlide = 500 * time.Millisecond
)

// toastEase is the curve notices move along: fast away, long to settle.
var toastEase = ggui.CubicBezier(.22, 1, .36, 1)

// The roles a toast's type asks an icon set for first. The bundled set has
// none of them yet, so each falls back to the nearest shared role.
const (
	toastSuccessRole icons.Role = "success"
	toastWarningRole icons.Role = "warning"
	toastErrorRole   icons.Role = "error"
)

// ToastMessage describes one notification. Push copies it into a Toaster.
type ToastMessage struct {
	title, description string
	duration           time.Duration
	kind               ToastType
	countdown          bool
	actionLabel        string
	action             func()
}

// Toast creates a notification lasting five seconds after Push.
func Toast(title, description string) ToastMessage {
	return ToastMessage{title: title, description: description, duration: 5 * time.Second}
}

// Duration sets the lifetime. Zero or negative durations require manual dismissal.
func (t ToastMessage) Duration(d time.Duration) ToastMessage { t.duration = d; return t }

// Type sets what the notice reports, which picks its leading icon.
func (t ToastMessage) Type(k ToastType) ToastMessage { t.kind = k; return t }

// Success leads the notice with a check.
func (t ToastMessage) Success() ToastMessage { return t.Type(ToastSuccess) }

// Info leads the notice with an information mark.
func (t ToastMessage) Info() ToastMessage { return t.Type(ToastInfo) }

// Warning leads the notice with a warning mark.
func (t ToastMessage) Warning() ToastMessage { return t.Type(ToastWarning) }

// Error leads the notice with an error mark in the theme's Destructive color.
func (t ToastMessage) Error() ToastMessage { return t.Type(ToastError) }

// Loading leads the notice with a spinner and keeps it until it is
// dismissed or replaced with Update; a later Duration gives it a lifetime.
func (t ToastMessage) Loading() ToastMessage {
	t.duration = 0
	return t.Type(ToastLoading)
}

// Destructive marks the notice as an error, as Error does.
func (t ToastMessage) Destructive() ToastMessage { return t.Error() }

// Countdown draws a thin bar along the notice's bottom edge that empties
// as its lifetime runs out. The bar is painted every frame while it runs.
func (t ToastMessage) Countdown() ToastMessage { t.countdown = true; return t }

// Action adds a button that dismisses the toast before running fn.
func (t ToastMessage) Action(label string, fn func()) ToastMessage {
	t.actionLabel, t.action = label, fn
	return t
}

// ToastID identifies a notification within its Toaster. Zero is never assigned.
type ToastID uint64

// ToasterWidget owns a bounded notification queue. It takes no layout space
// and paints nonmodal notices at the bottom right, or where Position says:
// the newest in front, the older ones peeking out behind it, and the whole
// stack spread out while the pointer or the keyboard is in it. It has no
// timers or goroutines: lifetimes advance with the frame clock when
// painted, and stand still while the stack is spread. Call it only on the
// UI thread.
type ToasterWidget struct {
	props    property.Owner
	entries  []*toastEntry
	next     ToastID
	limit    int
	position ToastPosition
	closed   bool
	expanded bool // spread out: the pointer or the keyboard is in the stack
	env      ggui.Env
}

type toastEntry struct {
	id              ToastID
	title           string
	panel           *toastPanel
	dismiss, action *ButtonWidget
	remaining       time.Duration
	duration        time.Duration
	persistent      bool
	last            time.Time
	held            bool // the stack was spread at the last paint, so no time ran since
	leaving         bool
	behind          bool // collapsed under a newer notice, its content hidden
	exitAt          time.Time
	size            ggui.Size // the panel's own, from the last paint
	hit             ggui.Rect // where it took the pointer at the last paint

	// Where the notice is drawn. enter is 0 off the window's edge and 1
	// in the stack; near is how far its near edge stands from the stack's
	// anchor line; scale, height and shown (the content's opacity) are
	// what a notice behind the front one gives up.
	enter, near, scale, height, shown ggui.Motion
}

// Stable identity prevents motion from announcing the same live region again.
func (e *toastEntry) HitID() any          { return e }
func (e *toastEntry) Describe() ggui.Node { return ggui.Node{Role: ggui.RoleStatus, Name: e.title} }

// HandlePointer implements ggui.PointerHandler. A notice blocks clicks on
// its surface but leaves the rest of the window live.
func (e *toastEntry) HandlePointer(ev ggui.PointerEvent) bool {
	return ev.Kind != ggui.PointerScroll
}

// engaged reports whether the pointer, at p when known, or the keyboard is
// on the notice. The pointer is read where it is rather than followed by
// its events, so a notice that slides in under a resting pointer is held
// too, and the paint that sees the pointer arrive is the one from which
// the lifetimes stand still.
func (e *toastEntry) engaged(p ggui.Point, known bool) bool {
	return known && e.hit.Contains(p) || e.dismiss.Focused || (e.action != nil && e.action.Focused)
}

// NewToaster creates a queue with room for three notices. Keep it outside the
// rebuilding Builder, or create it in Component setup and register
// ggui.OnCleanup(toaster.Close). There is no process-global notification state.
func NewToaster() *ToasterWidget { return &ToasterWidget{limit: 3} }

// Limit sets the queue size (at least one); excess oldest notices are discarded.
func (t *ToasterWidget) Limit(n int) *ToasterWidget {
	defer property.Watch(&t.props, &t.limit)()
	t.limit = max(1, n)
	t.trim()
	return t
}

// Position stacks the notices at p instead of the bottom right. Notices
// come in from, and leave over, the edge they stack at.
func (t *ToasterWidget) Position(p ToastPosition) *ToasterWidget {
	defer property.Watch(&t.props, &t.position)()
	t.position = p
	return t
}

func (t *ToasterWidget) trim() {
	if len(t.entries) > t.limit {
		t.entries = slices.DeleteFunc(t.entries, func(e *toastEntry) bool { return e.leaving })
	}
	if len(t.entries) > t.limit {
		t.entries = slices.Delete(t.entries, 0, len(t.entries)-t.limit)
	}
}

// Push appends a notification and returns its ID, or zero after Close.
func (t *ToasterWidget) Push(message ToastMessage) ToastID {
	if t.closed {
		return 0
	}
	t.next++
	e := &toastEntry{id: t.next}
	for _, m := range []*ggui.Motion{&e.enter, &e.near, &e.scale, &e.height, &e.shown} {
		m.Easing(toastEase)
	}
	t.fill(e, message)
	e.enter.MoveTo(0, e.last, 0)
	t.entries = append(t.entries, e)
	t.trim()
	return e.id
}

// Update replaces the notice id with message where it stands and starts
// its lifetime again: a loading notice becomes the result it waited for.
// It reports whether id was still showing.
func (t *ToasterWidget) Update(id ToastID, message ToastMessage) bool {
	for _, e := range t.entries {
		if e.id == id && !e.leaving {
			t.fill(e, message)
			return true
		}
	}
	return false
}

// fill gives e the content and lifetime of message.
func (t *ToasterWidget) fill(e *toastEntry, message ToastMessage) {
	e.title = message.title
	e.remaining, e.duration, e.persistent = message.duration, message.duration, message.duration <= 0
	e.last, e.held = ggui.Now(), false
	closeIcon := Icon(icons.Close)
	e.dismiss = ButtonOf(closeIcon, func() { t.Dismiss(e.id) }).Variant(toastClose).Size(ButtonIconSmall).Name("Dismiss " + message.title)
	e.dismiss.Key(e)
	e.action = nil
	if message.actionLabel != "" {
		e.action = Button(message.actionLabel, func() {
			if e.leaving {
				return
			}
			t.Dismiss(e.id)
			if message.action != nil {
				message.action()
			}
		}).Outline().Size(ButtonSmall)
		e.action.Key(&e.action)
	}
	e.panel = &toastPanel{entry: e, title: ggui.Text(message.title), closeIcon: closeIcon, kind: message.kind, countdown: message.countdown}
	if message.description != "" {
		e.panel.description = ggui.Text(message.description)
	}
}

// toastClose is the close button of a notice: a ghost button whose mark
// is muted.
var toastClose = NewVariant("toast-close", func(t uitheme.Theme) ButtonStyle {
	s := ButtonGhost.Style(t)
	s.Label = t.MutedFg
	return s
})

// Dismiss starts the exit animation. The notice immediately stops accepting
// input and no longer counts toward Len. An unknown ID is a no-op.
func (t *ToasterWidget) Dismiss(id ToastID) {
	for _, e := range t.entries {
		if e.id == id && !e.leaving {
			e.leaving, e.exitAt = true, ggui.Now()
		}
	}
}

// Len returns active notices, excluding those finishing their exit animation.
// Expiration is processed when the host paints.
func (t *ToasterWidget) Len() int {
	n := 0
	for _, e := range t.entries {
		if !e.leaving {
			n++
		}
	}
	return n
}

// Clear drops all notifications and their callbacks.
func (t *ToasterWidget) Clear() {
	clear(t.entries)
	t.entries = nil
}

// Close drops the queue and rejects future Push calls. It is safe to call twice.
func (t *ToasterWidget) Close() { t.closed = true; t.Clear() }

// Layout implements ggui.Widget; put the host anywhere in the root tree.
func (t *ToasterWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer t.props.Layout().EndLayout()
	t.env = env
	return c.Constrain(ggui.Size{})
}

// Paint implements ggui.Widget. Adding a toast does not move keyboard focus.
func (t *ToasterWidget) Paint(dst *ggui.Canvas, _ ggui.Rect) {
	if t.closed || len(t.entries) == 0 {
		return
	}
	// A resting stack asks for no frames. A lifetime asks to be woken
	// when it ends rather than counting every frame down to it, and a
	// notice on its way out is woken for when it has gone; the motions
	// ask for the frames in between themselves. Push, Update and Dismiss
	// read the clock, so the frame after one of them comes.
	now := ggui.FrameTime()
	reduced := t.env.ReducedMotion()
	t.expanded = false
	pointer, known := dst.Pointer()
	for _, e := range t.entries {
		t.expanded = t.expanded || (!e.leaving && e.engaged(pointer, known))
	}
	var wake time.Time
	wakeBy := func(at time.Time) {
		if wake.IsZero() || at.Before(wake) {
			wake = at
		}
	}
	animate := false
	t.entries = slices.DeleteFunc(t.entries, func(e *toastEntry) bool {
		if e.leaving {
			gone := reduced || now.Sub(e.exitAt) >= toastSlide
			if !gone {
				wakeBy(e.exitAt.Add(toastSlide))
			}
			return gone
		}
		if e.persistent {
			return false
		}
		// Time runs between two paints unless the stack was held at the
		// first of them: no frame comes while it is held, so the paint
		// that lets go must not count the hold.
		if !e.held {
			e.remaining -= max(time.Duration(0), now.Sub(e.last))
		}
		e.last, e.held = now, t.expanded
		if e.remaining <= 0 {
			e.leaving, e.exitAt = true, now
			wakeBy(now.Add(toastSlide))
			return reduced
		}
		if !e.held {
			wakeBy(now.Add(e.remaining))
			animate = animate || e.panel.countdown
		}
		return false
	})
	if animate {
		ggui.Now()
	}
	ggui.WakeAt(wake)
	if len(t.entries) > 0 {
		dst.Overlay(t.paintNotices)
	}
}

func (t *ToasterWidget) paintNotices(dst *ggui.Canvas) {
	// The motions ask for frames themselves while they move.
	now := ggui.FrameTime()
	screen := dst.Size()
	theme := uitheme.Of(t.env)
	margin := theme.Space * 2
	if screen.W <= 0 || screen.H <= 0 {
		return
	}
	width := min(toastWidth, max(0, screen.W-margin*2))
	left := margin + (screen.W-margin*2-width)*t.position.alignX()
	// The older notices peek out by, and spread out with a gap of, the
	// same step.
	step := theme.Space * 1.5
	slide, resize, fadeIn := t.env.Motion(toastSlide), t.env.Motion(theme.MotionFast), t.env.Motion(toastSlide/2)
	cs := ggui.Constraints{MinW: width, MaxW: width, MaxH: max(0, screen.H-margin*2)}
	front, stacked, index := -1.0, 0.0, 0
	for _, e := range slices.Backward(t.entries) {
		e.size = e.panel.Layout(cs, t.env)
		if e.leaving {
			// Out over the edge it came in from, from wherever it stood.
			e.enter.MoveTo(0, now, slide)
			e.near.MoveTo(0, now, slide)
			e.scale.MoveTo(1, now, slide)
			continue
		}
		if front < 0 {
			front = e.size.H
		}
		near, scale, height, shown := stacked, 1.0, e.size.H, 1.0
		if !t.expanded {
			// Behind the front notice: its height, a tenth narrower for
			// each step back, its far edge one step past the one before.
			scale = max(0, 1-.1*float64(index))
			near, height = float64(index)*step+(1-scale)*front, front
			if index > 0 {
				shown = 0
			}
		}
		e.behind = shown == 0
		e.enter.MoveTo(1, now, slide)
		e.near.MoveTo(near, now, slide)
		e.scale.MoveTo(scale, now, slide)
		e.height.MoveTo(height, now, resize)
		e.shown.MoveTo(shown, now, fadeIn)
		stacked += e.size.H + step
		index++
	}
	clip := dst.Clip(ggui.Rct(ggui.Pt(0, 0), screen))
	for _, e := range t.entries {
		t.paintNotice(clip, e, theme, now, left, width, step)
	}
}

// paintNotice draws one notice where its motions have it: the surface
// scaled about the middle of its near edge, and the content, when it shows
// at all, scaled and faded with it.
func (t *ToasterWidget) paintNotice(dst *ggui.Canvas, e *toastEntry, theme *uitheme.Theme, now time.Time, left, width, gap float64) {
	screen := dst.Size()
	margin := theme.Space * 2
	scale, height := e.scale.Value(now), e.height.Value(now)
	// Off the edge by one and a half of its height until it has come in.
	near := e.near.Value(now) - (1-e.enter.Value(now))*1.5*height
	top := t.position.top()
	// box is the notice before it is scaled, about the middle of its
	// near edge, into shown.
	box := ggui.Rct(ggui.Pt(left, screen.H-margin-near-height), ggui.Sz(width, height))
	about := ggui.Pt(left+width/2, box.Origin.Y+height)
	if top {
		box.Origin.Y = margin + near
		about.Y = box.Origin.Y
	}
	shown := ggui.Rct(ggui.Pt(about.X-width*scale/2, about.Y-pick(top, 0, height*scale)), ggui.Sz(width*scale, height*scale))
	if scale <= 0 {
		return
	}
	radius := theme.RadiusXl * scale
	dst.Shadow(shown, radius, theme.OverlayShadow)
	dst.FillRoundRect(shown, radius, theme.Popover)
	dst.StrokeRoundRect(shown, radius, theme.BorderWidth, theme.Border)
	e.hit = ggui.Rect{}
	if !e.leaving {
		// Spread out, a notice also takes the pointer over the gap on
		// its near side, so crossing from one to the next does not let
		// go of the stack.
		e.hit = shown
		if t.expanded {
			e.hit.Size.H += gap
			if top {
				e.hit.Origin.Y -= gap
			}
		}
		dst.HitPointer(e.hit, e)
		dst.HitCursor(e.hit, ggui.CursorShapeDefault) // not what the notice lies over asks for
	}
	// A notice is a live region: it appeared without the user asking,
	// so it is announced where it stands rather than waited for.
	dst.DescribeNode(shown, e, func(dst *ggui.Canvas) {
		opacity := e.shown.Value(now)
		if opacity <= 0 && e.behind {
			return
		}
		paint := func(dst *ggui.Canvas) {
			inner := dst.Clip(box)
			if e.leaving || e.behind {
				inner = inner.Inert()
			}
			// The content keeps its own height in a notice held to
			// the front one's, centered and cut off.
			inner.Paint(e.panel, ggui.Rct(ggui.Pt(box.Origin.X, box.Origin.Y+(height-e.size.H)/2), e.size))
		}
		if opacity < 1 {
			content := paint
			paint = func(dst *ggui.Canvas) { dst.Layer(ggui.LayerOptions{Fade: 1 - opacity}, content) }
		}
		if scale != 1 {
			dst.Transform(ggui.Affine{}.Scale(scale, scale).About(about), paint)
			return
		}
		paint(dst)
	})
}

// toastPanel is the content of a notice: an icon for its type, the title
// over the description, an optional action, and a named close button, in
// one row. Colors follow the current theme.
type toastPanel struct {
	entry              *toastEntry
	title, description *ggui.TextWidget
	closeIcon          *icons.Widget
	kind               ToastType
	countdown          bool
	theme              *uitheme.Theme

	// The panel's parts, built once at the first layout (the entry's action
	// and mark never change), so their layout caches survive a relayout.
	body    *ggui.BoxWidget
	rowFlow *ggui.RowWidget
	column  *ggui.ColumnWidget
	parts   []ggui.Widget // the row's children, in order
}

func (p *toastPanel) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	p.theme = uitheme.Of(env)
	t := p.theme
	if p.body == nil {
		text := []ggui.Widget{p.title}
		if p.description != nil {
			text = append(text, p.description)
		}
		p.column = ggui.Column(text...).Align(ggui.AlignStretch)
		if p.kind != ToastPlain {
			p.parts = append(p.parts, &toastMark{kind: p.kind})
		}
		p.parts = append(p.parts, ggui.Expanded(p.column))
		if p.entry.action != nil {
			p.parts = append(p.parts, p.entry.action)
		}
		p.parts = append(p.parts, p.entry.dismiss)
		p.rowFlow = ggui.Row(p.parts...).Align(ggui.AlignCenter)
		p.body = ggui.Box(p.rowFlow)
	}
	p.title.Style(t.Text.Merge(t.Label)).Color(t.PopoverFg)
	if p.description != nil {
		p.description.Style(t.Text).Color(t.MutedFg)
	}
	p.closeIcon.Size(t.IconSize)
	p.column.Gap(t.Space / 2)
	p.rowFlow.Gap(t.Space * 1.5)
	p.body.Pad(t.Space * 2)
	return p.body.Layout(c, env.WithText(ggui.TextStyle{Color: t.PopoverFg}))
}

func (p *toastPanel) Paint(dst *ggui.Canvas, r ggui.Rect) {
	dst.Paint(p.body, r)
	e := p.entry
	if !p.countdown || e.persistent || e.duration <= 0 {
		return
	}
	t := p.theme
	fraction := clamp(float64(e.remaining)/float64(e.duration), 0, 1)
	inset, thick := t.Space*2, t.BorderWidth*2
	bar := ggui.Rct(r.Origin.Add(ggui.Pt(inset, r.Size.H-t.Space/2-thick)), ggui.Sz((r.Size.W-inset*2)*fraction, thick))
	dst.FillRoundRect(bar, thick/2, pick(p.kind == ToastError, t.Destructive, t.Primary))
}

// toastMark is the icon a typed notice leads with, at the theme's icon
// size in the notice's text color: a spinner for one that is loading.
type toastMark struct {
	kind    ToastType
	role    icons.Role
	env     ggui.Env
	theme   *uitheme.Theme
	reduced bool
}

func (m *toastMark) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	m.env, m.theme, m.reduced = env, uitheme.Of(env), env.ReducedMotion()
	want, fallback := icons.Info, icons.Info
	switch m.kind {
	case ToastSuccess:
		want, fallback = toastSuccessRole, icons.Check
	case ToastWarning:
		want, fallback = toastWarningRole, icons.Alert
	case ToastError:
		want, fallback = toastErrorRole, icons.Alert
	case ToastLoading:
		want, fallback = icons.Loader, icons.Loader
	}
	m.role = want
	if want != fallback && icons.Resolve(env, lucide.Set(), want) == nil {
		m.role = fallback
	}
	return c.Constrain(ggui.Sz(m.theme.IconSize, m.theme.IconSize))
}

func (m *toastMark) Paint(dst *ggui.Canvas, r ggui.Rect) {
	phase := 0.0
	if m.kind == ToastLoading && !m.reduced {
		phase = spinPhase()
	}
	paintIcon(dst, m.env, m.role, r, pick(m.kind == ToastError, m.theme.Destructive, m.theme.PopoverFg), phase)
}
