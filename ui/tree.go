package ui

import (
	"maps"
	"math"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/ui/icons"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// TreeWidget is a hierarchy of keyed rows, each indented by its depth with a
// disclosure chevron when it has children. It follows the WAI-ARIA tree
// pattern: one row is the Tab stop and the arrow keys move between, open and
// close the rest. Build one with Tree.
type TreeWidget[T any, K comparable] struct {
	props         property.Owner
	key           func(T) K
	children      func(T) []T
	expanded      *ggui.StateValue[ggui.Binding[map[K]bool]]
	selected      ggui.Binding[K]
	localSelected *ggui.StateValue[K]
	onSelect      func(T)
	label         func(T) string
	name          string
	rowH          float64
	indent        float64
	height        float64

	flat    *ggui.DerivedValue[[]treeEntry[T, K]]
	body    *ggui.EachWidget[treeEntry[T, K], K]
	scroll  *ggui.ScrollWidget
	offset  *ggui.StateValue[float64]
	content ggui.Widget
	viewH   float64
	nav     rovingRows[treeEntry[T, K], K]
}

// treeEntry is one visible row of the flattened tree.
type treeEntry[T any, K comparable] struct {
	item        T
	key         K
	depth       int
	parent      int // the parent's index in the flattened tree, or -1
	hasChildren bool
	open        bool
}

// Tree creates a tree over roots: key identifies every item, unique across
// the whole tree, so a row's widgets survive expand, collapse and edits;
// children lists an item's children; label builds a row's content from its
// item as a Readable that follows the tree.
//
//	ui.Tree(roots, func(f File) string { return f.Path },
//		func(f File) []File { return f.Children },
//		func(f ggui.Readable[File]) ggui.Widget {
//			return ggui.TextOf(ggui.Map(f, func(f File) string { return f.Name }))
//		},
//	).RowName(func(f File) string { return f.Name }).BindSelected(chosen).Height(320)
func Tree[T any, K comparable](roots ggui.Readable[[]T], key func(T) K, children func(T) []T, label func(ggui.Readable[T]) ggui.Widget) *TreeWidget[T, K] {
	t := &TreeWidget[T, K]{key: key, children: children, rowH: 32, indent: 16}
	t.label = func(item T) string { return sprint(key(item)) }
	t.expanded = ggui.State[ggui.Binding[map[K]bool]](ggui.State(map[K]bool{}))
	t.flat = ggui.Derived(func() []treeEntry[T, K] { return t.flatten(roots.Get(), t.expanded.Get().Get()) })
	t.body = ggui.EachKeyed(t.flat, func(e treeEntry[T, K]) K { return e.key }, func(row ggui.EachItem[treeEntry[T, K]]) ggui.Widget {
		return t.row(row, label)
	}).Gap(0).Align(ggui.AlignStretch).Unlisted().ItemExtent(t.rowH)
	t.offset = ggui.State(0.0)
	t.scroll = ggui.Scroll(t.body).BindOffset(t.offset)
	t.content = t.body
	return t
}

// flatten lists the visible items depth first: every root, and the
// children of every open item under it.
func (t *TreeWidget[T, K]) flatten(roots []T, open map[K]bool) []treeEntry[T, K] {
	var out []treeEntry[T, K]
	var walk func(items []T, depth, parent int)
	walk = func(items []T, depth, parent int) {
		for _, item := range items {
			kids := t.children(item)
			k := t.key(item)
			e := treeEntry[T, K]{item: item, key: k, depth: depth, parent: parent, hasChildren: len(kids) > 0}
			e.open = e.hasChildren && open[k]
			out = append(out, e)
			if e.open {
				walk(kids, depth+1, len(out)-1)
			}
		}
	}
	walk(roots, 0, -1)
	return out
}

// BindExpanded binds the set of open items' keys: the chevrons, arrow keys
// and accessibility actions write it. Without it the tree keeps its own,
// starting fully collapsed.
func (t *TreeWidget[T, K]) BindExpanded(b ggui.Binding[map[K]bool]) *TreeWidget[T, K] {
	property.Require(b, "BindExpanded")
	if !property.Same(ggui.Untrack(t.expanded.Get), b) {
		t.expanded.Set(b)
	}
	return t
}

// Expanded detaches an expanded-set binding and opens the keys in open,
// which the tree then keeps as its own state.
func (t *TreeWidget[T, K]) Expanded(open map[K]bool) *TreeWidget[T, K] {
	return t.BindExpanded(ggui.State(maps.Clone(open)))
}

// BindSelected binds the key of the selected row: a click, Space or Enter
// sets it, and so does moving with the arrow keys.
func (t *TreeWidget[T, K]) BindSelected(b ggui.Binding[K]) *TreeWidget[T, K] {
	property.Require(b, "BindSelected")
	if !property.Same(t.selected, b) {
		t.selected = b
		t.props.Changed()
	}
	return t
}

// Selected detaches a selection binding and selects a local row key.
func (t *TreeWidget[T, K]) Selected(v K) *TreeWidget[T, K] {
	if t.localSelected == nil {
		t.localSelected = ggui.State(v)
	} else {
		t.localSelected.Set(v)
	}
	return t.BindSelected(t.localSelected)
}

// OnSelect fires with the item whose row was clicked or activated with
// Space or Enter. Arrow-key moves select without firing it.
func (t *TreeWidget[T, K]) OnSelect(fn func(T)) *TreeWidget[T, K] { t.onSelect = fn; return t }

// RowName names each row for Probe.Find, the inspector and screen readers;
// the default is the key.
func (t *TreeWidget[T, K]) RowName(fn func(T) string) *TreeWidget[T, K] { t.label = fn; return t }

// Name sets the tree's accessible name.
func (t *TreeWidget[T, K]) Name(name string) *TreeWidget[T, K] {
	defer property.Watch(&t.props, &t.name)()
	t.name = name
	return t
}

// RowHeight fixes every row's height; the default is 32. With Height the
// tree then lays out only the rows in view.
func (t *TreeWidget[T, K]) RowHeight(h float64) *TreeWidget[T, K] {
	defer property.Watch(&t.props, &t.rowH)()
	t.rowH = max(1, h)
	t.body.ItemExtent(t.rowH)
	return t
}

// Indent sets how far each level is inset from its parent; the default is 16.
func (t *TreeWidget[T, K]) Indent(px float64) *TreeWidget[T, K] {
	defer property.Watch(&t.props, &t.indent)()
	t.indent = max(0, px)
	return t
}

// Height bounds the tree and scrolls its rows. Without it the tree is as
// tall as its visible rows.
func (t *TreeWidget[T, K]) Height(h float64) *TreeWidget[T, K] {
	defer property.Watch(&t.props, &t.height)()
	t.height = h
	t.content = pick[ggui.Widget](h > 0, t.scroll, t.body)
	return t
}

func (t *TreeWidget[T, K]) row(row ggui.EachItem[treeEntry[T, K]], label func(ggui.Readable[T]) ggui.Widget) ggui.Widget {
	item := ggui.Map(row.Value, func(e treeEntry[T, K]) T { return e.item })
	e := row.Value.Get()
	r := &treeRow[T, K]{tree: t, entry: row.Value, index: row.Index, key: e.key, label: ggui.Align(label(item)).At(0, .5)}
	r.toggle.row = r
	r.box = ggui.Box(r.label)
	r.Role = ggui.RoleTreeItem
	r.SetName(t.label(e.item))
	r.AutoKey()
	return r
}

// Layout implements Widget.
func (t *TreeWidget[T, K]) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer t.props.Layout()()
	if t.height > 0 {
		h := min(t.height, c.MaxH)
		c.MinH, c.MaxH = h, h
	}
	sz := t.content.Layout(c, env)
	t.viewH = sz.H
	return sz
}

