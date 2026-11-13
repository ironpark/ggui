package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/ironpark/ggui"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// Picking a tab fades its surface in where it stands while the one left
// fades out, as shadcn's triggers do under transition-all; no surface
// travels between the two. A line strip's bar does the same.
func TestTabsCrossfadeTheShownSegmentInPlace(t *testing.T) {
	t.Parallel()
	for _, line := range []bool{false, true} {
		selected := ggui.State(0)
		tabs := Tabs(selected, Tab("One", ggui.Text("1")), Tab("Two", ggui.Text("2")), Tab("Three", ggui.Text("3")))
		if line {
			tabs.Line()
		}
		p := ggui.NewProbe(ggui.Column(tabs), ggui.Sz(320, 200))
		p.Advance(0)
		p.Frame()
		if got := tabs.shown; len(got) != 3 || got[0] != 1 || got[1] != 0 || got[2] != 0 {
			t.Fatalf("line=%v: at rest the labels are shown %v, want [1 0 0]", line, got)
		}
		p.Tap("Three")
		p.Frame()
		p.Advance(tabs.theme.MotionFast / 2)
		p.Frame()
		if a, b, c := tabs.shown[0], tabs.shown[1], tabs.shown[2]; !(a > 0 && a < 1) || b != 0 || !(c > 0 && c < 1) {
			t.Fatalf("line=%v: halfway the labels are shown %v, want the first leaving, the last arriving and the middle untouched", line, tabs.shown)
		}
		p.Advance(time.Second)
		p.Frame()
		if got := tabs.shown; got[0] != 0 || got[1] != 0 || got[2] != 1 {
			t.Fatalf("line=%v: settled the labels are shown %v, want [0 0 1]", line, got)
		}
		p.Close()
	}
}

// A label's color follows how far it is into being the shown one, so it
// eases with its surface rather than switching ahead of it.
func TestTabLabelColorEasesTowardTheTextColor(t *testing.T) {
	t.Parallel()
	tabs := Tabs(ggui.State(0), Tab("One", nil), Tab("Two", nil))
	p := ggui.NewProbe(ggui.Column(tabs), ggui.Sz(320, 200))
	defer p.Close()
	p.Frame()
	th := tabs.theme
	rest := fade(th.Fg, .6)
	if got := tabs.labelColor(1, 0); !sameColor(got, rest) {
		t.Fatalf("resting label %v, want %v", got, rest)
	}
	if got := tabs.labelColor(1, 1); !sameColor(got, th.Fg) {
		t.Fatalf("shown label %v, want %v", got, th.Fg)
	}
	if got := tabs.labelColor(1, .5); !sameColor(got, mix(rest, th.Fg, .5)) {
		t.Fatalf("label halfway %v, want %v", got, mix(rest, th.Fg, .5))
	}
}

// A closed section is shadcn's trigger: a 20px line inside py-2.5 and the
// one-pixel border that only focus colors, 42 in all, with the line under
// every section but the last taking a pixel more.
func TestAccordionSectionsHaveTheTriggerBorderAndRuleInTheirHeight(t *testing.T) {
	t.Parallel()
	one := Accordion(ggui.State([]string(nil)), AccordionItem("a", "A", ggui.Text("a")))
	three := Accordion(ggui.State([]string(nil)), AccordionItem("a", "A", ggui.Text("a")), AccordionItem("b", "B", ggui.Text("b")), AccordionItem("c", "C", ggui.Text("c")))
	env := uitheme.Default().Apply(ggui.Env{})
	c := ggui.Loose(ggui.Sz(320, 400))
	h1, h3 := one.Layout(c, env).H, three.Layout(c, env).H
	if h1 != 42 || h3 != 3*42+2 {
		t.Fatalf("one section is %v tall and three are %v, want 42 and 128", h1, h3)
	}
}

// A pointer over an accordion header underlines its title as CSS's
// hover:underline does: the same lines, laid out the same, each with a line
// under it, rather than one line under the last.
func TestAccordionHoverUnderlinesEveryLineOfTheTitle(t *testing.T) {
	t.Parallel()
	title := "A title long enough to wrap onto a second line in a narrow accordion"
	a := Accordion(ggui.State([]string(nil)), AccordionItem("a", title, ggui.Text("a")))
	spot := &placed{Widget: a}
	p := ggui.NewProbe(ggui.Column(spot).Align(ggui.AlignStretch), ggui.Sz(200, 400))
	defer p.Close()
	p.Frame()
	h := a.headers[0]
	if h.lined == nil || h.linedSize != h.size || h.size.H < 2*a.line {
		t.Fatalf("the underlined title is laid out %v and the plain one %v on lines %v tall, want the same, wrapped", h.linedSize, h.size, a.line)
	}
	if !strings.Contains(h.lined.Text(), "second line") {
		t.Fatalf("the underlined title reads %q", h.lined.Text())
	}
	p.Move(ggui.Pt(spot.at.Origin.X+20, spot.at.Origin.Y+20))
	p.Frame()
	if !h.Hovered {
		t.Fatal("the pointer over the header did not reach it")
	}
}

