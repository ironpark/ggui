package ui

import (
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// ListBoxWidget is a list of keyed rows of which one is selected, as in a
// file list or a master-detail sidebar. It follows the WAI-ARIA listbox
// pattern: the list is one Tab stop, on the selected row; Up and Down move
// the selection, Home and End go to the ends, and Space or Enter, like a
// click, activates the row. Build one with ListBox.
//
// A selection changed from elsewhere, such as a canvas the same items are
// drawn on, scrolls the enclosing Scroll to its row without moving focus.
type ListBoxWidget[T any, K comparable] struct {
	props         property.Owner
	items         ggui.Readable[[]T]
	key           func(T) K
	selected      ggui.Binding[K]
	localSelected *ggui.StateValue[K]
	onSelect      func(T)
	label         func(T) string
	name          string

	body *ggui.EachWidget[T, K]
	nav  rovingRows[T, K]

	shown  rowCursor[K] // the selection as of the last paint
	reveal bool         // the selected row is to be scrolled into view
}

// ListBox creates a list over items: key identifies every item, so a row's
// widgets survive edits and reordering, and row builds a row's content from
// its item as a Readable that follows the list.
//
//	ui.ListBox(files, func(f File) string { return f.Path },
//		func(f ggui.Readable[File]) ggui.Widget {
//			return ggui.TextOf(ggui.Map(f, func(f File) string { return f.Name }))
//		},
//	).RowName(func(f File) string { return f.Name }).BindSelected(chosen)
func ListBox[T any, K comparable](items ggui.Readable[[]T], key func(T) K, row func(ggui.Readable[T]) ggui.Widget) *ListBoxWidget[T, K] {
	l := &ListBoxWidget[T, K]{items: items, key: key}
	l.label = func(item T) string { return sprint(key(item)) }
	l.body = ggui.EachKeyed(items, key, func(it ggui.EachItem[T]) ggui.Widget { return l.row(it, row) }).
		Gap(2).Align(ggui.AlignStretch).Unlisted()
	return l
}

// BindSelected binds the key of the selected row: a click, the arrow keys,
// Space and Enter set it. A key no row has selects none.
func (l *ListBoxWidget[T, K]) BindSelected(b ggui.Binding[K]) *ListBoxWidget[T, K] {
	property.Require(b, "BindSelected")
	if !property.Same(l.selected, b) {
		l.selected = b
		l.props.Changed()
	}
	return l
}

// Selected detaches a selection binding and selects a local row key.
func (l *ListBoxWidget[T, K]) Selected(v K) *ListBoxWidget[T, K] {
	if l.localSelected == nil {
		l.localSelected = ggui.State(v)
	} else {
		l.localSelected.Set(v)
	}
	return l.BindSelected(l.localSelected)
}

// OnSelect fires with the item whose row was clicked or activated with
// Space or Enter. Arrow-key moves select without firing it.
func (l *ListBoxWidget[T, K]) OnSelect(fn func(T)) *ListBoxWidget[T, K] { l.onSelect = fn; return l }

// RowName names each row for Probe.Find, the inspector and screen readers;
// the default is the key.
func (l *ListBoxWidget[T, K]) RowName(fn func(T) string) *ListBoxWidget[T, K] { l.label = fn; return l }

// Name sets the list's accessible name.
func (l *ListBoxWidget[T, K]) Name(name string) *ListBoxWidget[T, K] {
	defer property.Watch(&l.props, &l.name)()
	l.name = name
	return l
}

// Gap sets the space between rows; the default is 2.
func (l *ListBoxWidget[T, K]) Gap(v float64) *ListBoxWidget[T, K] { l.body.Gap(v); return l }

// Else shows build's widget in place of the rows while there are none.
func (l *ListBoxWidget[T, K]) Else(build func() ggui.Widget) *ListBoxWidget[T, K] {
	l.body.Else(build)
	return l
}

// Layout implements Widget.
func (l *ListBoxWidget[T, K]) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer l.props.Layout()()
	return l.body.Layout(c, env)
}

// Paint implements Widget.
func (l *ListBoxWidget[T, K]) Paint(dst *ggui.Canvas, r ggui.Rect) {
	if sel, ok := l.current(); ok != l.shown.ok || sel != l.shown.key {
		l.shown, l.reveal = rowCursor[K]{sel, ok}, ok
	}
	dst.Node(r, ggui.Node{Role: ggui.RoleList, Name: l.name}, func(dst *ggui.Canvas) { dst.Paint(l.body, r) })
}

// current returns the selected key, if a selection is bound.
func (l *ListBoxWidget[T, K]) current() (K, bool) {
	if l.selected == nil {
		var zero K
		return zero, false
	}
	return ggui.Untrack(l.selected.Get), true
}

// moveTo selects row i and focuses it, which scrolls it into view.
func (l *ListBoxWidget[T, K]) moveTo(list []T, i int) {
	if i < 0 || i >= len(list) {
		return
	}
	k := l.key(list[i])
	l.nav.ask(k)
	if l.selected != nil {
		l.selected.Set(k)
	}
	l.props.Changed()
}

func (l *ListBoxWidget[T, K]) activate(item T, k K) {
	if l.selected != nil {
		l.selected.Set(k)
	}
	if l.onSelect != nil {
		l.onSelect(item)
	}
}

