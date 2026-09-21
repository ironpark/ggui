package ui

import (
	"image/color"
	"math"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
)

// AttachmentState is the caller-owned upload lifecycle. Attachment does not
// read files, upload data or start background work.
type AttachmentState string

const (
	AttachmentIdle       AttachmentState = "idle"
	AttachmentUploading  AttachmentState = "uploading"
	AttachmentProcessing AttachmentState = "processing"
	AttachmentError      AttachmentState = "error"
	AttachmentDone       AttachmentState = "done"
)

// AttachmentSize selects shadcn's default, sm or xs attachment geometry.
type AttachmentSize uint8

const (
	AttachmentDefault AttachmentSize = iota
	AttachmentSmall
	AttachmentExtraSmall
)

// AttachmentWidget presents a file's media, metadata and independent actions.
// Its optional full-card trigger sits behind the actions in the hit order.
// Geometry follows shadcn/ui's Rhea attachment; colors follow the theme.
type AttachmentWidget struct {
	props                                property.Owner
	title, description                   string
	titleText, descriptionText           attachmentText
	media                                ggui.Widget
	mediaView                            *ggui.StyledWidget
	image                                *ebiten.Image
	imageAlt                             string
	actions                              []ggui.Widget
	trigger                              *ButtonWidget
	state                                AttachmentState
	stateReader                          ggui.Readable[AttachmentState]
	size                                 AttachmentSize
	vertical                             bool
	width                                float64
	theme                                ggui.Theme
	reduced                              bool
	current                              AttachmentState
	padX, padY, gap, radius, mediaSide   float64
	mediaRect, textRect, descriptionRect ggui.Rect
	actionRects                          []ggui.Rect
}

// Attachment creates a completed, horizontal file card. Media and actions are
// optional. State and BindState change presentation without owning the upload.
func Attachment(title, description string) *AttachmentWidget {
	return &AttachmentWidget{title: title, description: description, state: AttachmentDone}
}

// Media sets an icon or custom media widget, replacing an image preview.
func (a *AttachmentWidget) Media(w ggui.Widget) *AttachmentWidget {
	a.media, a.image, a.mediaView = w, nil, nil
	return a
}

// Image sets a rounded, square, cover-cropped preview. Nil restores icon media.
func (a *AttachmentWidget) Image(img *ebiten.Image, alt string) *AttachmentWidget {
	a.image, a.imageAlt = img, alt
	return a
}

// Actions sets independently focusable actions. Use AttachmentAction for the
// default compact ghost style, or supply any widget.
func (a *AttachmentWidget) Actions(actions ...ggui.Widget) *AttachmentWidget {
	a.actions = actions
	return a
}

// Trigger makes the whole card activatable. Give the action a descriptive name,
// e.g. "Preview report.pdf". Passing nil removes the trigger.
func (a *AttachmentWidget) Trigger(name string, fn func()) *AttachmentWidget {
	if fn == nil {
		a.trigger = nil
	} else {
		a.trigger = ButtonOf(ggui.Box(), fn).Name(name).Ghost().Pad(0)
	}
	return a
}

// State sets a fixed upload state, replacing a BindState reader.
func (a *AttachmentWidget) State(s AttachmentState) *AttachmentWidget {
	defer property.Watch(&a.props, &a.state)()
	defer property.Watch(&a.props, &a.stateReader)()
	a.state, a.stateReader = s, nil
	return a
}

// BindState follows an upload state without rebuilding the attachment.
func (a *AttachmentWidget) BindState(r ggui.Readable[AttachmentState]) *AttachmentWidget {
	property.Require(r, "BindState")
	if !property.Same(a.stateReader, r) {
		a.stateReader = r
		a.props.Changed()
	}
	return a
}

// Size selects default, small or extra-small geometry.
func (a *AttachmentWidget) Size(s AttachmentSize) *AttachmentWidget {
	defer property.Watch(&a.props, &a.size)()
	a.size = s
	return a
}

// Vertical places square media above the metadata; actions overlay its top right.
func (a *AttachmentWidget) Vertical() *AttachmentWidget { a.vertical = true; return a }

// Width overrides the intrinsic card width, still constrained by its parent.
func (a *AttachmentWidget) Width(px float64) *AttachmentWidget {
	defer property.Watch(&a.props, &a.width)()
	a.width = max(0, px)
	return a
}

