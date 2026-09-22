//go:build ggui_inspector

package panel

import (
	"math"
	"slices"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/inspect"
	"github.com/ironpark/ggui/internal/fn"
)

type inspectVisibility struct {
	depth         int
	folded, match bool
}

func inspectRowRange(count int, scroll, height float64) (int, int) {
	first := fn.Clamp(int(math.Floor(scroll/inspectRowHeight)), 0, count)
	end := fn.Clamp(int(math.Ceil((scroll+max(height, 0))/inspectRowHeight)), first, count)
	return first, end
}

type inspectPanelRow struct {
	key              inspectKey
	label, kind      string
	index            int
	folded, children bool
}

type inspectPanelState struct {
	panel, tree, layout                                                ggui.Rect
	scale, split, scroll, detailScroll, layoutScroll                   float64
	hoverRow, hoverChip                                                int // what the pointer is over, not where it is
	dark, pinned, picking, copied, filterFocus, selectFilter, outlines bool
	filter                                                             string
	dock                                                               ggui.InspectorDock
	tab                                                                inspectTab
	selected, total, shown, matches                                    int
	box                                                                inspect.Box
}

// Copy displayed values separately from widget identities: a widget can
// mutate in place without changing identity or the layout generation.
// Only the panes on screen are described: switching tabs changes state.tab,
// so hidden panes need no fields.
type inspectPanelSnapshot struct {
	state        inspectPanelState
	rows, crumbs []inspectPanelRow
	fields       [2][]inspect.Field // details pane, layout column
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

// panelSnapshot describes the panel as the last paintPanel laid it out,
// reusing buf's storage. Hover is keyed by the row and chip under the
// pointer, so moving within one row does not repaint the panel.
func (in *View) panelSnapshot(dst *ggui.Canvas, fr *inspect.Frame, shown []int, sel int, buf inspectPanelSnapshot) inspectPanelSnapshot {
	s := inspectPanelSnapshot{rows: buf.rows[:0], crumbs: buf.crumbs[:0]}
	s.state = inspectPanelState{
		panel: in.panel, tree: in.tree, layout: in.layout, scale: dst.Scale(), split: in.split, scroll: in.scroll, detailScroll: in.detailScroll, layoutScroll: in.layoutScroll,
		hoverRow: -1, hoverChip: -1, dark: luminance(ggui.Untrack(ggui.UseEnv).Background()) < .5,
		pinned: in.pinned, picking: in.picking, copied: in.copied, filterFocus: in.filterFocus, selectFilter: in.selectFilter, outlines: in.outlines,
		filter: in.filter, dock: in.dock, tab: in.tab, selected: sel, total: len(fr.Nodes), shown: len(shown), matches: in.matches,
	}
	if p, ok := dst.Pointer(); ok && in.panel.Contains(p) {
		s.state.hoverChip = slices.IndexFunc(in.chips, func(c inspectChip) bool { return c.rect.Contains(p) })
		if in.tree.Contains(p) {
			s.state.hoverRow = slices.IndexFunc(in.rows, func(r inspectRow) bool { return p.Y >= r.y && p.Y < r.y+r.h })
		}
	}
	row := func(i int) inspectPanelRow {
		e := fr.Describe(i)
		return inspectPanelRow{key: keyOf(e), label: e.Label, kind: fr.Badge(i), index: i, folded: in.folded(e), children: inspect.HasChildren(fr.Nodes, i)}
	}
	first, end := inspectRowRange(len(shown), in.scroll, in.tree.Size.H)
	for _, i := range shown[first:end] {
		s.rows = append(s.rows, row(i))
	}
	if sel >= 0 {
		s.state.box = fr.BoxOf(sel)
		for _, i := range inspect.Ancestors(fr.Nodes, sel) {
			s.crumbs = append(s.crumbs, row(i))
		}
		s.crumbs = append(s.crumbs, row(sel))
		s.fields[0] = fr.Details(in.detailTab(), sel)
		if !in.layout.Empty() {
			s.fields[1] = fr.Details(inspect.Layout, sel)
		}
	}
	return s
}

type inspectorPanelCache struct {
	image    *ggfx.Image
	snapshot inspectPanelSnapshot
	spare    inspectPanelSnapshot // storage for the next comparison
}

func (c *inspectorPanelCache) release() {
	if c.image != nil {
		c.image.Deallocate()
	}
	*c = inspectorPanelCache{}
}
func (c *inspectorPanelCache) draw(dst *ggui.Canvas) {
	op := &ggfx.DrawImageOptions{}
	b := c.image.Bounds()
	op.GeoM.Translate(float64(b.Min.X), float64(b.Min.Y))
	dst.Image.DrawImage(c.image, op)
}

// Release implements ggui.InspectorPanel: the panel image goes with the app.
func (in *View) Release() { in.cache.release() }
