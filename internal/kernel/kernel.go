// Package kernel is where the root package leaves what package widget needs
// of it and nothing outside ggui may call: functions whose types name an
// internal package, which the root package cannot export without making a
// public API no other module can use.
//
// The root package fills each variable as it initializes, which is before
// any package importing it does. A variable holds a func typed in terms of
// the root package's own types, which this package cannot name; the reader
// asserts it to that type once, in a package variable of its own.
package kernel

// TextEngineStyle is a func(ggui.TextStyle) (paragraph.Style, uint64): st as
// the text engine sets text in it, with what st leaves unset filled in as
// with no Env (Env.ResolveText's result is returned as it is), and the
// generation of the active world's fonts, which changes whenever a
// measurement made in them may no longer hold.
var TextEngineStyle any
