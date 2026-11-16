//go:build (unix && !ios && !android) || windows

package main

import "github.com/ironpark/ggwd/remote"

// windowChanges are the changes an app's window options make to the
// viewer's window that is open already. A nil field is no change.
type windowChanges struct {
	title     *string
	size      *[2]float64 // width, height
	limits    *[4]float64 // min width, min height, max width, max height; 0 is none
	resizable *bool
	decorated *bool
	floating  *bool
}

// planOptions returns what next, the options of an app that attached or of
// the app that changed them, changes in a window that shows prev, the
// options the window was last given by an app; nil when no app gave it any,
// as a window opened for a build failure was not. restored says that the
// window's size is one remembered from an earlier session, which an app's
// size does not override unless the app asks for another than its last.
//
// The title bar's hiding and transparency are a window's from its creation
// and are not changed.
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
	return c
}

// windowSetter is what of a window planOptions' changes are applied to.
type windowSetter interface {
	SetTitle(string)
	SetSize(width, height float64)
	SetSizeLimits(minWidth, minHeight, maxWidth, maxHeight float64)
	SetResizable(bool)
	SetDecorated(bool)
	SetFloating(bool)
}

// apply makes the changes to w. The limits go first, so that a new size
// within them is not clamped by the old ones.
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
	if c.floating != nil {
		w.SetFloating(*c.floating)
	}
	if c.size != nil {
		w.SetSize(c.size[0], c.size[1])
	}
}
