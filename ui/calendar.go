package ui

import (
	"strconv"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ironpark/ggui"
)

// CalendarWidget selects a single civil date. Navigation does not change value.
type CalendarWidget struct {
	ggui.Interactive
	value           ggui.Binding[time.Time]
	active, last    time.Time
	location        *time.Location
	weekStart       time.Weekday
	weekdays        [7]string
	monthLabel      func(time.Time) string
	disabledDate    func(time.Time) bool
	onChange        func(time.Time)
	onPick          func()
	min, max        time.Time
	lowest, highest time.Time // min and max normalized, refreshed each layout
	days            [42]*calendarDay
	weekdayText     [7]*ggui.TextWidget
	weekdaySize     [7]ggui.Size
	previous, next  *ButtonWidget
	title           *ggui.TextWidget
	header          ggui.Widget
	theme           ggui.Theme
	env             ggui.Env
	headerSize      ggui.Size
	cellH           float64
}

// Calendar binds a date; zero means no selection. Dates are interpreted in
// Location (time.Local by default), and user selections are local midnight.
func Calendar(value ggui.Binding[time.Time]) *CalendarWidget {
	c := &CalendarWidget{value: value, location: time.Local, weekStart: time.Sunday,
		weekdays: [7]string{"Su", "Mo", "Tu", "We", "Th", "Fr", "Sa"}, monthLabel: func(t time.Time) string { return t.Format("January 2006") }}
	c.Role, c.Name = ggui.RoleGroup, "Calendar"
	c.AutoKey()
	c.previous = Button("‹", func() { c.moveMonth(-1) }).Ghost().Label("Previous month")
	c.next = Button("›", func() { c.moveMonth(1) }).Ghost().Label("Next month")
	c.title = ggui.Text("").NoWrap()
	c.header = ggui.Row(c.previous, ggui.Expanded(ggui.Center(c.title)), c.next).Gap(8)
	for i := range c.days {
		c.days[i] = &calendarDay{owner: c, text: ggui.Text("").NoWrap()}
		c.days[i].Role = ggui.RoleOption
	}
	for i := range c.weekdayText {
		c.weekdayText[i] = ggui.Text("").NoWrap()
	}
	return c
}

// Named sets the calendar's accessible name.
func (c *CalendarWidget) Named(s string) *CalendarWidget { c.Name = s; return c }

// Location sets the timezone used to interpret dates. Nil means time.Local.
func (c *CalendarWidget) Location(l *time.Location) *CalendarWidget {
	if l == nil {
		l = time.Local
	}
	c.location = l
	c.active = time.Time{}
	return c
}

// WeekStartsOn configures the first weekday column.
func (c *CalendarWidget) WeekStartsOn(d time.Weekday) *CalendarWidget {
	if d < time.Sunday || d > time.Saturday {
		panic("ui: invalid weekday")
	}
	c.weekStart = d
	return c
}

// WeekdayLabels sets localized labels in Sunday-to-Saturday order.
func (c *CalendarWidget) WeekdayLabels(labels [7]string) *CalendarWidget {
	c.weekdays = labels
	return c
}

// MonthLabel formats the month heading; nil keeps the current formatter.
func (c *CalendarWidget) MonthLabel(fn func(time.Time) string) *CalendarWidget {
	if fn != nil {
		c.monthLabel = fn
	}
	return c
}

// Bounds sets inclusive selectable dates; a zero endpoint is unbounded.
func (c *CalendarWidget) Bounds(minimum, maximum time.Time) *CalendarWidget {
	c.min, c.max = minimum, maximum
	return c
}

// DisabledDate prevents selection of dates for which fn returns true.
func (c *CalendarWidget) DisabledDate(fn func(time.Time) bool) *CalendarWidget {
	c.disabledDate = fn
	return c
}

// Disabled disables navigation and date selection.
func (c *CalendarWidget) Disabled(v bool) *CalendarWidget { c.Inert = v; return c }

// OnChange runs only after a user selects a different civil date.
func (c *CalendarWidget) OnChange(fn func(time.Time)) *CalendarWidget { c.onChange = fn; return c }
func (c *CalendarWidget) date(t time.Time) time.Time {
	if t.IsZero() {
		return t
	}
	t = t.In(c.location)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, c.location)
}
func (c *CalendarWidget) enabled(t time.Time) bool {
	return !c.Inert && (c.lowest.IsZero() || !t.Before(c.lowest)) && (c.highest.IsZero() || !t.After(c.highest)) && (c.disabledDate == nil || !c.disabledDate(t))
}

