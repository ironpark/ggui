package ui

import (
	"image/color"
	"slices"
	"strings"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/ui/icons"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// SelectWidget is a dropdown that picks one of a list of values into a
// signal. Build one with Select.
type SelectWidget[T comparable] struct {
	props property.Owner
	ggui.Interactive
	value         ggui.Binding[T]
	options       []T
	optionProp    property.Value[[]T]
	label         func(T) string
	group         func(T) string // the heading an option goes under, if grouped
	disabled      func(T) bool   // the options that cannot be chosen
	placeholder   string
	small         bool
	invalid       bool // set by Invalid
	shownInvalid  bool // Invalid, or inside an invalid Field
	minWidth      float64
	onChange      func(T)
	alignItem     bool
	wasOpen       bool
	native        bool
	anchorRect    ggui.Rect
	alignedHeight float64
	placedAnchor  ggui.Rect
	placedScreen  ggui.Size

	popup          *ggui.PopupWidget
	text           *ggui.TextWidget
	box            *ggui.BoxWidget // the field's fill and border
	viewport       *selectViewport[T]
	list           *ggui.BoxWidget // the open list's panel
	items          []*selectItem[T]
	rowOf          []int // each option's place among the list's rows, which include headings and separators
	motion         time.Duration
	pad            ggui.EdgeInsets
	textSize       ggui.Size
	triggerContent ggui.Widget
	triggerSize    ggui.Size
	highlight      int         // the option the keyboard is on while open, or -1
	sides          JoinedSides // the sides a ButtonGroup joins the field on
	typed          string      // the letters of the typeahead search under way
	typedAt        time.Time   // when the last of them was typed
	theme          *uitheme.Theme
	env            ggui.Env
}

// Select creates a dropdown bound to value, showing each option through
// fmt.Sprint until Format says otherwise. Space, Enter, Up or Down opens it;
// the arrow keys, Home and End move through the options and Escape closes
// it. Typing letters goes to the option they start: it is highlighted in
// the open list, and chosen while the list is closed. The options slice is
// shallow-copied.
func Select[T comparable](value ggui.Binding[T]) *SelectWidget[T] {
	s := &SelectWidget[T]{value: value, label: sprint[T], highlight: -1, alignItem: true}
	s.Role = ggui.RoleSelect
	s.AutoKey()
	s.box = ggui.Box()
	s.text = ggui.Text("").NoWrap()
	column := newMenuColumn(nil)
	column.fill = true
	s.viewport = &selectViewport[T]{owner: s, column: column, offset: ggui.State(0.0)}
	s.viewport.scroll = ggui.Scroll(column).BindOffset(s.viewport.offset)
	s.list = ggui.Box(s.viewport)
	s.popup = ggui.Popup(selectAnchor[T]{s}, menuSurface(s.list)).Keys(s).Owner(s)
	return s
}

// NativeSelect prefers the platform's checked native menu and falls back to
// the regular Select popup when no native menu is available (including probes).
func NativeSelect[T comparable](value ggui.Binding[T]) *SelectWidget[T] {
	return Select(value).Native(true)
}

// Native chooses whether opening first attempts a platform-native menu.
func (s *SelectWidget[T]) Native(v bool) *SelectWidget[T] { s.native = v; return s }

func (s *SelectWidget[T]) show() {
	if s.IsInert() {
		return
	}
	if s.native {
		entries := make([]ggui.MenuItem, len(s.options))
		selected := ggui.Untrack(s.value.Get)
		for j, value := range s.options {
			entries[j] = ggui.MenuAction(s.label(value), "", func() {
				if !s.IsInert() {
					setChanged(s.value, value, s.onChange)
				}
			}).BindChecked(ggui.Const(value == selected))
			if s.disabled != nil && s.disabled(value) {
				entries[j] = entries[j].BindEnabled(ggui.Const(false))
			}
		}
		if w := ggui.UseWindow(); w != nil && w.ShowContextMenu(s.anchorRect.Origin.Add(ggui.Pt(0, s.anchorRect.Size.H)), entries...) {
			return
		}
	}
	s.popup.Show()
}

// Options replaces the options with a shallow copy, including after mount,
// and removes any BindOptions binding. It leaves the value and open state
// unchanged and does not call OnChange. If the value is absent it remains
// displayed, with no highlighted option until keyboard or pointer input.
func (s *SelectWidget[T]) Options(options []T) *SelectWidget[T] {
	changed := s.setOptions(options)
	detached := s.optionProp.Set(s.options)
	if changed || detached {
		s.props.Changed()
	}
	return s
}

// BindOptions follows r at layout without rebuilding the control. Changes
// use the same snapshot and selection rules as Options. The last setting
// wins; r must be non-nil. Use Options to detach the source.
func (s *SelectWidget[T]) BindOptions(r ggui.Readable[[]T]) *SelectWidget[T] {
	if s.optionProp.Bind(r, "BindOptions") {
		s.props.Changed()
	}
	return s
}

func (s *SelectWidget[T]) setOptions(options []T) bool {
	if slices.Equal(s.options, options) {
		return false
	}
	s.options = slices.Clone(options)
	s.build()
	return true
}

// build makes the list's rows of the options: one row an option, under a
// heading where GroupBy names one, with a separator between groups.
func (s *SelectWidget[T]) build() {
	s.items = make([]*selectItem[T], len(s.options))
	s.rowOf = make([]int, len(s.options))
	rows := make([]ggui.Widget, 0, len(s.options))
	previous := ""
	for i, value := range s.options {
		if s.group != nil {
			group := s.group(value)
			if i > 0 && group != previous {
				rows = append(rows, MenuDivider())
			}
			if group != "" && (i == 0 || group != previous) {
				rows = append(rows, &MenuLabelWidget{text: Caption(group).NoWrap()})
			}
			previous = group
		}
		name := s.label(value)
		it := &selectItem[T]{owner: s, index: i, text: ggui.Text(name).NoWrap()}
		it.Role = ggui.RoleOption
		it.SetName(name)
		it.SetInert(s.disabled != nil && s.disabled(value))
		// Replaced rows must not adopt a removed row's pointer state by position.
		it.SetKey(it)
		s.items[i], s.rowOf[i] = it, len(rows)
		rows = append(rows, it)
	}
	s.viewport.column = newMenuColumn(rows)
	s.viewport.column.fill = true
	s.viewport.scroll = ggui.Scroll(s.viewport.column).BindOffset(s.viewport.offset)
	s.highlight = s.index()
	s.viewport.reveal = true
}

// Format sets how each option is shown.
func (s *SelectWidget[T]) Format(fn func(T) string) *SelectWidget[T] {
	s.label = fn
	s.build()
	s.props.Changed()
	return s
}

// GroupBy puts each option under the heading fn names for it, with a
// separator between one group and the next. Options of a group are the
// ones that follow each other; an empty name gives a group no heading.
// Passing nil lists the options flat again.
func (s *SelectWidget[T]) GroupBy(fn func(T) string) *SelectWidget[T] {
	s.group = fn
	s.build()
	s.props.Changed()
	return s
}

// DisabledOption greys out the options fn reports and keeps the pointer
// and the keyboard from choosing them. Passing nil enables them all.
func (s *SelectWidget[T]) DisabledOption(fn func(T) bool) *SelectWidget[T] {
	s.disabled = fn
	s.build()
	s.props.Changed()
	return s
}

// Placeholder sets the muted text the field shows while its value is not
// one of the options. Without one the field shows the value itself.
func (s *SelectWidget[T]) Placeholder(text string) *SelectWidget[T] {
	defer property.Watch(&s.props, &s.placeholder)()
	s.placeholder = text
	return s
}

// Small makes the field 28 pixels tall instead of 32, for dense toolbars
// and table rows.
func (s *SelectWidget[T]) Small() *SelectWidget[T] {
	defer property.Watch(&s.props, &s.small)()
	s.small = true
	return s
}

// Invalid marks the field as holding a value that failed validation: a
// Destructive border and ring. A Select inside a Field with an error is
// marked without it.
func (s *SelectWidget[T]) Invalid(v bool) *SelectWidget[T] {
	defer property.Watch(&s.props, &s.invalid)()
	s.invalid = v
	return s
}

// enabled reports whether option i can be chosen.
func (s *SelectWidget[T]) enabled(i int) bool { return !s.items[i].IsInert() }

// rowSpan is where option i sits in the list: its top, from the top of
// the first row, and its height.
func (s *SelectWidget[T]) rowSpan(i int) (top, height float64) {
	sizes := s.viewport.column.sizes
	for _, size := range sizes[:s.rowOf[i]] {
		top += size.H
	}
	return top, sizes[s.rowOf[i]].H
}

// radius is the field's corner radius.
func (s *SelectWidget[T]) radius(t *uitheme.Theme) float64 {
	if s.small {
		return min(t.RadiusSm, 10)
	}
	return t.Radius
}

// Describe implements ggui.Describer: the chosen option and whether the
// list is showing. The options themselves paint through the popup's
// overlay and hang under this node, not beside the tree.
func (s *SelectWidget[T]) Describe() ggui.Node {
	return ggui.Node{
		Role:     ggui.RoleSelect,
		Name:     s.SemanticName(),
		Value:    s.label(ggui.Untrack(s.value.Get)),
		Expanded: ggui.Expandable(s.popup.IsOpen()),
		Disabled: s.IsInert(),
		Actions: ggui.ActionPress | ggui.ActionFocus | ggui.ActionSelect |
			pick(s.popup.IsOpen(), ggui.ActionCollapse, ggui.ActionExpand),
	}
}

// Act implements ggui.Actor: opening and closing the list, and choosing
// the option a Select action names by value.
func (s *SelectWidget[T]) Act(a ggui.Action) bool {
	if s.IsInert() {
		return false
	}
	switch a.Kind {
	case ggui.ActionExpand:
		s.highlight = s.index()
		s.viewport.reveal = true
		s.show()
	case ggui.ActionCollapse:
		s.popup.Hide()
	case ggui.ActionSetValue, ggui.ActionSelect:
		for i, o := range s.options {
			if s.label(o) == a.Text && s.enabled(i) {
				s.choose(i)
				return true
			}
		}
		return false
	default:
		return false
	}
	return true
}

// Name names the dropdown for Probe.Find and the inspector.
func (s *SelectWidget[T]) Name(name string) *SelectWidget[T] { s.SetName(name); return s }

// ConsumesKey implements ggui.KeyConsumer: Up and Down open the list or
// move through it, and Home and End go to its ends while it is open.
func (s *SelectWidget[T]) ConsumesKey(ev ggui.KeyEvent) bool {
	if ev.Kind != ggui.KeyPress {
		return false
	}
	switch ev.Key {
	case ggui.KeyArrowUp, ggui.KeyArrowDown:
		return true
	case ggui.KeyHome, ggui.KeyEnd:
		return s.popup.IsOpen()
	}
	return ggui.Activates(ev)
}

// Disabled greys the dropdown out and ignores input while v is true.
func (s *SelectWidget[T]) Disabled(v bool) *SelectWidget[T] { s.SetInert(v); return s }

// BindDisabled follows r for Disabled without a rebuild.
func (s *SelectWidget[T]) BindDisabled(r ggui.Readable[bool]) *SelectWidget[T] {
	s.BindInert(r)
	return s
}

// TriggerContent replaces the visible label and chevron with a decorative
// widget inside the field's padding. The Select keeps its own input handling,
// accessible name, formatted value and option list; the face cannot take focus
// or intercept presses. Bind the face's text to value when it shows selection.
// Passing nil restores the default label and chevron.
func (s *SelectWidget[T]) TriggerContent(child ggui.Widget) *SelectWidget[T] {
	if !property.Same(s.triggerContent, child) {
		s.triggerContent = child
		s.props.Changed()
	}
	return s
}

// MinWidth sets the least width the field asks for; it is otherwise as
// wide as its widest option or custom trigger content, and fills a tight width.
func (s *SelectWidget[T]) MinWidth(w float64) *SelectWidget[T] {
	defer property.Watch(&s.props, &s.minWidth)()
	s.minWidth = w
	return s
}

// OnChange fires with the new value after the user picks one.
func (s *SelectWidget[T]) OnChange(fn func(T)) *SelectWidget[T] { s.onChange = fn; return s }

// AlignItemWithTrigger places the selected row over the trigger when opening
// (the default). Near window edges the list scrolls to preserve that alignment.
// False restores the usual below/above dropdown placement.
func (s *SelectWidget[T]) AlignItemWithTrigger(v bool) *SelectWidget[T] {
	defer property.Watch(&s.props, &s.alignItem)()
	if s.alignItem != v {
		s.wasOpen = false
	}
	s.alignItem = v
	return s
}

// Popup returns the popup the options open in.
func (s *SelectWidget[T]) Popup() *ggui.PopupWidget { return s.popup }

// Layout implements Widget.
func (s *SelectWidget[T]) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer s.props.Layout().EndLayout()
	s.setOptions(s.optionProp.Get())
	s.Sync()
	inert := s.IsInert()
	if inert {
		s.popup.Hide()
	}
	t := uitheme.Of(env)
	s.theme = t
	// The sides a ButtonGroup joins the field on are the field's own: the
	// list it opens is laid out without them.
	s.sides, env = takeJoinedSides(env)
	s.env = env
	s.motion = env.Motion(t.MotionFast)
	inField, _ := env.Get(fieldInvalid)
	s.shownInvalid = s.invalid || inField
	s.pad = t.FieldPad
	if s.small {
		s.pad.Top, s.pad.Bottom = max(s.pad.Top-t.Space/4, 0), max(s.pad.Bottom-t.Space/4, 0)
	}
	if s.triggerContent == nil {
		// The chevron stands 8px in from the edge of a Select, shadcn's
		// pr-2, and 10px in from a NativeSelect's, its right-2.5.
		s.pad.Right = pick(s.native, t.Space*1.25, t.Space)
	}
	s.box.Radius(s.radius(t))
	if t.Menu().Popover != t.Popover {
		mt := t.Menu()
		panelBox(s.list, &mt)
	} else {
		panelBox(s.list, t)
	}
	// A list that opens over its trigger appears at once: it has no side
	// to come from.
	index := s.index()
	s.popup.Animated(!(s.alignItem && index >= 0))
	least := t.Space * 18 // the list is never narrower than this
	// The label is one Text for the field's life, so its shaping survives
	// a relayout that changes neither the value nor the style.
	var ink color.Color = t.Fg
	content := ""
	if index < 0 && s.placeholder != "" {
		ink, content = t.MutedFg, s.placeholder
	} else {
		content = s.label(ggui.Untrack(s.value.Get))
	}
	if inert {
		ink = t.Disabled(ink)
	}
	s.text.Content(content).Color(ink)
	minH := 20 + s.pad.Vertical()
	if s.triggerContent != nil {
		// A custom face owns all available inner width, including the space the
		// default chevron occupies. Measure again at the chosen width so wrapped
		// labels and trailing icons respond to MinWidth and stretched layouts.
		faceEnv := env.WithText(ggui.TextStyle{Color: ink})
		inner := s.pad.Shrink(c).Loosen()
		natural := s.triggerContent.Layout(inner, faceEnv)
		w := clamp(max(s.pad.Inflate(natural).W, s.minWidth), c.MinW, c.MaxW)
		inner.MinW = max(0, w-s.pad.Horizontal())
		inner.MaxW = inner.MinW
		s.triggerSize = s.triggerContent.Layout(inner, faceEnv)
		s.list.Width(max(w, least))
		return s.popup.Layout(ggui.Constraints{MinW: w, MaxW: w, MinH: min(c.MaxH, max(c.MinH, minH)), MaxH: c.MaxH}, env)
	}
	// As wide as the widest option, so the field does not resize as the
	// value changes, and never wider than the row wants unless told to.
	widest := 0.0
	for _, it := range s.items {
		widest = max(widest, it.text.Layout(ggui.Loose(ggui.Sz(ggui.Unbounded, c.MaxH)), env).W)
	}
	if s.placeholder != "" && index < 0 {
		widest = max(widest, s.text.Layout(ggui.Loose(ggui.Sz(ggui.Unbounded, c.MaxH)), env).W)
	}
	// The field holds its label, a gap and the chevron; the list under it,
	// just as wide, holds the same label and the room for a check mark.
	trail := s.chevronRoom(t)
	natural := widest + max(s.pad.Left+trail, t.PanelPad.Horizontal()+t.ItemPad.Left+menuMarkRoom(t))
	w := clamp(max(natural, s.minWidth), c.MinW, c.MaxW)
	s.list.Width(max(w, least)) // the popup follows its trigger, not the window width
	s.textSize = s.text.Layout(ggui.Loose(ggui.Sz(max(w-s.pad.Left-trail, 0), c.MaxH)), env)
	inner := ggui.Constraints{MinW: w, MaxW: w, MinH: min(c.MaxH, max(c.MinH, minH)), MaxH: c.MaxH}
	return s.popup.Layout(inner, env)
}

