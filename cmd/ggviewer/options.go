//go:build (unix && !ios && !android) || windows

package main

import (
	"github.com/ironpark/ggwd"
	"github.com/ironpark/ggwd/remote"
)

// titlebarChange holds a live update to the window's titlebar hiding and
// traffic-light button position.
type titlebarChange struct {
	hidden        bool
	trafficLights *ggwd.Point
}

// windowChanges are the changes an app's window options make to the
// viewer's window that is open already. A nil field is no change.
type windowChanges struct {
	title            *string
	size             *[2]float64 // width, height
	position         *[2]float64 // x, y
	limits           *[4]float64 // min width, min height, max width, max height; 0 is none
	resizable        *bool
	decorated        *bool
	titlebar         *titlebarChange
	floating         *bool
	mousePassthrough *bool
	maximized        *bool
	hidden           *bool
}

// planOptions returns what next, the options of an app that attached or of
// the app that changed them, changes in a window that shows prev, the
// options the window was last given by an app; nil when no app gave it any,
// as a window opened for a build failure was not. restored says that the
// window's size and position are remembered from an earlier session, which an
// app's size or position does not override unless the app asks for another
// than its last.
func planOptions(prev *remote.OpenWindow, next *remote.OpenWindow, title string, restored bool) windowChanges {
	var c windowChanges
	if next.Title != title {
		t := next.Title
		c.title = &t
	}
	var p remote.OpenWindow
	if prev != nil {
		p = *prev
	}
	if (prev == nil && !restored || prev != nil && (p.Width != next.Width || p.Height != next.Height)) && next.Width > 0 && next.Height > 0 {
		c.size = &[2]float64{next.Width, next.Height}
	}
	if next.PositionSet && ((prev == nil && !restored) || (prev != nil && (!p.PositionSet || p.X != next.X || p.Y != next.Y))) {
		c.position = &[2]float64{next.X, next.Y}
	}
	limit := func(v float64) float64 { return max(v, 0) }
	nl := [4]float64{limit(next.MinWidth), limit(next.MinHeight), limit(next.MaxWidth), limit(next.MaxHeight)}
	if pl := [4]float64{limit(p.MinWidth), limit(p.MinHeight), limit(p.MaxWidth), limit(p.MaxHeight)}; prev == nil || pl != nl {
		c.limits = &nl
	}
	flag := func(changed bool, v bool) *bool {
		if !changed {
			return nil
		}
		return &v
	}
	c.resizable = flag(prev == nil || p.Resizable != next.Resizable, next.Resizable)
	c.decorated = flag(prev == nil || p.Decorated != next.Decorated, next.Decorated)
	c.floating = flag(prev == nil || p.Floating != next.Floating, next.Floating)
	c.mousePassthrough = flag((prev == nil && next.MousePassthrough) || (prev != nil && p.MousePassthrough != next.MousePassthrough), next.MousePassthrough)
	c.maximized = flag((prev == nil && next.Maximized) || (prev != nil && p.Maximized != next.Maximized), next.Maximized)
	c.hidden = flag((prev == nil && next.Hidden) || (prev != nil && p.Hidden != next.Hidden), next.Hidden)

	titlebarChanged := (prev == nil && (next.TitlebarHidden || next.TrafficLightsSet)) ||
		(prev != nil && (p.TitlebarHidden != next.TitlebarHidden ||
			p.TrafficLightsSet != next.TrafficLightsSet ||
			(next.TrafficLightsSet && (p.TrafficLightsX != next.TrafficLightsX || p.TrafficLightsY != next.TrafficLightsY)))) ||
		(c.decorated != nil && *c.decorated && (next.TitlebarHidden || next.TrafficLightsSet))
	if titlebarChanged {
		tb := &titlebarChange{hidden: next.TitlebarHidden}
		if next.TitlebarHidden && next.TrafficLightsSet {
			tb.trafficLights = &ggwd.Point{X: next.TrafficLightsX, Y: next.TrafficLightsY}
		}
		c.titlebar = tb
	}
	return c
}

// windowSetter is what of a window planOptions' changes are applied to.
type windowSetter interface {
	SetTitle(string)
	SetSize(width, height float64)
	SetPosition(x, y float64)
	SetSizeLimits(minWidth, minHeight, maxWidth, maxHeight float64)
	SetResizable(bool)
	SetDecorated(bool)
	SetTitlebarHidden(bool)
	SetTrafficLights(*ggwd.Point)
	SetFloating(bool)
	SetMousePassthrough(bool)
	Maximize()
	Restore()
	Show()
	Hide()
}

// apply makes the changes to w. The limits go before size so that a new size
// within them is not clamped by the old ones, and decoration goes before
// titlebar customization so macOS style-mask updates do not clobber it.
func (c windowChanges) apply(w windowSetter) {
	if c.title != nil {
		w.SetTitle(*c.title)
	}
	if c.limits != nil {
		w.SetSizeLimits(c.limits[0], c.limits[1], c.limits[2], c.limits[3])
	}
	if c.resizable != nil {
		w.SetResizable(*c.resizable)
	}
	if c.decorated != nil {
		w.SetDecorated(*c.decorated)
	}
	if c.titlebar != nil {
		w.SetTitlebarHidden(c.titlebar.hidden)
		w.SetTrafficLights(c.titlebar.trafficLights)
	}
	if c.floating != nil {
		w.SetFloating(*c.floating)
	}
	if c.mousePassthrough != nil {
		w.SetMousePassthrough(*c.mousePassthrough)
	}
	if c.size != nil {
		w.SetSize(c.size[0], c.size[1])
	}
	if c.position != nil {
		w.SetPosition(c.position[0], c.position[1])
	}
	if c.maximized != nil {
		if *c.maximized {
			w.Maximize()
		} else {
			w.Restore()
		}
	}
	if c.hidden != nil {
		if *c.hidden {
			w.Hide()
		} else {
			w.Show()
		}
	}
}
