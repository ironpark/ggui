package ui

import (
	"image/color"
	"maps"
	"strconv"
	"strings"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// CodeLineStyle decorates a logical source line without changing its contents.
type CodeLineStyle uint8

const (
	CodeLineNormal CodeLineStyle = iota
	CodeLineHighlighted
	CodeLineAdded
	CodeLineRemoved
)

// CodeBlockWidget presents selectable code with a copy action, optional wrapping
// and line decorations, and scrolling along both axes. Build one with CodeBlock.
type CodeBlockWidget struct {
	ggui.Interactive
	wrap                          bool
	rtl                           bool
	viewportW                     float64
	lineStyles                    map[int]CodeLineStyle
	startLine                     int
	xController, yController      ggui.ScrollController
	props                         property.Owner
	code                          property.Value[string]
	language, header              string
	lineNumbers, copyButton       bool
	height                        float64
	onCopy                        func(string)
	text                          *ggui.RichTextWidget
	label, copyLabel              *ggui.TextWidget
	copy                          *ButtonWidget
	xScroll, yScroll              *ggui.ScrollWidget
	body                          *codeBlockBody
	theme                         *uitheme.Theme
	labelSize, copySize, bodySize ggui.Size
	headerH                       float64
	laidCode                      string
	copied                        bool
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
	c.label = ggui.Text("").NoWrap().Ellipsis()
	c.copyLabel = ggui.Text("Copy")
	c.copy = ButtonOf(c.copyLabel, c.copyCode).Name("Copy code").Ghost().Size(ButtonSmall)
	c.body = &codeBlockBody{owner: c, measure: ggui.RichText()}
	c.xScroll = ggui.Scroll(c.body).Horizontal().Controller(&c.xController)
	c.yScroll = ggui.Scroll(c.xScroll).Shrink().Controller(&c.yController)
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

// Language sets the language label. Unknown languages render as plain code.
func (c *CodeBlockWidget) Language(language string) *CodeBlockWidget {
	defer property.Watch(&c.props, &c.language)()
	c.language = language
	return c
}

// Header sets an optional title, such as a filename, before the language label.
func (c *CodeBlockWidget) Header(header string) *CodeBlockWidget {
	defer property.Watch(&c.props, &c.header)()
	c.header = header
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

// Height caps the code viewport in logical pixels, excluding the header.
// Zero lets the parent alone constrain its height. The default is 320.
func (c *CodeBlockWidget) Height(px float64) *CodeBlockWidget {
	defer property.Watch(&c.props, &c.height)()
	c.height = max(0, px)
	return c
}

// OnCopy runs after copying, with the exact source that was written.
func (c *CodeBlockWidget) OnCopy(fn func(string)) *CodeBlockWidget { c.onCopy = fn; return c }

func (c *CodeBlockWidget) copyCode() {
	code := ggui.Untrack(c.code.Get)
	ggui.CurrentClipboard().Write(code)
	c.copied = true
	c.copyLabel.Content("Copied")
	if c.onCopy != nil {
		c.onCopy(code)
	}
}

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

// Layout implements Widget.
func (c *CodeBlockWidget) Layout(cs ggui.Constraints, env ggui.Env) ggui.Size {
	defer c.props.Layout().EndLayout()
	c.viewportW = cs.MaxW
	c.rtl = env.Direction() == ggui.DirRTL
	c.theme = uitheme.Of(env)
	t := c.theme
	code := c.code.Get()
	if code != c.laidCode {
		c.copied = false
		c.copyLabel.Content("Copy")
	}
	c.laidCode = code
	c.text.Spans(ggui.Span(code)).Style(t.Text.Merge(t.Mono)).Color(t.Fg)
	c.label.Content(c.caption()).Style(t.Caption).Color(t.MutedFg)
	innerW := max(0, cs.MaxW-24)
	c.copySize = ggui.Size{}
	if c.copyButton {
		c.copySize = c.copy.Layout(ggui.Loose(ggui.Sz(innerW, cs.MaxH)), env)
	}
	c.labelSize = c.label.Layout(ggui.Loose(ggui.Sz(max(0, innerW-c.copySize.W-8), cs.MaxH)), env)
	c.headerH = min(cs.MaxH, max(c.labelSize.H, c.copySize.H)+16)
	bodyH := max(0, cs.MaxH-c.headerH)
	if c.height > 0 {
		bodyH = min(bodyH, c.height)
	}
	c.bodySize = c.yScroll.Layout(ggui.Constraints{MinW: cs.MinW, MaxW: cs.MaxW, MaxH: bodyH}, env.With(ggui.DirectionKey, ggui.DirLTR))
	return cs.Constrain(ggui.Sz(max(c.bodySize.W, c.labelSize.W+c.copySize.W+32), c.headerH+c.bodySize.H))
}

// Paint implements Widget.
func (c *CodeBlockWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t := c.theme
	dst.FillRoundRect(r, t.Radius, t.Muted)
	dst.StrokeRoundRect(r, t.Radius, t.BorderWidth, t.Border)
	dst = dst.Clip(r)
	dst.Node(r, ggui.Node{Role: ggui.RoleGroup, Name: c.caption(), Description: "Code block"}, func(dst *ggui.Canvas) {
		labelX := 12.0
		copyX := max(0, r.Size.W-12-c.copySize.W)
		if c.rtl {
			labelX, copyX = max(0, r.Size.W-12-c.labelSize.W), 12
		}
		dst.Paint(c.label, ggui.Rct(r.Origin.Add(ggui.Pt(labelX, (c.headerH-c.labelSize.H)/2)), c.labelSize))
		if c.copyButton {
			dst.Paint(c.copy, ggui.Rct(r.Origin.Add(ggui.Pt(copyX, (c.headerH-c.copySize.H)/2)), c.copySize))
		}
		if c.copied {
			dst.Leaf(r, ggui.Node{Role: ggui.RoleStatus, Name: "Code copied"})
		}
		viewport := ggui.Rct(r.Origin.Add(ggui.Pt(0, c.headerH)), ggui.Sz(r.Size.W, c.bodySize.H))
		c.Hit(dst, viewport, c, ggui.CursorShapeDefault)
		dst.Paint(c.yScroll, viewport)
		c.FocusRing(dst, viewport, t.Radius, t.Ring)
	})
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

// The gutter is painted separately so selecting or copying code cannot include it.
type codeBlockBody struct {
	owner                           *CodeBlockWidget
	measure                         *ggui.RichTextWidget
	lines                           []codeBlockLine
	textSize                        ggui.Size
	gutter, numberW, markerW, lineH float64
}

type codeBlockLine struct {
	number, marker *ggui.TextWidget
	top, height    float64
	style          CodeLineStyle
}

func (b *codeBlockBody) Layout(cs ggui.Constraints, env ggui.Env) ggui.Size {
	c := b.owner
	// Plain logs need no second pass over every source line for gutter geometry.
	if !c.lineNumbers && len(c.lineStyles) == 0 {
		b.lines, b.gutter = b.lines[:0], 0
		textW := ggui.Unbounded
		if c.wrap {
			textW = max(1, c.viewportW-24)
		}
		b.textSize = c.text.Layout(ggui.Loose(ggui.Sz(textW, ggui.Unbounded)), env)
		return cs.Constrain(ggui.Sz(b.textSize.W+24, b.textSize.H+24))
	}
	style := c.theme.Text.Merge(c.theme.Mono)
	resolved := env.ResolveText(style)
	b.measure.Style(style)
	loose := ggui.Loose(ggui.Sz(ggui.Unbounded, ggui.Unbounded))
	b.lineH = b.measure.Spans(ggui.Span("")).Layout(loose, env).H
	// Every line uses the same font metrics. A paragraph omits the final line's
	// leading; adding it back gives the next logical line's exact top offset.
	leading := resolved.Size*resolved.LineHeight - b.lineH
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
			line.marker.Content(pick(line.style == CodeLineAdded, "+", "−")).Style(style).Color(pick(line.style == CodeLineRemoved, c.theme.Destructive, c.theme.Fg))
			b.markerW = max(b.markerW, line.marker.Layout(loose, env).W)
		}
	}
	if b.numberW > 0 {
		b.gutter += b.numberW + 16
	}
	if b.markerW > 0 {
		b.gutter += b.markerW + 8
	}
	textW := ggui.Unbounded
	if c.wrap {
		textW = max(1, c.viewportW-b.gutter-24)
	}
	textCS := ggui.Loose(ggui.Sz(textW, ggui.Unbounded))
	b.textSize = c.text.Layout(textCS, env)
	top := 0.0
	for i, source := range sourceLines {
		line := &b.lines[i]
		line.top = top
		line.height = b.measure.Spans(ggui.Span(source)).Layout(textCS, env).H + leading
		top += line.height
	}
	return cs.Constrain(ggui.Sz(b.textSize.W+b.gutter+24, b.textSize.H+24))
}

func (b *codeBlockBody) Paint(dst *ggui.Canvas, r ggui.Rect) {
	c := b.owner
	// The gutter draws only the lines in view; a long log has few of them.
	visible, gutter := dst.Visible(r), dst.Inert()
	for i := range b.lines {
		line := &b.lines[i]
		at := r.Origin.Add(ggui.Pt(12, 12+line.top))
		var fill color.Color
		var annotation string
		switch line.style {
		case CodeLineHighlighted:
			fill, annotation = mix(c.theme.Muted, c.theme.Fg, .08), "Highlighted line "
		case CodeLineAdded:
			fill, annotation = fade(c.theme.Primary, .12), "Added line "
		case CodeLineRemoved:
			fill, annotation = fade(c.theme.Destructive, .12), "Removed line "
		}
		if annotation != "" {
			dst.FillRect(ggui.Rct(ggui.Pt(r.Origin.X, at.Y), ggui.Sz(r.Size.W, line.height)), fill)
			dst.Leaf(ggui.Rct(at, ggui.Sz(max(0, r.Size.W-24), line.height)), ggui.Node{Role: ggui.RoleText, Name: annotation + strconv.Itoa(c.startLine+i)})
		}
		if at.Y+line.height < visible.Origin.Y || at.Y > visible.Origin.Y+visible.Size.H {
			continue
		}
		if c.lineNumbers {
			gutter.Paint(line.number, ggui.Rct(at, ggui.Sz(b.numberW, b.lineH)))
			at.X += b.numberW + 16
		}
		if line.style == CodeLineAdded || line.style == CodeLineRemoved {
			gutter.Paint(line.marker, ggui.Rct(at, ggui.Sz(b.markerW, b.lineH)))
		}
	}
	dst.Paint(c.text, ggui.Rct(r.Origin.Add(ggui.Pt(12+b.gutter, 12)), b.textSize))
}
