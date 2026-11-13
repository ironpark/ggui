package ggui

import (
	"image/color"
	"sync/atomic"
	"time"

	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/pkg/layout"
)

// BackgroundKey supplies the window background when Config.Background is nil.
var BackgroundKey = NewEnvKey[color.Color]("background")

// SpacingKey supplies the logical-pixel unit used by layout Space setters.
var SpacingKey = NewEnvKey[float64]("spacing")

// EditorStyle describes the colors used by the unadorned text editor.
type EditorStyle struct {
	Muted     color.Color // placeholder and disabled text
	Selection color.Color
}

var EditorStyleKey = NewEnvKey[EditorStyle]("editor style")

// ScrollStyle describes a scrollbar independently of any design system.
type ScrollStyle struct {
	Color, HoverColor color.Color
	Duration          time.Duration
	// Thickness is how wide the thumb is across the direction it moves in,
	// in logical pixels; zero is 3.
	Thickness float64
	// HoverThickness is how wide the thumb grows under the pointer and
	// while it is dragged; zero is one and a half times Thickness, and a
	// value equal to Thickness keeps the thumb from growing.
	HoverThickness float64
	// Margin is the space between the thumb and the edge of the scroll it
	// rides along; zero is 2.
	Margin float64
}

var ScrollStyleKey = NewEnvKey[ScrollStyle]("scroll style")

// PopupDurationKey controls the built-in popup reveal animation.
var PopupDurationKey = NewEnvKey[time.Duration]("popup duration")

// Background returns the inherited background, or white.
func (e Env) Background() color.Color {
	if c, ok := e.Get(BackgroundKey); ok && c != nil {
		return c
	}
	return defaultBackground
}

// defaultBackground is color.White boxed once: boxing it on each call, a
// frame's background fill would allocate.
var defaultBackground color.Color = color.White

// Spacing returns the inherited layout unit, or eight logical pixels.
func (e Env) Spacing() float64 {
	if v, ok := e.Get(SpacingKey); ok {
		return v
	}
	return 8
}

// EditorStyle resolves missing colors to standalone editor defaults.
func (e Env) EditorStyle() EditorStyle {
	s, _ := e.Get(EditorStyleKey)
	if s.Muted == nil {
		s.Muted = color.Gray{Y: 0x71}
	}
	if s.Selection == nil {
		s.Selection = color.Gray{Y: 0xd4}
	}
	return s
}

// ScrollStyle resolves missing colors and sizes to standalone scrollbar
// defaults.
func (e Env) ScrollStyle() ScrollStyle {
	s, ok := e.Get(ScrollStyleKey)
	if !ok {
		s.Duration = 150 * time.Millisecond
	}
	if s.Color == nil {
		s.Color = color.Gray{Y: 0x71}
	}
	if s.HoverColor == nil {
		s.HoverColor = color.Black
	}
	if s.Thickness <= 0 {
		s.Thickness = 3
	}
	if s.HoverThickness <= 0 {
		s.HoverThickness = 1.5 * s.Thickness
	}
	if s.Margin <= 0 {
		s.Margin = 2
	}
	return s
}

// PopupDuration returns the inherited reveal duration, or 150 milliseconds.
func (e Env) PopupDuration() time.Duration {
	if d, ok := e.Get(PopupDurationKey); ok {
		return d
	}
	return 150 * time.Millisecond
}

// The application environment is kept per world: the App's windows share
// one, each Probe has its own, and a goroutine's Base world holds what is
// set there before any app or probe runs. A world's starts as a copy of the
// Base world beside it when it is first read, which for an app or a probe
// is its first frame, so SetEnv before Run, or before a probe's first
// frame, reaches it. From then on the two are separate.

// environment returns w's environment signal, copying its parent's the
// first time.
func (w *world) environment() *StateValue[Env] {
	if w.env == nil {
		var start Env
		if w.parent != nil {
			start = Untrack(w.parent.environment().Get)
		}
		w.env = State(start).WithEqual(nil)
	}
	return w.env
}

