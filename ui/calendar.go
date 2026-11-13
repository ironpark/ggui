package ui

import (
	"image/color"
	"math"
	"strconv"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/ui/icons"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// CalendarWidget selects civil dates in single, range or multiple mode.
// Navigation does not change the selection. The arrow keys browse by day
// and week, Home and End to the ends of the week, PageUp and PageDown by
// month and, with Shift, by year; they pass over dates that cannot be
// chosen.
type CalendarWidget struct {
	props property.Owner
	ggui.Interactive
	value                                          ggui.Binding[time.Time]
	rangeValue                                     ggui.Binding[DateRange]
	multipleValue                                  ggui.Binding[[]time.Time]
	onRangeChange                                  func(DateRange)
	onMultipleChange                               func([]time.Time)
	required, excludeDisabled, resetOnSelect       bool
	minSelected, maxSelected, minNights, maxNights int
	active, last                                   time.Time
	first                                          time.Time // the first month shown; the browsed date stays within the months from it
	location                                       *time.Location
	weekStart                                      time.Weekday
	weekdays                                       [7]string
	monthNames                                     [12]string
	monthLabel                                     func(time.Time) string
	disabledDate                                   func(time.Time) bool
	onChange                                       func(time.Time)
	onPick                                         func()
	min, max                                       time.Time
	lowest, highest                                time.Time         // min and max normalized, refreshed each layout
	today                                          time.Time         // refreshed each layout
	selection                                      calendarSelection // the binding normalized, refreshed each layout
	count                                          int               // months shown at once
	dropdown                                       bool              // month and year selects in place of the caption
	yearFrom, yearTo                               int               // the years the dropdown offers; zero follows the bounds
	years                                          []int
	panes                                          []*calendarPane
	previous, next                                 *ButtonWidget
	previousIcon, nextIcon                         *icons.Widget
	theme                                          *uitheme.Theme
	env                                            ggui.Env
	hover                                          color.Color // the fill under the pointer, resolved each layout
	metrics                                        calendarMetrics
}

// calendarMetrics is the geometry one layout settles for Paint.
type calendarMetrics struct {
	pad, cell, rowGap, monthGap float64 // root padding, the square day fill, and the gaps between weeks and months
	column                      float64 // the width of a weekday column, the cell's unless the calendar is stretched
	caption, weekday            float64 // the heights of the caption and weekday rows
	captionGap                  float64 // between the caption and the weekday row
	monthW, monthH              float64
	perRow                      int
	nav                         ggui.Size
}

// calendarPane is one month: its caption, weekday labels and six weeks.
type calendarPane struct {
	title               *ggui.TextWidget
	titleSize           ggui.Size
	weekdayText         [7]*ggui.TextWidget
	weekdaySize         [7]ggui.Size
	days                [42]*calendarDay
	month               *SelectWidget[time.Month]
	year                *SelectWidget[int]
	monthSize, yearSize ggui.Size
	yearFrom, yearTo    int // the years the year select was last given
}

// calendarPart is the binding behind a caption dropdown: it reads what a
// pane shows and browses elsewhere when the user picks another value.
type calendarPart[T any] struct {
	get func() T
	set func(T)
}

func (p calendarPart[T]) Get() T  { return p.get() }
func (p calendarPart[T]) Set(v T) { p.set(v) }

var calendarMonths = []time.Month{time.January, time.February, time.March, time.April, time.May, time.June,
	time.July, time.August, time.September, time.October, time.November, time.December}

// Calendar binds a date; zero means no selection. Dates are interpreted in
// Location (time.Local by default), and user selections are local midnight.
func Calendar(value ggui.Binding[time.Time]) *CalendarWidget {
	property.Require(value, "Calendar")
	c := newCalendar()
	c.value = value
	return c
}

func newCalendar() *CalendarWidget {
	c := &CalendarWidget{location: time.Local, weekStart: time.Sunday, count: 1,
		weekdays:   [7]string{"Su", "Mo", "Tu", "We", "Th", "Fr", "Sa"},
		monthNames: [12]string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"},
		monthLabel: func(t time.Time) string { return t.Format("January 2006") }}
	c.Role = ggui.RoleGroup
	c.AutoKey()
	c.previousIcon, c.nextIcon = Icon(icons.ChevronLeft), Icon(icons.ChevronRight)
	c.previous = ButtonOf(c.previousIcon, func() { c.moveMonth(-1) }).Ghost().Size(ButtonIconSmall).Name("Previous month")
	c.next = ButtonOf(c.nextIcon, func() { c.moveMonth(1) }).Ghost().Size(ButtonIconSmall).Name("Next month")
	c.setPanes(1)
	return c
}

// setPanes keeps n month panes, reusing the ones already there.
func (c *CalendarWidget) setPanes(n int) {
	for len(c.panes) < n {
		p := &calendarPane{title: ggui.Text("").NoWrap()}
		for i := range p.days {
			p.days[i] = &calendarDay{owner: c, column: i % 7, text: ggui.Text("").NoWrap()}
			p.days[i].Role = ggui.RoleOption
		}
		for i := range p.weekdayText {
			p.weekdayText[i] = ggui.Text("").NoWrap()
		}
		c.panes = append(c.panes, p)
	}
	c.panes = c.panes[:n]
}

// Name sets the calendar's accessible name.
func (c *CalendarWidget) Name(s string) *CalendarWidget { c.SetName(s); return c }

// Location sets the timezone used to interpret dates. Nil means time.Local.
func (c *CalendarWidget) Location(l *time.Location) *CalendarWidget {
	defer property.Watch(&c.props, &c.active)()
	if l == nil {
		l = time.Local
	}
	// A Location is compared by identity: a deep comparison reads the
	// insides of time.Local, which another goroutine may be loading.
	if l != c.location {
		c.location = l
		c.props.Changed()
	}
	c.active, c.first = time.Time{}, time.Time{}
	return c
}

// WeekStartsOn configures the first weekday column.
func (c *CalendarWidget) WeekStartsOn(d time.Weekday) *CalendarWidget {
	defer property.Watch(&c.props, &c.weekStart)()
	if d < time.Sunday || d > time.Saturday {
		panic("ui: invalid weekday")
	}
	c.weekStart = d
	return c
}

// WeekdayLabels sets localized labels in Sunday-to-Saturday order.
func (c *CalendarWidget) WeekdayLabels(labels [7]string) *CalendarWidget {
	defer property.Watch(&c.props, &c.weekdays)()
	c.weekdays = labels
	return c
}

// MonthLabel formats the month heading; nil keeps the current formatter.
func (c *CalendarWidget) MonthLabel(fn func(time.Time) string) *CalendarWidget {
	if fn != nil {
		c.monthLabel = fn
		c.props.Changed()
	}
	return c
}

// Months shows n consecutive months side by side, wrapping onto further
// rows where the width does not hold them all. The previous and next
// buttons still move one month at a time. One month is the default.
func (c *CalendarWidget) Months(n int) *CalendarWidget {
	if n < 1 {
		panic("ui: a calendar shows at least one month")
	}
	defer property.Watch(&c.props, &c.count)()
	c.count = n
	c.setPanes(n)
	return c
}

// CaptionDropdown replaces the month heading with a month Select and a year
// Select, as shadcn's captionLayout="dropdown" does. The years offered
// follow DropdownYears, then Bounds; without either they run from a hundred
// years back to ten ahead. The year shown is always among them.
func (c *CalendarWidget) CaptionDropdown(v bool) *CalendarWidget {
	defer property.Watch(&c.props, &c.dropdown)()
	c.dropdown = v
	return c
}

// DropdownYears sets the inclusive years the caption dropdown offers. Zero
// for either end follows Bounds, then the default span around today.
func (c *CalendarWidget) DropdownYears(from, to int) *CalendarWidget {
	if from != 0 && to != 0 && to < from {
		panic("ui: invalid calendar dropdown years")
	}
	defer property.Watch(&c.props, &c.yearTo)()
	defer property.Watch(&c.props, &c.yearFrom)()
	c.yearFrom, c.yearTo = from, to
	return c
}

// MonthNames sets the localized short month names the caption dropdown
// lists, January first.
func (c *CalendarWidget) MonthNames(names [12]string) *CalendarWidget {
	if names != c.monthNames {
		c.monthNames = names
		for _, p := range c.panes {
			p.month, p.year = nil, nil // built again with the new names at the next layout
		}
		c.props.Changed()
	}
	return c
}

// Bounds sets inclusive selectable dates; a zero endpoint is unbounded.
// The previous and next buttons stop at the months that hold them.
func (c *CalendarWidget) Bounds(minimum, maximum time.Time) *CalendarWidget {
	defer property.Watch(&c.props, &c.max)()
	defer property.Watch(&c.props, &c.min)()
	c.min, c.max = minimum, maximum
	return c
}

// DisabledDate prevents selection of dates for which fn returns true.
func (c *CalendarWidget) DisabledDate(fn func(time.Time) bool) *CalendarWidget {
	c.disabledDate = fn
	c.props.Changed()
	return c
}

// Disabled disables navigation and date selection.
func (c *CalendarWidget) Disabled(v bool) *CalendarWidget { c.SetInert(v); return c }

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
	return !c.IsInert() && (c.lowest.IsZero() || !t.Before(c.lowest)) && (c.highest.IsZero() || !t.After(c.highest)) && (c.disabledDate == nil || !c.disabledDate(t))
}

