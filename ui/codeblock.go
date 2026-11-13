package ui

import (
	"image/color"
	"maps"
	"strconv"
	"strings"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/ui/icons"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// CodeLineStyle decorates a logical source line without changing its contents.
type CodeLineStyle uint8

// The decorations of a source line.
const (
	CodeLineNormal      CodeLineStyle = iota // no decoration
	CodeLineHighlighted                      // a muted band behind the line
	CodeLineAdded                            // a muted band and a + before the line
	CodeLineRemoved                          // a destructive band and a − before the line
)

// codeCopiedFor is how long the copy action shows that it copied.
const codeCopiedFor = 2 * time.Second

// CodeBlockWidget presents selectable code with a copy action, optional wrapping
// and line decorations, and scrolling along both axes. It is a bordered surface
// with no fill of its own on a light page. A Header or Language gives it a
// header row holding the copy action; without one the action floats over the
// top end corner and shows while the pointer or focus is in the block. Build
// one with CodeBlock.
type CodeBlockWidget struct {
	ggui.Interactive
	wrap                     bool
	rtl                      bool
	viewportW                float64
	lineStyles               map[int]CodeLineStyle
	lineStylesGen            int // counts LineStyles calls, for the body to tell one map from the next
	startLine                int
	xController, yController ggui.ScrollController
	props                    property.Owner
	code                     property.Value[string]
	language, header         string
	lineNumbers, copyButton  bool
	destructive, small       bool
	height                   float64
	onCopy                   func(string)
	footer                   ggui.Widget
	text                     *ggui.RichTextWidget
	title, lang              *ggui.TextWidget
	glyph                    *codeCopyGlyph
	copy                     *ButtonWidget
	xScroll, yScroll         *ggui.ScrollWidget
	body                     *codeBlockBody
	theme                    *uitheme.Theme
	motion                   time.Duration
	codeStyle                ggui.TextStyle
	// The header row, the code's padding and the footer, as last laid out.
	hasHeader                       bool
	pad, padTop, padBottom, headerH float64
	titleSize, langSize, copySize   ggui.Size
	bodySize, footerSize            ggui.Size
	laidCode                        string
	// Where the code was last painted, and paintBody as a value, so that
	// painting it behind a fade makes no closure a frame.
	viewport    ggui.Rect
	paintBodyFn func(*ggui.Canvas)
	copied      bool
	copiedAt    time.Time
}

// CodeBlock creates a code surface. The language is a label, not a syntax
// highlighter. Code remains selectable and Copy code writes the original
// string, without the optional line numbers, to the current window's clipboard.
func CodeBlock(code string) *CodeBlockWidget {
	c := &CodeBlockWidget{copyButton: true, height: 320, startLine: 1}
	c.Role = ggui.RoleGroup
	c.SetName("Code content")
	c.AutoKey()
	c.code.Set(code)
	c.text = ggui.RichText().Selectable()
	c.title = ggui.Text("").NoWrap().Ellipsis()
	c.lang = ggui.Text("").NoWrap().Ellipsis()
	c.glyph = &codeCopyGlyph{owner: c}
	c.copy = ButtonOf(c.glyph, c.copyCode).Name("Copy code").Ghost().Size(ButtonIconExtraSmall)
	c.body = &codeBlockBody{owner: c, measure: ggui.RichText()}
	c.xScroll = ggui.Scroll(c.body).Horizontal().Controller(&c.xController)
	c.yScroll = ggui.Scroll(c.xScroll).Shrink().Controller(&c.yController)
	c.paintBodyFn = c.paintBody
	return c
}

// CodeBlockOf follows code without rebuilding the block.
func CodeBlockOf(code ggui.Readable[string]) *CodeBlockWidget { return CodeBlock("").BindCode(code) }

// Key gives the focusable viewport a stable identity across rebuilds.
func (c *CodeBlockWidget) Key(key any) *CodeBlockWidget { c.SetKey(key); return c }

// Wrap wraps long source lines to the viewport width. Gutter numbers and diff
// markers remain aligned to the first visual row of each logical source line.
func (c *CodeBlockWidget) Wrap(wrap bool) *CodeBlockWidget {
	defer property.Watch(&c.props, &c.wrap)()
	c.wrap = wrap
	return c
}

// LineStyles decorates 1-based source lines. Indices are independent of StartLine;
// missing entries are normal. The map is copied. Added and removed lines have
// visible + and − markers as well as tinted backgrounds and accessible labels.
func (c *CodeBlockWidget) LineStyles(styles map[int]CodeLineStyle) *CodeBlockWidget {
	defer property.Watch(&c.props, &c.lineStyles)()
	c.lineStyles = maps.Clone(styles)
	c.lineStylesGen++
	return c
}

// Code replaces the source and detaches any BindCode reader.
func (c *CodeBlockWidget) Code(code string) *CodeBlockWidget {
	property.Set(&c.props, &c.code, code)
	return c
}

// BindCode follows a source reader during layout and when copying.
func (c *CodeBlockWidget) BindCode(code ggui.Readable[string]) *CodeBlockWidget {
	property.Bind(&c.props, &c.code, code, "BindCode")
	return c
}

// Language sets the language label, drawn muted in the header row. Unknown
// languages render as plain code.
func (c *CodeBlockWidget) Language(language string) *CodeBlockWidget {
	defer property.Watch(&c.props, &c.language)()
	c.language = language
	return c
}

// Header sets an optional title, such as a filename, before the language
// label. A block with neither has no header row.
func (c *CodeBlockWidget) Header(header string) *CodeBlockWidget {
	defer property.Watch(&c.props, &c.header)()
	c.header = header
	return c
}

// Footer places a row under the code, such as a caption or a show-more
// action, inset as the header is and drawn in the muted color. Nil removes it.
func (c *CodeBlockWidget) Footer(w ggui.Widget) *CodeBlockWidget {
	defer property.Watch(&c.props, &c.footer)()
	c.footer = w
	return c
}

// Destructive draws the block as an error's output: a destructive border,
// title and code.
func (c *CodeBlockWidget) Destructive() *CodeBlockWidget {
	defer property.Watch(&c.props, &c.destructive)()
	c.destructive = true
	return c
}

// Small sets the code and the title at the caption size, in tighter padding.
func (c *CodeBlockWidget) Small() *CodeBlockWidget {
	defer property.Watch(&c.props, &c.small)()
	c.small = true
	return c
}

// LineNumbers shows a non-selectable gutter, starting at one.
func (c *CodeBlockWidget) LineNumbers(show bool) *CodeBlockWidget {
	defer property.Watch(&c.props, &c.lineNumbers)()
	c.lineNumbers = show
	return c
}

// StartLine sets the first gutter number, clamped to one. It does not change code.
func (c *CodeBlockWidget) StartLine(line int) *CodeBlockWidget {
	defer property.Watch(&c.props, &c.startLine)()
	c.startLine = max(1, line)
	return c
}

// CopyButton controls the built-in copy action; code remains selectable.
func (c *CodeBlockWidget) CopyButton(show bool) *CodeBlockWidget {
	defer property.Watch(&c.props, &c.copyButton)()
	c.copyButton = show
	return c
}

// Height caps the code viewport in logical pixels, excluding the header and
// the footer. Zero lets the parent alone constrain its height. The default
// is 320.
func (c *CodeBlockWidget) Height(px float64) *CodeBlockWidget {
	defer property.Watch(&c.props, &c.height)()
	c.height = max(0, px)
	return c
}

// OnCopy runs after copying, with the exact source that was written.
func (c *CodeBlockWidget) OnCopy(fn func(string)) *CodeBlockWidget { c.onCopy = fn; return c }

// copyCode writes the source to the clipboard. The action's icon turns to a
// check for codeCopiedFor, or until the source changes.
func (c *CodeBlockWidget) copyCode() {
	code := ggui.Untrack(c.code.Get)
	ggui.CurrentClipboard().Write(code)
	c.copied, c.copiedAt = true, ggui.Now()
	if c.onCopy != nil {
		c.onCopy(code)
	}
}

// caption names the block for assistive technology.
func (c *CodeBlockWidget) caption() string {
	if c.header != "" && c.language != "" {
		return c.header + " · " + c.language
	}
	if c.header != "" {
		return c.header
	}
	if c.language != "" {
		return c.language
	}
	return "Code"
}

// ink is the color of the code and the title.
func (c *CodeBlockWidget) ink() color.Color {
	return pick(c.destructive, c.theme.Destructive, c.theme.Fg)
}

// Layout implements Widget.
func (c *CodeBlockWidget) Layout(cs ggui.Constraints, env ggui.Env) ggui.Size {
	defer c.props.Layout().EndLayout()
	c.viewportW = cs.MaxW
	c.rtl = env.Direction() == ggui.DirRTL
	c.theme = uitheme.Of(env)
	t := c.theme
	c.motion = env.Motion(t.MotionFast)
	code := c.code.Get()
	if code != c.laidCode {
		c.copied = false
	}
	c.laidCode = code
	// Small is the caption's share of the text size; the code keeps a
	// relaxed line either way.
	scale := 1.0
	if c.small && t.Text.Size > 0 && t.Caption.Size > 0 {
		scale = t.Caption.Size / t.Text.Size
	}
	c.codeStyle = t.Text.Merge(t.Mono)
	c.codeStyle.Size *= scale
	c.codeStyle.LineHeight = 1.625
	c.text.Spans(ggui.Span(code)).Style(c.codeStyle).Color(c.ink())
	c.pad = t.Space * pick(c.small, 1.25, 1.5)
	c.padTop, c.padBottom = c.pad, c.pad
	innerW := max(0, cs.MaxW-2*c.pad)

	c.copySize = ggui.Size{}
	if c.copyButton {
		c.copy.Radius(min(t.RadiusSm, 10))
		c.copySize = c.copy.Layout(ggui.Loose(ggui.Sz(cs.MaxW, cs.MaxH)), env)
	}
	c.hasHeader = c.header != "" || c.language != ""
	c.headerH, c.titleSize, c.langSize = 0, ggui.Size{}, ggui.Size{}
	if c.hasHeader {
		label := t.Text
		label.Size *= scale
		c.title.Content(c.header).Style(label.Merge(t.Label)).Size(label.Size).Color(c.ink())
		c.lang.Content(c.language).Style(label).Color(pick(c.destructive, t.Destructive, t.MutedFg))
		avail := innerW
		if c.copyButton {
			// The action hangs half a space into the padding.
			avail = max(0, avail-c.copySize.W+t.Space*.75-t.Space)
		}
		if c.language != "" {
			share := avail
			if c.header != "" {
				share = avail / 2
			}
			c.langSize = c.lang.Layout(ggui.Loose(ggui.Sz(share, cs.MaxH)), env)
			avail = max(0, avail-c.langSize.W-t.Space)
		}
		if c.header != "" {
			c.titleSize = c.title.Layout(ggui.Loose(ggui.Sz(avail, cs.MaxH)), env)
		}
		c.headerH = min(cs.MaxH, c.pad+max(c.titleSize.H, c.langSize.H))
		c.padTop = t.Space * pick(c.small, .75, 1.0)
	}
	c.footerSize = ggui.Size{}
	footerH := 0.0
	if c.footer != nil {
		c.footerSize = c.footer.Layout(ggui.Loose(ggui.Sz(innerW, max(0, cs.MaxH-c.headerH))), env.WithText(ggui.TextStyle{Color: t.MutedFg, Size: t.Text.Size * scale}))
		footerH = min(max(0, cs.MaxH-c.headerH), c.footerSize.H+c.pad)
		c.padBottom = t.Space
	}
	bodyH := max(0, cs.MaxH-c.headerH-footerH)
	if c.height > 0 {
		bodyH = min(bodyH, c.height)
	}
	c.bodySize = c.yScroll.Layout(ggui.Constraints{MinW: cs.MinW, MaxW: cs.MaxW, MaxH: bodyH}, env.With(ggui.DirectionKey, ggui.DirLTR))
	headW := c.titleSize.W + c.langSize.W + c.copySize.W + 2*c.pad + t.Space
	return cs.Constrain(ggui.Sz(max(c.bodySize.W, headW, c.footerSize.W+2*c.pad), c.headerH+c.bodySize.H+footerH))
}

// codeActionsSlot keeps the floating action's fade across frames.
var codeActionsSlot = ggui.NewSlot[*ggui.Motion]("code block actions")

// Paint implements Widget.
func (c *CodeBlockWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t := c.theme
	dark := isDark(t.Bg)
	if c.copied {
		// The check turns back into the copy icon by itself.
		if until := c.copiedAt.Add(codeCopiedFor); ggui.FrameTime().Before(until) {
			ggui.WakeAt(until)
		} else {
			c.copied = false
		}
	}
	// The floating action shows while the pointer or focus is in the block.
	shown := 1.0
	if c.copyButton && !c.hasHeader {
		p, ok := dst.Pointer()
		shown = dst.Ease(c.Anchor(r), codeActionsSlot, pick(ok && r.Contains(p) || dst.FocusWithin(r), 1.0, 0.0), c.motion)
	}
	border := t.InputBorder
	if c.destructive {
		border = fade(t.Destructive, pick(dark, .4, .3))
	} else if dark {
		dst.FillRoundRect(r, t.Radius, fade(t.InputBorder, .3))
	}
	outer := dst
	dst = dst.Clip(r)
	dst.Node(r, ggui.Node{Role: ggui.RoleGroup, Name: c.caption(), Description: "Code block"}, func(dst *ggui.Canvas) {
		line := max(0, c.headerH-c.pad)
		if c.hasHeader {
			x := c.pad
			if c.header != "" {
				dst.Paint(c.title, ggui.Rct(r.Origin.Add(ggui.Pt(c.side(r, x, c.titleSize.W), c.pad+(line-c.titleSize.H)/2)), c.titleSize))
				x += c.titleSize.W + t.Space
			}
			if c.language != "" {
				dst.Paint(c.lang, ggui.Rct(r.Origin.Add(ggui.Pt(c.side(r, x, c.langSize.W), c.pad+(line-c.langSize.H)/2)), c.langSize))
			}
		}
		if c.copied {
			dst.Leaf(r, ggui.Node{Role: ggui.RoleStatus, Name: "Code copied"})
		}
		viewport := ggui.Rct(r.Origin.Add(ggui.Pt(0, c.headerH)), ggui.Sz(r.Size.W, c.bodySize.H))
		c.Hit(dst, viewport, c, ggui.CursorShapeDefault)
		// shadcn's scroll-fade-x, and scroll-fade-y for wrapped code: the
		// code fades out toward an end there is more beyond, by as much as
		// there is, as a mask over whatever the block stands on.
		c.viewport = viewport
		lead, trail := codeFade(&c.xController, 2*t.Space)
		if c.wrap {
			lead, trail = codeFade(&c.yController, 3*t.Space)
		}
		dst.FadeEdges(viewport, !c.wrap, lead, trail, c.paintBodyFn)
		if c.footer != nil {
			at := ggui.Pt(c.side(r, c.pad, c.footerSize.W), c.headerH+c.bodySize.H)
			dst.Paint(c.footer, ggui.Rct(r.Origin.Add(at), c.footerSize))
		}
		if !c.copyButton {
			return
		}
		if c.hasHeader {
			// In the header the action hangs into the padding, so its icon
			// lines up with the code's edge.
			x := r.Size.W - c.pad + t.Space*.75 - c.copySize.W
			dst.Paint(c.copy, ggui.Rct(r.Origin.Add(ggui.Pt(c.side(r, x, c.copySize.W), c.pad+(line-c.copySize.H)/2)), c.copySize))
			return
		}
		inset := t.Space * pick(c.small, .75, 1.0)
		at := ggui.Rct(r.Origin.Add(ggui.Pt(c.side(r, r.Size.W-inset-c.copySize.W, c.copySize.W), inset)), c.copySize)
		if shown <= 0 {
			// Out of sight it still takes Tab and a click, which bring it back.
			c.copy.Hit(dst, at, c.copy, ggui.CursorShapeDefault)
			return
		}
		float := func(dst *ggui.Canvas) {
			radius := min(t.RadiusSm, 10)
			dst.FillRoundRect(at, radius, t.Bg)
			dst.StrokeRoundRect(at.Grow(1), radius+1, 1, fade(t.Fg, .1))
			dst.Paint(c.copy, at)
		}
		if shown < 1 {
			dst.Layer(ggui.LayerOptions{Fade: 1 - shown}, float)
		} else {
			float(dst)
		}
	})
	focused := c.Focused && c.FocusVisible && !c.IsInert()
	outer.StrokeRoundRect(r, t.Radius, t.BorderWidth, pick(focused && !c.destructive, t.Ring, border))
	if focused {
		fieldHalo(outer, t, r, t.Radius, pick(c.destructive, destructiveRing(t), t.Ring))
	}
}

// paintBody paints the scrolling code into the viewport Paint noted.
func (c *CodeBlockWidget) paintBody(dst *ggui.Canvas) { dst.Paint(c.yScroll, c.viewport) }

// codeFade is how long the faded ends of a scrolled axis are: what lies
// beyond each end, up to limit.
func codeFade(axis *ggui.ScrollController, limit float64) (lead, trail float64) {
	at := axis.Offset()
	return clamp(at, 0, limit), clamp(axis.MaxOffset()-at, 0, limit)
}

// side places a run w wide x in from the block's start edge: from the left,
// or from the right under a right-to-left direction.
func (c *CodeBlockWidget) side(r ggui.Rect, x, w float64) float64 {
	if c.rtl {
		return max(0, r.Size.W-x-w)
	}
	return x
}

// Describe exposes the focusable code viewport separately from its copy action.
func (c *CodeBlockWidget) Describe() ggui.Node {
	return ggui.Node{Role: ggui.RoleGroup, Name: "Code content", Description: "Use arrow keys or Page Up and Page Down to scroll", Actions: ggui.ActionFocus}
}

// HandlePointer focuses empty space in the code viewport; the selectable text
// and scrollbars paint above it and keep their own pointer behavior.
func (c *CodeBlockWidget) HandlePointer(ev ggui.PointerEvent) bool { return c.Pointer(ev, nil) }

// HandleKey scrolls the viewport. Copy and select-all use the RichText selection;
// Shift-arrow keeps selecting text.
func (c *CodeBlockWidget) HandleKey(ev ggui.KeyEvent) {
	c.Keyboard(ev, nil)
	if ev.Kind != ggui.KeyPress || ev.Mods.Shift {
		c.text.HandleKey(ev)
		return
	}
	switch ev.Key {
	case ggui.KeyArrowLeft:
		c.xController.JumpTo(c.xController.Offset() - 40)
	case ggui.KeyArrowRight:
		c.xController.JumpTo(c.xController.Offset() + 40)
	case ggui.KeyArrowUp:
		c.yController.JumpTo(c.yController.Offset() - 40)
	case ggui.KeyArrowDown:
		c.yController.JumpTo(c.yController.Offset() + 40)
	case ggui.KeyPageUp:
		c.yController.JumpTo(c.yController.Offset() - c.bodySize.H)
	case ggui.KeyPageDown:
		c.yController.JumpTo(c.yController.Offset() + c.bodySize.H)
	case ggui.KeyHome:
		c.xController.JumpTo(0)
		c.yController.JumpTo(0)
	case ggui.KeyEnd:
		c.yController.JumpTo(c.yController.MaxOffset())
	default:
		c.text.HandleKey(ev)
	}
}

// codeCopyGlyph is the copy action's icon: a copy sign that is a check
// while the block shows it copied.
type codeCopyGlyph struct {
	owner *CodeBlockWidget
	env   ggui.Env
}

func (g *codeCopyGlyph) Layout(cs ggui.Constraints, env ggui.Env) ggui.Size {
	g.env = env
	side := uitheme.Of(env).IconSize * .75
	return cs.Constrain(ggui.Sz(side, side))
}

func (g *codeCopyGlyph) Paint(dst *ggui.Canvas, r ggui.Rect) {
	c := g.owner
	// In a header the action is muted until the pointer or focus is on it.
	ink := c.ink()
	if c.hasHeader && !c.destructive && !c.copy.Hovered && !c.copy.Focused {
		ink = c.theme.MutedFg
	}
	paintChatIcon(dst, g.env, pick(c.copied, icons.Check, iconCopy), r, ink, 0)
}

// The gutter is painted separately so selecting or copying code cannot include it.
type codeBlockBody struct {
	owner                           *CodeBlockWidget
	measure                         *ggui.RichTextWidget
	lines                           []codeBlockLine
	textSize                        ggui.Size
	gutter, numberW, markerW, lineH float64
	leading                         float64 // the space a line keeps under its glyphs
	laid                            codeBlockGutter
}

// codeBlockGutter is what the gutter's geometry was measured from. A view
// lays its content out at every step of a scroll, and shaping each line of
// a long file again for the same answer is most of what that cost.
type codeBlockGutter struct {
	code                string
	style               ggui.TextStyle // the code's, with its size and line height
	theme               *uitheme.Theme
	wrapW               float64 // the viewport's width when lines wrap, else 0
	pad                 float64 // the code's side padding, which a wrapped line's width leaves out
	lineNumbers         bool
	startLine, styleGen int
}

func (g codeBlockGutter) same(o codeBlockGutter) bool {
	return g.code == o.code && g.style.Equal(o.style) && g.theme == o.theme && g.wrapW == o.wrapW && g.pad == o.pad &&
		g.lineNumbers == o.lineNumbers && g.startLine == o.startLine && g.styleGen == o.styleGen
}

type codeBlockLine struct {
	number, marker *ggui.TextWidget
	top, height    float64
	style          CodeLineStyle
}

// codeNumberMin is the least width of the line number column, and
// codeNumberGap what parts it from the code.
const (
	codeNumberMin = 24
	codeNumberGap = 12
)

func (b *codeBlockBody) Layout(cs ggui.Constraints, env ggui.Env) ggui.Size {
	c := b.owner
	sides, ends := 2*c.pad, c.padTop+c.padBottom
	// Plain logs need no second pass over every source line for gutter geometry.
	if !c.lineNumbers && len(c.lineStyles) == 0 {
		b.lines, b.gutter = b.lines[:0], 0
		textW := ggui.Unbounded
		if c.wrap {
			textW = max(1, c.viewportW-sides)
		}
		b.textSize = c.text.Layout(ggui.Loose(ggui.Sz(textW, ggui.Unbounded)), env)
		return cs.Constrain(ggui.Sz(b.textSize.W+sides, b.textSize.H+ends))
	}
	style := c.codeStyle
	resolved := env.ResolveText(style)
	textW := ggui.Unbounded
	inputs := codeBlockGutter{code: c.laidCode, style: resolved, theme: c.theme, pad: c.pad, lineNumbers: c.lineNumbers, startLine: c.startLine, styleGen: c.lineStylesGen}
	if c.wrap {
		inputs.wrapW = c.viewportW
		textW = max(1, c.viewportW-b.gutter-sides)
	}
	if len(b.lines) > 0 && inputs.same(b.laid) {
		b.textSize = c.text.Layout(ggui.Loose(ggui.Sz(textW, ggui.Unbounded)), env)
		return cs.Constrain(ggui.Sz(b.textSize.W+b.gutter+sides, b.textSize.H+ends))
	}
	b.laid = inputs
	b.measure.Style(style)
	loose := ggui.Loose(ggui.Sz(ggui.Unbounded, ggui.Unbounded))
	b.lineH = b.measure.Spans(ggui.Span("")).Layout(loose, env).H
	// Every line uses the same font metrics. A paragraph omits the final line's
	// leading; adding it back gives the next logical line's exact top offset.
	b.leading = resolved.Size*resolved.LineHeight - b.lineH
	sourceLines := strings.Split(c.laidCode, "\n")
	for len(b.lines) < len(sourceLines) {
		b.lines = append(b.lines, codeBlockLine{number: ggui.Text("").NoWrap().Align(1), marker: ggui.Text("").NoWrap()})
	}
	b.lines = b.lines[:len(sourceLines)]
	b.gutter, b.numberW, b.markerW = 0, 0, 0
	for i := range b.lines {
		line := &b.lines[i]
		line.style = c.lineStyles[i+1]
		if c.lineNumbers {
			line.number.Content(strconv.Itoa(c.startLine + i)).Style(style).Color(c.theme.MutedFg)
			b.numberW = max(b.numberW, line.number.Layout(loose, env).W)
		}
		// Only a changed line has a marker to shape; Paint asks the same.
		if line.style == CodeLineAdded || line.style == CodeLineRemoved {
			line.marker.Content(pick(line.style == CodeLineAdded, "+", "−")).Style(style).Color(pick(line.style == CodeLineRemoved, c.theme.Destructive, c.theme.MutedFg))
			b.markerW = max(b.markerW, line.marker.Layout(loose, env).W)
		}
	}
	// The numbers take a column; a marker sits in the padding before it.
	if c.lineNumbers {
		b.numberW = max(b.numberW, codeNumberMin)
		b.gutter = b.numberW + codeNumberGap
	}
	if c.wrap {
		textW = max(1, c.viewportW-b.gutter-sides)
	}
	textCS := ggui.Loose(ggui.Sz(textW, ggui.Unbounded))
	b.textSize = c.text.Layout(textCS, env)
	top := 0.0
	for i, source := range sourceLines {
		line := &b.lines[i]
		line.top = top
		line.height = b.measure.Spans(ggui.Span(source)).Layout(textCS, env).H + b.leading
		top += line.height
	}
	return cs.Constrain(ggui.Sz(b.textSize.W+b.gutter+sides, b.textSize.H+ends))
}

func (b *codeBlockBody) Paint(dst *ggui.Canvas, r ggui.Rect) {
	c := b.owner
	// The gutter draws only the lines in view; a long log has few of them.
	visible, gutter := dst.Visible(r), dst.Inert()
	for i := range b.lines {
		line := &b.lines[i]
		at := r.Origin.Add(ggui.Pt(c.pad, c.padTop+line.top))
		var fill color.Color
		var annotation string
		switch line.style {
		case CodeLineHighlighted:
			fill, annotation = c.theme.Muted, "Highlighted line "
		case CodeLineAdded:
			fill, annotation = c.theme.Muted, "Added line "
		case CodeLineRemoved:
			fill, annotation = fade(c.theme.Destructive, .1), "Removed line "
		}
		if annotation != "" {
			// The band runs from edge to edge of the viewport or the longest
			// line, whichever is wider, with the line's leading shared above
			// and below its glyphs.
			dst.FillRect(ggui.Rct(ggui.Pt(r.Origin.X, at.Y-b.leading/2), ggui.Sz(max(r.Size.W, c.viewportW), line.height)), fill)
			dst.Leaf(ggui.Rct(at, ggui.Sz(max(0, r.Size.W-2*c.pad), line.height)), ggui.Node{Role: ggui.RoleText, Name: annotation + strconv.Itoa(c.startLine+i)})
		}
		if at.Y+line.height < visible.Origin.Y || at.Y > visible.Origin.Y+visible.Size.H {
			continue
		}
		if line.style == CodeLineAdded || line.style == CodeLineRemoved {
			gutter.Paint(line.marker, ggui.Rct(ggui.Pt(r.Origin.X+c.theme.Space/2, at.Y), ggui.Sz(b.markerW, b.lineH)))
		}
		if c.lineNumbers {
			gutter.Paint(line.number, ggui.Rct(at, ggui.Sz(b.numberW, b.lineH)))
		}
	}
	dst.Paint(c.text, ggui.Rct(r.Origin.Add(ggui.Pt(c.pad+b.gutter, c.padTop)), b.textSize))
}
