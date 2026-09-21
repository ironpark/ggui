package ui

import (
	"math"
	"slices"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
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
	theme                                                         ggui.Theme
	duration                                                      time.Duration
	animFrom, animTo                                              float64
	animStart                                                     time.Time
	reduced                                                       bool
	pendingID                                                     string
	pendingAlign                                                  ScrollAlignment
	restore                                                       *TranscriptPosition
	visibility                                                    TranscriptVisibility
	onVisibility                                                  func(TranscriptVisibility)
	startButton, endButton                                        *ButtonWidget
	pause                                                         func()
}

func MessageScroller(items ggui.Readable[[]MessageEntry]) *MessageScrollerWidget {
	s := &MessageScrollerWidget{source: items, height: 320, gap: 32, peek: 64, threshold: 8, opening: ScrollEnd, duration: 200 * time.Millisecond}
	s.Role = ggui.RoleGroup
	s.SetName("Conversation")
	s.AutoKey()
	s.pause = s.Pause
	s.startButton = Button("↑", func() { s.ScrollToStart() }).Name("Scroll to start").Outline().Pad(4, 10)
	s.endButton = Button("↓", func() { s.ScrollToEnd() }).Name("Scroll to end").Outline().Pad(4, 10)
	return s
}
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
func (s *MessageScrollerWidget) Pause() { s.following = false; s.animStart = time.Time{} }
func (s *MessageScrollerWidget) jump(v float64, animate bool) {
	v = clamp(v, 0, s.limit())
	if animate && !s.reduced && s.duration > 0 {
		s.animFrom, s.animTo, s.animStart = s.offset, v, ggui.Now()
	} else {
		s.offset = v
		s.animStart = time.Time{}
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
	defer s.props.Layout()()
	saved := s.Save()
	oldRows := s.rows
	s.theme, s.reduced = env.Theme(), env.ReducedMotion()
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
			s.total += s.gap
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
	s.startButton.Layout(c.Loosen(), env)
	s.endButton.Layout(c.Loosen(), env)
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
		f := min(1, float64(ggui.Now().Sub(s.animStart))/float64(s.duration))
		s.offset = s.animFrom + (s.animTo-s.animFrom)*(1-math.Pow(1-f, 3))
		if f >= 1 {
			s.animStart = time.Time{}
		}
	}
	s.offset = clamp(s.offset, 0, s.limit())
	v := TranscriptVisibility{CanScrollStart: s.offset > s.threshold, CanScrollEnd: s.limit()-s.offset > s.threshold}
	dst.DescribeNode(r, s, func(dst *ggui.Canvas) {
		clip := dst.Clip(r)
		for i, row := range s.rows {
			y := s.starts[i] - s.offset
			if row.Anchor && y <= s.margin+s.peek {
				v.CurrentAnchorID = row.ID
			}
			if y+s.sizes[i].H > 0 && y < r.Size.H {
				v.VisibleMessageIDs = append(v.VisibleMessageIDs, row.ID)
			}
			clip.Paint(row.Content, ggui.Rct(r.Origin.Add(ggui.Pt(0.0, y)), ggui.Sz(r.Size.W, s.sizes[i].H)))
		}
		if v.CanScrollEnd {
			edgeFade(clip, r, 0, -1, s.theme.Card, .8)
		}
		if v.CanScrollStart {
			clip.Paint(s.startButton, ggui.Rct(r.Origin.Add(ggui.Pt((r.Size.W-32)/2, 16.0)), ggui.Sz(32, 32)))
		}
		if v.CanScrollEnd {
			clip.Paint(s.endButton, ggui.Rct(r.Origin.Add(ggui.Pt((r.Size.W-32)/2, max(0, r.Size.H-48))), ggui.Sz(32, 32)))
		}
	})
	changed := v.CurrentAnchorID != s.visibility.CurrentAnchorID || v.CanScrollStart != s.visibility.CanScrollStart || v.CanScrollEnd != s.visibility.CanScrollEnd || !slices.Equal(v.VisibleMessageIDs, s.visibility.VisibleMessageIDs)
	s.visibility = v
	if changed && s.onVisibility != nil {
		s.onVisibility(s.Visibility())
	}
	s.FocusRing(dst, r, 0, s.theme.Ring)
}
func (s *MessageScrollerWidget) Describe() ggui.Node {
	return ggui.Node{Role: ggui.RoleGroup, Name: s.SemanticName(), Actions: ggui.ActionFocus | ggui.ActionScrollIntoView}
}
func (s *MessageScrollerWidget) HandlePointer(ev ggui.PointerEvent) bool {
	if ev.Kind != ggui.PointerScroll || ev.Scroll.Y == 0 {
		return false
	}
	s.Pause()
	s.anchor = ""
	speed := 20.0
	if ev.ScrollPixels {
		speed = 1
	}
	before := s.offset
	s.jump(before-ev.Scroll.Y*speed, false)
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