// month is the first of the browsed month.
func (c *CalendarWidget) month() time.Time {
	return time.Date(c.active.Year(), c.active.Month(), 1, 0, 0, 0, 0, c.location)
}

func (c *CalendarWidget) show(t time.Time) {
	c.active = t
	ggui.Invalidate(c.env)
}
func (c *CalendarWidget) syncDate() {
	v := c.selectionAnchor()
	if c.active.IsZero() || !v.Equal(c.last) {
		if v.IsZero() {
			c.show(c.date(ggui.Now()))
		} else {
			c.show(v)
		}
		c.last = v
	}
}

// syncView moves the months shown just far enough to hold the browsed date.
func (c *CalendarWidget) syncView() {
	m := c.month()
	switch {
	case c.first.IsZero() || m.Before(c.first):
		c.first = m
	case m.After(c.first.AddDate(0, c.count-1, 0)):
		c.first = m.AddDate(0, 1-c.count, 0)
	}
}

// moveMonth browses n months on, keeping the day of the month where the
// month is long enough, and slides the months shown along with it.
func (c *CalendarWidget) moveMonth(n int) {
	if c.IsInert() || n == 0 {
		return
	}
	c.syncDate()
	c.syncView()
	c.first = c.first.AddDate(0, n, 0)
	m := c.month().AddDate(0, n, 0)
	last := m.AddDate(0, 1, -1).Day()
	c.show(m.AddDate(0, 0, min(c.active.Day(), last)-1))
}

