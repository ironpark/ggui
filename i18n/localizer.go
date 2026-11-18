package i18n

import (
	"fmt"
	"strings"

	"github.com/ironpark/ggui"
)

// Localizer translates into the language a reactive value names, from the
// catalogs it was made with. Provide hands it to a subtree and Install to
// the application, where the Messages of T and N find it; S and N
// translate with it directly, for text off the tree.
type Localizer struct {
	lang ggui.Readable[string] // nil: the source language
	cats map[string]*Catalog   // by normalized tag
}

// New returns a Localizer translating into the language lang names, a BCP
// 47 tag such as "ko" or "pt-BR", from catalogs. lang is read each time
// something is translated, so setting it switches every Message shown,
// at the next layout. Catalogs of the same language are merged, a later
// one's translations winning; nil ones are skipped. The catalogs are copied:
// changing one later changes nothing.
//
// A language is looked up as its tag, then with its subtags dropped one
// by one ("pt-BR", then "pt"); a sentence none of those translate is shown
// in the source language. A nil lang is the source language.
//
//	lang := ggui.State("ko")
//	loc := i18n.New(lang, ko, ja)
func New(lang ggui.Readable[string], catalogs ...*Catalog) *Localizer {
	l := &Localizer{lang: lang, cats: map[string]*Catalog{}}
	for _, c := range catalogs {
		if c == nil {
			continue
		}
		tag := normalize(c.Lang)
		m := l.cats[tag]
		if m == nil {
			m = &Catalog{Lang: c.Lang, Messages: map[string]string{}, Plurals: map[string]Forms{}}
			l.cats[tag] = m
		}
		for k, v := range c.Messages {
			m.Messages[k] = v
		}
		for k, v := range c.Plurals {
			m.Plurals[k] = v
		}
	}
	return l
}

// source is the Localizer of an Env none was provided in: it shows every
// message in the source language.
var source = &Localizer{}

// Lang returns the language l translates into now, and subscribes the
// running builder or effect to it as reading lang does.
func (l *Localizer) Lang() string {
	if l == nil || l.lang == nil {
		return ""
	}
	return l.lang.Get()
}

// S returns key translated into l's language now, formatted with args as
// fmt.Sprintf formats; see T. It reads the language, so a builder or an
// effect calling it runs again when it changes. For text the tree shows,
// pass T's Message instead, which needs no rebuild.
//
//	tray.SetTooltip(loc.S("%d sandboxes running", n))
func (l *Localizer) S(key string, args ...any) string {
	return l.message(T(key, args...))
}

// N returns the plural message of N translated into l's language now; see
// N and S.
func (l *Localizer) N(n int, one, other string, args ...any) string {
	return l.message(N(n, one, other, args...))
}

// localizerKey holds the Localizer Provide or Install gave a subtree.
var localizerKey = ggui.NewEnvKey[*Localizer]("i18n localizer")

// From returns the Localizer provided to the tree under env, or one
// showing every message in the source language when none was. A widget of
// your own that is handed an Env translates with it in Layout:
//
//	label := i18n.From(env).S("Untitled")
func From(env ggui.Env) *Localizer {
	if l, ok := env.Get(localizerKey); ok && l != nil {
		return l
	}
	return source
}

// with returns env with l and the language lang provided: the Localizer,
// ggui.LanguageKey and ggui.DirectionKey.
func with(env ggui.Env, l *Localizer, lang string) ggui.Env {
	env = env.With(localizerKey, l)
	if lang == "" {
		return env
	}
	return env.With(ggui.LanguageKey, lang).With(ggui.DirectionKey, Direction(lang))
}

// Provide gives the subtree under child the Localizer l: the Messages
// shown there are translated by it. It provides the language too, as
// ggui.LanguageKey, which text is shaped by, and its direction as
// ggui.DirectionKey, so an Arabic or Hebrew subtree reads right to left.
// All three follow l's language: when it changes the subtree is laid out
// again, and nothing is rebuilt.
//
//	root := i18n.Provide(loc, app)
//
// A subtree under it can Provide another Localizer, a preview of the app
// in another language say. Text read off the tree, in a menu or a window
// title, finds the application's Localizer instead: see Install.
func Provide(l *Localizer, child ggui.Widget) *ggui.EnvWidget {
	if l == nil {
		panic("i18n: Provide requires a non-nil Localizer")
	}
	return ggui.WithEnv(func(env ggui.Env) ggui.Env { return with(env, l, l.Lang()) }, child)
}

// Install makes l the application's Localizer: it puts l, its language and
// that language's direction in the application environment (ggui.SetEnv),
// and puts them there again whenever l's language changes. A Message read
// off the tree, by a menu, a tray, a toast or a window title, is translated
// by it, and so is every tree no Provide covers, popups included.
//
// Call it on the UI thread: in a builder or Setup, where it lasts as long
// as what is being built, or before the app runs, where it lasts as long
// as the program.
//
//	app.Setup(func() { i18n.Install(loc) })
func Install(l *Localizer) {
	if l == nil {
		panic("i18n: Install requires a non-nil Localizer")
	}
	ggui.Observe(func() {
		lang := l.Lang()
		ggui.Untrack(func() struct{} {
			env := ggui.UseEnv()
			if From(env) != l || env.Language() != lang || env.Direction() != Direction(lang) {
				ggui.SetEnv(with(env, l, lang))
			}
			return struct{}{}
		})
	})
}

