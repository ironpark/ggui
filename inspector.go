package ggui

import (
	"fmt"
	"image/color"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
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

// inspectAction is what a toolbar chip does when clicked.
type inspectAction uint8

const (
	inspectDockRight inspectAction = iota
	inspectDockBottom
	inspectToggleOutlines
	inspectUnpin
)

// inspectChip is one toolbar button, kept for the next frame's click.
type inspectChip struct {
	rect Rect
	act  inspectAction
}

// inspector is what the overlay remembers between frames. The zero value
// follows the pointer, docks right, outlines every widget and is scrolled
// to the top.
type inspector struct {
	sel        inspectKey
	pinned     bool
	scroll     float64
	dock       InspectorDock
	noOutlines bool
	move       int // arrow keys pressed since the last paint, resolved there

	// Written by paint, read by input on the following frame, the way
	// every other hit region in this library works.
	panel   Rect
	treeTop float64
	rows    []inspectRow
	chips   []inspectChip
}

const (
	inspectPanelMax = 340 // the panel never takes more than this many points
	inspectPad      = 8
	inspectWheel    = 24 // points per wheel notch
	inspectBar      = 4  // scrollbar thumb width
)

// inspectDepth colors an outline by how deep the widget sits, so that nesting
// reads at a glance.
var inspectDepth = []color.Color{
	color.RGBA{0xe5, 0x39, 0x35, 0xff}, color.RGBA{0xfb, 0x8c, 0x00, 0xff},
	color.RGBA{0x43, 0xa0, 0x47, 0xff}, color.RGBA{0x1e, 0x88, 0xe5, 0xff},
	color.RGBA{0x8e, 0x24, 0xaa, 0xff}, color.RGBA{0x00, 0x89, 0x7b, 0xff},
}

var (
	inspectPanelBg = color.RGBA{0x1b, 0x1e, 0x23, 0xf4}
	inspectEdge    = color.RGBA{0x4a, 0x50, 0x5a, 0xff}
	inspectRule    = color.RGBA{0x30, 0x35, 0x3d, 0xff}
	inspectFg      = color.RGBA{0xe6, 0xe9, 0xed, 0xff}
	inspectDim     = color.RGBA{0x8b, 0x94, 0x9e, 0xff}
	inspectSelBg   = color.RGBA{0x1e, 0x88, 0xe5, 0xff}
	inspectHoverBg = color.RGBA{0xff, 0xff, 0xff, 0x12}
	inspectChipBg  = color.RGBA{0x2a, 0x2f, 0x37, 0xff}
	inspectHiFill  = color.RGBA{0x1e, 0x88, 0xe5, 0x40}
	inspectThumb   = color.RGBA{0xff, 0xff, 0xff, 0x40}
)

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
			in.act(in.chips[i].act)
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
		case ebiten.KeyEscape:
			in.pinned = false
		}
	}
	return true
}

