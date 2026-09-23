package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/examples/internal/offscreen"
	"github.com/ironpark/ggui/ui"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// This catalog deliberately excludes the recently added charts, Carousel and OTP,
// and framework-only demonstrations. It exercises the actual gallery widgets.
var auditNames = []string{
	"Buttons", "Text", "Field", "Choices", "Menubar", "Calendar", "Date picker",
	"Context menu", "Search and commands", "Notices and empty states", "Loading and shortcuts",
	"Toast", "Dialog", "Progress", "Collapsible", "Pagination", "Accordion", "Tabs", "Resizable",
	"Data Table", "Table", "Sheets and drawers", "Sidebar", "Groups and addons", "Profile and media",
	"Attachment", "Bubble", "Message", "Marker", "Message Scroller", "Questionnaire", "Control states",
}

// actionNames is the control each example activates for its alternate
// state, and hoverNames the one the pointer rests on.
var actionNames = map[string]string{"Data Table": "Select row p2", "Table": "Grace", "Accordion": "Keyboard controls", "Collapsible": "Delivery preferences", "Dialog": "Reset form…", "Sheets and drawers": "Open filters", "Date picker": "Appointment date", "Menubar": "File", "Choices": "Choose fruit", "Toast": "Notify", "Progress": "Advance download", "Tabs": "Details", "Search and commands": "Commands…", "Groups and addons": "Lock alignment", "Control states": "Enabled switch"}

var hoverNames = map[string]string{"Buttons": "Hover for help", "Profile and media": "Details"}

type galleryAudit struct {
	dir   string
	names []string
	index int
}

