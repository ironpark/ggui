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
// listed down the right-hand side, and whichever one is selected is
// described underneath. Moving the pointer selects; clicking a row pins the
// selection so that it survives moving away, which is the only way to read
// anything about a widget that is only there while hovered.
//
// It draws with Canvas primitives and the fallback font rather than with the
// ui package, because ui is built on this one and cannot be imported back.

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

// inspector is what the overlay remembers between frames. The zero value
// follows the pointer and is scrolled to the top.
type inspector struct {
	sel    inspectKey
	pinned bool
	scroll float64

	// Written by paint, read by input on the following frame, the way
	// every other hit region in this library works.
	panel   Rect
	treeTop float64
	rows    []inspectRow
}

const (
	inspectPanelMax = 340 // the panel never takes more than this many points
	inspectPad      = 6
	inspectWheel    = 24 // points per wheel notch
)

// inspectDepth colors an outline by how deep the widget sits, so that nesting
// reads at a glance.
var inspectDepth = []color.Color{
	color.RGBA{0xe5, 0x39, 0x35, 0xff}, color.RGBA{0xfb, 0x8c, 0x00, 0xff},
	color.RGBA{0x43, 0xa0, 0x47, 0xff}, color.RGBA{0x1e, 0x88, 0xe5, 0xff},
	color.RGBA{0x8e, 0x24, 0xaa, 0xff}, color.RGBA{0x00, 0x89, 0x7b, 0xff},
}

var (
	inspectPanelBg = color.RGBA{0x1f, 0x23, 0x28, 0xf2}
	inspectRule    = color.RGBA{0x3a, 0x40, 0x48, 0xff}
	inspectFg      = color.RGBA{0xe6, 0xe9, 0xed, 0xff}
	inspectDim     = color.RGBA{0x9a, 0xa3, 0xad, 0xff}
	inspectSelBg   = color.RGBA{0x1e, 0x88, 0xe5, 0xff}
	inspectHiFill  = color.RGBA{0x1e, 0x88, 0xe5, 0x40}
)

// input takes the frame's pointer events while they are over the panel, so
// that scrolling the tree does not also scroll the app underneath. It
// reports whether it consumed them; everywhere else the app goes on working
// normally, which is the point of leaving the inspector on while using it.
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
	return true
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

// paint draws the outlines, the tree panel and the details for the selection.
func (in *inspector) paint(dst *Canvas) {
	size := dst.Size()
	if size == (Size{}) || len(dst.trace) == 0 {
		in.panel = Rect{}
		return
	}
	for i := range dst.trace {
		e := &dst.trace[i]
		dst.StrokeRoundRect(e.rect, 0, 1, inspectDepth[e.depth%len(inspectDepth)])
	}

	face := fallbackFont().face(11 * dst.Scale())
	m := face.Metrics()
	lh := dst.dp(m.HAscent+m.HDescent) + 3
	width := min(inspectPanelMax, size.W*0.42)
	in.panel = Rct(Pt(size.W-width, 0), Sz(width, size.H))

	// The pointer picks a widget only while it is over the app; over the
	// panel it is reading, not aiming.
	sel := -1
	if p, ok := dst.Pointer(); ok && !in.panel.Contains(p) {
		sel = deepest(dst.trace, p)
	}
	if in.pinned {
		if found := in.find(dst.trace); found >= 0 {
			sel = found
		} else {
			in.pinned = false
		}
	}
	if sel >= 0 {
		e := &dst.trace[sel]
		dst.FillRect(e.rect, inspectHiFill)
		if !in.pinned {
			in.sel = inspectKey{e.name, e.depth, e.rect}
		}
	}

	detail := in.details(dst, sel)
	dst.FillRect(in.panel, inspectPanelBg)
	x := in.panel.Origin.X + inspectPad
	textW := width - 2*inspectPad

	// Header: what this is, and what the frame cost.
	y := float64(inspectPad)
	head := "Elements"
	if in.pinned {
		head = "Elements · pinned, click here to follow"
	}
	drawLine(dst, face, head, x, y, inspectFg)
	y += lh
	drawLine(dst, face, fmt.Sprintf("%d widgets   %.0f fps", len(dst.trace), ebiten.ActualFPS()), x, y, inspectDim)
	y += lh + inspectPad
	in.treeTop = y
	rule(dst, in.panel, y)

	detailH := float64(len(detail))*lh + 2*inspectPad
	treeBottom := max(size.H-detailH, y)
	treeH := treeBottom - y
	content := float64(len(dst.trace)) * lh

	// Follow the selection while it is the pointer's, so the tree keeps up
	// without being dragged.
	if sel >= 0 && !in.pinned {
		if top := float64(sel) * lh; top < in.scroll {
			in.scroll = top
		} else if top+lh > in.scroll+treeH {
			in.scroll = top + lh - treeH
		}
	}
	in.scroll = clamp(in.scroll, 0, max(content-treeH, 0))

	in.rows = in.rows[:0]
	for i := range dst.trace {
		ry := y + float64(i)*lh - in.scroll
		if ry+lh <= y || ry >= treeBottom {
			continue // above or below the window; nothing to draw
		}
		e := &dst.trace[i]
		key := inspectKey{e.name, e.depth, e.rect}
		in.rows = append(in.rows, inspectRow{key: key, index: i, y: ry, h: lh})
		var col color.Color = inspectFg
		if i == sel {
			dst.FillRect(Rct(Pt(in.panel.Origin.X, ry), Sz(width, lh)), inspectSelBg)
			col = color.White
		}
		indent := min(float64(e.depth)*8, textW/2)
		drawLine(dst, face, e.name, x+indent, ry, col)
		dims := num(e.rect.Size.W) + "×" + num(e.rect.Size.H)
		drawLine(dst, face, dims, in.panel.Origin.X+width-inspectPad-dst.dp(text.Advance(dims, face)), ry, pick[color.Color](i == sel, color.White, inspectDim))
	}

	if len(detail) == 0 {
		return
	}
	rule(dst, in.panel, treeBottom)
	y = treeBottom + inspectPad
	for _, line := range detail {
		drawLine(dst, face, line, x, y, pick[color.Color](strings.HasPrefix(line, " "), inspectDim, inspectFg))
		y += lh
	}
}

// details describes the selection the way the panel's lower half reads: what
// it is and where, then the accessibility node there with everything it sits
// inside, which is what a screen reader walks.
func (in *inspector) details(dst *Canvas, sel int) []string {
	if sel < 0 {
		return nil
	}
	e := &dst.trace[sel]
	lines := []string{
		e.name,
		fmt.Sprintf(" %s×%s at %s,%s   depth %d", num(e.rect.Size.W), num(e.rect.Size.H), num(e.rect.Origin.X), num(e.rect.Origin.Y), e.depth),
	}
	center := Pt(e.rect.Origin.X+e.rect.Size.W/2, e.rect.Origin.Y+e.rect.Size.H/2)
	if chain := semanticChain(dst, center); len(chain) > 0 {
		lines = append(lines, "")
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

func drawLine(dst *Canvas, face text.Face, s string, x, y float64, col color.Color) {
	op := &text.DrawOptions{}
	op.ColorScale.ScaleWithColor(col)
	op.GeoM.Translate(dst.px(x), dst.px(y))
	text.Draw(dst.Image, s, face, op)
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
