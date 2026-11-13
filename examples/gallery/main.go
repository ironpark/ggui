// Command gallery shows the layout widgets and the ui controls together:
// Wrap and Grid, every control bound to a signal, a labelled Field with
// validation, a modal Dialog, tooltips, and the inspector on F1.
package main

import (
	"flag"
	"fmt"
	"image/color"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/ironpark/ggui"
	_ "github.com/ironpark/ggui/inspect/panel"
	"github.com/ironpark/ggui/ui"
	"github.com/ironpark/ggui/ui/icons"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// Person is a row of the table below. Rows are keyed by ID, so editing or
// removing one keeps every other row's widgets.
type Person struct {
	ID   int
	Name string
	Role string
	Age  int
}

func newGallery() (ggui.Builder, func(), func()) { return newGalleryPreview(nil) }

// newGalleryPreview lets the visual audit render the same examples individually.
func newGalleryPreview(present func([]ggui.Widget) ggui.Widget) (ggui.Builder, func(), func()) {
	// The mode is the theme's source of truth; dark reads it, and the
	// header's day/night switch writes a fixed light or dark mode back.
	mode := ggui.State(uitheme.ModeLight)
	dark := ggui.Bind(func() bool {
		if m := mode.Get(); m != uitheme.ModeSystem {
			return m == uitheme.ModeDark
		}
		return ggui.SystemDark().Get()
	}, func(v bool) {
		if v {
			mode.Set(uitheme.ModeDark)
		} else {
			mode.Set(uitheme.ModeLight)
		}
	})
	baseColor := ggui.State(uitheme.BaseNeutral)
	accentColor := ggui.State(uitheme.AccentBase)
	themeStyle := ggui.State(uitheme.StyleNova)
	themeRadius := ggui.State(uitheme.RadiusDefault)
	menuAccent := ggui.State(uitheme.MenuAccentSubtle)
	menuColor := ggui.State(uitheme.MenuColorDefault)
	chartColor := ggui.State(uitheme.AccentBase)
	iconLibrary := ggui.State(uitheme.IconsDefault)
	themeAction := ggui.State("Change a token preset or try the actions.")
	emojiText := ggui.State("Hello 👋🏽  🇰🇷  👩🏽‍💻  1️⃣")
	chatPreviews := newChatPreviews()
	toasts := ui.NewToaster()
	toastPosition := ggui.State(ui.ToastBottomRight)
	parityPreviews := newParityPreviews()
	partPreviews := newPartPreviews(toasts)
	selectedDate := ggui.State(time.Now())
	calendar := ui.Calendar(selectedDate).WeekStartsOn(time.Monday).CaptionDropdown(true)
	stay := ggui.State(ui.DateRange{})
	rangePicker := ui.DateRangePicker(stay).Name("Stay dates")
	rangePicker.Calendar().WeekStartsOn(time.Monday)
	datePicker := ui.DatePicker(selectedDate).Name("Appointment date")
	datePicker.Calendar().WeekStartsOn(time.Monday)
	search := ggui.State("")
	category := ggui.State("All")
	scroll := ggui.State(0.0)
	verticalSplit := ggui.State(.45)
	people := ggui.State([]Person{
		{1, "Ada", "Engineer", 36}, {2, "Grace", "Admiral", 45}, {3, "Linus", "Kernel", 28},
		{4, "Ken", "Plan 9", 60}, {5, "Rob", "Gopher", 62}, {6, "Barbara", "Architect", 57},
	})
	paymentTable := newPaymentTable()
	chosen := ggui.State(0)
	chosenName := ggui.Combine(people, chosen, func(ps []Person, id int) string {
		for _, p := range ps {
			if p.ID == id {
				return p.Name
			}
		}
		return "nobody"
	})
	name := ggui.State("")
	carouselIndex, otpValue := ggui.State(0), ggui.State("")
	email := ggui.State("")
	confirm := ggui.State(false)
	cleared := ggui.State(0)
	size := ggui.State(16.0)
	notes := ggui.State("")
	fruit := ggui.State("Banana")
	details := ggui.State(true)
	revealed := ggui.State(false)
	tab := ggui.State(0)
	more := ggui.State(false)
	progress := ggui.State(0.3)
	page, pageCount := ggui.State(1), ggui.State(12)
	accordionOpen := ggui.State([]string{"intro"})
	split := ggui.State(.35)
	commandQuery := ggui.State("")
	paletteOpen := ggui.State(false)
	notify := ggui.State(true)
	plan := ggui.State("free")
	tags := ggui.State([]string{"go", "gui", "ebiten", "signals", "flutter", "svelte", "layout", "hidpi"})
	nav := ggui.State("inbox")
	align := ggui.State("center")
	site := ggui.State("")
	groupAction := ggui.State("Choose an action")
	sitePreview := ggui.State("Enter a site and press Go")
	textStyle := ggui.State([]string{"Bold"})
	searchQuery := ggui.State("")
	zoom := ggui.State(100)
	controlsLocked := ggui.State(false)
	disclosureOpen := ggui.State(false)
	download := ggui.State(0.0)
	filtersOpen, drawerOpen, removeOpen := ggui.State(false), ggui.State(false), ggui.State(false)
	removed := ggui.State(0)

	// emailError follows the field and is empty while the address looks
	// right; Field shows it in place of the help text.
	emailError := email.Map(func(s string) string {
		if s == "" || strings.Contains(s, "@") {
			return ""
		}
		return "An address needs an @"
	})
	// The menubar owns its menus' open state, so it outlives a rebuild the
	// same way the calendar and the date picker do.
	menubar := ui.Menubar(
		ui.Menu("File", ui.MenuItem("New document", func() { toasts.Push(ui.Toast("Created", "A new document is ready.")) }), ui.MenuItem("Export", nil).Disabled(true)),
		ui.Menu("Edit", ui.MenuItem("Undo edit", func() { toasts.Push(ui.Toast("Undone", "The last edit was reverted.")) })),
		ui.Menu("View", ui.MenuItem("Switch theme", func() { dark.Set(!ggui.Peek(dark)) })),
	).Compact()
	// The sheets, the drawer, the confirmation and the sidebar own state that
	// has to outlive a rebuild -- an animation in flight, the highlighted
	// destination -- so they are built once, beside the signals they bind.
	filtersSheet := ui.Sheet(filtersOpen, ggui.Column(
		ui.Checkbox(notify, "Only unread"),
		ui.Radios(plan).Options([]string{"free", "pro", "team"}),
	).Space(1.5).Stretch()).Title("Filters").Description("Narrow the list to what you need.").Size(320).
		Footer(ui.Button("Apply", func() { filtersOpen.Set(false) }), ui.Button("Cancel", func() { filtersOpen.Set(false) }).Outline())
	shareDrawer := ui.Drawer(drawerOpen, ggui.Column(
		ggui.Text("A drawer rises from the bottom edge, as tall as what it holds, with a grab handle on top."),
		ui.Button("Close", func() { drawerOpen.Set(false) }).Outline(),
	).Space(1.5).Stretch()).Title("Share").Description("Send this page to a teammate.").Handle(true)
	removeDialog := ui.AlertDialog(removeOpen, "Remove the row?", "A click beside this question will not dismiss it.").
		Confirm("Remove", func() { ggui.Add(removed, 1) }).Destructive().Media(ui.Icon(icons.Alert))
	sidebarCollapsed := ggui.State(false)
	navBar := ui.Sidebar(nav,
		ui.SidebarSection("Mail"),
		ui.SidebarItem("inbox", "Inbox").Icon(icons.Download).Badge("12").Tooltip("Inbox"),
		ui.SidebarItem("sent", "Sent").Icon(icons.Redo).Tooltip("Sent"),
		ui.SidebarItem("spam", "Spam").Icon(icons.Alert).Disabled(true),
		ui.SidebarSection("Workspace"),
		ui.SidebarItem("projects", "Projects").Icon(icons.File).Tooltip("Projects").Collapsible().Open(true).
			Action(ui.ButtonOf(ui.Icon(icons.Plus), func() { nav.Set("projects/new") }).Name("New project").Ghost().Size(ui.ButtonIconExtraSmall)).ActionOnHover().
			Sub(ui.SidebarItem("projects/design", "Design"), ui.SidebarItem("projects/research", "Research")),
		ui.SidebarItem("settings", "Settings").Icon(icons.Info).Tooltip("Settings"),
	).Name("Mail navigation").Width(180).Collapsible(ui.SidebarCollapseIcon).BindCollapsed(sidebarCollapsed).Rail(true).Shortcut("cmd+b")
	reset := func() {
		name.Set("")
		email.Set("")
		size.Set(16)
		ggui.Add(cleared, 1)
		confirm.Set(false)
	}

	iconDemo := iconPreview()
	build := func() ggui.Widget {
		t := uitheme.Use()
		// A swatch paints its colour over the page background, as a
		// control does: Input is translucent, a tint of whatever is
		// behind a field, so on its own it would show nothing.
		swatch := func(label string, c color.Color) ggui.Widget {
			return ggui.Column(
				colorChip(c, t.Bg, t.Border, t.Radius),
				ui.Caption(label),
			).Gap(2).Stretch()
		}
		entries := []ggui.Widget{
			preview("Theme presets", ggui.Column(
				ggui.Text("Choose surfaces, accent and geometry independently. The whole gallery updates."),
				ggui.Grid(2,
					ui.Field("Base color", ui.Select(baseColor).Options(uitheme.BaseColors()).Name("Theme base")),
					ui.Field("Accent color", ui.Select(accentColor).Options(uitheme.AccentColors()).Name("Theme accent").Format(func(v uitheme.AccentColor) string {
						if v == "" {
							return "base"
						}
						return string(v)
					})),
					ui.Field("Style", ui.Select(themeStyle).Options(uitheme.Styles()).Name("Theme style")),
					ui.Field("Radius", ui.Select(themeRadius).Options(uitheme.RadiusOptions()).Name("Theme radius").Format(radiusLabel)),
					ui.Field("Menu accent", ui.Select(menuAccent).Options(uitheme.MenuAccents()).Name("Menu accent").Format(func(v uitheme.MenuAccent) string {
						if v == uitheme.MenuAccentSubtle {
							return "subtle"
						}
						return string(v)
					})),
					ui.Field("Menu color", ui.Select(menuColor).Options(uitheme.MenuColors()).Name("Menu color").Format(func(v uitheme.MenuColor) string {
						if v == uitheme.MenuColorDefault {
							return "default"
						}
						return string(v)
					})),
					ui.Field("Chart color", ui.Select(chartColor).Options(uitheme.ChartColors()).Name("Chart color").Format(func(v uitheme.AccentColor) string {
						if v == uitheme.AccentBase {
							return "follow accent"
						}
						return string(v)
					})),
					ui.Field("Icon library", ui.Select(iconLibrary).Options(uitheme.IconLibraries()).Name("Theme icons").Format(func(v uitheme.IconLibrary) string {
						if v == uitheme.IconsDefault {
							return "default"
						}
						return string(v)
					})),
				).Gap(12),
				ui.Field("Mode", ui.ThemeMode(mode)),
				ggui.Grid(4,
					swatch("Bg", t.Bg),
					swatch("Primary", t.Primary),
					swatch("Secondary", t.Secondary),
					swatch("Accent", t.Accent),
					swatch("Card", t.Card),
					swatch("Border", t.Border),
					swatch("Input", t.Input),
					swatch("Ring", t.Ring),
					swatch("Input border", t.InputBorder),
					swatch("Muted", t.Muted),
					swatch("Destructive", t.Destructive),
					swatch("Popover", t.Popover),
				).Gap(8),
				ggui.Row(
					ui.Button("Primary action", func() { themeAction.Set("Primary action selected.") }),
					ui.Button("Secondary action", func() { themeAction.Set("Secondary action selected.") }).Secondary(),
				).Gap(8),
				ui.Bubble(ggui.Text("Theme tokens also shape chat bubbles 👍")).End(),
				ui.CaptionOf(themeAction),
			).Gap(12).Stretch()),
			preview("Icons", iconDemo),

			preview("Emoji", ggui.Column(
				ggui.Text("Faces 😀 😭 🫩   Reactions 👍 🔥 👀").Size(22),
				ggui.Text("Skin tones 👋🏻 👋🏽 👋🏿   Flags 🇰🇷 🇺🇸").Size(20),
				ggui.Text("Families 👨‍👩‍👧‍👦   Work 👩🏽‍💻   Keycaps 1️⃣ #️⃣").Size(20),
				ui.TextField(emojiText).Name("Emoji text").Multiline().Lines(2),
				ggui.TextOf(emojiText).Size(24),
				ui.Button("Insert emoji sequence", func() { emojiText.Set(ggui.Peek(emojiText) + " 🏳️‍🌈 👨‍👩‍👧‍👦") }).Outline(),
				ui.Caption("Type, paste, select and delete: composed emoji stay together."),
			).Gap(16).Stretch()),
			preview("Buttons", ggui.Column(
				ggui.Wrap(
					ui.Button("Save changes", func() { toasts.Push(ui.Toast("Saved", "Your changes are stored.")) }),
					ui.Button("Outline", func() { toasts.Push(ui.Toast("Outline action", "Outline buttons support quieter actions.")) }).Outline(),
					ui.Button("Disabled", nil).Disabled(true),
				).Gap(8),
				ggui.Wrap(ui.Button("Secondary", nil).Secondary(), ui.Button("Ghost", nil).Ghost(), ui.Button("Delete", func() {
					toasts.Push(ui.Toast("Destructive action", "A red button makes the intent clear.").Destructive())
				}).Destructive()).Gap(8),
				ggui.Wrap(
					ui.Button("Extra small", nil).Size(ui.ButtonExtraSmall).Outline(),
					ui.Button("Small", nil).Size(ui.ButtonSmall).Outline(),
					ui.Button("Default", nil).Outline(),
					ui.Button("Large", nil).Size(ui.ButtonLarge).Outline(),
				).Gap(8),
				ggui.Wrap(
					ui.ButtonOf(ui.Icon(icons.Plus), nil).Name("Extra small icon").Size(ui.ButtonIconExtraSmall).Outline(),
					ui.ButtonOf(ui.Icon(icons.Plus), nil).Name("Small icon").Size(ui.ButtonIconSmall).Outline(),
					ui.ButtonOf(ui.Icon(icons.Plus), nil).Name("Default icon").Size(ui.ButtonIcon).Outline(),
					ui.ButtonOf(ui.Icon(icons.Plus), nil).Name("Large icon").Size(ui.ButtonIconLarge).Outline(),
					ui.Button("Link", nil).Link(),
				).Gap(8),
				ggui.Wrap(
					ui.Button("New branch", nil).Icon(ui.Icon(icons.Plus)).Outline(),
					ui.Button("Download", nil).Icon(ui.Icon(icons.Download)),
					ui.Button("Next", nil).TrailingIcon(ui.Icon(icons.ChevronRight)).Secondary(),
					ui.Button("Search", nil).Icon(ui.Icon(icons.Search)).Size(ui.ButtonSmall).Ghost(),
				).Gap(8),
				ggui.Wrap(
					ui.Badge("Draft"),
					ui.Badge("Published").Primary(),
					ui.Badge("Verified").Icon(icons.Check),
					ui.Badge("Ghost").Ghost(),
					ui.Badge("Link").Link(),
					ui.Badge("Failed").Destructive().Icon(icons.Alert),
				).Gap(8),
				ggui.Wrap(
					ui.Tooltip(ui.Button("Hover for help", nil).Outline(), "Tooltips add context to an action."),
					ui.Tooltip(ui.Button("Save", nil).Outline().Shortcut("cmd+s"), "Save the draft"),
				).Gap(8),
			).Gap(16)),

			preview("Text", ggui.Column(
				ui.TextField(name).Placeholder("Type here, IME works"),
				ui.TextField(notes).Placeholder("Notes: Enter breaks the line, ⌘+Enter submits").Lines(3),
				ggui.Reactive(func() ggui.Widget {
					return ggui.Text(fmt.Sprintf("Hello, %s", or(name.Get(), "stranger"))).Size(size.Get())
				}),
				ggui.Row(
					ggui.Text("Size").Color(t.MutedFg),
					ggui.Expanded(ui.Slider(size, 10, 40).Step(1)),
					ggui.Textf("%2.0f", size).NoWrap(),
				).Space(1),
			).Space(1)),

			preview("Field", ggui.Column(
				ui.Field("Email", ui.TextField(email).Placeholder("you@example.com")).
					Help("Help text sits here until there is an error").BindError(emailError),
				ui.Field("Text size", ui.Slider(size, 10, 40).Step(1)).Help("A Field labels any control"),
			).Space(1).Stretch()),

			preview("Choices", ggui.Column(
				ui.Checkbox(notify, "Send notifications"),
				ui.Radios(plan).Options([]string{"free", "pro", "team"}),
				ui.Radios(plan).Options([]string{"free", "pro", "team"}).Horizontal().Name("Plan, in a row"),
				ggui.Wrap(
					ui.Select(fruit).Options([]string{"Apple", "Banana", "Cherry", "Durian"}).Name("Choose fruit"),
					ui.Menu("Actions",
						ui.MenuLabel("Text"),
						ui.MenuItem("Reset text size", func() { size.Set(16) }).Icon(ui.Icon(icons.Undo)),
						ui.MenuItem("Clear name", func() { name.Set("") }).Icon(ui.Icon(icons.Close)),
						ui.MenuDivider(),
						ui.MenuCheckbox("Dark theme", dark),
					),
					ui.Captionf("picked %s", fruit),
				).Space(1),
			).Space(1)),

			preview("Shadows", ggui.Padding(ggui.Wrap(
				ui.Card(ggui.Text("Subtle")).Shadow(ggui.ShadowStyle{Offset: ggui.Pt(0, 2), Blur: 4, Color: color.NRGBA{A: 40}}),
				ui.Card(ggui.Text("Floating")).Shadow(ggui.ShadowStyle{Offset: ggui.Pt(0, 6), Blur: 12, Color: color.NRGBA{A: 55}}),
				ui.Card(ggui.Text("Colored")).Shadow(ggui.ShadowStyle{Offset: ggui.Pt(0, 4), Blur: 16, Spread: 2, Color: color.NRGBA{R: 70, G: 100, B: 230, A: 85}}),
			).Gap(24), 20)),
			preview("Effects", effectsDemo()),
			preview("Menubar", menubar),
			preview("Calendar", calendar),
			preview("Date picker", ggui.Column(
				ggui.Wrap(datePicker, rangePicker).Gap(8),
				ui.Caption("The second picker takes a range: the first night, then the last, across two months."),
			).Gap(12)),
			preview("Context menu", ui.ContextMenu(
				ggui.Box(ggui.Column(ui.Title("Project notes"), ui.Caption("Right-click here, or use Tab then Shift+F10.")).Space(1)).Pad(24).Border(1, t.Border).Radius(t.Radius),
				ui.MenuLabel("Notes"),
				ui.MenuItem("Open notes", func() {
					toasts.Push(ui.Toast("Notes opened", "Context-menu actions work with keyboard and pointer input."))
				}).Icon(ui.Icon(icons.File)).Shortcut("cmd+o"),
				ui.MenuItem("Share notes", nil).Disabled(true).Inset(),
				ui.MenuSub("Export as",
					ui.MenuItem("Markdown", func() { toasts.Push(ui.Toast("Exported", "Saved as Markdown.").Success()) }),
					ui.MenuItem("PDF", func() { toasts.Push(ui.Toast("Exported", "Saved as PDF.").Success()) }),
				).Icon(ui.Icon(icons.Download)),
				ui.MenuDivider(),
				ui.MenuItem("Delete notes", func() { toasts.Push(ui.Toast("Notes deleted", "Your project notes have been deleted.").Error()) }).Destructive().Shortcut("cmd+backspace"),
			).Name("Project notes actions")),

			preview("Notices and empty states", ggui.Column(
				ui.Alert("Changes saved", "Your settings are up to date.").Icon(icons.Check),
				ui.Alert("Connection interrupted", "You can retry without losing your work.").Destructive().Icon(icons.Alert).
					Action(ui.Button("Retry", func() { progress.Set(0) }).Size(ui.ButtonExtraSmall).Outline()),
				ui.Empty("No attachments", "Add a file to get started.").
					MediaIcon(icons.File).Action(ui.Button("Add sample", func() { progress.Set(1) })).Outline(),
			).Space(1).Stretch()),

			preview("Loading and shortcuts", ggui.Column(
				ggui.Row(ui.Spinner(), ggui.Text("Loading…"), ui.KbdGroup(ui.Kbd("Ctrl"), ui.Kbd("K"))).Space(1),
				ggui.Row(ui.Spinner().Color(t.Primary), ggui.Text("Press"), ui.KbdGroup(ui.Kbd("⌘"), ggui.Text("+"), ui.Kbd("B")), ggui.Text("to fold the panel")).Space(1),
				ggui.Row(ui.Skeleton(40, 40).Circle(), ggui.Column(ui.Skeleton(180, 14), ui.Skeleton(120, 14)).Space(1)).Space(1),
			).Space(1)),

			preview("Progress", ggui.Column(
				ui.Progress(download).Label("Download").ShowValue(),
				ui.Progress(download).Label("Files").FormatValue(func(v float64) string { return fmt.Sprintf("%.0f of 12", v*12) }),
				ggui.Wrap(
					ui.Button("Advance download", func() { download.Set(min(1, ggui.Peek(download)+0.25)) }).BindDisabled(download.Map(func(v float64) bool { return v >= 1 })),
					ui.Button("Restart download", func() { download.Set(0) }).Outline(),
				).Gap(8),
			).Space(1).Stretch()),

			preview("Collapsible", ggui.Column(
				ui.Collapsible(disclosureOpen, "Delivery preferences", ggui.Column(
					ui.Checkbox(notify, "Email delivery updates"),
					ui.Caption("Your selection is preserved when this section is closed."),
				).Space(1)),
				ui.Collapsible(ggui.State(false), "Unavailable preferences", ggui.Text("Not available")).Disabled(true),
			).Space(1).Stretch()),

			preview("Pagination", ggui.Column(
				ui.Pagination(page, pageCount),
				ui.Captionf("Page %d — bind this value to your data query or slice.", page),
			).Space(1)),

			preview("Accordion", ui.Accordion(accordionOpen,
				ui.AccordionItem("intro", "How does it work?", ggui.Text("Use the arrow keys to choose a header, then Enter to expand it.")),
				ui.AccordionItem("keys", "Keyboard controls", ggui.Row(ui.Kbd("Up / Down"), ui.Kbd("Home / End"), ui.Kbd("Enter")).Space(1)),
				ui.AccordionItem("disabled", "Unavailable section", ggui.Text("Hidden")).Disabled(true),
			)),

			preview("Search and commands", ggui.Wrap(
				ui.Combobox(fruit).Options([]string{"Apple", "Banana", "Cherry", "Durian", "Grape", "Mango", "Orange"}).Name("Search fruit"),
				ui.Button("Commands…", func() { paletteOpen.Set(true) }).Outline(),
			).Space(1)),
			ui.CommandDialog(paletteOpen, ui.Command(commandQuery,
				ui.CommandItem("Toggle dark theme", func() {
					dark.Set(!ggui.Peek(dark))
					paletteOpen.Set(false)
				}).Keywords("appearance", "light").Group("Appearance"),
				ui.CommandItem("Reset text size", func() {
					size.Set(16)
					paletteOpen.Set(false)
				}).Keywords("font").Group("Appearance"),
				ui.CommandItem("Show notification", func() {
					toasts.Push(ui.Toast("Done", "The command ran successfully."))
					paletteOpen.Set(false)
				}).Group("Actions"),
			).Height(176).StableHeight().Hints()),

			preview("Resizable", ggui.Box(ui.Resizable(split,
				ggui.Center(ggui.Text("Sidebar")),
				ui.Resizable(verticalSplit, ggui.Center(ggui.Text("Header")), ggui.Center(ggui.Text("Content"))).Vertical().WithHandle().MinSizes(48, 48),
			).WithHandle().MinSizes(100, 140)).Height(200).Border(1, t.Border).Radius(t.Radius)),

			preview("Toast", ggui.Column(ggui.Wrap(
				ui.Button("Notify", func() { toasts.Push(ui.Toast("Saved", "Your changes are stored.")) }).Outline(),
				ui.Button("Success", func() { toasts.Push(ui.Toast("Published", "The page is live.").Success()) }).Outline(),
				ui.Button("Info", func() { toasts.Push(ui.Toast("Heads up", "A new version is available.").Info()) }).Outline(),
				ui.Button("Warning", func() { toasts.Push(ui.Toast("Storage almost full", "Free some space soon.").Warning()) }).Outline(),
				ui.Button("Loading", func() {
					id := toasts.Push(ui.Toast("Uploading", "Sending the file…").Loading())
					ggui.Async(func() (bool, error) { time.Sleep(1500 * time.Millisecond); return true, nil }, func(bool, error) {
						toasts.Update(id, ui.Toast("Uploaded", "The file is stored.").Success())
					})
				}).Outline(),
				ui.Button("Countdown", func() { toasts.Push(ui.Toast("Link copied", "The bar shows the time left.").Countdown()) }).Outline(),
				ui.Button("Notify with action", func() {
					toasts.Push(ui.Toast("Item removed", "You can undo this change.").Action("Undo", func() { toasts.Push(ui.Toast("Restored", "The item is back.")) }))
				}).Outline(),
				ui.Button("Persistent error", func() {
					toasts.Push(ui.Toast("Upload failed", "Dismiss this message when you are ready.").Destructive().Duration(0))
				}).Outline(),
			).Space(1),
				ui.Field("Position", ui.Select(toastPosition).Options([]ui.ToastPosition{
					ui.ToastTopLeft, ui.ToastTopCenter, ui.ToastTopRight, ui.ToastBottomLeft, ui.ToastBottomCenter, ui.ToastBottomRight,
				}).Format(toastPositionLabel).Name("Notice position")),
			).Space(1).Stretch()),
			toasts,

			preview("Tabs", ui.Tabs(tab,
				ui.Tab("Overview", ggui.Column(
					ggui.Row(ggui.Text("Status"), ui.Badge("stable"), ui.Badge("new").Primary()).Space(1),
					ui.Progress(progress), // reads the signal every frame: no Reactive needed
					ggui.Row(
						ui.Button("+10%", func() { progress.Set(min(ggui.Peek(progress)+0.1, 1)) }).Outline(),
						ui.Button("Reset", func() { progress.Set(0) }).Outline(),
					).Space(1),
				).Space(1).Stretch()),
				ui.Tab("Details", ggui.Column(
					ui.Collapsible(more, "More options", ggui.Column(
						ui.Checkbox(notify, "Send notifications"),
						ggui.Text("Folded content fades and slides.").Color(t.MutedFg),
					).Space(1)),
				)),
				ui.Tab("About", ggui.Text("Tabs lay out only the page they show; Left and Right switch while focused.")),
			)),

			preview("Transition", ggui.Column(
				ui.Switch(details, "Show details"),
				ggui.Presence(details, ggui.Transition(
					ggui.Box(ggui.Text("Slides and fades in, and back out when hidden. A leaving widget takes no input.")).
						Fill(t.Card).Radius(t.Radius).Pad(t.Space),
				).Slide(0, -8).Fade()),
			).Space(1).Stretch()),

			preview("Reveal", ggui.Column(
				ui.Switch(revealed, "Show more"),
				ggui.Reveal(revealed, ggui.Box(ggui.Text("Folds open beneath the switch, moving what follows down, and back up when hidden.")).
					Fill(t.Card).Radius(t.Radius).Pad(t.Space)),
				ui.Caption("Below the reveal."),
			).Space(1).Stretch()),

			preview("Dialog", ggui.Column(
				ggui.Row(
					ui.Button("Reset form…", func() { confirm.Set(true) }).Outline(),
					ui.Captionf("reset %d times", cleared),
				).Space(1),
				// Dialog takes no space where it sits; it paints over the
				// window while confirm is true. Escape, the scrim or Cancel
				// closes it and focus returns to the button.
				ui.Dialog(confirm, ggui.Text("The name, the email and the text size go back to their defaults.")).
					Title("Reset the form?").Description("Nothing else on the page changes.").
					Footer(ui.Button("Cancel", func() { confirm.Set(false) }).Outline(), ui.Button("Reset", reset)),
				ggui.Row(
					ui.Button("Remove row…", func() { removeOpen.Set(true) }).Destructive(),
					ui.Captionf("removed %d times", removed),
				).Space(1),
				removeDialog,
			).Space(1)),

			preview("Sheets and drawers", ggui.Column(
				ggui.Wrap(
					ui.Button("Open filters", func() { filtersOpen.Set(true) }).Outline(),
					ui.Button("Open drawer", func() { drawerOpen.Set(true) }).Outline(),
				).Space(1),
				ui.Caption("A scrim takes the clicks, Tab stays inside, and Escape hands focus back."),
			).Space(1)),
			filtersSheet, shareDrawer,

			preview("Sidebar", ggui.Column(
				ui.Breadcrumb(
					ui.Crumb("Home", func() { nav.Set("inbox") }),
					ui.Crumb("Mail", func() { nav.Set("inbox") }),
					ui.Crumb("Message", nil),
				),
				roundFrame(ggui.Box(ggui.Row(navBar, ggui.Expanded(ggui.Padding(ggui.Column(
					ggui.Row(ui.SidebarTrigger(sidebarCollapsed), ggui.Expanded(ggui.TextOf(nav).NoWrap().Ellipsis())).Gap(8),
					ui.Divider(),
					ggui.Expanded(ggui.Center(ui.Caption("The panel button, the rail or ⌘B folds the sidebar to its icons."))),
				).Gap(8).Stretch(), 8)))).Height(320), t.RadiusLg, t.Border),
			).Space(1).Stretch()),

			preview("Groups and addons", ggui.Column(
				ui.Switch(controlsLocked, "Lock alignment"),
				ui.ToggleGroup(align).Options([]string{"left", "center", "right"}).Name("Text alignment").BindDisabled(controlsLocked),
				ggui.Textf("Alignment: %s", align),
				ui.ToggleGroupMultiple(textStyle).Options([]string{"Bold", "Italic", "Underline"}).Name("Text style").Outline().Spacing(0),
				ui.ButtonGroup(
					ui.Button("Copy", func() { groupAction.Set("Copy selected") }).Outline(),
					ui.Button("Cut", func() { groupAction.Set("Cut selected") }).Outline(),
					ui.Button("Paste", func() { groupAction.Set("Paste selected") }).Outline(),
				),
				ui.CaptionOf(groupAction),
				ggui.Wrap(
					ui.ButtonGroup(
						ui.ButtonGroupText("Zoom"),
						ui.ButtonOf(ui.Icon(icons.Minus), func() { zoom.Set(max(25, ggui.Peek(zoom)-25)) }).Name("Zoom out").Size(ui.ButtonIcon).Outline(),
						ui.ButtonGroupTextOf(ggui.Textf("%d%%", zoom)),
						ui.ButtonOf(ui.Icon(icons.Plus), func() { zoom.Set(min(400, ggui.Peek(zoom)+25)) }).Name("Zoom in").Size(ui.ButtonIcon).Outline(),
					),
					ui.ButtonGroup(
						ui.Button("Undo", func() { groupAction.Set("Undo selected") }).Icon(ui.Icon(icons.Undo)).Secondary(),
						ui.ButtonGroupSeparator(),
						ui.Button("Redo", func() { groupAction.Set("Redo selected") }).Icon(ui.Icon(icons.Redo)).Secondary(),
					),
				).Gap(8),
				ui.ButtonGroup(
					ui.TextField(searchQuery).Placeholder("Search the docs…").Name("Search query"),
					ui.ButtonOf(ui.Icon(icons.Search), func() { groupAction.Set("Searched for " + ggui.Peek(searchQuery)) }).Name("Run search").Size(ui.ButtonIcon).Outline(),
				),
				ui.InputGroup(ggui.TextInput(site).Placeholder("example.com").Name("Site")).
					Leading(ggui.Text("https://").Color(t.MutedFg)).
					Trailing(ui.Button("Go", func() {
						if host := strings.TrimSpace(ggui.Peek(site)); host != "" {
							sitePreview.Set("Preview: https://" + host)
						} else {
							sitePreview.Set("Enter a site first")
						}
					}).Ghost()),
				ui.CaptionOf(sitePreview),
			).Space(1).Stretch()),

			preview("Profile and media", ggui.Column(
				ggui.Wrap(
					ui.Avatar("Ada Lovelace").Small(),
					ui.Avatar("Ada Lovelace"),
					ui.Avatar("Ada Lovelace").Large().Badge(color.NRGBA{R: 34, G: 197, B: 94, A: 255}),
					ui.Avatar("Grace Hopper").Size(48).Square(),
					ui.Avatar("").Name("Unknown person").BadgeIcon(icons.Plus),
					ui.AvatarGroup(ui.Avatar("Ada Lovelace"), ui.Avatar("Grace Hopper"), ui.Avatar("Alan Turing"), ui.Avatar("Edsger Dijkstra")).Max(3).Count(5),
				).Gap(12),
				ui.Caption("Small, default and large avatars with status badges, a square portrait, a fallback and a group that counts the rest."),
				ui.ItemGroup(
					ui.Item("Ada Lovelace", "Analytical engine").
						Media(ui.Avatar("Ada Lovelace")).
						Action(ui.HoverCard(
							ui.Button("Details", nil).Outline(),
							ggui.Column(ui.Title("Ada Lovelace").Size(16), ui.Caption("Rest the cursor to preview.")).Gap(4),
						).Side(ggui.PopupLeft)),
					ui.ItemSeparator(),
					ui.Item("Quarterly report", "Shared with you yesterday").MediaIcon(icons.File).
						Action(ui.ButtonOf(ui.Icon(icons.Download), nil).Name("Download report").Size(ui.ButtonIconSmall).Ghost()),
				),
				ui.Item("Two-factor sign-in is on", "").MediaIcon(icons.Check).Small().Muted(),
				ui.AspectRatio(16.0/9, ggui.Box(ggui.Center(ui.Caption("16 : 9"))).Fill(t.Muted).Radius(t.Radius)),
			).Space(1).Stretch()),

			preview("Wrap", ggui.View(tags, func(list []string) *ggui.WrapWidget {
				return ggui.Wrap(ggui.Children(list, func(tag string) ggui.Widget {
					return ui.Tooltip(
						ui.Button(tag+"  ×", func() { ggui.Remove(tags, func(s string) bool { return s == tag }) }).Outline().Pad(4, 10),
						"Click to remove",
					)
				})...).Space(0.5)
			})),

			preview("Data Table", paymentTable),

			preview("Table", ggui.Column(
				ui.Table(people, func(p Person) int { return p.ID },
					ui.TextCol("Name", func(p Person) string { return p.Name }),
					ui.TextCol("Role", func(p Person) string { return p.Role }).Grow(2),
					ui.TextCol("Age", func(p Person) string { return strconv.Itoa(p.Age) }).W(48).Right(),
					ui.Col("", func(r ggui.Readable[Person]) ggui.Widget {
						id := ggui.Peek(r).ID
						return ui.Button("×", func() { ggui.Remove(people, func(p Person) bool { return p.ID == id }) }).Outline().Pad(0, 8)
					}).W(48),
				).BindSelected(chosen).RowName(func(p Person) string { return p.Name }).Height(260).
					Footer(ggui.Text("Average age"), nil, ggui.TextOf(people.Map(func(ps []Person) string {
						total := 0
						for _, p := range ps {
							total += p.Age
						}
						return strconv.Itoa(total / max(len(ps), 1))
					}))).
					Caption("The team and its average age."),
				ui.Captionf("selected: %s. Click a row, or Tab to it and press Space; the body scrolls under the heading.", chosenName),
			).Space(1).Stretch()),

			preview("Grid", ggui.Cached(ggui.Grid(4,
				swatch("Bg", t.Bg), swatch("Card", t.Card), swatch("Input", t.Input), swatch("Border", t.Border),
				swatch("Accent", t.Primary), swatch("AccentHover", t.PrimaryHover), swatch("Muted", t.MutedFg), swatch("Fg", t.Fg),
			).Space(1))),
		}
		entries = append(entries, chatPreviews()...)
		entries = append(entries, parityPreviews()...)
		entries = append(entries, partPreviews()...)
		entries = append(entries, reactivityPreview())
		entries = append(entries, preview("Carousel", ui.Carousel(carouselIndex, ui.Card(ggui.Center(ui.Title("1"))), ui.Card(ggui.Center(ui.Title("2"))), ui.Card(ggui.Center(ui.Title("3")))).Height(180)))
		entries = append(entries, preview("Rich text", ggui.Column(
			ggui.RichText(
				ggui.Span("Spans wrap as one paragraph, in "),
				ggui.Span("bold").Weight(ggui.WeightBold),
				ggui.Span(", "),
				ggui.Span("larger").Size(20),
				ggui.Span(" or "),
				ggui.Span("linked").Color(t.Primary).Underline().OnTap(func() { toasts.Push(ui.Toast("Link", "The span was tapped.")) }),
				ggui.Span(" text. Drag across it to select, then copy."),
			).Selectable(),
			ggui.Directionality(ggui.DirRTL, ggui.Text("مرحبا بالعالم · שלום עולם · ggui").Selectable()),
		).Space(1).Stretch()))
		entries = append(entries, preview("Input OTP", ui.Field("Verification code", ui.InputOTP(otpValue, 6).Groups(3, 3)).Help("Paste a code or edit individual slots.")))
		entries = append(entries, preview("Charts", ggui.Column(
			chartPreview(),
			ui.Caption("Area, bar, line, pie, radar and radial charts. Run examples/charts for all variants."),
		).Gap(12)))
		if present != nil {
			return present(entries)
		}
		return galleryPage(dark, search, category, scroll, func() { paletteOpen.Set(true) }, entries)

	}

	setup := func() {
		setupEmojiFont()
		ggui.Effect(func() ggui.Cleanup {
			recipe := uitheme.Config{
				Preset:     uitheme.Preset{Base: baseColor.Get(), Accent: accentColor.Get(), Style: themeStyle.Get()},
				Radius:     themeRadius.Get(),
				MenuAccent: menuAccent.Get(),
				MenuColor:  menuColor.Get(),
				Chart:      chartColor.Get(),
				Icons:      iconLibrary.Get(),
			}
			t := recipe.For(dark.Get())
			t.Text.Font = chatFont()
			uitheme.Set(t)
			return nil
		})
		ggui.Effect(func() ggui.Cleanup {
			toasts.Position(toastPosition.Get())
			return nil
		})
		ggui.OnCleanup(toasts.Close)
	}
	return func() ggui.Widget { return ggui.Reactive(build) }, setup, func() { paletteOpen.Set(true) }
}

func run() error {
	var app *ggui.App
	dispose := ggui.Root(func() {
		build, setup, commands := newGallery()
		app = ggui.New(ggui.Config{
			Title:     "ggui · gallery",
			Width:     1180,
			Height:    820,
			Resizable: true,
			Inspector: "f1",
		}, build)
		app.Setup(setup)
		app.Shortcut("cmd+k", commands)
	})
	defer dispose()
	defer app.Close()
	return app.Run()
}

func main() {
	renderDir := flag.String("render-dir", "", "Render existing component audit to a directory")
	renderExamples := flag.String("render-examples", "", "Comma-separated gallery examples to render (default: all existing components)")
	flag.Parse()
	if *renderDir != "" {
		if err := renderGalleryAudit(*renderDir, *renderExamples); err != nil {
			log.Fatal(err)
		}
		return
	}
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

// radiusLabel names a radius choice with its --radius.
func radiusLabel(r uitheme.Radius) string {
	switch r {
	case uitheme.RadiusDefault:
		return "default (style)"
	case uitheme.RadiusNone:
		return "none (0)"
	case uitheme.RadiusSmall:
		return "small (7.2px)"
	case uitheme.RadiusMedium:
		return "medium (10px)"
	}
	return "large (14px)"
}

// toastPositionLabel names where a Toaster stacks its notices.
func toastPositionLabel(p ui.ToastPosition) string {
	return map[ui.ToastPosition]string{
		ui.ToastTopLeft: "top left", ui.ToastTopCenter: "top center", ui.ToastTopRight: "top right",
		ui.ToastBottomLeft: "bottom left", ui.ToastBottomCenter: "bottom center", ui.ToastBottomRight: "bottom right",
	}[p]
}

func or(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