// month is the first of the browsed month; the browsed date defines it.
func (c *CalendarWidget) month() time.Time {
	return time.Date(c.active.Year(), c.active.Month(), 1, 0, 0, 0, 0, c.location)
}

func (c *CalendarWidget) show(t time.Time) {
	c.active = t
	ggui.Invalidate(c.env)
}
func (c *CalendarWidget) syncDate() {
	v := c.date(c.value.Get())
	if c.active.IsZero() || !v.Equal(c.last) {
		if v.IsZero() {
			c.show(c.date(time.Now()))
		} else {
			c.show(v)
		}
		c.last = v
	}
}
func (c *CalendarWidget) moveMonth(n int) {
	if c.Inert {
		return
	}
	c.syncDate()
	m := c.month().AddDate(0, n, 0)
	last := m.AddDate(0, 1, -1).Day()
	c.show(m.AddDate(0, 0, min(c.active.Day(), last)-1))
}
func (c *CalendarWidget) choose(t time.Time) {
	if !c.enabled(t) {
		return
	}
	c.show(t)
	if !c.date(c.value.Peek()).Equal(t) {
		c.value.Set(t)
		c.last = t
		if c.onChange != nil {
			c.onChange(t)
		}
	}
	if c.onPick != nil {
		c.onPick()
	}
}

// Layout implements ggui.Widget.
func (c *CalendarWidget) Layout(cs ggui.Constraints, env ggui.Env) ggui.Size {
	c.Sync()
	c.env = env
	c.theme = env.Theme()
	c.syncDate()
	c.lowest, c.highest = c.date(c.min), c.date(c.max)
	month := c.month()
	c.title.Set(c.monthLabel(month))
	c.previous.Disabled(c.Inert)
	c.next.Disabled(c.Inert)
	w := cs.Constrain(ggui.Sz(294, 0)).W
	headerHeight := c.previous.Layout(ggui.Loose(ggui.Sz(w, cs.MaxH)), env).H
	c.headerSize = c.header.Layout(ggui.Loose(ggui.Sz(w, headerHeight)), env)
	c.cellH = max(36, c.theme.Text.Size+16)
	cell := ggui.Loose(ggui.Sz(w/7, c.cellH))
	for i, label := range c.weekdayText {
		c.weekdaySize[i] = label.Set(c.weekdays[(i+int(c.weekStart))%7]).Color(c.theme.Muted).Layout(cell, env)
	}
	selected := c.date(c.value.Peek())
	start := month.AddDate(0, 0, -(int(month.Weekday())-int(c.weekStart)+7)%7)
	for i, d := range c.days {
		if date := start.AddDate(0, 0, i); !date.Equal(d.date) {
			d.date = date
			d.Name = date.Format("2006-01-02")
		}
		d.Inert = !c.enabled(d.date)
		col := c.theme.Fg
		if d.Inert || d.date.Month() != month.Month() {
			col = c.theme.Muted
		}
		if selected.Equal(d.date) {
			col = c.theme.OnAccent
		}
		d.size = d.text.Set(strconv.Itoa(d.date.Day())).Color(col).Layout(cell, env)
	}
	return cs.Constrain(ggui.Sz(w, c.headerSize.H+7*c.cellH+8))
}

// Paint implements ggui.Widget.
func (c *CalendarWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	dst.Node(r, ggui.Node{Role: ggui.RoleGroup, Name: c.Name}, func(dst *ggui.Canvas) {
		dst.Paint(c.header, ggui.Rct(r.Origin, ggui.Sz(r.Size.W, c.headerSize.H)))
		y := r.Origin.Y + c.headerSize.H + 8
		w := r.Size.W / 7
		grid := ggui.Rct(ggui.Pt(r.Origin.X, y+c.cellH), ggui.Sz(r.Size.W, 6*c.cellH))
		if !c.Inert {
			dst.HitKey(grid, c)
		}
		for i, label := range c.weekdayText {
			s := c.weekdaySize[i]
			dst.Paint(label, ggui.Rct(ggui.Pt(r.Origin.X+float64(i)*w+(w-s.W)/2, y+(c.cellH-s.H)/2), s))
		}
		cells := dst.Clip(r)
		for i, d := range c.days {
			cells.Paint(d, ggui.Rct(ggui.Pt(r.Origin.X+float64(i%7)*w, y+float64(i/7+1)*c.cellH), ggui.Sz(w, c.cellH)))
		}
	})
}

