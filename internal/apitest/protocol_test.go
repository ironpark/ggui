package apitest_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/ironpark/ggui"
)

// The widgets here are written as a package other than ggui writes one,
// on the kernel's public API alone: a drag source and a drop target, a
// panel that holds focus while it is open, and a keyed list that keeps the
// rows the user is busy with. Each proves the API they use is enough for
// what ggui's own Reorderable, Popup and EachKeyed do with it.

// card is a drag source of its own: it carries its label.
type card struct {
	label    string
	at, grab ggui.Point
	moves    int
	ended    []bool
}

func (c *card) Layout(ggui.Constraints, ggui.Env) ggui.Size { return ggui.Sz(100, 40) }
func (c *card) Paint(dst *ggui.Canvas, r ggui.Rect)         { dst.HitDrag(r, c) }
func (c *card) DragValue() any                              { return c.label }
func (c *card) DragMoved(at, grab ggui.Point)               { c.at, c.grab = at, grab; c.moves++ }
func (c *card) DragEnded(dropped bool)                      { c.ended = append(c.ended, dropped) }

// bin is a drop target of its own: it takes the strings accept takes.
type bin struct {
	accept func(string) bool
	hovers []bool
	over   []ggui.Point
	got    []string
	at     []ggui.Point
}

func (b *bin) Layout(ggui.Constraints, ggui.Env) ggui.Size { return ggui.Sz(100, 40) }
func (b *bin) Paint(dst *ggui.Canvas, r ggui.Rect)         { dst.HitPointer(r, b) }
func (b *bin) HandlePointer(ggui.PointerEvent) bool        { return false }
func (b *bin) AcceptsDrop(v any) bool {
	s, ok := v.(string)
	return ok && (b.accept == nil || b.accept(s))
}
func (b *bin) DropHover(over bool)           { b.hovers = append(b.hovers, over) }
func (b *bin) DropOver(_ any, at ggui.Point) { b.over = append(b.over, at) }
func (b *bin) Drop(v any, at ggui.Point)     { b.got, b.at = append(b.got, v.(string)), append(b.at, at) }

func TestADragSourceAndDropTargetOfOurOwn(t *testing.T) {
	t.Parallel()
	src := &card{label: "apple"}
	under := &bin{}
	refusing := &bin{accept: func(string) bool { return false }}
	p := ggui.NewProbe(ggui.Column(src, ggui.Stack(under, refusing)), ggui.Sz(100, 80))
	defer p.Close()
	p.Frame()

	p.Press(ggui.Pt(30, 10))
	p.Move(ggui.Pt(31, 11)) // within the threshold: still a press
	if src.moves != 0 {
		t.Fatal("a drag started before the pointer moved far enough")
	}
	p.Move(ggui.Pt(30, 50))
	p.Move(ggui.Pt(40, 60))
	if src.at != ggui.Pt(40, 60) || src.grab != ggui.Pt(30, 10) {
		t.Fatalf("the source heard of the drag at %v holding %v, want (40,60) holding (30,10)", src.at, src.grab)
	}
	if len(refusing.hovers) != 0 || !slices.Equal(under.hovers, []bool{true}) {
		t.Fatalf("hovers: the refusing target %v, the one beneath %v; want the drag passed down", refusing.hovers, under.hovers)
	}
	if n := len(under.over); n == 0 || under.over[n-1] != ggui.Pt(40, 20) {
		t.Fatalf("the target followed the drag at %v, want it last at (40,20) of its own", under.over)
	}
	p.Release(ggui.Pt(40, 60))
	if !slices.Equal(under.got, []string{"apple"}) || under.at[0] != ggui.Pt(40, 20) || len(refusing.got) != 0 {
		t.Fatalf("dropped %v at %v, the refusing target took %v", under.got, under.at, refusing.got)
	}
	if !slices.Equal(src.ended, []bool{true}) || !slices.Equal(under.hovers, []bool{true, false}) {
		t.Fatalf("ended %v, hovers %v; want a drop that leaves the target", src.ended, under.hovers)
	}

	// Escape puts it back, and the target hears it go.
	p.Press(ggui.Pt(30, 10))
	p.Move(ggui.Pt(30, 60))
	p.Type(ggui.Mods{}, ggui.KeyEscape)
	p.Release(ggui.Pt(30, 60))
	if len(under.got) != 1 || !slices.Equal(src.ended, []bool{true, false}) || under.hovers[len(under.hovers)-1] {
		t.Fatalf("Escape: dropped %v, ended %v, hovers %v", under.got, src.ended, under.hovers)
	}
}

