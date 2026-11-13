package ui

import (
	"image/color"
	"math"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/ui/icons"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

const (
	breadcrumbGap       = 6.0  // between steps and separators, shadcn's gap-1.5
	breadcrumbSeparator = 14.0 // the chevron between steps, shadcn's size-3.5
)

// BreadcrumbEntry is one step of a trail; build it with Crumb.
type BreadcrumbEntry struct {
	label  string
	onTap  func()
	elided bool // stands for the steps Max left out
}

// Crumb names one step. A nil onTap makes it a plain label, which is what
// the step the user is already on wants; the last crumb is that step
// whether or not it was given one.
func Crumb(label string, onTap func()) BreadcrumbEntry {
	return BreadcrumbEntry{label: label, onTap: onTap}
}

// BreadcrumbWidget shows the path to the page the user is on, with every
// step but the last one clickable. Build one with Breadcrumb.
//
//	ui.Breadcrumb(
//		ui.Crumb("Home", goHome),
//		ui.Crumb("Projects", goProjects),
//		ui.Crumb("ggui", nil),
//	)
//
// Each link is its own tab stop, as a row of links is; the trail itself is
// a group, so a screen reader reads it as one thing. A trail too long for
// its width wraps onto further lines.
type BreadcrumbWidget struct {
	nameReader ggui.Readable[string]
	props      property.Owner
	crumbs     []BreadcrumbEntry
	separator  string      // text between steps; empty draws the chevron
	custom     ggui.Widget // what SeparatorOf set, drawn between steps
	limit      int
	name       string

	row      *ggui.WrapWidget
	muted    []*ggui.TextWidget // text separators and the steps that are no link
	chevrons []*icons.Widget    // the default separators
	current  *ggui.TextWidget   // the page the user is on
}

// Breadcrumb creates a trail. The last crumb is drawn as the current page:
// plain text in the normal color, with no click of its own. A chevron
// pointing along the reading direction separates the steps.
func Breadcrumb(crumbs ...BreadcrumbEntry) *BreadcrumbWidget {
	return &BreadcrumbWidget{crumbs: crumbs, name: "Breadcrumb"}
}

// Separator draws the text s between steps in place of the chevron, such as
// "/"; an empty s brings the chevron back.
func (b *BreadcrumbWidget) Separator(s string) *BreadcrumbWidget {
	if b.separator != s || b.custom != nil {
		b.separator, b.custom = s, nil
		b.rebuild()
	}
	return b
}

// SeparatorOf draws w between steps in place of the chevron: an icon, say,
// or a dot. The one widget is drawn at every gap, so it should be something
// that only draws; text and icons in it take the trail's muted color unless
// they set their own.
func (b *BreadcrumbWidget) SeparatorOf(w ggui.Widget) *BreadcrumbWidget {
	if b.custom != w {
		b.separator, b.custom = "", w
		b.rebuild()
	}
	return b
}

// rebuild drops the built row, so the next layout makes it again.
func (b *BreadcrumbWidget) rebuild() {
	b.row, b.muted, b.chevrons, b.current = nil, nil, nil, nil
	b.props.Changed()
}

// Name sets the accessible name of the trail.
func (b *BreadcrumbWidget) Name(s string) *BreadcrumbWidget {
	if b.nameReader != nil {
		b.nameReader = nil
		b.props.Changed()
	}
	defer property.Watch(&b.props, &b.name)()
	b.name = s
	return b
}

// Max keeps the trail to n crumbs: the first, an ellipsis standing for what
// was left out, and the last n-2. A trail already that short is untouched,
// and an n below three is ignored, since there would be nothing to elide.
func (b *BreadcrumbWidget) Max(n int) *BreadcrumbWidget {
	if b.limit != n {
		b.limit = n
		b.rebuild()
	}
	return b
}

// shown returns the crumbs to draw, with the elided middle replaced by one
// crumb that stands for it.
func (b *BreadcrumbWidget) shown() []BreadcrumbEntry {
	if b.limit < 3 || len(b.crumbs) <= b.limit {
		return b.crumbs
	}
	out := []BreadcrumbEntry{b.crumbs[0], {elided: true}}
	return append(out, b.crumbs[len(b.crumbs)-(b.limit-2):]...)
}

// build makes the row once, so the links keep their hover and focus across
// the layouts that follow.
func (b *BreadcrumbWidget) build() {
	crumbs := b.shown()
	var parts []ggui.Widget
	for i, c := range crumbs {
		if i > 0 {
			switch {
			case b.custom != nil:
				parts = append(parts, &breadcrumbSeparatorOf{child: b.custom})
			case b.separator != "":
				sep := ggui.Text(b.separator).NoWrap()
				b.muted = append(b.muted, sep)
				parts = append(parts, sep)
			default:
				sep := Icon(icons.ChevronRight).Size(breadcrumbSeparator)
				b.chevrons = append(b.chevrons, sep)
				parts = append(parts, sep)
			}
		}
		last := i == len(crumbs)-1
		switch {
		case c.elided:
			parts = append(parts, &moreIcon{icon: Icon(icons.MoreHorizontal).Alt("More"), muted: true, side: func(t *uitheme.Theme) float64 { return t.IconSize + 4 }})
		case last:
			b.current = ggui.Text(c.label).NoWrap()
			parts = append(parts, b.current)
		case c.onTap == nil:
			label := ggui.Text(c.label).NoWrap()
			b.muted = append(b.muted, label)
			parts = append(parts, label)
		default:
			parts = append(parts, newBreadcrumbLink(c.label, c.onTap))
		}
	}
	b.row = ggui.Wrap(parts...).Gap(breadcrumbGap).Align(ggui.AlignCenter)
}

// Layout implements ggui.Widget.
func (b *BreadcrumbWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer b.props.Layout().EndLayout()
	if b.nameReader != nil {
		b.name = b.nameReader.Get()
	}
	if b.row == nil {
		b.build()
	}
	t := uitheme.Of(env)
	for _, w := range b.muted {
		w.Style(t.Text).Color(t.MutedFg)
	}
	turn := pick(env.Direction() == ggui.DirRTL, math.Pi, 0.0)
	for _, w := range b.chevrons {
		w.Color(t.MutedFg).Rotate(turn)
	}
	if b.current != nil {
		b.current.Style(t.Text).Color(t.Fg)
	}
	return b.row.Layout(c, env)
}

// Paint implements ggui.Widget. The trail is one group: the steps belong
// together, and only the current page is read as where the user is.
func (b *BreadcrumbWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	dst.Node(r, ggui.Node{Role: ggui.RoleGroup, Name: b.name}, func(dst *ggui.Canvas) {
		dst.Paint(b.row, r)
	})
}

// BindName follows a non-nil accessible-name reader.
func (b *BreadcrumbWidget) BindName(r ggui.Readable[string]) *BreadcrumbWidget {
	property.Require(r, "BindName")
	if !property.Same(b.nameReader, r) {
		b.nameReader = r
		b.props.Changed()
	}
	return b
}

// breadcrumbLink is a step that goes somewhere: muted text that eases to the
// text color under the pointer, with no surface of its own.
type breadcrumbLink struct {
	ggui.Interactive
	label  *ggui.TextWidget
	onTap  func()
	theme  *uitheme.Theme
	motion time.Duration
	ink    color.Color // what Paint last drew the label in
}

func newBreadcrumbLink(label string, onTap func()) *breadcrumbLink {
	l := &breadcrumbLink{label: ggui.Text(label).NoWrap(), onTap: onTap}
	l.Role = ggui.RoleLink
	l.SetName(label)
	l.AutoKey()
	return l
}

// Layout implements ggui.Widget.
func (l *breadcrumbLink) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	l.Sync()
	t := uitheme.Of(env)
	l.theme, l.motion = t, env.Motion(t.MotionFast)
	// Paint recolors the label as the pointer crosses it; here it keeps
	// the color it was last drawn in, so a layout does not undo a hover.
	l.label.Color(colorOr(l.ink, t.MutedFg))
	return l.label.Layout(c, env.WithText(t.Text))
}

