package ui

import (
	"image/color"
	"strconv"
	"strings"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/ui/icons"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// The avatar sizes of shadcn/ui: Small, the default and Large.
const (
	avatarSmall   = 24.0
	avatarDefault = 32.0
	avatarLarge   = 40.0
)

// AvatarWidget is a round portrait with a fallback: the person's initials on
// the theme's muted surface until an image is given, and again if the image
// turns out to be nil. Build one with Avatar.
type AvatarWidget struct {
	nameReader ggui.Readable[string]
	props      property.Owner
	name       string
	initials   *ggui.TextWidget
	img        *ggfx.Image
	side       float64 // negative is the default size
	square     bool
	theme      *uitheme.Theme
	env        ggui.Env
	textSize   ggui.Size

	badge      bool
	badgeColor color.Color // nil is the theme's Primary
	badgeIcon  icons.Role
}

// Avatar creates a 32-pixel circle showing the initials of name. Small and
// Large are the other two sizes of the set, and Size any other.
//
//	ui.Avatar("Ada Lovelace").Image(portrait).Large()
func Avatar(name string) *AvatarWidget {
	return &AvatarWidget{name: name, initials: ggui.Text(initialsOf(name)).NoWrap(), side: -1}
}

// initialsOf takes the first letter of the first and last words, which is
// what a name badge shows: "Ada Lovelace" is AL, "Ada" is A.
func initialsOf(name string) string {
	words := strings.Fields(name)
	if len(words) == 0 {
		return "?"
	}
	first := []rune(words[0])[:1]
	if len(words) == 1 {
		return strings.ToUpper(string(first))
	}
	last := []rune(words[len(words)-1])[:1]
	return strings.ToUpper(string(first) + string(last))
}

// Image sets the portrait. It is drawn to cover the avatar, so a photo of
// any shape is cropped rather than squashed.
func (a *AvatarWidget) Image(img *ggfx.Image) *AvatarWidget { a.img = img; return a }

// Size sets the diameter in logical pixels.
func (a *AvatarWidget) Size(px float64) *AvatarWidget {
	defer property.Watch(&a.props, &a.side)()
	a.side = max(0, px)
	return a
}

// Small makes the avatar 24 pixels across, with smaller initials.
func (a *AvatarWidget) Small() *AvatarWidget { return a.Size(avatarSmall) }

// Large makes the avatar 40 pixels across.
func (a *AvatarWidget) Large() *AvatarWidget { return a.Size(avatarLarge) }

// Square rounds the portrait to the theme's radius instead of a circle.
func (a *AvatarWidget) Square() *AvatarWidget {
	defer property.Watch(&a.props, &a.square)()
	a.square = true
	return a
}

// Badge marks the avatar with a dot at its bottom right, ringed in the
// page's background: a presence or status mark. c is the dot's color, and
// nil the theme's Primary.
func (a *AvatarWidget) Badge(c color.Color) *AvatarWidget {
	a.badge, a.badgeColor = true, c
	return a
}

// BadgeIcon marks the avatar as Badge does and draws the icon of role in
// the dot, in the theme's PrimaryFg. A small avatar's dot has no room for
// one and stays bare.
func (a *AvatarWidget) BadgeIcon(role icons.Role) *AvatarWidget {
	a.badge, a.badgeIcon = true, role
	return a
}

// Name replaces the name read out, for an avatar whose label already
// appears beside it.
func (a *AvatarWidget) Name(s string) *AvatarWidget {
	if a.nameReader != nil {
		a.nameReader = nil
		a.props.Changed()
	}
	defer property.Watch(&a.props, &a.name)()
	a.name = s
	return a
}

// diameter is the avatar's side when nothing constrains it.
func (a *AvatarWidget) diameter() float64 { return pick(a.side >= 0, a.side, avatarDefault) }

// Layout implements ggui.Widget.
func (a *AvatarWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer a.props.Layout().EndLayout()
	if a.nameReader != nil {
		a.name = a.nameReader.Get()
	}
	t := uitheme.Of(env)
	a.theme, a.env = t, env
	size := c.Constrain(ggui.Sz(a.diameter(), a.diameter()))
	// The body text's size, the caption's in a small avatar, and in step
	// with one larger than the set's.
	style := t.Text
	switch {
	case size.H <= avatarSmall:
		style = t.Text.Merge(t.Caption)
	case size.H > avatarLarge:
		style.Size = size.H * .35
	}
	style.Color = t.MutedFg
	a.initials.Style(style).Align(.5)
	a.textSize = a.initials.Layout(ggui.Loose(size), env)
	return size
}

// Paint implements ggui.Widget.
func (a *AvatarWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t := a.theme
	dst.Leaf(r, ggui.Node{Role: ggui.RoleImage, Name: a.name})
	side := min(r.Size.W, r.Size.H)
	radius := side / 2
	if a.square {
		radius = t.Radius
	}
	if a.img == nil || dst == nil || dst.Image == nil {
		dst.FillRoundRect(r, radius, t.Muted)
		at := ggui.Pt(r.Origin.X+(r.Size.W-a.textSize.W)/2, r.Origin.Y+(r.Size.H-a.textSize.H)/2)
		dst.Paint(a.initials, ggui.Rct(at, a.textSize))
	} else {
		dst.DrawImage(a.img, ggui.Rct(r.Origin, ggui.Sz(side, side)), ggui.ImageOptions{Fit: ggui.FitCover, Radius: radius})
	}
	// A hairline keeps a portrait as pale as the page apart from it.
	dst.StrokeRoundRect(r, radius, t.BorderWidth, t.Border)
	if !a.badge {
		return
	}
	// The dot grows with the avatar: 8, 10 and 12 pixels for the three
	// sizes, in a ring of the page's background.
	dot := 10.0
	switch {
	case side <= avatarSmall:
		dot = 8
	case side >= avatarLarge:
		dot = 12
	}
	ring := t.BorderWidth * 2
	center := ggui.Pt(r.Origin.X+r.Size.W-dot/2, r.Origin.Y+r.Size.H-dot/2)
	dst.FillCircle(center, dot/2+ring, t.Bg)
	dst.FillCircle(center, dot/2, colorOr(a.badgeColor, t.Primary))
	if a.badgeIcon != "" && side > avatarSmall {
		paintIconAt(dst, a.env, a.badgeIcon, center, dot*.8, t.PrimaryFg, 0)
	}
}

// BindName follows a non-nil accessible-name reader.
func (a *AvatarWidget) BindName(r ggui.Readable[string]) *AvatarWidget {
	property.Require(r, "BindName")
	if !property.Same(a.nameReader, r) {
		a.nameReader = r
		a.props.Changed()
	}
	return a
}

// AvatarGroupWidget is a row of overlapping avatars, each ringed in the
// page's background, ending in a bubble that counts the ones not shown.
// Build one with AvatarGroup.
type AvatarGroupWidget struct {
	props   property.Owner
	avatars []*AvatarWidget
	most    int // how many avatars are shown; zero or less is all of them
	extra   int // more people than there are avatars, added to the count
	count   *AvatarWidget
	counted int // what count says
	theme   *uitheme.Theme
	shown   []*AvatarWidget // the avatars and the count bubble, as laid out
	sizes   []ggui.Size
	step    float64 // how far each overlaps the one before
}

// AvatarGroup creates a row of avatars, each overlapping the one before it
// by the theme's Space.
//
//	ui.AvatarGroup(ui.Avatar("Ada Lovelace"), ui.Avatar("Grace Hopper"), ui.Avatar("Alan Turing")).Max(2)
func AvatarGroup(avatars ...*AvatarWidget) *AvatarGroupWidget {
	return &AvatarGroupWidget{avatars: avatars}
}

// Max shows the first n avatars and counts the rest in a bubble at the
// end, "+3". Zero or less shows them all.
func (g *AvatarGroupWidget) Max(n int) *AvatarGroupWidget {
	defer property.Watch(&g.props, &g.most)()
	g.most = n
	return g
}

// Count adds n to the bubble at the end, for people the group has no
// avatar for: the rest of a team whose first few are shown.
func (g *AvatarGroupWidget) Count(n int) *AvatarGroupWidget {
	defer property.Watch(&g.props, &g.extra)()
	g.extra = max(0, n)
	return g
}

// Layout implements ggui.Widget.
func (g *AvatarGroupWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer g.props.Layout().EndLayout()
	t := uitheme.Of(env)
	g.theme, g.step = t, t.Space
	visible := g.avatars
	if g.most > 0 && len(visible) > g.most {
		visible = visible[:g.most]
	}
	g.shown = append(g.shown[:0], visible...)
	if hidden := len(g.avatars) - len(visible) + g.extra; hidden > 0 {
		// The bubble is an avatar whose initials are the count, the size
		// of the first one beside it.
		if g.count == nil || g.counted != hidden {
			label := "+" + strconv.Itoa(hidden)
			g.count = &AvatarWidget{name: strconv.Itoa(hidden) + " more", initials: ggui.Text(label).NoWrap(), side: -1}
			g.counted = hidden
		}
		side := -1.0
		if len(visible) > 0 {
			side = visible[0].side
		}
		g.count.side = side
		g.shown = append(g.shown, g.count)
	}
	g.sizes = g.sizes[:0]
	width, height := 0.0, 0.0
	for i, a := range g.shown {
		size := a.Layout(ggui.Loose(ggui.Sz(c.MaxW, c.MaxH)), env)
		g.sizes = append(g.sizes, size)
		if i > 0 {
			width -= g.step
		}
		width += size.W
		height = max(height, size.H)
	}
	return c.Constrain(ggui.Sz(max(0, width), height))
}

// Paint implements ggui.Widget. Each avatar is painted over the one before
// it, in a ring that sets it apart.
func (g *AvatarGroupWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t := g.theme
	ring := t.BorderWidth * 2
	dst.Node(r, ggui.Node{Role: ggui.RoleGroup}, func(dst *ggui.Canvas) {
		x := r.Origin.X
		for i, a := range g.shown {
			size := g.sizes[i]
			at := ggui.Rct(ggui.Pt(x, r.Origin.Y+(r.Size.H-size.H)/2), size)
			radius := min(size.W, size.H) / 2
			if a.square {
				radius = t.Radius
			}
			dst.FillRoundRect(at.Grow(ring), radius+ring, t.Bg)
			dst.Paint(a, at)
			x += size.W - g.step
		}
	})
}
