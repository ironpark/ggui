//go:build ggui_inspector

package ggui

import (
	"image/color"
	"slices"
	"strings"
)

// inspectKey prefers the widget's explicit identity, then its instance and
// structural path. Geometry is only a fallback for traces without a widget.
// inspectorEnabled reports whether this build contains the inspector.
const inspectorEnabled = true

type inspectKey struct {
	name   string
	depth  int
	rect   Rect
	id     any
	path   string
	stable bool
}

func keyOf(e *traceEntry) inspectKey {
	id := e.id
	if id == nil {
		id = inspectComparable(e.widget)
	}
	return inspectKey{name: e.name, depth: e.depth, rect: e.rect, id: id, path: e.path, stable: e.id != nil}
}

// Folding follows identity even when the widget moves or siblings reorder.
func foldKey(k inspectKey) inspectKey {
	if k.id != nil {
		k.depth, k.rect, k.path = 0, Rect{}, ""
	} else if k.path != "" {
		k.depth, k.rect = 0, Rect{}
	}
	return k
}

type inspectRow struct {
	key   inspectKey
	index int
	y, h  float64
}

type inspectAction uint8

const (
	inspectDockRight inspectAction = iota
	inspectDockBottom
	inspectToggleOutlines
	inspectUnpin
	inspectPin
	inspectCollapse
	inspectClearFilter
	inspectSelectTab
	inspectClose
	inspectCopy
)

// inspectTab is a details pane. The zero value is the Layout tab, so a fresh
// inspector needs no special case to show it.
type inspectTab uint8

const (
	inspectTabLayout inspectTab = iota
	inspectTabComputed
	inspectTabSemantics
)

// inspectMoveEnd is a Home/End step: further than any tree can be long,
// clamped to the last visible row.
const inspectMoveEnd = 1 << 30

type inspectChip struct {
	rect Rect
	act  inspectAction
	key  inspectKey
	tab  inspectTab // for inspectSelectTab
}

type inspectDrag uint8

const (
	inspectNoDrag inspectDrag = iota
	inspectResizePanel
	inspectResizeSplit
	inspectScrollTree
	inspectScrollDetail
	inspectScrollLayout
)

type inspector struct {
	cache                              inspectorPanelCache
	visibility                         []inspectVisibility
	visibilityNext                     []inspectVisibility
	matches                            int
	visibilityFiltered                 bool
	sel                                inspectKey
	pinned                             bool
	picking                            bool
	capture                            bool // a picker/panel press owns its release, even outside the panel
	scroll, detailScroll, layoutScroll float64
	dock                               InspectorDock
	outlines                           bool
	move                               int
	branch                             int // left/right tree navigation, resolved against the next trace
	reveal                             bool
	collapsed                          map[inspectKey]bool
	filter                             string
	filterFocus, focus                 bool
	selectFilter                       bool
	tab                                inspectTab
	closed                             bool
	copySource                         *Canvas // borrowed, like lastTrace, until the next paint
	copied                             bool

	width, height, split                                   float64
	drag                                                   inspectDrag
	dragStart                                              Point
	dragValue                                              float64
	layoutThumb, layoutBody                                Rect
	layoutContent                                          float64
	viewport                                               Size
	panel, tree, detail, layout, filterRect, edge, divider Rect
	treeThumb, detailThumb, detailBody                     Rect
	treeContent, detailContent                             float64
	treeTop                                                float64
	rows                                                   []inspectRow
	chips                                                  []inspectChip
	lastTrace                                              []traceEntry // borrowed until the next paint; input runs before paint
	visibleRows, filterParents, filterStack                []int
	filterKeep                                             []bool
}

const (
	inspectPad       = 10
	inspectWheel     = 28
	inspectBar       = 5
	inspectIndent    = 14
	inspectRowHeight = 23
)

var inspectDepth = []color.Color{
	color.NRGBA{0x49, 0x86, 0xe8, 0xff}, color.NRGBA{0xa0, 0x6c, 0xd5, 0xff},
	color.NRGBA{0x24, 0x9c, 0x89, 0xff}, color.NRGBA{0xd8, 0x8b, 0x42, 0xff},
}

type inspectPalette struct {
	bg, edge, fg, dim, sel, selFg, hover, chip, hi, key, num, field color.Color
	padding, border, content                                        color.Color
}

