package ggui

import (
	"image/color"
	"time"

	"github.com/ironpark/ggui/pkg/layout"
)

// BackgroundKey supplies the window background when Config.Background is nil.
var BackgroundKey = NewEnvKey[color.Color]("background")

// SpacingKey supplies the logical-pixel unit used by layout Space setters.
var SpacingKey = NewEnvKey[float64]("spacing")

// EditorStyle describes the colors used by the unadorned text editor.
type EditorStyle struct {
	Muted     color.Color // placeholder and disabled text
	Selection color.Color
}

var EditorStyleKey = NewEnvKey[EditorStyle]("editor style")

// ScrollStyle describes a scrollbar independently of any design system.
type ScrollStyle struct {
	Color, HoverColor color.Color
	Duration          time.Duration
}

var ScrollStyleKey = NewEnvKey[ScrollStyle]("scroll style")

// PopupDurationKey controls the built-in popup reveal animation.
var PopupDurationKey = NewEnvKey[time.Duration]("popup duration")

// Background returns the inherited background, or white.
func (e Env) Background() color.Color {
	if c, ok := e.Get(BackgroundKey); ok && c != nil {
		return c
	}
	return color.White
}

// Spacing returns the inherited layout unit, or eight logical pixels.
func (e Env) Spacing() float64 {
	if v, ok := e.Get(SpacingKey); ok {
		return v
	}
	return 8
}

// EditorStyle resolves missing colors to standalone editor defaults.
func (e Env) EditorStyle() EditorStyle {
	s, _ := e.Get(EditorStyleKey)
	if s.Muted == nil {
		s.Muted = color.Gray{Y: 0x71}
	}
	if s.Selection == nil {
		s.Selection = color.Gray{Y: 0xd4}
	}
	return s
}

// ScrollStyle resolves missing colors to standalone scrollbar defaults.
func (e Env) ScrollStyle() ScrollStyle {
	s, ok := e.Get(ScrollStyleKey)
	if !ok {
		s.Duration = 150 * time.Millisecond
	}
	if s.Color == nil {
		s.Color = color.Gray{Y: 0x71}
	}
	if s.HoverColor == nil {
		s.HoverColor = color.Black
	}
	return s
}

// PopupDuration returns the inherited reveal duration, or 150 milliseconds.
func (e Env) PopupDuration() time.Duration {
	if d, ok := e.Get(PopupDurationKey); ok {
		return d
	}
	return 150 * time.Millisecond
}

// The application environment is kept per world: the App's windows share
// one, each Probe has its own, and a goroutine's Base world holds what is
// set there before any app or probe runs. A world's starts as a copy of the
// Base world beside it when it is first read, which for an app or a probe
// is its first frame, so SetEnv before Run, or before a probe's first
// frame, reaches it. From then on the two are separate.

// environment returns w's environment signal, copying its parent's the
// first time.
func (w *world) environment() *StateValue[Env] {
	if w.env == nil {
		var start Env
		if w.parent != nil {
			start = Untrack(w.parent.environment().Get)
		}
		w.env = State(start).WithEqual(nil)
	}
	return w.env
}

// UseEnv reads the application environment and subscribes the current builder
// or effect to replacements. Layout receives this environment automatically.
func UseEnv() Env { return activeWorld().environment().Get() }

// SetEnv replaces the application environment and invalidates layout: the
// environment of the App or Probe whose frames run on this goroutine, or
// before any has, the one the next to start copies. Call on the UI thread
// or before starting an application.
func SetEnv(env Env) {
	activeWorld().environment().Set(env)
}

func rootEnv() Env { return Untrack(UseEnv) }

// TextDirection is the direction a subtree reads in: left to right, as
// English does, or right to left, as Arabic and Hebrew do. The zero value
// is DirLTR.
type TextDirection = layout.Direction

const (
	DirLTR = layout.LTR
	DirRTL = layout.RTL
)

// DirectionKey holds the direction of the subtree below it; Directionality
// provides it and Env.Direction reads it, DirLTR by default.
var DirectionKey = NewEnvKey[TextDirection]("text direction")

