// Command gallery shows the layout widgets and the ui controls together:
// Wrap and Grid, every control bound to a signal, a labelled Field with
// validation, a modal Dialog, tooltips, and the inspector on F1.
package main

import (
	"fmt"
	"image/color"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

// Person is a row of the table below. Rows are keyed by ID, so editing or
// removing one keeps every other row's widgets.
type Person struct {
	ID   int
	Name string
	Role string
	Age  int
}

func newGallery() (ggui.Builder, func(), func()) {
	dark := ggui.State(false)
	selectedDate := ggui.State(time.Now())
	calendar := ui.Calendar(selectedDate).WeekStartsOn(time.Monday)
	datePicker := ui.DatePicker(selectedDate).Named("Appointment date")
	datePicker.Calendar().WeekStartsOn(time.Monday)
	search := ggui.State("")
	category := ggui.State("All")
	scroll := ggui.State(0.0)
	verticalSplit := ggui.State(.45)
	people := ggui.State([]Person{
		{1, "Ada", "Engineer", 36}, {2, "Grace", "Admiral", 45}, {3, "Linus", "Kernel", 28},
		{4, "Ken", "Plan 9", 60}, {5, "Rob", "Gopher", 62}, {6, "Barbara", "Architect", 57},
	})
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
	email := ggui.State("")
	confirm := ggui.State(false)
	cleared := ggui.State(0)
	size := ggui.State(16.0)
	notes := ggui.State("")
	fruit := ggui.State("Banana")
	details := ggui.State(true)
	tab := ggui.State(0)
	more := ggui.State(false)
	progress := ggui.State(0.3)
	page, pageCount := ggui.State(1), ggui.State(12)
	accordionOpen := ggui.State([]string{"intro"})
	split := ggui.State(.35)
	commandQuery := ggui.State("")
	paletteOpen := ggui.State(false)
	toasts := ui.NewToaster()
	notify := ggui.State(true)
	plan := ggui.State("free")
	tags := ggui.State([]string{"go", "gui", "ebiten", "signals", "flutter", "svelte", "layout", "hidpi"})

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
		ui.Menu("View", ui.MenuItem("Switch theme", func() { ggui.Toggle(dark) })),
	).Compact()
	reset := func() {
		name.Set("")
		email.Set("")
		size.Set(16)
		ggui.Add(cleared, 1)
		confirm.Set(false)
	}

	build := func() ggui.Widget {
		t := ggui.UseTheme()
		section := func(title string, body ggui.Widget) ggui.Widget {
			return preview(title, body)
		}
		swatch := func(label string, c color.Color) ggui.Widget {
			return ggui.Column(
				ggui.Box().Height(28).Fill(c).Radius(t.Radius).Border(1, t.Border),
				ggui.Caption(label),
			).Gap(2).Align(ggui.AlignStretch)
		}
		entries := []ggui.Widget{
			section("Buttons", ggui.Column(
				ggui.Wrap(ui.Button("Save changes", func() { toasts.Push(ui.Toast("Saved", "Your changes are stored.")) }), ui.Button("Secondary", func() { toasts.Push(ui.Toast("Secondary action", "Outline buttons support quieter actions.")) }).Secondary(), ui.Button("Disabled", nil).Disabled(true)).Gap(8),
				ggui.Wrap(ui.Button("Muted", nil).Muted(), ui.Button("Ghost", nil).Ghost(), ui.Button("Delete", func() {
					toasts.Push(ui.Toast("Destructive action", "A red button makes the intent clear.").Destructive())
				}).Destructive()).Gap(8),
				ggui.Wrap(ui.Badge("Draft"), ui.Badge("Published").Accent(), ggui.Tooltip(ui.Button("Hover for help", nil).Secondary(), "Tooltips add context to an action.")).Gap(8),
			).Gap(16)),

			section("Text", ggui.Column(
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

			section("Field", ggui.Column(
				ui.Field("Email", ui.TextField(email).Placeholder("you@example.com")).
					Help("Help text sits here until there is an error").Error(emailError),
				ui.Field("Text size", ui.Slider(size, 10, 40).Step(1)).Help("A Field labels any control"),
			).Space(1).Align(ggui.AlignStretch)),

			section("Choices", ggui.Column(
				ui.Checkbox(notify, "Send notifications"),
				ui.Radios(plan, []string{"free", "pro", "team"}),
				ggui.Wrap(
					ui.Select(fruit, []string{"Apple", "Banana", "Cherry", "Durian"}).Named("Choose fruit"),
					ui.Menu("Actions",
						ui.MenuItem("Reset text size", func() { size.Set(16) }),
						ui.MenuItem("Clear name", func() { name.Set("") }),
						ui.MenuDivider(),
						ui.MenuItem("Toggle dark", func() { ggui.Toggle(dark) }),
					),
					ggui.Textf("picked %s", fruit).AsCaption(),
				).Space(1),
			).Space(1)),

			section("Shadows", ggui.Padding(ggui.Wrap(
				ui.Card(ggui.Text("Subtle")).Shadow(ggui.ShadowStyle{Offset: ggui.Pt(0, 2), Blur: 4, Color: color.NRGBA{A: 40}}),
				ui.Card(ggui.Text("Floating")).Shadow(ggui.ShadowStyle{Offset: ggui.Pt(0, 6), Blur: 12, Color: color.NRGBA{A: 55}}),
				ui.Card(ggui.Text("Colored")).Shadow(ggui.ShadowStyle{Offset: ggui.Pt(0, 4), Blur: 16, Spread: 2, Color: color.NRGBA{R: 70, G: 100, B: 230, A: 85}}),
			).Gap(24), 20)),
			section("Menubar", menubar),
			section("Calendar", calendar),
			section("Date picker", datePicker),
			section("Context menu", ui.ContextMenu(
				ggui.Box(ggui.Column(ggui.Title("Project notes"), ggui.Caption("Right-click here, or use Tab then Shift+F10.")).Space(1)).Pad(24).Border(1, t.Border).Radius(t.Radius),
				ui.MenuItem("Open notes", func() {
					toasts.Push(ui.Toast("Notes opened", "Context-menu actions work with keyboard and pointer input."))
				}),
				ui.MenuItem("Share notes", nil).Disabled(true),
				ui.MenuDivider(),
				ui.MenuItem("Archive notes", func() { toasts.Push(ui.Toast("Notes archived", "Your project notes have been archived.")) }),
			).Named("Project notes actions")),

			section("Notices and empty states", ggui.Column(
				ui.Alert("Changes saved", "Your settings are up to date."),
				ui.Alert("Connection interrupted", "You can retry without losing your work.").Destructive().
					Action(ui.Button("Retry", func() { progress.Set(0) }).Secondary()),
				ui.Empty("No attachments", "Add a file to get started.").
					Media(ui.Badge("Files")).Action(ui.Button("Add sample", func() { progress.Set(1) })),
			).Space(1).Align(ggui.AlignStretch)),

			section("Loading and shortcuts", ggui.Column(
				ggui.Row(ui.Spinner(), ggui.Text("Loading…"), ui.Kbd("Ctrl"), ui.Kbd("K")).Space(1),
				ggui.Row(ui.Skeleton(40, 40).Circle(), ggui.Column(ui.Skeleton(180, 14), ui.Skeleton(120, 14)).Space(1)).Space(1),
			).Space(1)),

			section("Pagination", ggui.Column(
				ui.Pagination(page, pageCount),
				ggui.Textf("Page %d — bind this value to your data query or slice.", page).AsCaption(),
			).Space(1)),

			section("Accordion", ui.Accordion(accordionOpen,
				ui.AccordionItem("intro", "How does it work?", ggui.Text("Use the arrow keys to choose a header, then Enter to expand it.")),
				ui.AccordionItem("keys", "Keyboard controls", ggui.Row(ui.Kbd("Up / Down"), ui.Kbd("Home / End"), ui.Kbd("Enter")).Space(1)),
				ui.AccordionItem("disabled", "Unavailable section", ggui.Text("Hidden")).Disabled(true),
			)),

			section("Search and commands", ggui.Wrap(
				ui.Combobox(fruit, []string{"Apple", "Banana", "Cherry", "Durian", "Grape", "Mango", "Orange"}).Named("Search fruit"),
				ui.Button("Commands…", func() { paletteOpen.Set(true) }).Secondary(),
			).Space(1)),
			ui.CommandDialog(paletteOpen, ui.Command(commandQuery,
				ui.CommandItem("Toggle dark theme", func() { ggui.Toggle(dark); paletteOpen.Set(false) }).Keywords("appearance", "light").Group("Appearance"),
				ui.CommandItem("Reset text size", func() { size.Set(16); paletteOpen.Set(false) }).Keywords("font").Group("Appearance"),
				ui.CommandItem("Show notification", func() { toasts.Push(ui.Toast("Done", "The command ran successfully.")); paletteOpen.Set(false) }).Group("Actions"),
			).Height(176).StableHeight().Hints()),

			section("Resizable", ggui.Box(ui.Resizable(split,
				ggui.Center(ggui.Text("Sidebar")),
				ui.Resizable(verticalSplit, ggui.Center(ggui.Text("Header")), ggui.Center(ggui.Text("Content"))).Vertical().WithHandle().MinSizes(48, 48),
			).WithHandle().MinSizes(100, 140)).Height(200).Border(1, t.Border).Radius(t.Radius)),

			section("Toast", ggui.Wrap(
				ui.Button("Notify", func() { toasts.Push(ui.Toast("Saved", "Your changes are stored.")) }).Secondary(),
				ui.Button("Notify with action", func() {
					toasts.Push(ui.Toast("Item removed", "You can undo this change.").Action("Undo", func() { toasts.Push(ui.Toast("Restored", "The item is back.")) }))
				}).Secondary(),
				ui.Button("Persistent error", func() {
					toasts.Push(ui.Toast("Upload failed", "Dismiss this message when you are ready.").Destructive().Duration(0))
				}).Secondary(),
			).Space(1)),
			toasts,

			section("Tabs", ui.Tabs(tab,
				ui.Tab("Overview", ggui.Column(
					ggui.Row(ggui.Text("Status"), ui.Badge("stable"), ui.Badge("new").Accent()).Space(1),
					ui.Progress(progress), // reads the signal every frame: no Reactive needed
					ggui.Row(
						ui.Button("+10%", func() { progress.Set(min(progress.Peek()+0.1, 1)) }).Secondary(),
						ui.Button("Reset", func() { progress.Set(0) }).Secondary(),
					).Space(1),
				).Space(1).Align(ggui.AlignStretch)),
				ui.Tab("Details", ggui.Column(
					ui.Collapsible(more, "More options", ggui.Column(
						ui.Checkbox(notify, "Send notifications"),
						ggui.Text("Folded content fades and slides.").Color(t.MutedFg),
					).Space(1)),
				)),
				ui.Tab("About", ggui.Text("Tabs lay out only the page they show; Left and Right switch while focused.")),
			)),

			section("Transition", ggui.Column(
				ui.Switch(details, "Show details"),
				ggui.Presence(details, ggui.Transition(
					ggui.Box(ggui.Text("Slides and fades in, and back out when hidden. A leaving widget takes no input.")).
						Fill(t.Card).Radius(t.Radius).Pad(t.Space),
				).Slide(0, -8).Fade()),
			).Space(1).Align(ggui.AlignStretch)),

			section("Dialog", ggui.Column(
				ggui.Row(
					ui.Button("Reset form…", func() { confirm.Set(true) }).Secondary(),
					ggui.Textf("reset %d times", cleared).AsCaption(),
				).Space(1),
				// Dialog takes no space where it sits; it paints over the
				// window while confirm is true. Escape, the scrim or Cancel
				// closes it and focus returns to the button.
				ui.Dialog(confirm, ggui.Column(
					ggui.Text("Clear the name, the email and the text size?"),
					ggui.Row(
						ui.Button("Reset", reset),
						ui.Button("Cancel", func() { confirm.Set(false) }).Secondary(),
					).Space(1).Justify(ggui.JustifyEnd),
				).Space(1.5).Align(ggui.AlignStretch)).Title("Reset?"),
			).Space(1)),

			section("Wrap", ggui.View(tags, func(list []string) *ggui.WrapWidget {
				return ggui.Wrap(ggui.Children(list, func(tag string) ggui.Widget {
					return ggui.Tooltip(
						ui.Button(tag+"  ×", func() { ggui.Remove(tags, func(s string) bool { return s == tag }) }).Secondary().Pad(4, 10),
						"Click to remove",
					)
				})...).Space(0.5)
			})),

			section("Table", ggui.Column(
				ui.Table(people, func(p Person) int { return p.ID },
					ui.TextCol("Name", func(p Person) string { return p.Name }),
					ui.TextCol("Role", func(p Person) string { return p.Role }).Grow(2),
					ui.TextCol("Age", func(p Person) string { return strconv.Itoa(p.Age) }).W(48).Right(),
					ui.Col("", func(r ggui.Reader[Person]) ggui.Widget {
						id := r.Get().ID
						return ui.Button("×", func() { ggui.Remove(people, func(p Person) bool { return p.ID == id }) }).Secondary().Pad(0, 8)
					}).W(32),
				).Selected(chosen).Label(func(p Person) string { return p.Name }).Height(160),
				ggui.Textf("selected: %s. Click a row, or Tab to it and press Space; the body scrolls under the heading.", chosenName).AsCaption(),
			).Space(1).Align(ggui.AlignStretch)),

			section("Grid", ggui.Cached(ggui.Grid(4,
				swatch("Bg", t.Bg), swatch("Card", t.Card), swatch("Input", t.Input), swatch("Border", t.Border),
				swatch("Accent", t.Primary), swatch("AccentHover", t.PrimaryHover), swatch("Muted", t.MutedFg), swatch("Fg", t.Fg),
			).Space(1))),
		}
		return galleryPage(dark, search, category, scroll, func() { paletteOpen.Set(true) }, entries)

	}

	setup := func() {
		ggui.BindTheme(dark, ggui.DarkTheme(), ggui.DefaultTheme())
		ggui.OnCleanup(toasts.Close)
	}
	return build, setup, func() { paletteOpen.Set(true) }
}

func main() {
	build, setup, commands := newGallery()
	app := ggui.New(ggui.Config{Title: "ggui · gallery", Width: 1180, Height: 820, Resizable: true, Inspector: ebiten.KeyF1}, build)
	app.Setup(setup)
	app.Shortcut("cmd+k", commands)
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

func or(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