// chevronRoom is the width the field keeps right of its label: a gap, the
// chevron and the padding beside it.
func (s *SelectWidget[T]) chevronRoom(t *uitheme.Theme) float64 {
	return menuGap(t) + t.IconSize + s.pad.Right
}

// selectHoverSlot keeps the field's hover fill easing across rebuilds.
var selectHoverSlot = ggui.NewSlot[*ggui.Motion]("select hover")

// paintField paints the box with the current label and a chevron; it is
// what the popup's anchor draws.
func (s *SelectWidget[T]) paintField(dst *ggui.Canvas, r ggui.Rect) {
	t := s.theme
	inert := s.IsInert()
	s.anchorRect = r
	s.Hit(dst, r, s, ggui.CursorShapeDefault)
	s.positionList(dst, r)
	radius := s.radius(t)
	// On a dark theme the field lightens under the pointer.
	fill := t.Input
	if hover := dst.Ease(s.Anchor(r), selectHoverSlot, pick(s.Hovered && !inert && isDark(t.Bg), 1.0, 0.0), s.motion); hover > 0 {
		fill = mix(fill, fade(t.InputBorder, .5), hover)
	}
	// The border keeps the InputBorder while the list is open or the field
	// was clicked: the Ring is the keyboard's, drawn by controlFocusRing
	// below, as shadcn's focus-visible:border-ring.
	border := t.InputBorder
	if s.shownInvalid {
		border = pick(isDark(t.Bg), fade(t.Destructive, .5), t.Destructive)
	}
	muted := t.MutedFg
	if inert {
		fill, border, muted = t.Disabled(fill), t.Disabled(border), t.Disabled(muted)
	}
	// In a ButtonGroup the field is joined to its neighbours as a button
	// is: square corners and no border of its own on the joined sides, the
	// group's radius, and its ring drawn by the group over them.
	sides := s.sides.within(dst)
	if sides.Any() {
		radius = t.Radius
		s.box.Fill(nil).Border(0, nil)
		paintJoinedBox(dst, r, radius, sides, fill, t.BorderWidth, border)
		ring := joinedFocus{on: true, r: r, radius: radius, sides: sides, width: t.BorderWidth}
		switch {
		case inert:
			ring.on = false
		case s.shownInvalid:
			ring.line, ring.halo = border, t.Halo(fieldRing(t, true))
		case s.Focused && s.FocusVisible:
			ring.line, ring.halo = t.Ring, t.Halo(t.Ring)
		default:
			ring.on = false
		}
		if g, ok := dst.Enclosing().(*ButtonGroupWidget); ok && ring.on {
			g.ring(ring)
		}
	} else {
		if s.shownInvalid && !inert {
			fieldHalo(dst, t, r, radius, fieldRing(t, true))
		}
		s.box.Fill(fill).Border(t.BorderWidth, border)
	}
	dst.Paint(s.box, r)
	if s.triggerContent != nil {
		inner := ggui.Rct(r.Origin.Add(ggui.Pt(s.pad.Left, s.pad.Top)), ggui.Sz(max(0, r.Size.W-s.pad.Horizontal()), max(0, r.Size.H-s.pad.Vertical())))
		at := ggui.Rct(inner.Origin.Add(ggui.Pt(0, (inner.Size.H-s.triggerSize.H)/2)), s.triggerSize)
		dst.Clip(inner).Inert().Paint(s.triggerContent, at)
	} else {
		dst.Clip(r).Paint(s.text, ggui.Rct(ggui.Pt(r.Origin.X+s.pad.Left, r.Origin.Y+(r.Size.H-s.textSize.H)/2), s.textSize))
		// The chevron is muted and points down whether or not the list is open.
		center := ggui.Pt(r.Origin.X+r.Size.W-s.pad.Right-t.IconSize/2, r.Origin.Y+r.Size.H/2)
		chevron(dst, s.env, t, center, 2, muted)
	}
	if !s.shownInvalid && !sides.Any() {
		controlFocusRing(dst, &s.Interactive, t, r, radius, t.Ring)
	}
}

