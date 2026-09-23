//go:build ggui_inspector

package panel

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/fn"

	"github.com/ironpark/ggfx"
	"golang.org/x/image/font/gofont/gomono"

	"github.com/ironpark/ggui/inspect"
)

// The panel draws the way any Overlay does: imperatively, through the
// ggui.Canvas shape and text methods, in its own monospaced font. It cannot be
// a widget, since App.Draw leaves the trace on while it paints, and a label
// painted through dst.Paint(Text(...)) would join the trace it is reading.

// inspectFontSize is the panel's type size in logical pixels.
const inspectFontSize = 12

var inspectFontOnce sync.Once
var inspectFont *ggui.Font

// inspectorFont is the panel's own monospaced font, or ggui's default if
// gomono fails to load.
func inspectorFont() *ggui.Font {
	inspectFontOnce.Do(func() { inspectFont, _ = ggui.LoadFont(gomono.TTF) })
	if inspectFont == nil {
		return ggui.DefaultFont()
	}
	return inspectFont
}

// textWidth measures s in logical pixels.
func textWidth(dst *ggui.Canvas, font *ggui.Font, s string) float64 {
	return dst.TextWidth(s, font, inspectFontSize)
}

// drawLine draws one unwrapped line with its top-left at the logical (x, y).
func drawLine(dst *ggui.Canvas, font *ggui.Font, s string, x, y float64, col color.Color) {
	dst.DrawText(s, font, inspectFontSize, ggui.Pt(x, y), col)
}

// panelBounds is r in the physical pixels the panel's cached image is
// allocated in.
func panelBounds(dst *ggui.Canvas, r ggui.Rect) image.Rectangle { return dst.Physical(r) }

func fitText(dst *ggui.Canvas, font *ggui.Font, s string, width float64) string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\n", " "), "\t", " ")
	if width <= 0 {
		return ""
	}
	if textWidth(dst, font, s) <= width {
		return s
	}
	r := []rune(s)
	lo, hi := 0, len(r)
	for lo < hi {
		m := (lo + hi + 1) / 2
		if textWidth(dst, font, string(r[:m])+"…") <= width {
			lo = m
		} else {
			hi = m - 1
		}
	}
	if lo == 0 && textWidth(dst, font, "…") > width {
		return ""
	}
	return string(r[:lo]) + "…"
}
func fittedLine(dst *ggui.Canvas, font *ggui.Font, s string, x, y, width float64, col color.Color) {
	drawLine(dst, font, fitText(dst, font, s, width), x, y, col)
}
func inset(r ggui.Rect, v float64) ggui.Rect {
	return ggui.Rct(r.Origin.Add(ggui.Pt(v, v)), ggui.Sz(max(r.Size.W-2*v, 0), max(r.Size.H-2*v, 0)))
}

// View chrome, in logical pixels: the toolbar above the body, the status
// bar below it, and within the tree pane the filter field and breadcrumbs.
const (
	inspectToolbarH = 35
	inspectStatusH  = 22
	inspectFilterH  = 36
	inspectCrumbH   = 24
)

// body is the panel between the toolbar and the status bar.
func (in *View) body() ggui.Rect {
	return ggui.Rct(ggui.Pt(in.panel.Origin.X, in.panel.Origin.Y+inspectToolbarH), ggui.Sz(in.panel.Size.W, max(in.panel.Size.H-inspectToolbarH-inspectStatusH, 0)))
}

// detailTab is the pane the details column shows: Layout moves to its own
// column on wide panels, where the column defaults to Computed.
func (in *View) detailTab() inspectTab {
	if !in.layout.Empty() && in.tab == inspectTabLayout {
		return inspectTabComputed
	}
	return in.tab
}