// Paint implements Widget.
func (t *TreeWidget[T, K]) Paint(dst *ggui.Canvas, r ggui.Rect) {
	dst.Node(r, ggui.Node{Role: ggui.RoleTree, Name: t.name}, func(dst *ggui.Canvas) { dst.Paint(t.content, r) })
}

// setOpen opens or closes the item keyed k.
func (t *TreeWidget[T, K]) setOpen(k K, open bool) {
	b := ggui.Untrack(t.expanded.Get)
	cur := ggui.Untrack(b.Get)
	if cur[k] == open {
		return
	}
	b.Set(withKey(cur, k, open))
}

// moveTo focuses row i, selects it when selection is bound and scrolls it
// into view.
func (t *TreeWidget[T, K]) moveTo(list []treeEntry[T, K], i int) {
	if i < 0 || i >= len(list) {
		return
	}
	k := list[i].key
	t.nav.ask(k)
	if t.selected != nil {
		t.selected.Set(k)
	}
	if t.height > 0 {
		revealRow(t.offset, i, t.rowH, t.viewH)
	}
	t.props.Changed()
}

func (t *TreeWidget[T, K]) activate(item T, k K) {
	if t.selected != nil {
		t.selected.Set(k)
	}
	if t.onSelect != nil {
		t.onSelect(item)
	}
}

