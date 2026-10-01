package ui_test

import (
	"testing"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

func TestDatePickerPlaceholderAndFormatShowOnTheTrigger(t *testing.T) {
	t.Parallel()
	// The trigger is as wide as the text on it.
	width := func(d *ui.DatePickerWidget) float64 {
		p := ggui.NewProbe(ggui.Row(d.Name("Due")), ggui.Sz(380, 450))
		defer p.Close()
		return find(t, p, ggui.RoleButton, "Due").Rect.Size.W
	}
	short := width(ui.DatePicker(ggui.State(time.Time{})).Placeholder("-"))
	long := width(ui.DatePicker(ggui.State(time.Time{})).Placeholder("No due date yet"))
	if long <= short {
		t.Fatalf("a long placeholder left the trigger %v wide, a short one %v", long, short)
	}
	value := ggui.State(time.Time{})
	d := ui.DatePicker(value).Name("Due").Format(func(t time.Time) string { return t.Format("Jan 2") }).Format(nil)
	d.Calendar().Location(time.UTC)
	p := ggui.NewProbe(ggui.Column(d), ggui.Sz(380, 450))
	defer p.Close()
	if trigger, _ := p.Semantics().Find(ggui.RoleButton, "Due"); trigger.Value != "" {
		t.Fatalf("a zero date has the value %q, want none", trigger.Value)
	}
	value.Set(date(2024, 3, 14))
	if trigger, _ := p.Semantics().Find(ggui.RoleButton, "Due"); trigger.Value != "Mar 14" {
		t.Fatalf("trigger value %q, want the custom format Mar 14 (Format(nil) keeps it)", trigger.Value)
	}
}

func TestDatePickerExpandAndCollapseActions(t *testing.T) {
	t.Parallel()
	d := ui.DatePicker(ggui.State(date(2024, 3, 14))).Name("Due")
	p := ggui.NewProbe(ggui.Column(d), ggui.Sz(380, 450))
	defer p.Close()
	trigger, _ := p.Semantics().Find(ggui.RoleButton, "Due")
	p.Perform(trigger.ID, ggui.Action{Kind: ggui.ActionExpand})
	if !d.Popup().IsOpen() {
		t.Fatal("ActionExpand did not open the calendar")
	}
	p.Perform(trigger.ID, ggui.Action{Kind: ggui.ActionCollapse})
	if d.Popup().IsOpen() {
		t.Fatal("ActionCollapse did not close the calendar")
	}
	if d.Act(ggui.Action{Kind: ggui.ActionScrollIntoView}) {
		t.Fatal("the picker claimed an action it does not handle")
	}
	disabled := ggui.State(true)
	d.BindDisabled(disabled)
	p.Frame()
	if d.Act(ggui.Action{Kind: ggui.ActionExpand}) || d.Popup().IsOpen() {
		t.Fatal("a disabled picker opened")
	}
}

func TestDatePickerKeyKeepsThePopupAndMonthAcrossAMovingRebuild(t *testing.T) {
	t.Parallel()
	value := ggui.State(date(2024, 3, 14))
	offset := ggui.State(0.0)
	var picker *ui.DatePickerWidget
	p := ggui.ProbeBuilder(func() ggui.Widget {
		return ggui.Reactive(func() ggui.Widget {
			picker = ui.DatePicker(value).Name("Due").Key("due")
			picker.Calendar().Location(time.UTC)
			return ggui.Column(ggui.Box().Height(offset.Get()), picker)
		})
	}, ggui.Sz(400, 520))
	defer p.Close()
	p.Tap("Due")
	p.Tap("Next month")
	offset.Set(30)
	p.Frame()
	if !picker.Popup().IsOpen() {
		t.Fatal("the rebuilt picker closed its calendar")
	}
	if _, ok := p.Semantics().Find(ggui.RoleOption, "2024-04-30"); !ok {
		t.Fatalf("the rebuilt picker lost the browsed month (April):\n%s", p.Semantics())
	}
}

func TestDatePickerBindNameNamesTriggerAndCalendar(t *testing.T) {
	t.Parallel()
	name := ggui.State("Start")
	d := ui.DatePicker(ggui.State(date(2024, 3, 14))).BindName(name)
	p := ggui.NewProbe(ggui.Column(d), ggui.Sz(380, 450))
	defer p.Close()
	p.Tap("Start")
	if _, ok := p.Semantics().Find(ggui.RoleGroup, "Start"); !ok {
		t.Fatalf("the calendar is not named after the picker:\n%s", p.Semantics())
	}
	name.Set("Begin")
	if _, ok := p.Semantics().Find(ggui.RoleButton, "Begin"); !ok {
		t.Fatal("the trigger's name did not follow its reader")
	}
}

func TestCalendarWeekdayAndMonthLabelsLocalize(t *testing.T) {
	t.Parallel()
	c := ui.Calendar(ggui.State(date(2024, 3, 14))).Location(time.UTC).WeekStartsOn(time.Monday).
		WeekdayLabels([7]string{"일", "월", "화", "수", "목", "금", "토"}).
		MonthLabel(func(t time.Time) string { return t.Format("2006.01") }).MonthLabel(nil)
	p := ggui.NewProbe(c, ggui.Sz(320, 400))
	defer p.Close()
	tree := p.Semantics()
	if _, ok := tree.Find(ggui.RoleText, "2024.03"); !ok {
		t.Fatalf("custom month label missing:\n%s", tree)
	}
	var days []string
	for _, n := range tree.Nodes(ggui.RoleText) {
		if len([]rune(n.Name)) == 1 {
			days = append(days, n.Name)
		}
	}
	// Monday first: the Sunday label moves to the end.
	if len(days) != 7 || days[0] != "월" || days[6] != "일" {
		t.Fatalf("weekday headings %q, want Monday-first Korean labels", days)
	}
}
