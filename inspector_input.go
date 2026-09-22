//go:build ggui_inspector

package ggui

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/ironpark/ggui/inspect"
)

func (in *inspector) input(f frameInput) bool {
	if in.panel.Empty() {
		return false
	}
	leftDown := slices.Contains(f.down, MouseButtonLeft)
	leftUp := slices.Contains(f.up, MouseButtonLeft)
	over := in.panel.Contains(f.pos) || in.edge.Contains(f.pos)
	if in.drag != inspectNoDrag {
		switch in.drag {
		case inspectResizePanel:
			if in.dock == InspectorBottom {
				in.height = in.viewport.H - f.pos.Y
			} else {
				in.width = in.viewport.W - f.pos.X
			}
		case inspectResizeSplit:
			if in.sideBySide() {
				in.split = (f.pos.X - in.panel.Origin.X) / in.panel.Size.W
			} else {
				body := in.body()
				in.split = (f.pos.Y - body.Origin.Y) / max(body.Size.H, 1)
			}
			in.split = clamp(in.split, .25, .75)
		case inspectScrollTree:
			in.scroll = in.dragValue + (f.pos.Y-in.dragStart.Y)*max(in.treeContent-in.tree.Size.H, 0)/max(in.tree.Size.H-in.treeThumb.Size.H, 1)
		case inspectScrollLayout:
			in.layoutScroll = in.dragValue + (f.pos.Y-in.dragStart.Y)*max(in.layoutContent-in.layoutBody.Size.H, 0)/max(in.layoutBody.Size.H-in.layoutThumb.Size.H, 1)
		case inspectScrollDetail:
			in.detailScroll = in.dragValue + (f.pos.Y-in.dragStart.Y)*max(in.detailContent-in.detailBody.Size.H, 0)/max(in.detailBody.Size.H-in.detailThumb.Size.H, 1)
		}
		if leftUp {
			in.drag = inspectNoDrag
			in.capture = false
		}
		return true
	}
	// Picker clicks belong to devtools, including the release after pinning.
	if in.picking && !over && leftDown {
		in.selectEntry(in.frame, inspect.Deepest(in.nodes(), f.pos))
		in.picking = false
		in.capture = true
		in.focus = true
		in.filterFocus = false
		return true
	}
	if in.capture && leftUp {
		in.capture = false
		return true
	}
	if leftDown && !over {
		in.focus, in.filterFocus = false, false
	}
	if leftDown && over {
		in.focus, in.capture = true, true
		in.filterFocus = in.filterRect.Contains(f.pos)
		in.selectFilter = false
		switch {
		case in.edge.Contains(f.pos):
			in.drag = inspectResizePanel
		case in.divider.Contains(f.pos):
			in.drag = inspectResizeSplit
		case in.treeThumb.Contains(f.pos):
			in.drag = inspectScrollTree
			in.dragValue = in.scroll
		case in.layoutThumb.Contains(f.pos):
			in.drag = inspectScrollLayout
			in.dragValue = in.layoutScroll
		case in.detailThumb.Contains(f.pos):
			in.drag = inspectScrollDetail
			in.dragValue = in.detailScroll
		default:
			if i := slices.IndexFunc(in.chips, func(c inspectChip) bool { return c.rect.Contains(f.pos) }); i >= 0 {
				in.act(in.chips[i])
			} else if in.tree.Contains(f.pos) {
				for _, r := range in.rows {
					if f.pos.Y >= r.y && f.pos.Y < r.y+r.h {
						in.selectEntry(in.frame, r.index)
						if len(in.nodes()) == 0 {
							in.sel, in.pinned = r.key, true
						}
						break
					}
				}
			}
		}
		in.dragStart = f.pos
	}
	if over {
		switch {
		case in.detail.Contains(f.pos):
			in.detailScroll -= f.wheel.Y * inspectWheel
		case in.layout.Contains(f.pos):
			in.layoutScroll -= f.wheel.Y * inspectWheel
		case in.tree.Contains(f.pos):
			in.scroll -= f.wheel.Y * inspectWheel
		}
	}
	// Match the familiar devtools shortcuts without requiring pointer focus.
	if f.mods.Cmd() && f.mods.Shift {
		for _, k := range f.keys {
			switch k {
			case KeyC:
				in.act(inspectChip{act: inspectUnpin})
				return true
			case KeyD:
				in.dock = pick(in.dock == InspectorRight, InspectorBottom, InspectorRight)
				return true
			case KeyO:
				in.outlines = !in.outlines
				return true
			}
		}
	}
	// Find is available throughout the open inspector; typing itself only
	// edits the filter after an explicit click or this shortcut.
	if f.mods.Cmd() && slices.Contains(f.keys, KeyF) {
		in.focus, in.filterFocus, in.selectFilter = true, true, true
	}
	if !over && !in.focus && !in.picking && !in.capture {
		return false
	}
	for _, k := range f.keys {
		switch k {
		case KeyEscape:
			if in.filter != "" {
				in.filter = ""
				in.scroll = 0
			} else {
				in.picking, in.pinned, in.filterFocus, in.focus = false, false, false, false
			}
		case KeyA:
			if in.filterFocus && f.mods.Cmd() {
				in.selectFilter = true
			}
		case KeyV:
			if in.filterFocus && f.mods.Cmd() {
				in.typeFilter(currentClipboard().Read())
			}
		case KeyC:
			if f.mods.Cmd() {
				if in.filterFocus && in.selectFilter {
					currentClipboard().Write(in.filter)
				} else if !in.filterFocus {
					in.act(inspectChip{act: inspectCopy})
				}
			}
		case KeyBackspace:
			if in.filterFocus && in.filter != "" {
				if in.selectFilter {
					in.filter = ""
				} else {
					_, n := utf8.DecodeLastRuneInString(in.filter)
					in.filter = in.filter[:len(in.filter)-n]
				}
				in.selectFilter = false
				in.scroll = 0
			}
		case KeyArrowUp:
			in.move--
			in.filterFocus = false
		case KeyArrowDown:
			in.move++
			in.filterFocus = false
		case KeyArrowLeft:
			if !in.filterFocus {
				in.branch = -1
			}
		case KeyArrowRight:
			if !in.filterFocus {
				in.branch = 1
			}
		case KeyHome:
			if !in.filterFocus {
				in.move = -inspectMoveEnd
			}
		case KeyEnd:
			if !in.filterFocus {
				in.move = inspectMoveEnd
			}
		case KeyEnter:
			in.filterFocus = false
			in.focus = true
			in.reveal = true
		case KeyTab:
			in.filterFocus = !in.filterFocus
			in.focus = true
		}
	}
	if in.filterFocus && !f.mods.Cmd() {
		in.typeFilter(f.text)
	}
	return over || in.picking || in.capture || len(f.keys) > 0 || f.text != ""
}