// canBrowse reports whether a month of the bounds lies before the months
// shown, for a negative direction, or after them.
func (c *CalendarWidget) canBrowse(direction int) bool {
	if direction < 0 {
		return c.lowest.IsZero() || c.lowest.Before(c.first)
	}
	return c.highest.IsZero() || !c.highest.Before(c.first.AddDate(0, c.count, 0))
}

// step browses to the date move gives for the browsed one and, while that
// day cannot be chosen, on to the one move gives for it: the keyboard
// passes over disabled days as react-day-picker's focus does, and stays
// where it is when a year of steps reaches none. The months shown slide
// by as many months as the date moved when slide is set, as PageUp and
// PageDown have them do.
func (c *CalendarWidget) step(slide bool, move func(time.Time) time.Time) {
	d := c.active
	for range 366 {
		next := move(d)
		if next.Equal(d) {
			return
		}
		if d = next; !c.enabled(d) {
			continue
		}
		if slide {
			c.syncView()
			c.first = c.first.AddDate(0, (d.Year()-c.active.Year())*12+int(d.Month()-c.active.Month()), 0)
		}
		c.show(d)
		return
	}
}

// addMonths is the same day n months from d, or the last day of a month
// too short to have it.
func (c *CalendarWidget) addMonths(d time.Time, n int) time.Time {
	m := time.Date(d.Year(), d.Month(), 1, 0, 0, 0, 0, c.location).AddDate(0, n, 0)
	return m.AddDate(0, 0, min(d.Day(), m.AddDate(0, 1, -1).Day())-1)
}

