// Package i18n translates an application's own text, the way ui.Strings
// words the controls' own.
//
// A Localizer holds an app's catalogs and follows a reactive language.
// Provide puts it in the Env of a subtree, Install in the application's.
// What T and N return is a Message: a value, not a string, resolved when it
// is read. Read as a widget lays out it takes the language of the subtree
// it is shown in, so a language change lays the text out again and rebuilds
// nothing, and two subtrees can show two languages at once. A Message is a
// ggui.Readable[string], so every widget that follows a reader takes one:
//
//	lang := ggui.State("ko")
//	loc := i18n.New(lang, ko, ja) // *i18n.Catalog values
//	root := i18n.Provide(loc, widget.Column(
//		widget.TextOf(i18n.T("New sandbox")),
//		widget.TextOf(i18n.N(count, "%d sandbox running", "%d sandboxes running")),
//	))
//	// lang.Set("ja") re-lays the text out in Japanese
//
// The key of a message is its sentence in the source language, English
// here; a sentence no catalog translates is shown as it is. There is no
// global current language: what a Message reads is the Localizer of the
// Env it is resolved in, and off the tree, the application's.
//
// See docs/i18n.md for catalogs, plurals, Korean particles, dates, numbers
// and sizes, and how to combine it with ui.Strings.
package i18n
