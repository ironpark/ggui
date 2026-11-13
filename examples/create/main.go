// Command create is a native theme playground inspired by ui.shadcn.com/create.
// It keeps the neutral editor separate from the live theme in its preview.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/format"
	"io"
	"log"
	"math/rand/v2"
	"strings"

	"github.com/ironpark/ggui"
	_ "github.com/ironpark/ggui/inspect/panel"
	"github.com/ironpark/ggui/ui"
	"github.com/ironpark/ggui/ui/icons"
	"github.com/ironpark/ggui/ui/icons/lucide"
	"github.com/ironpark/ggui/ui/icons/tabler"
	"github.com/ironpark/ggui/ui/theme"
)

type customization struct {
	Theme    theme.Config
	Dark     bool
	Icons    string
	BoldMenu bool
}

func defaults() customization {
	return customization{Theme: theme.Config{Preset: theme.Preset{Base: theme.BaseNeutral, Style: theme.StyleNova}, Font: theme.FontInter, Heading: theme.FontInter}, Dark: true, Icons: "Lucide"}
}

type model struct {
	locked                             *ggui.StateValue[[lockCount]bool]
	example                            *ggui.StateValue[string]
	specimens                          ggui.Widget
	settings                           *ggui.StateValue[customization]
	status                             *ggui.StateValue[string]
	codeOpen, settingsOpen, presetOpen *ggui.StateValue[bool]
	presetText, presetError            *ggui.StateValue[string]
	preview                            *previewModel
	panel                              ggui.Widget
	codeDialog, presetDialog           *ui.DialogWidget
	settingsSheet                      *ui.SheetWidget
	source                             ggui.Readable[string]
}

func newModel() *model {
	m := &model{
		locked:  ggui.State([lockCount]bool{}),
		example: ggui.State("01"), specimens: newSpecimens(),
		settings: ggui.State(defaults()), status: ggui.State("Changes appear instantly."),
		codeOpen: ggui.State(false), settingsOpen: ggui.State(false), presetOpen: ggui.State(false),
		presetText: ggui.State(""), presetError: ggui.State(""), preview: newPreview(),
	}
	m.source = m.settings.Map(sourceCode)
	m.panel = m.customizer()
	m.settingsSheet = ui.Sheet(m.settingsOpen, m.panel).Title("Customize").Size(292)
	m.codeDialog = ui.Dialog(m.codeOpen, ggui.Column(
		ui.Caption("Use this theme in any GGUI application."),
		ui.CodeBlockOf(m.source).Language("go").LineNumbers(true).Height(340),
		ui.CaptionOf(m.status).Role(ggui.RoleStatus),
		ggui.Row(ui.Button("Copy Go code", m.copyCode), ui.Button("Done", func() { m.codeOpen.Set(false) }).Outline()).Gap(8),
	).Gap(16).Stretch()).Title("Your theme is ready").Width(680)
	m.presetDialog = ui.Dialog(m.presetOpen, ggui.Column(
		ui.Caption("Paste a preset copied from this playground."),
		ui.TextField(m.presetText).Name("Preset JSON").Multiline().Lines(8),
		ui.CaptionOf(m.presetError),
		ggui.Row(ui.Button("Apply preset", m.importPreset), ui.Button("Cancel", func() { m.presetOpen.Set(false) }).Outline()).Gap(8),
	).Gap(12).Stretch()).Title("Open preset").Width(560)
	return m
}

