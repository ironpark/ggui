package ui

import (
	"testing"

	"github.com/ironpark/ggui"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

func TestCommandSearchIconLeadsTheField(t *testing.T) {
	t.Parallel()
	c := Command(ggui.State(""), CommandItem("Alpha", nil))
	p := ggui.NewProbe(c, ggui.Sz(320, 240))
	defer p.Close()
	pill, ok := p.Find("Search commands")
	if !ok {
		t.Fatal("no search field")
	}
	icon := c.searchRow.icon
	// shadcn's pl-2, from the edge of the pill.
	if got, want := icon.Origin.X-pill.Rect.Origin.X, 8.0; got != want {
		t.Fatalf("the search icon starts %v into the field, want %v", got, want)
	}
	rtl := Command(ggui.State(""), CommandItem("Alpha", nil))
	q := ggui.NewProbe(ggui.Directionality(ggui.DirRTL, rtl), ggui.Sz(320, 240))
	defer q.Close()
	pill, _ = q.Find("Search commands")
	icon = rtl.searchRow.icon
	if got, want := pill.Rect.Origin.X+pill.Rect.Size.W-icon.Origin.X-icon.Size.W, 8.0; got != want {
		t.Fatalf("under RTL the search icon ends %v from the field's right, want %v", got, want)
	}
}

func TestSheetCastsTheOverlayShadowAndADrawerNone(t *testing.T) {
	t.Parallel()
	th := uitheme.Default()
	// shadcn's SheetContent is shadow-lg, which is the token as it stands.
	if got := Sheet(ggui.State(true), ggui.Text("Body")).shape(&th).shadow; got != th.OverlayShadow {
		t.Fatalf("a sheet casts %+v, want the theme's OverlayShadow %+v", got, th.OverlayShadow)
	}
	if got := Drawer(ggui.State(true), ggui.Text("Body")).shape(&th).shadow; got != (ggui.ShadowStyle{}) {
		t.Fatalf("a drawer casts %+v, want none", got)
	}
}
