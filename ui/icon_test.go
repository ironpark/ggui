package ui_test

import (
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
	"github.com/ironpark/ggui/ui/icons"
	"github.com/ironpark/ggui/ui/icons/lucide"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

type observedIcons struct{ seen map[icons.Role]int }

func (s *observedIcons) Resolve(role icons.Role) *icons.SVG {
	s.seen[role]++
	return lucide.Set().Resolve(role)
}
func TestControlsResolveInheritedIcons(t *testing.T) {
	set := &observedIcons{seen: map[icons.Role]int{}}
	tree := ggui.Provide(icons.SetKey, icons.Set(set), ggui.Column(
		ui.Checkbox(ggui.State(true), "Check"),
		ui.Select(ggui.State("one")).Options([]string{"one", "two"}),
		ui.Collapsible(ggui.State(false), "More", ggui.Text("Body")),
		ui.Spinner(), ui.Icon(icons.Close),
	))
	p := ggui.NewProbe(tree, ggui.Sz(300, 400))
	defer p.Close()
	p.Frame()
	for _, role := range []icons.Role{icons.Check, icons.ChevronDown, icons.ChevronRight, icons.Loader, icons.Close} {
		if set.seen[role] == 0 {
			t.Errorf("control did not resolve %s through inherited set", role)
		}
	}
}
func TestIconDecorativeAndAccessible(t *testing.T) {
	p := ggui.NewProbe(ggui.Row(ui.Icon(icons.Check), ui.Icon(icons.Download).Alt("Download icon")), ggui.Sz(100, 30))
	defer p.Close()
	node(t, p.Semantics(), ggui.RoleImage, "Download icon")
}

type iconColorProbe struct{ env ggui.Env }

func (w *iconColorProbe) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	w.env = env
	return c.Constrain(ggui.Sz(16, 16))
}
func (*iconColorProbe) Paint(*ggui.Canvas, ggui.Rect) {}
func TestIconButtonInheritsForeground(t *testing.T) {
	content := &iconColorProbe{}
	b := ui.ButtonOf(content, nil)
	env := ggui.Env{}
	b.Layout(ggui.Loose(ggui.Sz(100, 50)), env)
	if content.env.Text().Color != uitheme.From(env).PrimaryFg {
		t.Fatal("icon button did not inherit primary foreground")
	}
	b.Disabled(true).Layout(ggui.Loose(ggui.Sz(100, 50)), env)
	if content.env.Text().Color == uitheme.From(env).PrimaryFg {
		t.Fatal("disabled icon did not inherit muted foreground")
	}
}