func title[T ~string](v T) string {
	s := string(v)
	if s == "" {
		return "Default"
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func fontTitle(v theme.FontFamily) string {
	switch v {
	case theme.FontInter:
		return "Inter"
	case theme.FontSans:
		return "Go Sans"
	case theme.FontMono:
		return "Go Mono"
	default:
		return "System default"
	}
}

func accentTitle(v theme.AccentColor) string {
	if v == theme.AccentBase {
		return "Neutral"
	}
	return title(v)
}

func (m *model) customizer() ggui.Widget {
	base := m.settings.Field(func(s *customization) *theme.BaseColor { return &s.Theme.Preset.Base })
	accent := m.settings.Field(func(s *customization) *theme.AccentColor { return &s.Theme.Preset.Accent })
	style := m.settings.Field(func(s *customization) *theme.Style { return &s.Theme.Preset.Style })
	chart := m.settings.Field(func(s *customization) *theme.AccentColor { return &s.Theme.Chart })
	radius := m.settings.Field(func(s *customization) *theme.Radius { return &s.Theme.Radius })
	font := m.settings.Field(func(s *customization) *theme.FontFamily { return &s.Theme.Font })
	heading := m.settings.Field(func(s *customization) *theme.FontFamily { return &s.Theme.Heading })
	iconSet := m.settings.Field(func(s *customization) *string { return &s.Icons })
	bold := m.settings.Field(func(s *customization) *bool { return &s.BoldMenu })
	group := func(children ...ggui.Widget) ggui.Widget {
		return ggui.Padding(ggui.Column(children...).Gap(12).Stretch(), 12)
	}
	fields := ggui.Column(
		group(
			choice(m, lockStyle, "Style", style, theme.Styles(), func(v theme.Style) string {
				if v == "" {
					return "Nova"
				}
				return title(v)
			}),
		), ui.Divider(), group(
			choice(m, lockBase, "Base color", base, theme.BaseColors(), func(v theme.BaseColor) string {
				if v == "" {
					return "Neutral"
				}
				return title(v)
			}),
			choice(m, lockAccent, "Theme", accent, theme.AccentColors(), accentTitle),
			choice(m, lockChart, "Chart color", chart, theme.AccentColors(), accentTitle),
		), ui.Divider(), group(
			choice(m, lockHeading, "Heading", heading, theme.Fonts(), fontTitle),
			choice(m, lockFont, "Font", font, theme.Fonts(), fontTitle),
		), ui.Divider(), group(
			choice(m, lockIcons, "Icon library", iconSet, []string{"Lucide", "Tabler"}, func(s string) string { return s }),
			choice(m, lockRadius, "Radius", radius, theme.RadiusOptions(), title[theme.Radius]),
			choice(m, lockMenu, "Menu accent", bold, []bool{false, true}, func(v bool) string {
				if v {
					return "Bold"
				}
				return "Subtle"
			}),
		),
	).Gap(0).Stretch()
	menu := ui.MenuOf(ggui.Row(ggui.Text("Menu"), ggui.Spacer(), menuMark()).Gap(12),
		ui.MenuItem("Reset", m.reset),
		ui.MenuItem("Shuffle", m.shuffle),
		ui.MenuItem("Light / Dark", m.toggleAppearance),
		ui.MenuDivider(),
		ui.MenuItem("Open preset", m.openPreset),
		ui.MenuItem("Copy preset", m.copyPreset),
		ui.MenuItem("Get Code", func() { m.codeOpen.Set(true) }),
	).Name("Menu").Width(224)
	return ggui.Column(
		ggui.Padding(menu, 12), ui.Divider(),
		ggui.Expanded(ggui.Scroll(fields).Key("settings-scroll")),
		ui.Divider(),
		ggui.Padding(ggui.Column(
			ui.Button("Copy preset", m.copyPreset).Outline(),
			ui.Button("Open Preset", m.openPreset).Name("Open preset").Outline(),
			ui.Button("Shuffle", m.shuffle).Outline(),
		).Gap(8).Stretch(), 12), ui.Divider(),
		ggui.Padding(ggui.Column(ui.Button("Get Code", func() { m.codeOpen.Set(true) })).Stretch(), 12),
	).Gap(0).Stretch()
}

func icon(role icons.Role) *icons.Widget {
	return icons.Placeholder(role).Fallback(lucide.Set()).Size(16)
}

func (m *model) reset() {
	m.settings.Set(defaults())
	m.locked.Set([lockCount]bool{})
	m.status.Set("Default theme restored.")
}

func (m *model) toggleAppearance() { s := ggui.Peek(m.settings); s.Dark = !s.Dark; m.settings.Set(s) }

func randomItem[T any](items []T) T { return items[rand.IntN(len(items))] }

func (m *model) shuffle() {
	s := ggui.Peek(m.settings)
	locks := ggui.Peek(m.locked)
	if !locks[lockBase] {
		s.Theme.Preset.Base = randomItem(theme.BaseColors())
	}
	if !locks[lockAccent] {
		s.Theme.Preset.Accent = randomItem(theme.AccentColors())
	}
	if !locks[lockStyle] {
		s.Theme.Preset.Style = randomItem(theme.Styles())
	}
	if !locks[lockChart] {
		s.Theme.Chart = randomItem(theme.AccentColors())
	}
	if !locks[lockRadius] {
		s.Theme.Radius = randomItem(theme.RadiusOptions())
	}
	if !locks[lockHeading] {
		s.Theme.Heading = randomItem(theme.Fonts())
	}
	if !locks[lockFont] {
		s.Theme.Font = randomItem(theme.Fonts())
	}
	if !locks[lockIcons] {
		s.Icons = randomItem([]string{"Lucide", "Tabler"})
	}
	if !locks[lockMenu] {
		s.BoldMenu = randomItem([]bool{false, true})
	}
	m.settings.Set(s)
	m.status.Set("A new palette. Make it yours.")
}

func (m *model) copyCode() {
	ggui.CurrentClipboard().Write(ggui.Peek(m.source))
	m.status.Set("Go code copied.")
}
func (m *model) copyPreset() {
	data, _ := json.MarshalIndent(ggui.Peek(m.settings), "", "  ")
	ggui.CurrentClipboard().Write(string(data))
	m.status.Set("Preset copied.")
}
func (m *model) openPreset() { m.presetText.Set(""); m.presetError.Set(""); m.presetOpen.Set(true) }

func (m *model) importPreset() {
	var s customization
	decoder := json.NewDecoder(strings.NewReader(ggui.Peek(m.presetText)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&s); err != nil {
		m.presetError.Set("Invalid preset: " + err.Error())
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		m.presetError.Set("Paste a single JSON preset.")
		return
	}
	if err := validateCustomization(s); err != nil {
		m.presetError.Set(err.Error())
		return
	}
	m.settings.Set(s)
	m.presetOpen.Set(false)
	m.status.Set("Preset applied.")
}

func validateCustomization(s customization) (err error) {
	if s.Icons != "Lucide" && s.Icons != "Tabler" {
		return fmt.Errorf("Icon library must be Lucide or Tabler.")
	}
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("Invalid theme: %v", r)
		}
	}()
	_ = s.Theme.For(s.Dark)
	return nil
}

func sourceCode(s customization) string {
	library := "lucide"
	if s.Icons == "Tabler" {
		library = "tabler"
	}
	code := fmt.Sprintf(`package main

import (
 "github.com/ironpark/ggui/ui/theme"
 "github.com/ironpark/ggui/ui/icons"
 "github.com/ironpark/ggui/ui/icons/%s"
)

// Call applyTheme before creating your application window.
func applyTheme() {
 t := %s
`, library, s.Theme.GoCode(s.Dark))
	if s.BoldMenu {
		code += "t.Accent, t.AccentFg = t.Primary, t.PrimaryFg\n"
	}
	code += fmt.Sprintf("t = t.Set(icons.SetKey, %s.Set())\ntheme.Set(t)\n}\n", library)
	formatted, err := format.Source([]byte(code))
	if err != nil {
		panic(err)
	}
	return string(formatted)
}

func (m *model) build() ggui.Widget {
	s := m.settings.Get()
	shell := (theme.Config{Font: theme.FontInter}).For(s.Dark)
	selected := s.Theme.For(s.Dark)
	if s.BoldMenu {
		selected.Accent, selected.AccentFg = selected.Primary, selected.PrimaryFg
	}
	var set icons.Set = lucide.Set()
	if s.Icons == "Tabler" {
		set = tabler.Set()
	}
	content := m.preview.build()
	if m.example.Get() == "02" {
		content = m.specimens
	}
	preview := theme.With(selected, ggui.Provide(icons.SetKey, set, content))
	switches := ggui.Padding(ggui.Box(ui.ToggleGroup(m.example).Options([]string{"01", "02"}).Key("preview-mode")).Fill(shell.Card).Radius(12).Pad(4), 12)
	previewFrame := ggui.Box(ggui.Stack(preview, ggui.Align(switches).Right().Bottom())).Fill(selected.Bg).Radius(18).Border(1, shell.Border)
	panel := ggui.Box(m.panel).Fill(shell.Card).Radius(16).Border(1, shell.Border).Width(208)
	modeRole := icons.Sun
	if !s.Dark {
		modeRole = icons.Moon
	}
	mode := ui.ButtonOf(icon(modeRole), m.toggleAppearance).Ghost().Name("Toggle appearance").Key("toggle-appearance")
	brand := ggui.Row(ggui.Text("//").Size(24).Weight(ggui.WeightSemibold), ggui.Text("ggui").Weight(ggui.WeightSemibold), ui.Caption("/"), ggui.Text("Create")).Gap(12)
	getCode := ui.Button("Get Code", func() { m.codeOpen.Set(true) })
	wideHeader := ggui.Row(brand, ggui.Spacer(), ui.CaptionOf(m.status), mode, getCode).Gap(16)
	narrowHeader := ggui.Row(ggui.Text("ggui / Create").Weight(ggui.WeightSemibold), ggui.Spacer(), mode, getCode).Gap(8)
	header := ggui.Padding(ggui.Responsive(760, wideHeader, narrowHeader), 16, 24)
	body := ggui.Responsive(760,
		ggui.Row(panel, ggui.Expanded(previewFrame)).Gap(24).Stretch(),
		ggui.Column(ggui.Row(ui.Button("Customize", func() { m.settingsOpen.Set(true) }).Outline(), ggui.Spacer(), ui.Caption("Live preview")).Gap(8), ggui.Expanded(previewFrame)).Gap(12).Stretch(),
	)
	page := ggui.Column(header, ggui.Expanded(ggui.Padding(body, 0, 24, 24, 24))).Gap(0).Stretch()
	return theme.With(shell, ggui.Stack(ggui.Box(page).Fill(shell.Bg), m.codeDialog, m.presetDialog, ggui.Responsive(760, ggui.Box(), m.settingsSheet)))
}

func main() {
	renderDir := flag.String("render-dir", "", "Render example screenshots and exit")
	flag.Parse()
	if *renderDir != "" {
		if err := render(*renderDir); err != nil {
			log.Fatal(err)
		}
		return
	}
	var m *model
	app := ggui.New(ggui.Config{Title: "ggui · create", Width: 1440, Height: 960, Resizable: true, Inspector: "f1"}, func() ggui.Widget { m = newModel(); return ggui.Reactive(m.build) })
	app.Shortcut("cmd+shift+c", func() { m.copyCode() })
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
