package ggui

import (
	"math"
	"slices"

	"github.com/hajimehoshi/ebiten/v2"
)

type inspectVisibility struct {
	depth         int
	folded, match bool
}

func inspectRowRange(count int, scroll, height float64) (int, int) {
	first := clamp(int(math.Floor(scroll/inspectRowHeight)), 0, count)
	end := clamp(int(math.Ceil((scroll+max(height, 0))/inspectRowHeight)), first, count)
	return first, end
}

type inspectPanelRow struct {
	key              inspectKey
	label, kind      string
	index            int
	folded, children bool
}

type inspectPanelState struct {
	panel                                                                          Rect
	scale, split, scroll, detailScroll, layoutScroll                               float64
	pointer                                                                        Point
	hasPointer, dark, pinned, picking, copied, filterFocus, selectFilter, outlines bool
	filter                                                                         string
	dock                                                                           InspectorDock
	tab                                                                            inspectAction
	selected, total, shown, matches                                                int
	box                                                                            inspectBox
}

// Copy displayed values separately from widget identities: a widget can
// mutate in place without changing identity or the layout generation.
type inspectPanelSnapshot struct {
	state        inspectPanelState
	rows, crumbs []inspectPanelRow
	fields       [3][]inspectField
}

func (a inspectPanelSnapshot) equal(b inspectPanelSnapshot) bool {
	if a.state != b.state || !slices.Equal(a.rows, b.rows) || !slices.Equal(a.crumbs, b.crumbs) {
		return false
	}
	for i := range a.fields {
		if !slices.Equal(a.fields[i], b.fields[i]) {
			return false
		}
	}
	return true
}

func (in *inspector) panelSnapshot(dst *Canvas, shown []int, sel int) inspectPanelSnapshot {
	pointer, hasPointer := dst.Pointer()
	if !hasPointer || !in.panel.Contains(pointer) {
		pointer, hasPointer = Point{}, false
	}
	s := inspectPanelSnapshot{state: inspectPanelState{
		panel: in.panel, scale: dst.Scale(), split: in.split, scroll: in.scroll, detailScroll: in.detailScroll, layoutScroll: in.layoutScroll,
		pointer: pointer, hasPointer: hasPointer, dark: luminance(Untrack(theme.Get).Bg) < .5,
		pinned: in.pinned, picking: in.picking, copied: in.copied, filterFocus: in.filterFocus, selectFilter: in.selectFilter, outlines: in.outlines,
		filter: in.filter, dock: in.dock, tab: in.tab, selected: sel, total: len(dst.trace), shown: len(shown), matches: in.matches,
	}}
	row := func(i int) inspectPanelRow {
		e := &dst.trace[i]
		return inspectPanelRow{key: keyOf(e), label: inspectLabel(e), kind: inspectKind(e), index: i, folded: in.folded(e), children: hasChildren(dst.trace, i)}
	}
	first, end := inspectRowRange(len(shown), in.scroll, in.tree.Size.H)
	for _, i := range shown[first:end] {
		s.rows = append(s.rows, row(i))
	}
	if sel >= 0 {
		s.state.box = inspectedBox(dst.trace, sel)
		for _, i := range ancestors(dst.trace, sel) {
			s.crumbs = append(s.crumbs, row(i))
		}
		s.crumbs = append(s.crumbs, row(sel))
		for i, tab := range []inspectAction{inspectTabLayout, inspectTabComputed, inspectTabSemantics} {
			view := inspector{tab: tab}
			s.fields[i] = view.details(dst, sel)
		}
	}
	return s
}

type inspectorPanelCache struct {
	image    *ebiten.Image
	snapshot inspectPanelSnapshot
}

func (c *inspectorPanelCache) release() {
	if c.image != nil {
		c.image.Deallocate()
	}
	*c = inspectorPanelCache{}
}
func (c *inspectorPanelCache) draw(dst *Canvas) {
	op := &ebiten.DrawImageOptions{}
	b := c.image.Bounds()
	op.GeoM.Translate(float64(b.Min.X), float64(b.Min.Y))
	dst.Image.DrawImage(c.image, op)
}