// ConsumesKey implements ggui.KeyConsumer.
func (c *CalendarWidget) ConsumesKey(e ggui.KeyEvent) bool {
	if c.Inert || e.Kind != ggui.KeyPress {
		return false
	}
	switch e.Key {
	case ebiten.KeyArrowLeft, ebiten.KeyArrowRight, ebiten.KeyArrowUp, ebiten.KeyArrowDown, ebiten.KeyHome, ebiten.KeyEnd, ebiten.KeyPageUp, ebiten.KeyPageDown:
		return true
	}
	return ggui.Activates(e)
}

// HandleKey navigates by day/week, Home/End within the week, and PageUp/Down by month.
func (c *CalendarWidget) HandleKey(e ggui.KeyEvent) {
	c.Keyboard(e, nil)
	if c.Inert || e.Kind != ggui.KeyPress {
		return
	}
	c.syncDate()
	delta := 0
	switch e.Key {
	case ebiten.KeyArrowLeft:
		delta = -1
	case ebiten.KeyArrowRight:
		delta = 1
	case ebiten.KeyArrowUp:
		delta = -7
	case ebiten.KeyArrowDown:
		delta = 7
	case ebiten.KeyHome:
		delta = -(int(c.active.Weekday()) - int(c.weekStart) + 7) % 7
	case ebiten.KeyEnd:
		delta = 6 - (int(c.active.Weekday())-int(c.weekStart)+7)%7
	case ebiten.KeyPageUp:
		c.moveMonth(-1)
		return
	case ebiten.KeyPageDown:
		c.moveMonth(1)
		return
	default:
		if ggui.Activates(e) {
			c.choose(c.active)
		}
		return
	}
	c.show(c.active.AddDate(0, 0, delta))
}

// Adopt preserves the browsed month across rebuilds; binding changes still win.
func (c *CalendarWidget) Adopt(prev any) {
	c.Interactive.Adopt(prev)
	if p, ok := prev.(*CalendarWidget); ok && p.location == c.location {
		c.active, c.last = p.active, p.last
		c.syncDate()
	}
}

type calendarDay struct {
	ggui.Interactive
	owner *CalendarWidget
	date  time.Time
	text  *ggui.TextWidget
	size  ggui.Size
}

func (d *calendarDay) Layout(cs ggui.Constraints, _ ggui.Env) ggui.Size { return cs.Constrain(d.size) }
func (d *calendarDay) Describe() ggui.Node {
	return ggui.Node{Role: ggui.RoleOption, Name: d.Name, Selected: d.owner.date(d.owner.value.Peek()).Equal(d.date), Disabled: d.Inert, Actions: ggui.ActionPress | ggui.ActionSelect}
}
func (d *calendarDay) Act(a ggui.Action) bool {
	if !d.owner.enabled(d.date) || (a.Kind != ggui.ActionPress && a.Kind != ggui.ActionSelect) {
		return false
	}
	d.owner.choose(d.date)
	return true
}
func (d *calendarDay) HandlePointer(e ggui.PointerEvent) bool {
	if !d.owner.enabled(d.date) {
		return false
	}
	return d.Pointer(e, func() { d.owner.choose(d.date) })
}
func (d *calendarDay) Paint(dst *ggui.Canvas, r ggui.Rect) {
	c := d.owner
	t := c.theme
	dst.Describe(r, d)
	if !d.Inert {
		dst.HitPointer(r, d)
		dst.HitCursor(r, ebiten.CursorShapePointer)
	}
	if c.date(c.value.Peek()).Equal(d.date) {
		dst.FillRoundRect(r, t.Radius, t.Accent)
	} else if d.Hovered {
		dst.FillRoundRect(r, t.Radius, mutedSurface(t))
	}
	if c.Focused && c.active.Equal(d.date) {
		dst.StrokeRoundRect(r, t.Radius, 2, t.Accent)
	}
	dst.Paint(d.text, ggui.Rct(ggui.Pt(r.Origin.X+(r.Size.W-d.size.W)/2, r.Origin.Y+(r.Size.H-d.size.H)/2), d.size))
}