// positionList uses Popup's point placement, retaining its focus scope and
// dismissal behavior. Measuring selectViewport does not reset its scroll state.
func (s *SelectWidget[T]) positionList(dst *ggui.Canvas, r ggui.Rect) {
	open := s.popup.IsOpen()
	opening := open && !s.wasOpen
	s.wasOpen = open
	if !open {
		return
	}
	index := s.index()
	screen := dst.Size()
	if !s.alignItem || index < 0 || screen.H <= 0 {
		s.list.MaxHeight(0)
		s.popup.AnchorPosition()
		s.popup.Gap(4)
		return
	}
	// Keep placement stable while the user highlights or scrolls other rows.
	if !opening && r == s.placedAnchor && screen == s.placedScreen {
		return
	}
	s.placedAnchor, s.placedScreen = r, screen
	s.list.MaxHeight(0)
	natural := s.list.Layout(ggui.Loose(ggui.Sz(screen.W, ggui.Unbounded)), uitheme.MenuEnv(s.env))
	top, rowHeight := s.rowSpan(index)
	center := s.theme.PanelPad.Top + top + rowHeight/2
	wantedY := r.Center().Y - center
	height := min(natural.H, screen.H)
	y := clamp(wantedY, 0, max(0, screen.H-height))
	offset := clamp(y-wantedY, 0, max(0, natural.H-height))
	if natural.H > screen.H {
		// For a scrolling list, reserve only the room needed below the
		// selected row. This permits exact alignment even near the last row.
		y = max(0, wantedY)
		offset = max(0, -wantedY)
		height = max(0, min(natural.H-offset, screen.H-y))
	}
	s.alignedHeight = height
	s.list.MaxHeight(height)
	s.viewport.offset.Set(offset)
	s.viewport.reveal = true
	// Align label starts as well where the window has enough horizontal room.
	x := r.Origin.X + s.pad.Left - s.theme.PanelPad.Left - s.theme.ItemPad.Left
	s.popup.Gap(0).ShowAt(ggui.Pt(x, y))
}

