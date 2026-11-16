// Package ggui is a cross-platform GUI framework for Go: Flutter's widget tree
// and layout model, driven by Svelte-style reactivity, rendered by ggfx.
//
// This package is the kernel, what every widget stands on: the [App] and
// its windows, the frame loop, the [Canvas] a widget paints on, pointer,
// key, focus and drag input, the [Env] that flows down the tree,
// reactivity ([State], [Derived], [Effect], [Component]), the widget
// protocol ([Widget], [Scope], [LayoutBoundary], [Invalidate], [Cached])
// and the [Probe] and [WindowProbe] that drive a tree in tests. It holds no
// widget of its own beyond those the protocol needs.
//
// The widgets are in two packages above it, each written on the public API
// of those below, as an application's own widgets would be:
//
//   - [github.com/ironpark/ggui/widget] holds the unstyled building blocks:
//     text, boxes, rows, columns, stacks and grids, scrolling, keyed lists,
//     popups, transitions, compositing, pointer and drag and drop, the text
//     input and images. They take their looks from the Env and what they
//     are given.
//   - [github.com/ironpark/ggui/ui] is the themed component library.
//
// An app runs on the kernel and builds its tree from the widgets:
//
//	count := ggui.State(0)
//	app := ggui.New(ggui.Config{Title: "Hello"}, func() ggui.Widget {
//		return widget.Center(widget.Textf("Count: %d", count))
//	})
package ggui