func (in *View) bounds(size ggui.Size) {
	in.viewport = size
	if in.dock == ggui.InspectorBottom {
		h := in.height
		if h == 0 {
			h = min(430, size.H*.55)
		}
		h = fn.Clamp(h, min(180.0, size.H*.8), size.H*.85)
		in.panel = ggui.Rct(ggui.Pt(0, size.H-h), ggui.Sz(size.W, h))
		in.edge = ggui.Rct(ggui.Pt(0, in.panel.Origin.Y-3), ggui.Sz(size.W, 6))
	} else {
		w := in.width
		if w == 0 {
			w = min(440, size.W*.48)
		}
		w = fn.Clamp(w, min(260.0, size.W*.6), size.W*.85)
		in.panel = ggui.Rct(ggui.Pt(size.W-w, 0), ggui.Sz(w, size.H))
		in.edge = ggui.Rct(ggui.Pt(in.panel.Origin.X-3, 0), ggui.Sz(6, size.H))
	}
}
func (in *View) paint(dst *ggui.Canvas, fr *inspect.Frame) {
	if dst.Size().W <= 0 || dst.Size().H <= 0 {
		in.panel = ggui.Rect{}
		return
	}
	in.bounds(dst.Size())
	in.frame = fr
	pal, font := inspectColors(), inspectorFont()
	sel, shown := in.selection(dst, fr)
	// FPS and status hints remain live even when the panel image is reused.
	defer in.paintStatus(dst, font, pal, sel)
	if in.outlines {
		for _, e := range fr.Nodes {
			r := e.Rect
			if e.Clipped {
				r = r.Intersect(e.Clip)
			}
			if !r.Empty() {
				dst.StrokeRoundRect(r, 0, 1, withAlpha(inspectDepth[e.Depth%len(inspectDepth)], 100))
			}
		}
	}
	if sel >= 0 {
		in.paintHighlight(dst, font, pal, sel)
	}
	if dst.Image != nil {
		// The snapshot is taken against last frame's layout; paintPanel may
		// clamp or reveal the scroll, in which case it is taken again.
		snapshot := in.panelSnapshot(dst, fr, shown, sel, in.cache.spare)
		if in.cache.image != nil && in.cache.snapshot.equal(snapshot) && !in.reveal {
			in.cache.spare = snapshot
			in.cache.draw(dst)
			return
		}
		bounds := panelBounds(dst, in.panel)
		if !bounds.Empty() {
			if in.cache.image == nil || in.cache.image.Bounds() != bounds {
				in.cache.release()
				in.cache.image = ggfx.NewImageWithOptions(bounds, nil)
			}
			in.cache.image.Clear()
			cached := *dst
			cached.Image = in.cache.image
			before := in.scroll
			in.paintPanel(&cached, font, pal, sel, shown)
			if in.scroll != before || snapshot.state.tree != in.tree || snapshot.state.layout != in.layout {
				snapshot = in.panelSnapshot(dst, fr, shown, sel, snapshot)
			}
			in.cache.spare, in.cache.snapshot = in.cache.snapshot, snapshot
			in.cache.draw(dst)
			in.reveal = false
			return
		}
	}
	in.paintPanel(dst, font, pal, sel, shown)
	in.reveal = false
}

func (in *View) paintPanel(dst *ggui.Canvas, font *ggui.Font, pal inspectPalette, sel int, shown []int) {
	in.chips = in.chips[:0]
	in.rows = in.rows[:0]
	panel := dst.Clip(in.panel)
	panel.FillRect(in.panel, pal.bg)
	panel.StrokeRoundRect(in.panel, 0, 1, pal.edge)
	in.paintToolbar(panel, font, pal)
	body := in.body()
	ratio := in.split
	if ratio == 0 {
		ratio = .46
	}
	var treePane ggui.Rect
	in.layout, in.layoutThumb = ggui.Rect{}, ggui.Rect{}
	if in.sideBySide() {
		w := math.Round(body.Size.W * ratio)
		treePane = ggui.Rct(body.Origin, ggui.Sz(w, body.Size.H))
		in.detail = ggui.Rct(body.Origin.Add(ggui.Pt(w, 0.0)), ggui.Sz(body.Size.W-w, body.Size.H))
		in.divider = ggui.Rct(body.Origin.Add(ggui.Pt(w-2, 0.0)), ggui.Sz(5, body.Size.H))
		if body.Size.W >= 1100 {
			dw := math.Round(in.detail.Size.W * .53)
			in.layout = ggui.Rct(in.detail.Origin.Add(ggui.Pt(dw, 0.0)), ggui.Sz(in.detail.Size.W-dw, in.detail.Size.H))
			in.detail.Size.W = dw
		}
	} else {
		h := math.Round(body.Size.H * ratio)
		treePane = ggui.Rct(body.Origin, ggui.Sz(body.Size.W, h))
		in.detail = ggui.Rct(body.Origin.Add(ggui.Pt(0.0, h)), ggui.Sz(body.Size.W, body.Size.H-h))
		in.divider = ggui.Rct(body.Origin.Add(ggui.Pt(0.0, h-2)), ggui.Sz(body.Size.W, 5))
	}
	in.paintFilter(panel, treePane, font, pal, shown)
	in.tree = ggui.Rct(treePane.Origin.Add(ggui.Pt(0.0, inspectFilterH)), ggui.Sz(treePane.Size.W, max(treePane.Size.H-inspectFilterH-inspectCrumbH, 0)))
	in.treeTop = in.tree.Origin.Y
	pointer, hasPointer := dst.Pointer()
	in.paintTree(panel, font, pal, shown, sel, pointer, hasPointer)
	crumb := ggui.Rct(ggui.Pt(treePane.Origin.X, treePane.Origin.Y+max(treePane.Size.H-inspectCrumbH, 0)), ggui.Sz(treePane.Size.W, min(inspectCrumbH, treePane.Size.H)))
	in.paintCrumbs(panel, crumb, font, pal, sel)
	in.paintDetail(panel, font, pal, sel)
	if !in.layout.Empty() {
		in.paintLayout(panel, font, pal, sel)
	}
	if in.sideBySide() {
		panel.FillRect(ggui.Rct(ggui.Pt(in.divider.Origin.X+2, body.Origin.Y), ggui.Sz(1, body.Size.H)), pal.edge)
	} else {
		panel.FillRect(ggui.Rct(ggui.Pt(body.Origin.X, in.divider.Origin.Y+2), ggui.Sz(body.Size.W, 1)), pal.edge)
	}
	in.reveal = false
}

