package main

import (
	"fmt"
	"strings"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

type previewInfo struct{ category, description string }

var previewDetails = map[string]previewInfo{
	"Carousel":                 {"Navigation", "Swipeable slides, responsive sizing, looping, keyboard navigation and autoplay."},
	"Input OTP":                {"Inputs", "Grouped code slots with native editing, paste, filtering and validation states."},
	"Charts":                   {"Data", "Native GPU charts with themed tooltips, legends, curves, stacking and animations."},
	"Reactive blocks":          {"Data", "Cancellable search, await branches, keyed row identity and component cleanup."},
	"Theme presets":            {"Layout", "shadcn semantic tokens: base palettes, accents, seven styles, radius, menu and chart colors, icon sets and light, dark or system mode."},
	"Icons":                    {"Inputs", "Embedded Lucide SVGs, semantic placeholders and scoped icon-set overrides."},
	"Rich text":                {"Inputs", "Styled spans in one paragraph, tappable links, selection and copy, and right-to-left text."},
	"Emoji":                    {"Inputs", "Color emoji in ordinary text and editors, with skin tones, flags, keycaps and joined sequences."},
	"Attachment":               {"Data", "File and image attachments, upload states, sizes, independent actions and snapping groups."},
	"Bubble":                   {"Layout", "Seven conversation surfaces, sender alignment, reactions and expandable content."},
	"Message":                  {"Layout", "Conversation rows with avatars, sender headers, delivery footers and shared files."},
	"Marker":                   {"Feedback", "Conversation status, system notes, labeled separators and bordered updates."},
	"Message Scroller":         {"Data", "Anchored turns, streaming replies, reading position preservation and earlier history."},
	"Questionnaire":            {"Inputs", "Single and multiple answers, freeform input, explicit skips, validation and keyboard shortcuts."},
	"Buttons":                  {"Inputs", "Primary, secondary and other variants, sizes, leading and trailing icons, icon-only actions, badge variants and tooltips."},
	"Text":                     {"Inputs", "Text input, multiline editing and a live typography preview."},
	"Field":                    {"Inputs", "Labels, helper text and inline validation."},
	"Choices":                  {"Inputs", "Checkboxes, vertical and horizontal radio groups, select menus and dropdown actions."},
	"Shadows":                  {"Layout", "GPU-rendered soft shadows with configurable blur, spread and offset."},
	"Effects":                  {"Layout", "Transforms that keep input aligned, gradients, shape clips, opacity and blurs."},
	"Menubar":                  {"Navigation", "Coordinated File, Edit and View menus with arrow-key navigation."},
	"Calendar":                 {"Inputs", "Single-date selection with month and year dropdowns and day, week and month keyboard navigation."},
	"Date picker":              {"Inputs", "Calendar popups for an appointment date and for a range of nights."},
	"Context menu":             {"Navigation", "Right-click actions with a label, icons, a submenu and a destructive row, or focus the target and press Shift+F10."},
	"Search and commands":      {"Inputs", "A combobox typed into, with a clear button, and a keyboard-friendly command palette."},
	"Notices and empty states": {"Feedback", "Alerts with icons and actions, and an outlined starting point for an empty collection."},
	"Loading and shortcuts":    {"Feedback", "Spinners, skeleton placeholders and grouped key caps."},
	"Toast":                    {"Feedback", "Transient messages by type, a loading notice that resolves, undo actions, persistent errors and six positions."},
	"Dialog":                   {"Feedback", "Dialog and AlertDialog confirmation flows with a description, a footer, media and destructive actions."},
	"Progress":                 {"Feedback", "Labelled bars showing their value, with completion, restart and reactive disabled state."},
	"Collapsible":              {"Navigation", "An expandable section that preserves its controls, plus a disabled example."},
	"Pagination":               {"Navigation", "Move through pages with a compact set of controls."},
	"Accordion":                {"Navigation", "Expandable sections with keyboard navigation."},
	"Tabs":                     {"Navigation", "Switch between related views without leaving the page."},
	"Resizable":                {"Layout", "Drag either grip to resize nested horizontal and vertical panels."},
	"Transition":               {"Layout", "Reveal content with a subtle fade and slide."},
	"Wrap":                     {"Layout", "Removable tags that flow naturally onto the next line."},
	"Data Table":               {"Data", "Search, sorting, column visibility, page selection and pagination."},
	"Table":                    {"Data", "Selectable rows, flexible columns, row actions, a footer and a caption."},
	"Grid":                     {"Data", "The shared color palette, presented in a responsive grid."},
	"Sheets and drawers":       {"Navigation", "Sheet and Drawer: edge-anchored modal panels with descriptions, footers and a grab handle that hand focus back."},
	"Sidebar":                  {"Navigation", "Sidebar with icons, a badge, a folding sub-menu, a trigger and an icon rail, beside a Breadcrumb trail."},
	"Popover":                  {"Feedback", "Anchored rich content with a title, a description, a chosen side, dismissal and focus restoration."},
	"Navigation menu":          {"Navigation", "Navigation links and rich dropdown content with keyboard navigation."},
	"Code block":               {"Data", "Selectable code, line numbers, scrolling and clipboard actions."},
	"Trace":                    {"Feedback", "Nested execution steps, status, duration and expandable details."},
	"Range slider":             {"Inputs", "Multiple independently focusable thumbs, snapping and minimum separation."},
	"Toggle and label":         {"Inputs", "Pressed buttons with text or icons, multiple selection and labels that focus their control."},
	"Multiple combobox":        {"Inputs", "Searchable multiple selection with checked options and removable chips."},
	"Native select":            {"Inputs", "Platform selection menu with a themed fallback."},
	"Drawer snapping":          {"Feedback", "Draggable drawer with controlled snap points and dismissal."},
	"Calendar modes":           {"Inputs", "Range selection over two months and independent dates, with bounds and disabled-date rules."},
	"Tabs modes":               {"Navigation", "Vertical tabs, disabled pages and manual keyboard activation."},
	"RTL controls":             {"Inputs", "Mirrored geometry and directional keyboard behavior."},
	"Groups and addons":        {"Inputs", "ToggleGroup, ButtonGroup and InputGroup: joined segments, group text and separators, a search field with its action, and URL previews."},
	"Card":                     {"Layout", "Header with an action, content and a footer, at the default and the small size."},
	"Form layout":              {"Inputs", "A field set with a legend, grouped select options, separators and horizontal fields."},
	"Validation states":        {"Inputs", "Checkbox, radio, switch and select marked invalid until answered."},
	"Dropdown menu":            {"Navigation", "Labels, icons, shortcuts, check and radio rows, a submenu and a destructive row."},
	"Input group":              {"Inputs", "Icons, text and badges as addons, and a multiline composer with rows above and below."},
	"Tooltip":                  {"Feedback", "Tips on each side of their trigger and a key hint drawn as a key cap."},
	"Alert dialog":             {"Feedback", "A small confirmation with media that must be answered."},
	"Profile and media":        {"Layout", "Avatar sizes, badges and groups, Item lists with media icons, HoverCard previews and AspectRatio media."},
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
			ggui.Wrap(ui.Title(title).Size(18), ui.Badge(info.category)).Gap(8),
			ui.Caption(info.description),
			ui.Divider(),
			body,
		).Gap(16).Stretch()),
	}
}

