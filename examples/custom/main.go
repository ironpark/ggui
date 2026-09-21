// Command custom shows what the built-in widgets are made of: a Dial
// written from Layout and Paint on top of ggui.Interactive, dragged through
// its pointer handler with a Spring easing the needle; a Disclosure that
// keeps its open flag outside any signal and calls Invalidate when its size
// changes; and a list of 5,000 rows that ItemExtent and Retain keep as
// cheap as the rows in view. The inspector opens on F1.
//
// The UI lives in build so main_test.go can drive it headlessly with a
// Probe.
package main

import (
	"fmt"
	"log"
	"math"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

// dial is a rotary control over a float in [0, 1]. Interactive supplies
// hover, press, focus, identity across rebuilds and the semantics node; the
// dial adds the geometry, the drawing and what a drag or an arrow key does.
type dial struct {
	ggui.Interactive
	value  *ggui.StateValue[float64]
	needle *ggui.Sprung[float64] // eases toward value; Paint reads it
	theme  ggui.Theme
	rect   ggui.Rect
}

func newDial(value *ggui.StateValue[float64], name string) *dial {
	d := &dial{value: value, needle: ggui.Spring(ggui.Untrack(value.Get))}
	d.Role = ggui.RoleSlider
	d.SetName(name)
	d.AutoKey()
	return d
}

// Layout resolves the theme now, because Paint receives only the canvas.
func (d *dial) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	d.Sync()
	d.theme = env.Theme()
	s := min(c.MaxW, c.MaxH, 160)
	return c.Constrain(ggui.Sz(s, s))
}

// Paint registers the hit region through Hit and draws the ring, the arc
// the needle has swept and the needle itself at the spring's position.
func (d *dial) Paint(dst *ggui.Canvas, r ggui.Rect) {
	d.rect = r
	d.Hit(dst, r, d, ggui.CursorShapePointer)
	t := d.theme
	c := ggui.Pt(r.Origin.X+r.Size.W/2, r.Origin.Y+r.Size.H/2)
	radius := min(r.Size.W, r.Size.H)/2 - 4
	ring := t.Border
	if d.Hovered || d.Pressed {
		ring = t.Ring
	}
	dst.FillCircle(c, radius, t.Card)
	dst.StrokeRoundRect(ggui.Rct(ggui.Pt(c.X-radius, c.Y-radius), ggui.Sz(2*radius, 2*radius)), radius, 2, ring)
	// The spring moves a little every frame until it settles; the runtime
	// repaints each frame, so reading it here is enough to animate.
	angle := angleOf(d.needle.Get())
	tip := ggui.Pt(c.X+math.Cos(angle)*(radius-10), c.Y+math.Sin(angle)*(radius-10))
	dst.StrokeLine(c, tip, 3, t.Primary)
	dst.FillCircle(c, 5, t.Primary)
	d.FocusRing(dst, r, radius+4, t.Ring)
}

// angleOf maps [0, 1] onto the 270° sweep from 7 o'clock to 5 o'clock.
func angleOf(v float64) float64 { return (0.75 + 1.5*v) * math.Pi }

// set moves the value to where p points from the centre and retargets the
// needle; the spring, not the pointer, decides where it is drawn.
func (d *dial) set(p ggui.Point) {
	c := ggui.Pt(d.rect.Origin.X+d.rect.Size.W/2, d.rect.Origin.Y+d.rect.Size.H/2)
	a := math.Atan2(p.Y-c.Y, p.X-c.X)/math.Pi - 0.75 // turns past 7 o'clock
	for a < 0 {
		a += 2
	}
	d.write(min(a/1.5, 1))
}

func (d *dial) write(v float64) {
	v = max(0, min(v, 1))
	d.value.Set(v)
	d.needle.Set(v)
}

// HandlePointer implements ggui.PointerHandler. Pointer keeps hover and
// press current; a press or a drag anywhere while pressed sets the value.
func (d *dial) HandlePointer(ev ggui.PointerEvent) bool {
	handled := d.Pointer(ev, nil)
	if ev.Kind == ggui.PointerDown || ev.Kind == ggui.PointerDrag {
		d.set(ev.Pos)
	}
	return handled
}

// HandleKey implements ggui.KeyHandler: arrows step the value.
func (d *dial) HandleKey(ev ggui.KeyEvent) {
	d.Keyboard(ev, nil)
	if ev.Kind != ggui.KeyPress {
		return
	}
	switch ev.Key {
	case ggui.KeyArrowLeft, ggui.KeyArrowDown:
		d.write(ggui.Untrack(d.value.Get) - 0.05)
	case ggui.KeyArrowRight, ggui.KeyArrowUp:
		d.write(ggui.Untrack(d.value.Get) + 0.05)
	}
}

