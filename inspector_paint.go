//go:build ggui_inspector

package ggui

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"golang.org/x/image/font/gofont/gomono"

	"github.com/ironpark/ggui/inspect"
)

// Every place the inspector reaches past ggui's public API to draw itself.
//
// It cannot draw the way a widget does: App.Draw leaves frameState.tracing
// on while the inspector paints, and Canvas.Inert keeps the frame's trace
// rather than detaching it, so a label painted through dst.Paint(Text(...))
// would be appended to the trace the inspector is reading back mid-paint.
// An overlay has to draw imperatively, and of that ggui exposes shapes and
// paths but no text: no *Font-to-text.Face, no measurement, no draw.
//
// Collecting the reaches here keeps the rest of the inspector on public
// calls, and leaves one file to consult if the panel ever moves into a
// package of its own. What it reads is already public: the inspect.Frame
// that App.OnInspect hands out, built in inspector_frame.go.

var inspectFontOnce sync.Once
var inspectFont *Font

// inspectorFace is the inspector's own monospaced face, sized in physical
// pixels. It falls back to ggui's default font if gomono fails to load.
func inspectorFace(dst *Canvas) text.Face {
	inspectFontOnce.Do(func() { inspectFont, _ = LoadFont(gomono.TTF) })
	if inspectFont == nil {
		return fallbackFont().face(12 * dst.Scale())
	}
	return inspectFont.face(12 * dst.Scale())
}

// textWidth measures s in logical pixels.
func textWidth(dst *Canvas, face text.Face, s string) float64 { return dst.dp(lineWidth(s, face)) }

// drawLine draws one unwrapped line with its top-left at the logical (x, y).
func drawLine(dst *Canvas, face text.Face, s string, x, y float64, col color.Color) {
	if dst == nil || dst.Image == nil {
		return
	}
	op := &text.DrawOptions{}
	op.ColorScale.ScaleWithColor(col)
	op.GeoM.Translate(dst.px(x), dst.px(y))
	drawText(dst.Image, s, face, op)
}

// panelBounds is r in the physical pixels the panel's cached image is
// allocated in.
func panelBounds(dst *Canvas, r Rect) image.Rectangle { return dst.physical(r) }

func fitText(dst *Canvas, face text.Face, s string, width float64) string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\n", " "), "\t", " ")
	if width <= 0 {
		return ""
	}
	if textWidth(dst, face, s) <= width {
		return s
	}
	r := []rune(s)
	lo, hi := 0, len(r)
	for lo < hi {
		m := (lo + hi + 1) / 2
		if textWidth(dst, face, string(r[:m])+"…") <= width {
			lo = m
		} else {
			hi = m - 1
		}
	}
	if lo == 0 && textWidth(dst, face, "…") > width {
		return ""
	}
	return string(r[:lo]) + "…"
}
func fittedLine(dst *Canvas, face text.Face, s string, x, y, width float64, col color.Color) {
	drawLine(dst, face, fitText(dst, face, s, width), x, y, col)
}
func inset(r Rect, v float64) Rect {
	return Rct(r.Origin.Add(Pt(v, v)), Sz(max(r.Size.W-2*v, 0), max(r.Size.H-2*v, 0)))
}

// Panel chrome, in logical pixels: the toolbar above the body, the status
// bar below it, and within the tree pane the filter field and breadcrumbs.
const (
	inspectToolbarH = 35
	inspectStatusH  = 22
	inspectFilterH  = 36
	inspectCrumbH   = 24
)

// body is the panel between the toolbar and the status bar.
func (in *inspector) body() Rect {
	return Rct(Pt(in.panel.Origin.X, in.panel.Origin.Y+inspectToolbarH), Sz(in.panel.Size.W, max(in.panel.Size.H-inspectToolbarH-inspectStatusH, 0)))
}

// detailTab is the pane the details column shows: Layout moves to its own
// column on wide panels, where the column defaults to Computed.
func (in *inspector) detailTab() inspectTab {
	if !in.layout.Empty() && in.tab == inspectTabLayout {
		return inspectTabComputed
	}
	return in.tab
}

