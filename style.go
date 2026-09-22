package ggui

import (
	"image/color"
	"sync/atomic"
	"time"
)

// Styling has three layers. A TextStyle is a value: build one, merge others
// onto it, hand it to Text or Styled. An Env flows down the tree at layout
// time, so a Styled container sets the text style every descendant starts
// from, the way CSS inherits font and color. Design systems can provide
// additional typed values through EnvKey without coupling the core to them.

// TextStyle describes how text is drawn. The zero value of a field means
// "inherit": Merge lets a set field win over an unset one, and Text resolves
// what is still unset from the Env and then from built-in defaults.
type TextStyle struct {
	Font       *Font
	Size       float64 // pixels
	Color      color.Color
	LineHeight float64 // multiple of Size between baselines
}

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
	key  any
	val  any
	next *envNode
}

// Text returns the inherited text style, the base a Text merges its own
// style onto.
func (e Env) Text() TextStyle { return e.text }

// WithText returns e with s merged onto the inherited text style.
func (e Env) WithText(s TextStyle) Env {
	return e.WithTextStyle(e.text.Merge(s))
}

// WithTextStyle replaces the inherited text style, including zero fields.
// Use WithText to merge a partial style instead.
func (e Env) WithTextStyle(s TextStyle) Env {
	if sameAny(s, e.text) {
		return e
	}
	return e.derive(textKey, s, func() Env {
		e.text, e.rev = s, nextRev()
		return e
	})
}

var textKey = new(byte)

// TextScaleKey holds the factor every Text and TextInput multiplies its
// size by, for a user who asked for larger text: Provide it above the tree
// or a subtree. Env.TextScale reads it, 1 by default.
var TextScaleKey = NewEnvKey[float64]("text scale")

// InputDisabled disables TextInput editing in a subtree without replacing the
// editor's own Disabled or BindDisabled setting. Containers combine inherited
// and local values with OR; false must not enable an already-disabled ancestor.
var InputDisabled = NewEnvKey[bool]("input disabled")

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
}

// NewEnvKey creates a distinct EnvKey; name is for messages only.
func NewEnvKey[T any](name string) EnvKey[T] { return EnvKey[T]{id: new(byte), name: name} }

// With returns e with v stored under k for the subtree below. Storing the
// value already there, by ==, leaves e unchanged, and storing the value
// stored last frame under the same parent yields the same revision, so
// caches below a Provide rebuilt every frame hold. InputDisabled is cumulative:
// once true in an ancestor, a descendant cannot clear it.
func (e Env) With[T any](k EnvKey[T], v T) Env {
	if k.id == InputDisabled.id {
		if disabled, _ := e.Get(InputDisabled); disabled {
			return e
		}
	}
	cur, exists := e.Get(k)
	if exists && sameAny(cur, v) {
		return e
	}
	return e.derive(k, v, func() Env {
		if exists {
			e.vals = withEnvValue(e.vals, k, v)
		} else {
			e.vals = &envNode{key: k, val: v, next: e.vals}
		}
		e.rev = nextRev()
		return e
	})
}

// Replace the nearest binding persistently instead of retaining every old
// value. Root environment updates remain bounded while ancestor Envs stay intact.
func withEnvValue(node *envNode, key, value any) *envNode {
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

// envMemo remembers the Envs derived this frame and last, so the same
// derivation yields the same revision frame after frame. Canvas.nextFrame
// rotates it.
// envMemo holds the Envs derived this frame and last, so a container that
// derives the same Env every frame reuses it. Written from derive and
// rotated once a frame, both during layout on the UI goroutine.
var envMemo struct {
	cur, prev map[derivedKey]Env
}

func rotateEnvMemo() {
	envMemo.prev, envMemo.cur = envMemo.cur, envMemo.prev
	clear(envMemo.cur)
}

// derive returns the Env derived from e with key and val last frame or
// this one, else fn's. A val that cannot be hashed is never memoized.
func (e Env) derive(key, val any, fn func() Env) (out Env) {
	k := derivedKey{e.rev, key, val}
	memoized := true
	func() {
		defer func() {
			if recover() != nil {
				memoized = false
			}
		}()
		var ok bool
		if out, ok = envMemo.cur[k]; ok {
			return
		}
		if out, ok = envMemo.prev[k]; ok {
			return
		}
		ok = false
		out = fn()
		if envMemo.cur == nil {
			envMemo.cur = map[derivedKey]Env{}
		}
		envMemo.cur[k] = out
	}()
	if !memoized {
		return fn()
	}
	return out
}

// Get returns the nearest value stored under k, if any ancestor set one.
func (e Env) Get[T any](k EnvKey[T]) (T, bool) {
	for n := e.vals; n != nil; n = n.next {
		if n.key == any(k) {
			return n.val.(T), true
		}
	}
	var zero T
	return zero, false
}