func inspectColors() inspectPalette {
	// Devtools use a quiet neutral surface, independent of an app's accent.
	// This keeps the tree and color-coded measurements readable in every theme.
	if luminance(Untrack(theme.Get).Bg) < .5 {
		return inspectPalette{
			bg: color.NRGBA{27, 29, 34, 255}, edge: color.NRGBA{57, 61, 70, 255}, fg: color.NRGBA{222, 226, 233, 255},
			dim: color.NRGBA{151, 160, 175, 255}, sel: color.NRGBA{43, 70, 104, 255}, selFg: color.NRGBA{221, 236, 255, 255},
			hover: color.NRGBA{37, 41, 49, 255}, chip: color.NRGBA{43, 47, 56, 255}, hi: color.NRGBA{77, 151, 242, 45},
			key: color.NRGBA{195, 158, 238, 255}, num: color.NRGBA{131, 190, 253, 255}, field: color.NRGBA{22, 24, 29, 255},
			padding: color.NRGBA{59, 94, 70, 255}, border: color.NRGBA{119, 91, 52, 255}, content: color.NRGBA{48, 78, 110, 255},
		}
	}
	return inspectPalette{
		bg: color.NRGBA{255, 255, 255, 255}, edge: color.NRGBA{220, 224, 231, 255}, fg: color.NRGBA{40, 46, 57, 255},
		dim: color.NRGBA{112, 120, 134, 255}, sel: color.NRGBA{226, 239, 255, 255}, selFg: color.NRGBA{30, 80, 147, 255},
		hover: color.NRGBA{244, 247, 251, 255}, chip: color.NRGBA{237, 240, 245, 255}, hi: color.NRGBA{66, 139, 233, 40},
		key: color.NRGBA{134, 68, 160, 255}, num: color.NRGBA{30, 101, 187, 255}, field: color.NRGBA{247, 248, 251, 255},
		padding: color.NRGBA{218, 239, 214, 255}, border: color.NRGBA{247, 222, 182, 255}, content: color.NRGBA{211, 232, 254, 255},
	}
}

func luminance(c color.Color) float64 {
	if c == nil {
		return 1
	}
	n := color.NRGBAModel.Convert(c).(color.NRGBA)
	return (.2126*float64(n.R) + .7152*float64(n.G) + .0722*float64(n.B)) / 255
}
func withAlpha(c color.Color, a uint8) color.Color {
	n := color.NRGBAModel.Convert(c).(color.NRGBA)
	n.A = a
	return n
}
func (in *inspector) apply(o InspectorOptions) {
	in.dock, in.outlines = o.Dock, o.ShowOutlines
}

// reset forgets the session when the inspector closes: borrowed frame data,
// selection, folds, scratch buffers and the panel image. Docking, outlines,
// sizes and the chosen tab persist to the next open.
func (in *inspector) reset() {
	in.cache.release()
	*in = inspector{dock: in.dock, outlines: in.outlines, width: in.width, height: in.height, split: in.split, tab: in.tab}
}