// Paint implements Widget.
func (s *SelectWidget[T]) Paint(dst *ggui.Canvas, r ggui.Rect) {
	for i, it := range s.items {
		it.active = i == s.highlight
	}
	dst.Paint(s.popup, r)
}

func (s *SelectWidget[T]) index() int {
	v := ggui.Untrack(s.value.Get)
	for i, o := range s.options {
		if o == v {
			return i
		}
	}
	return -1
}

func (s *SelectWidget[T]) choose(i int) {
	if i < 0 || i >= len(s.options) || !s.enabled(i) {
		return
	}
	setChanged(s.value, s.options[i], s.onChange)
	s.popup.Hide()
}

func (s *SelectWidget[T]) toggle() {
	if s.popup.IsOpen() {
		s.popup.Hide()
		return
	}
	s.highlight = s.index()
	s.viewport.reveal = true
	s.show()
}

// HandleKey implements KeyHandler.
func (s *SelectWidget[T]) HandleKey(ev ggui.KeyEvent) {
	if s.IsInert() {
		return
	}
	s.Keyboard(ev, nil)
	if ev.Kind == ggui.KeyBlur {
		s.popup.Hide()
	}
	if ev.Kind == ggui.KeyText {
		s.typeahead(ev.Text)
		return
	}
	if ev.Kind != ggui.KeyPress {
		return
	}
	open := s.popup.IsOpen()
	if ggui.Activates(ev) {
		if open && s.highlight >= 0 {
			s.choose(s.highlight)
		} else {
			s.toggle()
		}
		return
	}
	switch ev.Key {
	case ggui.KeyEscape:
		s.popup.Hide()
	case ggui.KeyArrowUp, ggui.KeyArrowDown, ggui.KeyHome, ggui.KeyEnd:
		if len(s.options) == 0 {
			return
		}
		if open {
			s.highlight, _ = navIndex(ev, navVertical, s.highlight, len(s.options), s.enabled)
			s.viewport.reveal = true
		} else if ev.Key == ggui.KeyArrowUp || ev.Key == ggui.KeyArrowDown {
			// An arrow opens the list on the chosen option, as Base UI's
			// trigger does, or on the near end when nothing is chosen.
			s.toggle()
			if s.highlight < 0 {
				s.highlight, _ = navIndex(ev, navVertical, -1, len(s.options), s.enabled)
			}
		}
	}
}