func (p *componentPreview) matches(category, query string) bool {
	return (category == "All" || p.info.category == category) &&
		strings.Contains(strings.ToLower(p.title+" "+p.info.category+" "+p.info.description), strings.ToLower(strings.TrimSpace(query)))
}

// previewGrid chooses its column count at layout time, so resizing the
// window does not rebuild controls or discard their state.
func previewGrid(cards []ggui.Widget) ggui.Widget {
	return ggui.Responsive(1000, ggui.Grid(2, cards...).Gap(20), ggui.Grid(1, cards...).Gap(20))
}

// galleryPage builds the chrome once. The filter row follows category, the
// preview grid follows category and search, and the header's surface color
// follows the theme; nothing else is rebuilt, so the search field keeps its
// caret while typing and the scroll keeps its offset while filtering.
func galleryPage(dark ggui.Binding[bool], search, category *ggui.StateValue[string], scroll *ggui.StateValue[float64], commands func(), entries []ggui.Widget) ggui.Widget {
	var previews []*componentPreview
	var extras []ggui.Widget
	counts := map[string]int{}
	for _, entry := range entries {
		if p, ok := entry.(*componentPreview); ok {
			previews = append(previews, p)
			counts[p.info.category]++
		} else {
			extras = append(extras, entry)
		}
	}
	counts["All"] = len(previews)
	shown := ggui.Combine(category, search, func(selected, query string) []ggui.Widget {
		var cards []ggui.Widget
		for _, p := range previews {
			if p.matches(selected, query) {
				cards = append(cards, p)
			}
		}
		return cards
	})
	reset := func() {
		search.Set("")
		category.Set("All")
		scroll.Set(0)
	}

	filters := ggui.View(category, func(selected string) *ggui.WrapWidget {
		var buttons []ggui.Widget
		for _, label := range []string{"All", "Inputs", "Navigation", "Feedback", "Layout", "Data"} {
			button := ui.Button(fmt.Sprintf("%s  %d", label, counts[label]), func() {
				category.Set(label)
				scroll.Set(0)
			}).Name(label)
			if label != selected {
				button.Outline()
			}
			buttons = append(buttons, button)
		}
		return ggui.Wrap(buttons...).Gap(8)
	})
	field := ui.TextField(search).Name("Search components").Placeholder("Search components…").OnChange(func(string) { scroll.Set(0) })
	header := ggui.Box(ggui.Column(
		ggui.Row(ui.Title("Component gallery").Size(28), ggui.Spacer(), ui.ThemeSwitch(dark)).Gap(16),
		ui.Caption("Explore the building blocks. Try an interaction, adjust the theme, make it yours."),
		ggui.Row(ggui.Expanded(field), ui.Button("Commands", commands).Outline()).Gap(12),
		filters,
	).Gap(14).Stretch()).Pad(24, 28).Fill(uitheme.Card)
	content := ggui.View(shown, func(cards []ggui.Widget) ggui.Widget {
		if len(cards) == 0 {
			return ui.Empty("No matching components", "Try a different search or explore another category.").Action(ui.Button("Clear filters", reset))
		}
		return previewGrid(cards)
	})
	footer := ggui.Padding(ggui.Row(
		ui.Captionf("%d of %d previews", shown.Map(func(cards []ggui.Widget) int { return len(cards) }), len(previews)), ggui.Spacer(),
		ui.Caption("⌘K  Commands   ·   Tab  Navigate   ·   F1  Inspect"),
	).Gap(12), 12, 28)
	children := []ggui.Widget{header, ui.Divider(), ggui.Expanded(ggui.Scroll(ggui.Padding(content, 24, 28)).BindOffset(scroll)), ui.Divider(), footer}
	children = append(children, extras...)
	return ggui.Column(children...).Stretch()
}
