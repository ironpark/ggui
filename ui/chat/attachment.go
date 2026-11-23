package chat

import (
	"image/color"
	"strings"
	"time"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/property"
	"github.com/ironpark/ggui/ui"
	"github.com/ironpark/ggui/ui/internal/controlkit"
	uitheme "github.com/ironpark/ggui/ui/theme"
	"github.com/ironpark/ggui/widget"
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
// Geometry follows shadcn/ui's default attachment; colors and corners follow
// the theme.
type AttachmentWidget struct {
	props                                property.Owner
	title, description                   string
	titleText, descriptionText           attachmentText
	media                                ggui.Widget
	mediaView                            *widget.StyledWidget
	image                                *ggfx.Image
	imageAlt                             string
	actions                              []ggui.Widget
	trigger                              *ui.ButtonWidget
	state                                AttachmentState
	words                                *ui.Strings // the Strings of the last layout
	stateReader                          ggui.Readable[AttachmentState]
	size                                 AttachmentSize
	vertical                             bool
	width                                float64
	theme                                *uitheme.Theme
	reduced                              bool
	motion                               time.Duration
	current                              AttachmentState
	padX, padY, gap, radius, mediaSide   float64
	mediaRect, textRect, descriptionRect ggui.Rect
	actionRects                          []ggui.Rect
}

// The widths of a vertical attachment: with a title or a description under
// its media, and with the media alone.
const (
	attachmentVerticalWidth = 120.0
	attachmentMediaWidth    = 96.0
)

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
func (a *AttachmentWidget) Image(img *ggfx.Image, alt string) *AttachmentWidget {
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
		a.trigger = ui.ButtonOf(widget.Box(), fn).Name(name).Ghost().Pad(0)
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
func AttachmentAction(name string, content ggui.Widget, fn func()) *ui.ButtonWidget {
	return ui.ButtonOf(widget.Box(widget.Center(content)).Width(24).Height(24), fn).Name(name).Ghost().Pad(0)
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

// attachmentMeasure carries one Layout's measurements from phase to phase:
// the card's size, the text column's width, the title's and description's
// sizes and the text's height, and the extent of the row of actions.
type attachmentMeasure struct {
	w, h             float64
	textW, textH     float64
	title, desc      ggui.Size
	actionW, actionH float64
}

func (a *AttachmentWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer a.props.Layout().EndLayout()
	a.theme, a.reduced, a.current = uitheme.Of(env), env.ReducedMotion(), a.uploadState()
	a.words = ui.StringsOf(env)
	a.motion = env.Motion(a.theme.MotionFast)
	fontSize := a.layoutMetrics()
	// The title is the body font in medium weight; an error's description
	// is the destructive color at 80%.
	a.titleText.set(a.title, fontSize, a.theme.Fg)
	a.titleText.text.Weight(ggui.WeightMedium)
	descColor := a.theme.MutedFg
	if a.current == AttachmentError {
		descColor = fade(a.theme.Destructive, .8)
	}
	a.descriptionText.set(a.description, 12, descColor)
	var m attachmentMeasure
	free := ggui.Loose(ggui.Sz(ggui.Unbounded, ggui.Unbounded))
	m.title = a.titleText.Layout(free, env)
	m.desc = a.descriptionText.Layout(free, env)
	m.actionW, m.actionH = a.layoutActions(c, env)
	m.w = a.cardWidth(c, max(m.title.W, m.desc.W), m.actionW)
	m.textW = a.fitMedia(m.w, m.actionW)
	m.title = a.titleText.Layout(ggui.Loose(ggui.Sz(m.textW, ggui.Unbounded)), env)
	m.desc = a.descriptionText.Layout(ggui.Loose(ggui.Sz(m.textW, ggui.Unbounded)), env)
	m.textH = m.title.H
	if a.description != "" {
		m.textH += 2 + m.desc.H
	}
	var at ggui.Point
	if a.vertical {
		at = a.placeVertical(&m)
	} else {
		at = a.placeHorizontal(&m)
	}
	a.textRect = ggui.Rct(at, ggui.Sz(m.textW, m.title.H))
	a.descriptionRect = ggui.Rct(ggui.Pt(at.X, at.Y+m.title.H+2), ggui.Sz(m.textW, m.desc.H))
	a.layoutMedia(env)
	if a.trigger != nil {
		a.trigger.Layout(ggui.Loose(ggui.Sz(m.w, m.h)), env)
	}
	return c.Constrain(ggui.Sz(m.w, m.h))
}

func (a *AttachmentWidget) hasMedia() bool   { return a.media != nil || a.image != nil }
func (a *AttachmentWidget) hasContent() bool { return a.title != "" || a.description != "" }

// layoutMetrics sets the paddings, gap, radius and media side of the card's
// size and returns the title's font size.
func (a *AttachmentWidget) layoutMetrics() float64 {
	a.padX, a.padY, a.gap, a.mediaSide = 10, 8, 8, 40
	a.radius = a.theme.Chat.AttachmentRadius
	fontSize := 14.0
	switch a.size {
	case AttachmentSmall:
		a.padX, a.padY, a.gap, a.mediaSide, fontSize = 8, 6, 10, 32, 12
	case AttachmentExtraSmall:
		a.padX, a.padY, a.gap, a.radius, a.mediaSide, fontSize = 6, 4, 6, a.theme.Chat.AttachmentXSRadius, 28, 12
	}
	if a.hasMedia() {
		a.padX = a.padY
	}
	// Reserve the border separately from content padding, like CSS border-box.
	a.padX += 1
	a.padY += 1
	return fontSize
}

// layoutActions lays the actions out in a row from the origin, 4px apart on
// a vertical card, and returns the row's width and height.
func (a *AttachmentWidget) layoutActions(c ggui.Constraints, env ggui.Env) (float64, float64) {
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
	return actionW, actionH
}

// cardWidth is the card's width within c: a vertical card's fixed width,
// else the text's natural width textW with the media and the actions beside
// it, unless Width set one.
func (a *AttachmentWidget) cardWidth(c ggui.Constraints, textW, actionW float64) float64 {
	w := a.padX*2 + textW
	// A card with no text is media alone: shadcn's w-24, which
	// has-data-[slot=attachment-content] widens to w-30.
	if a.vertical {
		w = pick(a.hasContent(), attachmentVerticalWidth, attachmentMediaWidth)
	} else {
		if a.hasMedia() {
			w += a.mediaSide + a.gap
		}
		if len(a.actions) > 0 {
			w += actionW + a.gap
		}
		w = max(160, w)
	}
	if a.width > 0 {
		w = a.width
	}
	return min(c.MaxW, max(c.MinW, w))
}

// fitMedia sizes the media within the card of width w, as wide as the card
// on a vertical one, and returns the width left for the text.
func (a *AttachmentWidget) fitMedia(w, actionW float64) float64 {
	inner := max(0, w-2*a.padX)
	a.mediaRect = ggui.Rect{}
	if a.vertical {
		if a.hasMedia() {
			a.mediaSide = inner
			a.mediaRect = ggui.Rct(ggui.Pt(a.padX, a.padY), ggui.Sz(inner, inner))
		}
		return max(0, inner-8)
	}
	textW := inner
	if a.hasMedia() {
		a.mediaSide = min(a.mediaSide, inner)
		textW -= a.mediaSide + a.gap
	}
	if len(a.actions) > 0 {
		textW -= actionW + a.gap
	}
	return max(0, textW)
}

// placeVertical sets m's height for the text under the media, puts the
// actions over the media's top right corner, and returns the text's origin.
func (a *AttachmentWidget) placeVertical(m *attachmentMeasure) ggui.Point {
	x, y := a.padX, a.padY
	if a.hasMedia() {
		y += a.mediaSide + a.gap
	}
	x += 4
	m.h = y + m.textH + a.padY
	if !a.hasContent() {
		// Nothing under the media, and no gap kept for it.
		m.h = a.padY + pick(a.hasMedia(), a.mediaSide, 0) + a.padY
	}
	for i := range a.actionRects {
		a.actionRects[i].Origin = ggui.Pt(max(a.padX, m.w-12-m.actionW)+a.actionRects[i].Origin.X, 12)
	}
	return ggui.Pt(x, y)
}

// placeHorizontal sets m's height for the media, the text and the actions
// side by side, each centered on the row, and returns the text's origin.
func (a *AttachmentWidget) placeHorizontal(m *attachmentMeasure) ggui.Point {
	x, y := a.padX, a.padY
	m.h = max(m.textH, m.actionH)
	if a.hasMedia() {
		m.h = max(m.h, a.mediaSide)
		a.mediaRect = ggui.Rct(ggui.Pt(x, a.padY+(m.h-a.mediaSide)/2), ggui.Sz(a.mediaSide, a.mediaSide))
		x += a.mediaSide + a.gap
	}
	y += (m.h - m.textH) / 2
	for i := range a.actionRects {
		a.actionRects[i].Origin = ggui.Pt(max(a.padX, m.w-a.padX-m.actionW)+a.actionRects[i].Origin.X, a.padY+(m.h-a.actionRects[i].Size.H)/2)
	}
	m.h += 2 * a.padY
	return ggui.Pt(x, y)
}

// layoutMedia lays a media widget out in the media rect, in the
// destructive color on an error.
func (a *AttachmentWidget) layoutMedia(env ggui.Env) {
	if a.media == nil {
		return
	}
	if a.mediaView == nil {
		a.mediaView = widget.Styled(widget.Center(a.media))
	}
	fg := a.theme.Fg
	if a.current == AttachmentError {
		fg = a.theme.Destructive
	}
	a.mediaView.Color(fg)
	a.mediaView.Layout(ggui.Tight(a.mediaRect.Size), env)
}

func translated(r ggui.Rect, at ggui.Point) ggui.Rect { r.Origin = r.Origin.Add(at); return r }

// attachmentHoverSlot keeps a card's hover easing across frames.
var attachmentHoverSlot = ggui.NewSlot[*ggui.Motion]("attachment hover")

func (a *AttachmentWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	state := a.current
	t := a.theme
	// The card surface, half the muted one under the pointer when the card
	// opens something, inside the border, or the destructive color at 30%
	// for an error.
	fill, border := t.Card, t.Border
	if a.trigger != nil {
		// transition-colors: the hover surface eases in and out.
		hover := controlkit.Ease(dst, a.theme, a.trigger.Anchor(r), attachmentHoverSlot, pick(a.trigger.Hovered, 1.0, 0.0), a.motion)
		fill = mix(fill, fade(t.Muted, .5), hover)
	}
	if state == AttachmentError {
		border = fade(t.Destructive, .3)
	}
	dst.FillRoundRect(r, a.radius, fill)
	if state == AttachmentIdle {
		ui.DashedBorder(dst, r, a.radius, t.BorderWidth, border)
	} else {
		dst.StrokeRoundRect(r, a.radius, t.BorderWidth, border)
	}
	dst.Node(r, ggui.Node{Role: ggui.RoleGroup, Name: a.title, Description: attachmentState(a.words, state)}, func(dst *ggui.Canvas) {
		// Register the full-card trigger before painting the independent actions.
		if a.trigger != nil {
			a.trigger.Hit(dst, r, a.trigger, ggui.CursorShapePointer)
			// shadcn's focus-within:ring-1 ring-ring/50: a 1px ring just
			// outside the border, which stays as it is.
			if tr := &a.trigger.Interactive; tr.Focused && tr.FocusVisible {
				dst.StrokeRoundRect(r.Grow(t.BorderWidth/2), a.radius+t.BorderWidth/2, t.BorderWidth, fade(t.Ring, .5))
			}
		}
		inner := dst.Clip(r)
		a.paintMedia(inner, translated(a.mediaRect, r.Origin), state)
		titleRect := translated(a.textRect, r.Origin)
		a.titleText.Paint(inner, titleRect)
		if !a.reduced && (state == AttachmentUploading || state == AttachmentProcessing) && !inner.Visible(titleRect).Empty() {
			// A moving highlight across the glyphs, rather than pulsing the
			// whole card: from the foreground toward the muted one, which
			// reads on the card surface whichever way the theme runs.
			paintShimmer(inner, a.titleText.text, titleRect, t.Fg, t.MutedFg)
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
	// shadcn's rounded-lg, rounded-md for the extra small size, and a tenth
	// of the destructive color for an error.
	radius := pick(a.size == AttachmentExtraSmall, a.theme.RadiusSm, a.theme.Radius)
	bg := a.theme.Muted
	if state == AttachmentError {
		bg = fade(a.theme.Destructive, .1)
	}
	dst.FillRoundRect(r, radius, bg)
	if a.image != nil {
		if a.imageAlt != "" {
			dst.Leaf(r, ggui.Node{Role: ggui.RoleImage, Name: a.imageAlt})
		}
		o := ggui.ImageOptions{Fit: ggui.FitCover, Radius: radius}
		if state != AttachmentIdle && state != AttachmentDone {
			o.Fade = .4
		}
		dst.DrawImage(a.image, r, o)
	} else if a.media != nil {
		dst.Paint(a.mediaView, r)
	}
}

// attachmentText ends a name too long for its line with an ellipsis while
// keeping the complete name in the semantics tree.
type attachmentText struct {
	text  *widget.TextWidget
	value string
}

func (t *attachmentText) set(value string, size float64, col color.Color) {
	t.value = strings.ReplaceAll(value, "\n", " ")
	if t.text == nil {
		t.text = widget.Text(t.value).NoWrap().Ellipsis()
	}
	t.text.Content(t.value).Size(size).LineHeight(1.25).Color(col)
}
func (t *attachmentText) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	return t.text.Layout(c, env)
}
func (t *attachmentText) Paint(dst *ggui.Canvas, r ggui.Rect) {
	if t.value == "" || r.Empty() {
		return
	}
	dst.Leaf(r, ggui.Node{Role: ggui.RoleText, Name: t.value})
	dst.Clip(r).Inert().Paint(t.text, r)
}