// ConsumesKey implements ggui.KeyConsumer, so a bare-key shortcut on an
// arrow yields to a focused dial.
func (d *dial) ConsumesKey(ev ggui.KeyEvent) bool {
	switch ev.Key {
	case ggui.KeyArrowLeft, ggui.KeyArrowRight, ggui.KeyArrowUp, ggui.KeyArrowDown:
		return true
	}
	return false
}

// Describe implements ggui.Describer: assistive technology sees a slider.
func (d *dial) Describe() ggui.Node {
	return ggui.Node{Role: ggui.RoleSlider, Name: d.SemanticName(), Min: 0, Max: 1, Now: ggui.Untrack(d.value.Get), Disabled: d.IsInert(), Actions: ggui.ActionFocus}
}

// disclosure shows its body only while open. The flag is an ordinary bool,
// not a signal, so nothing rebuilds when it flips; instead Invalidate tells
// the runtime the widget's size changed and the cache above it measures
// again. Scroll keeps its offset the same way.
type disclosure struct {
	open   bool
	env    ggui.Env
	header ggui.Widget
	body   ggui.Widget
	column *ggui.ColumnWidget
}

func newDisclosure(title string, body ggui.Widget) *disclosure {
	d := &disclosure{body: body}
	d.header = ggui.Tap(ggui.Row(ggui.Text(title), ggui.Spacer(), ggui.Caption("tap to toggle")).Space(1), d.toggle)
	return d
}

func (d *disclosure) toggle() {
	d.open = !d.open
	ggui.Invalidate(d.env)
}

func (d *disclosure) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	d.env = env
	if d.open {
		d.column = ggui.Column(d.header, d.body).Space(1).Align(ggui.AlignStretch)
	} else {
		d.column = ggui.Column(d.header).Align(ggui.AlignStretch)
	}
	return d.column.Layout(c, env)
}

// Paint goes through dst.Paint so the inspector sees the children.
func (d *disclosure) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(d.column, r) }

type item struct {
	ID   int
	Name string
}

const rowCount = 5000
const rowHeight = 32.0

type model struct {
	Level  *ggui.StateValue[float64]
	Items  *ggui.StateValue[[]item]
	Scroll *ggui.StateValue[float64]
	Detail *disclosure
}

func newModel() *model {
	items := make([]item, rowCount)
	for i := range items {
		items[i] = item{ID: i + 1, Name: fmt.Sprintf("Row %d", i+1)}
	}
	return &model{Level: ggui.State(0.35), Items: ggui.State(items), Scroll: ggui.State(0.0)}
}

func (m *model) jump(index int) { m.Scroll.Set(float64(index) * rowHeight) }

// build is the root Builder. The dial and the list are bound to signals,
// the disclosure holds its own flag, and nothing here reads reactively.
func (m *model) build() ggui.Widget {
	m.Detail = newDisclosure("How this works", ggui.Styled(ggui.Column(
		ggui.Text("The dial embeds ggui.Interactive and draws itself; a Spring eases its needle."),
		ggui.Text("This panel calls Invalidate instead of writing a signal when it opens."),
		ggui.Text("The list below builds only the rows in view and keeps 24 spares mounted."),
	).Space(0.5)).Size(12))
	return ggui.Box(ggui.Column(
		ggui.Row(
			newDial(m.Level, "Level"),
			ggui.Column(
				ggui.Title("Custom widgets"),
				ggui.Textf("Level: %.0f%%", m.Level.Map(func(v float64) float64 { return v * 100 })),
				ggui.Caption("Drag the dial, or focus it and use the arrows."),
				ui.Progress(m.Level),
			).Space(1).Align(ggui.AlignStretch),
		).Space(2).Align(ggui.AlignCenter),
		ui.Card(m.Detail),
		ggui.Row(
			ggui.Textf("%d rows", m.Items.Map(func(it []item) int { return len(it) })).AsCaption(),
			ggui.Spacer(),
			ui.Button("Top", func() { m.jump(0) }).Outline(),
			ui.Button("Middle", func() { m.jump(rowCount / 2) }).Outline(),
			ui.Button("End", func() { m.jump(rowCount) }).Outline(),
		).Space(1),
		ggui.Expanded(ggui.Scroll(
			// Each row keeps a checkbox of its own; Retain bounds how many
			// offscreen rows keep that state before being rebuilt.
			ggui.EachKeyed(m.Items, func(it item) int { return it.ID }, func(row ggui.EachItem[item]) ggui.Widget {
				picked := ggui.State(false)
				return ui.Checkbox(picked, row.Value.Get().Name)
			}).ItemExtent(rowHeight).Retain(24),
		).BindOffset(m.Scroll)),
	).Space(1.5).Align(ggui.AlignStretch)).Pad(24)
}

func main() {
	m := newModel()
	app := ggui.New(ggui.Config{Title: "ggui · custom widgets", Width: 560, Height: 640, Resizable: true, Inspector: "f1"}, m.build)
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