// LanguageKey holds the BCP 47 language of the subtree below it, such as
// "ar" or "he-IL", which text hands the shaper with each right-to-left
// run so that it picks the language's forms. Env.Language reads it.
var LanguageKey = NewEnvKey[string]("language")

// Direction returns the direction the tree under e reads in.
func (e Env) Direction() TextDirection {
	d, _ := e.Get(DirectionKey)
	return d
}

// Language returns the BCP 47 language set for the tree under e, or "".
func (e Env) Language() string {
	l, _ := e.Get(LanguageKey)
	return l
}

// Directionality lays child out reading in dir. Under DirRTL a paragraph
// orders its runs right to left, text starts at the right edge, a Row
// places its first child rightmost and a Column puts AlignStart children
// on the right; Align's Left and Right, and an explicit Text.Align, stay
// where they say.
//
//	ggui.Directionality(ggui.DirRTL, ggui.Text("שלום עולם"))
func Directionality(dir TextDirection, child Widget) *EnvWidget {
	return Provide(DirectionKey, dir, child)
}

// startAlign is where a line of text starts by default: the left edge, or
// the right one under DirRTL.
func startAlign(d TextDirection) float64 {
	if d == DirRTL {
		return 1
	}
	return 0
}

// EnvColor is a color the Env holds, such as a theme's primary color. A
// widget given one resolves it against the Env it is laid out in, so the
// widget follows a theme switch, or a subtree's own theme, without being
// rebuilt. Its RGBA is the value in the application's environment, for code
// that draws it without an Env.
//
// An EnvColor is compared with ==, as every color property is, so it must
// be comparable; the theme package's tokens are.
type EnvColor interface {
	color.Color
	Resolve(env Env) color.Color
}

// ResolveColor returns c as env has it: an EnvColor's value there, and any
// other color as it is. A custom widget that takes a color resolves it this
// way in Layout and paints the result.
func ResolveColor(c color.Color, env Env) color.Color {
	if e, ok := c.(EnvColor); ok {
		return e.Resolve(env)
	}
	return c
}

// IsEnvColor reports whether c is an EnvColor, which a widget can only
// paint once layout has resolved it.
func IsEnvColor(c color.Color) bool {
	_, ok := c.(EnvColor)
	return ok
}

// shownColor is a color property as it was given and as it is painted: the
// given color itself, or an EnvColor's value in the Env of the last layout.
type shownColor struct{ given, shown color.Color }

// set gives the property c. A widget lays out again for an EnvColor, given
// or replaced, since only layout knows its Env.
func (s *shownColor) set(c color.Color, changed func()) {
	if c == s.given {
		return
	}
	if IsEnvColor(c) || IsEnvColor(s.given) {
		changed()
	}
	s.given, s.shown = c, c
}

func (s *shownColor) layout(env Env) { s.shown = ResolveColor(s.given, env) }

// ResponsiveWidget lays out one of two arrangements by the width it is
// given. Build one with Responsive.
type ResponsiveWidget struct {
	breakpoint   float64
	wide, narrow Widget
	current      Widget
}

// Responsive lays out wide while it is given at least breakpoint of width,
// and narrow below that. The two arrangements may hold the same widgets, so
// a control keeps its state, focus included, as the window crosses the
// breakpoint:
//
//	title, actions := ui.Title("Tasks"), ui.Button("New", create)
//	ggui.Responsive(680,
//		ggui.Row(ggui.Expanded(title), actions),
//		ggui.Column(title, actions).Stretch())
func Responsive(breakpoint float64, wide, narrow Widget) *ResponsiveWidget {
	return &ResponsiveWidget{breakpoint: breakpoint, wide: wide, narrow: narrow, current: wide}
}

// Layout implements Widget.
func (r *ResponsiveWidget) Layout(c Constraints, env Env) Size {
	r.current = pick(c.MaxW < r.breakpoint, r.narrow, r.wide)
	return r.current.Layout(c, env)
}

// Baseline implements Baseliner: the shown arrangement's.
func (r *ResponsiveWidget) Baseline() (float64, bool) { return baselineOf(r.current) }

// Paint implements Widget.
func (r *ResponsiveWidget) Paint(dst *Canvas, rect Rect) { dst.Paint(r.current, rect) }