// UseEnv reads the application environment and subscribes the current builder
// or effect to replacements. Layout receives this environment automatically.
func UseEnv() Env { return activeWorld().environment().Get() }

// SetEnv replaces the application environment and invalidates layout: the
// environment of the App or Probe whose frames run on this goroutine, or
// before any has, the one the next to start copies. Call on the UI thread
// or before starting an application.
func SetEnv(env Env) {
	activeWorld().environment().Set(env)
}

func rootEnv() Env { return Untrack(UseEnv) }

// TextDirection is the direction a subtree reads in: left to right, as
// English does, or right to left, as Arabic and Hebrew do. The zero value
// is DirLTR.
type TextDirection = layout.Direction

const (
	DirLTR = layout.LTR
	DirRTL = layout.RTL
)

// DirectionKey holds the direction of the subtree below it; Directionality
// provides it and Env.Direction reads it, DirLTR by default.
var DirectionKey = NewEnvKey[TextDirection]("text direction")

// LanguageKey holds the BCP 47 language of the subtree below it, such as
// "ar" or "he-IL", which text hands the shaper with each right-to-left
// run so that it picks the language's forms. Env.Language reads it.
var LanguageKey = NewEnvKey[string]("language")

// Direction returns the direction the tree under e reads in.
func (e Env) Direction() TextDirection {
	d, _ := e.Get(DirectionKey)
	return d
}

// Language returns the BCP 47 language set for the tree under e, or "".
func (e Env) Language() string {
	l, _ := e.Get(LanguageKey)
	return l
}

// Directionality lays child out reading in dir. Under DirRTL a paragraph
// orders its runs right to left, text starts at the right edge, a Row
// places its first child rightmost and a Column puts AlignStart children
// on the right; Align's Left and Right, and an explicit Text.Align, stay
// where they say.
//
//	ggui.Directionality(ggui.DirRTL, ggui.Text("שלום עולם"))
func Directionality(dir TextDirection, child Widget) *EnvWidget {
	return Provide(DirectionKey, dir, child)
}

// startAlign is where a line of text starts by default: the left edge, or
// the right one under DirRTL.
func startAlign(d TextDirection) float64 {
	if d == DirRTL {
		return 1
	}
	return 0
}

// EnvColor is a color the Env holds, such as a theme's primary color. A
// widget given one resolves it against the Env it is laid out in, so the
// widget follows a theme switch, or a subtree's own theme, without being
// rebuilt. Its RGBA is the value in the application's environment, for code
// that draws it without an Env.
//
// An EnvColor is compared with ==, as every color property is, so it must
// be comparable; the theme package's tokens are.
type EnvColor interface {
	color.Color
	Resolve(env Env) color.Color
}

// ResolveColor returns c as env has it: an EnvColor's value there, and any
// other color as it is. A custom widget that takes a color resolves it this
// way in Layout and paints the result.
func ResolveColor(c color.Color, env Env) color.Color {
	if e, ok := c.(EnvColor); ok {
		return e.Resolve(env)
	}
	return c
}

// IsEnvColor reports whether c is an EnvColor, which a widget can only
// paint once layout has resolved it.
func IsEnvColor(c color.Color) bool {
	_, ok := c.(EnvColor)
	return ok
}

// shownColor is a color property as it was given and as it is painted: the
// given color itself, or an EnvColor's value in the Env of the last layout.
type shownColor struct{ given, shown color.Color }

// set gives the property c. A widget lays out again for an EnvColor, given
// or replaced, since only layout knows its Env.
func (s *shownColor) set(c color.Color, changed func()) {
	if c == s.given {
		return
	}
	if IsEnvColor(c) || IsEnvColor(s.given) {
		changed()
	}
	s.given, s.shown = c, c
}

func (s *shownColor) layout(env Env) { s.shown = ResolveColor(s.given, env) }

// ResponsiveWidget lays out one of two arrangements by the width it is
// given. Build one with Responsive.
type ResponsiveWidget struct {
	breakpoint   float64
	wide, narrow Widget
	current      Widget
}

