package ui

import (
	"strings"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	uitheme "github.com/ironpark/ggui/ui/theme"
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
	side       float64
	square     bool
	theme      uitheme.Theme
	textSize   ggui.Size
}

// Avatar creates a 40-pixel circle showing the initials of name.
//
//	ui.Avatar("Ada Lovelace").Image(portrait).Size(32)
func Avatar(name string) *AvatarWidget {
	return &AvatarWidget{name: name, initials: ggui.Text(initialsOf(name)).NoWrap(), side: 40}
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

// Square rounds the portrait to the theme's radius instead of a circle.
func (a *AvatarWidget) Square() *AvatarWidget {
	defer property.Watch(&a.props, &a.square)()
	a.square = true
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

// Layout implements ggui.Widget.
func (a *AvatarWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer a.props.Layout()()
	if a.nameReader != nil {
		a.name = a.nameReader.Get()
	}
	a.theme = uitheme.From(env)
	size := c.Constrain(ggui.Sz(a.side, a.side))
	a.initials.Style(a.theme.Text).Size(max(size.H*0.4, 1)).Color(a.theme.MutedFg).Align(.5)
	a.textSize = a.initials.Layout(ggui.Loose(size), env)
	return size
}

// Paint implements ggui.Widget.
func (a *AvatarWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	dst.Leaf(r, ggui.Node{Role: ggui.RoleImage, Name: a.name})
	radius := min(r.Size.W, r.Size.H) / 2
	if a.square {
		radius = a.theme.Radius
	}
	if a.img == nil || dst == nil || dst.Image == nil {
		dst.FillRoundRect(r, radius, a.theme.Muted)
		at := ggui.Pt(r.Origin.X+(r.Size.W-a.textSize.W)/2, r.Origin.Y+(r.Size.H-a.textSize.H)/2)
		dst.Paint(a.initials, ggui.Rct(at, a.textSize))
		return
	}
	side := min(r.Size.W, r.Size.H)
	drawCover(dst, a.img, ggui.Rct(r.Origin, ggui.Sz(side, side)), radius, ggui.ImageOptions{})
}

// drawCover draws img scaled to cover the logical Rect r, keeping its aspect
// ratio, and cut to r with its corners rounded by radius, which at half the
// side of a square is a circle.
func drawCover(dst *ggui.Canvas, img *ggfx.Image, r ggui.Rect, radius float64, o ggui.ImageOptions) {
	b := img.Bounds()
	if b.Empty() {
		return
	}
	at := ggui.FitCover.Place(ggui.Sz(b.Dx(), b.Dy()), r)
	dst.ClipRoundRect(r, radius, func(dst *ggui.Canvas) { dst.DrawImage(img, at, o) })
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