// field is a focusable control that logs its focus under its name. It
// has a HitID, so a field rebuilt each frame is still the one field, and
// adopts the one it replaced, so a rebuild is not a focus of its own.
type field struct {
	name string
	log  *[]string
}

func (f *field) Layout(ggui.Constraints, ggui.Env) ggui.Size { return ggui.Sz(100, 20) }
func (f *field) Paint(dst *ggui.Canvas, r ggui.Rect)         { dst.HitKey(r, f) }
func (f *field) HitID() any                                  { return f.name }
func (f *field) Adopt(any)                                   {}
func (f *field) HandleKey(ev ggui.KeyEvent) {
	switch ev.Kind {
	case ggui.KeyFocus:
		*f.log = append(*f.log, "focus "+f.name)
	case ggui.KeyBlur:
		*f.log = append(*f.log, "blur "+f.name)
	}
}

// panel is a focus trap of its own, as a nonmodal popup is: opening it
// focuses the first control it holds, focus moved to another control
// closes it, and closing it gives focus back to the control that had it
// when it opened, though that control was rebuilt since.
type panel struct {
	content ggui.Widget
	open    bool
	shown   bool // painted open last frame
	opener  ggui.KeyHandler
	keys    []ggui.KeyHandler
	held    bool // focus has been inside since it opened
	restore bool
	closed  int
}

func (p *panel) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	p.content.Layout(c, env)
	return ggui.Sz(100, 40)
}

func (p *panel) Paint(dst *ggui.Canvas, r ggui.Rect) {
	if p.restore {
		dst.RestoreFocus(p.opener)
		p.restore = false
	}
	if p.open && !p.shown {
		p.opener, p.held = dst.Focused(), false
	}
	inside := slices.ContainsFunc(p.keys, dst.HasFocus)
	p.held = p.held || inside
	if p.open && p.held && !inside && dst.Focused() != nil && !dst.FocusStale() {
		p.Close()
	}
	opening := p.open && !p.shown
	p.shown = p.open
	if !p.open {
		p.keys = p.keys[:0]
		return
	}
	mark := dst.HitMark()
	dst.Paint(p.content, r)
	p.keys = dst.KeysSince(mark, p.keys[:0])
	if opening && len(p.keys) > 0 {
		dst.RequestFocus(p.keys[0])
	}
}

func (p *panel) Close() { p.open, p.restore = false, true; p.closed++ }

// rebuilt paints a new field named name every frame, then w.
type rebuilt struct {
	name string
	log  *[]string
	w    ggui.Widget
}

func (b *rebuilt) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	b.w.Layout(c, env)
	return ggui.Sz(100, 100)
}

func (b *rebuilt) Paint(dst *ggui.Canvas, r ggui.Rect) {
	dst.Paint(&field{name: b.name, log: b.log}, ggui.Rct(r.Origin, ggui.Sz(100, 20)))
	dst.Paint(b.w, ggui.Rct(r.Origin.Add(ggui.Pt(0, 20)), ggui.Sz(100, 80)))
}

