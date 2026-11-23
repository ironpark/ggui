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
	c.title = planTitle(next, title)
	var p remote.OpenWindow
	if prev != nil {
		p = *prev
	}
	c.size = planSize(prev, next, p, restored)
	c.position = planPosition(prev, next, p, restored)
	c.limits = planLimits(prev, next, p)
	c.resizable = flagIf(prev == nil || p.Resizable != next.Resizable, next.Resizable)
	c.decorated = flagIf(prev == nil || p.Decorated != next.Decorated, next.Decorated)
	c.floating = flagIf(prev == nil || p.Floating != next.Floating, next.Floating)
	c.mousePassthrough = flagIf((prev == nil && next.MousePassthrough) || (prev != nil && p.MousePassthrough != next.MousePassthrough), next.MousePassthrough)
	c.maximized = flagIf((prev == nil && next.Maximized) || (prev != nil && p.Maximized != next.Maximized), next.Maximized)
	c.hidden = flagIf((prev == nil && next.Hidden) || (prev != nil && p.Hidden != next.Hidden), next.Hidden)
	c.titlebar = planTitlebar(prev, next, p, c.decorated)
	return c
}

// planTitle reports a new window title when the app's differs from the one
// the window shows.
func planTitle(next *remote.OpenWindow, title string) *string {
	if next.Title == title {
		return nil
	}
	t := next.Title
	return &t
}

// planSize reports a new window size when the app's differs from the one
// the window shows, unless a remembered size stands and the app did not ask
// for another.
func planSize(prev *remote.OpenWindow, next *remote.OpenWindow, p remote.OpenWindow, restored bool) *[2]float64 {
	if (prev == nil && !restored || prev != nil && (p.Width != next.Width || p.Height != next.Height)) && next.Width > 0 && next.Height > 0 {
		return &[2]float64{next.Width, next.Height}
	}
	return nil
}

// planPosition reports a new window position when the app set one that
// differs from the one the window shows.
func planPosition(prev *remote.OpenWindow, next *remote.OpenWindow, p remote.OpenWindow, restored bool) *[2]float64 {
	if next.PositionSet && ((prev == nil && !restored) || (prev != nil && (!p.PositionSet || p.X != next.X || p.Y != next.Y))) {
		return &[2]float64{next.X, next.Y}
	}
	return nil
}

// planLimits reports new window size limits when the app's differ from the
// ones the window shows; 0 is no limit.
func planLimits(prev *remote.OpenWindow, next *remote.OpenWindow, p remote.OpenWindow) *[4]float64 {
	limit := func(v float64) float64 { return max(v, 0) }
	nl := [4]float64{limit(next.MinWidth), limit(next.MinHeight), limit(next.MaxWidth), limit(next.MaxHeight)}
	pl := [4]float64{limit(p.MinWidth), limit(p.MinHeight), limit(p.MaxWidth), limit(p.MaxHeight)}
	if prev == nil || pl != nl {
		return &nl
	}
	return nil
}

// flagIf reports v when apply is true, nil otherwise.
func flagIf(apply bool, v bool) *bool {
	if !apply {
		return nil
	}
	return &v
}

// planTitlebar reports titlebar hiding and traffic-light changes when the
// app's differ from the window's, or when decoration was just re-enabled
// over a hidden titlebar.
func planTitlebar(prev *remote.OpenWindow, next *remote.OpenWindow, p remote.OpenWindow, decorated *bool) *titlebarChange {
	if !titlebarChanged(prev, next, p, decorated) {
		return nil
	}
	tb := &titlebarChange{hidden: next.TitlebarHidden}
	if next.TitlebarHidden && next.TrafficLightsSet {
		tb.trafficLights = &ggwd.Point{X: next.TrafficLightsX, Y: next.TrafficLightsY}
	}
	return tb
}

// titlebarChanged reports whether any of the titlebar triggers fired: the
// first app's options, a change since the last app's, or decoration just
// re-enabled over a hidden titlebar.
func titlebarChanged(prev *remote.OpenWindow, next *remote.OpenWindow, p remote.OpenWindow, decorated *bool) bool {
	return firstTitlebar(prev, next) || changedTitlebar(prev, next, p) || redecoratedTitlebar(next, decorated)
}

// firstTitlebar reports the first app's titlebar options, which always apply.
func firstTitlebar(prev *remote.OpenWindow, next *remote.OpenWindow) bool {
	return prev == nil && (next.TitlebarHidden || next.TrafficLightsSet)
}

// changedTitlebar reports titlebar options that differ from the last app's.
func changedTitlebar(prev *remote.OpenWindow, next *remote.OpenWindow, p remote.OpenWindow) bool {
	return prev != nil && (p.TitlebarHidden != next.TitlebarHidden ||
		p.TrafficLightsSet != next.TrafficLightsSet ||
		(next.TrafficLightsSet && (p.TrafficLightsX != next.TrafficLightsX || p.TrafficLightsY != next.TrafficLightsY)))
}

// redecoratedTitlebar reports a hidden titlebar under decoration that was
// just re-enabled, which macOS style-mask updates would otherwise clobber.
func redecoratedTitlebar(next *remote.OpenWindow, decorated *bool) bool {
	return decorated != nil && *decorated && (next.TitlebarHidden || next.TrafficLightsSet)
}

// windowFrame is the window geometry and state planOptions' changes apply
// to.
type windowFrame interface {
	SetTitle(string)
	SetSize(width, height float64)
	SetPosition(x, y float64)
	SetSizeLimits(minWidth, minHeight, maxWidth, maxHeight float64)
	Maximize()
	Restore()
	Show()
	Hide()
}

// windowChrome is the window appearance planOptions' changes apply to.
type windowChrome interface {
	SetResizable(bool)
	SetDecorated(bool)
	SetTitlebarHidden(bool)
	SetTrafficLights(*ggwd.Point)
	SetFloating(bool)
	SetMousePassthrough(bool)
}

// windowSetter is what of a window planOptions' changes are applied to.
type windowSetter interface {
	windowFrame
	windowChrome
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
