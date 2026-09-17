package ui

import (
	"slices"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ironpark/ggui"
)

// ToastMessage describes one notification. Push copies it into a Toaster.
type ToastMessage struct {
	title, description string
	duration           time.Duration
	destructive        bool
	actionLabel        string
	action             func()
}

// Toast creates a notification lasting five seconds after Push.
func Toast(title, description string) ToastMessage {
	return ToastMessage{title: title, description: description, duration: 5 * time.Second}
}

// Duration sets the lifetime. Zero or negative durations require manual dismissal.
func (t ToastMessage) Duration(d time.Duration) ToastMessage { t.duration = d; return t }

// Destructive uses the theme's Destructive color for the notice.
func (t ToastMessage) Destructive() ToastMessage { t.destructive = true; return t }

// Action adds a button that dismisses the toast before running fn.
func (t ToastMessage) Action(label string, fn func()) ToastMessage {
	t.actionLabel, t.action = label, fn
	return t
}

// ToastID identifies a notification within its Toaster. Zero is never assigned.
type ToastID uint64

// ToasterWidget owns a bounded notification queue. It takes no layout space and
// paints nonmodal notices at the bottom right. It has no timers or goroutines:
// lifetimes advance with ggui.Now when painted. Call it only on the UI thread.
type ToasterWidget struct {
	entries []*toastEntry
	next    ToastID
	limit   int
	closed  bool
	env     ggui.Env
	buffer  *ebiten.Image
}
type toastEntry struct {
	id               ToastID
	title            string
	panel            *toastPanel
	dismiss, action  *ButtonWidget
	remaining        time.Duration
	persistent       bool
	last             time.Time
	hover            bool
	leaving          bool
	exitAt           time.Time
	motion, position ggui.Motion
	duration         time.Duration
}

// Stable identity prevents motion from announcing the same live region again.
func (e *toastEntry) HitID() any          { return e }
func (e *toastEntry) Describe() ggui.Node { return ggui.Node{Role: ggui.RoleStatus, Name: e.title} }

// NewToaster creates a queue with room for three notices. Keep it outside the
// rebuilding Builder, or create it in Component setup and register
// ggui.OnCleanup(toaster.Close). There is no process-global notification state.
func NewToaster() *ToasterWidget { return &ToasterWidget{limit: 3} }

// Limit sets the queue size (at least one); excess oldest notices are discarded.
func (t *ToasterWidget) Limit(n int) *ToasterWidget { t.limit = max(1, n); t.trim(); return t }
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
	e := &toastEntry{id: t.next, title: message.title, remaining: message.duration, persistent: message.duration <= 0, last: ggui.Now()}
	e.motion.MoveTo(0, e.last, 0)
	e.duration = message.duration
	e.dismiss = Button("×", func() { t.Dismiss(e.id) }).Ghost().Pad(2, 8).Label("Dismiss " + message.title)
	e.dismiss.Key(e)

	if message.actionLabel != "" {
		e.action = Button(message.actionLabel, func() {
			if e.leaving {
				return
			}
			t.Dismiss(e.id)
			if message.action != nil {
				message.action()
			}
		}).Secondary()
		e.action.Key(&e.action)

	}
	e.panel = &toastPanel{entry: e, title: ggui.Text(message.title), description: ggui.Text(message.description), destructive: message.destructive}
	t.entries = append(t.entries, e)
	t.trim()
	return e.id
}

const toastMotionDuration = 240 * time.Millisecond

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
	if t.buffer != nil {
		t.buffer.Deallocate()
		t.buffer = nil
	}
}

// Close drops the queue and rejects future Push calls. It is safe to call twice.
func (t *ToasterWidget) Close() { t.closed = true; t.Clear() }

// Layout implements ggui.Widget; put the host anywhere in the root tree.
func (t *ToasterWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	t.env = env
	return c.Constrain(ggui.Size{})
}