// browse shows year and month in the pane at index, as its dropdowns ask.
func (c *CalendarWidget) browse(index, year int, month time.Month) {
	shown := c.first.AddDate(0, index, 0)
	c.moveMonth((year-shown.Year())*12 + int(month-shown.Month()))
}

func (c *CalendarWidget) choose(t time.Time) {
	if !c.enabled(t) {
		return
	}
	c.show(t)
	if c.rangeValue != nil {
		c.chooseRange(t)
	} else if c.multipleValue != nil {
		c.chooseMultiple(t)
	} else if !c.date(ggui.Untrack(c.value.Get)).Equal(t) {
		c.value.Set(t)
		if c.onChange != nil {
			c.onChange(t)
		}
	}
	c.last = ggui.Untrack(c.selectionAnchor)
	if c.onPick != nil {
		c.onPick()
	}
}

// layoutDropdown measures the month and year selects of the pane at index,
// building them the first time.
func (c *CalendarWidget) layoutDropdown(p *calendarPane, index int, cs ggui.Constraints, env ggui.Env) {
	if p.month == nil {
		shown := func() time.Time { return c.first.AddDate(0, index, 0) }
		p.month = Select[time.Month](calendarPart[time.Month]{
			get: func() time.Month { return shown().Month() },
			set: func(m time.Month) { c.browse(index, shown().Year(), m) },
		}).Format(func(m time.Month) string { return c.monthNames[(int(m)+11)%12] }).Options(calendarMonths).Name("Month")
		p.year = Select[int](calendarPart[int]{
			get: func() int { return shown().Year() },
			set: func(y int) { c.browse(index, y, shown().Month()) },
		}).Name("Year")
		p.yearFrom, p.yearTo = 0, 0
	}
	if from, to := c.years[0], c.years[len(c.years)-1]; p.yearFrom != from || p.yearTo != to {
		p.yearFrom, p.yearTo = from, to
		p.year.Options(c.years)
	}
	p.month.Disabled(c.IsInert())
	p.year.Disabled(c.IsInert())
	p.monthSize = p.month.Layout(cs, env)
	p.yearSize = p.year.Layout(cs, env)
}

// syncYears lists the years the dropdown offers, widened to the years shown.
func (c *CalendarWidget) syncYears() {
	from, to := c.yearFrom, c.yearTo
	if from == 0 {
		from = pick(c.lowest.IsZero(), c.today.Year()-100, c.lowest.Year())
	}
	if to == 0 {
		to = pick(c.highest.IsZero(), c.today.Year()+10, c.highest.Year())
	}
	from = min(from, c.first.Year())
	to = max(to, from, c.first.AddDate(0, c.count-1, 0).Year())
	if n := to - from + 1; len(c.years) != n || c.years[0] != from {
		c.years = make([]int, n)
		for i := range c.years {
			c.years[i] = from + i
		}
	}
}

