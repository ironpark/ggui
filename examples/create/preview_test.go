package main

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/ironpark/ggui"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

func previewTestNode(t *testing.T, p *ggui.Probe, role ggui.Role, name string) ggui.SemNode {
	t.Helper()
	n, ok := p.Semantics().Find(role, name)
	if !ok {
		t.Fatalf("missing %s %q:\n%s", role, name, p.Semantics())
	}
	return n
}

func TestCreatePreviewReflowsWithoutReplacingControls(t *testing.T) {
	t.Parallel()
	m := newPreview()
	if m.build() != m.build() {
		t.Fatal("preview build replaced the persistent tree")
	}
	p := ggui.NewProbe(m.build(), ggui.Sz(1180, 2400))
	defer p.Close()
	heading := func(name string) ggui.Rect { return previewTestNode(t, p, ggui.RoleHeading, name).Full }
	a, b, c := heading("Contribution History"), heading("Payout Threshold"), heading("Savings Targets")
	if !(a.Origin.X < b.Origin.X && b.Origin.X < c.Origin.X && a.Origin.Y == b.Origin.Y && b.Origin.Y == c.Origin.Y) {
		t.Fatal("wide preview does not have three columns", a, b, c)
	}
	p.Tap("Payout notes")
	p.Text("Keep this note")
	before := previewTestNode(t, p, ggui.RoleTextField, "Payout notes")
	p.Resize(ggui.Sz(800, 2400))
	p.Frame()
	a, b, c = heading("Contribution History"), heading("Payout Threshold"), heading("Savings Targets")
	if !(a.Origin.X < b.Origin.X && a.Origin.X == c.Origin.X && c.Origin.Y > a.Origin.Y) {
		t.Fatal("medium preview does not have two columns", a, b, c)
	}
	p.Resize(ggui.Sz(320, 2800))
	p.Frame()
	a, b, c = heading("Contribution History"), heading("Payout Threshold"), heading("Savings Targets")
	if a.Origin.X != b.Origin.X || b.Origin.X != c.Origin.X || !(a.Origin.Y < b.Origin.Y && b.Origin.Y < c.Origin.Y) {
		t.Fatal("narrow preview does not stack cards", a, b, c)
	}
	after := previewTestNode(t, p, ggui.RoleTextField, "Payout notes")
	if before.ID.ID != after.ID.ID || after.Value != "Keep this note" {
		t.Fatal("responsive layout replaced notes identity or draft")
	}
	if focused, ok := p.Semantics().Focused(); !ok || focused.Name != "Payout notes" {
		t.Fatal("responsive layout dropped editor focus")
	}
	for _, name := range []string{"Contribution History", "Payout Threshold", "Savings Targets", "Preferences", "Claimable Balance", "Distribute Track", "Recent Transactions", "Schedule a Payout", "Notifications"} {
		r := heading(name)
		if r.Origin.X < 24 || r.Origin.X+r.Size.W > 296.01 {
			t.Fatalf("%s overflows narrow preview: %v", name, r)
		}
	}
	for _, name := range []string{"Dec", "Jan", "Feb", "Mar", "Apr", "May"} {
		previewTestNode(t, p, ggui.RoleText, name)
	}
	for _, control := range []struct {
		role ggui.Role
		name string
	}{
		{ggui.RoleSelect, "Payout currency"}, {ggui.RoleSlider, "Payout amount"}, {ggui.RoleTextField, "Payout notes"},
		{ggui.RoleButton, "View Full Report"}, {ggui.RoleButton, "Save Threshold"},
	} {
		r := previewTestNode(t, p, control.role, control.name).Full
		if r.Origin.X < 31.99 || r.Origin.X+r.Size.W > 288.01 || r.Size.W < 250 {
			t.Fatalf("%s does not fill the narrow card safely: %v", control.name, r)
		}
	}

}

func TestCreatePreviewPayoutPreferencesAndNotifications(t *testing.T) {
	t.Parallel()
	m := newPreview()
	p := ggui.NewProbe(m.build(), ggui.Sz(1200, 2600))
	defer p.Close()
	p.Tap("Payout currency")
	p.Tap("EUR — Euro")
	slider := previewTestNode(t, p, ggui.RoleSlider, "Payout amount")
	if slider.Min != 50 || slider.Max != 10000 || slider.Now != 2500 {
		t.Fatal("payout range does not match sample", slider)
	}
	p.Perform(slider.ID, ggui.Action{Kind: ggui.ActionFocus})
	p.Type(ggui.Mods{}, ggui.KeyArrowRight)
	p.Tap("Payout notes")
	p.Text("Quarterly transfer")
	p.Tap("Save Threshold")
	if ggui.Peek(m.threshold) != 2525 || ggui.Peek(m.payoutCurrency) != "EUR" {
		t.Fatal("payout controls did not update")
	}
	previewTestNode(t, p, ggui.RoleStatus, "Threshold saved: €2,525.00. Note included.")
	p.Tap("Done")
	p.Tap("Email updates")
	p.Tap("Compact view")
	p.Tap("Preferred currency")
	p.Tap("GBP")
	p.Tap("Save preferences")
	if ggui.Peek(m.emailUpdates) || !ggui.Peek(m.compactView) || ggui.Peek(m.preferredCurrency) != "GBP" {
		t.Fatal("preference controls did not update")
	}
	previewTestNode(t, p, ggui.RoleStatus, "Preferences saved in this preview.")
	p.Tap("Done")
	p.Tap("Reset preferences")
	if !ggui.Peek(m.emailUpdates) || ggui.Peek(m.compactView) || ggui.Peek(m.preferredCurrency) != "USD" {
		t.Fatal("reset did not restore defaults")
	}
	p.Tap("Done")
	p.Tap("Product updates")
	p.Tap("Save notifications")
	previewTestNode(t, p, ggui.RoleStatus, "Saved: 3 of 3 notifications enabled.")
}