func (in *View) paintToolbar(dst *ggui.Canvas, font *ggui.Font, pal inspectPalette) {
	r := ggui.Rct(in.panel.Origin, ggui.Sz(in.panel.Size.W, inspectToolbarH))
	dst.FillRect(r, pal.field)
	dst.FillRect(ggui.Rct(ggui.Pt(r.Origin.X, r.Origin.Y+inspectToolbarH-1), ggui.Sz(r.Size.W, 1)), pal.edge)
	x, y := r.Origin.X+6, r.Origin.Y+5
	in.tool(dst, font, pal, ggui.Rct(ggui.Pt(x, y), ggui.Sz(25, 25)), inspectUnpin, in.picking)
	label := "Elements"
	if r.Size.W < 260 {
		label = "Tree"
	}
	drawLine(dst, font, label, x+34, y+6, pal.fg)
	dst.FillRect(ggui.Rct(ggui.Pt(x+32, r.Origin.Y+32), ggui.Sz(textWidth(dst, font, label)+4, 2)), pal.num)
	x = r.Origin.X + r.Size.W - 112
	if x-(r.Origin.X+110) > 70 {
		stats := fmt.Sprintf("%d nodes", len(in.nodes()))
		fittedLine(dst, font, stats, r.Origin.X+120, y+6, x-r.Origin.X-125, pal.dim)
	}
	for _, act := range []inspectAction{inspectToggleOutlines, inspectDockRight, inspectDockBottom, inspectClose} {
		on := act == inspectToggleOutlines && in.outlines || act == inspectDockRight && in.dock == ggui.InspectorRight || act == inspectDockBottom && in.dock == ggui.InspectorBottom
		in.tool(dst, font, pal, ggui.Rct(ggui.Pt(x, y), ggui.Sz(25, 25)), act, on)
		x += 27
	}
}
func (in *View) tool(dst *ggui.Canvas, font *ggui.Font, pal inspectPalette, r ggui.Rect, act inspectAction, on bool) {
	col := pal.dim
	if on {
		dst.FillRoundRect(r, 4, pal.sel)
		col = pal.num
	}
	p, ok := dst.Pointer()
	if ok && r.Contains(p) && !on {
		dst.FillRoundRect(r, 4, pal.chip)
		col = pal.fg
	}
	b := inset(r, 6)
	x, y, w, h := b.Origin.X, b.Origin.Y, b.Size.W, b.Size.H
	switch act {
	case inspectClose:
		dst.StrokeLine(ggui.Pt(x+1, y+1), ggui.Pt(x+w-1, y+h-1), 1.4, col)
		dst.StrokeLine(ggui.Pt(x+w-1, y+1), ggui.Pt(x+1, y+h-1), 1.4, col)
	case inspectDockRight, inspectDockBottom:
		dst.StrokeRoundRect(b, 1, 1.2, col)
		if act == inspectDockRight {
			dst.FillRect(ggui.Rct(ggui.Pt(x+w*.6, y+2), ggui.Sz(w*.4-2, h-4)), col)
		} else {
			dst.FillRect(ggui.Rct(ggui.Pt(x+2, y+h*.6), ggui.Sz(w-4, h*.4-2)), col)
		}
	case inspectToggleOutlines:
		dst.StrokeRoundRect(b, 0, 1, col)
		dst.StrokeRoundRect(inset(b, 3), 0, 1, col)
	case inspectUnpin:
		dst.StrokeRoundRect(b, 1, 1, col)
		dst.StrokeLine(ggui.Pt(x+w/2, y-2), ggui.Pt(x+w/2, y+h+2), 1, col)
		dst.StrokeLine(ggui.Pt(x-2, y+h/2), ggui.Pt(x+w+2, y+h/2), 1, col)
	case inspectCopy:
		dst.StrokeRoundRect(ggui.Rct(ggui.Pt(x, y), ggui.Sz(w-3, h-3)), 1, 1, col)
		dst.FillRect(ggui.Rct(ggui.Pt(x+3, y+3), ggui.Sz(w-3, h-3)), pal.bg)
		dst.StrokeRoundRect(ggui.Rct(ggui.Pt(x+3, y+3), ggui.Sz(w-3, h-3)), 1, 1, col)
	case inspectClearFilter:
		drawLine(dst, font, "×", x, y-1, col)
	}
	in.chips = append(in.chips, inspectChip{rect: r.Intersect(in.panel), act: act})
}
func (in *View) paintFilter(dst *ggui.Canvas, pane ggui.Rect, font *ggui.Font, pal inspectPalette, shown []int) {
	r := ggui.Rct(pane.Origin.Add(ggui.Pt(8.0, 6.0)), ggui.Sz(max(pane.Size.W-16, 0), 25))
	in.filterRect = r
	clip := dst.Clip(pane)
	clip.FillRoundRect(r, 4, pal.field)
	clip.StrokeRoundRect(r, 4, 1, fn.Pick(in.filterFocus, pal.num, pal.edge))
	x, y := r.Origin.X, r.Origin.Y
	clip.StrokeRoundRect(ggui.Rct(ggui.Pt(x+7, y+7), ggui.Sz(9, 9)), 4, 1.2, pal.dim)
	clip.StrokeLine(ggui.Pt(x+15, y+15), ggui.Pt(x+19, y+19), 1.2, pal.dim)
	label, col := in.filter, pal.fg
	if label == "" {
		label, col = "Find type, name, role…", pal.dim
	}
	if in.filterFocus && in.selectFilter {
		clip.FillRect(ggui.Rct(ggui.Pt(x+25, y+4), ggui.Sz(min(textWidth(dst, font, label), max(r.Size.W-55, 0)), 17)), pal.sel)
	}
	fittedLine(clip, font, label, x+25, y+5, r.Size.W-53, col)
	if in.filterFocus && !in.selectFilter {
		cx := x + 25 + min(textWidth(dst, font, in.filter), max(r.Size.W-55, 0))
		clip.FillRect(ggui.Rct(ggui.Pt(cx, y+5), ggui.Sz(1, 15)), pal.num)
	}
	if in.filter != "" {
		in.tool(clip, font, pal, ggui.Rct(ggui.Pt(x+r.Size.W-25, y), ggui.Sz(25, 25)), inspectClearFilter, false)
	}
}
func (in *View) paintTree(dst *ggui.Canvas, font *ggui.Font, pal inspectPalette, shown []int, sel int, pointer ggui.Point, hasPointer bool) {
	r := in.tree
	lh := float64(inspectRowHeight)
	in.treeContent = float64(len(shown)) * lh
	if at := slices.Index(shown, sel); at >= 0 && in.reveal {
		top := float64(at) * lh
		if top < in.scroll {
			in.scroll = top
		} else if top+lh > in.scroll+r.Size.H {
			in.scroll = top + lh - r.Size.H
		}
	}
	in.scroll = fn.Clamp(in.scroll, 0, max(in.treeContent-r.Size.H, 0))
	clip := dst.Clip(r)
	if len(shown) == 0 {
		fittedLine(clip, font, "No matching widgets", r.Origin.X+12, r.Origin.Y+16, r.Size.W-24, pal.dim)
	}
	first, end := inspectRowRange(len(shown), in.scroll, r.Size.H)
	for at := first; at < end; at++ {
		i := shown[at]
		y := r.Origin.Y + float64(at)*lh - in.scroll
		if y+lh <= r.Origin.Y || y >= r.Origin.Y+r.Size.H {
			continue
		}
		e := in.frame.Describe(i)
		key := keyOf(e)
		in.rows = append(in.rows, inspectRow{key: key, index: i, y: y, h: lh})
		row := ggui.Rct(ggui.Pt(r.Origin.X, y), ggui.Sz(r.Size.W, lh))
		fg, dim := pal.key, pal.dim
		if i == sel {
			clip.FillRect(row, pal.sel)
			clip.FillRect(ggui.Rct(row.Origin, ggui.Sz(2, lh)), pal.num)
			fg, dim = pal.selFg, pal.selFg
		} else if hasPointer && row.Contains(pointer) {
			clip.FillRect(row, pal.hover)
		}
		depth := min(float64(e.Depth)*inspectIndent, max(r.Size.W*.3, 0))
		x := r.Origin.X + 8 + depth
		for d := 0; d < e.Depth && float64(d)*inspectIndent < depth; d++ {
			gx := r.Origin.X + 14 + float64(d)*inspectIndent
			clip.FillRect(ggui.Rct(ggui.Pt(gx, y), ggui.Sz(1, lh)), pal.edge)
		}
		if inspect.HasChildren(in.frame.Nodes, i) {
			triangle(clip, ggui.Pt(x+5, y+lh/2), in.folded(e) && in.filter == "", dim)
			in.chips = append(in.chips, inspectChip{rect: ggui.Rct(ggui.Pt(x-2, y), ggui.Sz(15, lh)).Intersect(r), act: inspectCollapse, key: key})
		}
		x += 15
		right := r.Origin.X + r.Size.W - 12
		dims := num(e.Rect.Size.W) + " × " + num(e.Rect.Size.H)
		if r.Size.W >= 320 {
			dw := textWidth(dst, font, dims)
			drawLine(clip, font, dims, right-dw, y+5, dim)
			right -= dw + 12
		}
		name := fitText(dst, font, e.Name, max(right-x, 0))
		drawLine(clip, font, name, x, y+5, fg)
		x += textWidth(dst, font, name) + 7
		if badge := in.frame.Badge(i); badge != "" {
			bw := textWidth(dst, font, badge) + 10
			if x+bw < right {
				clip.FillRoundRect(ggui.Rct(ggui.Pt(x, y+4), ggui.Sz(bw, 15)), 3, pal.chip)
				drawLine(clip, font, badge, x+5, y+5, pal.dim)
				x += bw + 7
			}
		}
		if label := e.Label; label != "" {
			fittedLine(clip, font, strconv.Quote(label), x, y+5, right-x, pal.dim)
		}
	}
	in.treeThumb = inspectScrollbar(dst, r, in.treeContent, in.scroll, pal)
}
func triangle(dst *ggui.Canvas, c ggui.Point, folded bool, col color.Color) {
	var p ggui.Path
	const s = 3.0
	if folded {
		p.MoveTo(c.X-s/2, c.Y-s)
		p.LineTo(c.X+s/2+1, c.Y)
		p.LineTo(c.X-s/2, c.Y+s)
	} else {
		p.MoveTo(c.X-s, c.Y-s/2)
		p.LineTo(c.X+s, c.Y-s/2)
		p.LineTo(c.X, c.Y+s/2+1)
	}
	p.Close()
	dst.FillPath(&p, col)
}
func inspectScrollbar(dst *ggui.Canvas, r ggui.Rect, content, scroll float64, pal inspectPalette) ggui.Rect {
	if r.Empty() || content <= r.Size.H {
		return ggui.Rect{}
	}
	h := min(max(r.Size.H*r.Size.H/content, 18), r.Size.H)
	y := r.Origin.Y + (r.Size.H-h)*scroll/max(content-r.Size.H, 1)
	thumb := ggui.Rct(ggui.Pt(r.Origin.X+r.Size.W-7, y), ggui.Sz(inspectBar, h))
	dst.FillRoundRect(thumb, 2, withAlpha(pal.dim, 110))
	return thumb
}
func (in *View) paintCrumbs(dst *ggui.Canvas, r ggui.Rect, font *ggui.Font, pal inspectPalette, sel int) {
	clip := dst.Clip(r)
	clip.FillRect(r, pal.field)
	clip.FillRect(ggui.Rct(r.Origin, ggui.Sz(r.Size.W, 1)), pal.edge)
	if sel < 0 {
		return
	}
	nodes := in.frame.Nodes
	chain := inspect.Ancestors(nodes, sel)
	slices.Reverse(chain)
	chain = append(chain, sel)
	total := 0.0
	for _, i := range chain {
		total += textWidth(dst, font, nodes[i].Name) + 18
	}
	x := r.Origin.X + 8 - min(max(total-r.Size.W+8, 0), total)
	for n, i := range chain {
		e := &nodes[i]
		w := textWidth(dst, font, e.Name)
		drawLine(clip, font, e.Name, x, r.Origin.Y+6, fn.Pick(i == sel, pal.fg, pal.dim))
		in.chips = append(in.chips, inspectChip{rect: ggui.Rct(ggui.Pt(x, r.Origin.Y), ggui.Sz(w, r.Size.H)).Intersect(r), act: inspectPin, key: keyOf(e)})
		x += w + 6
		if n < len(chain)-1 {
			drawLine(clip, font, "›", x, r.Origin.Y+6, pal.dim)
			x += 12
		}
	}
}
func (in *View) tabButton(dst *ggui.Canvas, font *ggui.Font, pal inspectPalette, x, y float64, label string, tab inspectTab, on bool) float64 {
	w := textWidth(dst, font, label) + 18
	r := ggui.Rct(ggui.Pt(x, y), ggui.Sz(w, 31))
	if on {
		dst.FillRect(ggui.Rct(ggui.Pt(x, y+29), ggui.Sz(w, 2)), pal.num)
	}
	drawLine(dst, font, label, x+9, y+10, fn.Pick(on, pal.num, pal.dim))
	in.chips = append(in.chips, inspectChip{rect: r.Intersect(in.detail), act: inspectSelectTab, tab: tab})
	return x + w
}
func (in *View) paintDetail(dst *ggui.Canvas, font *ggui.Font, pal inspectPalette, sel int) {
	r := in.detail
	clip := dst.Clip(r)
	clip.FillRect(ggui.Rct(r.Origin, ggui.Sz(r.Size.W, 32)), pal.field)
	clip.FillRect(ggui.Rct(ggui.Pt(r.Origin.X, r.Origin.Y+31), ggui.Sz(r.Size.W, 1)), pal.edge)
	x, y := r.Origin.X, r.Origin.Y
	tabs := []inspect.Tab{inspect.Layout, inspect.Computed, inspect.Semantics}
	if !in.layout.Empty() {
		tabs = tabs[1:]
	}
	shownTab := in.detailTab()
	for _, tab := range tabs {
		label := "Layout"
		if tab == inspect.Computed {
			label = "Computed"
		}
		if tab == inspect.Semantics {
			label = "Accessibility"
		}
		if r.Size.W < 330 {
			switch tab {
			case inspect.Layout:
				label = "Box"
			case inspect.Computed:
				label = "Style"
			case inspect.Semantics:
				label = "A11y"
			}
		}
		x = in.tabButton(clip, font, pal, x, y, label, tab, tab == shownTab)
	}
	if sel >= 0 && x < r.Origin.X+r.Size.W-28 {
		in.tool(clip, font, pal, ggui.Rct(ggui.Pt(r.Origin.X+r.Size.W-28, y+3), ggui.Sz(25, 25)), inspectCopy, in.copied)
	}
	in.detailBody = ggui.Rct(ggui.Pt(r.Origin.X, y+32), ggui.Sz(r.Size.W, max(r.Size.H-32, 0)))
	if sel < 0 {
		in.detailScroll = 0
		in.detailThumb = ggui.Rect{}
		message := "Select a widget to inspect"
		if in.pinned {
			message = "Selected widget is no longer painted"
		}
		fittedLine(clip, font, message, r.Origin.X+12, y+54, r.Size.W-24, pal.dim)
		return
	}
	nodes := in.frame.Nodes
	fields := in.frame.Details(shownTab, sel)
	boxH := 0.0
	if shownTab == inspect.Layout {
		boxH = inspectBoxHeight(in.frame.BoxOf(sel))
	}
	in.detailContent = 32 + boxH + inspectFieldsHeight(fields) + 8
	in.detailScroll = fn.Clamp(in.detailScroll, 0, max(in.detailContent-in.detailBody.Size.H, 0))
	content := clip.Clip(in.detailBody)
	cy := in.detailBody.Origin.Y + 10 - in.detailScroll
	fittedLine(content, font, nodes[sel].Name, r.Origin.X+12, cy, r.Size.W-24, pal.fg)
	cy += 24
	if boxH > 0 {
		in.paintBox(content, font, pal, ggui.Rct(ggui.Pt(r.Origin.X+12, cy), ggui.Sz(r.Size.W-24, boxH)), in.frame.BoxOf(sel))
		cy += boxH
	}
	paintInspectFields(content, font, pal, ggui.Rct(ggui.Pt(r.Origin.X, cy), ggui.Sz(r.Size.W, inspectFieldsHeight(fields))), fields)
	in.detailThumb = inspectScrollbar(clip, in.detailBody, in.detailContent, in.detailScroll, pal)
}
func inspectFieldsHeight(fields []inspect.Field) float64 {
	h := 0.0
	for _, f := range fields {
		h += fn.Pick(f.Value == "", 28.0, 23.0)
	}
	return h
}
func paintInspectFields(dst *ggui.Canvas, font *ggui.Font, pal inspectPalette, r ggui.Rect, fields []inspect.Field) {
	y := r.Origin.Y
	kw := min(112.0, r.Size.W*.42)
	for _, f := range fields {
		if f.Value == "" {
			dst.FillRect(ggui.Rct(ggui.Pt(r.Origin.X, y), ggui.Sz(r.Size.W, 25)), pal.field)
			fittedLine(dst, font, f.Key, r.Origin.X+12, y+6, r.Size.W-24, pal.dim)
			y += 28
			continue
		}
		fittedLine(dst, font, f.Key, r.Origin.X+12, y+4, kw-15, pal.key)
		vx := r.Origin.X + kw + 8
		value := f.Value
		if strings.HasPrefix(value, "#") && (len(value) == 7 || len(value) == 9) {
			if rgba, err := strconv.ParseUint(value[1:], 16, 32); err == nil {
				var c color.NRGBA
				if len(value) == 7 {
					c = color.NRGBA{uint8(rgba >> 16), uint8(rgba >> 8), uint8(rgba), 255}
				} else {
					c = color.NRGBA{uint8(rgba >> 24), uint8(rgba >> 16), uint8(rgba >> 8), uint8(rgba)}
				}
				dst.FillRoundRect(ggui.Rct(ggui.Pt(vx, y+5), ggui.Sz(11, 11)), 2, c)
				dst.StrokeRoundRect(ggui.Rct(ggui.Pt(vx, y+5), ggui.Sz(11, 11)), 2, 1, pal.edge)
				vx += 17
			}
		}
		fittedLine(dst, font, value, vx, y+4, r.Origin.X+r.Size.W-vx-12, fn.Pick(f.Number, pal.num, pal.fg))
		y += 23
	}
}
func inspectBoxHeight(box inspect.Box) float64 {
	if box.Valid {
		return 190
	}
	return 92
}
func (in *View) paintBox(dst *ggui.Canvas, font *ggui.Font, pal inspectPalette, r ggui.Rect, box inspect.Box) {
	if r.Size.W < 60 {
		return
	}
	label := "CONTENT BOUNDS"
	if box.Valid {
		label = "BOX MODEL"
	}
	drawLine(dst, font, label, r.Origin.X, r.Origin.Y, pal.dim)
	outer := ggui.Rct(ggui.Pt(r.Origin.X, r.Origin.Y+24), ggui.Sz(r.Size.W, fn.Pick(box.Valid, 132.0, 44.0)))
	if !box.Valid {
		dst.FillRect(outer, pal.content)
		dst.StrokeRoundRect(outer, 0, 1, pal.edge)
		centerText(dst, font, num(box.Content.W)+" × "+num(box.Content.H), outer, pal.fg)
		return
	}
	dst.FillRect(outer, pal.border)
	dst.StrokeRoundRect(outer, 0, 1, pal.edge)
	pad := inset(outer, 17)
	dst.FillRect(pad, pal.padding)
	dst.StrokeRoundRect(pad, 0, 1, withAlpha(pal.dim, 100))
	content := ggui.Rct(pad.Origin.Add(ggui.Pt(32.0, 28.0)), ggui.Sz(max(pad.Size.W-64, 0), max(pad.Size.H-56, 0)))
	dst.FillRect(content, pal.content)
	dst.StrokeRoundRect(content, 0, 1, withAlpha(pal.dim, 100))
	drawLine(dst, font, "border", outer.Origin.X+5, outer.Origin.Y+3, pal.fg)
	centerText(dst, font, num(box.Border), ggui.Rct(ggui.Pt(outer.Origin.X+outer.Size.W*.5-10, outer.Origin.Y), ggui.Sz(20, 17)), pal.fg)
	drawLine(dst, font, "padding", pad.Origin.X+5, pad.Origin.Y+3, pal.fg)
	centerText(dst, font, num(box.Padding.Top), ggui.Rct(ggui.Pt(content.Origin.X, pad.Origin.Y+13), ggui.Sz(content.Size.W, 15)), pal.fg)
	centerText(dst, font, num(box.Padding.Bottom), ggui.Rct(ggui.Pt(content.Origin.X, content.Origin.Y+content.Size.H), ggui.Sz(content.Size.W, 28)), pal.fg)
	centerText(dst, font, num(box.Padding.Left), ggui.Rct(ggui.Pt(pad.Origin.X, content.Origin.Y), ggui.Sz(32, content.Size.H)), pal.fg)
	centerText(dst, font, num(box.Padding.Right), ggui.Rct(ggui.Pt(content.Origin.X+content.Size.W, content.Origin.Y), ggui.Sz(32, content.Size.H)), pal.fg)
	centerText(dst, font, num(box.Content.W)+" × "+num(box.Content.H), content, pal.fg)
	fittedLine(dst, font, "Border paints inside the bounds", r.Origin.X, r.Origin.Y+165, r.Size.W, pal.dim)
}
func centerText(dst *ggui.Canvas, font *ggui.Font, s string, r ggui.Rect, c color.Color) {
	s = fitText(dst, font, s, r.Size.W-4)
	drawLine(dst, font, s, r.Origin.X+(r.Size.W-textWidth(dst, font, s))/2, r.Origin.Y+(r.Size.H-13)/2, c)
}
func (in *View) paintLayout(dst *ggui.Canvas, font *ggui.Font, pal inspectPalette, sel int) {
	r := in.layout
	clip := dst.Clip(r)
	clip.FillRect(ggui.Rct(r.Origin, ggui.Sz(1, r.Size.H)), pal.edge)
	clip.FillRect(ggui.Rct(r.Origin, ggui.Sz(r.Size.W, 32)), pal.field)
	drawLine(clip, font, "Layout", r.Origin.X+12, r.Origin.Y+10, pal.fg)
	if sel < 0 {
		return
	}
	box := in.frame.BoxOf(sel)
	fields := in.frame.Details(inspect.Layout, sel)
	body := ggui.Rct(r.Origin.Add(ggui.Pt(0.0, 32.0)), ggui.Sz(r.Size.W, max(r.Size.H-32, 0)))
	content := inspectBoxHeight(box) + inspectFieldsHeight(fields) + 24
	in.layoutBody, in.layoutContent = body, content
	in.layoutScroll = fn.Clamp(in.layoutScroll, 0, max(content-body.Size.H, 0))
	bc := clip.Clip(body)
	y := body.Origin.Y + 12 - in.layoutScroll
	in.paintBox(bc, font, pal, ggui.Rct(ggui.Pt(r.Origin.X+12, y), ggui.Sz(r.Size.W-24, inspectBoxHeight(box))), box)
	y += inspectBoxHeight(box)
	paintInspectFields(bc, font, pal, ggui.Rct(ggui.Pt(r.Origin.X, y), ggui.Sz(r.Size.W, inspectFieldsHeight(fields))), fields)
	in.layoutThumb = inspectScrollbar(clip, body, content, in.layoutScroll, pal)
}
func (in *View) paintHighlight(dst *ggui.Canvas, font *ggui.Font, pal inspectPalette, sel int) {
	e := &in.frame.Nodes[sel]
	r := e.Rect
	clip := dst
	if e.Clipped {
		clip = dst.Clip(e.Clip)
	}
	box := in.frame.BoxOf(sel)
	if box.Valid {
		clip.FillRect(r, withAlpha(pal.padding, 100))
		clip.FillRect(ggui.Rct(r.Origin.Add(ggui.Pt(box.Padding.Left, box.Padding.Top)), box.Content), pal.hi)
	} else {
		clip.FillRect(r, pal.hi)
	}
	clip.StrokeRoundRect(r, 0, 1.5, pal.num)
	visible := r
	if e.Clipped {
		visible = r.Intersect(e.Clip)
	}
	if visible.Empty() {
		return
	}
	label := e.Name + "  " + num(r.Size.W) + " × " + num(r.Size.H)
	w := min(textWidth(dst, font, label)+18, dst.Size().W-12)
	x := fn.Clamp(visible.Origin.X, 6, max(dst.Size().W-w-6, 6))
	y := max(6.0, visible.Origin.Y-29)
	badge := ggui.Rct(ggui.Pt(x, y), ggui.Sz(w, 24))
	if !badge.Intersect(in.panel).Empty() {
		return
	}
	dst.FillRoundRect(badge, 4, pal.bg)
	dst.StrokeRoundRect(badge, 4, 1, pal.num)
	fittedLine(dst, font, label, x+9, y+6, w-18, pal.fg)
}
func (in *View) paintStatus(dst *ggui.Canvas, font *ggui.Font, pal inspectPalette, sel int) {
	r := ggui.Rct(ggui.Pt(in.panel.Origin.X, in.panel.Origin.Y+in.panel.Size.H-inspectStatusH), ggui.Sz(in.panel.Size.W, inspectStatusH))
	dst.FillRect(r, pal.field)
	dst.FillRect(ggui.Rct(r.Origin, ggui.Sz(r.Size.W, 1)), pal.edge)
	status := "Click a row to pin · ⌘/Ctrl F to find"
	if in.picking {
		status = "Pick a widget in the app · Esc cancels"
	} else if in.pinned {
		status = "Pinned · ← → expand · ↑ ↓ navigate"
	}
	if in.filter != "" {
		status = fmt.Sprintf("%d matches · Esc clears the filter", in.matches)
	}
	if in.copied {
		status = "ggui.Widget details copied"
	}
	p, ok := dst.Pointer()
	if ok {
		for _, c := range in.chips {
			if c.rect.Contains(p) {
				switch c.act {
				case inspectClose:
					status = "Close inspector"
				case inspectUnpin:
					status = "Pick a widget without activating it"
				case inspectDockRight:
					status = "Dock to the right"
				case inspectDockBottom:
					status = "Dock to the bottom"
				case inspectToggleOutlines:
					status = "Toggle all widget outlines"
				case inspectCopy:
					status = "Copy layout, computed and accessibility details"
				}
				break
			}
		}
	}
	right := fmt.Sprintf("%d fps · %s×", in.frames.count(time.Now()), num(dst.Scale()))
	space := r.Size.W - 16
	if r.Size.W > 600 {
		drawLine(dst, font, right, r.Origin.X+r.Size.W-textWidth(dst, font, right)-10, r.Origin.Y+5, pal.dim)
		space -= textWidth(dst, font, right) + 20
	}
	fittedLine(dst, font, status, r.Origin.X+8, r.Origin.Y+5, space, pal.dim)
}