// Layout implements ggui.Widget.
func (c *CalendarWidget) Layout(cs ggui.Constraints, env ggui.Env) ggui.Size {
	defer c.props.Layout().EndLayout()
	c.Sync()
	c.env = env
	angle := pick(env.Direction() == ggui.DirRTL, math.Pi, 0.0)
	c.previousIcon.Rotate(angle)
	c.nextIcon.Rotate(angle)
	t := uitheme.Of(env)
	c.theme = t
	c.syncDate()
	c.syncView()
	c.lowest, c.highest = c.date(c.min), c.date(c.max)
	// FrameTime, not Now: marking today must not keep the app painting.
	c.today = c.date(ggui.FrameTime())
	c.resolveSelection()
	// As react-day-picker's start and end months do, the bounds end the
	// browsing: a button that would show only months outside them is off.
	c.previous.Disabled(c.IsInert() || !c.canBrowse(-1))
	c.next.Disabled(c.IsInert() || !c.canBrowse(1))

	m := &c.metrics
	m.pad, m.cell, m.rowGap, m.monthGap, m.captionGap = t.Space, 3.5*t.Space, t.Space, 2*t.Space, 2*t.Space
	loose := ggui.Loose(ggui.Sz(cs.MaxW, cs.MaxH))
	m.nav = c.previous.Layout(loose, env)
	c.next.Layout(loose, env)
	if c.dropdown {
		c.syncYears()
	}
	captionW := 0.0
	m.caption = max(m.cell, m.nav.H)
	captionStyle := t.Text.Merge(t.Label)
	for i, p := range c.panes {
		size := ggui.Size{}
		if c.dropdown {
			c.layoutDropdown(p, i, loose, env)
			size = ggui.Sz(p.monthSize.W+calendarDropdownGap(t)+p.yearSize.W, max(p.monthSize.H, p.yearSize.H))
		} else {
			p.titleSize = p.title.Content(c.monthLabel(c.first.AddDate(0, i, 0))).Style(captionStyle).Layout(loose, env)
			size = p.titleSize
		}
		captionW, m.caption = max(captionW, size.W), max(m.caption, size.H)
	}

	// A month is seven cells wide, or as wide as its caption between the
	// two buttons; as many months share a row as the width holds.
	monthW := max(7*m.cell, captionW+2*(m.nav.W+t.Space/2))
	m.perRow = c.count
	if avail := cs.MaxW - 2*m.pad; !math.IsInf(avail, 1) {
		m.perRow = clamp(int((avail+m.monthGap)/(monthW+m.monthGap)), 1, c.count)
	}
	gaps := float64(m.perRow-1) * m.monthGap
	w := cs.Constrain(ggui.Sz(2*m.pad+float64(m.perRow)*monthW+gaps, 0)).W
	m.monthW = max(0, (w-2*m.pad-gaps)/float64(m.perRow))
	m.column = m.monthW / 7
	m.cell = min(m.cell, m.column)

	cell := ggui.Loose(ggui.Sz(m.column, m.cell))
	disabled := fade(t.MutedFg, .5)
	c.hover = pick[color.Color](isDark(t.Bg), fade(t.Muted, .5), t.Muted)
	end := c.first.AddDate(0, c.count, 0)
	m.weekday = 0
	for i, p := range c.panes {
		for j, label := range p.weekdayText {
			p.weekdaySize[j] = label.Content(c.weekdays[(j+int(c.weekStart))%7]).Color(t.MutedFg).Size(12.8).Layout(cell, env)
			m.weekday = max(m.weekday, p.weekdaySize[j].H)
		}
		month := c.first.AddDate(0, i, 0)
		start := month.AddDate(0, 0, -(int(month.Weekday())-int(c.weekStart)+7)%7)
		for j, d := range p.days {
			if date := start.AddDate(0, 0, j); !date.Equal(d.date) {
				d.date = date
				d.SetName(date.Format("2006-01-02"))
			}
			d.outside = d.date.Month() != month.Month()
			// With several months a day outside its month may be another
			// pane's own day; only that one is described.
			d.repeat = d.outside && c.count > 1 && !d.date.Before(c.first) && d.date.Before(end)
			d.mark = c.selection.mark(d.date)
			d.today = d.date.Equal(c.today)
			d.SetInert(!c.enabled(d.date))
			col := t.Fg
			switch {
			case d.mark == calendarSingle || d.mark == calendarStart || d.mark == calendarEnd:
				col = t.PrimaryFg
			case d.IsInert():
				col = disabled
			case d.mark == calendarMiddle || d.today:
			case d.outside:
				col = t.MutedFg
			}
			d.size = d.text.Content(strconv.Itoa(d.date.Day())).Color(col).Layout(cell, env)
		}
	}
	m.monthH = m.caption + m.captionGap + m.weekday + 6*(m.rowGap+m.cell)
	rows := float64((c.count + m.perRow - 1) / m.perRow)
	return cs.Constrain(ggui.Sz(w, 2*m.pad+rows*m.monthH+(rows-1)*m.monthGap))
}