func TestCreatePreviewSavingsCalendarAndTransactions(t *testing.T) {
	t.Parallel()
	m := newPreview()
	p := ggui.NewProbe(m.build(), ggui.Sz(1200, 2600))
	defer p.Close()
	p.Tap("Manage savings targets")
	p.Tap("Add $1,000 to Retirement")
	p.Tap("Add $1,000 to Real Estate")
	p.Tap("Close savings")
	if ggui.Peek(m.retirement) != 274000 || ggui.Peek(m.property) != 28200 {
		t.Fatal("savings actions did not apply")
	}
	var progress []float64
	for _, n := range p.Semantics().Nodes(ggui.RoleProgress) {
		progress = append(progress, n.Now)
	}
	if len(progress) != 2 || math.Abs(progress[0]-274000.0/420000) > .0001 || math.Abs(progress[1]-28200.0/85000) > .0001 {
		t.Fatal("progress did not follow savings", progress)
	}
	p.Tap("2026-06-23")
	if ggui.Peek(m.selectedDate).Day() != 23 {
		t.Fatal("calendar did not select date")
	}
	previewTestNode(t, p, ggui.RoleStatus, "Selected: Tuesday, June 23")
	p.Tap("View all transactions")
	previewTestNode(t, p, ggui.RoleText, "Figma")
	p.Tap("Show fewer transactions")
	if _, ok := p.Semantics().Find(ggui.RoleText, "Figma"); ok {
		t.Fatal("extra transactions remained visible")
	}
	p.Tap("View Full Report")
	previewTestNode(t, p, ggui.RoleStatus, "December–May contributions: $13,350.00. Highest month: May, $3,000.00.")
	p.Tap("Done")
	p.Tap("Connect Bank")
	if ggui.Peek(m.balance) != 0 {
		t.Fatal("preview simulated a real payout")
	}
	previewTestNode(t, p, ggui.RoleStatus, "Bank connections are not available in this local preview. No account was connected and no funds were transferred.")
	p.Tap("Done")
	p.Tap("Create Release")
	p.Tap("Save release draft")
	previewTestNode(t, p, ggui.RoleStatus, "Enter a release title to save your draft.")
	p.Tap("Release title")
	p.Text("Midnight Sessions")
	p.Tap("Save release draft")
	previewTestNode(t, p, ggui.RoleStatus, "Draft saved locally: Midnight Sessions.")
	p.Tap("Close release")
	p.Tap("Create Release")
	if previewTestNode(t, p, ggui.RoleTextField, "Release title").Value != "Midnight Sessions" {
		t.Fatal("release draft did not persist")
	}
	p.Tap("Close release")
	chart := previewTestNode(t, p, ggui.RoleGroup, "Monthly contributions")
	p.Perform(chart.ID, ggui.Action{Kind: ggui.ActionFocus})
	p.Type(ggui.Mods{}, ggui.KeyHome, ggui.KeyEnter)
	if !strings.Contains(ggui.Peek(m.chartFeedback), "contribution:") {
		t.Fatal("chart selection did not produce feedback")
	}
}

func TestCreatePreviewFollowsInheritedThemeAndScrolls(t *testing.T) {
	t.Parallel()
	m := newPreview()
	dark := ggui.State(false)
	p := ggui.ProbeBuilder(func() ggui.Widget {
		return ggui.Reactive(func() ggui.Widget {
			theme := uitheme.Default()
			if dark.Get() {
				theme = uitheme.Dark()
			}
			return uitheme.With(theme, m.build())
		})
	}, ggui.Sz(680, 420))
	defer p.Close()
	p.Tap("Payout notes")
	p.Text("Persistent draft")
	dark.Set(true)
	p.Frame()
	if ggui.Peek(m.notes) != "Persistent draft" {
		t.Fatal("theme change reset input")
	}
	p.Scroll(ggui.Pt(500, 300), ggui.Pt(0, -10))
	p.Advance(250 * time.Millisecond)
	if m.scroll.Offset() <= 0 {
		t.Fatal("preview does not scroll")
	}
}

func TestCreatePreviewDesktopPanPreservesVerticalState(t *testing.T) {
	t.Parallel()
	m := newPreview()
	p := ggui.NewProbe(m.build(), ggui.Sz(1100, 600))
	defer p.Close()
	p.Frame()
	if math.Abs(m.pan.MaxOffset()-188) > .01 {
		t.Fatalf("desktop pan extent = %v", m.pan.MaxOffset())
	}
	p.Tap("Payout notes")
	p.Text("Keep while panning")
	before := previewTestNode(t, p, ggui.RoleTextField, "Payout notes")
	p.Scroll(ggui.Pt(500, 300), ggui.Pt(-10, 0))
	p.Advance(250 * time.Millisecond)
	if m.pan.Offset() <= 0 {
		t.Fatal("desktop does not pan horizontally")
	}
	m.scroll.JumpTo(180)
	p.Frame()
	vertical := m.scroll.Offset()
	p.Resize(ggui.Sz(800, 600))
	p.Frame()
	after := previewTestNode(t, p, ggui.RoleTextField, "Payout notes")
	if before.ID.ID != after.ID.ID || after.Value != "Keep while panning" || math.Abs(m.scroll.Offset()-vertical) > .01 {
		t.Fatalf("pan breakpoint lost draft, identity, or vertical offset: before %v, after %v, scroll %v→%v", before.ID, after.ID, vertical, m.scroll.Offset())
	}
}
