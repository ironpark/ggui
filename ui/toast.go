package ui

import (
	"slices"
	"time"

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

// Destructive uses the DangerColor for the notice.
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
}
type toastEntry struct {
	id              ToastID
	panel           *AlertWidget
	dismiss, action *ButtonWidget
	remaining       time.Duration
	persistent      bool
	last            time.Time
	hover           bool
}

// NewToaster creates a queue with room for three notices. Keep it outside the
// rebuilding Builder, or create it in Component setup and register
// ggui.OnCleanup(toaster.Close). There is no process-global notification state.
func NewToaster() *ToasterWidget { return &ToasterWidget{limit: 3} }

// Limit sets the queue size (at least one); excess oldest notices are discarded.
func (t *ToasterWidget) Limit(n int) *ToasterWidget { t.limit = max(1, n); t.trim(); return t }
func (t *ToasterWidget) trim() {
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
	e := &toastEntry{id: t.next, remaining: message.duration, persistent: message.duration <= 0, last: ggui.Now()}
	e.dismiss = Button("Dismiss", func() { t.Dismiss(e.id) }).Secondary().Label("Dismiss " + message.title)
	e.dismiss.Key(e)
	actions := []ggui.Widget{}
	if message.actionLabel != "" {
		e.action = Button(message.actionLabel, func() {
			t.Dismiss(e.id)
			if message.action != nil {
				message.action()
			}
		}).Secondary()
		e.action.Key(&e.action)
		actions = append(actions, e.action)
	}
	actions = append(actions, e.dismiss)
	e.panel = Alert(message.title, message.description).Action(ggui.Wrap(actions...).Space(.5))
	if message.destructive {
		e.panel.Destructive()
	}
	t.entries = append(t.entries, e)
	t.trim()
	return e.id
}

// Dismiss removes the notice with id. An unknown ID is a no-op.
func (t *ToasterWidget) Dismiss(id ToastID) {
	t.entries = slices.DeleteFunc(t.entries, func(e *toastEntry) bool { return e.id == id })
}

// Len returns queued notices; expiration is processed when the host paints.
func (t *ToasterWidget) Len() int { return len(t.entries) }

// Clear drops all notifications and their callbacks.
func (t *ToasterWidget) Clear() { clear(t.entries); t.entries = nil }

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
		paused := e.hover || e.dismiss.Focused || e.dismiss.Hovered || (e.action != nil && (e.action.Focused || e.action.Hovered))
		if !e.persistent && !paused {
			e.remaining -= max(time.Duration(0), now.Sub(e.last))
		}
		e.last = now
		return !e.persistent && e.remaining <= 0
	})
	if len(t.entries) > 0 {
		dst.Overlay(t.paintNotices)
	}
}
func (t *ToasterWidget) paintNotices(dst *ggui.Canvas) {
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
		rect := ggui.Rct(ggui.Pt(screen.W-margin-size.W, bottom-size.H), size)
		clip := dst.Clip(rect)
		clip.HitPointer(rect, toastHover{e})
		clip.Paint(e.panel, rect)
		bottom -= size.H + theme.Space
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