// Responsive lays out wide while it is given at least breakpoint of width,
// and narrow below that. The two arrangements may hold the same widgets, so
// a control keeps its state, focus included, as the window crosses the
// breakpoint:
//
//	title, actions := ui.Title("Tasks"), ui.Button("New", create)
//	ggui.Responsive(680,
//		ggui.Row(ggui.Expanded(title), actions),
//		ggui.Column(title, actions).Stretch())
func Responsive(breakpoint float64, wide, narrow Widget) *ResponsiveWidget {
	return &ResponsiveWidget{breakpoint: breakpoint, wide: wide, narrow: narrow, current: wide}
}

// Layout implements Widget.
func (r *ResponsiveWidget) Layout(c Constraints, env Env) Size {
	r.current = pick(c.MaxW < r.breakpoint, r.narrow, r.wide)
	return r.current.Layout(c, env)
}

// Baseline implements Baseliner: the shown arrangement's.
func (r *ResponsiveWidget) Baseline() (float64, bool) { return baselineOf(r.current) }

// Paint implements Widget.
func (r *ResponsiveWidget) Paint(dst *Canvas, rect Rect) { dst.Paint(r.current, rect) }

// Styling has three layers. A TextStyle is a value: build one, merge others
// onto it, hand it to Text or Styled. An Env flows down the tree at layout
// time, so a Styled container sets the text style every descendant starts
// from, the way CSS inherits font and color. Design systems can provide
// additional typed values through EnvKey without coupling the core to them.

// TextStyle describes how text is drawn. The zero value of a field means
// "inherit": Merge lets a set field win over an unset one, and Text resolves
// what is still unset from the Env and then from built-in defaults.
type TextStyle struct {
	Font  *Font
	Size  float64 // pixels
	Color color.Color
	// LineHeight is the height of a line as a multiple of Size, as CSS's
	// line-height: a Text or RichText it was set for, by its own style or
	// an inherited one, makes every line a box that high with the glyphs
	// centered in it. Unset, text is as tall as its glyphs, with 1.2 times
	// the size between baselines.
	LineHeight float64
	Weight     FontWeight // drawn with the Font's face nearest this weight
	// Tracking is the space added after each letter as a multiple of
	// Size, as CSS letter-spacing in em is: -0.025 sets a heading tight,
	// 0.1 a shortcut hint wide. Text is measured, wrapped, hit and drawn
	// with it. Zero inherits.
	Tracking float64
}

// Equal reports whether s and o draw the same: the same Font, size, line
// height, weight and tracking, and colors that draw alike.
func (s TextStyle) Equal(o TextStyle) bool {
	if s.Font != o.Font || s.Size != o.Size || s.LineHeight != o.LineHeight || s.Weight != o.Weight || s.Tracking != o.Tracking {
		return false
	}
	if s.Color == nil || o.Color == nil {
		return s.Color == nil && o.Color == nil
	}
	ar, ag, ab, aa := s.Color.RGBA()
	br, bg, bb, ba := o.Color.RGBA()
	return ar == br && ag == bg && ab == bb && aa == ba
}

func init() { property.RegisterEqual(TextStyle.Equal) }

// Merge returns s with every set field of o laid over it.
func (s TextStyle) Merge(o TextStyle) TextStyle {
	if o.Font != nil {
		s.Font = o.Font
	}
	if o.Size != 0 {
		s.Size = o.Size
	}
	if o.Color != nil {
		s.Color = o.Color
	}
	if o.LineHeight != 0 {
		s.LineHeight = o.LineHeight
	}
	if o.Weight != 0 {
		s.Weight = o.Weight
	}
	if o.Tracking != 0 {
		s.Tracking = o.Tracking
	}
	return s
}

// resolved fills what is still unset with the built-in defaults.
func (s TextStyle) resolved() TextStyle {
	if s.Font == nil {
		s.Font = fallbackFont()
	}
	if s.Size == 0 {
		s.Size = DefaultTextSize
	}
	if s.Color == nil {
		s.Color = color.Black
	}
	if s.LineHeight == 0 {
		s.LineHeight = 1.2
	}
	return s
}