func (l *ListBoxWidget[T, K]) row(it ggui.EachItem[T], content func(ggui.Readable[T]) ggui.Widget) ggui.Widget {
	item := ggui.Untrack(it.Value.Get)
	r := &listRow[T, K]{list: l, item: it.Value, index: it.Index, key: l.key(item)}
	r.box = ggui.Box(content(it.Value))
	r.Role = ggui.RoleOption
	r.SetName(l.label(item))
	r.AutoKey()
	return r
}

// listRow is one row: its content over the selection fill.
type listRow[T any, K comparable] struct {
	ggui.Interactive
	list   *ListBoxWidget[T, K]
	item   ggui.Readable[T]
	index  ggui.Readable[int]
	key    K
	box    *ggui.BoxWidget
	theme  uitheme.Theme
	motion time.Duration
}

func (r *listRow[T, K]) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	r.theme = uitheme.From(env)
	r.Sync()
	r.SetName(r.list.label(r.item.Get()))
	r.motion = env.Motion(r.theme.MotionFast)
	r.box.Padding(r.theme.ItemPad)
	return r.box.Layout(c, env)
}

// Baseline implements Baseliner: the content's.
func (r *listRow[T, K]) Baseline() (float64, bool) { return r.box.Baseline() }

func (r *listRow[T, K]) chosen() bool {
	sel, ok := r.list.current()
	return ok && sel == r.key
}

// Describe implements ggui.Describer: an option says whether it is selected.
func (r *listRow[T, K]) Describe() ggui.Node {
	return ggui.Node{Role: ggui.RoleOption, Name: r.SemanticName(), Selected: r.chosen(), Disabled: r.IsInert(),
		Actions: ggui.ActionSelect | ggui.ActionPress | ggui.ActionFocus}
}

func (r *listRow[T, K]) Paint(dst *ggui.Canvas, rc ggui.Rect) {
	dst.DescribeNode(rc, r, func(dst *ggui.Canvas) { r.paint(dst, rc) })
}

func (r *listRow[T, K]) paint(dst *ggui.Canvas, rc ggui.Rect) {
	th, l := r.theme, r.list
	r.Hit(dst, rc, r, ggui.CursorShapePointer)
	chosen := r.chosen()
	target := 0.0
	if chosen {
		target = 1
	} else if r.Hovered && !r.IsInert() {
		target = .5
	}
	if amount := dst.Ease(r.Anchor(rc), listRowFillSlot, target, r.motion); amount > 0 {
		dst.FillRoundRect(rc, th.Radius, fade(th.Accent, amount))
	}
	dst.Clip(rc).Paint(r.box, rc)
	r.FocusRing(dst, rc, th.Radius, th.Ring)
	if l.nav.claim(r.key) {
		dst.RequestFocus(r)
	} else if chosen && l.reveal {
		dst.RequestReveal(rc)
	}
	if chosen {
		l.reveal = false
	}
}

func (r *listRow[T, K]) activate() {
	if !r.IsInert() {
		r.list.nav.active = at(r.key)
		r.list.activate(ggui.Untrack(r.item.Get), r.key)
	}
}

// TabStop implements ggui.TabStopper: only one row of the list is in the
// Tab order.
func (r *listRow[T, K]) TabStop() bool {
	l := r.list
	var sel rowCursor[K]
	if k, ok := l.current(); ok {
		sel = at(k)
	}
	return l.nav.isStop(r.key, ggui.Untrack(l.items.Get), l.key, sel)
}

// Act implements ggui.Actor: selecting the row.
func (r *listRow[T, K]) Act(a ggui.Action) bool {
	if r.IsInert() || (a.Kind != ggui.ActionSelect && a.Kind != ggui.ActionPress) {
		return false
	}
	r.activate()
	return true
}

// ConsumesKey implements ggui.KeyConsumer: a row acts on Space, Enter,
// Up, Down, Home and End.
func (r *listRow[T, K]) ConsumesKey(ev ggui.KeyEvent) bool {
	if ggui.Activates(ev) {
		return true
	}
	if ev.Kind != ggui.KeyPress || ev.Mods != (ggui.Mods{}) {
		return false
	}
	switch ev.Key {
	case ggui.KeyArrowUp, ggui.KeyArrowDown, ggui.KeyHome, ggui.KeyEnd:
		return true
	}
	return false
}

// HandleKey implements KeyHandler: the WAI-ARIA listbox keys.
func (r *listRow[T, K]) HandleKey(ev ggui.KeyEvent) {
	if r.IsInert() {
		return
	}
	l := r.list
	if ev.Kind == ggui.KeyFocus {
		l.nav.active = at(r.key)
	}
	r.Keyboard(ev, r.activate)
	if !r.ConsumesKey(ev) || ggui.Activates(ev) {
		return
	}
	list, i := ggui.Untrack(l.items.Get), ggui.Untrack(r.index.Get)
	switch ev.Key {
	case ggui.KeyArrowUp:
		l.moveTo(list, i-1)
	case ggui.KeyArrowDown:
		l.moveTo(list, i+1)
	case ggui.KeyHome:
		l.moveTo(list, 0)
	case ggui.KeyEnd:
		l.moveTo(list, len(list)-1)
	}
}

// HandlePointer implements PointerHandler: a click activates the row.
func (r *listRow[T, K]) HandlePointer(ev ggui.PointerEvent) bool {
	if r.IsInert() {
		return false
	}
	return r.Pointer(ev, r.activate)
}

var listRowFillSlot = ggui.NewSlot[*ggui.Motion]("list row fill")