// A spinner is a status named "Loading" to a screen reader, shadcn's
// role="status" aria-label="Loading", until Name renames or removes it.
func TestSpinnerIsAStatusNamedLoading(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		spinner *SpinnerWidget
		name    string
	}{{Spinner(), "Loading"}, {Spinner().Name("Saving"), "Saving"}, {Spinner().Name(""), ""}} {
		p := ggui.NewProbe(ggui.Column(tc.spinner), ggui.Sz(100, 100))
		n, ok := p.Semantics().Find(ggui.RoleStatus, tc.name)
		if ok != (tc.name != "") {
			t.Errorf("spinner named %q: found a status %v, want %v", tc.name, ok, tc.name != "")
		}
		if ok && n.Rect.Size != ggui.Sz(16, 16) {
			t.Errorf("spinner named %q: its status is %v, want the spinner's 16x16", tc.name, n.Rect.Size)
		}
		p.Close()
	}
}

// An alert is one status holding its title, its description and its
// action, the nearest role to shadcn's role="alert".
func TestAlertIsAStatusAroundItsTextAndAction(t *testing.T) {
	t.Parallel()
	a := Alert("Upload failed", "Try again when connected.").Action(Button("Retry", nil))
	p := ggui.NewProbe(ggui.Column(a), ggui.Sz(320, 200))
	defer p.Close()
	tree := p.Semantics()
	status, ok := tree.Find(ggui.RoleStatus, "")
	if !ok {
		t.Fatalf("no status in\n%s", tree)
	}
	for _, want := range []struct {
		role ggui.Role
		name string
	}{{ggui.RoleText, "Upload failed"}, {ggui.RoleText, "Try again when connected."}, {ggui.RoleButton, "Retry"}} {
		n, ok := tree.Find(want.role, want.name)
		if !ok || !status.Rect.Contains(n.Rect.Origin) {
			t.Errorf("%s %q found=%v at %v, want it inside the status at %v\n%s", want.role, want.name, ok, n.Rect, status.Rect, tree)
		}
	}
}

// An ItemSeparator keeps shadcn's my-2 around its line, which a plain
// Divider does not.
func TestItemSeparatorKeepsASpaceAboveAndBelowItsLine(t *testing.T) {
	t.Parallel()
	env := uitheme.Default().Apply(ggui.Env{})
	c := ggui.Loose(ggui.Sz(200, 100))
	if got := Divider().Layout(c, env); got != ggui.Sz(200, 1) {
		t.Fatalf("a divider is %v, want 200x1", got)
	}
	sep := ItemSeparator()
	if got := sep.Layout(c, env); got != ggui.Sz(200, 17) {
		t.Fatalf("an item separator is %v, want 200x17", got)
	}
	r := ggui.Rct(ggui.Pt(10, 30), ggui.Sz(200, 17))
	if got, want := sep.line(r), ggui.Rct(ggui.Pt(10, 38), ggui.Sz(200, 1)); got != want {
		t.Fatalf("its line is at %v, want %v", got, want)
	}
}

// A progress bar's indicator is a square-ended run from the end reading
// starts at, which the track's round ends cut: the left of the track, or
// the right under a right-to-left direction.
func TestProgressIndicatorRunsFromTheReadingEdge(t *testing.T) {
	t.Parallel()
	bar := ggui.Rct(ggui.Pt(10, 20), ggui.Sz(200, 4))
	if got, want := progressFill(bar, .25, false), ggui.Rct(ggui.Pt(10, 20), ggui.Sz(50, 4)); got != want {
		t.Fatalf("left to right the indicator covers %v, want %v", got, want)
	}
	if got, want := progressFill(bar, .25, true), ggui.Rct(ggui.Pt(160, 20), ggui.Sz(50, 4)); got != want {
		t.Fatalf("right to left the indicator covers %v, want %v", got, want)
	}
	if got := progressFill(bar, 2, false); got != bar {
		t.Fatalf("a value past one covers %v, want the track %v", got, bar)
	}
}