// step audits one example in one theme, style and width.
func (g *galleryAudit) step() (bool, error) {
	name := g.names[g.index%len(g.names)]
	dark := g.index/len(g.names)%2 == 1
	width := 640
	if g.index/(len(g.names)*2)%2 == 1 {
		width = 320
	}
	now := time.Unix(100, 0)
	restore := ggui.SetClock(func() time.Time { return now })
	defer restore()
	style := uitheme.StyleNova
	if g.index/(len(g.names)*4) == 1 {
		style = uitheme.StyleRhea
	}
	preset := uitheme.Preset{Base: uitheme.BaseNeutral, Accent: uitheme.AccentBlue, Style: style}
	theme := preset.Light()
	mode := "light"
	if dark {
		theme = preset.Dark()
		mode = "dark"
	}
	uitheme.Set(theme)
	app := ggui.ProbeBuilder(func() ggui.Widget {
		build, _, _ := newGalleryPreview(func(entries []ggui.Widget) ggui.Widget {
			var widgets []ggui.Widget
			if name == "Control states" {
				widgets = append(widgets, auditControlStates())
			}
			for _, entry := range entries {
				if p, ok := entry.(*componentPreview); ok {
					if p.title == name {
						widgets = append(widgets, p)
					}
				} else {
					widgets = append(widgets, entry)
				}
			}
			return ggui.Stack(widgets...)
		})
		return build()
	}, ggui.Sz(width, 1200))
	defer app.Close()
	img := ggfx.NewImage(width, 1200)
	defer img.Deallocate()
	draw := func() {
		img.Fill(theme.Bg)
		app.Draw(img)
	}
	for frame := 0; frame < 3; frame++ {
		draw()
		now = now.Add(time.Second)
	}
	stem := strings.ToLower(strings.ReplaceAll(name, " ", "-"))
	// The first failure is kept; the audit goes on to the end of the step.
	var err error
	save := func(state string) {
		if e := offscreen.SavePNG(filepath.Join(g.dir, fmt.Sprintf("%s-%s-%s-%d-%s.png", stem, style, mode, width, state)), img); err == nil {
			err = e
		}
	}
	save("initial")
	// Activate a real accessible control where the example has a meaningful
	// alternate state. Capture both the transition and its settled appearance.
	if target, ok := actionNames[name]; ok {
		node, found := app.Semantics().Find("", target)
		if !found {
			return false, fmt.Errorf("%s: missing action %q", name, target)
		}
		action := ggui.ActionPress
		if node.Actions&ggui.ActionExpand != 0 {
			action = ggui.ActionExpand
		}
		app.Perform(node.ID, ggui.Action{Kind: action})
	}

	if target, ok := hoverNames[name]; ok {
		node, ok := app.Find(target)
		if !ok {
			return false, fmt.Errorf("missing hover target %s", target)
		}
		app.Move(node.Center())
		now = now.Add(600 * time.Millisecond)
		draw()
	}

	for _, state := range []struct {
		name  string
		delay time.Duration
	}{{"active", 60 * time.Millisecond}, {"settled", time.Second}} {
		draw()
		now = now.Add(state.delay)
		draw()
		save(state.name)
	}
	app.Move(ggui.Pt(-1, -1))
	app.Type(ggui.Mods{}, ggui.KeyEscape)
	if name == "Accordion" {
		app.Tap("Keyboard controls")
	}
	if name == "Collapsible" {
		app.Tap("Delivery preferences")
	}
	draw()
	now = now.Add(60 * time.Millisecond)
	draw()
	save("exit")

	if name == "Data Table" {
		perform := func(label string, action ggui.Action) {
			node, ok := app.Semantics().Find("", label)
			if !ok {
				if err == nil {
					err = fmt.Errorf("Data Table: missing %q", label)
				}
				return
			}
			app.Perform(node.ID, action)
			draw()
			now = now.Add(time.Second)
			draw()
		}
		perform("Sort Email", ggui.Action{Kind: ggui.ActionPress})
		save("sorted")
		perform("Filter rows", ggui.Action{Kind: ggui.ActionSetValue, Text: "missing"})
		save("empty")
		perform("Filter rows", ggui.Action{Kind: ggui.ActionSetValue, Text: "example"})
		save("filtered")
		perform("Next", ggui.Action{Kind: ggui.ActionPress})
		save("next-page")
		perform("Columns", ggui.Action{Kind: ggui.ActionExpand})
		save("columns")
	}

	g.index++
	if err != nil {
		return false, err
	}
	return g.index == len(g.names)*8, nil
}
func renderGalleryAudit(dir, only string) error {
	names := auditNames
	if only != "" {
		names = nil
		for _, name := range strings.Split(only, ",") {
			name = strings.TrimSpace(name)
			if !slices.Contains(auditNames, name) {
				return fmt.Errorf("unknown audit example %q", name)
			}
			names = append(names, name)
		}
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	g := &galleryAudit{dir: dir, names: names}
	return offscreen.Run(g.step)
}

func auditControlStates() ggui.Widget {
	on, off := ggui.State(true), ggui.State(false)
	text := ggui.State("invalid@example")
	return ui.Card(ggui.Column(
		ui.Title("Control states"),
		ggui.Wrap(
			ui.Checkbox(on, "Checked"),
			ui.Checkbox(on, "Disabled checked").Disabled(true),
			ui.Checkbox(ggui.State(false), "Disabled empty").Disabled(true),
		).Gap(12),
		ggui.Wrap(ui.Radio(on, true, "Selected"), ui.Radio(on, true, "Disabled selected").Disabled(true)).Gap(12),
		ggui.Wrap(
			ui.Switch(off, "Enabled switch"),
			ui.Switch(on, "Disabled on").Disabled(true),
			ui.Switch(ggui.State(false), "Disabled off").Disabled(true),
		).Gap(12),
		ggui.Wrap(ui.Button("Primary", nil), ui.Button("Delete", nil).Destructive(), ui.Button("Disabled", nil).Disabled(true)).Gap(8),
		ggui.Wrap(ui.ThemeSwitch(ggui.State(false)), ui.ThemeSwitch(ggui.State(true)).Disabled(true)).Gap(12),
		ui.Field("Email", ui.TextField(text)).BindError(ggui.State("Enter a valid email address.")),
		ui.Slider(ggui.State(.5), 0, 1).Disabled(true),
		ui.Progress(ggui.State(.6)),
	).Gap(20).Align(ggui.AlignStretch))
}
