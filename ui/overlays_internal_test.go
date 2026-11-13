package ui

import (
	"slices"
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
	// shadcn's SheetContent is shadow-lg: the token and its tight layer.
	if got, want := Sheet(ggui.State(true), ggui.Text("Body")).shape(&th).shadow, ([2]ggui.ShadowStyle{th.OverlayShadow, th.OverlayContact}); got != want {
		t.Fatalf("a sheet casts %+v, want the theme's OverlayShadow over its OverlayContact %+v", got, want)
	}
	if got := Drawer(ggui.State(true), ggui.Text("Body")).shape(&th).shadow; got != ([2]ggui.ShadowStyle{}) {
		t.Fatalf("a drawer casts %+v, want none", got)
	}
}

func TestCommandScoreRanksAbbreviations(t *testing.T) {
	t.Parallel()
	if got := commandScore("Calendar", "Calendar"); got != 1 {
		t.Errorf("the same text scores %v, want 1", got)
	}
	if got := commandScore("Calendar", "xyz"); got != 0 {
		t.Errorf("letters the text has none of score %v, want 0", got)
	}
	if got := commandScore("Calendar", "rc"); got != 0 {
		t.Errorf("letters out of order score %v, want 0", got)
	}
	// A prefix scores the whole of what an unfinished match keeps, a jump
	// to the start of a word most of it, and one into a word little.
	if got := commandScore("Calendar", "cal"); got != penaltyIncomplete*penaltyCase {
		t.Errorf("a prefix in the other case scores %v, want %v", got, penaltyIncomplete*penaltyCase)
	}
	initials, inside := commandScore("Toggle dark theme", "tdt"), commandScore("Toggle dark theme", "gg")
	if initials <= inside || inside <= 0 {
		t.Errorf("initials score %v and letters inside a word %v, want the first above the second above 0", initials, inside)
	}
	if word, split := commandScore("Open dark mode", "dark"), commandScore("Drag marker", "dark"); word <= split || split <= 0 {
		t.Errorf("a whole word scores %v and its letters apart %v, want the first above the second above 0", word, split)
	}
	if got := commandScore("Calendar", "claendar"); got <= 0 {
		t.Errorf("two letters the wrong way round score %v, want a little", got)
	}
}

func TestCommandListsTheBestMatchFirst(t *testing.T) {
	t.Parallel()
	query := ggui.State("")
	picked := ""
	item := func(label string) CommandEntry { return CommandItem(label, func() { picked = label }) }
	c := Command(query,
		item("Drag marker").Group("Files"),
		item("Rename"),
		item("Open dark mode").Group("View").Keywords("night"),
		item("Dark reader").Group("View").Disabled(true),
		item("Darken colors"),
	)
	p := ggui.NewProbe(c, ggui.Sz(400, 400))
	defer p.Close()
	labels := func() []string {
		var out []string
		for _, i := range c.matched {
			out = append(out, c.entries[i].label)
		}
		return out
	}
	query.Set("dark")
	p.Frame()
	// The loose entry first, then the group with the better match, the
	// better match first inside it.
	want := []string{"Darken colors", "Dark reader", "Open dark mode", "Drag marker"}
	if got := labels(); !slices.Equal(got, want) {
		t.Fatalf("\"dark\" lists %q, want %q", got, want)
	}
	// The highlight starts on the first of them that can be run.
	p.Tap("Search commands")
	p.Type(ggui.Mods{}, ggui.KeyEnter)
	if picked != "Darken colors" {
		t.Fatalf("Enter ran %q, want the first match", picked)
	}
	// A keyword is searched as the label is, and nothing else matches it.
	query.Set("night")
	p.Frame()
	if got := labels(); !slices.Equal(got, []string{"Open dark mode"}) {
		t.Fatalf("\"night\" lists %q, want the entry with that keyword", got)
	}
	// With nothing typed the entries are in the order they were given.
	query.Set("")
	p.Frame()
	if got := labels(); got[0] != "Drag marker" || len(got) != 5 {
		t.Fatalf("an empty query lists %q", got)
	}
}
