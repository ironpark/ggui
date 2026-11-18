package i18n

import "github.com/ironpark/ggui"

// Message is text to be translated when it is read: a sentence with its
// arguments, a plural, or a date, a number or a size to be written the
// way a language writes it. T, N, Since, Bytes, Number and Date make one.
//
// A Message is a ggui.Readable[string]. Its Get translates it into the
// language of the Localizer of the Env it is read in (ggui.CurrentEnv):
// the subtree's while a widget lays it out or paints it, the
// application's anywhere else. So it goes wherever a reader does, and a
// widget showing it follows the language without being rebuilt:
//
//	widget.TextOf(i18n.T("New sandbox"))
//	button.BindName(i18n.T("Delete %s", name))
//
// Messages without arguments compare equal when their text and context
// are, so a rebuilt tree handing a widget the same one again changes
// nothing. A Message with arguments is equal only to its copies.
type Message struct {
	kind  kind
	key   string // the source sentence; a plural's singular
	other string // a plural's plural
	ctx   string
	n     int
	args  *[]any // nil when there are none, keeping the Message comparable
}

// kind is what a Message holds.
type kind uint8

const (
	kindText kind = iota
	kindPlural
	kindSince
	kindBytes
	kindNumber
	kindDate
)

// T returns the message key formatted with args: key is the sentence in
// the source language, which a catalog translates, and a fmt format the
// args fill in. An argument that is itself a Message, or any
// ggui.Readable[string], is resolved as the message is, in the same
// language; one with a GetAny method, as a *ggui.StateValue has, is read.
// A message with no arguments is shown as written, so a % in it is
// written once.
//
//	i18n.T("New sandbox")
//	i18n.T("Delete %s?", name)            // name a string or a reader
//	i18n.T("Moved to %s", i18n.T("Trash"))
func T(key string, args ...any) Message {
	return Message{kind: kindText, key: key, args: argsOf(args)}
}

// N returns a plural message: one is the source sentence for a count of
// one, other for any other count, and a catalog translates them into the
// forms of its language under one's key. n is the count, which picks the
// form, and the first argument, so the sentences start with it; args
// follow.
//
//	i18n.N(len(files), "%d file selected", "%d files selected")
//	i18n.N(n, "%d sandbox in %s", "%d sandboxes in %s", project)
func N(n int, one, other string, args ...any) Message {
	return Message{kind: kindPlural, key: one, other: other, n: n, args: argsOf(args)}
}

// argsOf keeps a copy of args, or nil when there are none.
func argsOf(args []any) *[]any {
	if len(args) == 0 {
		return nil
	}
	a := append([]any(nil), args...)
	return &a
}

// arguments returns m's arguments.
func (m Message) arguments() []any {
	if m.args == nil {
		return nil
	}
	return *m.args
}

// value returns the one value a format Message holds.
func (m Message) value() any { return (*m.args)[0] }

// Context returns m distinguished by ctx from another message of the same
// source text that a language translates differently: "Open" the verb on
// a button and "Open" the state of an issue. A catalog translates it under
// the key "Open [verb]", and falls back to "Open".
//
//	i18n.T("Open").Context("verb")
func (m Message) Context(ctx string) Message {
	m.ctx = ctx
	return m
}

// Get returns m translated in the Env it is read in, ggui.CurrentEnv: the
// Localizer of the subtree laying out or painting now, or the
// application's. It reads the Localizer's language, so a builder or an
// effect reading it runs again when it changes.
func (m Message) Get() string { return m.In(ggui.CurrentEnv()) }

// In returns m translated by the Localizer of env, for a widget of your
// own reading it in Layout with the Env it was given.
func (m Message) In(env ggui.Env) string { return From(env).message(m) }

// String returns m in the source language, as written in the code.
func (m Message) String() string { return source.message(m) }