// calendarDropdownGap separates the month select from the year select.
func calendarDropdownGap(t *uitheme.Theme) float64 { return t.Space * .75 }

// paneRect is where the pane at index sits in r, before mirroring.
func (m *calendarMetrics) paneRect(index int, r ggui.Rect) ggui.Rect {
	x := r.Origin.X + m.pad + float64(index%m.perRow)*(m.monthW+m.monthGap)
	y := r.Origin.Y + m.pad + float64(index/m.perRow)*(m.monthH+m.monthGap)
	return ggui.Rct(ggui.Pt(x, y), ggui.Sz(m.monthW, m.monthH))
}

// gridTop is the top of a pane's first week, including half the row gap
// that belongs to each day's hit target.
func (m *calendarMetrics) gridTop(pane ggui.Rect) float64 {
	return pane.Origin.Y + m.caption + m.captionGap + m.weekday + m.rowGap/2
}

// dayRect is the hit target of the day at index in pane: a whole column
// wide and a row with its gap tall.
func (m *calendarMetrics) dayRect(pane ggui.Rect, index int) ggui.Rect {
	return ggui.Rct(ggui.Pt(pane.Origin.X+float64(index%7)*m.column, m.gridTop(pane)+float64(index/7)*(m.cell+m.rowGap)), ggui.Sz(m.column, m.cell+m.rowGap))
}

// fill is the square a day paints, centred in its hit target.
func (m *calendarMetrics) fill(day ggui.Rect) ggui.Rect {
	return ggui.Rct(ggui.Pt(day.Origin.X+(day.Size.W-m.cell)/2, day.Origin.Y+(day.Size.H-m.cell)/2), ggui.Sz(m.cell, m.cell))
}

// Paint implements ggui.Widget.
func (c *CalendarWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	dst.Node(r, ggui.Node{Role: ggui.RoleGroup, Name: c.name(), Disabled: c.IsInert()}, func(dst *ggui.Canvas) {
		m, t := &c.metrics, c.theme
		rtl := c.env.Direction() == ggui.DirRTL
		head, tail := m.paneRect(0, r), m.paneRect(m.perRow-1, r)
		navY := head.Origin.Y + (m.caption-m.nav.H)/2
		dst.Paint(c.previous, mirrorHorizontal(ggui.Rct(ggui.Pt(head.Origin.X, navY), m.nav), r, rtl))
		dst.Paint(c.next, mirrorHorizontal(ggui.Rct(ggui.Pt(tail.Origin.X+tail.Size.W-m.nav.W, navY), m.nav), r, rtl))
		if !c.IsInert() {
			top := m.gridTop(head)
			dst.HitKey(ggui.Rct(ggui.Pt(r.Origin.X+m.pad, top), ggui.Sz(r.Size.W-2*m.pad, r.Origin.Y+r.Size.H-m.pad-top)), c)
		}
		cells := dst.Clip(r)
		var focus ggui.Rect
		for i, p := range c.panes {
			pane := m.paneRect(i, r)
			if c.dropdown && p.month != nil {
				x := pane.Origin.X + (pane.Size.W-p.monthSize.W-calendarDropdownGap(t)-p.yearSize.W)/2
				dst.Paint(p.month, mirrorHorizontal(ggui.Rct(ggui.Pt(x, pane.Origin.Y+(m.caption-p.monthSize.H)/2), p.monthSize), r, rtl))
				x += p.monthSize.W + calendarDropdownGap(t)
				dst.Paint(p.year, mirrorHorizontal(ggui.Rct(ggui.Pt(x, pane.Origin.Y+(m.caption-p.yearSize.H)/2), p.yearSize), r, rtl))
			} else {
				at := ggui.Pt(pane.Origin.X+(pane.Size.W-p.titleSize.W)/2, pane.Origin.Y+(m.caption-p.titleSize.H)/2)
				dst.Paint(p.title, mirrorHorizontal(ggui.Rct(at, p.titleSize), r, rtl))
			}
			y := pane.Origin.Y + m.caption + m.captionGap
			for j, label := range p.weekdayText {
				s := p.weekdaySize[j]
				dst.Paint(label, mirrorHorizontal(ggui.Rct(ggui.Pt(pane.Origin.X+float64(j)*m.column+(m.column-s.W)/2, y+(m.weekday-s.H)/2), s), r, rtl))
			}
			for j, d := range p.days {
				day := mirrorHorizontal(m.dayRect(pane, j), r, rtl)
				cells.Paint(d, day)
				if !d.repeat && d.date.Equal(c.active) {
					focus = m.fill(day)
				}
			}
		}
		// Last, so the halo lies over the neighbouring days' fills.
		if focus.Size.W > 0 {
			controlFocusRing(dst, &c.Interactive, t, focus, t.RadiusSm, t.Ring)
		}
	})
}

