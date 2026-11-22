package textedit

import (
	"time"

	"github.com/ironpark/ggui/layout"
)

// Clicks turns a pointer's clicks on text into a selection: one places the
// caret, a quick second near the first selects a word, a third the line,
// and a drag after one click extends the selection. widget.TextInput and
// selectable rich text share it, so a double click means the same in both.
type Clicks struct {
	clicks    int
	lastClick time.Time
	lastPos   layout.Point
}

// Press counts a left press at pos, made at now, and selects in ed for it;
// at reports the text offset for a point. extend, as Shift does, extends
// the selection to pos instead and starts the count over. now is the
// caller's frame clock, so a test that moves it reaches a double click.
func (c *Clicks) Press(ed *Editor, now time.Time, pos layout.Point, at func(layout.Point) int, extend bool) {
	if now.Sub(c.lastClick) < 400*time.Millisecond && near(pos, c.lastPos) {
		c.clicks++
	} else {
		c.clicks = 1
	}
	c.lastClick, c.lastPos = now, pos
	idx := at(pos)
	switch {
	case extend:
		c.clicks = 1
		ed.MoveTo(idx, true)
	case c.clicks == 1:
		ed.MoveTo(idx, false)
	case c.clicks == 2:
		ed.SelectWord(idx)
	default:
		ed.SelectLine(idx)
	}
}

// Drag extends ed's selection to pos after a single click, and reports
// whether it did.
func (c *Clicks) Drag(ed *Editor, pos layout.Point, at func(layout.Point) int) bool {
	if c.clicks != 1 {
		return false
	}
	ed.MoveTo(at(pos), true)
	return true
}

func near(a, b layout.Point) bool {
	dx, dy := a.X-b.X, a.Y-b.Y
	return dx*dx+dy*dy < 16
}

// WordWise reports whether the modifiers held ask for the word-wise form of
// a motion or a delete: Alt (Option) anywhere, or Ctrl without Meta. Ctrl
// stands in for Cmd off macOS, but no Cmd shortcut uses an arrow or a
// delete, which is why Ctrl+Left jumps a word on Windows and Linux.
func WordWise(alt, ctrl, meta bool) bool { return alt || (ctrl && !meta) }