// Paint implements ggui.Widget.
func (l *breadcrumbLink) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t := l.theme
	hover := ease(dst, l.theme, l.Anchor(r), breadcrumbHoverSlot, pick(l.Hovered && !l.IsInert(), 1.0, 0.0), l.motion)
	l.ink = t.MutedFg
	if hover >= 1 {
		l.ink = t.Fg
	} else if hover > 0 {
		l.ink = mix(t.MutedFg, t.Fg, hover)
	}
	l.label.Color(l.ink)
	l.Hit(dst, r, l, ggui.CursorShapePointer)
	dst.Paint(l.label, r)
	ring := ggui.Rct(r.Origin.Add(ggui.Pt(-3, 0)), ggui.Sz(r.Size.W+6, r.Size.H))
	controlFocusRing(dst, &l.Interactive, t, ring, t.RadiusSm/2, t.Ring)
}

var breadcrumbHoverSlot = ggui.NewSlot[*ggui.Motion]("breadcrumb hover")

// Baseline implements ggui.Baseliner: the label's.
func (l *breadcrumbLink) Baseline() (float64, bool) { return l.label.Baseline() }

// Describe implements ggui.Describer.
func (l *breadcrumbLink) Describe() ggui.Node {
	return ggui.Node{Role: ggui.RoleLink, Name: l.SemanticName(), Disabled: l.IsInert(), Actions: ggui.ActionPress | ggui.ActionFocus}
}