// AttachmentAction is a 24px ghost action with an explicit accessible name.
// content is usually an icon. Button setters can customize its style or state.
func AttachmentAction(name string, content ggui.Widget, fn func()) *ButtonWidget {
	return ButtonOf(ggui.Box(ggui.Center(content)).Width(24).Height(24), fn).Name(name).Ghost().Pad(0)
}

func (a *AttachmentWidget) uploadState() AttachmentState {
	s := a.state
	if a.stateReader != nil {
		s = a.stateReader.Get()
	}
	switch s {
	case AttachmentIdle, AttachmentUploading, AttachmentProcessing, AttachmentError, AttachmentDone:
		return s
	}
	return AttachmentDone
}

func (a *AttachmentWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer a.props.Layout()()
	a.theme, a.reduced, a.current = env.Theme(), env.ReducedMotion(), a.uploadState()
	a.padX, a.padY, a.gap, a.mediaSide = 10, 8, 8, 40
	a.radius = a.theme.ChatTokens().AttachmentRadius
	fontSize := 14.0
	switch a.size {
	case AttachmentSmall:
		a.padX, a.padY, a.gap, a.mediaSide, fontSize = 8, 6, 10, 32, 12
	case AttachmentExtraSmall:
		a.padX, a.padY, a.gap, a.radius, a.mediaSide, fontSize = 6, 4, 6, a.theme.ChatTokens().AttachmentXSRadius, 28, 12
	}
	hasMedia := a.media != nil || a.image != nil
	if hasMedia {
		a.padX = a.padY
	}
	// Reserve the border separately from content padding, like CSS border-box.
	a.padX += 1
	a.padY += 1
	a.titleText.set(a.title, fontSize, a.theme.Fg)
	a.titleText.text.Font(a.theme.Title.Font)
	descColor := a.theme.MutedFg
	if a.current == AttachmentError {
		descColor = mix(a.theme.Destructive, a.theme.Card, .2)
	}
	a.descriptionText.set(a.description, 12, descColor)
	free := ggui.Loose(ggui.Sz(ggui.Unbounded, ggui.Unbounded))
	title := a.titleText.Layout(free, env)
	desc := a.descriptionText.Layout(free, env)
	textW := max(title.W, desc.W)
	a.actionRects = a.actionRects[:0]
	actionW, actionH := 0.0, 0.0
	for _, action := range a.actions {
		s := action.Layout(c.Loosen(), env)
		if a.vertical && len(a.actionRects) > 0 {
			actionW += 4
		}
		a.actionRects = append(a.actionRects, ggui.Rct(ggui.Pt(actionW, 0), s))
		actionW += s.W
		actionH = max(actionH, s.H)
	}
	w := a.padX*2 + textW
	if a.vertical {
		w = 120
	} else {
		if hasMedia {
			w += a.mediaSide + a.gap
		}
		if len(a.actions) > 0 {
			w += actionW + a.gap
		}
	}
	if !a.vertical {
		w = max(160, w)
	}
	if a.width > 0 {
		w = a.width
	}
	w = min(c.MaxW, max(c.MinW, w))
	inner := max(0, w-2*a.padX)
	a.mediaRect = ggui.Rect{}
	if a.vertical {
		if hasMedia {
			a.mediaSide = inner
			a.mediaRect = ggui.Rct(ggui.Pt(a.padX, a.padY), ggui.Sz(inner, inner))
		}
		textW = max(0, inner-8)
	} else {
		textW = inner
		if hasMedia {
			a.mediaSide = min(a.mediaSide, inner)
			textW -= a.mediaSide + a.gap
		}
		if len(a.actions) > 0 {
			textW -= actionW + a.gap
		}
		textW = max(0, textW)
	}
	title = a.titleText.Layout(ggui.Loose(ggui.Sz(textW, ggui.Unbounded)), env)
	desc = a.descriptionText.Layout(ggui.Loose(ggui.Sz(textW, ggui.Unbounded)), env)
	textH := title.H
	if a.description != "" {
		textH += 2 + desc.H
	}
	h := max(textH, actionH)
	x, y := a.padX, a.padY
	if a.vertical {
		if hasMedia {
			y += a.mediaSide + a.gap
		}
		x += 4
		h = y + textH + a.padY
		for i := range a.actionRects {
			a.actionRects[i].Origin = ggui.Pt(max(a.padX, w-12-actionW)+a.actionRects[i].Origin.X, 12)
		}
	} else {
		if hasMedia {
			h = max(h, a.mediaSide)
			a.mediaRect = ggui.Rct(ggui.Pt(x, a.padY+(h-a.mediaSide)/2), ggui.Sz(a.mediaSide, a.mediaSide))
			x += a.mediaSide + a.gap
		}
		y += (h - textH) / 2
		for i := range a.actionRects {
			a.actionRects[i].Origin = ggui.Pt(max(a.padX, w-a.padX-actionW)+a.actionRects[i].Origin.X, a.padY+(h-a.actionRects[i].Size.H)/2)
		}
		h += 2 * a.padY
	}
	a.textRect = ggui.Rct(ggui.Pt(x, y), ggui.Sz(textW, title.H))
	a.descriptionRect = ggui.Rct(ggui.Pt(x, y+title.H+2), ggui.Sz(textW, desc.H))
	if a.media != nil {
		if a.mediaView == nil {
			a.mediaView = ggui.Styled(ggui.Center(a.media))
		}
		fg := a.theme.Fg
		if a.current == AttachmentError {
			fg = a.theme.Destructive
		}
		a.mediaView.Color(fg)
		a.mediaView.Layout(ggui.Tight(a.mediaRect.Size), env)
	}
	if a.trigger != nil {
		a.trigger.Layout(ggui.Loose(ggui.Sz(w, h)), env)
	}
	return c.Constrain(ggui.Sz(w, h))
}

