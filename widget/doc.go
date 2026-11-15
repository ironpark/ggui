// Package widget holds ggui's unstyled widgets: the text input, images and
// Reveal, which draw only what they are and take their looks from the Env
// and what they are given.
//
// It is the middle of three layers. Package ggui is the kernel: the frame
// loop, layout, paint, input, semantics and the primitives the kernel
// itself needs, and nothing that can be written on what it exports. A
// widget that can be is here, written on ggui's public API and the
// module's internal packages, as an application's own widget would be.
// Package ui, the themed component library, builds on both: what has a
// theme, an icon set or an embedded font belongs there, since widget
// imports none of ui, ui/theme, ui/icons or fonts, and an application
// that wants an unstyled editor does not carry the design system for it.
package widget