func (in *inspector) bounds(size Size) {
	in.viewport = size
	if in.dock == InspectorBottom {
		h := in.height
		if h == 0 {
			h = min(430, size.H*.55)
		}
		h = clamp(h, min(180.0, size.H*.8), size.H*.85)
		in.panel = Rct(Pt(0, size.H-h), Sz(size.W, h))
		in.edge = Rct(Pt(0, in.panel.Origin.Y-3), Sz(size.W, 6))
	} else {
		w := in.width
		if w == 0 {
			w = min(440, size.W*.48)
		}
		w = clamp(w, min(260.0, size.W*.6), size.W*.85)
		in.panel = Rct(Pt(size.W-w, 0), Sz(w, size.H))
		in.edge = Rct(Pt(in.panel.Origin.X-3, 0), Sz(6, size.H))
	}
}
func (in *inspector) paint(dst *Canvas, fr *inspect.Frame) {
	if dst.Size().W <= 0 || dst.Size().H <= 0 {
		in.panel = Rect{}
		return
	}
	in.bounds(dst.Size())
	in.frame = fr
	pal, face := inspectColors(), inspectorFace(dst)
	sel, shown := in.selection(dst, fr)
	// FPS and status hints remain live even when the panel image is reused.
	defer in.paintStatus(dst, face, pal, sel)
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
		in.paintHighlight(dst, face, pal, sel)
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
				in.cache.image = ebiten.NewImageWithOptions(bounds, nil)
			}
			in.cache.image.Clear()
			cached := *dst
			cached.Image = in.cache.image
			before := in.scroll
			in.paintPanel(&cached, face, pal, sel, shown)
			if in.scroll != before || snapshot.state.tree != in.tree || snapshot.state.layout != in.layout {
				snapshot = in.panelSnapshot(dst, fr, shown, sel, snapshot)
			}
			in.cache.spare, in.cache.snapshot = in.cache.snapshot, snapshot
			in.cache.draw(dst)
			in.reveal = false
			return
		}
	}
	in.paintPanel(dst, face, pal, sel, shown)
	in.reveal = false
}

func (in *inspector) paintPanel(dst *Canvas, face text.Face, pal inspectPalette, sel int, shown []int) {
	in.chips = in.chips[:0]
	in.rows = in.rows[:0]
	panel := dst.Clip(in.panel)
	panel.FillRect(in.panel, pal.bg)
	panel.StrokeRoundRect(in.panel, 0, 1, pal.edge)
	in.paintToolbar(panel, face, pal)
	body := in.body()
	ratio := in.split
	if ratio == 0 {
		ratio = .46
	}
	var treePane Rect
	in.layout, in.layoutThumb = Rect{}, Rect{}
	if in.sideBySide() {
		w := math.Round(body.Size.W * ratio)
		treePane = Rct(body.Origin, Sz(w, body.Size.H))
		in.detail = Rct(body.Origin.Add(Pt(w, 0.0)), Sz(body.Size.W-w, body.Size.H))
		in.divider = Rct(body.Origin.Add(Pt(w-2, 0.0)), Sz(5, body.Size.H))
		if body.Size.W >= 1100 {
			dw := math.Round(in.detail.Size.W * .53)
			in.layout = Rct(in.detail.Origin.Add(Pt(dw, 0.0)), Sz(in.detail.Size.W-dw, in.detail.Size.H))
			in.detail.Size.W = dw
		}
	} else {
		h := math.Round(body.Size.H * ratio)
		treePane = Rct(body.Origin, Sz(body.Size.W, h))
		in.detail = Rct(body.Origin.Add(Pt(0.0, h)), Sz(body.Size.W, body.Size.H-h))
		in.divider = Rct(body.Origin.Add(Pt(0.0, h-2)), Sz(body.Size.W, 5))
	}
	in.paintFilter(panel, treePane, face, pal, shown)
	in.tree = Rct(treePane.Origin.Add(Pt(0.0, inspectFilterH)), Sz(treePane.Size.W, max(treePane.Size.H-inspectFilterH-inspectCrumbH, 0)))
	in.treeTop = in.tree.Origin.Y
	pointer, hasPointer := dst.Pointer()
	in.paintTree(panel, face, pal, shown, sel, pointer, hasPointer)
	crumb := Rct(Pt(treePane.Origin.X, treePane.Origin.Y+max(treePane.Size.H-inspectCrumbH, 0)), Sz(treePane.Size.W, min(inspectCrumbH, treePane.Size.H)))
	in.paintCrumbs(panel, crumb, face, pal, sel)
	in.paintDetail(panel, face, pal, sel)
	if !in.layout.Empty() {
		in.paintLayout(panel, face, pal, sel)
	}
	if in.sideBySide() {
		panel.FillRect(Rct(Pt(in.divider.Origin.X+2, body.Origin.Y), Sz(1, body.Size.H)), pal.edge)
	} else {
		panel.FillRect(Rct(Pt(body.Origin.X, in.divider.Origin.Y+2), Sz(body.Size.W, 1)), pal.edge)
	}
	in.reveal = false
}

