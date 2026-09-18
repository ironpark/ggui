// Package icons supplies themeable, monochrome SVG icons. Semantic roles let
// controls share placeholders without depending on a particular icon library.
package icons

import (
	"github.com/ironpark/ggui"
	"image/color"
)

// Role identifies what an icon means, independently of its library's filename.
type Role string

const (
	Check        Role = "check"
	Close        Role = "close"
	ChevronDown  Role = "chevron-down"
	ChevronUp    Role = "chevron-up"
	ChevronLeft  Role = "chevron-left"
	ChevronRight Role = "chevron-right"
	Search       Role = "search"
	Alert        Role = "alert"
	Info         Role = "info"
	Plus         Role = "plus"
	Minus        Role = "minus"
	Download     Role = "download"
	File         Role = "file"
	Sun          Role = "sun"
	Moon         Role = "moon"
	Grip         Role = "grip"
	Loader       Role = "loader"
)

// Set resolves semantic roles to reusable SVG assets. Return nil for an unknown
// role so that the theme or fallback set can supply it.
type Set interface{ Resolve(Role) *SVG }

// Map is a small custom set. Treat it as immutable after providing it to a tree.
type Map map[Role]*SVG

func (m Map) Resolve(r Role) *SVG { return m[r] }

// SetKey selects icons in an Env or Theme. Env overrides Theme per role.
var SetKey = ggui.NewEnvKey[Set]("icons")

// Resolve looks in the local Env, the theme, and finally fallback.
func Resolve(env ggui.Env, fallback Set, role Role) *SVG {
	local, _ := env.Get(SetKey)
	themed, _ := env.Theme().Get(SetKey)
	for _, set := range []Set{local, themed, fallback} {
		if set != nil {
			if asset := set.Resolve(role); asset != nil {
				return asset
			}
		}
	}
	return nil
}

// Widget is a decorative icon by default. Alt adds an accessible image name.
type Widget struct {
	role     Role
	asset    *SVG
	fallback Set
	size     float64
	color    color.Color
	rotation float64
	alt      string
	env      ggui.Env
}

// Placeholder resolves a role during paint using the Env captured in Layout.
// Set a fallback explicitly, or use ui.Icon for the built-in Lucide fallback.
func Placeholder(role Role) *Widget { return &Widget{role: role, size: 16} }

// New uses an explicit asset independent of the inherited icon set.
func New(asset *SVG) *Widget                  { return &Widget{asset: asset, size: 16} }
func (w *Widget) Fallback(set Set) *Widget    { w.fallback = set; return w }
func (w *Widget) Size(px float64) *Widget     { w.size = max(0, px); return w }
func (w *Widget) Color(c color.Color) *Widget { w.color = c; return w }

// Rotate sets clockwise rotation in radians around the icon's center.
func (w *Widget) Rotate(radians float64) *Widget { w.rotation = radians; return w }
func (w *Widget) Alt(name string) *Widget        { w.alt = name; return w }
func (w *Widget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	w.env = env
	return c.Constrain(ggui.Sz(w.size, w.size))
}
func (w *Widget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	if w.alt != "" && dst != nil {
		dst.Leaf(r, ggui.Node{Role: ggui.RoleImage, Name: w.alt})
	}
	asset := w.asset
	if asset == nil {
		asset = Resolve(w.env, w.fallback, w.role)
	}
	col := w.color
	if col == nil {
		col = w.env.Text().Color
	}
	if col == nil {
		col = w.env.Theme().Fg
	}
	side := min(w.size, r.Size.W, r.Size.H)
	at := ggui.Rct(r.Origin.Add(ggui.Pt((r.Size.W-side)/2, (r.Size.H-side)/2)), ggui.Sz(side, side))
	asset.Draw(dst, at, col, w.rotation)
}
