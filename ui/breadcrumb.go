package ui

import (
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// BreadcrumbEntry is one step of a trail; build it with Crumb.
type BreadcrumbEntry struct {
	label string
	onTap func()
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
// a group, so a screen reader reads it as one thing.
type BreadcrumbWidget struct {
	nameReader ggui.Readable[string]
	props      property.Owner
	crumbs     []BreadcrumbEntry
	separator  string
	limit      int
	name       string

	row     *ggui.RowWidget
	muted   []*ggui.TextWidget // separators and the steps behind the current one
	current *ggui.TextWidget   // the page the user is on
}

// Breadcrumb creates a trail. The last crumb is drawn as the current page:
// plain text in the normal color, with no click of its own.
func Breadcrumb(crumbs ...BreadcrumbEntry) *BreadcrumbWidget {
	return &BreadcrumbWidget{crumbs: crumbs, separator: "/", name: "Breadcrumb"}
}

// Separator replaces the "/" drawn between steps.
func (b *BreadcrumbWidget) Separator(s string) *BreadcrumbWidget {
	defer property.Watch(&b.props, &b.separator)()
	b.separator = s
	return b
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
	defer property.Watch(&b.props, &b.limit)()
	b.limit = n
	return b
}

// shown returns the crumbs to draw, with the elided middle replaced by a
// crumb whose label is an ellipsis and whose onTap is nil.
func (b *BreadcrumbWidget) shown() []BreadcrumbEntry {
	if b.limit < 3 || len(b.crumbs) <= b.limit {
		return b.crumbs
	}
	out := []BreadcrumbEntry{b.crumbs[0], {label: "…"}}
	return append(out, b.crumbs[len(b.crumbs)-(b.limit-2):]...)
}

// build makes the row once, so the links keep their hover and focus across
// the layouts that follow.
func (b *BreadcrumbWidget) build() {
	crumbs := b.shown()
	var parts []ggui.Widget
	for i, c := range crumbs {
		if i > 0 {
			sep := ggui.Text(b.separator).NoWrap()
			b.muted = append(b.muted, sep)
			parts = append(parts, sep)
		}
		label := ggui.Text(c.label).NoWrap()
		last := i == len(crumbs)-1
		switch {
		case last:
			b.current = label
			parts = append(parts, label)
		case c.onTap == nil:
			b.muted = append(b.muted, label)
			parts = append(parts, label)
		default:
			// ButtonOf rather than Button, so the label stays this
			// widget's to color: a link is muted until it is the page.
			b.muted = append(b.muted, label)
			link := ButtonOf(label, c.onTap).Ghost().Pad(0, 2).Name(c.label)
			link.Role = ggui.RoleLink
			parts = append(parts, link)
		}
	}
	b.row = ggui.Row(parts...)
}

// Layout implements ggui.Widget.
func (b *BreadcrumbWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer b.props.Layout()()
	if b.nameReader != nil {
		b.name = b.nameReader.Get()
	}
	if b.row == nil {
		b.build()
	}
	t := uitheme.From(env)
	for _, w := range b.muted {
		w.Style(t.Text).Color(t.MutedFg)
	}
	if b.current != nil {
		b.current.Style(t.Text).Color(t.Fg)
	}
	return b.row.Gap(t.Space/2).Layout(c, env)
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