// Paint implements ggui.Widget. Adding a toast does not move keyboard focus.
func (t *ToasterWidget) Paint(dst *ggui.Canvas, _ ggui.Rect) {
	if t.closed {
		return
	}
	now := ggui.Now()
	t.entries = slices.DeleteFunc(t.entries, func(e *toastEntry) bool {
		if e.leaving {
			return t.env.ReducedMotion() || now.Sub(e.exitAt) >= toastMotionDuration
		}
		paused := e.hover || e.dismiss.Focused || e.dismiss.Hovered || (e.action != nil && (e.action.Focused || e.action.Hovered))
		if !e.persistent && !paused {
			e.remaining -= max(time.Duration(0), now.Sub(e.last))
		}
		e.last = now
		if !e.persistent && e.remaining <= 0 {
			e.leaving, e.exitAt = true, now
			return t.env.ReducedMotion()
		}
		return false
	})
	if len(t.entries) > 0 {
		dst.Overlay(t.paintNotices)
	}
}
func (t *ToasterWidget) paintNotices(dst *ggui.Canvas) {
	now := ggui.Now()
	screen := dst.Size()
	theme := t.env.Theme()
	margin := theme.Space * 2
	if screen.W <= 0 || screen.H <= 0 {
		return
	}
	width := min(360.0, max(0, screen.W-margin*2))
	bottom := screen.H - margin
	for i := len(t.entries) - 1; i >= 0; i-- {
		e := t.entries[i]
		if bottom <= margin {
			break
		}
		size := e.panel.Layout(ggui.Constraints{MinW: width, MaxW: width, MaxH: bottom - margin}, t.env)
		targetY := bottom - size.H
		duration := t.env.Motion(toastMotionDuration)
		e.position.MoveTo(targetY, now, duration)
		target := 1.0
		if e.leaving {
			target = 0
		}
		e.motion.MoveTo(target, now, duration)
		progress := e.motion.Value(now)
		if t.env.ReducedMotion() {
			progress = target
		}
		y := e.position.Value(now)
		if t.env.ReducedMotion() {
			y = targetY
		}
		rect := ggui.Rct(ggui.Pt(screen.W-margin-size.W+(1-progress)*32, y), size)
		clip := dst.Clip(ggui.Rct(ggui.Pt(0, 0), screen))
		if e.leaving {
			clip = clip.Inert()
		}
		output := clip.Image
		fading := progress < 1 && output != nil
		if fading {
			bounds := output.Bounds()
			if t.buffer == nil || t.buffer.Bounds().Size() != bounds.Size() {
				if t.buffer != nil {
					t.buffer.Deallocate()
				}
				t.buffer = ebiten.NewImage(bounds.Dx(), bounds.Dy())
			}
			t.buffer.Clear()
			clip.Image = t.buffer
		}
		// Shadow stays outside the panel's own clipping rectangle.
		clip.Shadow(rect, theme.RadiusLg, theme.PanelShadow)
		clip = clip.Clip(rect)
		clip.HitPointer(rect, toastHover{e})
		// A notice is a live region: it appeared without the user asking,
		// so it is announced where it stands rather than waited for.
		clip.DescribeNode(rect, e, func(clip *ggui.Canvas) {
			clip.Paint(e.panel, rect)
		})
		if fading {
			options := &ebiten.DrawImageOptions{}
			options.ColorScale.ScaleAlpha(float32(progress))
			output.DrawImage(t.buffer, options)
		}
		if !e.leaving {
			bottom -= size.H + theme.Space
		}
	}
}

// A notice blocks clicks on its surface but leaves the rest of the window live.
type toastHover struct{ entry *toastEntry }

func (h toastHover) HandlePointer(ev ggui.PointerEvent) bool {
	switch ev.Kind {
	case ggui.PointerEnter, ggui.PointerMove:
		h.entry.hover = true
	case ggui.PointerExit:
		h.entry.hover = false
	case ggui.PointerScroll:
		return false
	}
	return true
}

// toastPanel is deliberately compact: a status marker, text hierarchy, an
// optional action, and a named close button. Colors follow the current theme.
type toastPanel struct {
	entry              *toastEntry
	title, description *ggui.TextWidget
	destructive        bool
	body               ggui.Widget
	theme              ggui.Theme
}

func (p *toastPanel) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	p.theme = env.Theme()
	t := p.theme
	accent := t.Primary
	mark := "✓"
	if p.destructive {
		accent = t.Destructive
		mark = "!"
	}
	p.title.Style(t.Text).Color(t.Fg)
	p.description.Style(t.Caption).Color(t.MutedFg)
	parts := []ggui.Widget{p.title, p.description}
	if p.entry.action != nil {
		parts = append(parts, ggui.Row(p.entry.action))
	}
	text := ggui.Column(parts...).Gap(4).Align(ggui.AlignStretch)
	p.body = ggui.Box(ggui.Row(
		ggui.Box(ggui.Center(ggui.Text(mark).Color(t.PrimaryFg).Size(14))).Size(24, 24).Fill(accent).Radius(12),
		ggui.Expanded(text), p.entry.dismiss,
	).Gap(12).Align(ggui.AlignStart)).Pad(t.Space*2).Fill(t.Popover).Border(t.BorderWidth, t.Border).Radius(t.RadiusLg)
	return p.body.Layout(c, env)
}

func (p *toastPanel) Paint(dst *ggui.Canvas, r ggui.Rect) {
	dst.Paint(p.body, r)
	e := p.entry
	if e.persistent || e.duration <= 0 {
		return
	}
	fraction := max(0, min(1, float64(e.remaining)/float64(e.duration)))
	accent := p.theme.Primary
	if p.destructive {
		accent = p.theme.Destructive
	}
	dst.FillRoundRect(ggui.Rct(r.Origin.Add(ggui.Pt(16.0, r.Size.H-5)), ggui.Sz((r.Size.W-32)*fraction, 2.0)), 1, accent)
}