func translated(r ggui.Rect, at ggui.Point) ggui.Rect { r.Origin = r.Origin.Add(at); return r }

func (a *AttachmentWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	state := a.current
	t := a.theme
	fill, border := mix(t.Bg, t.Muted, .4), t.Border
	if a.trigger != nil && a.trigger.Hovered {
		fill = mix(t.Card, t.Muted, .5)
	}
	if state == AttachmentError {
		border = mix(t.Destructive, t.Card, .7)
	}
	dst.FillRoundRect(r, a.radius, fill)
	if state == AttachmentIdle {
		attachmentDashedBorder(dst, r, a.radius, t.BorderWidth, border)
	} else {
		dst.StrokeRoundRect(r, a.radius, t.BorderWidth, border)
	}
	dst.Node(r, ggui.Node{Role: ggui.RoleGroup, Name: a.title, Description: string(state)}, func(dst *ggui.Canvas) {
		// Register the full-card trigger before painting the independent actions.
		if a.trigger != nil {
			a.trigger.Hit(dst, r, a.trigger, ggui.CursorShapePointer)
			a.trigger.FocusRing(dst, r, a.radius, t.Ring)
		}
		inner := dst.Clip(r)
		a.paintMedia(inner, translated(a.mediaRect, r.Origin), state)
		titleRect := translated(a.textRect, r.Origin)
		a.titleText.Paint(inner, titleRect)
		if !a.reduced && (state == AttachmentUploading || state == AttachmentProcessing) {
			// A moving highlight across the glyphs, rather than pulsing the whole card.
			phase := float64(ggui.Now().UnixMilli()%2000) / 2000
			center := (titleRect.Size.W+80)*phase - 40
			highlight := t.Card
			if isDark(t.Card) {
				highlight = color.White
			}
			for i := 0; i < 16; i++ {
				x := center + float64(i)*4 - 32
				band := ggui.Rct(titleRect.Origin.Add(ggui.Pt(x, 0.0)), ggui.Sz(4.0, titleRect.Size.H)).Intersect(titleRect)
				if band.Empty() {
					continue
				}
				strength := 1 - math.Abs(float64(i)-7.5)/8
				a.titleText.text.Color(mix(t.Fg, highlight, .8*strength))
				inner.Clip(band).Inert().Paint(a.titleText.text, titleRect)
			}
			a.titleText.text.Color(t.Fg)
		}
		if a.description != "" {
			a.descriptionText.Paint(inner, translated(a.descriptionRect, r.Origin))
		}
		for i, action := range a.actions {
			inner.Paint(action, translated(a.actionRects[i], r.Origin))
		}
	})
}