func (in *inspector) find(tr []traceEntry) int {
	if in.sel.id != nil {
		for i := range tr {
			k := keyOf(&tr[i])
			if k.name == in.sel.name && k.id == in.sel.id {
				return i
			}
		}
		if in.sel.stable {
			return -1
		} // a removed keyed row must not select its neighbour
	}
	if in.sel.path != "" {
		for i := range tr {
			if tr[i].path == in.sel.path && tr[i].name == in.sel.name {
				return i
			}
		}
		return -1
	}
	loose := -1
	for i := range tr {
		e := &tr[i]
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

func deepest(tr []traceEntry, p Point) int {
	found := -1
	for i := range tr {
		e := &tr[i]
		if e.rect.Contains(p) && (!e.clipped || e.clip.Contains(p)) {
			found = i
		}
	}
	return found
}
func hasChildren(tr []traceEntry, i int) bool { return i+1 < len(tr) && tr[i+1].depth > tr[i].depth }
func ancestors(tr []traceEntry, i int) []int {
	var out []int
	need := tr[i].depth - 1
	for j := i - 1; j >= 0 && need >= 0; j-- {
		if tr[j].depth == need {
			out = append(out, j)
			need--
		}
	}
	return out
}
func (in *inspector) folded(e *traceEntry) bool { return in.collapsed[foldKey(keyOf(e))] }

// inspectMatches expects a lower-cased filter.
func inspectMatches(e *traceEntry, filter string) bool {
	return strings.Contains(strings.ToLower(e.name+" "+inspectLabel(e)+" "+string(nodeOf(e.widget).Role)), filter)
}

// inspectFoldLimit bounds the collapsed map: folds of widgets that were not
// painted this frame are dropped once the map outgrows it.
const inspectFoldLimit = 128

// visible returns scratch storage valid until the next call.
func (in *inspector) visible(tr []traceEntry) []int {
	in.visibilityNext = in.visibilityNext[:0]
	in.matches = 0
	filter, folds := strings.ToLower(in.filter), 0
	for i := range tr {
		match := filter != "" && inspectMatches(&tr[i], filter)
		if match {
			in.matches++
		}
		folded := in.folded(&tr[i])
		if folded {
			folds++
		}
		in.visibilityNext = append(in.visibilityNext, inspectVisibility{tr[i].depth, folded, match})
	}
	if slices.Equal(in.visibility, in.visibilityNext) && in.visibility != nil && in.visibilityFiltered == (in.filter != "") {
		return in.visibleRows
	}
	in.visibility, in.visibilityNext = in.visibilityNext, in.visibility
	in.visibilityFiltered = in.filter != ""
	if len(in.collapsed) > inspectFoldLimit && folds < len(in.collapsed) {
		in.pruneFolds(tr)
	}
	out := in.visibleRows[:0]
	defer func() { in.visibleRows = out }()
	if in.filter != "" {
		// A reverse pass propagates matches to parents in linear time, even
		// for deeply nested trees whose every name matches the filter.
		in.filterKeep = resize(in.filterKeep, len(tr))
		in.filterParents = resize(in.filterParents, len(tr))
		keep, parents := in.filterKeep, in.filterParents
		stack := in.filterStack[:0]
		defer func() { in.filterStack = stack }()
		for i, e := range tr {
			for len(stack) > 0 && tr[stack[len(stack)-1]].depth >= e.depth {
				stack = stack[:len(stack)-1]
			}
			parents[i] = -1
			if len(stack) > 0 {
				parents[i] = stack[len(stack)-1]
			}
			stack = append(stack, i)
			keep[i] = in.visibility[i].match
		}
		for i := len(tr) - 1; i >= 0; i-- {
			if keep[i] && parents[i] >= 0 {
				keep[parents[i]] = true
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
	for i := range tr {
		e := &tr[i]
		if hideBelow >= 0 && e.depth > hideBelow {
			continue
		}
		hideBelow = -1
		out = append(out, i)
		if in.folded(e) && hasChildren(tr, i) {
			hideBelow = e.depth
		}
	}
	return out
}

// pruneFolds keeps only folds of widgets in tr. Fold keys hold widget
// instances, so a long session would otherwise pin every rebuilt widget.
func (in *inspector) pruneFolds(tr []traceEntry) {
	kept := make(map[inspectKey]bool, len(in.collapsed))
	for i := range tr {
		if k := foldKey(keyOf(&tr[i])); in.collapsed[k] {
			kept[k] = true
		}
	}
	in.collapsed = kept
}

func (in *inspector) selectEntry(tr []traceEntry, i int) {
	if i < 0 || i >= len(tr) {
		return
	}
	in.sel, in.pinned, in.reveal = keyOf(&tr[i]), true, true
	in.detailScroll, in.copied = 0, false
	for _, a := range ancestors(tr, i) {
		delete(in.collapsed, foldKey(keyOf(&tr[a])))
	}
}
func (in *inspector) selection(dst *Canvas) (int, []int) {
	tr := dst.frameTrace()
	sel := in.find(tr)
	pointer, hasPointer := dst.Pointer()
	if !in.pinned && hasPointer && !in.panel.Contains(pointer) {
		if hit := deepest(tr, pointer); hit >= 0 {
			sel = hit
		}
	}
	if sel >= 0 && !in.pinned && in.filter == "" {
		for _, a := range ancestors(tr, sel) {
			delete(in.collapsed, foldKey(keyOf(&tr[a])))
		}
	}
	if in.branch != 0 && sel >= 0 {
		if in.branch < 0 {
			if hasChildren(tr, sel) && !in.folded(&tr[sel]) {
				in.act(inspectChip{act: inspectCollapse, key: keyOf(&tr[sel])})
			} else if a := ancestors(tr, sel); len(a) > 0 {
				sel = a[0]
			}
		} else if hasChildren(tr, sel) {
			if in.folded(&tr[sel]) {
				delete(in.collapsed, foldKey(keyOf(&tr[sel])))
			} else {
				sel++
			}
		}
		in.selectEntry(tr, sel)
	}
	in.branch = 0
	shown := in.visible(tr)
	if in.move != 0 && len(shown) > 0 {
		at := slices.Index(shown, sel)
		if at < 0 {
			at = pick(in.move < 0, len(shown), -1)
		}
		sel = shown[clamp(at+in.move, 0, len(shown)-1)]
		in.selectEntry(tr, sel)
	}
	in.move = 0
	if sel >= 0 {
		next := keyOf(&tr[sel])
		if foldKey(next) != foldKey(in.sel) {
			in.reveal = true
			in.detailScroll, in.layoutScroll = 0, 0
			in.copied = false
		}
		in.sel = next
	}
	return sel, shown
}
