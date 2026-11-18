// Package kernel is where the root package leaves what package widget needs
// of it and nothing outside ggui may call: functions whose types name an
// internal package, which the root package cannot export without making a
// public API no other module can use.
//
// The root package fills each variable as it initializes, which is before
// any package importing it does. A variable holds a func typed in terms of
// the root package's own types, which this package cannot name; the reader
// asserts it to that type once, in a package variable of its own.
//
// Record goes the other way: package record, which imports the root
// package, fills it, and the root package calls it when it is set.
package kernel

// TextEngineStyle is a func(ggui.TextStyle) (paragraph.Style, uint64): st as
// the text engine sets text in it, with what st leaves unset filled in as
// with no Env (Env.ResolveText's result is returned as it is), and the
// generation of the active world's fonts, which changes whenever a
// measurement made in them may no longer hold.
var TextEngineStyle any

// Record runs what the GGUI_RECORD environment variable asks for, request
// being its value, and returns the status the process exits with. Package
// record sets it as it initializes; App.Run calls it in place of opening
// the app when the variable is set, which is how ggui record runs a tour
// the app registered. Its type names no root type, so it is typed here.
var Record func(request string) (exitCode int)