// ConsumesKey implements ggui.KeyConsumer.
func (c *CalendarWidget) ConsumesKey(e ggui.KeyEvent) bool {
	if c.IsInert() || e.Kind != ggui.KeyPress {
		return false
	}
	switch e.Key {
	case ggui.KeyArrowLeft, ggui.KeyArrowRight, ggui.KeyArrowUp, ggui.KeyArrowDown, ggui.KeyHome, ggui.KeyEnd, ggui.KeyPageUp, ggui.KeyPageDown:
		return true
	}
	return ggui.Activates(e)
}

// HandleKey navigates by day and week with the arrows, to the ends of the
// week with Home and End, by month with PageUp and PageDown and by year
// with Shift held, passing over the days that cannot be chosen.
func (c *CalendarWidget) HandleKey(e ggui.KeyEvent) {
	c.Keyboard(e, nil)
	if c.IsInert() || e.Kind != ggui.KeyPress {
		return
	}
	c.syncDate()
	e = directionalKey(e, c.env.Direction() == ggui.DirRTL)
	days := func(n int) func(time.Time) time.Time {
		return func(d time.Time) time.Time { return d.AddDate(0, 0, n) }
	}
	weekday := func(d time.Time) int { return (int(d.Weekday()) - int(c.weekStart) + 7) % 7 }
	slide := false
	var move func(time.Time) time.Time
	switch e.Key {
	case ggui.KeyArrowLeft:
		move = days(-1)
	case ggui.KeyArrowRight:
		move = days(1)
	case ggui.KeyArrowUp:
		move = days(-7)
	case ggui.KeyArrowDown:
		move = days(7)
	case ggui.KeyHome:
		move = func(d time.Time) time.Time { return d.AddDate(0, 0, -weekday(d)) }
	case ggui.KeyEnd:
		move = func(d time.Time) time.Time { return d.AddDate(0, 0, 6-weekday(d)) }
	case ggui.KeyPageUp, ggui.KeyPageDown:
		n := pick(e.Key == ggui.KeyPageUp, -1, 1) * pick(e.Mods.Shift, 12, 1)
		slide, move = true, func(d time.Time) time.Time { return c.addMonths(d, n) }
	default:
		if ggui.Activates(e) {
			c.choose(c.active)
		}
		return
	}
	// A grid focused by a click shows no ring until the keys browse it.
	c.FocusVisible = true
	c.step(slide, move)
}