// treeRow is one visible item: indent, chevron and label.
type treeRow[T any, K comparable] struct {
	ggui.Interactive
	tree   *TreeWidget[T, K]
	entry  ggui.Readable[treeEntry[T, K]]
	index  ggui.Readable[int]
	key    K
	label  ggui.Widget
	box    *ggui.BoxWidget
	toggle treeToggle[T, K]
	env    ggui.Env
	theme  uitheme.Theme
	motion time.Duration
}

// treeToggle is the chevron's click target.
type treeToggle[T any, K comparable] struct{ row *treeRow[T, K] }

// HandlePointer implements PointerHandler: a click opens or closes the row.
func (g *treeToggle[T, K]) HandlePointer(ev ggui.PointerEvent) bool {
	if ev.Kind == ggui.PointerTap && ev.Button == ggui.MouseButtonLeft {
		g.row.setOpen(!ggui.Untrack(g.row.entry.Get).open)
	}
	return ev.Kind != ggui.PointerScroll
}

// chevronX is where the row's chevron starts, and inset where its label
// does.
func (r *treeRow[T, K]) chevronX(e treeEntry[T, K]) float64 {
	return r.theme.Space + float64(e.depth)*r.tree.indent
}
func (r *treeRow[T, K]) inset(e treeEntry[T, K]) float64 {
	return r.chevronX(e) + r.theme.IconSize + r.theme.Space/2
}

func (r *treeRow[T, K]) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	r.env, r.theme = env, uitheme.From(env)
	r.Sync()
	e := r.entry.Get()
	r.SetName(r.tree.label(e.item))
	r.motion = env.Motion(r.theme.MotionFast)
	r.box.Padding(ggui.EdgeInsets{Left: r.inset(e), Right: 8}).Height(r.tree.rowH)
	return r.box.Layout(c, env)
}

func (r *treeRow[T, K]) chosen() bool {
	return r.tree.selected != nil && r.tree.selected.Get() == r.key
}

// Describe implements ggui.Describer: a tree item reports its depth, whether
// it is open when it has children, and whether it is selected.
func (r *treeRow[T, K]) Describe() ggui.Node {
	e := ggui.Untrack(r.entry.Get)
	n := ggui.Node{Role: ggui.RoleTreeItem, Name: r.SemanticName(), Level: e.depth + 1, Selected: r.chosen(), Disabled: r.IsInert(),
		Actions: ggui.ActionSelect | ggui.ActionPress | ggui.ActionFocus}
	if e.hasChildren {
		n.Expanded = ggui.Expandable(e.open)
		n.Actions |= pick(e.open, ggui.ActionCollapse, ggui.ActionExpand)
	}
	return n
}

func (r *treeRow[T, K]) Paint(dst *ggui.Canvas, rc ggui.Rect) {
	dst.DescribeNode(rc, r, func(dst *ggui.Canvas) { r.paint(dst, rc) })
}

func (r *treeRow[T, K]) paint(dst *ggui.Canvas, rc ggui.Rect) {
	th, t := r.theme, r.tree
	e := ggui.Untrack(r.entry.Get)
	r.Hit(dst, rc, r, ggui.CursorShapePointer)
	target := 0.0
	if r.chosen() {
		target = 1
	} else if r.Hovered && !r.IsInert() {
		target = .5
	}
	if amount := dst.Ease(r.Anchor(rc), treeRowFillSlot, target, r.motion); amount > 0 {
		dst.FillRoundRect(rc, th.Radius, fade(th.Muted, amount))
	}
	if e.hasChildren {
		x := rc.Origin.X + r.chevronX(e)
		turn := dst.Ease(r.Anchor(rc), treeChevronSlot, pick(e.open, 1.0, 0.0), r.motion)
		paintIconAt(dst, r.env, icons.ChevronRight, ggui.Pt(x+th.IconSize/2, rc.Center().Y), th.IconSize, th.MutedFg, turn*math.Pi/2)
		if !r.IsInert() {
			dst.HitPointer(ggui.Rct(ggui.Pt(x-th.Space/2, rc.Origin.Y), ggui.Sz(th.IconSize+th.Space, rc.Size.H)), &r.toggle)
		}
	}
	dst.Clip(rc).Paint(r.box, rc)
	r.FocusRing(dst, rc, th.Radius, th.Ring)
	if t.nav.claim(r.key) {
		dst.RequestFocus(r)
	}
}