// selectTypeaheadReset is how long after a letter the next one still
// continues the same search, as Base UI's typeahead has it.
const selectTypeaheadReset = 750 * time.Millisecond

// typeahead takes typed text as a search of the options' labels: while the
// list is open the match is highlighted, and while it is closed the match
// becomes the value. Letters typed in quick succession spell a prefix; one
// letter repeated steps through the options starting with it.
func (s *SelectWidget[T]) typeahead(text string) {
	now := ggui.Now()
	if now.Sub(s.typedAt) > selectTypeaheadReset {
		s.typed = ""
	}
	if s.typed == "" && strings.TrimSpace(text) == "" {
		return // Space opens the list; it starts no search
	}
	s.typed, s.typedAt = s.typed+strings.ToLower(text), now
	open := s.popup.IsOpen()
	from := pick(open, s.highlight, s.index())
	query, step := s.typed, 0
	if runes := []rune(query); len(runes) > 1 && strings.Count(query, string(runes[0])) == len(runes) {
		query, step = string(runes[0]), 1 // the same letter again: the next option under it
	} else if len(runes) == 1 {
		step = 1
	}
	n, start := len(s.options), max(from+step, 0)
	for k := range n {
		i := (start + k) % n
		if !s.enabled(i) || !strings.HasPrefix(strings.ToLower(s.label(s.options[i])), query) {
			continue
		}
		if open {
			s.highlight, s.viewport.reveal = i, true
		} else if i != s.index() {
			setChanged(s.value, s.options[i], s.onChange)
		}
		return
	}
}

