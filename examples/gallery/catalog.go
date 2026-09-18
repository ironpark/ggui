package main

import (
	"fmt"
	"strings"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

type previewInfo struct{ category, description string }

var previewDetails = map[string]previewInfo{
	"Carousel":                 {"Navigation", "Swipeable slides, responsive sizing, looping, keyboard navigation and autoplay."},
	"Input OTP":                {"Inputs", "Grouped code slots with native editing, paste, filtering and validation states."},
	"Charts":                   {"Data", "Native GPU charts with themed tooltips, legends, curves, stacking and animations."},
	"Reactive blocks":          {"Data", "Cancellable search, await branches, keyed row identity and component cleanup."},
	"Theme presets":            {"Layout", "shadcn semantic tokens: seven base palettes, seventeen accents, Nova and Rhea geometry."},
	"Icons":                    {"Inputs", "Embedded Lucide SVGs, semantic placeholders and scoped icon-set overrides."},
	"Emoji":                    {"Inputs", "Color emoji in ordinary text and editors, with skin tones, flags, keycaps and joined sequences."},
	"Attachment":               {"Data", "File and image attachments, upload states, sizes, independent actions and snapping groups."},
	"Bubble":                   {"Layout", "Seven conversation surfaces, sender alignment, reactions and expandable content."},
	"Message":                  {"Layout", "Conversation rows with avatars, sender headers, delivery footers and shared files."},
	"Marker":                   {"Feedback", "Conversation status, system notes, labeled separators and bordered updates."},
	"Message Scroller":         {"Data", "Anchored turns, streaming replies, reading position preservation and earlier history."},
	"Questionnaire":            {"Inputs", "Single and multiple answers, freeform input, explicit skips, validation and keyboard shortcuts."},
	"Buttons":                  {"Inputs", "Primary, secondary and disabled actions, badges and tooltips."},
	"Text":                     {"Inputs", "Text input, multiline editing and a live typography preview."},
	"Field":                    {"Inputs", "Labels, helper text and inline validation."},
	"Choices":                  {"Inputs", "Checkboxes, radio groups, select menus and dropdown actions."},
	"Shadows":                  {"Layout", "GPU-rendered soft shadows with configurable blur, spread and offset."},
	"Menubar":                  {"Navigation", "Coordinated File, Edit and View menus with arrow-key navigation."},
	"Calendar":                 {"Inputs", "Single-date selection with day, week and month keyboard navigation."},
	"Date picker":              {"Inputs", "A calendar popup for choosing an appointment date."},
	"Context menu":             {"Navigation", "Right-click actions, or focus the target and press Shift+F10."},
	"Search and commands":      {"Inputs", "Searchable options and a keyboard-friendly command palette."},
	"Notices and empty states": {"Feedback", "Alerts and helpful starting points for empty collections."},
	"Loading and shortcuts":    {"Feedback", "Spinners, skeleton placeholders and keyboard hints."},
	"Toast":                    {"Feedback", "Transient messages, undo actions and persistent errors."},
	"Dialog":                   {"Feedback", "Dialog and AlertDialog confirmation flows with cancel, reset and destructive actions."},
	"Progress":                 {"Feedback", "A progress bar with completion, restart and reactive disabled state."},
	"Collapsible":              {"Navigation", "An expandable section that preserves its controls, plus a disabled example."},
	"Pagination":               {"Navigation", "Move through pages with a compact set of controls."},
	"Accordion":                {"Navigation", "Expandable sections with keyboard navigation."},
	"Tabs":                     {"Navigation", "Switch between related views without leaving the page."},
	"Resizable":                {"Layout", "Drag either grip to resize nested horizontal and vertical panels."},
	"Transition":               {"Layout", "Reveal content with a subtle fade and slide."},
	"Wrap":                     {"Layout", "Removable tags that flow naturally onto the next line."},
	"Table":                    {"Data", "Selectable rows, flexible columns and row actions."},
	"Grid":                     {"Data", "The shared color palette, presented in a responsive grid."},
	"Sheets and drawers":       {"Navigation", "Sheet and Drawer: edge-anchored modal panels that slide in and hand focus back."},
	"Sidebar":                  {"Navigation", "Sidebar navigation and a Breadcrumb trail with clickable ancestors."},
	"Groups and addons":        {"Inputs", "ToggleGroup, ButtonGroup and InputGroup: reactive disabled choices, action feedback and URL previews."},
	"Profile and media":        {"Layout", "Avatar sizes and fallbacks, Item rows, HoverCard previews and AspectRatio media."},
}

type componentPreview struct {
	ggui.Widget
	title string
	info  previewInfo
}

func preview(title string, body ggui.Widget) *componentPreview {
	info := previewDetails[title]
	return &componentPreview{
		title: title, info: info,
		Widget: ui.Card(ggui.Column(
			ggui.Row(ggui.Title(title).Size(18), ggui.Spacer(), ui.Badge(info.category)).Gap(8),
			ggui.Caption(info.description),
			ui.Divider(),
			body,
		).Gap(16).Align(ggui.AlignStretch)),
	}
}

func (p *componentPreview) matches(category, query string) bool {
	return (category == "All" || p.info.category == category) &&
		strings.Contains(strings.ToLower(p.title+" "+p.info.category+" "+p.info.description), strings.ToLower(strings.TrimSpace(query)))
}

// The grid chooses its column count at layout time, so resizing the window
// does not rebuild controls or discard their state.
type previewGrid struct {
	children []ggui.Widget
	grid     *ggui.GridWidget
}

func (g *previewGrid) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	cols := 1
	if c.MaxW >= 1000 {
		cols = 2
	}
	g.grid = ggui.Grid(cols, g.children...).Gap(20)
	return g.grid.Layout(c, env)
}
func (g *previewGrid) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(g.grid, r) }