func (r *treeRow[T, K]) setOpen(open bool) {
	if !r.IsInert() && ggui.Untrack(r.entry.Get).hasChildren {
		r.tree.setOpen(r.key, open)
	}
}

func (r *treeRow[T, K]) activate() {
	if !r.IsInert() {
		r.tree.activate(ggui.Untrack(r.entry.Get).item, r.key)
	}
}

// TabStop implements ggui.TabStopper: only one row of the tree is in the
// Tab order.
func (r *treeRow[T, K]) TabStop() bool {
	t := r.tree
	var sel rowCursor[K]
	if t.selected != nil {
		sel = at(ggui.Untrack(t.selected.Get))
	}
	return t.nav.isStop(r.key, ggui.Untrack(t.flat.Get), func(e treeEntry[T, K]) K { return e.key }, sel)
}

// Act implements ggui.Actor: expanding, collapsing and selecting the row.
func (r *treeRow[T, K]) Act(a ggui.Action) bool {
	if r.IsInert() {
		return false
	}
	switch a.Kind {
	case ggui.ActionExpand, ggui.ActionCollapse:
		if !ggui.Untrack(r.entry.Get).hasChildren {
			return false
		}
		r.setOpen(a.Kind == ggui.ActionExpand)
	case ggui.ActionSelect, ggui.ActionPress:
		r.activate()
	default:
		return false
	}
	return true
}

// ConsumesKey implements ggui.KeyConsumer: a row acts on Space, Enter,
// the arrows, Home and End.
func (r *treeRow[T, K]) ConsumesKey(ev ggui.KeyEvent) bool {
	if ggui.Activates(ev) {
		return true
	}
	if ev.Kind != ggui.KeyPress || ev.Mods != (ggui.Mods{}) {
		return false
	}
	switch ev.Key {
	case ggui.KeyArrowUp, ggui.KeyArrowDown, ggui.KeyArrowLeft, ggui.KeyArrowRight, ggui.KeyHome, ggui.KeyEnd:
		return true
	}
	return false
}

// HandleKey implements KeyHandler: the WAI-ARIA tree keys.
func (r *treeRow[T, K]) HandleKey(ev ggui.KeyEvent) {
	if r.IsInert() {
		return
	}
	t := r.tree
	if ev.Kind == ggui.KeyFocus {
		t.nav.active = at(r.key)
	}
	r.Keyboard(ev, r.activate)
	if !r.ConsumesKey(ev) || ggui.Activates(ev) {
		return
	}
	list, i := ggui.Untrack(t.flat.Get), ggui.Untrack(r.index.Get)
	if i < 0 || i >= len(list) {
		return
	}
	e := list[i]
	switch ev.Key {
	case ggui.KeyArrowUp:
		t.moveTo(list, i-1)
	case ggui.KeyArrowDown:
		t.moveTo(list, i+1)
	case ggui.KeyHome:
		t.moveTo(list, 0)
	case ggui.KeyEnd:
		t.moveTo(list, len(list)-1)
	case ggui.KeyArrowRight:
		if e.hasChildren && !e.open {
			r.setOpen(true)
		} else if e.open {
			t.moveTo(list, i+1)
		}
	case ggui.KeyArrowLeft:
		if e.open {
			r.setOpen(false)
		} else {
			t.moveTo(list, e.parent)
		}
	}
}

// HandlePointer implements PointerHandler: a click activates the row.
func (r *treeRow[T, K]) HandlePointer(ev ggui.PointerEvent) bool {
	if r.IsInert() {
		return false
	}
	return r.Pointer(ev, r.activate)
}

var (
	treeRowFillSlot = ggui.NewSlot[*ggui.Motion]("tree row fill")
	treeChevronSlot = ggui.NewSlot[*ggui.Motion]("tree chevron")
)