func (in *inspector) typeFilter(s string) {
	s = strings.Map(func(r rune) rune {
		if r < ' ' {
			return -1
		}
		return r
	}, s)
	if s == "" {
		return
	}
	if in.selectFilter {
		in.filter = ""
		in.selectFilter = false
	}
	runes := []rune(in.filter + s)
	in.filter = string(runes[:min(len(runes), 256)])
	in.scroll = 0
}

// Input runs before the next paint, so the frame held is the most recently
// painted one. Serialize only when requested, resolving selection again so
// a click followed by Copy does not copy the previous element.
func (in *inspector) copySelection() {
	fr := in.frame
	if fr == nil {
		return
	}
	sel := in.find(fr)
	if sel < 0 {
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", fr.Nodes[sel].Name)
	for _, tab := range []inspect.Tab{inspect.Layout, inspect.Computed, inspect.Semantics} {
		for _, f := range fr.Details(tab, sel) {
			if f.Value == "" {
				fmt.Fprintf(&b, "\n%s\n", f.Key)
			} else {
				fmt.Fprintf(&b, "%s: %s\n", f.Key, f.Value)
			}
		}
	}
	currentClipboard().Write(b.String())
	in.copied = true
}

func (in *inspector) act(c inspectChip) {
	switch c.act {
	case inspectDockRight:
		in.dock = InspectorRight
	case inspectDockBottom:
		in.dock = InspectorBottom
	case inspectToggleOutlines:
		in.outlines = !in.outlines
	case inspectUnpin:
		in.picking = !in.picking
		in.pinned = false
		in.filterFocus = false
	case inspectPin:
		in.sel, in.pinned, in.reveal = c.key, true, true
		in.detailScroll = 0
	case inspectCollapse:
		if in.collapsed == nil {
			in.collapsed = map[inspectKey]bool{}
		}
		k := foldKey(c.key)
		if in.collapsed[k] {
			delete(in.collapsed, k)
		} else {
			in.collapsed[k] = true
		}
	case inspectClearFilter:
		in.filter = ""
		in.scroll = 0
		in.filterFocus = true
	case inspectSelectTab:
		in.tab = c.tab
		in.detailScroll = 0
		in.copied = false
	case inspectClose:
		in.closed = true
		in.picking, in.filterFocus, in.focus = false, false, false
	case inspectCopy:
		in.copySelection()
	}
}
func (in *inspector) sideBySide() bool { return in.dock == InspectorBottom && in.panel.Size.W >= 640 }
func (in *inspector) cursor(p Point) (CursorShape, bool) {
	switch {
	case in.drag == inspectResizePanel || in.edge.Contains(p):
		return pick(in.dock == InspectorBottom, CursorShapeNSResize, CursorShapeEWResize), true
	case in.drag == inspectResizeSplit || in.divider.Contains(p):
		return pick(in.sideBySide(), CursorShapeEWResize, CursorShapeNSResize), true
	case in.filterRect.Contains(p):
		return CursorShapeText, true
	case in.panel.Contains(p):
		return CursorShapeDefault, true
	case in.picking:
		return CursorShapeCrosshair, true
	default:
		return 0, false
	}
}