// Adopt preserves the browsed month across rebuilds; binding changes still win.
func (c *CalendarWidget) Adopt(prev any) {
	c.Interactive.Adopt(prev)
	if p, ok := prev.(*CalendarWidget); ok && p.location == c.location {
		c.active, c.last, c.first = p.active, p.last, p.first
		c.syncDate()
	}
}

type calendarDay struct {
	ggui.Interactive
	owner   *CalendarWidget
	date    time.Time
	text    *ggui.TextWidget
	size    ggui.Size
	column  int          // 0 on the side the week starts
	mark    calendarMark // its part in the selection
	today   bool
	outside bool // belongs to the month before or after its pane's
	repeat  bool // outside, and shown by another pane as its own
}

func (d *calendarDay) Layout(cs ggui.Constraints, _ ggui.Env) ggui.Size { return cs.Constrain(d.size) }
func (d *calendarDay) Describe() ggui.Node {
	return ggui.Node{Role: ggui.RoleOption, Name: d.SemanticName(), Selected: d.mark != calendarUnmarked, Disabled: d.IsInert(), Actions: ggui.ActionPress | ggui.ActionSelect}
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

// Paint fills the day's square inside r, its hit target. A range paints as
// one band: the days between its ends fill the whole column, rounded only
// where a week begins or ends, and each end lays the band's half behind its
// own primary square.
func (d *calendarDay) Paint(dst *ggui.Canvas, r ggui.Rect) {
	c := d.owner
	t, m := c.theme, &c.metrics
	if !d.repeat {
		dst.Describe(r, d)
	}
	if !d.IsInert() {
		dst.HitPointer(r, d)
		dst.HitCursor(r, ggui.CursorShapePointer)
	}
	fill := m.fill(r)
	center := fill.Origin.X + fill.Size.W/2
	// The halves of the column on the side of earlier and of later dates.
	before := ggui.Rct(ggui.Pt(r.Origin.X, fill.Origin.Y), ggui.Sz(center-r.Origin.X, fill.Size.H))
	after := ggui.Rct(ggui.Pt(center, fill.Origin.Y), ggui.Sz(r.Origin.X+r.Size.W-center, fill.Size.H))
	if c.env.Direction() == ggui.DirRTL {
		before, after = after, before
	}
	weekStart, weekEnd := d.column == 0, d.column == 6
	switch d.mark {
	case calendarMiddle:
		if weekStart || weekEnd {
			dst.FillRoundRect(fill, t.RadiusSm, t.Muted)
		}
		if !weekStart {
			dst.FillRect(before, t.Muted)
		}
		if !weekEnd {
			dst.FillRect(after, t.Muted)
		}
	case calendarStart:
		if !weekEnd {
			dst.FillRect(after, t.Muted)
		}
		dst.FillRoundRect(fill, t.RadiusSm, t.Primary)
	case calendarEnd:
		if !weekStart {
			dst.FillRect(before, t.Muted)
		}
		dst.FillRoundRect(fill, t.RadiusSm, t.Primary)
	case calendarSingle:
		dst.FillRoundRect(fill, t.RadiusSm, t.Primary)
	default:
		if d.today {
			dst.FillRoundRect(fill, t.RadiusSm, t.Muted)
		} else if d.Hovered && !d.IsInert() {
			dst.FillRoundRect(fill, t.RadiusSm, c.hover)
		}
	}
	dst.Paint(d.text, ggui.Rct(ggui.Pt(fill.Origin.X+(fill.Size.W-d.size.W)/2, fill.Origin.Y+(fill.Size.H-d.size.H)/2), d.size))
}

// BindDisabled follows r for Disabled without rebuilding the control.
func (c *CalendarWidget) BindDisabled(r ggui.Readable[bool]) *CalendarWidget {
	c.BindInert(r)
	return c
}

func (c *CalendarWidget) name() string {
	return pick(c.SemanticName() != "", c.SemanticName(), "Calendar")
}

// Semantics implements ggui.Semantic, including the built-in fallback name.
func (c *CalendarWidget) Semantics() (ggui.Role, string) { return c.Role, c.name() }