func (a *AttachmentWidget) paintMedia(dst *ggui.Canvas, r ggui.Rect, state AttachmentState) {
	if r.Empty() {
		return
	}
	radius := pick(a.size == AttachmentExtraSmall, 6.0, 8.0)
	bg := a.theme.Muted
	if state == AttachmentError {
		bg = mix(a.theme.Destructive, a.theme.Card, .9)
	}
	dst.FillRoundRect(r, radius, bg)
	if a.image != nil {
		if a.imageAlt != "" {
			dst.Leaf(r, ggui.Node{Role: ggui.RoleImage, Name: a.imageAlt})
		}
		if dst == nil || dst.Image == nil {
			return
		}
		cut := roundedCut(a.image, int(math.Ceil(float64(dst.Px(r.Size.W)))), float64(dst.Px(radius)))
		if cut == nil {
			return
		}
		op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
		op.GeoM.Scale(1/dst.Scale(), 1/dst.Scale())
		op.GeoM.Concat(dst.Geo(r.Origin))
		if state != AttachmentIdle && state != AttachmentDone {
			op.ColorScale.ScaleAlpha(.6)
		}
		dst.Image.DrawImage(cut, op)
	} else if a.media != nil {
		dst.Paint(a.mediaView, r)
	}
}

// attachmentText clips and ellipsizes visually while keeping the complete name
// in the semantics tree. Measurements use the same font and scale as drawing.
type attachmentText struct {
	text     *ggui.TextWidget
	value    string
	fitted   bool // truncation computed for value at fitWidth
	fitWidth float64
}

func (t *attachmentText) set(value string, size float64, col color.Color) {
	t.value = strings.ReplaceAll(value, "\n", " ")
	if t.text == nil {
		t.text = ggui.Text(t.value).NoWrap()
	}
	t.text.Content(t.value).Size(size).LineHeight(1.25).Color(col)
	t.fitted = false
}
func (t *attachmentText) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	if t.fitted && t.fitWidth == c.MaxW {
		return t.text.Layout(c, env)
	}
	t.fitted, t.fitWidth = true, c.MaxW
	t.text.Content(t.value)
	natural := t.text.Layout(ggui.Loose(ggui.Sz(ggui.Unbounded, c.MaxH)), env)
	if natural.W > c.MaxW {
		runes := []rune(t.value)
		lo, hi := 0, len(runes)
		for lo < hi {
			mid := (lo + hi + 1) / 2
			t.text.Content(string(runes[:mid]) + "…")
			if t.text.Layout(ggui.Loose(ggui.Sz(ggui.Unbounded, c.MaxH)), env).W <= c.MaxW {
				lo = mid
			} else {
				hi = mid - 1
			}
		}
		t.text.Content(string(runes[:lo]) + "…")
	}
	return t.text.Layout(c, env)
}
func (t *attachmentText) Paint(dst *ggui.Canvas, r ggui.Rect) {
	if t.value == "" || r.Empty() {
		return
	}
	dst.Leaf(r, ggui.Node{Role: ggui.RoleText, Name: t.value})
	dst.Clip(r).Inert().Paint(t.text, r)
}

// The idle treatment is a dashed rounded border, including the corners.
func attachmentDashedBorder(dst *ggui.Canvas, r ggui.Rect, radius, width float64, col color.Color) {
	inset := width / 2
	x, y := r.Origin.X+inset, r.Origin.Y+inset
	w, h := max(0, r.Size.W-width), max(0, r.Size.H-width)
	radius = max(0, min(radius-inset, min(w, h)/2))
	straightX, straightY := w-2*radius, h-2*radius
	arc := math.Pi * radius / 2
	lengths := []float64{straightX, arc, straightY, arc, straightX, arc, straightY, arc}
	point := func(d float64) ggui.Point {
		segment := 0
		for segment < 7 && d > lengths[segment] {
			d -= lengths[segment]
			segment++
		}
		switch segment {
		case 0:
			return ggui.Pt(x+radius+d, y)
		case 2:
			return ggui.Pt(x+w, y+radius+d)
		case 4:
			return ggui.Pt(x+w-radius-d, y+h)
		case 6:
			return ggui.Pt(x, y+h-radius-d)
		}
		theta := float64(segment-1)*math.Pi/4 - math.Pi/2
		if radius > 0 {
			theta += d / radius
		}
		cx, cy := x+w-radius, y+radius
		if segment >= 3 {
			cy = y + h - radius
		}
		if segment >= 5 {
			cx = x + radius
		}
		if segment == 7 {
			cy = y + radius
		}
		return ggui.Pt(cx+radius*math.Cos(theta), cy+radius*math.Sin(theta))
	}
	total := 2*(straightX+straightY) + 4*arc
	for d := 0.0; d < total; d += 7 {
		for at := d; at < min(d+4, total); at += 1 {
			dst.StrokeLine(point(at), point(min(at+1, min(d+4, total))), width, col)
		}
	}
}