// Adopt implements ggui.Adopter: an open list carries across a rebuild.
func (s *SelectWidget[T]) Adopt(prev any) {
	s.Interactive.Adopt(prev)
	if p, ok := prev.(*SelectWidget[T]); ok {
		s.highlight = p.highlight
		s.typed, s.typedAt = p.typed, p.typedAt
		s.viewport.offset.Set(ggui.Untrack(p.viewport.offset.Get))
		s.viewport.reveal = p.viewport.reveal
		s.wasOpen = p.wasOpen
		s.alignedHeight = p.alignedHeight
		s.placedAnchor, s.placedScreen = p.placedAnchor, p.placedScreen
		s.list.MaxHeight(s.alignedHeight)
		if p.popup.IsOpen() {
			s.popup.Show()
		}
	}
}

// HandlePointer implements PointerHandler.
func (s *SelectWidget[T]) HandlePointer(ev ggui.PointerEvent) bool {
	if s.IsInert() {
		return false
	}
	return s.Pointer(ev, s.toggle)
}

// selectAnchor is the popup's anchor: the field, painted by its Select.
type selectAnchor[T comparable] struct{ s *SelectWidget[T] }

func (a selectAnchor[T]) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	s := a.s
	size := s.textSize
	if s.triggerContent != nil {
		size = s.triggerSize
	}
	s.box.Size(c.MaxW, s.pad.Inflate(size).H)
	return s.box.Layout(c, env)
}

func (a selectAnchor[T]) Paint(dst *ggui.Canvas, r ggui.Rect) { a.s.paintField(dst, r) }

// selectItem is one option row in the open list.
type selectItem[T comparable] struct {
	ggui.Interactive
	owner  *SelectWidget[T]
	index  int
	text   *ggui.TextWidget
	active bool // highlighted from the keyboard

	pad      ggui.EdgeInsets
	textSize ggui.Size
	theme    *uitheme.Theme // the list's, which may be the menus' own
}

func (it *selectItem[T]) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	it.Sync()
	t := uitheme.Of(env)
	it.theme = t
	// The label starts at the row's padding, and the room at the right is
	// the check mark's.
	it.pad = t.ItemPad
	it.pad.Right = menuMarkRoom(t)
	it.text.Color(pick(it.IsInert(), t.Disabled(t.PopoverFg), t.PopoverFg)) // Paint swaps in AccentFg under the highlight
	it.textSize = it.text.Layout(it.pad.Shrink(c).Loosen(), env)
	return c.Constrain(it.pad.Inflate(it.textSize))
}

// Describe implements ggui.Describer: whether this option is the value.
func (it *selectItem[T]) Describe() ggui.Node {
	return ggui.Node{
		Role:     ggui.RoleOption,
		Name:     it.SemanticName(),
		Selected: it.index == it.owner.index(),
		Disabled: it.IsInert(),
		Actions:  ggui.ActionSelect | ggui.ActionPress | ggui.ActionFocus,
	}
}