func TestAFocusTrapOfOurOwn(t *testing.T) {
	t.Parallel()
	var log []string
	pn := &panel{content: ggui.Column(&field{name: "a", log: &log}, &field{name: "b", log: &log})}
	other := &field{name: "other", log: &log}
	p := ggui.NewProbe(&rebuilt{name: "opener", log: &log, w: ggui.Column(other, pn)}, ggui.Sz(100, 100))
	defer p.Close()
	p.Frame()
	p.Click(ggui.Pt(50, 10))
	p.Frame()
	if !slices.Equal(log, []string{"focus opener"}) {
		t.Fatalf("clicking the opener: %v", log)
	}

	log = nil
	pn.open = true
	p.Frame()
	p.Frame()
	if !slices.Equal(log, []string{"blur opener", "focus a"}) {
		t.Fatalf("opening: %v, want focus moved to the first control inside", log)
	}

	// Closing gives focus back to the opener, a field painted anew since.
	log = nil
	pn.Close()
	p.Frame()
	p.Frame()
	if !slices.Equal(log, []string{"blur a", "focus opener"}) {
		t.Fatalf("closing: %v, want focus back on the rebuilt opener", log)
	}

	// Focus moved to another control closes it.
	pn.open, pn.closed = true, 0
	p.Frame()
	p.Frame()
	p.Click(ggui.Pt(50, 30))
	p.Frame()
	if pn.open || pn.closed != 1 {
		t.Fatalf("focus moved outside: open %v, closed %d times", pn.open, pn.closed)
	}
}

// keyedList shows the rows of keys from first, count at a time, one
// field each, and keeps a row it no longer shows only while the user is
// busy with it.
type keyedList struct {
	keys         []string
	first, count int
	rows         map[string]*listRow
	log          *[]string
	disposed     []string
}

type listRow struct{ field *field }

func (l *keyedList) Layout(ggui.Constraints, ggui.Env) ggui.Size {
	shown := l.keys[l.first : l.first+l.count]
	for _, k := range shown {
		if l.rows[k] == nil {
			l.rows[k] = &listRow{field: &field{name: k, log: l.log}}
		}
	}
	for k, row := range l.rows {
		if !slices.Contains(shown, k) && !ggui.GroupBusy(row) {
			delete(l.rows, k)
			l.disposed = append(l.disposed, k)
		}
	}
	return ggui.Sz(100, float64(20*l.count))
}

func (l *keyedList) Paint(dst *ggui.Canvas, r ggui.Rect) {
	for i, k := range l.keys[l.first : l.first+l.count] {
		row := l.rows[k]
		rc := ggui.Rct(r.Origin.Add(ggui.Pt(0, float64(20*i))), ggui.Sz(100, 20))
		dst.Group(row, func() { dst.Paint(row.field, rc) })
	}
}

func TestAKeyedListOfOurOwnKeepsTheRowInUse(t *testing.T) {
	t.Parallel()
	var log []string
	keys := make([]string, 10)
	for i := range keys {
		keys[i] = fmt.Sprint("row", i)
	}
	l := &keyedList{keys: keys, count: 3, rows: map[string]*listRow{}, log: &log}
	p := ggui.NewProbe(l, ggui.Sz(100, 60))
	defer p.Close()
	p.Frame()
	p.Click(ggui.Pt(50, 30)) // row1
	p.Frame()
	if !ggui.GroupBusy(l.rows["row1"]) || ggui.GroupBusy(l.rows["row0"]) {
		t.Fatal("the focused row is not busy, or another is")
	}

	l.first = 3
	ggui.Invalidate(ggui.Env{})
	p.Frame()
	slices.Sort(l.disposed)
	if !slices.Equal(l.disposed, []string{"row0", "row2"}) || l.rows["row1"] == nil {
		t.Fatalf("scrolled away: disposed %v; want row1, which holds focus, kept", l.disposed)
	}

	// Focus moved on, the row goes at the next layout, and the row that
	// has it now stays.
	p.Click(ggui.Pt(50, 10)) // row3
	l.disposed = nil
	l.first = 4
	ggui.Invalidate(ggui.Env{})
	p.Frame()
	if !slices.Equal(l.disposed, []string{"row1"}) || l.rows["row3"] == nil {
		t.Fatalf("focus moved on to row3: disposed %v, want row1 alone", l.disposed)
	}
}
