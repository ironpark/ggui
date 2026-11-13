package ui_test

import (
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
	"github.com/ironpark/ggui/ui/icons"
)

func TestBreadcrumbSeparatesStepsWithAChevronTextOrAWidget(t *testing.T) {
	t.Parallel()
	crumbs := func() []ui.BreadcrumbEntry {
		return []ui.BreadcrumbEntry{ui.Crumb("Home", func() {}), ui.Crumb("Projects", func() {}), ui.Crumb("ggui", nil)}
	}
	texts := func(b *ui.BreadcrumbWidget) int {
		p := ggui.NewProbe(b, ggui.Sz(400, 40))
		defer p.Close()
		return count(p, ggui.RoleText)
	}
	if got := texts(ui.Breadcrumb(crumbs()...)); got != 1 {
		t.Fatalf("a trail with chevrons has %d texts, want the current step alone", got)
	}
	if got := texts(ui.Breadcrumb(crumbs()...).Separator("/")); got != 3 {
		t.Fatalf("a trail with text separators has %d texts, want the current step and 2 separators", got)
	}
	custom := ui.Breadcrumb(crumbs()...).SeparatorOf(ui.Icon(icons.Minus).Alt("then"))
	p := ggui.NewProbe(custom, ggui.Sz(400, 40))
	defer p.Close()
	if got := count(p, ggui.RoleImage); got != 2 {
		t.Fatalf("SeparatorOf drew its widget %d times, want once at each of 2 gaps", got)
	}
}

// count is how many nodes of role the probe's tree holds.
func count(p *ggui.Probe, role ggui.Role) int {
	n := 0
	for range p.Semantics().Nodes(role) {
		n++
	}
	return n
}

func TestBreadcrumbWrapsWhenTheTrailIsTooLong(t *testing.T) {
	t.Parallel()
	var crumbs []ui.BreadcrumbEntry
	for _, label := range []string{"Workspace", "Documentation", "Components", "Navigation", "Trail"} {
		crumbs = append(crumbs, ui.Crumb(label, func() {}))
	}
	wide := ggui.NewProbe(ggui.Column(ui.Breadcrumb(crumbs...)), ggui.Sz(900, 200))
	defer wide.Close()
	narrow := ggui.NewProbe(ggui.Column(ui.Breadcrumb(crumbs...)), ggui.Sz(220, 200))
	defer narrow.Close()
	first, _ := narrow.Semantics().Find(ggui.RoleLink, "Workspace")
	last, ok := narrow.Semantics().Find(ggui.RoleText, "Trail")
	if !ok || last.Rect.Origin.Y <= first.Rect.Origin.Y {
		t.Fatalf("a trail wider than its space stayed on one line: first %v, last %v", first.Rect, last.Rect)
	}
	if last.Rect.Origin.X+last.Rect.Size.W > 220 {
		t.Fatalf("the wrapped trail runs past its width: %v", last.Rect)
	}
	first, _ = wide.Semantics().Find(ggui.RoleLink, "Workspace")
	last, _ = wide.Semantics().Find(ggui.RoleText, "Trail")
	if last.Rect.Origin.Y != first.Rect.Origin.Y {
		t.Fatal("a trail that fits wrapped")
	}
}

func TestBreadcrumbLinkTakesKeyboardAndAccessibleActivation(t *testing.T) {
	t.Parallel()
	home := 0
	p := ggui.NewProbe(ui.Breadcrumb(ui.Crumb("Home", func() { home++ }), ui.Crumb("Here", nil)), ggui.Sz(300, 40))
	defer p.Close()
	p.Key("tab")
	if focused, _ := p.Semantics().Focused(); focused.Name != "Home" || focused.Role != ggui.RoleLink {
		t.Fatalf("Tab focused %+v, want the Home link", focused)
	}
	p.Key("enter")
	act(t, p, ggui.RoleLink, "Home", ggui.Action{Kind: ggui.ActionPress})
	if home != 2 {
		t.Fatalf("the link ran %d times, want once for Enter and once for the press action", home)
	}
}