// Act implements ggui.Actor: following the link.
func (l *breadcrumbLink) Act(a ggui.Action) bool {
	if l.IsInert() || a.Kind != ggui.ActionPress || l.onTap == nil {
		return false
	}
	l.onTap()
	return true
}

// HandleKey implements ggui.KeyHandler: Space or Enter follows the link.
func (l *breadcrumbLink) HandleKey(ev ggui.KeyEvent) { l.Keyboard(ev, l.onTap) }

// HandlePointer implements ggui.PointerHandler.
func (l *breadcrumbLink) HandlePointer(ev ggui.PointerEvent) bool { return l.Pointer(ev, l.onTap) }

// breadcrumbSeparatorOf draws the caller's one separator widget at one gap of
// the trail. Each gap has a wrapper of its own, so the layout tells the gaps
// apart though they share what they draw.
type breadcrumbSeparatorOf struct{ child ggui.Widget }

func (s *breadcrumbSeparatorOf) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	t := uitheme.Of(env)
	return s.child.Layout(c.Loosen(), env.WithText(t.Text.Merge(ggui.TextStyle{Color: t.MutedFg})))
}

func (s *breadcrumbSeparatorOf) Paint(dst *ggui.Canvas, r ggui.Rect) { s.child.Paint(dst, r) }

// moreIcon is the three dots that stand for what a trail or a pager leaves
// out, centered in a square of side. It is read out as an image named by the
// icon's Alt.
type moreIcon struct {
	icon  *icons.Widget
	side  func(*uitheme.Theme) float64
	muted bool // drawn in the muted color rather than the text color
	size  ggui.Size
}

func (m *moreIcon) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	t := uitheme.Of(env)
	m.icon.Size(t.IconSize).Color(pick(m.muted, t.MutedFg, t.Fg))
	m.size = m.icon.Layout(c.Loosen(), env)
	side := m.side(t)
	return c.Constrain(ggui.Sz(side, side))
}

func (m *moreIcon) Paint(dst *ggui.Canvas, r ggui.Rect) {
	dst.Paint(m.icon, ggui.Rct(r.Origin.Add(ggui.Pt((r.Size.W-m.size.W)/2, (r.Size.H-m.size.H)/2)), m.size))
}
