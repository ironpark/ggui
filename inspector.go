package ggui

import (
	"fmt"
	"image/color"
	"math"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// The inspector is a development overlay in the shape of a browser's element
// panel: every widget the frame painted is outlined, the tree of them is
// listed in a panel docked to the right or the bottom, and whichever one is
// selected is described beside it. Moving the pointer selects; clicking a
// row pins the selection so that it survives moving away, which is the only
// way to read anything about a widget that is only there while hovered.
//
// It draws with Canvas primitives and the fallback font rather than with the
// ui package, because ui is built on this one and cannot be imported back.
// Its colors come from the theme, so it reads as part of the app it is
// inspecting in light and dark alike.

// InspectorDock is the edge the inspector's panel is docked to.
type InspectorDock uint8

const (
	InspectorRight  InspectorDock = iota // a column down the right-hand side
	InspectorBottom                      // a strip along the bottom, tree beside details
)

// InspectorOptions configures the inspector from code; the panel's toolbar
// changes the same settings while it is open. The zero value docks right
// with outlines on.
type InspectorOptions struct {
	Dock         InspectorDock
	HideOutlines bool // draw no outlines over the app, only the selection
}

// inspectKey identifies a traced widget from frame to frame. The trace is
// rebuilt from scratch every frame and carries no identity, so a pinned
// selection is found again by what it looked like. Name and depth alone
// would match every row of a list; the Rect is what separates them, and a
// widget that moves is matched by the looser pass in find.
type inspectKey struct {
	name  string
	depth int
	rect  Rect
}

// inspectRow is one line of the tree panel, kept so that the next frame's
// click knows what it landed on.
type inspectRow struct {
	key   inspectKey
	index int
	y, h  float64
}

// inspectAction is what a toolbar chip, a crumb or a disclosure triangle
// does when clicked.
type inspectAction uint8

const (
	inspectDockRight inspectAction = iota
	inspectDockBottom
	inspectToggleOutlines
	inspectUnpin
	inspectPin         // pin the chip's key: a breadcrumb
	inspectCollapse    // fold or unfold the chip's key's children
	inspectClearFilter // empty the filter box
	inspectTabLayout
	inspectTabSemantics
)

// inspectChip is one clickable area of the panel, kept for the next
// frame's click.
type inspectChip struct {
	rect Rect
	act  inspectAction
	key  inspectKey
}

// inspector is what the overlay remembers between frames. The zero value
// follows the pointer, docks right, outlines every widget, folds nothing
// and is scrolled to the top.
type inspector struct {
	sel        inspectKey
	pinned     bool
	scroll     float64
	dock       InspectorDock
	noOutlines bool
	move       int // arrow keys pressed since the last paint, resolved there
	collapsed  map[inspectKey]bool
	filter     string
	tab        inspectAction // inspectTabLayout or inspectTabSemantics

	// Written by paint, read by input on the following frame, the way
	// every other hit region in this library works.
	panel   Rect
	treeTop float64
	rows    []inspectRow
	chips   []inspectChip
}

const (
	inspectPanelMax = 360 // the panel never takes more than this many points
	inspectPad      = 8
	inspectWheel    = 24 // points per wheel notch
	inspectBar      = 4  // scrollbar thumb width
	inspectIndent   = 12 // per depth level in the tree
)

// inspectDepth colors an outline by how deep the widget sits, so that nesting
// reads at a glance.
var inspectDepth = []color.Color{
	color.RGBA{0xe5, 0x39, 0x35, 0xff}, color.RGBA{0xfb, 0x8c, 0x00, 0xff},
	color.RGBA{0x43, 0xa0, 0x47, 0xff}, color.RGBA{0x1e, 0x88, 0xe5, 0xff},
	color.RGBA{0x8e, 0x24, 0xaa, 0xff}, color.RGBA{0x00, 0x89, 0x7b, 0xff},
}

// inspectPalette is the panel's colors for one frame, derived from the
// theme so the panel belongs to the app it sits over.
type inspectPalette struct {
	bg, edge, fg, dim, sel, selFg, hover, chip, hi, key, num, field color.Color
}

func inspectColors() inspectPalette {
	t := theme.Peek()
	or := func(c, fallback color.Color) color.Color {
		if c == nil {
			return fallback
		}
		return c
	}
	card := or(t.Card, or(t.Bg, color.White))
	fg := or(t.Fg, color.Black)
	primary := or(t.Primary, fg)
	dark := luminance(card) < .5
	p := inspectPalette{
		bg:    card,
		edge:  or(t.Border, compositeColor(fg, card, .15)),
		fg:    fg,
		dim:   or(t.MutedFg, compositeColor(fg, card, .6)),
		sel:   primary,
		selFg: or(t.PrimaryFg, card),
		hover: compositeColor(fg, card, .06),
		chip:  compositeColor(fg, card, .08),
		hi:    withAlpha(primary, 0x40),
		field: compositeColor(fg, card, .04),
	}
	if dark {
		p.key, p.num = color.RGBA{0xc5, 0x9c, 0xff, 0xff}, color.RGBA{0x7f, 0xc9, 0xff, 0xff}
	} else {
		p.key, p.num = color.RGBA{0x6f, 0x42, 0xc1, 0xff}, color.RGBA{0x0b, 0x5c, 0xb0, 0xff}
	}
	return p
}

func luminance(c color.Color) float64 {
	n := color.NRGBAModel.Convert(c).(color.NRGBA)
	return (0.2126*float64(n.R) + 0.7152*float64(n.G) + 0.0722*float64(n.B)) / 255
}

func withAlpha(c color.Color, a uint8) color.Color {
	n := color.NRGBAModel.Convert(c).(color.NRGBA)
	n.A = a
	return n
}

// apply takes options set from code.
func (in *inspector) apply(o InspectorOptions) {
	in.dock, in.noOutlines = o.Dock, o.HideOutlines
}

// input takes the frame's pointer and key events while the pointer is over
// the panel, so that scrolling the tree does not also scroll the app
// underneath. It reports whether it consumed them; everywhere else the app
// goes on working normally, which is the point of leaving the inspector on
// while using it.
func (in *inspector) input(f frameInput) bool {
	if in.panel.Size.W == 0 || !in.panel.Contains(f.pos) {
		return false
	}
	// Positive wheel scrolls up, as ScrollWidget reads it.
	in.scroll -= f.wheel.Y * inspectWheel
	for _, b := range f.down {
		if b != ebiten.MouseButtonLeft {
			continue
		}
		if i := slices.IndexFunc(in.chips, func(c inspectChip) bool { return c.rect.Contains(f.pos) }); i >= 0 {
			in.act(in.chips[i])
			continue
		}
		if f.pos.Y < in.treeTop {
			in.pinned = false // the header is the way back to following
			continue
		}
		for _, r := range in.rows {
			if f.pos.Y >= r.y && f.pos.Y < r.y+r.h {
				in.sel, in.pinned = r.key, true
				break
			}
		}
	}
	for _, k := range f.keys {
		switch k {
		case ebiten.KeyArrowUp:
			in.move--
		case ebiten.KeyArrowDown:
			in.move++
		case ebiten.KeyBackspace:
			if in.filter != "" {
				_, n := utf8.DecodeLastRuneInString(in.filter)
				in.filter = in.filter[:len(in.filter)-n]
			}
		case ebiten.KeyEscape:
			if in.filter != "" {
				in.filter = ""
			} else {
				in.pinned = false
			}
		}
	}
	for _, r := range f.text {
		if r >= ' ' {
			in.filter += string(r)
		}
	}
	return true
}

func (in *inspector) act(c inspectChip) {
	switch c.act {
	case inspectDockRight:
		in.dock = InspectorRight
	case inspectDockBottom:
		in.dock = InspectorBottom
	case inspectToggleOutlines:
		in.noOutlines = !in.noOutlines
	case inspectUnpin:
		in.pinned = false
	case inspectPin:
		in.sel, in.pinned = c.key, true
	case inspectCollapse:
		if in.collapsed == nil {
			in.collapsed = map[inspectKey]bool{}
		}
		if in.collapsed[c.key] {
			delete(in.collapsed, c.key)
		} else {
			in.collapsed[c.key] = true
		}
	case inspectClearFilter:
		in.filter = ""
	case inspectTabLayout, inspectTabSemantics:
		in.tab = c.act
	}
}

// find locates the pinned widget in this frame's trace: the same name, depth
// and place if it is still there, and otherwise the first widget of that
// name and depth, so that a selection survives the thing it points at
// moving. It returns -1 once there is no such widget at all.
func (in *inspector) find(trace []traceEntry) int {
	loose := -1
	for i := range trace {
		e := &trace[i]
		if e.name != in.sel.name || e.depth != in.sel.depth {
			continue
		}
		if e.rect == in.sel.rect {
			return i
		}
		if loose < 0 {
			loose = i
		}
	}
	return loose
}

// deepest returns the innermost widget painted under p, which is the one a
// click would reach.
func deepest(trace []traceEntry, p Point) int {
	found := -1
	for i := range trace {
		e := &trace[i]
		if e.rect.Contains(p) && (found < 0 || e.depth >= trace[found].depth) {
			found = i
		}
	}
	return found
}

func keyOf(e *traceEntry) inspectKey { return inspectKey{e.name, e.depth, e.rect} }

// hasChildren reports whether entry i painted anything of its own.
func hasChildren(trace []traceEntry, i int) bool {
	return i+1 < len(trace) && trace[i+1].depth > trace[i].depth
}

// ancestors returns the entries above i, innermost first: the trace is in
// paint order, so each one is the nearest earlier entry one level up.
func ancestors(trace []traceEntry, i int) []int {
	var out []int
	need := trace[i].depth - 1
	for j := i - 1; j >= 0 && need >= 0; j-- {
		if trace[j].depth == need {
			out = append(out, j)
			need--
		}
	}
	return out
}

// visible lists the trace entries the tree shows: everything not inside a
// folded widget, or, while filtering, the matches and what they sit in.
func (in *inspector) visible(trace []traceEntry) []int {
	out := make([]int, 0, len(trace))
	if in.filter != "" {
		keep := make([]bool, len(trace))
		needle := strings.ToLower(in.filter)
		for i := range trace {
			if strings.Contains(strings.ToLower(trace[i].name), needle) {
				keep[i] = true
				for _, a := range ancestors(trace, i) {
					keep[a] = true
				}
			}
		}
		for i, k := range keep {
			if k {
				out = append(out, i)
			}
		}
		return out
	}
	hideBelow := -1
	for i := range trace {
		e := &trace[i]
		if hideBelow >= 0 && e.depth > hideBelow {
			continue
		}
		hideBelow = -1
		out = append(out, i)
		if in.collapsed[keyOf(e)] && hasChildren(trace, i) {
			hideBelow = e.depth
		}
	}
	return out
}

// paint draws the outlines, then the panel: toolbar, filter, tree with its
// breadcrumb, and the details for the selection.
func (in *inspector) paint(dst *Canvas) {
	size := dst.Size()
	if size == (Size{}) || len(dst.trace) == 0 {
		in.panel, in.chips = Rect{}, in.chips[:0]
		return
	}
	pal := inspectColors()
	if !in.noOutlines {
		for i := range dst.trace {
			e := &dst.trace[i]
			dst.StrokeRoundRect(e.rect, 0, 1, inspectDepth[e.depth%len(inspectDepth)])
		}
	}

	face := fallbackFont().face(11 * dst.Scale())
	m := face.Metrics()
	lh := dst.dp(m.HAscent+m.HDescent) + 4
	if in.dock == InspectorBottom {
		h := min(inspectPanelMax, size.H*0.45)
		in.panel = Rct(Pt(0, size.H-h), Sz(size.W, h))
	} else {
		w := min(inspectPanelMax, size.W*0.42)
		in.panel = Rct(Pt(size.W-w, 0), Sz(w, size.H))
	}

	// The pointer picks a widget only while it is over the app; over the
	// panel it is reading, not aiming.
	sel := -1
	pointer, hasPointer := dst.Pointer()
	if hasPointer && !in.panel.Contains(pointer) {
		sel = deepest(dst.trace, pointer)
	}
	if in.pinned {
		if found := in.find(dst.trace); found >= 0 {
			sel = found
		} else {
			in.pinned = false
		}
	}
	// A widget the pointer found is unfolded to, the way an element panel
	// reveals what the picker chose.
	if sel >= 0 && !in.pinned && in.filter == "" {
		for _, a := range ancestors(dst.trace, sel) {
			delete(in.collapsed, keyOf(&dst.trace[a]))
		}
	}
	shown := in.visible(dst.trace)
	// Arrow keys step through the rows as shown, and pin where they land so
	// the pointer does not take it straight back.
	if in.move != 0 && len(shown) > 0 {
		at := slices.Index(shown, sel)
		if at < 0 {
			at = pick(in.move < 0, len(shown), -1)
		}
		sel = shown[clamp(at+in.move, 0, len(shown)-1)]
		in.pinned = true
		in.move = 0
	}
	if sel >= 0 {
		e := &dst.trace[sel]
		dst.FillRect(e.rect, pal.hi)
		dst.StrokeRoundRect(e.rect, 0, 1, pal.sel)
		in.sel = keyOf(e)
	}

	panel := in.panel
	dst.FillRect(panel, pal.bg)
	if in.dock == InspectorBottom {
		dst.FillRect(Rct(panel.Origin, Sz(panel.Size.W, 1)), pal.edge)
	} else {
		dst.FillRect(Rct(panel.Origin, Sz(1, panel.Size.H)), pal.edge)
	}
	in.chips = in.chips[:0]
	y := in.paintToolbar(dst, panel, face, lh, pal)
	y = in.paintFilter(dst, panel, face, lh, pal, y)
	in.treeTop = y
	dst.FillRect(Rct(Pt(panel.Origin.X, y), Sz(panel.Size.W, 1)), pal.edge)

	// Below the filter the tree and the details share the panel: stacked
	// in a right-hand column, side by side along the bottom.
	bottom := panel.Origin.Y + panel.Size.H
	detail := in.details(dst, sel)
	var tree, det Rect
	if in.dock == InspectorBottom {
		tw := math.Round(min(panel.Size.W*0.5, 480))
		tree = Rct(Pt(panel.Origin.X, y), Sz(tw, bottom-y))
		det = Rct(Pt(panel.Origin.X+tw, y), Sz(panel.Size.W-tw, bottom-y))
		dst.FillRect(Rct(Pt(det.Origin.X, y), Sz(1, bottom-y)), pal.edge)
	} else {
		detailH := 0.0
		if sel >= 0 {
			detailH = min(float64(len(detail)+1)*lh+3*inspectPad, panel.Size.H*0.45)
		}
		treeBottom := max(bottom-detailH, y)
		tree = Rct(Pt(panel.Origin.X, y), Sz(panel.Size.W, treeBottom-y))
		det = Rct(Pt(panel.Origin.X, treeBottom), Sz(panel.Size.W, bottom-treeBottom))
		if sel >= 0 {
			dst.FillRect(Rct(Pt(panel.Origin.X, treeBottom), Sz(panel.Size.W, 1)), pal.edge)
		}
	}
	crumbH := lh + 6
	in.paintTree(dst, Rct(tree.Origin, Sz(tree.Size.W, max(tree.Size.H-crumbH, 0))), face, lh, pal, shown, sel, pointer, hasPointer)
	in.paintCrumbs(dst, Rct(Pt(tree.Origin.X, tree.Origin.Y+tree.Size.H-crumbH), Sz(tree.Size.W, crumbH)), face, lh, pal, sel)
	if sel >= 0 {
		in.paintDetail(dst, det, face, lh, pal, detail)
	}
}

// chip draws one toolbar button and remembers where it went.
func (in *inspector) chip(dst *Canvas, face text.Face, lh float64, pal inspectPalette, x, y float64, label string, on bool, act inspectAction, key inspectKey) float64 {
	w := textWidth(dst, face, label) + 12
	r := Rct(Pt(x, y), Sz(w, lh+2))
	dst.FillRoundRect(r, 4, pick(on, pal.sel, pal.chip))
	drawLine(dst, face, label, x+6, y+1, pick(on, pal.selFg, pal.fg))
	in.chips = append(in.chips, inspectChip{rect: r, act: act, key: key})
	return x + w + 4
}

// paintToolbar draws the picker, dock and outline controls with the frame
// cost at the far end, and returns where the next row starts.
func (in *inspector) paintToolbar(dst *Canvas, panel Rect, face text.Face, lh float64, pal inspectPalette) float64 {
	x := panel.Origin.X + inspectPad
	y := panel.Origin.Y + inspectPad
	x = in.chip(dst, face, lh, pal, x, y, pick(in.pinned, "Pinned", "Pick"), !in.pinned, inspectUnpin, inspectKey{})
	x += 6
	x = in.chip(dst, face, lh, pal, x, y, "Right", in.dock == InspectorRight, inspectDockRight, inspectKey{})
	x = in.chip(dst, face, lh, pal, x, y, "Bottom", in.dock == InspectorBottom, inspectDockBottom, inspectKey{})
	x += 6
	in.chip(dst, face, lh, pal, x, y, "Outlines", !in.noOutlines, inspectToggleOutlines, inspectKey{})
	stats := fmt.Sprintf("%d · %.0f fps", len(dst.trace), ebiten.ActualFPS())
	drawLine(dst, face, stats, panel.Origin.X+panel.Size.W-inspectPad-textWidth(dst, face, stats), y+1, pal.dim)
	return y + lh + 2 + inspectPad
}

// paintFilter draws the search box: what has been typed while the pointer
// is over the panel, or a hint when nothing has.
func (in *inspector) paintFilter(dst *Canvas, panel Rect, face text.Face, lh float64, pal inspectPalette, y float64) float64 {
	r := Rct(Pt(panel.Origin.X+inspectPad, y), Sz(panel.Size.W-2*inspectPad, lh+4))
	dst.FillRoundRect(r, 4, pal.field)
	dst.StrokeRoundRect(r, 4, 1, pal.edge)
	if in.filter == "" {
		drawLine(dst, face, "Filter widgets…", r.Origin.X+6, y+2, pal.dim)
	} else {
		drawLine(dst, face, in.filter, r.Origin.X+6, y+2, pal.fg)
		cx := r.Origin.X + r.Size.W - lh - 2
		cr := Rct(Pt(cx, y+2), Sz(lh, lh))
		drawLine(dst, face, "×", cx+(lh-textWidth(dst, face, "×"))/2, y+2, pal.dim)
		in.chips = append(in.chips, inspectChip{rect: cr, act: inspectClearFilter})
	}
	return y + lh + 4 + inspectPad
}

// paintTree lists the shown entries in r, one row per widget indented by
// depth with a disclosure triangle where there are children, with the
// selection followed and the rows it laid out kept for the next click.
func (in *inspector) paintTree(dst *Canvas, r Rect, face text.Face, lh float64, pal inspectPalette, shown []int, sel int, pointer Point, hasPointer bool) {
	content := float64(len(shown)) * lh
	// Follow the selection while it is the pointer's, so the tree keeps up
	// without being dragged.
	if at := slices.Index(shown, sel); at >= 0 && !in.pinned {
		if top := float64(at) * lh; top < in.scroll {
			in.scroll = top
		} else if top+lh > in.scroll+r.Size.H {
			in.scroll = top + lh - r.Size.H
		}
	}
	in.scroll = clamp(in.scroll, 0, max(content-r.Size.H, 0))

	clip := dst.Clip(r)
	x := r.Origin.X + inspectPad
	right := r.Origin.X + r.Size.W - inspectPad - inspectBar
	bottom := r.Origin.Y + r.Size.H
	in.rows = in.rows[:0]
	for at, i := range shown {
		ry := r.Origin.Y + float64(at)*lh - in.scroll
		if ry+lh <= r.Origin.Y || ry >= bottom {
			continue // above or below the window; nothing to draw
		}
		e := &dst.trace[i]
		key := keyOf(e)
		in.rows = append(in.rows, inspectRow{key: key, index: i, y: ry, h: lh})
		row := Rct(Pt(r.Origin.X, ry), Sz(r.Size.W, lh))
		fg, dim := pal.fg, pal.dim
		if i == sel {
			clip.FillRect(row, pal.sel)
			fg, dim = pal.selFg, pal.selFg
		} else if hasPointer && row.Contains(pointer) {
			clip.FillRect(row, pal.hover)
		}
		indent := min(float64(e.depth)*inspectIndent, (right-x)/2)
		tx := x + indent
		if hasChildren(dst.trace, i) {
			folded := in.collapsed[key] && in.filter == ""
			triangle(clip, Pt(tx+5, ry+lh/2), folded, dim)
			in.chips = append(in.chips, inspectChip{rect: Rct(Pt(tx-2, ry), Sz(14, lh)), act: inspectCollapse, key: key})
		}
		tx += 12
		clip.FillCircle(Pt(tx+2, ry+lh/2), 2.5, inspectDepth[e.depth%len(inspectDepth)])
		drawLine(clip, face, e.name, tx+8, ry+2, fg)
		dims := num(e.rect.Size.W) + "×" + num(e.rect.Size.H)
		drawLine(clip, face, dims, right-textWidth(dst, face, dims), ry+2, dim)
	}
	if content > r.Size.H {
		th := max(r.Size.H*r.Size.H/content, 12)
		ty := r.Origin.Y + (r.Size.H-th)*in.scroll/(content-r.Size.H)
		dst.FillRoundRect(Rct(Pt(r.Origin.X+r.Size.W-inspectBar-2, ty), Sz(inspectBar, th)), 2, withAlpha(pal.fg, 0x40))
	}
}

// triangle draws a disclosure marker: pointing right when folded, down
// when open.
func triangle(dst *Canvas, c Point, folded bool, col color.Color) {
	var p vector.Path
	const s = 3.5
	if folded {
		p.MoveTo(dst.Px(c.X-s/2), dst.Px(c.Y-s))
		p.LineTo(dst.Px(c.X+s/2+1), dst.Px(c.Y))
		p.LineTo(dst.Px(c.X-s/2), dst.Px(c.Y+s))
	} else {
		p.MoveTo(dst.Px(c.X-s), dst.Px(c.Y-s/2))
		p.LineTo(dst.Px(c.X+s), dst.Px(c.Y-s/2))
		p.LineTo(dst.Px(c.X), dst.Px(c.Y+s/2+1))
	}
	p.Close()
	vector.FillPath(dst.Image, &p, &vector.FillOptions{}, pathOptions(col))
}

// paintCrumbs draws the selection's ancestry along the bottom of the tree,
// outermost first, each crumb clickable to pin that ancestor.
func (in *inspector) paintCrumbs(dst *Canvas, r Rect, face text.Face, lh float64, pal inspectPalette, sel int) {
	dst.FillRect(Rct(r.Origin, Sz(r.Size.W, 1)), pal.edge)
	if sel < 0 {
		return
	}
	chain := ancestors(dst.trace, sel)
	slices.Reverse(chain)
	chain = append(chain, sel)
	clip := dst.Clip(r)
	x := r.Origin.X + inspectPad
	y := r.Origin.Y + 3
	// Keep the innermost crumbs in view when the chain is wider than the bar.
	total := 0.0
	for _, i := range chain {
		total += textWidth(dst, face, dst.trace[i].name) + textWidth(dst, face, " › ")
	}
	if over := total - (r.Size.W - 2*inspectPad); over > 0 {
		x -= over
	}
	for n, i := range chain {
		e := &dst.trace[i]
		w := textWidth(dst, face, e.name)
		col := pal.dim
		if i == sel {
			col = pal.fg
		}
		drawLine(clip, face, e.name, x, y, col)
		if i != sel {
			in.chips = append(in.chips, inspectChip{rect: Rct(Pt(x, r.Origin.Y), Sz(w, r.Size.H)).Intersect(r), act: inspectPin, key: keyOf(e)})
		}
		x += w
		if n < len(chain)-1 {
			drawLine(clip, face, " › ", x, y, pal.dim)
			x += textWidth(dst, face, " › ")
		}
	}
}

// inspectField is one line of the details: a key and its value, or a
// section heading when the value is empty.
type inspectField struct {
	key, value string
	number     bool // color the value as a number
}

// paintDetail draws the tab bar and the fields under it, keys in one color
// and values aligned in a column after them.
func (in *inspector) paintDetail(dst *Canvas, r Rect, face text.Face, lh float64, pal inspectPalette, fields []inspectField) {
	x := r.Origin.X + inspectPad
	y := r.Origin.Y + inspectPad/2
	x2 := in.chip(dst, face, lh, pal, x, y, "Layout", in.tab != inspectTabSemantics, inspectTabLayout, inspectKey{})
	in.chip(dst, face, lh, pal, x2, y, "Semantics", in.tab == inspectTabSemantics, inspectTabSemantics, inspectKey{})
	y += lh + 2 + inspectPad/2
	clip := dst.Clip(Rct(Pt(r.Origin.X, y), Sz(r.Size.W, r.Origin.Y+r.Size.H-y)))
	keyW := 0.0
	for _, f := range fields {
		if f.value != "" {
			keyW = max(keyW, textWidth(dst, face, f.key))
		}
	}
	keyW = min(keyW, r.Size.W*0.4)
	for _, f := range fields {
		if f.value == "" {
			y += 2
			drawLine(clip, face, strings.ToUpper(f.key), x, y, pal.dim)
			y += lh
			continue
		}
		drawLine(clip, face, f.key, x+inspectPad, y, pal.key)
		drawLine(clip, face, f.value, x+inspectPad+keyW+10, y, pick(f.number, pal.num, pal.fg))
		y += lh
	}
}

// details describes the selection for the current tab: its box and place
// in the tree, or the accessibility node there and everything it sits
// inside, which is what a screen reader walks.
func (in *inspector) details(dst *Canvas, sel int) []inspectField {
	if sel < 0 {
		return nil
	}
	e := &dst.trace[sel]
	if in.tab == inspectTabSemantics {
		return in.semanticFields(dst, e)
	}
	kids := 0
	for i := sel + 1; i < len(dst.trace) && dst.trace[i].depth > e.depth; i++ {
		if dst.trace[i].depth == e.depth+1 {
			kids++
		}
	}
	parent := "—"
	if a := ancestors(dst.trace, sel); len(a) > 0 {
		parent = dst.trace[a[0]].name
	}
	return []inspectField{
		{key: "Box"},
		{"type", e.name, false},
		{"x", num(e.rect.Origin.X), true},
		{"y", num(e.rect.Origin.Y), true},
		{"width", num(e.rect.Size.W), true},
		{"height", num(e.rect.Size.H), true},
		{key: "Tree"},
		{"depth", strconv.Itoa(e.depth), true},
		{"parent", parent, false},
		{"children", strconv.Itoa(kids), true},
	}
}

// semanticFields lists the accessibility node under the widget's center
// and the chain above it.
func (in *inspector) semanticFields(dst *Canvas, e *traceEntry) []inspectField {
	center := Pt(e.rect.Origin.X+e.rect.Size.W/2, e.rect.Origin.Y+e.rect.Size.H/2)
	found := semanticAt(dst, center)
	if found < 0 {
		return []inspectField{{key: "Node"}, {"role", "none", false}}
	}
	n := &dst.sem[found].node
	out := []inspectField{{key: "Node"}, {"role", string(n.Role), false}}
	add := func(k, v string) {
		if v != "" {
			out = append(out, inspectField{k, v, false})
		}
	}
	add("name", n.Name)
	add("description", n.Description)
	add("value", n.Value)
	switch n.Checked {
	case TriOff:
		add("checked", "false")
	case TriOn:
		add("checked", "true")
	case TriMixed:
		add("checked", "mixed")
	}
	if n.Expanded != nil {
		add("expanded", strconv.FormatBool(*n.Expanded))
	}
	if n.Selected {
		add("selected", "true")
	}
	if n.Disabled {
		add("disabled", "true")
	}
	if n.Offscreen {
		add("offscreen", "true")
	}
	if n.Min != 0 || n.Max != 0 || n.Now != 0 {
		out = append(out, inspectField{"range", num(n.Min) + " – " + num(n.Max), true}, inspectField{"now", num(n.Now), true})
	}
	if chain := semanticChain(dst, center); len(chain) > 1 {
		out = append(out, inspectField{key: "Path"})
		for _, c := range chain[:len(chain)-1] {
			out = append(out, inspectField{"", strings.TrimLeft(c, " "), false})
		}
	}
	return out
}

// num formats a length the way a ruler would. Layout arithmetic leaves
// 35.516000000000005 behind and %g prints every digit of it, which buries
// the number that was being read.
func num(v float64) string {
	return strconv.FormatFloat(math.Round(v*100)/100, 'f', -1, 64)
}

func textWidth(dst *Canvas, face text.Face, s string) float64 {
	return dst.dp(lineWidth(s, face))
}

func drawLine(dst *Canvas, face text.Face, s string, x, y float64, col color.Color) {
	op := &text.DrawOptions{}
	op.ColorScale.ScaleWithColor(col)
	op.GeoM.Translate(dst.px(x), dst.px(y))
	drawText(dst.Image, s, face, op)
}

// semanticAt returns the innermost accessibility node painted under p, or
// -1: the last one described there, since parents describe before children.
func semanticAt(dst *Canvas, p Point) int {
	for i, v := range slices.Backward(dst.sem) {
		if !v.node.Offscreen && v.rect.Contains(p) {
			return i
		}
	}
	return -1
}

// semanticChain describes the accessibility node under p and everything it
// is inside of, outermost first and indented, which is what a screen reader
// walks: the single role and label the inspector used to show could not say
// that a tab is inside a strip or an option inside its combobox.
func semanticChain(dst *Canvas, p Point) []string {
	found := semanticAt(dst, p)
	if found < 0 {
		return nil
	}
	var chain []int
	for i := found; i >= 0; i = dst.sem[i].parent - 1 {
		chain = append(chain, i)
	}
	lines := make([]string, 0, len(chain))
	for i, c := range slices.Backward(chain) {
		e := &dst.sem[c]
		n := SemNode{Node: e.node}
		lines = append(lines, fmt.Sprintf("%s%s %q%s",
			strings.Repeat("  ", len(chain)-1-i), e.node.Role, e.node.Name, n.flags()))
	}
	return lines
}