// Env is the set of inherited values a widget lays out under. The runtime
// hands the root one to the tree; containers pass it down unchanged, and
// Styled or Provide hand their child a modified copy. It is a value: adding
// to it never changes the parent's.
type Env struct {
	text TextStyle
	vals *envNode
	rev  uint64 // advanced by every change; Cached compares it
}

// envRev numbers every distinct Env, so a layout cache can tell whether
// anything inherited changed without comparing the values themselves.
var envRev atomic.Uint64

func nextRev() uint64 { return envRev.Add(1) }

type envNode struct {
	key  *byte // the EnvKey's id: a pointer, which an interface holds without allocating
	val  any
	next *envNode
}

// Text returns the inherited text style, the base a Text merges its own
// style onto.
func (e Env) Text() TextStyle { return e.text }

// ResolveText returns the style a Text with style s draws in under e: s
// merged onto the inherited style, unset fields filled with the defaults
// and the size multiplied by TextScale. A custom widget keeps it from
// Layout to draw a label with Canvas.DrawText in Paint.
func (e Env) ResolveText(s TextStyle) TextStyle {
	r := e.text.Merge(s).resolved()
	r.Size *= e.TextScale()
	r.Color = ResolveColor(r.Color, e)
	return r
}

// WithText returns e with s merged onto the inherited text style.
func (e Env) WithText(s TextStyle) Env {
	return e.WithTextReplaced(e.text.Merge(s))
}

// WithTextReplaced replaces the inherited text style, including zero fields.
// Use WithText to merge a partial style instead.
func (e Env) WithTextReplaced(s TextStyle) Env {
	if sameAny(s, e.text) {
		return e
	}
	val := any(s)
	out, ok, hashable := e.memo(textKey, val)
	if ok {
		return out
	}
	out = e
	out.text, out.rev = s, nextRev()
	if hashable {
		e.remember(textKey, val, out)
	}
	return out
}

var textKey = new(byte)

// TextScaleKey holds the factor every Text and TextInput multiplies its
// size by, for a user who asked for larger text: Provide it above the tree
// or a subtree. Env.TextScale reads it, 1 by default.
var TextScaleKey = NewEnvKey[float64]("text scale")

// InputDisabledKey disables TextInput editing in a subtree without replacing the
// editor's own Disabled or BindDisabled setting. Containers combine inherited
// and local values with OR; false must not enable an already-disabled ancestor.
var InputDisabledKey = NewEnvKey[bool]("input disabled")

// ReducedMotionKey asks widgets not to animate: transitions land at once,
// eased motions jump. Provide it above the tree; Env.Motion reads it.
var ReducedMotionKey = NewEnvKey[bool]("reduced motion")

// TextScale returns the factor text sizes are multiplied by under e.
func (e Env) TextScale() float64 {
	if s, ok := e.Get(TextScaleKey); ok && s > 0 {
		return s
	}
	return 1
}

// ReducedMotion reports whether the tree under e asked for no animation.
func (e Env) ReducedMotion() bool {
	r, _ := e.Get(ReducedMotionKey)
	return r
}

// Motion returns d, or zero when the tree under e asked for reduced
// motion: what a widget hands to Ease or a Transition.
func (e Env) Motion(d time.Duration) time.Duration {
	if e.ReducedMotion() {
		return 0
	}
	return d
}

// EnvKey names a value that can travel down the tree in an Env. Make one per
// concept with NewEnvKey; the type parameter keeps reads and writes in step.
type EnvKey[T any] struct {
	id   *byte
	name string
	same *envSame[T] // what equal means for With; nil is ==
}

// NewEnvKey creates a distinct EnvKey; name is for messages only.
func NewEnvKey[T any](name string) EnvKey[T] { return EnvKey[T]{id: new(byte), name: name} }