func (it *selectItem[T]) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t := it.theme
	dst.Describe(r, it)
	ink := t.Disabled(t.PopoverFg)
	if !it.IsInert() {
		dst.HitPointer(r, it)
		dst.HitCursor(r, ggui.CursorShapeDefault)
		lit := it.Hovered || it.active
		if lit {
			dst.FillRoundRect(r, t.RadiusSm, t.Accent)
		}
		ink = pick(lit, t.AccentFg, t.PopoverFg)
	}
	it.text.Color(ink)
	at := ggui.Pt(r.Origin.X+it.pad.Left, r.Origin.Y+(r.Size.H-it.textSize.H)/2)
	dst.Clip(r).Paint(it.text, ggui.Rct(at, it.textSize))
	if it.index == it.owner.index() {
		// The mark is the row's own color, not the theme's accent.
		center := ggui.Pt(r.Origin.X+r.Size.W-t.Space-t.IconSize/2, r.Origin.Y+r.Size.H/2)
		paintIconAt(dst, it.owner.env, icons.Check, center, t.IconSize, ink, 0)
	}
}

// Act implements ggui.Actor: choosing this option.
func (it *selectItem[T]) Act(a ggui.Action) bool {
	if it.IsInert() || (a.Kind != ggui.ActionSelect && a.Kind != ggui.ActionPress) {
		return false
	}
	it.owner.choose(it.index)
	return true
}

func (it *selectItem[T]) HandlePointer(ev ggui.PointerEvent) bool {
	if it.IsInert() {
		return ev.Kind != ggui.PointerScroll
	}
	if ev.Kind == ggui.PointerEnter || ev.Kind == ggui.PointerMove {
		it.owner.highlight = it.index
	}
	return it.Pointer(ev, func() { it.owner.choose(it.index) })
}

// selectViewport measures the full list without changing its scroll position.
// Popup first measures with unlimited height to choose above/below placement;
// only the final painted viewport may clamp the scroll offset.
type selectViewport[T comparable] struct {
	owner    *SelectWidget[T]
	column   *menuColumn
	scroll   *ggui.ScrollWidget
	offset   *ggui.StateValue[float64]
	reveal   bool
	env      ggui.Env
	natural  float64 // the height of the whole list
	up, down selectScrollArrow[T]
}

func (v *selectViewport[T]) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	v.env = env
	natural := v.column.Layout(ggui.Constraints{MinW: c.MinW, MaxW: c.MaxW, MaxH: ggui.Unbounded}, env)
	v.natural = natural.H
	return c.Constrain(natural)
}

func (v *selectViewport[T]) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t := uitheme.Of(v.env)
	// What a scroll arrow covers of the viewport: its strip starts at the
	// panel's edge, in the padding around the list.
	coverTop, coverBottom := selectArrowHeight(t)-t.PanelPad.Top, selectArrowHeight(t)-t.PanelPad.Bottom
	limit := max(0, v.natural-r.Size.H)
	offset := scrollEdge(ggui.Untrack(v.offset.Get), limit)
	v.up.view, v.up.up, v.down.view = v, true, v
	for _, a := range []*selectScrollArrow[T]{&v.up, &v.down} {
		if a.due(offset, limit) {
			offset = a.step(offset, limit, r.Size.H, pick(a.up, coverTop, coverBottom))
			v.offset.Set(offset)
		}
	}
	if v.reveal {
		v.reveal = false
		if i := v.owner.highlight; i >= 0 && i < len(v.owner.items) {
			// The row is shown clear of the arrow that stands over the
			// list's edge while there is more that way.
			top, height := v.owner.rowSpan(i)
			if top < offset+coverTop {
				offset = scrollEdge(top-coverTop, limit)
				v.offset.Set(offset)
			} else if top+height > offset+r.Size.H-coverBottom {
				offset = scrollEdge(top+height-r.Size.H+coverBottom, limit)
				v.offset.Set(offset)
			}
		}
	}
	v.scroll.Layout(ggui.Constraints{MinW: r.Size.W, MaxW: r.Size.W, MinH: r.Size.H, MaxH: r.Size.H}, v.env)
	dst.Paint(v.scroll, r)
	v.up.paint(dst, t, r, offset > 0)
	v.down.paint(dst, t, r, offset < limit)
}

// selectArrowHeight is the height of a scroll arrow: shadcn's py-1 around
// a size-4 icon.
func selectArrowHeight(t *uitheme.Theme) float64 { return t.IconSize + t.Space }

// selectArrowPace is how often a list scrolls a row while the pointer is on
// one of its arrows, as Base UI's SelectScrollArrow has it.
const selectArrowPace = 40 * time.Millisecond