// Watch runs fn with l now and again each time l's language changes, and
// returns a function to stop: for what is not a widget and takes strings,
// such as a menu bar or a tray, to be rebuilt in the new language. What fn
// reads besides l's language does not run it again.
//
//	stop := i18n.Watch(loc, func(loc *i18n.Localizer) {
//		app.SetMenu(menu(loc)) // built with loc.S("Quit"), …
//	})
func Watch(l *Localizer, fn func(*Localizer)) (dispose func()) {
	return ggui.Observe(func() {
		l.Lang()
		ggui.Untrack(func() struct{} { fn(l); return struct{}{} })
	})
}

// lookup returns the translation of key with context ctx in the language
// tag, from l's catalogs and then the built-in ones, and the tag of the
// language it was found in.
func (l *Localizer) lookup(tag, key, ctx string) (string, string, bool) {
	tags := fallbacks(tag)
	for _, k := range keys(key, ctx) {
		for _, t := range tags {
			if c := l.cats[t]; c != nil {
				if s, ok := c.Messages[k]; ok && s != "" {
					return s, t, true
				}
			}
			if c := builtin[t]; c != nil {
				if s, ok := c.Messages[k]; ok && s != "" {
					return s, t, true
				}
			}
		}
	}
	return "", "", false
}

// lookupPlural is lookup for a plural message, by its singular key.
func (l *Localizer) lookupPlural(tag, key, ctx string) (Forms, string, bool) {
	tags := fallbacks(tag)
	for _, k := range keys(key, ctx) {
		for _, t := range tags {
			if c := l.cats[t]; c != nil {
				if f, ok := c.Plurals[k]; ok {
					return f, t, true
				}
			}
			if c := builtin[t]; c != nil {
				if f, ok := c.Plurals[k]; ok {
					return f, t, true
				}
			}
		}
	}
	return Forms{}, "", false
}

// keys returns the keys a message is looked up under: with its context,
// then without.
func keys(key, ctx string) []string {
	if ctx == "" {
		return []string{key}
	}
	return []string{contextKey(key, ctx), key}
}

// text returns key with context ctx translated into tag, or key.
func (l *Localizer) text(tag, key, ctx string) string {
	if s, _, ok := l.lookup(tag, key, ctx); ok {
		return s
	}
	return key
}

// message resolves m in l's language.
func (l *Localizer) message(m Message) string {
	tag := l.Lang()
	switch m.kind {
	case kindText:
		s, found, ok := l.lookup(tag, m.key, m.ctx)
		if !ok {
			s = m.key
		}
		return l.format(found, s, m.arguments())
	case kindPlural:
		return l.plural(tag, m.n, m.key, m.other, m.ctx, m.arguments())
	case kindSince:
		return l.since(tag, m.value().(timeArg).t)
	case kindBytes:
		return l.bytes(tag, m.value().(int64))
	case kindNumber:
		return l.number(tag, m.value())
	case kindDate:
		d := m.value().(timeArg)
		return l.date(tag, d.t, d.style)
	}
	return m.key
}

// plural resolves a plural message: the form of n's category in tag, with
// n the first argument.
func (l *Localizer) plural(tag string, n int, singular, plural, ctx string, args []any) string {
	all := append([]any{n}, args...)
	if forms, found, ok := l.lookupPlural(tag, singular, ctx); ok {
		if f := forms.pick(pluralOf(tag, n)); f != "" {
			return l.format(found, f, all)
		}
	}
	if n == 1 {
		return l.format("", singular, all)
	}
	return l.format("", plural, all)
}

// format formats f, found in the language tag ("" for the source), with
// args, each Message or reader among them resolved first. A message with
// no arguments is shown as written.
func (l *Localizer) format(tag, f string, args []any) string {
	if len(args) == 0 {
		return f
	}
	resolved := make([]any, len(args))
	for i, a := range args {
		resolved[i] = l.resolve(a)
	}
	if base(tag) == "ko" {
		return sprintfKorean(f, resolved)
	}
	s := fmt.Sprintf(f, resolved...)
	// A translation may leave an argument out, as a plural's form for one
	// often leaves out the count ("a file"): fmt's note of it is dropped.
	if i := strings.LastIndex(s, "%!(EXTRA "); i >= 0 && strings.HasSuffix(s, ")") {
		s = s[:i]
	}
	return s
}

// resolve returns the value an argument stands for: a Message translated
// by l, a reader's value, or the argument itself.
func (l *Localizer) resolve(a any) any {
	switch v := a.(type) {
	case Message:
		return l.message(v)
	case ggui.Readable[string]:
		return v.Get()
	case interface{ GetAny() any }:
		return v.GetAny()
	}
	return a
}