func galleryPage(dark *ggui.StateValue[bool], search, category *ggui.StateValue[string], scroll *ggui.StateValue[float64], commands func(), entries []ggui.Widget) ggui.Widget {
	return ggui.Reactive(func() ggui.Widget {
		theme := ggui.UseTheme()
		selected, query := category.Get(), search.Get()
		var cards, overlays []ggui.Widget
		counts := map[string]int{}
		total := 0
		for _, entry := range entries {
			if p, ok := entry.(*componentPreview); ok {
				total++
				counts[p.info.category]++
				if p.matches(selected, query) {
					cards = append(cards, p)
				}
			} else {
				overlays = append(overlays, entry)
			}
		}
		counts["All"] = total
		var filters []ggui.Widget
		for _, label := range []string{"All", "Inputs", "Navigation", "Feedback", "Layout", "Data"} {
			button := ui.Button(fmt.Sprintf("%s  %d", label, counts[label]), func() { category.Set(label); scroll.Set(0) }).Named(label)
			button.Key("category-" + label)
			if label != selected {
				button.Outline()
			}
			filters = append(filters, button)
		}
		field := ui.TextField(search).Named("Search components").Placeholder("Search components…").OnChange(func(string) { scroll.Set(0) })
		field.Input().Key("gallery-search")
		var content ggui.Widget = &previewGrid{children: cards}
		if len(cards) == 0 {
			content = ui.Empty("No matching components", "Try a different search or explore another category.").Action(ui.Button("Clear filters", func() { search.Set(""); category.Set("All"); scroll.Set(0) }))
		}
		header := ggui.Box(ggui.Column(
			ggui.Row(ggui.Title("Component gallery").Size(28), ggui.Spacer(), ui.ThemeSwitch(dark)).Gap(16),
			ggui.Caption("Explore the building blocks. Try an interaction, adjust the theme, make it yours."),
			ggui.Row(ggui.Expanded(field), ui.Button("Commands", commands).Outline()).Gap(12),
			ggui.Wrap(filters...).Gap(8),
		).Gap(14).Align(ggui.AlignStretch)).Pad(24, 28).Fill(theme.Card)
		footer := ggui.Padding(ggui.Row(
			ggui.Caption(fmt.Sprintf("%d of %d previews", len(cards), total)), ggui.Spacer(),
			ggui.Caption("⌘K  Commands   ·   Tab  Navigate   ·   F1  Inspect"),
		).Gap(12), 12, 28)
		children := []ggui.Widget{header, ui.Divider(), ggui.Expanded(ggui.Scroll(ggui.Padding(content, 24, 28)).Key("gallery-previews").Offset(scroll)), ui.Divider(), footer}
		children = append(children, overlays...)
		return ggui.Column(children...).Align(ggui.AlignStretch)
	})
}
