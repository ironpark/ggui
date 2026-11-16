// Package widget holds ggui's unstyled building blocks: text and rich text,
// boxes, rows, columns, stacks, grids and the other layouts, scrolling,
// keyed lists, popups, transitions, compositing, pointer, focus and drag
// and drop, the text input, images and Reveal. Each draws only what it is
// and takes its looks from the Env and what it is given.
//
// It is the middle of three layers. Package ggui is the kernel: the app and
// its windows, the frame loop, the canvas, input, reactivity, the widget
// protocol and the Probe, and no widget that can be written on what it
// exports. Every widget here is written on ggui's public API and the
// module's internal packages, as an application's own widget would be.
// Package ui, the themed component library, builds on both: what has a
// theme, an icon set or an embedded font belongs there, since widget
// imports none of ui, ui/theme, ui/icons or fonts, and an application
// that wants an unstyled editor does not carry the design system for it.
//
//	ggui.New(ggui.Config{Title: "Hello"}, func() ggui.Widget {
//		return widget.Column(
//			widget.Text("Hello").Size(24),
//			widget.Textf("Count: %d", count),
//		).Gap(8)
//	})
package widget