// NewEnvKeyFunc is NewEnvKey for values whose == is not what equal means,
// as for a pointer to a value that is the same as another when what they
// point to is. With takes same in place of ==: storing a value same to
// the one there leaves the Env unchanged, and storing one same to a value
// stored before under the same parent gives back that Env, revision and
// value included, so the caches below hold and the first value stays the
// one read. same must not panic.
func NewEnvKeyFunc[T any](name string, same func(a, b T) bool) EnvKey[T] {
	return EnvKey[T]{id: new(byte), name: name, same: &envSame[T]{same}}
}

// envSame holds an EnvKey's equality behind a pointer, so EnvKeys stay
// comparable.
type envSame[T any] struct{ fn func(a, b T) bool }

// equal reports whether With treats a and b as the same value.
func (k EnvKey[T]) equal(a, b T) bool {
	if k.same != nil {
		return k.same.fn(a, b)
	}
	return sameAny(a, b)
}

// With returns e with v stored under k for the subtree below. Storing the
// value already there, by ==, leaves e unchanged, and storing the value
// stored last frame under the same parent yields the same revision, so
// caches below a Provide rebuilt every frame hold. InputDisabledKey is cumulative:
// once true in an ancestor, a descendant cannot clear it.
func (e Env) With[T any](k EnvKey[T], v T) Env {
	if k.id == InputDisabledKey.id {
		if disabled, _ := e.Get(InputDisabledKey); disabled {
			return e
		}
	}
	cur, exists := e.Get(k)
	if exists && k.equal(cur, v) {
		return e
	}
	if k.same != nil {
		return e.withSame(k, v, exists)
	}
	// The memo below is found by v as an any, which boxing a value that
	// is not a pointer allocates. The Env last derived from e under k is
	// found without: when it holds v already, as it does on a frame where
	// nothing changed, it is the answer.
	m := &activeWorld().envMemo
	lastKey := derivedKey{from: e.rev, key: k.id, val: envLastDerived{}}
	if last, ok := m.get(lastKey); ok {
		if lv, _ := last.Get(k); k.equal(lv, v) {
			return last
		}
	}
	out := e.derive(k.id, v, exists)
	m.put(lastKey, out)
	return out
}

// envLastDerived marks the memo entry of the Env last derived from a
// parent under a key, apart from the entries of the values derived.
type envLastDerived struct{}

// derive returns e with val stored under key: the Env derived the same
// way before, if the memo has it. exists says whether e stores a value
// under key already.
func (e Env) derive(key *byte, val any, exists bool) Env {
	out, ok, hashable := e.memo(key, val)
	if ok {
		return out
	}
	out = e
	if exists {
		out.vals = withEnvValue(e.vals, key, val)
	} else {
		out.vals = &envNode{key: key, val: val, next: e.vals}
	}
	out.rev = nextRev()
	if hashable {
		e.remember(key, val, out)
	}
	return out
}

// withSame is With for a key with its own equality, which a map cannot
// hash by: the Envs derived from e under k are kept in a list and found by
// k.same.
func (e Env) withSame[T any](k EnvKey[T], v T, exists bool) Env {
	m := &activeWorld().envMemo.same
	key := derivedKey{from: e.rev, key: k.id}
	derived := m.get(key)
	for i, d := range derived {
		if k.same.fn(d.val.(T), v) {
			if i > 0 { // most recent first, so the one in use is found at once
				copy(derived[1:i+1], derived[:i])
				derived[0] = d
			}
			return d.out
		}
	}
	out := e
	if exists {
		out.vals = withEnvValue(e.vals, k.id, v)
	} else {
		out.vals = &envNode{key: k.id, val: v, next: e.vals}
	}
	out.rev = nextRev()
	if len(derived) == envSameMax {
		derived = derived[:envSameMax-1]
	}
	m.put(key, append([]derivedSame{{v, out}}, derived...))
	return out
}

// Replace the nearest binding persistently instead of retaining every old
// value. Root environment updates remain bounded while ancestor Envs stay intact.
func withEnvValue(node *envNode, key *byte, value any) *envNode {
	if node == nil {
		return &envNode{key: key, val: value}
	}
	if node.key == key {
		return &envNode{key: key, val: value, next: node.next}
	}
	return &envNode{key: node.key, val: node.val, next: withEnvValue(node.next, key, value)}
}

