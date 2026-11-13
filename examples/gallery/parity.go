package main

import (
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
	"github.com/ironpark/ggui/ui/icons"
)

// newParityPreviews keeps controlled state alive while the gallery is rebuilt.
func newParityPreviews() func() []ggui.Widget {
	bold, starred, pinned := ggui.State(false), ggui.State(true), ggui.State(false)
	formats := ggui.State([]string{"Bold"})
	languages := ggui.State([]string{"Go", "TypeScript"})
	budget := ggui.State([]float64{25, 75})
	destination := ggui.State("Choose a destination")
	nativeValue := ggui.State("Standard")
	tabPage := ggui.State(0)
	dateStart := time.Date(2026, time.November, 10, 0, 0, 0, 0, time.Local)
	dateRange := ggui.State(ui.DateRange{Start: dateStart, End: dateStart.AddDate(0, 0, 9)})
	dates := ggui.State([]time.Time{dateStart, dateStart.AddDate(0, 0, 2)})
	popover := ui.Popover("Edit profile", ggui.Column(
		ui.Field("Display name", ui.TextField(ggui.State("Ada Lovelace"))),
	).Gap(8).Stretch()).Title("Profile").Description("Update the name shown on your profile.")
	sidePopover := ui.Popover("Open to the right", ui.Caption("A popover can open on any side of its trigger, and flips when that side has no room.")).
		Title("Placement").Side(ggui.PopupRight).Align(ggui.PopupStart)
	nav := ui.NavigationMenu(
		ui.NavigationMenuLink("Home", func() { destination.Set("Home selected") }),
		ui.NavigationMenuItem("Explore", ggui.Column(
			ui.NavigationMenuLink("Components", func() { destination.Set("Components selected") }),
			ui.NavigationMenuLink("Documentation", func() { destination.Set("Documentation selected") }),
		).Gap(4).Stretch()),
		ui.NavigationMenuLink("Settings", func() { destination.Set("Settings selected") }),
	).Name("Main navigation")
	trace := ui.Trace(
		ui.TraceStep{ID: "fetch", Title: "Fetched repository", Status: ui.TraceCompleted, Duration: 240 * time.Millisecond},
		ui.TraceStep{ID: "build", Title: "Build application", Status: ui.TraceRunning, Details: ui.CodeBlock("go build ./...\n").Language("shell"), Children: []ui.TraceStep{
			{ID: "compile", Title: "Compile packages", Status: ui.TraceCompleted, Duration: 120 * time.Millisecond},
			{ID: "bundle", Title: "Bundle assets", Status: ui.TracePending},
		}},
		ui.TraceStep{ID: "publish", Title: "Publish release", Status: ui.TracePending},
	)
	code := ui.CodeBlock("package main\n\nimport \"fmt\"\n\nfunc main() {\n    fmt.Println(\"Hello, GGUI\")\n}\n").Header("main.go").Language("go").LineNumbers(true).LineStyles(map[int]ui.CodeLineStyle{3: ui.CodeLineHighlighted, 6: ui.CodeLineAdded})
	drawerOpen := ggui.State(false)
	drawer := ui.Drawer(drawerOpen, ggui.Column(ui.Heading("Resizable drawer"), ui.Caption("Drag the handle to change the height or dismiss."), ui.Button("Done", func() { drawerOpen.Set(false) })).Gap(12)).SnapPoints(160, 280, 420).SnapPoint(1).Handle(true)
	return func() []ggui.Widget {
		return []ggui.Widget{
			preview("Popover", ggui.Wrap(popover, sidePopover).Gap(8)),
			preview("Navigation menu", ggui.Column(nav, ui.CaptionOf(destination)).Gap(12)),
			preview("Code block", code),
			preview("Trace", trace),
			preview("Range slider", ggui.Column(ui.RangeSlider(budget, 0, 100).Step(5).MinGap(10).ThumbNames("Minimum budget", "Maximum budget"), ui.Captionf("Budget: %v", budget)).Gap(12)),
			preview("Toggle and label", ggui.Column(ggui.Wrap(ui.Toggle(bold, "Bold").Outline(), ui.Toggle(starred, "Starred").Icon(ui.Icon(icons.Check)), ui.ToggleOf(pinned, ui.Icon(icons.Plus)).Name("Pin").Outline()).Gap(8), ui.ToggleGroupMultiple(formats).Options([]string{"Bold", "Italic", "Underline"}).Name("Formatting"), ui.Field("Associated label", ui.TextField(ggui.State("Click the label to focus")))).Gap(12)),
			preview("Multiple combobox", ui.ComboboxMultiple(languages).Options([]string{"Go", "TypeScript", "Rust", "Python", "Swift"}).Name("Languages")),
			preview("Native select", ui.NativeSelect(nativeValue).Options([]string{"Standard", "Express", "Overnight"}).Name("Delivery speed")),
			preview("Drawer snapping", ggui.Column(ui.Button("Try drawer snapping", func() { drawerOpen.Set(true) }).Outline(), ui.Caption("Three snap points; keyboard and touch handle support.")).Gap(8)),
			drawer,
			preview("Calendar modes", ggui.Column(
				ui.Caption("Inclusive range across weeks, two months side by side"), ui.CalendarRange(dateRange).RangeNights(1, 14).ResetOnSelect(true).Months(2).Name("Trip dates"),
				ui.Caption("Independent dates"), ui.CalendarMultiple(dates).SelectionLimits(0, 4).Name("Available dates"),
			).Gap(8)),
			preview("Tabs modes", ui.Tabs(tabPage, ui.Tab("Profile", ggui.Text("Account information")), ui.Tab("Locked", ggui.Text("Unavailable")).Disabled(true), ui.Tab("Security", ggui.Text("Security preferences"))).Vertical().Line()),
			preview("RTL controls", ggui.Directionality(ggui.DirRTL, ggui.Column(
				ui.Tabs(ggui.State(0), ui.Tab("الأول", ggui.Text("مرحبا")), ui.Tab("الثاني", ggui.Text("שלום"))),
				ui.ToggleGroup(ggui.State("A")).Options([]string{"A", "B", "C"}),
				ui.Slider(ggui.State(30.0), 0, 100),
				ui.Checkbox(ggui.State(true), "אפשרות"),
			).Gap(12))),
		}
	}
}
