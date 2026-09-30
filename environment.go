package ggui

import (
	"image/color"
	"time"
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
		if parent := w.rt.Parent(); parent != nil {
			start = Untrack(worldOf(parent).environment().Get)
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