func (in *inspector) act(a inspectAction) {
	switch a {
	case inspectDockRight:
		in.dock = InspectorRight
	case inspectDockBottom:
		in.dock = InspectorBottom
	case inspectToggleOutlines:
		in.noOutlines = !in.noOutlines
	case inspectUnpin:
		in.pinned = false
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

// paint draws the outlines, the panel with its toolbar, the tree and the
// details for the selection.
func (in *inspector) paint(dst *Canvas) {
	size := dst.Size()
	if size == (Size{}) || len(dst.trace) == 0 {
		in.panel, in.chips = Rect{}, in.chips[:0]
		return
	}
	if !in.noOutlines {
		for i := range dst.trace {
			e := &dst.trace[i]
			dst.StrokeRoundRect(e.rect, 0, 1, inspectDepth[e.depth%len(inspectDepth)])
		}
	}

	face := fallbackFont().face(11 * dst.Scale())
	m := face.Metrics()
	lh := dst.dp(m.HAscent+m.HDescent) + 3
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
	// Arrow keys step through the tree from wherever the selection is, and
	// pin where they land so the pointer does not take it straight back.
	if in.move != 0 {
		if sel < 0 {
			sel = pick(in.move < 0, len(dst.trace), -1)
		}
		sel = clamp(sel+in.move, 0, len(dst.trace)-1)
		in.pinned = true
		in.move = 0
	}
	if sel >= 0 {
		e := &dst.trace[sel]
		dst.FillRect(e.rect, inspectHiFill)
		dst.StrokeRoundRect(e.rect, 0, 1, inspectSelBg)
		in.sel = inspectKey{e.name, e.depth, e.rect}
	}

	detail := in.details(dst, sel)
	panel := in.panel
	dst.FillRect(panel, inspectPanelBg)
	if in.dock == InspectorBottom {
		dst.FillRect(Rct(panel.Origin, Sz(panel.Size.W, 1)), inspectEdge)
	} else {
		dst.FillRect(Rct(panel.Origin, Sz(1, panel.Size.H)), inspectEdge)
	}
	x := panel.Origin.X + inspectPad
	right := panel.Origin.X + panel.Size.W - inspectPad

	// Header: what this is and what the frame cost, then the toolbar.
	y := panel.Origin.Y + inspectPad
	drawLine(dst, face, "Inspector", x, y, inspectFg)
	stats := fmt.Sprintf("%d widgets · %.0f fps", len(dst.trace), ebiten.ActualFPS())
	drawLine(dst, face, stats, right-textWidth(dst, face, stats), y, inspectDim)
	y += lh + 4
	in.chips = in.chips[:0]
	cx := x
	chip := func(label string, on bool, act inspectAction) {
		w := textWidth(dst, face, label) + 12
		r := Rct(Pt(cx, y), Sz(w, lh+2))
		dst.FillRoundRect(r, 4, pick[color.Color](on, inspectSelBg, inspectChipBg))
		drawLine(dst, face, label, cx+6, y+1, pick[color.Color](on, color.White, inspectFg))
		in.chips = append(in.chips, inspectChip{rect: r, act: act})
		cx += w + 4
	}
	chip("Right", in.dock == InspectorRight, inspectDockRight)
	chip("Bottom", in.dock == InspectorBottom, inspectDockBottom)
	chip("Outlines", !in.noOutlines, inspectToggleOutlines)
	if in.pinned {
		chip("Pinned · follow", true, inspectUnpin)
	}
	y += lh + 2 + inspectPad
	in.treeTop = y
	rule(dst, panel, y)

	// Below the toolbar the tree and the details share the panel: stacked
	// in a right-hand column, side by side along the bottom.
	bottom := panel.Origin.Y + panel.Size.H
	var tree, det Rect
	if in.dock == InspectorBottom {
		tw := math.Round(min(panel.Size.W*0.55, 480))
		tree = Rct(Pt(panel.Origin.X, y), Sz(tw, bottom-y))
		det = Rct(Pt(panel.Origin.X+tw, y), Sz(panel.Size.W-tw, bottom-y))
		if len(detail) > 0 {
			dst.FillRect(Rct(Pt(det.Origin.X, y), Sz(1, bottom-y)), inspectRule)
		}
	} else {
		detailH := 0.0
		if len(detail) > 0 {
			detailH = float64(len(detail))*lh + 2*inspectPad
		}
		treeBottom := max(bottom-detailH, y)
		tree = Rct(Pt(panel.Origin.X, y), Sz(panel.Size.W, treeBottom-y))
		det = Rct(Pt(panel.Origin.X, treeBottom), Sz(panel.Size.W, bottom-treeBottom))
		if len(detail) > 0 {
			rule(dst, panel, treeBottom)
		}
	}
	in.paintTree(dst, tree, face, lh, sel, pointer, hasPointer)
	if len(detail) > 0 {
		in.paintDetail(dst, det, face, lh, detail)
	}
}

// paintTree lists the trace in r, one row per widget indented by depth, with
// the selection followed and the rows it laid out kept for the next click.
func (in *inspector) paintTree(dst *Canvas, r Rect, face text.Face, lh float64, sel int, pointer Point, hasPointer bool) {
	content := float64(len(dst.trace)) * lh
	// Follow the selection while it is the pointer's, so the tree keeps up
	// without being dragged.
	if sel >= 0 && !in.pinned {
		if top := float64(sel) * lh; top < in.scroll {
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
	for i := range dst.trace {
		ry := r.Origin.Y + float64(i)*lh - in.scroll
		if ry+lh <= r.Origin.Y || ry >= bottom {
			continue // above or below the window; nothing to draw
		}
		e := &dst.trace[i]
		key := inspectKey{e.name, e.depth, e.rect}
		in.rows = append(in.rows, inspectRow{key: key, index: i, y: ry, h: lh})
		row := Rct(Pt(r.Origin.X, ry), Sz(r.Size.W, lh))
		var col, dim color.Color = inspectFg, inspectDim
		if i == sel {
			clip.FillRect(row, inspectSelBg)
			col, dim = color.White, color.White
		} else if hasPointer && row.Contains(pointer) {
			clip.FillRect(row, inspectHoverBg)
		}
		indent := min(float64(e.depth)*10, (right-x)/2)
		// A dot in the depth's outline color ties the row to its outline.
		clip.FillCircle(Pt(x+indent+3, ry+lh/2), 2.5, inspectDepth[e.depth%len(inspectDepth)])
		drawLine(clip, face, e.name, x+indent+10, ry, col)
		dims := num(e.rect.Size.W) + "×" + num(e.rect.Size.H)
		drawLine(clip, face, dims, right-textWidth(dst, face, dims), ry, dim)
	}
	if content > r.Size.H {
		track := r.Size.H
		th := max(track*r.Size.H/content, 12)
		ty := r.Origin.Y + (track-th)*in.scroll/(content-r.Size.H)
		dst.FillRoundRect(Rct(Pt(r.Origin.X+r.Size.W-inspectBar-2, ty), Sz(inspectBar, th)), 2, inspectThumb)
	}
}

// paintDetail writes the selection's description in r; lines that begin
// with a space are the secondary ones.
func (in *inspector) paintDetail(dst *Canvas, r Rect, face text.Face, lh float64, detail []string) {
	clip := dst.Clip(r)
	x := r.Origin.X + inspectPad
	y := r.Origin.Y + inspectPad
	for _, line := range detail {
		drawLine(clip, face, line, x, y, pick[color.Color](strings.HasPrefix(line, " "), inspectDim, inspectFg))
		y += lh
	}
}

// details describes the selection the way the panel's detail area reads:
// what it is and where, then the accessibility node there with everything
// it sits inside, which is what a screen reader walks.
func (in *inspector) details(dst *Canvas, sel int) []string {
	if sel < 0 {
		return nil
	}
	e := &dst.trace[sel]
	lines := []string{
		e.name,
		fmt.Sprintf(" size %s×%s   at %s,%s   depth %d", num(e.rect.Size.W), num(e.rect.Size.H), num(e.rect.Origin.X), num(e.rect.Origin.Y), e.depth),
	}
	center := Pt(e.rect.Origin.X+e.rect.Size.W/2, e.rect.Origin.Y+e.rect.Size.H/2)
	if chain := semanticChain(dst, center); len(chain) > 0 {
		lines = append(lines, "", "Semantics")
		for _, c := range chain {
			lines = append(lines, " "+c)
		}
	}
	return lines
}

// num formats a length the way a ruler would. Layout arithmetic leaves
// 35.516000000000005 behind and %g prints every digit of it, which buries
// the number that was being read.
func num(v float64) string {
	return strconv.FormatFloat(math.Round(v*100)/100, 'f', -1, 64)
}

func rule(dst *Canvas, panel Rect, y float64) {
	dst.FillRect(Rct(Pt(panel.Origin.X, y), Sz(panel.Size.W, 1)), inspectRule)
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

// semanticChain describes the accessibility node under p and everything it
// is inside of, outermost first and indented, which is what a screen reader
// walks: the single role and label the inspector used to show could not say
// that a tab is inside a strip or an option inside its combobox.
func semanticChain(dst *Canvas, p Point) []string {
	found := -1
	for i, v := range slices.Backward(dst.sem) {
		if !v.node.Offscreen && v.rect.Contains(p) {
			found = i
			break
		}
	}
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
