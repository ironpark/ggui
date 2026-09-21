package ggui

import "github.com/ironpark/ggui/a11y"

// The request types live in the a11y package, which cannot import this
// one, so that a bridge raises the same types the app acts on rather than
// copies of them. These are those types under these names.
type (
	// Action is one request aimed at a node.
	Action = a11y.Action

	// Politeness says how urgent an announcement is.
	Politeness = a11y.Politeness

	// Announcement is one thing to say out loud, queued by Announce.
	Announcement = a11y.Announcement
)

// How urgent an announcement is.
const (
	Polite    = a11y.Polite
	Assertive = a11y.Assertive
)

// Actions come from outside the pointer and the keyboard: a screen reader
// user activates a button from a rotor, drags a slider with a gesture that
// never touches it, or types into a field through the platform's own text
// API. The tree says what each node accepts through Node.Actions; this is
// the side that carries them out.
//
// Every entry point marshals onto the UI goroutine through Post and returns
// at once, without waiting for the work to happen. That is not an
// optimization, it is the only thing that cannot deadlock: the assistive
// technology calls from the platform's main thread and blocks there, while
// the UI goroutine may itself be blocked inside a call that needs the main
// thread. Waiting for a reply would have each side holding what the other
// wants. So an action is a request, never a question; read the result from
// the next frame's tree.

// Actor is a widget that carries out an action aimed at it. A widget needs
// one only for what its own logic must do: pressing falls back to the key
// press a control already acts on, and focusing and scrolling into view are
// the runtime's to arrange. Return false for an action the widget does not
// handle, and the fallback runs instead.
type Actor interface {
	Act(a Action) bool
}

// Announce queues text to be spoken. A tree diff cannot express "say this
// now": a toast that appears and goes, a status that changed while focus
// stayed put, a search that found nothing. Those are live regions, and this
// is how a widget raises one.
//
// It may be called from any goroutine. Nothing reads the queue yet; the
// platform bridge will.
func (a *App) Announce(text string, politeness Politeness) {
	a.announce(Announcement{Text: text, Politeness: politeness})
}

// Announce queues text as App.Announce does, for tests.
func (p *Probe) Announce(text string, politeness Politeness) {
	p.announce(Announcement{Text: text, Politeness: politeness})
}

// Perform asks the node identified by id to carry out act. It returns
// immediately: the work is queued onto the UI goroutine and happens before
// the next frame's input, and the frame after that publishes what came of
// it. See the note at the top of this file for why it must not wait.
func (a *App) Perform(id NodeID, act Action) {
	a.Post(func() { perform(&a.canvas, &a.input, id, act) })
}

// Perform queues act as App.Perform does and then runs a frame, so that a
// test sees what came of it without having to drive the loop by hand. The
// queueing is the same, so the path under test is the real one.
func (p *Probe) Perform(id NodeID, act Action) {
	p.Post(func() { perform(&p.canvas, &p.in, id, act) })
	p.Frame()
}

// perform routes act to the widget that described the node. It runs on the
// UI goroutine, against the regions and the tree the last frame left.
func perform(c *Canvas, in *inputState, id NodeID, act Action) bool {
	e := findSemNode(c, id)
	if e == nil {
		return false
	}
	// A disabled control is in the tree so that it can be read out, not so
	// that it can be worked; only being looked at is allowed.
	if e.node.Disabled && act.Kind != ActionFocus && act.Kind != ActionScrollIntoView {
		return false
	}
	if a, ok := e.handler.(Actor); ok && a.Act(act) {
		return true
	}
	switch act.Kind {
	case ActionPress:
		return press(e)
	case ActionFocus:
		return focusNode(in, e)
	case ActionScrollIntoView:
		in.reveal(e.full)
		return true
	}
	return false
}

// findSemNode returns the node id names in the frame's tree, matched the way
// Canvas.adopt matches a hit region: by identity when it has one, else by
// the bounds it painted.
func findSemNode(c *Canvas, id NodeID) *semNode {
	sem := c.fs().sem
	for i := range sem {
		e := &sem[i]
		if e.node.Role != id.Role {
			continue
		}
		if id.ID != nil && sameAny(e.id, id.ID) || id.ID == nil && e.id == nil && e.full == id.Rect {
			return e
		}
	}
	return nil
}

// press activates a control the way the keyboard does. Every control acts
// on Space through Activates, so the existing path serves: nothing has to
// learn a second way to be pressed. A handler that takes only the pointer,
// such as a menu item, gets the tap instead.
func press(e *semNode) bool {
	if k, ok := e.handler.(KeyHandler); ok {
		k.HandleKey(KeyEvent{Kind: KeyPress, Key: KeySpace})
		return true
	}
	if p, ok := e.handler.(PointerHandler); ok {
		at := Pt(e.full.Origin.X+e.full.Size.W/2, e.full.Origin.Y+e.full.Size.H/2)
		p.HandlePointer(PointerEvent{Kind: PointerTap, Pos: at, Button: MouseButtonLeft})
		return true
	}
	return false
}

// focusNode moves keyboard focus to the node's region, as Tab would, so the
// control shows its focus ring and every Revealer scrolls it into view.
func focusNode(in *inputState, e *semNode) bool {
	r := in.topmost(func(r *hitRegion) bool { return r.key != nil && sameAny(r.key, e.handler) })
	if r == nil {
		return false
	}
	in.focus(r, true)
	return true
}