// scrollEdge is offset kept within a list that scrolls as far as limit, and
// brought to an end it is within a pixel of: Base UI's
// normalizeScrollOffset, which is what decides whether an arrow shows.
func scrollEdge(offset, limit float64) float64 {
	const tolerance = 1
	limit = max(limit, 0)
	offset = clamp(offset, 0, limit)
	switch {
	case offset <= tolerance && limit-offset <= tolerance:
		return pick(offset <= limit-offset, 0, limit)
	case offset <= tolerance:
		return 0
	case limit-offset <= tolerance:
		return limit
	}
	return offset
}

// selectScrollArrow is the chevron over the top or bottom edge of a list
// with more options that way: shadcn's SelectScrollUpButton and
// SelectScrollDownButton. The list scrolls a row at a time while the
// pointer is moved onto one and stays there.
type selectScrollArrow[T comparable] struct {
	view  *selectViewport[T]
	up    bool
	at    ggui.Point // where the pointer was last seen on it
	seen  bool
	shown bool      // painted last: there is more that way
	next  time.Time // when the list scrolls next, zero while it is not scrolling
}

// due reports whether the pointer has rested on the arrow long enough for
// the next row, and asks for the frame that is.
func (a *selectScrollArrow[T]) due(offset, limit float64) bool {
	if a.next.IsZero() {
		return false
	}
	if (a.up && offset <= 0) || (!a.up && offset >= limit) {
		a.next = time.Time{}
		return false
	}
	now := ggui.FrameTime()
	due := !now.Before(a.next)
	if due {
		a.next = now.Add(selectArrowPace)
	}
	ggui.WakeAt(a.next)
	return due
}

// step is the offset one row on from offset in the arrow's direction, in a
// viewport height tall whose arrow covers cover of it: the row before the
// first one wholly shown comes out from under the up arrow, and the row
// after the last one from under the down arrow.
func (a *selectScrollArrow[T]) step(offset, limit, height, cover float64) float64 {
	const tolerance = 1
	s := a.view.owner
	n := len(s.items)
	if n == 0 {
		return pick(a.up, 0, limit)
	}
	if a.up {
		first := 0
		for i := range n {
			if top, _ := s.rowSpan(i); top >= offset+cover-tolerance {
				first = i
				break
			}
		}
		if first == 0 {
			return 0
		}
		top, _ := s.rowSpan(first - 1)
		return scrollEdge(top-cover, limit)
	}
	last := n - 1
	for i := range n {
		if top, h := s.rowSpan(i); top+h > offset+height-cover+tolerance {
			last = max(0, i-1)
			break
		}
	}
	if last >= n-1 {
		return limit
	}
	top, h := s.rowSpan(last + 1)
	return scrollEdge(top+h-height+cover, limit)
}

// paint draws the arrow over its edge of the viewport r and takes the
// pointer there, while shown.
func (a *selectScrollArrow[T]) paint(dst *ggui.Canvas, t *uitheme.Theme, r ggui.Rect, shown bool) {
	if a.shown = shown; !shown {
		a.seen, a.next = false, time.Time{}
		return
	}
	// The strip runs from the panel's edge, inside its border, over the
	// padding and the first or last of the list.
	pad, line, h := t.PanelPad, t.BorderWidth, selectArrowHeight(t)
	panel := ggui.Rct(ggui.Pt(r.Origin.X-pad.Left+line, r.Origin.Y-pad.Top+line), ggui.Sz(r.Size.W+pad.Horizontal()-2*line, r.Size.H+pad.Vertical()-2*line))
	strip := ggui.Rct(panel.Origin, ggui.Sz(panel.Size.W, h-line))
	if !a.up {
		strip.Origin.Y = panel.Origin.Y + panel.Size.H - strip.Size.H
	}
	dst.Clip(strip).FillRoundRect(panel, max(t.Radius-line, 0), t.Popover)
	dst.HitPointer(strip, a)
	dst.HitCursor(strip, ggui.CursorShapeDefault)
	center := ggui.Pt(strip.Origin.X+strip.Size.W/2, pick(a.up, panel.Origin.Y-line+h/2, panel.Origin.Y+panel.Size.H+line-h/2))
	paintIconAt(dst, a.view.env, pick(a.up, icons.ChevronUp, icons.ChevronDown), center, t.IconSize, t.PopoverFg, 0)
}

// HandlePointer implements ggui.PointerHandler: the pointer moving on the
// arrow starts the scroll and leaving it stops it. An arrow that appears
// under a pointer at rest does nothing until it moves, so a list opened
// over its trigger does not run off.
func (a *selectScrollArrow[T]) HandlePointer(ev ggui.PointerEvent) bool {
	switch ev.Kind {
	case ggui.PointerEnter:
		a.at, a.seen = ev.Pos, true
	case ggui.PointerMove:
		if a.seen && ev.Pos != a.at {
			a.view.owner.highlight = -1
			if a.next.IsZero() {
				a.next = ggui.FrameTime().Add(selectArrowPace)
				ggui.WakeAt(a.next)
			}
		}
		a.at, a.seen = ev.Pos, true
	case ggui.PointerExit:
		a.seen, a.next = false, time.Time{}
	case ggui.PointerScroll:
		return false
	}
	return true
}
