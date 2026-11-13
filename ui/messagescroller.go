package ui

import (
	"image/color"
	"math"
	"slices"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// MessageEntry is a stable transcript row. Content can be a Message, Marker,
// loading control or any widget. Anchor starts a new conversational turn.
type MessageEntry struct {
	ID      string
	Content ggui.Widget
	Anchor  bool
}
type ScrollPosition string

const (
	ScrollStart      ScrollPosition = "start"
	ScrollEnd        ScrollPosition = "end"
	ScrollLastAnchor ScrollPosition = "last-anchor"
)

type ScrollAlignment string

const (
	ScrollAlignStart   ScrollAlignment = "start"
	ScrollAlignCenter  ScrollAlignment = "center"
	ScrollAlignEnd     ScrollAlignment = "end"
	ScrollAlignNearest ScrollAlignment = "nearest"
)

// TranscriptPosition survives history prepends and height changes by naming
// the top visible row and the offset within it rather than only a pixel value.
type TranscriptPosition struct {
	MessageID string
	Offset    float64
	Following bool
}
type TranscriptVisibility struct {
	CurrentAnchorID              string
	VisibleMessageIDs            []string
	CanScrollStart, CanScrollEnd bool
}

// MessageScrollerWidget owns viewport behavior, never message storage or I/O.
// Keep it alive across transcript updates. AutoScroll defaults to false; the
// first nonempty transcript opens at the end without painting at the wrong edge.
type MessageScrollerWidget struct {
	props property.Owner
	ggui.Interactive
	source                                                        ggui.Readable[[]MessageEntry]
	rows                                                          []MessageEntry
	starts                                                        []float64
	sizes                                                         []ggui.Size
	offset, total, viewport, height, gap, margin, peek, threshold float64
	opening                                                       ScrollPosition
	opened, auto, following                                       bool
	anchor                                                        string
	rect                                                          ggui.Rect
	theme                                                         *uitheme.Theme
	duration                                                      time.Duration
	animFrom, animTo                                              float64
	animStart                                                     time.Time
	// animFor is how long the move under way takes, and stepping that it
	// is a wheel's notch easing, which the next notch adds to.
	animFor                time.Duration
	stepping               bool
	reduced                bool
	pendingID              string
	pendingAlign           ScrollAlignment
	restore                *TranscriptPosition
	visibility             TranscriptVisibility
	onVisibility           func(TranscriptVisibility)
	startButton, endButton *ButtonWidget
	pause                  func()
	// The scroll-to-edge buttons: their sizes, how far each has eased in,
	// and how long easing in and out takes.
	startSize, endSize   ggui.Size
	startShown, endShown ggui.Motion
	buttonIn, buttonOut  time.Duration
	fadeLen, buttonInset float64 // the faded end's length, and a button's distance from its edge
	// What paintRows paints into and reports to, and paintRows as a value,
	// so painting the rows behind a fade makes no closure a frame.
	rowsRect    ggui.Rect
	painting    *TranscriptVisibility
	paintRowsFn func(*ggui.Canvas)
}

// MessageScroller creates a transcript viewport over items, 320px tall, with
// three spaces between rows until Gap says otherwise. A round icon button
// eases in at an edge the reader has scrolled away from and scrolls back to it.
func MessageScroller(items ggui.Readable[[]MessageEntry]) *MessageScrollerWidget {
	s := &MessageScrollerWidget{source: items, height: 320, gap: -1, peek: 64, threshold: 8, opening: ScrollEnd, duration: 200 * time.Millisecond}
	s.Role = ggui.RoleGroup
	s.SetName("Conversation")
	s.AutoKey()
	s.pause = s.Pause
	s.paintRowsFn = s.paintRows
	s.startButton = ButtonOf(&scrollEdgeGlyph{up: true}, func() { s.ScrollToStart() }).Name("Scroll to start").Variant(scrollEdgeButton).Size(ButtonIconSmall)
	s.endButton = ButtonOf(&scrollEdgeGlyph{}, func() { s.ScrollToEnd() }).Name("Scroll to end").Variant(scrollEdgeButton).Size(ButtonIconSmall)
	return s
}

// scrollEdgeButton is the look of a transcript's scroll-to-edge button: the
// page's background inside a border, and the muted surface under the pointer.
var scrollEdgeButton = NewVariant("message-scroller", func(t uitheme.Theme) ButtonStyle {
	return ButtonStyle{Fill: t.Bg, Hover: t.Muted, Label: t.Fg, HoverLabel: t.Fg, Border: t.Border}
})

// The curves a scroll-to-edge button eases in and out along.
var (
	scrollButtonIn  = ggui.CubicBezier(.23, 1, .32, 1)
	scrollButtonOut = ggui.CubicBezier(.7, 0, .84, 0)
)

// scrollEdgeGlyph is the arrow on a scroll-to-edge button, pointing down to
// the end or up to the start.
type scrollEdgeGlyph struct {
	up  bool
	env ggui.Env
	ink color.Color
}

func (g *scrollEdgeGlyph) Layout(cs ggui.Constraints, env ggui.Env) ggui.Size {
	t := uitheme.Of(env)
	g.env, g.ink = env, colorOr(env.Text().Color, t.Fg)
	return cs.Constrain(ggui.Sz(t.IconSize, t.IconSize))
}

func (g *scrollEdgeGlyph) Paint(dst *ggui.Canvas, r ggui.Rect) {
	paintChatIcon(dst, g.env, iconArrowDown, r, g.ink, pick(g.up, math.Pi, 0))
}

// Name names the transcript for assistive technology.
func (s *MessageScrollerWidget) Name(v string) *MessageScrollerWidget { s.SetName(v); return s }
func (s *MessageScrollerWidget) Height(v float64) *MessageScrollerWidget {
	defer property.Watch(&s.props, &s.height)()
	s.height = max(0, v)
	return s
}
func (s *MessageScrollerWidget) Gap(v float64) *MessageScrollerWidget {
	defer property.Watch(&s.props, &s.gap)()
	s.gap = max(0, v)
	return s
}
func (s *MessageScrollerWidget) AutoScroll(v bool) *MessageScrollerWidget {
	defer property.Watch(&s.props, &s.auto)()
	defer property.Watch(&s.props, &s.following)()
	s.auto = v
	if !v {
		s.following = false
	}
	return s
}
func (s *MessageScrollerWidget) Opening(v ScrollPosition) *MessageScrollerWidget {
	defer property.Watch(&s.props, &s.opening)()
	s.opening = v
	return s
}
func (s *MessageScrollerWidget) ScrollMargin(v float64) *MessageScrollerWidget {
	defer property.Watch(&s.props, &s.margin)()
	s.margin = max(0, v)
	return s
}
func (s *MessageScrollerWidget) PreviousPeek(v float64) *MessageScrollerWidget {
	defer property.Watch(&s.props, &s.peek)()
	s.peek = max(0, v)
	return s
}
func (s *MessageScrollerWidget) EdgeThreshold(v float64) *MessageScrollerWidget {
	defer property.Watch(&s.props, &s.threshold)()
	s.threshold = max(0, v)
	return s
}
func (s *MessageScrollerWidget) Animation(d time.Duration) *MessageScrollerWidget {
	defer property.Watch(&s.props, &s.duration)()
	s.duration = max(0, d)
	return s
}
func (s *MessageScrollerWidget) OnVisibility(fn func(TranscriptVisibility)) *MessageScrollerWidget {
	s.onVisibility = fn
	return s
}
func (s *MessageScrollerWidget) Position() float64 { return s.offset }
func (s *MessageScrollerWidget) Visibility() TranscriptVisibility {
	v := s.visibility
	v.VisibleMessageIDs = slices.Clone(v.VisibleMessageIDs)
	return v
}
func (s *MessageScrollerWidget) limit() float64 { return max(0, s.total-s.viewport) }
func (s *MessageScrollerWidget) index(id string) int {
	return slices.IndexFunc(s.rows, func(row MessageEntry) bool { return row.ID == id })
}

// clearTargets drops every queued scroll destination; the caller sets its own.
func (s *MessageScrollerWidget) clearTargets() { s.pendingID, s.restore, s.anchor = "", nil, "" }

// Pause releases output following and smooth scrolling, without moving the reader.
// Pause stops following the end, and a move a command began. A wheel's
// notch easing goes on: it is the reader's own scroll, and the input that
// pauses, a move of the pointer among it, comes while it eases.
func (s *MessageScrollerWidget) Pause() {
	s.following = false
	if !s.stepping {
		s.animStart = time.Time{}
	}
}
func (s *MessageScrollerWidget) jump(v float64, animate bool) {
	v = clamp(v, 0, s.limit())
	if animate && !s.reduced && s.duration > 0 {
		s.animFrom, s.animTo, s.animStart = s.offset, v, ggui.Now()
		s.animFor, s.stepping = s.duration, false
	} else {
		s.offset = v
		s.animStart, s.stepping = time.Time{}, false
	}
}
func (s *MessageScrollerWidget) ScrollToStart() {
	s.clearTargets()
	s.Pause()
	s.jump(0, true)
}
func (s *MessageScrollerWidget) ScrollToEnd() {
	s.clearTargets()
	s.following = s.auto
	s.jump(s.limit(), true)
}

// ScrollToMessage queues a target even before the first layout. A missing ID
// stays pending until it arrives; later commands replace the pending target.
func (s *MessageScrollerWidget) ScrollToMessage(id string, align ScrollAlignment) {
	s.Pause()
	s.clearTargets()
	s.pendingID, s.pendingAlign = id, align
}
func (s *MessageScrollerWidget) Save() TranscriptPosition {
	for i, row := range s.rows {
		if s.starts[i]+s.sizes[i].H > s.offset {
			return TranscriptPosition{row.ID, s.offset - s.starts[i], s.following}
		}
	}
	return TranscriptPosition{Following: s.following}
}
func (s *MessageScrollerWidget) Restore(p TranscriptPosition) {
	s.clearTargets()
	s.restore = &p
	s.Pause()
}
func (s *MessageScrollerWidget) target(i int, align ScrollAlignment) float64 {
	start, end := s.starts[i], s.starts[i]+s.sizes[i].H
	switch align {
	case ScrollAlignCenter:
		return (start + end - s.viewport) / 2
	case ScrollAlignEnd:
		return end - s.viewport + s.margin
	case ScrollAlignNearest:
		if start >= s.offset+s.margin && end <= s.offset+s.viewport-s.margin {
			return s.offset
		}
		if start < s.offset+s.margin {
			return start - s.margin
		}
		return end - s.viewport + s.margin
	}
	return start - s.margin
}
func (s *MessageScrollerWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer s.props.Layout().EndLayout()
	saved := s.Save()
	oldRows := s.rows
	s.theme, s.reduced = uitheme.Of(env), env.ReducedMotion()
	t := s.theme
	gap := s.gap
	if gap < 0 {
		gap = 3 * t.Space
	}
	s.fadeLen, s.buttonInset = 3*t.Space, 2*t.Space
	s.buttonIn, s.buttonOut = env.Motion(t.MotionSlow), env.Motion(2*t.MotionSlow)
	size := c.Constrain(ggui.Sz(bounded(c.MaxW, 400), min(c.MaxH, s.height)))
	s.viewport = size.H
	rows := s.source.Get()
	s.rows = slices.Clone(rows)
	s.starts, s.sizes = s.starts[:0], s.sizes[:0]
	seen := map[string]bool{}
	s.total = 0
	lastOld := -1
	for i, row := range rows {
		if row.ID == "" || seen[row.ID] {
			panic("ui.MessageScroller: message IDs must be nonempty and unique")
		}
		seen[row.ID] = true
		if row.Content == nil {
			panic("ui.MessageScroller: nil content")
		}
		if i > 0 {
			s.total += gap
		}
		s.starts = append(s.starts, s.total)
		sz := row.Content.Layout(ggui.Loose(ggui.Sz(size.W, ggui.Unbounded)), env)
		s.sizes = append(s.sizes, sz)
		s.total += sz.H
		if len(oldRows) > 0 && row.ID == oldRows[len(oldRows)-1].ID {
			lastOld = i
		}
	}
	if len(rows) > 0 && !s.opened {
		s.opened = true
		s.offset = s.limit()
		if s.opening == ScrollStart {
			s.offset = 0
		}
		if s.opening == ScrollLastAnchor {
			for i := len(rows) - 1; i >= 0; i-- {
				if rows[i].Anchor {
					s.offset = min(s.starts[i]-s.margin, s.limit())
					break
				}
			}
		}
		s.following = s.auto && s.limit()-s.offset <= s.threshold
	} else if len(rows) > 0 {
		if s.following && s.auto {
			s.offset = s.limit()
		} else {
			if i := s.index(saved.MessageID); i >= 0 {
				s.offset = s.starts[i] + saved.Offset
			}
		}
		if lastOld >= 0 {
			for i := lastOld + 1; i < len(rows); i++ {
				if rows[i].Anchor {
					s.anchor = rows[i].ID
					s.offset = max(0, s.starts[i]-s.margin-s.peek)
					s.following = s.auto
				}
			}
		}
	}
	if i := s.index(s.anchor); s.anchor != "" && i >= 0 {
		start := max(0, s.starts[i]-s.margin-s.peek)
		s.total = max(s.total, start+s.viewport)
		if s.following {
			s.offset = max(start, s.limit())
		}
	}
	s.offset = clamp(s.offset, 0, s.limit())
	s.startSize = s.startButton.Layout(c.Loosen(), env)
	s.endSize = s.endButton.Layout(c.Loosen(), env)
	return size
}
func (s *MessageScrollerWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	s.rect = r
	dst.HitPointer(r, s)
	dst.HitKey(r, s)
	dst.ObserveInput(r, s.pause)
	if s.restore != nil {
		if i := s.index(s.restore.MessageID); i >= 0 {
			s.jump(s.starts[i]+s.restore.Offset, false)
			s.following = s.restore.Following && s.auto
			s.restore = nil
		}
	}
	if s.pendingID != "" {
		if i := s.index(s.pendingID); i >= 0 {
			s.jump(s.target(i, s.pendingAlign), true)
			s.pendingID = ""
		}
	}
	if !s.animStart.IsZero() {
		f := min(1, float64(ggui.Now().Sub(s.animStart))/float64(s.animFor))
		s.offset = s.animFrom + (s.animTo-s.animFrom)*(1-math.Pow(1-f, 3))
		if f >= 1 {
			s.animStart, s.stepping = time.Time{}, false
		}
	}
	s.offset = clamp(s.offset, 0, s.limit())
	v := TranscriptVisibility{CanScrollStart: s.offset > s.threshold, CanScrollEnd: s.limit()-s.offset > s.threshold}
	dst.DescribeNode(r, s, func(dst *ggui.Canvas) {
		clip := dst.Clip(r)
		// The rows fade out toward an end there is more beyond, by as much
		// as there is, up to fadeLen: a mask, so whatever surface is behind
		// the transcript shows through it.
		s.rowsRect, s.painting = r, &v
		clip.FadeEdges(r, false, 0, min(s.limit()-s.offset, s.fadeLen), s.paintRowsFn)
		s.painting = nil
		s.paintEdgeButton(clip, r, s.startButton, &s.startShown, s.startSize, v.CanScrollStart, -1)
		s.paintEdgeButton(clip, r, s.endButton, &s.endShown, s.endSize, v.CanScrollEnd, 1)
	})
	changed := v.CurrentAnchorID != s.visibility.CurrentAnchorID || v.CanScrollStart != s.visibility.CanScrollStart || v.CanScrollEnd != s.visibility.CanScrollEnd || !slices.Equal(v.VisibleMessageIDs, s.visibility.VisibleMessageIDs)
	s.visibility = v
	if changed && s.onVisibility != nil {
		s.onVisibility(s.Visibility())
	}
	s.FocusRing(dst, r, 0, s.theme.Ring)
}

// paintRows paints the transcript's rows over rowsRect and notes in painting
// which are in view and which anchor leads them.
func (s *MessageScrollerWidget) paintRows(dst *ggui.Canvas) {
	r, v := s.rowsRect, s.painting
	for i, row := range s.rows {
		y := s.starts[i] - s.offset
		if row.Anchor && y <= s.margin+s.peek {
			v.CurrentAnchorID = row.ID
		}
		if y+s.sizes[i].H > 0 && y < r.Size.H {
			v.VisibleMessageIDs = append(v.VisibleMessageIDs, row.ID)
		}
		dst.Paint(row.Content, ggui.Rct(r.Origin.Add(ggui.Pt(0.0, y)), ggui.Sz(r.Size.W, s.sizes[i].H)))
	}
}

// paintEdgeButton paints a scroll-to-edge button in the middle of the edge
// that side names, -1 the start and 1 the end. It eases in while active,
// and out, twice as slowly, when the reader is back at the edge: fading,
// shrinking a little and sliding its own height toward the edge, taking no
// input on its way out.
func (s *MessageScrollerWidget) paintEdgeButton(dst *ggui.Canvas, r ggui.Rect, b *ButtonWidget, shown *ggui.Motion, size ggui.Size, active bool, side float64) {
	shown.Easing(pick(active, scrollButtonIn, scrollButtonOut))
	v := shown.Toggle(active, ggui.FrameTime(), pick(active, s.buttonIn, s.buttonOut))
	if v <= 0 {
		return
	}
	y := s.buttonInset
	if side > 0 {
		y = max(0, r.Size.H-s.buttonInset-size.H)
	}
	at := ggui.Rct(r.Origin.Add(ggui.Pt((r.Size.W-size.W)/2, y+side*(1-v)*size.H)), size)
	if v >= 1 && active {
		dst.Paint(b, at)
		return
	}
	dst.Layer(ggui.LayerOptions{Fade: 1 - v, Scale: .95 + .05*v, About: at.Center()}, func(dst *ggui.Canvas) {
		if !active {
			dst = dst.Inert()
		}
		dst.Paint(b, at)
	})
}

// Describe implements ggui.Describer.
func (s *MessageScrollerWidget) Describe() ggui.Node {
	return ggui.Node{Role: ggui.RoleGroup, Name: s.SemanticName(), Actions: ggui.ActionFocus | ggui.ActionScrollIntoView}
}

// wheelEase is how long a wheel's notch takes to scroll a transcript, as
// long as a Scroll takes.
const wheelEase = 150 * time.Millisecond

func (s *MessageScrollerWidget) HandlePointer(ev ggui.PointerEvent) bool {
	if ev.Kind != ggui.PointerScroll || ev.Scroll.Y == 0 {
		return false
	}
	before, by := s.offset, ev.ScrollBy().Y
	if ev.ScrollStep && !ev.ScrollMomentum && !s.reduced && s.duration > 0 {
		// A wheel's notch is eased over, as Scroll eases it, and one that
		// comes while the last is easing adds to where that was heading.
		heading := before
		if s.stepping && !s.animStart.IsZero() {
			heading = s.animTo
		}
		s.Pause()
		s.anchor = ""
		if to := clamp(heading-by, 0, s.limit()); to != before {
			s.animFrom, s.animTo, s.animStart = before, to, ggui.Now()
			s.animFor, s.stepping = wheelEase, true
		}
		return s.limit() > 0
	}
	s.Pause()
	s.anchor = ""
	s.jump(before-by, false)
	return s.limit() > 0 && (!ev.ScrollMomentum || s.offset != before)
}
func (s *MessageScrollerWidget) HandleKey(ev ggui.KeyEvent) {
	s.Keyboard(ev, nil)
	if ev.Kind != ggui.KeyPress {
		return
	}
	s.Pause()
	s.anchor = ""
	switch ev.Key {
	case ggui.KeyArrowUp:
		s.jump(s.offset-40, false)
	case ggui.KeyArrowDown:
		s.jump(s.offset+40, false)
	case ggui.KeyPageUp:
		s.jump(s.offset-s.viewport*.9, false)
	case ggui.KeyPageDown:
		s.jump(s.offset+s.viewport*.9, false)
	case ggui.KeyHome:
		s.jump(0, false)
	case ggui.KeyEnd:
		s.ScrollToEnd()
	}
}
func (s *MessageScrollerWidget) ConsumesKey(ev ggui.KeyEvent) bool {
	switch ev.Key {
	case ggui.KeyArrowUp, ggui.KeyArrowDown, ggui.KeyPageUp, ggui.KeyPageDown, ggui.KeyHome, ggui.KeyEnd:
		return ev.Kind == ggui.KeyPress
	}
	return false
}
func (s *MessageScrollerWidget) Reveal(target ggui.Rect) {
	if target.Origin.X+target.Size.W <= s.rect.Origin.X || target.Origin.X >= s.rect.Origin.X+s.rect.Size.W {
		return
	}
	s.Pause()
	top, bottom := target.Origin.Y-s.rect.Origin.Y, target.Origin.Y+target.Size.H-s.rect.Origin.Y
	if top < 0 {
		s.jump(s.offset+top, false)
	} else if bottom > s.viewport {
		s.jump(s.offset+bottom-s.viewport, false)
	}
}