// derivedKey names an Env derived from another: the parent's revision and
// what was added.
type derivedKey struct {
	from uint64
	key  any
	val  any
}

// envMemo remembers derived Envs, so the same derivation yields the same
// revision every time it is made and a Cached below it holds. Each world
// keeps its own, written from derive on the goroutine running that world's
// frames. A revision names one Env across every world, so an Env derived in
// one is never mistaken for another's.
//
// Entries age by how many new ones arrive, not by frames: a subtree a
// Cached skipped derives nothing for as long as it is skipped, and when it
// is next laid out it must find its Envs again, or it mints new revisions
// and every cache inside it misses. So an entry lives in two generations.
// One that is used moves into the current one; the current one becomes the
// old one once it is as large as the old one, and whatever the old one
// still held then is dropped. A tree that stops deriving new Envs keeps all
// of its own, and the memo stays within about twice what is in use.
type envMemo struct {
	cur, prev map[derivedKey]Env
	same      envMemoSame // for keys with their own equality
}

// derivedSame is an Env derived with a value of a key with its own
// equality.
type derivedSame struct {
	val any
	out Env
}

// envSameMax bounds the values remembered for one key under one parent:
// enough for every theme an app derives in one place, with room to spare,
// while a value that changes every frame churns through it.
const envSameMax = 64

// envMemoSame is envMemo for keys with their own equality: the Envs
// derived from one parent under one key, most recent first, found by the
// key's equality. It ages as envMemo does.
type envMemoSame struct {
	cur, prev map[derivedKey][]derivedSame
}

func (m *envMemoSame) get(k derivedKey) []derivedSame {
	if d, ok := m.cur[k]; ok {
		return d
	}
	d, ok := m.prev[k]
	if ok {
		m.put(k, d)
	}
	return d
}

func (m *envMemoSame) put(k derivedKey, d []derivedSame) {
	if m.cur == nil {
		m.cur = map[derivedKey][]derivedSame{}
	}
	if _, ok := m.cur[k]; !ok && len(m.cur) >= max(envMemoMin, len(m.prev)) {
		m.prev, m.cur = m.cur, m.prev
		if m.cur == nil {
			m.cur = map[derivedKey][]derivedSame{}
		}
		clear(m.cur)
	}
	m.cur[k] = d
}

// envMemoMin is the size below which the memo never ages anything out.
const envMemoMin = 256

func (m *envMemo) get(k derivedKey) (Env, bool) {
	if out, ok := m.cur[k]; ok {
		return out, true
	}
	out, ok := m.prev[k]
	if ok {
		m.put(k, out)
	}
	return out, ok
}

func (m *envMemo) put(k derivedKey, e Env) {
	if m.cur == nil {
		m.cur = map[derivedKey]Env{}
	}
	if len(m.cur) >= max(envMemoMin, len(m.prev)) {
		m.prev, m.cur = m.cur, m.prev
		if m.cur == nil {
			m.cur = map[derivedKey]Env{}
		}
		clear(m.cur)
	}
	m.cur[k] = e
}

// memo returns the Env derived from e with key and val before, if one
// was. A val a map cannot hash is never memoized: hashable reports it.
func (e Env) memo(key, val any) (out Env, ok, hashable bool) {
	defer func() {
		if recover() != nil {
			hashable = false
		}
	}()
	out, ok = activeWorld().envMemo.get(derivedKey{e.rev, key, val})
	return out, ok, true
}

// remember keeps out as the Env derived from e with key and val, for the
// same derivation to yield the same revision.
func (e Env) remember(key, val any, out Env) {
	activeWorld().envMemo.put(derivedKey{e.rev, key, val}, out)
}

// Get returns the nearest value stored under k, if any ancestor set one.
func (e Env) Get[T any](k EnvKey[T]) (T, bool) {
	for n := e.vals; n != nil; n = n.next {
		if n.key == k.id {
			return n.val.(T), true
		}
	}
	var zero T
	return zero, false
}
