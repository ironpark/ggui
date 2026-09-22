package ui

import (
	"image/color"
	"math"
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
	side := int(math.Ceil(float64(dst.Px(min(r.Size.W, r.Size.H)))))
	cut := roundedCut(a.img, side, float64(dst.Px(radius)))
	if cut == nil {
		return
	}
	// The cut image is already in device pixels, so it is drawn one for
	// one rather than through the canvas scale a second time.
	op := &ggfx.DrawImageOptions{Filter: ggfx.FilterLinear}
	s := dst.Scale()
	op.GeoM.Scale(1/s, 1/s)
	op.GeoM.Concat(dst.Geo(r.Origin))
	dst.Image.DrawImage(cut, op)
}

// cutKey names one crop: the image it came from, the square of device
// pixels it was scaled to, and the corner radius it was cut with.
type cutKey struct {
	src          *ggfx.Image
	side, radius int
}

// Crops live in two generations: the ones asked for since the last sweep,
// and the ones from the sweep before. A crop nobody has asked for in two
// sweeps is freed, so a list longer than the cache loses its coldest crops
// rather than all of them at once, and an avatar rebuilt every frame
// allocates no texture at all. Both maps belong to the UI goroutine, as
// painting does.
var liveCuts, coldCuts = map[cutKey]*ggfx.Image{}, map[cutKey]*ggfx.Image{}

// cutGeneration is how many crops are made before the cold generation is
// freed; the warm ones are promoted back and survive.
const cutGeneration = 64

// roundedCut returns src scaled to cover a side-by-side square and cut to
// the rounded rectangle of the given radius, which at half the side is a
// circle. It is how an avatar gets a round photo at all: the canvas clips
// to rectangles and nothing else, so the shape has to come from an alpha
// mask multiplied into the image.
func roundedCut(src *ggfx.Image, side int, radius float64) *ggfx.Image {
	b := src.Bounds()
	if side <= 0 || b.Dx() == 0 || b.Dy() == 0 {
		return nil
	}
	key := cutKey{src, side, int(math.Round(radius))}
	if cut, ok := liveCuts[key]; ok {
		return cut
	}
	if cut, ok := coldCuts[key]; ok {
		liveCuts[key] = cut
		delete(coldCuts, key)
		return cut
	}
	if len(liveCuts) >= cutGeneration {
		sweepCuts()
	}
	cut := ggfx.NewImage(side, side)
	k := max(float64(side)/float64(b.Dx()), float64(side)/float64(b.Dy()))
	op := &ggfx.DrawImageOptions{Filter: ggfx.FilterLinear}
	op.GeoM.Scale(k, k)
	op.GeoM.Translate((float64(side)-float64(b.Dx())*k)/2, (float64(side)-float64(b.Dy())*k)/2)
	cut.DrawImage(src, op)
	mask := ggfx.NewImage(side, side)
	(&ggui.Canvas{Image: mask}).FillRoundRect(ggui.Rect{Size: ggui.Sz(side, side)}, radius, color.White)
	cut.DrawImage(mask, &ggfx.DrawImageOptions{Blend: ggfx.BlendDestinationIn})
	mask.Deallocate()
	liveCuts[key] = cut
	return cut
}

// sweepCuts frees what has gone two generations unasked for and starts a
// generation over.
func sweepCuts() {
	for k, img := range coldCuts {
		img.Deallocate()
		delete(coldCuts, k)
	}
	liveCuts, coldCuts = coldCuts, liveCuts
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