func (in *inspector) paintToolbar(dst *Canvas, face text.Face, pal inspectPalette) {
	r := Rct(in.panel.Origin, Sz(in.panel.Size.W, inspectToolbarH))
	dst.FillRect(r, pal.field)
	dst.FillRect(Rct(Pt(r.Origin.X, r.Origin.Y+inspectToolbarH-1), Sz(r.Size.W, 1)), pal.edge)
	x, y := r.Origin.X+6, r.Origin.Y+5
	in.tool(dst, face, pal, Rct(Pt(x, y), Sz(25, 25)), inspectUnpin, in.picking)
	label := "Elements"
	if r.Size.W < 260 {
		label = "Tree"
	}
	drawLine(dst, face, label, x+34, y+6, pal.fg)
	dst.FillRect(Rct(Pt(x+32, r.Origin.Y+32), Sz(textWidth(dst, face, label)+4, 2)), pal.num)
	x = r.Origin.X + r.Size.W - 112
	if x-(r.Origin.X+110) > 70 {
		stats := fmt.Sprintf("%d nodes", len(in.nodes()))
		fittedLine(dst, face, stats, r.Origin.X+120, y+6, x-r.Origin.X-125, pal.dim)
	}
	for _, act := range []inspectAction{inspectToggleOutlines, inspectDockRight, inspectDockBottom, inspectClose} {
		on := act == inspectToggleOutlines && in.outlines || act == inspectDockRight && in.dock == InspectorRight || act == inspectDockBottom && in.dock == InspectorBottom
		in.tool(dst, face, pal, Rct(Pt(x, y), Sz(25, 25)), act, on)
		x += 27
	}
}
func (in *inspector) tool(dst *Canvas, face text.Face, pal inspectPalette, r Rect, act inspectAction, on bool) {
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
		dst.StrokeLine(Pt(x+1, y+1), Pt(x+w-1, y+h-1), 1.4, col)
		dst.StrokeLine(Pt(x+w-1, y+1), Pt(x+1, y+h-1), 1.4, col)
	case inspectDockRight, inspectDockBottom:
		dst.StrokeRoundRect(b, 1, 1.2, col)
		if act == inspectDockRight {
			dst.FillRect(Rct(Pt(x+w*.6, y+2), Sz(w*.4-2, h-4)), col)
		} else {
			dst.FillRect(Rct(Pt(x+2, y+h*.6), Sz(w-4, h*.4-2)), col)
		}
	case inspectToggleOutlines:
		dst.StrokeRoundRect(b, 0, 1, col)
		dst.StrokeRoundRect(inset(b, 3), 0, 1, col)
	case inspectUnpin:
		dst.StrokeRoundRect(b, 1, 1, col)
		dst.StrokeLine(Pt(x+w/2, y-2), Pt(x+w/2, y+h+2), 1, col)
		dst.StrokeLine(Pt(x-2, y+h/2), Pt(x+w+2, y+h/2), 1, col)
	case inspectCopy:
		dst.StrokeRoundRect(Rct(Pt(x, y), Sz(w-3, h-3)), 1, 1, col)
		dst.FillRect(Rct(Pt(x+3, y+3), Sz(w-3, h-3)), pal.bg)
		dst.StrokeRoundRect(Rct(Pt(x+3, y+3), Sz(w-3, h-3)), 1, 1, col)
	case inspectClearFilter:
		drawLine(dst, face, "×", x, y-1, col)
	}
	in.chips = append(in.chips, inspectChip{rect: r.Intersect(in.panel), act: act})
}
func (in *inspector) paintFilter(dst *Canvas, pane Rect, face text.Face, pal inspectPalette, shown []int) {
	r := Rct(pane.Origin.Add(Pt(8.0, 6.0)), Sz(max(pane.Size.W-16, 0), 25))
	in.filterRect = r
	clip := dst.Clip(pane)
	clip.FillRoundRect(r, 4, pal.field)
	clip.StrokeRoundRect(r, 4, 1, pick(in.filterFocus, pal.num, pal.edge))
	x, y := r.Origin.X, r.Origin.Y
	clip.StrokeRoundRect(Rct(Pt(x+7, y+7), Sz(9, 9)), 4, 1.2, pal.dim)
	clip.StrokeLine(Pt(x+15, y+15), Pt(x+19, y+19), 1.2, pal.dim)
	label, col := in.filter, pal.fg
	if label == "" {
		label, col = "Find type, name, role…", pal.dim
	}
	if in.filterFocus && in.selectFilter {
		clip.FillRect(Rct(Pt(x+25, y+4), Sz(min(textWidth(dst, face, label), max(r.Size.W-55, 0)), 17)), pal.sel)
	}
	fittedLine(clip, face, label, x+25, y+5, r.Size.W-53, col)
	if in.filterFocus && !in.selectFilter {
		cx := x + 25 + min(textWidth(dst, face, in.filter), max(r.Size.W-55, 0))
		clip.FillRect(Rct(Pt(cx, y+5), Sz(1, 15)), pal.num)
	}
	if in.filter != "" {
		in.tool(clip, face, pal, Rct(Pt(x+r.Size.W-25, y), Sz(25, 25)), inspectClearFilter, false)
	}
}
func (in *inspector) paintTree(dst *Canvas, face text.Face, pal inspectPalette, shown []int, sel int, pointer Point, hasPointer bool) {
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
	in.scroll = clamp(in.scroll, 0, max(in.treeContent-r.Size.H, 0))
	clip := dst.Clip(r)
	if len(shown) == 0 {
		fittedLine(clip, face, "No matching widgets", r.Origin.X+12, r.Origin.Y+16, r.Size.W-24, pal.dim)
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
		row := Rct(Pt(r.Origin.X, y), Sz(r.Size.W, lh))
		fg, dim := pal.key, pal.dim
		if i == sel {
			clip.FillRect(row, pal.sel)
			clip.FillRect(Rct(row.Origin, Sz(2, lh)), pal.num)
			fg, dim = pal.selFg, pal.selFg
		} else if hasPointer && row.Contains(pointer) {
			clip.FillRect(row, pal.hover)
		}
		depth := min(float64(e.Depth)*inspectIndent, max(r.Size.W*.3, 0))
		x := r.Origin.X + 8 + depth
		for d := 0; d < e.Depth && float64(d)*inspectIndent < depth; d++ {
			gx := r.Origin.X + 14 + float64(d)*inspectIndent
			clip.FillRect(Rct(Pt(gx, y), Sz(1, lh)), pal.edge)
		}
		if inspect.HasChildren(in.frame.Nodes, i) {
			triangle(clip, Pt(x+5, y+lh/2), in.folded(e) && in.filter == "", dim)
			in.chips = append(in.chips, inspectChip{rect: Rct(Pt(x-2, y), Sz(15, lh)).Intersect(r), act: inspectCollapse, key: key})
		}
		x += 15
		right := r.Origin.X + r.Size.W - 12
		dims := num(e.Rect.Size.W) + " × " + num(e.Rect.Size.H)
		if r.Size.W >= 320 {
			dw := textWidth(dst, face, dims)
			drawLine(clip, face, dims, right-dw, y+5, dim)
			right -= dw + 12
		}
		name := fitText(dst, face, e.Name, max(right-x, 0))
		drawLine(clip, face, name, x, y+5, fg)
		x += textWidth(dst, face, name) + 7
		if badge := in.frame.Badge(i); badge != "" {
			bw := textWidth(dst, face, badge) + 10
			if x+bw < right {
				clip.FillRoundRect(Rct(Pt(x, y+4), Sz(bw, 15)), 3, pal.chip)
				drawLine(clip, face, badge, x+5, y+5, pal.dim)
				x += bw + 7
			}
		}
		if label := e.Label; label != "" {
			fittedLine(clip, face, strconv.Quote(label), x, y+5, right-x, pal.dim)
		}
	}
	in.treeThumb = inspectScrollbar(dst, r, in.treeContent, in.scroll, pal)
}
func triangle(dst *Canvas, c Point, folded bool, col color.Color) {
	var p Path
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
func inspectScrollbar(dst *Canvas, r Rect, content, scroll float64, pal inspectPalette) Rect {
	if r.Empty() || content <= r.Size.H {
		return Rect{}
	}
	h := min(max(r.Size.H*r.Size.H/content, 18), r.Size.H)
	y := r.Origin.Y + (r.Size.H-h)*scroll/max(content-r.Size.H, 1)
	thumb := Rct(Pt(r.Origin.X+r.Size.W-7, y), Sz(inspectBar, h))
	dst.FillRoundRect(thumb, 2, withAlpha(pal.dim, 110))
	return thumb
}
func (in *inspector) paintCrumbs(dst *Canvas, r Rect, face text.Face, pal inspectPalette, sel int) {
	clip := dst.Clip(r)
	clip.FillRect(r, pal.field)
	clip.FillRect(Rct(r.Origin, Sz(r.Size.W, 1)), pal.edge)
	if sel < 0 {
		return
	}
	nodes := in.frame.Nodes
	chain := inspect.Ancestors(nodes, sel)
	slices.Reverse(chain)
	chain = append(chain, sel)
	total := 0.0
	for _, i := range chain {
		total += textWidth(dst, face, nodes[i].Name) + 18
	}
	x := r.Origin.X + 8 - min(max(total-r.Size.W+8, 0), total)
	for n, i := range chain {
		e := &nodes[i]
		w := textWidth(dst, face, e.Name)
		drawLine(clip, face, e.Name, x, r.Origin.Y+6, pick(i == sel, pal.fg, pal.dim))
		in.chips = append(in.chips, inspectChip{rect: Rct(Pt(x, r.Origin.Y), Sz(w, r.Size.H)).Intersect(r), act: inspectPin, key: keyOf(e)})
		x += w + 6
		if n < len(chain)-1 {
			drawLine(clip, face, "›", x, r.Origin.Y+6, pal.dim)
			x += 12
		}
	}
}
func (in *inspector) tabButton(dst *Canvas, face text.Face, pal inspectPalette, x, y float64, label string, tab inspectTab, on bool) float64 {
	w := textWidth(dst, face, label) + 18
	r := Rct(Pt(x, y), Sz(w, 31))
	if on {
		dst.FillRect(Rct(Pt(x, y+29), Sz(w, 2)), pal.num)
	}
	drawLine(dst, face, label, x+9, y+10, pick(on, pal.num, pal.dim))
	in.chips = append(in.chips, inspectChip{rect: r.Intersect(in.detail), act: inspectSelectTab, tab: tab})
	return x + w
}
func (in *inspector) paintDetail(dst *Canvas, face text.Face, pal inspectPalette, sel int) {
	r := in.detail
	clip := dst.Clip(r)
	clip.FillRect(Rct(r.Origin, Sz(r.Size.W, 32)), pal.field)
	clip.FillRect(Rct(Pt(r.Origin.X, r.Origin.Y+31), Sz(r.Size.W, 1)), pal.edge)
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
		x = in.tabButton(clip, face, pal, x, y, label, tab, tab == shownTab)
	}
	if sel >= 0 && x < r.Origin.X+r.Size.W-28 {
		in.tool(clip, face, pal, Rct(Pt(r.Origin.X+r.Size.W-28, y+3), Sz(25, 25)), inspectCopy, in.copied)
	}
	in.detailBody = Rct(Pt(r.Origin.X, y+32), Sz(r.Size.W, max(r.Size.H-32, 0)))
	if sel < 0 {
		in.detailScroll = 0
		in.detailThumb = Rect{}
		message := "Select a widget to inspect"
		if in.pinned {
			message = "Selected widget is no longer painted"
		}
		fittedLine(clip, face, message, r.Origin.X+12, y+54, r.Size.W-24, pal.dim)
		return
	}
	nodes := in.frame.Nodes
	fields := in.frame.Details(shownTab, sel)
	boxH := 0.0
	if shownTab == inspect.Layout {
		boxH = inspectBoxHeight(in.frame.BoxOf(sel))
	}
	in.detailContent = 32 + boxH + inspectFieldsHeight(fields) + 8
	in.detailScroll = clamp(in.detailScroll, 0, max(in.detailContent-in.detailBody.Size.H, 0))
	content := clip.Clip(in.detailBody)
	cy := in.detailBody.Origin.Y + 10 - in.detailScroll
	fittedLine(content, face, nodes[sel].Name, r.Origin.X+12, cy, r.Size.W-24, pal.fg)
	cy += 24
	if boxH > 0 {
		in.paintBox(content, face, pal, Rct(Pt(r.Origin.X+12, cy), Sz(r.Size.W-24, boxH)), in.frame.BoxOf(sel))
		cy += boxH
	}
	paintInspectFields(content, face, pal, Rct(Pt(r.Origin.X, cy), Sz(r.Size.W, inspectFieldsHeight(fields))), fields)
	in.detailThumb = inspectScrollbar(clip, in.detailBody, in.detailContent, in.detailScroll, pal)
}
func inspectFieldsHeight(fields []inspect.Field) float64 {
	h := 0.0
	for _, f := range fields {
		h += pick(f.Value == "", 28.0, 23.0)
	}
	return h
}
func paintInspectFields(dst *Canvas, face text.Face, pal inspectPalette, r Rect, fields []inspect.Field) {
	y := r.Origin.Y
	kw := min(112.0, r.Size.W*.42)
	for _, f := range fields {
		if f.Value == "" {
			dst.FillRect(Rct(Pt(r.Origin.X, y), Sz(r.Size.W, 25)), pal.field)
			fittedLine(dst, face, f.Key, r.Origin.X+12, y+6, r.Size.W-24, pal.dim)
			y += 28
			continue
		}
		fittedLine(dst, face, f.Key, r.Origin.X+12, y+4, kw-15, pal.key)
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
				dst.FillRoundRect(Rct(Pt(vx, y+5), Sz(11, 11)), 2, c)
				dst.StrokeRoundRect(Rct(Pt(vx, y+5), Sz(11, 11)), 2, 1, pal.edge)
				vx += 17
			}
		}
		fittedLine(dst, face, value, vx, y+4, r.Origin.X+r.Size.W-vx-12, pick(f.Number, pal.num, pal.fg))
		y += 23
	}
}
func inspectBoxHeight(box inspect.Box) float64 {
	if box.Valid {
		return 190
	}
	return 92
}
func (in *inspector) paintBox(dst *Canvas, face text.Face, pal inspectPalette, r Rect, box inspect.Box) {
	if r.Size.W < 60 {
		return
	}
	label := "CONTENT BOUNDS"
	if box.Valid {
		label = "BOX MODEL"
	}
	drawLine(dst, face, label, r.Origin.X, r.Origin.Y, pal.dim)
	outer := Rct(Pt(r.Origin.X, r.Origin.Y+24), Sz(r.Size.W, pick(box.Valid, 132.0, 44.0)))
	if !box.Valid {
		dst.FillRect(outer, pal.content)
		dst.StrokeRoundRect(outer, 0, 1, pal.edge)
		centerText(dst, face, num(box.Content.W)+" × "+num(box.Content.H), outer, pal.fg)
		return
	}
	dst.FillRect(outer, pal.border)
	dst.StrokeRoundRect(outer, 0, 1, pal.edge)
	pad := inset(outer, 17)
	dst.FillRect(pad, pal.padding)
	dst.StrokeRoundRect(pad, 0, 1, withAlpha(pal.dim, 100))
	content := Rct(pad.Origin.Add(Pt(32.0, 28.0)), Sz(max(pad.Size.W-64, 0), max(pad.Size.H-56, 0)))
	dst.FillRect(content, pal.content)
	dst.StrokeRoundRect(content, 0, 1, withAlpha(pal.dim, 100))
	drawLine(dst, face, "border", outer.Origin.X+5, outer.Origin.Y+3, pal.fg)
	centerText(dst, face, num(box.Border), Rct(Pt(outer.Origin.X+outer.Size.W*.5-10, outer.Origin.Y), Sz(20, 17)), pal.fg)
	drawLine(dst, face, "padding", pad.Origin.X+5, pad.Origin.Y+3, pal.fg)
	centerText(dst, face, num(box.Padding.Top), Rct(Pt(content.Origin.X, pad.Origin.Y+13), Sz(content.Size.W, 15)), pal.fg)
	centerText(dst, face, num(box.Padding.Bottom), Rct(Pt(content.Origin.X, content.Origin.Y+content.Size.H), Sz(content.Size.W, 28)), pal.fg)
	centerText(dst, face, num(box.Padding.Left), Rct(Pt(pad.Origin.X, content.Origin.Y), Sz(32, content.Size.H)), pal.fg)
	centerText(dst, face, num(box.Padding.Right), Rct(Pt(content.Origin.X+content.Size.W, content.Origin.Y), Sz(32, content.Size.H)), pal.fg)
	centerText(dst, face, num(box.Content.W)+" × "+num(box.Content.H), content, pal.fg)
	fittedLine(dst, face, "Border paints inside the bounds", r.Origin.X, r.Origin.Y+165, r.Size.W, pal.dim)
}
func centerText(dst *Canvas, face text.Face, s string, r Rect, c color.Color) {
	s = fitText(dst, face, s, r.Size.W-4)
	drawLine(dst, face, s, r.Origin.X+(r.Size.W-textWidth(dst, face, s))/2, r.Origin.Y+(r.Size.H-13)/2, c)
}
func (in *inspector) paintLayout(dst *Canvas, face text.Face, pal inspectPalette, sel int) {
	r := in.layout
	clip := dst.Clip(r)
	clip.FillRect(Rct(r.Origin, Sz(1, r.Size.H)), pal.edge)
	clip.FillRect(Rct(r.Origin, Sz(r.Size.W, 32)), pal.field)
	drawLine(clip, face, "Layout", r.Origin.X+12, r.Origin.Y+10, pal.fg)
	if sel < 0 {
		return
	}
	box := in.frame.BoxOf(sel)
	fields := in.frame.Details(inspect.Layout, sel)
	body := Rct(r.Origin.Add(Pt(0.0, 32.0)), Sz(r.Size.W, max(r.Size.H-32, 0)))
	content := inspectBoxHeight(box) + inspectFieldsHeight(fields) + 24
	in.layoutBody, in.layoutContent = body, content
	in.layoutScroll = clamp(in.layoutScroll, 0, max(content-body.Size.H, 0))
	bc := clip.Clip(body)
	y := body.Origin.Y + 12 - in.layoutScroll
	in.paintBox(bc, face, pal, Rct(Pt(r.Origin.X+12, y), Sz(r.Size.W-24, inspectBoxHeight(box))), box)
	y += inspectBoxHeight(box)
	paintInspectFields(bc, face, pal, Rct(Pt(r.Origin.X, y), Sz(r.Size.W, inspectFieldsHeight(fields))), fields)
	in.layoutThumb = inspectScrollbar(clip, body, content, in.layoutScroll, pal)
}
func (in *inspector) paintHighlight(dst *Canvas, face text.Face, pal inspectPalette, sel int) {
	e := &in.frame.Nodes[sel]
	r := e.Rect
	clip := dst
	if e.Clipped {
		clip = dst.Clip(e.Clip)
	}
	box := in.frame.BoxOf(sel)
	if box.Valid {
		clip.FillRect(r, withAlpha(pal.padding, 100))
		clip.FillRect(Rct(r.Origin.Add(Pt(box.Padding.Left, box.Padding.Top)), box.Content), pal.hi)
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
	w := min(textWidth(dst, face, label)+18, dst.Size().W-12)
	x := clamp(visible.Origin.X, 6, max(dst.Size().W-w-6, 6))
	y := max(6.0, visible.Origin.Y-29)
	badge := Rct(Pt(x, y), Sz(w, 24))
	if !badge.Intersect(in.panel).Empty() {
		return
	}
	dst.FillRoundRect(badge, 4, pal.bg)
	dst.StrokeRoundRect(badge, 4, 1, pal.num)
	fittedLine(dst, face, label, x+9, y+6, w-18, pal.fg)
}
func (in *inspector) paintStatus(dst *Canvas, face text.Face, pal inspectPalette, sel int) {
	r := Rct(Pt(in.panel.Origin.X, in.panel.Origin.Y+in.panel.Size.H-inspectStatusH), Sz(in.panel.Size.W, inspectStatusH))
	dst.FillRect(r, pal.field)
	dst.FillRect(Rct(r.Origin, Sz(r.Size.W, 1)), pal.edge)
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
		status = "Widget details copied"
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
	right := fmt.Sprintf("%.0f fps · %s×", ebiten.ActualFPS(), num(dst.Scale()))
	space := r.Size.W - 16
	if r.Size.W > 600 {
		drawLine(dst, face, right, r.Origin.X+r.Size.W-textWidth(dst, face, right)-10, r.Origin.Y+5, pal.dim)
		space -= textWidth(dst, face, right) + 20
	}
	fittedLine(dst, face, status, r.Origin.X+8, r.Origin.Y+5, space, pal.dim)
}
