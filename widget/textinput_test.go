package widget

import (
	"image"
	"image/color"
	"math"
	"math/rand/v2"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode"
	"unsafe"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/a11y"
	"github.com/ironpark/ggui/internal/reactive"
	"github.com/ironpark/ggui/internal/textlayout"
	"github.com/ironpark/ggui/internal/third_party/text"
	"github.com/ironpark/ggui/runtime"
	"golang.org/x/image/font/gofont/goregular"
)

// fakeIME stands in for the platform IME: a test sets its composition or
// commit and the next tick delivers it.
type fakeIME struct {
	h         ggui.IMEHandler
	shown     string // the composition it last showed
	compose   string
	commit    string
	confirmed int
	cancelled int
	handled   bool
	// passthrough commits with the key that ended the composition passed on
	// to the editor, as Korean IMEs do with Enter.
	passthrough bool
}

// show shows s as what is being composed, "" for nothing.
func (f *fakeIME) show(s string, caret int) {
	f.shown = s
	f.h.Composition(s, caret)
}

func (f *fakeIME) Update(image.Rectangle) (bool, error) {
	if f.commit != "" {
		f.h.Session()
		f.h.Commit(f.commit)
		f.commit = ""
		f.show("", 0)
		return !f.passthrough, nil
	}
	if f.compose != "" {
		f.show(f.compose, len(f.compose))
		return true, nil
	}
	return f.handled, nil
}

func (f *fakeIME) Confirm() {
	f.confirmed++
	f.compose = ""
	if f.shown != "" {
		f.h.Session()
		f.h.Commit(f.shown)
	}
	f.show("", 0)
}

func (f *fakeIME) Rebase(string, string) {}

func (f *fakeIME) Cancel() {
	f.cancelled++
	f.compose = ""
	f.show("", 0)
}

// Typed ignores typing: an IME delivers it through Commit.
func (f *fakeIME) Typed(string) {}

// useFakeIME gives the editors the test goroutine makes, and the ones its
// probes build, a fake IME. A subtest runs on a goroutine of its own and
// calls it again.
func useFakeIME(t *testing.T) {
	t.Helper()
	t.Cleanup(ggui.SetIME(func(h ggui.IMEHandler) ggui.IME { return &fakeIME{h: h} }))
}

// paintFrame lays w out at size and paints it, as a frame does, into a
// Canvas with no image of its own: what a frame asks of w, with no loop
// around it to route input.
func paintFrame(w ggui.Widget, size ggui.Size) {
	// A frame settles its effects before it lays out, as the runtime's tick
	// does: a widget that follows a binding has then seen the last write.
	reactive.Flush()
	w.Paint(&ggui.Canvas{}, ggui.Rct(ggui.Pt(0, 0), w.Layout(ggui.Tight(size), ggui.Env{})))
}

// cmd is the platform's command modifier held: ⌘ on macOS, Ctrl elsewhere.
var cmd = ggui.Mods{Meta: isDarwin, Ctrl: !isDarwin}

// closeTo reports whether a and b are equal but for rounding.
func closeTo(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

// focusedInput focuses a TextInput holding value in a probe, with a click
// past the end of its text.
func focusedInput(t *testing.T, value *ggui.StateValue[string]) (*TextInputWidget, *ggui.Probe) {
	t.Helper()
	useFakeIME(t)
	w := TextInput(value)
	p := ggui.NewProbe(w, ggui.Sz(200, 20))
	t.Cleanup(p.Close)
	p.Click(ggui.Pt(190, 10))
	if !w.Focused() {
		t.Fatal("click did not focus the input")
	}
	return w, p
}

func TestTextInputEditsWriteTheSignal(t *testing.T) {
	t.Parallel()
	value := ggui.State("hello")
	changes := 0
	w, p := focusedInput(t, value)
	w.OnChange(func(string) { changes++ })
	if w.ed.Caret != len("hello") {
		t.Fatalf("click past the end put the caret at %d, want %d", w.ed.Caret, len("hello"))
	}
	p.Type(ggui.Mods{}, ggui.KeyBackspace, ggui.KeyBackspace)
	if ggui.Untrack(value.Get) != "hel" || changes != 2 {
		t.Fatalf("value = %q after two backspaces, changes = %d", ggui.Untrack(value.Get), changes)
	}
	p.Type(ggui.Mods{Shift: true}, ggui.KeyHome)
	p.Type(ggui.Mods{}, ggui.KeyDelete)
	if ggui.Untrack(value.Get) != "" {
		t.Fatalf("shift+home then delete left %q", ggui.Untrack(value.Get))
	}
}

func TestTextInputFollowsExternalWrites(t *testing.T) {
	t.Parallel()
	value := ggui.State("abc")
	w, p := focusedInput(t, value)
	value.Set("")
	p.Frame()
	if w.ed.Text != "" || w.ed.Caret != 0 {
		t.Fatalf("editor text %q caret %d after the signal was cleared", w.ed.Text, w.ed.Caret)
	}
}

func TestTextInputSubmitAndClipboard(t *testing.T) {
	t.Parallel()
	value := ggui.State("copy me")
	var submitted []string
	w, p := focusedInput(t, value)
	// The probe runs these frames, and its clipboard is the one the editor
	// reaches.
	clip := p.Clipboard()
	w.OnSubmit(func(s string) { submitted = append(submitted, s) })
	p.Type(cmd, ggui.KeyA)
	p.Type(cmd, ggui.KeyC)
	if clip.Read() != "copy me" {
		t.Fatalf("clipboard = %q after select all + copy", clip.Read())
	}
	p.Type(cmd, ggui.KeyX)
	if ggui.Untrack(value.Get) != "" {
		t.Fatalf("cut left %q", ggui.Untrack(value.Get))
	}
	clip.Write("line1\nline2")
	p.Type(cmd, ggui.KeyV)
	if ggui.Untrack(value.Get) != "line1 line2" {
		t.Fatalf("paste gave %q, want newlines folded to spaces", ggui.Untrack(value.Get))
	}
	p.Type(ggui.Mods{}, ggui.KeyEnter)
	if len(submitted) != 1 || submitted[0] != "line1 line2" {
		t.Fatalf("submitted = %v", submitted)
	}
}

func TestTextInputDragSelectsAndDoubleClickSelectsWord(t *testing.T) {
	t.Parallel()
	value := ggui.State("hello world")
	w, p := focusedInput(t, value)
	// Drag from the left edge to the far right selects everything.
	p.Press(ggui.Pt(0, 10))
	p.Move(ggui.Pt(199, 10))
	p.Release(ggui.Pt(199, 10))
	if w.ed.Selected() != "hello world" {
		t.Fatalf("drag selected %q, want everything", w.ed.Selected())
	}
	click := p.Click
	x := w.advance("hello wo")
	click(ggui.Pt(x, 10))
	click(ggui.Pt(x, 10))
	if w.ed.Selected() != "world" {
		t.Fatalf("double-click selected %q, want \"world\"", w.ed.Selected())
	}
	click(ggui.Pt(x, 10))
	if w.ed.Selected() != "hello world" {
		t.Fatalf("triple-click selected %q, want the line", w.ed.Selected())
	}
}

func TestTextInputTripleClickSelectsTheLineNotTheText(t *testing.T) {
	t.Parallel()
	useFakeIME(t)
	w := TextInput(ggui.State("first\nsecond line\nthird")).Multiline()
	p := ggui.NewProbe(w, ggui.Sz(300, 100))
	defer p.Close()
	p.Frame()
	at := ggui.Pt(w.advance("sec"), w.spacing()+5)
	for range 3 {
		p.Click(at)
	}
	if w.ed.Selected() != "second line" {
		t.Fatalf("triple-click selected %q, want the second line", w.ed.Selected())
	}
}

func TestTextInputWrapsOnceForAnUnchangedText(t *testing.T) {
	t.Parallel()
	useFakeIME(t)
	w := TextInput(ggui.State("one two three four five six seven")).Multiline()
	w.Layout(ggui.Loose(ggui.Sz(80, ggui.Unbounded)), ggui.Env{})
	a, b := w.spans(w.ed.Text), w.spans(w.ed.Text)
	if len(a) < 2 || &a[0] != &b[0] {
		t.Fatal("the same text at the same width was wrapped again")
	}
	w.Layout(ggui.Loose(ggui.Sz(400, ggui.Unbounded)), ggui.Env{})
	if c := w.spans(w.ed.Text); len(c) != 1 {
		t.Fatalf("after widening, %d lines; want the cache to follow the width", len(c))
	}
}

func TestTextInputComposesThenCommits(t *testing.T) {
	t.Parallel()
	value := ggui.State("ab")
	w, p := focusedInput(t, value)
	p.Type(ggui.Mods{}, ggui.KeyArrowLeft)
	f := w.ime.(*fakeIME)
	f.compose = "ㅎ"
	p.Type(ggui.Mods{}, ggui.KeyBackspace)
	// The IME swallowed the key: nothing was deleted.
	if ggui.Untrack(value.Get) != "ab" || w.composition != "ㅎ" {
		t.Fatalf("value = %q composition = %q; the IME should have swallowed the key", ggui.Untrack(value.Get), w.composition)
	}
	if shown, caret := w.rendered(); shown != "aㅎb" || caret != 1+len("ㅎ") {
		t.Fatalf("rendered = %q caret %d", shown, caret)
	}
	f.compose, f.commit = "", "한"
	p.Move(ggui.Pt(5, 5)) // the IME runs on the frame
	if ggui.Untrack(value.Get) != "a한b" || w.composition != "" || w.ed.Caret != 1+len("한") {
		t.Fatalf("value = %q composition = %q caret %d after commit", ggui.Untrack(value.Get), w.composition, w.ed.Caret)
	}
	// Moving the caret confirms whatever is being composed: the IME went
	// idle with "ㄱ" still shown, and End commits it before moving.
	f.compose = "ㄱ"
	p.Move(ggui.Pt(5, 5))
	f.compose = ""
	p.Type(ggui.Mods{}, ggui.KeyEnd)
	if ggui.Untrack(value.Get) != "a한ㄱb" || f.confirmed == 0 {
		t.Fatalf("value = %q, confirmed %d times", ggui.Untrack(value.Get), f.confirmed)
	}
}

func TestTextInputSurvivesRebuildWithCaret(t *testing.T) {
	t.Parallel()
	useFakeIME(t)
	value := ggui.State("abc")
	build := func() ggui.Widget { return TextInput(value) }
	// The editors are built outside the probe's tree, which holds whichever
	// is current: the second takes over from the first by its Rect.
	current := build()
	p := ggui.NewProbe(ggui.FromFuncs(
		func(c ggui.Constraints, env ggui.Env) ggui.Size { return current.Layout(c, env) },
		func(dst *ggui.Canvas, r ggui.Rect) { current.Paint(dst, r) },
	), ggui.Sz(200, 20))
	defer p.Close()
	p.Click(ggui.Pt(100, 10))
	p.Type(ggui.Mods{}, ggui.KeyArrowLeft)
	second := build().(*TextInputWidget)
	current = second
	p.Frame()
	p.Type(ggui.Mods{}, ggui.KeyBackspace)
	if !second.Focused() || ggui.Untrack(value.Get) != "ac" {
		t.Fatalf("rebuilt input focused = %v, value = %q; want focus and caret carried over", second.Focused(), ggui.Untrack(value.Get))
	}
}

// An editor rebuilt while the IME composes, as one whose own binding's
// write rebuilds the form around it, carries the composition on rather
// than confirming it: a Korean IME commits each syllable as the next one
// starts, and a rebuild that confirmed the next would split it into its
// letters.
func TestTextInputRebuiltWhileComposingKeepsComposing(t *testing.T) {
	t.Parallel()
	useFakeIME(t)
	value := ggui.State("")
	build := func() ggui.Widget { return TextInput(value) }
	first := build().(*TextInputWidget)
	var current ggui.Widget = first
	p := ggui.NewProbe(ggui.FromFuncs(
		func(c ggui.Constraints, env ggui.Env) ggui.Size { return current.Layout(c, env) },
		func(dst *ggui.Canvas, r ggui.Rect) { current.Paint(dst, r) },
	), ggui.Sz(200, 20))
	defer p.Close()
	p.Click(ggui.Pt(100, 10))
	f := first.ime.(*fakeIME)
	tick := func() { p.Move(ggui.Pt(100, 10)); p.Frame() } // the IME runs on the input, the paint after it
	// 한 committed as ㄱ starts the next syllable.
	f.commit = "한"
	tick()
	f.compose = "ㄱ"
	tick()
	if ggui.Untrack(value.Get) != "한" || first.composition != "ㄱ" {
		t.Fatalf("value = %q composition = %q before the rebuild", ggui.Untrack(value.Get), first.composition)
	}
	confirmed := f.confirmed
	second := build().(*TextInputWidget)
	current = second
	tick()
	if f.confirmed != confirmed || ggui.Untrack(value.Get) != "한" || second.composition != "ㄱ" {
		t.Fatalf("after the rebuild: confirmed %d times, value = %q, composition = %q; want ㄱ still composing",
			f.confirmed-confirmed, ggui.Untrack(value.Get), second.composition)
	}
	// The IME composes on in the rebuilt editor, and commits there.
	f.compose = "글"
	tick()
	f.compose, f.commit = "", "글"
	tick()
	if got := ggui.Untrack(value.Get); got != "한글" || second.composition != "" || second.ed.Caret != len("한글") {
		t.Fatalf("value = %q composition = %q caret %d, want 한글 with the caret after it", got, second.composition, second.ed.Caret)
	}
	// The editor left behind keeps nothing of the IME's.
	if first.composition != "" {
		t.Fatalf("the replaced editor still shows %q", first.composition)
	}
}

func TestTextInputPasswordHidesText(t *testing.T) {
	t.Parallel()
	useFakeIME(t)
	w := TextInput(ggui.State("abc")).Password()
	w.Layout(ggui.Loose(ggui.Sz(200, 20)), ggui.Env{})
	if w.display("abc") != "•••" {
		t.Fatalf("display = %q", w.display("abc"))
	}
	if w.advance("abc") != w.advance("xyz") {
		t.Fatal("masked strings of equal length measure differently")
	}
}

func TestTextInputFillsBoundedWidth(t *testing.T) {
	t.Parallel()
	useFakeIME(t)
	w := TextInput(ggui.State(""))
	if got := w.Layout(ggui.Loose(ggui.Sz(300, 100)), ggui.Env{}); got.W != 300 {
		t.Fatalf("width = %v, want 300", got.W)
	}
	if got := w.Layout(ggui.Loose(ggui.Sz(ggui.Unbounded, 100)), ggui.Env{}); got.W != 120 {
		t.Fatalf("unbounded width = %v, want the 120 minimum", got.W)
	}
}

func TestTextInputMultilineWrapsGrowsAndNavigates(t *testing.T) {
	t.Parallel()
	useFakeIME(t)
	w := TextInput(ggui.State("")).Multiline()
	one := w.Layout(ggui.Loose(ggui.Sz(200, ggui.Unbounded)), ggui.Env{})
	if one.H != w.linesHeight(3) || one.W != 200 {
		t.Fatalf("empty multiline is %v, want 3 lines tall and 200 wide", one)
	}
	long := "the quick brown fox jumps over the lazy dog and keeps on running far away"
	w.value.Set(long)
	reactive.Flush() // the editor follows its binding through an effect
	grown := w.Layout(ggui.Loose(ggui.Sz(200, ggui.Unbounded)), ggui.Env{})
	spans := w.spans(long)
	if len(spans) < 3 || grown.H != w.linesHeight(len(spans)) {
		t.Fatalf("wrapped into %d lines at height %v", len(spans), grown.H)
	}
	if fixed := w.Layout(ggui.Tight(ggui.Sz(200, 30)), ggui.Env{}); fixed.H != 30 {
		t.Fatalf("a tight height was not honoured: %v", fixed)
	}

	p := ggui.NewProbe(w, ggui.Sz(200, grown.H))
	defer p.Close()
	p.Click(ggui.Pt(1, 5))
	if !w.Focused() || w.ed.Caret != 0 {
		t.Fatalf("click at the top left: focused %v caret %d", w.Focused(), w.ed.Caret)
	}
	p.Type(ggui.Mods{}, ggui.KeyEnd)
	if w.ed.Caret != spans[0].End {
		t.Fatalf("End went to %d, want the end of the first line %d", w.ed.Caret, spans[0].End)
	}
	p.Type(ggui.Mods{}, ggui.KeyArrowDown)
	if got := textlayout.LineOf(spans, w.ed.Caret); got != 1 {
		t.Fatalf("Down landed on line %d", got)
	}
	p.Type(ggui.Mods{}, ggui.KeyHome)
	if w.ed.Caret != spans[1].Start {
		t.Fatalf("Home on line 2 went to %d, want %d", w.ed.Caret, spans[1].Start)
	}
	p.Type(ggui.Mods{}, ggui.KeyEnter)
	if got := ggui.Untrack(w.value.Get); got[spans[1].Start] != '\n' {
		t.Fatalf("Enter did not insert a line break: %q", got)
	}
	submitted := ""
	w.OnSubmit(func(s string) { submitted = s })
	p.Key("cmd+enter")
	if submitted == "" {
		t.Fatal("Cmd+Enter did not submit")
	}
	// A click on the second line lands there.
	p.Frame()
	p.Press(ggui.Pt(5, w.spacing()+2))
	if got := textlayout.LineOf(w.spans(w.ed.Text), w.ed.Caret); got != 1 {
		t.Fatalf("click on the second line put the caret on line %d", got)
	}
}

func TestUndoKeys(t *testing.T) {
	t.Parallel()
	v := ggui.State("hello")
	in := TextInput(v)
	p := ggui.NewProbe(in, ggui.Sz(200, 30))
	defer p.Close()
	p.Click(ggui.Pt(190, 10))
	p.Type(ggui.Mods{}, ggui.KeyBackspace, ggui.KeyBackspace)
	if ggui.Untrack(v.Get) != "hel" {
		t.Fatalf("value %q after two backspaces", ggui.Untrack(v.Get))
	}
	p.Key("cmd+z")
	if ggui.Untrack(v.Get) != "hell" {
		t.Fatalf("value %q after undo, want hell", ggui.Untrack(v.Get))
	}
	p.Key("cmd+shift+z")
	if ggui.Untrack(v.Get) != "hel" {
		t.Fatalf("value %q after redo, want hel", ggui.Untrack(v.Get))
	}
}

func TestTextInputKeyHookLeavesCompositionWithIME(t *testing.T) {
	t.Parallel()
	useFakeIME(t)
	calls := 0
	w := TextInput(ggui.State("")).OnKey(func(ev ggui.KeyEvent) bool { calls++; return ev.Key == ggui.KeyArrowDown })
	w.HandleKey(ggui.KeyEvent{Kind: ggui.KeyPress, Key: ggui.KeyArrowDown})
	if calls != 1 {
		t.Fatal("key hook did not receive navigation")
	}
	w.imeComposition("ㅎ", len("ㅎ"))
	w.HandleKey(ggui.KeyEvent{Kind: ggui.KeyPress, Key: ggui.KeyArrowDown})
	if calls != 1 {
		t.Fatal("hook intercepted an IME composition")
	}
	w.HandleKey(ggui.KeyEvent{Kind: ggui.KeyPress, Key: ggui.KeyEscape})
	if calls != 1 || !w.ConsumesKey(ggui.KeyEvent{Kind: ggui.KeyPress, Key: ggui.KeyEscape}) {
		t.Fatal("composition Escape escaped to a popup")
	}
	w.HandleKey(ggui.KeyEvent{Kind: ggui.KeyPress, Key: ggui.KeyEscape})
	if calls != 2 || w.ConsumesKey(ggui.KeyEvent{Kind: ggui.KeyPress, Key: ggui.KeyEscape}) {
		t.Fatal("ordinary Escape was not released")
	}
}

func TestNoWrapBreaksOnlyAtLineBreaks(t *testing.T) {
	useFakeIME(t)
	w := TextInput(ggui.State("a long first line that would wrap\nsecond")).NoWrap()
	if got := w.spans(w.ed.Text); len(got) != 2 || got[1] != (textlayout.Span{Start: 34, End: 40}) {
		t.Fatalf("spans = %v, want the two lines as written", got)
	}
	paintFrame(w, ggui.Sz(60, 60))
	w.ed.MoveTo(36, false)
	w.HandleKey(ggui.KeyEvent{Kind: ggui.KeyPress, Key: ggui.KeyArrowUp})
	if w.ed.Caret > 33 {
		t.Fatalf("Up left the caret at %d, not on the first line", w.ed.Caret)
	}
}

func TestReplaceIsOneUndoStep(t *testing.T) {
	v := ggui.State("ab")
	w, _ := focusedInput(t, v)
	w.Select(1, 1)
	w.Replace("\n    ")
	if got := ggui.Peek(v); got != "a\n    b" {
		t.Fatalf("value = %q", got)
	}
	w.HandleKey(ggui.KeyEvent{Kind: ggui.KeyPress, Key: ggui.KeyZ, Mods: cmd})
	if got := ggui.Peek(v); got != "ab" {
		t.Fatalf("after undo value = %q, want ab", got)
	}
}

func TestTheComposedTextIsBuiltOncePerChange(t *testing.T) {
	t.Parallel()
	value := ggui.State("ab")
	w, p := focusedInput(t, value)
	p.Type(ggui.Mods{}, ggui.KeyArrowLeft)
	f := w.ime.(*fakeIME)
	f.compose = "ㅎ"
	w.HandleTick()
	a, _ := w.rendered()
	b := w.EditingState().Text
	if a != "aㅎb" || unsafe.StringData(a) != unsafe.StringData(b) {
		t.Fatalf("rendered %q, then %q built again", a, b)
	}
	f.compose = "하"
	w.HandleTick()
	if c, _ := w.rendered(); c != "a하b" {
		t.Fatalf("after the composition changed, rendered %q", c)
	}
}

func TestNoWrapLayoutCountsLinesWithoutTheirTable(t *testing.T) {
	useFakeIME(t)
	w := TextInput(ggui.State("1\n2\n3\n4\n5\n6")).NoWrap()
	one := TextInput(ggui.State("one")).NoWrap()
	h3 := w.Layout(ggui.Loose(ggui.Sz(200, ggui.Unbounded)), ggui.Env{}).H
	h1 := one.Layout(ggui.Loose(ggui.Sz(200, ggui.Unbounded)), ggui.Env{}).H
	if h3 <= h1 || w.layout.Last() != nil {
		t.Fatalf("six lines %v tall against one's %v; line table made: %v", h3, h1, w.layout.Last() != nil)
	}
}

// Enter over a syllable being composed reaches OnKey after the syllable is
// in the text when the IME passes the key on, as a Korean one does, so a
// field that sends on Enter sends the whole word; an IME that keeps the key
// to confirm its conversion, as a Japanese one does, sends nothing.
func TestEnterThatEndsACompositionSeesItsCommit(t *testing.T) {
	t.Parallel()
	for _, passthrough := range []bool{true, false} {
		value := ggui.State("안녕")
		w, p := focusedInput(t, value)
		var sent []string
		w.OnKey(func(ev ggui.KeyEvent) bool {
			if ev.Key == ggui.KeyEnter {
				sent = append(sent, ggui.Untrack(value.Get))
				return true
			}
			return false
		})
		f := w.ime.(*fakeIME)
		f.compose = "하"
		p.Move(ggui.Pt(5, 5)) // the IME runs on the frame
		f.compose, f.commit, f.passthrough = "", "하", passthrough
		p.Type(ggui.Mods{}, ggui.KeyEnter)
		want := []string{"안녕하"}
		if !passthrough {
			want = nil
		}
		if !slices.Equal(sent, want) || ggui.Untrack(value.Get) != "안녕하" {
			t.Errorf("passthrough %v: sent %q, value %q; want %q", passthrough, sent, ggui.Untrack(value.Get), want)
		}
	}
}

// A write from outside leaves a caret that was at the end at the new end,
// so text appended or swapped in is typed after; one inside the text stays.
func TestOutsideWritesKeepACaretAtTheEnd(t *testing.T) {
	t.Parallel()
	value := ggui.State("")
	w, _ := focusedInput(t, value)
	value.Set("see /tmp/a.png")
	reactive.Flush()
	if w.ed.Caret != len("see /tmp/a.png") {
		t.Fatalf("caret %d after a write into an empty field, want the end", w.ed.Caret)
	}
	w.ed.MoveTo(3, false)
	value.Set("see /tmp/a.png and b")
	reactive.Flush()
	if w.ed.Caret != 3 {
		t.Fatalf("caret %d, want it left inside the text", w.ed.Caret)
	}
}

// A paste of a clipboard holding an image and no text goes to
// OnPasteImage; one with text pastes the text.
func TestPasteOfAnImageGoesToItsHook(t *testing.T) {
	useFakeIME(t)
	value := ggui.State("")
	var got []byte
	p := ggui.NewProbe(TextInput(value).OnPasteImage(func(png []byte) bool { got = png; return true }), ggui.Sz(200, 20))
	defer p.Close()
	var clip *runtime.MemoryClipboard
	p.OnFrame(func() { clip, _ = ggui.CurrentClipboard().(*runtime.MemoryClipboard) })
	p.Click(ggui.Pt(10, 10))
	paste := cmd
	clip.WriteImage([]byte("png"))
	p.Type(paste, ggui.KeyV)
	if string(got) != "png" || ggui.Untrack(value.Get) != "" {
		t.Fatalf("hook got %q, value %q", got, ggui.Untrack(value.Get))
	}
	clip.Write("text")
	p.Type(paste, ggui.KeyV)
	if ggui.Untrack(value.Get) != "text" {
		t.Fatalf("value %q after pasting text", ggui.Untrack(value.Get))
	}
}

// A key a text input claims is its own while it claims it: Tab reaches
// its OnKey rather than moving the focus, and moves it again once the
// input lets it go.
func TestTextInputClaimsTheKeysItIsAsked(t *testing.T) {
	t.Parallel()
	claim := true
	seen := 0
	a := TextInput(ggui.State("")).Name("a").
		ClaimKeys(func(ev ggui.KeyEvent) bool { return claim && ev.Key == ggui.KeyTab }).
		OnKey(func(ev ggui.KeyEvent) bool { seen++; return ev.Key == ggui.KeyTab })
	b := TextInput(ggui.State("")).Name("b")
	p := ggui.NewProbe(Column(a, b), ggui.Sz(200, 100))
	defer p.Close()
	p.Frame()
	field, _ := p.Semantics().Find(ggui.RoleTextField, "a")
	p.Click(field.Rect.Center())
	focused := func() string { n, _ := p.Semantics().Focused(); return n.Name }
	p.Type(ggui.Mods{}, ggui.KeyTab)
	if seen != 1 || focused() != "a" {
		t.Fatalf("a claimed Tab: OnKey saw %d keys, %q focused", seen, focused())
	}
	claim = false
	p.Type(ggui.Mods{}, ggui.KeyTab)
	if seen != 1 || focused() != "b" {
		t.Fatalf("a let Tab go: OnKey saw %d keys, %q focused", seen, focused())
	}
}

func TestTextInputFilterNativeCommitsAndCaret(t *testing.T) {
	t.Parallel()
	v := ggui.State("")
	e := TextInput(v).Filter(func(s string) string {
		return strings.Map(func(r rune) rune {
			if unicode.IsDigit(r) {
				return r
			}
			return -1
		}, s)
	})
	e.imeCommit("1a2-3")
	if v.Get() != "123" || e.ed.Caret != 3 {
		t.Fatalf("filtered commit %q caret %d", v.Get(), e.ed.Caret)
	}
	e.Select(1, 2)
	e.imeCommit("x9")
	if v.Get() != "193" || e.ed.Caret != 2 {
		t.Fatal("replacement")
	}
	e.imeStart, e.imeEnd = 0, 3
	e.imeReplace("1", "z8", "3")
	if v.Get() != "183" || e.ed.Caret != 2 {
		t.Fatal("IME replace")
	}
	e.Layout(ggui.Loose(ggui.Sz(200, 40)), ggui.Env{})
	e.PaintCustom(nil, ggui.Rct(ggui.Point{}, ggui.Sz(200, 40)), func(_ *ggui.Canvas, _ ggui.Rect, s TextInputState) ggui.Rect {
		if s.Text != "183" {
			t.Fatal(s)
		}
		return ggui.Rct(ggui.Pt(32, 8), ggui.Sz(1, 16))
	})
	if e.caretBounds.Min.X != 32 {
		t.Fatal("custom IME caret")
	}
}

// Native window APIs consume logical coordinates even on HiDPI displays.
func TestTextInputIMECaretUsesLogicalCoordinates(t *testing.T) {
	t.Parallel()
	for _, scale := range []float64{1, 1.5, 2} {
		for _, multiline := range []bool{false, true} {
			e := TextInput(ggui.State(""))
			e.multiline = multiline
			// A probe at the display's scale paints e at 32, 8, or, once
			// custom, through PaintCustom.
			custom := false
			p := ggui.NewProbe(ggui.FromFuncs(
				func(c ggui.Constraints, env ggui.Env) ggui.Size {
					e.Layout(ggui.Loose(ggui.Sz(200, 80)), env)
					return c.Constrain(ggui.Sz(300, 100))
				},
				func(dst *ggui.Canvas, _ ggui.Rect) {
					if !custom {
						e.Paint(dst, ggui.Rct(ggui.Pt(32, 8), ggui.Sz(200, 80)))
						return
					}
					e.PaintCustom(dst, ggui.Rct(ggui.Point{}, ggui.Sz(200, 80)), func(*ggui.Canvas, ggui.Rect, TextInputState) ggui.Rect {
						return ggui.Rct(ggui.Pt(32, 8), ggui.Sz(1, 16))
					})
				},
			), ggui.Sz(300, 100))
			p.SetScale(scale)
			p.Frame()
			if e.scale != scale {
				t.Fatalf("painted at scale %g, want %g", e.scale, scale)
			}
			want := image.Rect(32, 8, 33, int(8+e.height()))
			if got, _, _ := e.imeSession(); got != want {
				t.Fatalf("scale %g multiline %v: got %v, want %v", scale, multiline, got, want)
			}
			custom = true
			p.Frame()
			if got, _, _ := e.imeSession(); got != image.Rect(32, 8, 33, 24) {
				t.Fatalf("scale %g custom: got %v", scale, got)
			}
			p.Close()
		}
	}
}

func TestTextInputFractionalScaleCaretAndHitTestConsistency(t *testing.T) {
	t.Parallel()
	const sample = "The quick brown fox jumps over the lazy dog 0123456789"
	font := ggui.MustFont(goregular.TTF).NoFallback()

	for _, scale := range []float64{1, 1.25, 1.5, 1.75, 2} {
		for _, size := range []float64{13, 14} {
			// Single-line: advance, prefixWidths, hit-testing and painted glyphs agree.
			val := ggui.State(sample)
			e := TextInput(val).Style(ggui.TextStyle{Font: font, Size: size})
			p := ggui.NewProbe(e, ggui.Sz(800, 40))
			p.SetScale(scale)
			img := ggfx.NewImage(int(math.Ceil(800*scale)), int(math.Ceil(40*scale)))
			p.RenderTo(img)
			p.Frame()

			face := e.face(scale)
			wantTotal := textlayout.Width(sample, face) / scale
			if got := e.advance(sample); math.Abs(got-wantTotal) > 1e-9 {
				t.Fatalf("scale %g size %g: advance=%v, want painted width %v", scale, size, got, wantTotal)
			}

			// Every glyph's painted logical OriginX matches prefixWidths and hit-tests back to its offset.
			sp := textlayout.Span{Start: 0, End: len(sample)}
			pw := e.prefixWidths(sp)
			glyphs := text.AppendLazyGlyphs(nil, sample, face, nil)
			for _, g := range glyphs {
				paintedX := g.OriginX / scale
				if got := pw(g.StartIndexInBytes); math.Abs(got-paintedX) > 1e-9 {
					t.Fatalf("scale %g size %g byte %d: prefixWidth=%v, painted OriginX/scale=%v",
						scale, size, g.StartIndexInBytes, got, paintedX)
				}
				if hit := e.indexAt(ggui.Pt(e.rect.Origin.X+paintedX, e.rect.Origin.Y+5)); hit != g.StartIndexInBytes {
					t.Fatalf("scale %g size %g byte %d: indexAt(%v)=%d, want %d",
						scale, size, g.StartIndexInBytes, paintedX, hit, g.StartIndexInBytes)
				}
			}

			// End-of-line caret sits at the painted total width.
			if wantEnd := int(e.rect.Origin.X + wantTotal); e.caretBounds.Min.X != wantEnd {
				t.Fatalf("scale %g size %g: caretBounds.Min.X=%d, want %d", scale, size, e.caretBounds.Min.X, wantEnd)
			}

			// Accessibility runs match painted stops.
			runs := e.runs()
			if len(runs) != 1 || math.Abs(runs[0].Rect.Size.W-wantTotal) > 1e-9 {
				t.Fatalf("scale %g size %g: runs=%+v, want width %v", scale, size, runs, wantTotal)
			}

			img.Deallocate()
			p.Close()

			// Multiline wrapping: every wrapped line fits within e.width at scale.
			const wrapW = 160.0
			mval := ggui.State(sample + " " + sample)
			me := TextInput(mval).Multiline().Style(ggui.TextStyle{Font: font, Size: size})
			mp := ggui.NewProbe(me, ggui.Sz(wrapW, 200))
			mp.SetScale(scale)
			mimg := ggfx.NewImage(int(math.Ceil(wrapW*scale)), int(math.Ceil(200*scale)))
			mp.RenderTo(mimg)
			mp.Frame()

			mface := me.face(scale)
			w := me.wrap(me.ed.Text)
			if len(w.Spans) < 2 {
				t.Fatalf("scale %g size %g: expected multiline wrap, got %d spans", scale, size, len(w.Spans))
			}
			for i, lineSpan := range w.Spans {
				line := me.ed.Text[lineSpan.Start:lineSpan.End]
				paintedW := textlayout.Width(line, mface) / scale
				if paintedW > wrapW+1e-9 {
					t.Fatalf("scale %g size %g span %d %q: painted width %v exceeds wrapW %v",
						scale, size, i, line, paintedW, wrapW)
				}
				if got := me.xAt(w, i, lineSpan.End); math.Abs(got-paintedW) > 1e-9 {
					t.Fatalf("scale %g size %g span %d: xAt(end)=%v, want %v", scale, size, i, got, paintedW)
				}
			}

			mimg.Deallocate()
			mp.Close()
		}

		// Bidi text in TextInput measures, hit-tests, and paints cleanly at fractional scales.
		bval := ggui.State("hello שלום world")
		be := TextInput(bval).Style(ggui.TextStyle{Size: 13})
		bp := ggui.NewProbe(be, ggui.Sz(400, 40))
		bp.SetScale(scale)
		bimg := ggfx.NewImage(int(math.Ceil(400*scale)), int(math.Ceil(40*scale)))
		bp.RenderTo(bimg)
		bp.Frame()
		bw := be.wrap(be.ed.Text)
		bl := be.bidiAt(bw, 0)
		if bl == nil {
			t.Fatalf("scale %g: expected bidi layout", scale)
		}
		if got, want := be.lineWidth(be.ed.Text, bw.Spans[0], bl), bl.Width()/scale; math.Abs(got-want) > 1e-9 {
			t.Fatalf("scale %g: bidi lineWidth=%v, want %v", scale, got, want)
		}
		// Clicking past the right edge reaches the visual end without out-of-range offsets.
		at := be.indexAt(ggui.Pt(be.rect.Origin.X+bl.Width()/scale+10, be.rect.Origin.Y+5))
		if at < 0 || at > len(be.ed.Text) {
			t.Fatalf("scale %g: bidi indexAt returned out-of-range offset %d", scale, at)
		}
		bimg.Deallocate()
		bp.Close()
	}
}

func TestTextInputIsFoundByPlaceholderThenName(t *testing.T) {
	t.Parallel()
	useFakeIME(t)
	w := TextInput(ggui.State("")).Placeholder("Search")
	p := ggui.NewProbe(w, ggui.Sz(200, 30))
	defer p.Close()
	if _, ok := p.FindRole(ggui.RoleTextField, "Search"); !ok {
		t.Fatal("an unnamed field is not found by its placeholder")
	}
	w.Name("Query")
	if _, ok := p.FindRole(ggui.RoleTextField, "Query"); !ok {
		t.Fatal("a named field is not found by its name")
	}
	name := ggui.State("First")
	w.BindName(name)
	if _, ok := p.FindRole(ggui.RoleTextField, "First"); !ok {
		t.Fatal("a field is not found by its bound name")
	}
	name.Set("Second")
	if _, ok := p.FindRole(ggui.RoleTextField, "Second"); !ok || !w.HasName() {
		t.Fatal("the field's name did not follow its binding")
	}
}

func TestTextInputBindDisabledStopsInputAndActions(t *testing.T) {
	t.Parallel()
	useFakeIME(t)
	value, disabled := ggui.State("abc"), ggui.State(true)
	w := TextInput(value).BindDisabled(disabled)
	p := ggui.NewProbe(w, ggui.Sz(200, 30))
	defer p.Close()
	p.Click(ggui.Pt(195, 5))
	p.Type(ggui.Mods{}, ggui.KeyBackspace)
	if w.Focused() || ggui.Untrack(value.Get) != "abc" {
		t.Fatalf("a disabled field focused %v, value %q; want no focus and no edit", w.Focused(), ggui.Untrack(value.Get))
	}
	node, ok := p.Semantics().Find(ggui.RoleTextField, "")
	if !ok || !node.Disabled {
		t.Fatalf("a disabled field is described as %+v, want Disabled", node.Node)
	}
	p.PerformAction(node.ID, ggui.Action{Kind: ggui.ActionSetValue, Text: "set"})
	if got := ggui.Untrack(value.Get); got != "abc" {
		t.Fatalf("an assistive action set a disabled field to %q", got)
	}

	disabled.Set(false)
	p.Click(ggui.Pt(195, 5))
	p.Type(ggui.Mods{}, ggui.KeyBackspace)
	if got := ggui.Untrack(value.Get); !w.Focused() || got != "ab" {
		t.Fatalf("re-enabled field: focused %v, value %q; want focus and \"ab\"", w.Focused(), got)
	}
	node, _ = p.Semantics().Find(ggui.RoleTextField, "")
	p.PerformAction(node.ID, ggui.Action{Kind: ggui.ActionSetValue, Text: "set"})
	if got := ggui.Untrack(value.Get); got != "set" {
		t.Fatalf("ActionSetValue gave %q, want \"set\"", got)
	}
}

func TestTextInputInheritsDisabledFromItsSubtree(t *testing.T) {
	t.Parallel()
	useFakeIME(t)
	value := ggui.State("abc")
	w := TextInput(value)
	p := ggui.NewProbe(ggui.Provide(ggui.InputDisabledKey, true, w), ggui.Sz(200, 30))
	defer p.Close()
	p.Click(ggui.Pt(195, 5))
	p.Type(ggui.Mods{}, ggui.KeyBackspace)
	if !w.IsDisabled() || w.Focused() || ggui.Untrack(value.Get) != "abc" {
		t.Fatalf("under InputDisabledKey: disabled %v, focused %v, value %q", w.IsDisabled(), w.Focused(), ggui.Untrack(value.Get))
	}
}

func TestTextInputSizesFromStyleLinesAndMinWidth(t *testing.T) {
	t.Parallel()
	useFakeIME(t)
	free := ggui.Loose(ggui.Sz(ggui.Unbounded, ggui.Unbounded))
	plain := TextInput(ggui.State(""))
	one := plain.Layout(free, ggui.Env{})
	if got := TextInput(ggui.State("")).MinWidth(50).Layout(free, ggui.Env{}); got.W != 50 {
		t.Fatalf("MinWidth(50) asks for %v wide with the width left to it", got.W)
	}
	big := TextInput(ggui.State("")).Style(ggui.TextStyle{Size: 2 * ggui.DefaultTextSize})
	if got := big.Layout(free, ggui.Env{}); got.H <= one.H*1.5 {
		t.Fatalf("a field at twice the text size is %v tall, against %v", got.H, one.H)
	}
	two := TextInput(ggui.State("")).Lines(2)
	if got := two.Layout(ggui.Loose(ggui.Sz(200, ggui.Unbounded)), ggui.Env{}); !two.multiline || got.H != two.linesHeight(2) {
		t.Fatalf("Lines(2) is %v tall (multiline %v), want two lines, %v", got.H, two.multiline, two.linesHeight(2))
	}
	least := TextInput(ggui.State("")).Lines(0)
	if got := least.Layout(ggui.Loose(ggui.Sz(200, ggui.Unbounded)), ggui.Env{}); got.H != least.linesHeight(1) {
		t.Fatalf("Lines(0) is %v tall, want at least one line", got.H)
	}
}

func TestTextInputBaselineIsTheFirstLineAscent(t *testing.T) {
	t.Parallel()
	useFakeIME(t)
	w := TextInput(ggui.State("x"))
	if _, ok := w.Baseline(); ok {
		t.Fatal("a field reports a baseline before it has been laid out")
	}
	w.Layout(ggui.Loose(ggui.Sz(200, 30)), ggui.Env{})
	got, ok := w.Baseline()
	if want := w.face(1).Metrics().HAscent; !ok || got != want {
		t.Fatalf("baseline = %v, %v; want the ascent %v", got, ok, want)
	}
	// Beside a box, which sits on its bottom edge, the field's baseline
	// lines up with that edge.
	row := Row(Box().Size(10, 40), w).Align(AlignBaseline)
	row.Layout(ggui.Loose(ggui.Sz(300, 100)), ggui.Env{})
	if b, ok := row.Baseline(); !ok || !closeTo(b, 40) {
		t.Fatalf("row baseline = %v, %v; want the field's, on the box's bottom at 40", b, ok)
	}
}

func TestTextInputKeyKeepsCaretAndFocusWhenMoved(t *testing.T) {
	t.Parallel()
	useFakeIME(t)
	value := ggui.State("hello")
	moved := ggui.State(false)
	var input *TextInputWidget
	p := ggui.ProbeBuilder(func() ggui.Widget {
		return ggui.Reactive(func() ggui.Widget {
			input = TextInput(value).Key("query")
			if moved.Get() {
				return Column(Box().Size(10, 40), input)
			}
			return Column(input)
		})
	}, ggui.Sz(200, 100))
	defer p.Close()
	p.Click(ggui.Pt(195, 5))
	p.Key("left", "left")
	moved.Set(true)
	p.Frame()
	p.Key("backspace")
	if got := ggui.Untrack(value.Get); !input.Focused() || got != "helo" {
		t.Fatalf("the moved field: focused %v, value %q; want focus and the caret carried over (\"helo\")", input.Focused(), got)
	}
}

// editorProbe focuses a TextInput holding text in a probe, with the caret
// at the end, and returns the editor, the probe and the bound value. The
// space below the editor takes no input, so a click there blurs it.
func editorProbe(t *testing.T, text string, configure ...func(*TextInputWidget)) (*TextInputWidget, *ggui.Probe, *ggui.StateValue[string]) {
	t.Helper()
	useFakeIME(t)
	value := ggui.State(text)
	w := TextInput(value)
	for _, fn := range configure {
		fn(w)
	}
	p := ggui.NewProbe(Column(w, Box().Size(300, 100)), ggui.Sz(300, 140))
	t.Cleanup(p.Close)
	p.Click(ggui.Pt(295, 5))
	if !w.Focused() {
		t.Fatal("a click on the editor did not focus it")
	}
	return w, p, value
}

// selection returns what the editor has selected and where its caret is.
func selection(w *TextInputWidget) (string, int) { return w.ed.Selected(), w.ed.Caret }

func TestTextInputArrowsMoveByGraphemeAndWord(t *testing.T) {
	t.Parallel()
	w, p, _ := editorProbe(t, "hello big world")
	steps := []struct {
		chord    string
		selected string
		caret    int
	}{
		{"left", "", 14},
		{"home", "", 0},
		{"end", "", 15},
		{"alt+left", "", 10},          // to the start of "world"
		{"alt+shift+left", "big ", 6}, // extends by a word
		{"right", "", 10},             // a selection collapses to its end
		{"shift+home", "hello big ", 0},
		{"left", "", 0},      // ...or to its start
		{"alt+right", "", 6}, // over "hello" and the space after it
		{"shift+end", "big world", 15},
		{"escape", "", 15}, // drops the selection, keeping the caret
	}
	for _, s := range steps {
		p.Key(s.chord)
		if sel, caret := selection(w); sel != s.selected || caret != s.caret {
			t.Fatalf("after %s: selected %q, caret %d; want %q, %d", s.chord, sel, caret, s.selected, s.caret)
		}
	}
}

func TestTextInputStepsOverWholeGraphemes(t *testing.T) {
	t.Parallel()
	text := "aé👍🏽"
	w, p, value := editorProbe(t, text)
	p.Key("left")
	if w.ed.Caret != len("aé") {
		t.Fatalf("Left from the end put the caret at %d, inside the emoji with its modifier", w.ed.Caret)
	}
	p.Key("left")
	if w.ed.Caret != 1 {
		t.Fatalf("Left put the caret at %d, inside e and its accent", w.ed.Caret)
	}
	p.Key("delete")
	if got := ggui.Untrack(value.Get); got != "a👍🏽" {
		t.Fatalf("Delete left %q, want the accented e gone whole", got)
	}
}

func TestTextInputDeletesByGraphemeOrWord(t *testing.T) {
	t.Parallel()
	_, p, value := editorProbe(t, "one two three")
	steps := []struct{ chord, want string }{
		{"backspace", "one two thre"},
		{"alt+backspace", "one two "},
		{"home", "one two "},
		{"delete", "ne two "},
		{"alt+delete", "two "},
		{"shift+end", "two "},
		{"backspace", ""},
		{"backspace", ""}, // nothing left to delete
	}
	for _, s := range steps {
		p.Key(s.chord)
		if got := ggui.Untrack(value.Get); got != s.want {
			t.Fatalf("after %s: value %q, want %q", s.chord, got, s.want)
		}
	}
}

func TestTextInputEscapeReachesAShortcutUnlessItCancelsComposition(t *testing.T) {
	t.Parallel()
	w, p, value := editorProbe(t, "ab")
	escapes := 0
	p.Shortcut("escape", func() { escapes++ })
	p.Key("escape")
	if escapes != 1 {
		t.Fatalf("Escape with nothing composing ran the shortcut %d times, want 1", escapes)
	}
	f := w.ime.(*fakeIME)
	f.compose = "ㅎ"
	p.Move(ggui.Pt(5, 5)) // the IME runs on the frame
	f.compose = ""        // and goes idle with the composition still shown
	if w.composition != "ㅎ" {
		t.Fatalf("composition = %q, want the IME's", w.composition)
	}
	cancelled := f.cancelled
	p.Key("escape")
	if escapes != 1 || w.composition != "" || f.cancelled != cancelled+1 {
		t.Fatalf("Escape while composing: shortcut ran %d times, composition %q, cancelled %d more; want it to cancel only", escapes, w.composition, f.cancelled-cancelled)
	}
	if got := ggui.Untrack(value.Get); got != "ab" {
		t.Fatalf("a cancelled composition left %q", got)
	}
}

func TestTextInputCommitReplacesTheSelection(t *testing.T) {
	t.Parallel()
	w, p, value := editorProbe(t, "hello world")
	p.Key("alt+shift+left")
	f := w.ime.(*fakeIME)
	f.commit = "there"
	p.Move(ggui.Pt(5, 5))
	if got := ggui.Untrack(value.Get); got != "hello there" || w.ed.Caret != len(got) {
		t.Fatalf("committing over the selection gave %q with the caret at %d", got, w.ed.Caret)
	}
}

func TestTextInputBlurConfirmsCompositionAndCommits(t *testing.T) {
	t.Parallel()
	var committed []string
	w, p, value := editorProbe(t, "a", func(w *TextInputWidget) {
		w.OnCommit(func(s string) { committed = append(committed, s) })
	})
	f := w.ime.(*fakeIME)
	f.compose = "ㄱ"
	p.Move(ggui.Pt(5, 5))
	f.compose = ""
	p.Click(ggui.Pt(150, 100)) // below the editor
	if w.Focused() {
		t.Fatal("a click outside the editor left it focused")
	}
	if got := ggui.Untrack(value.Get); got != "aㄱ" {
		t.Fatalf("blur left %q, want the composition confirmed into the value", got)
	}
	if len(committed) != 1 || committed[0] != "aㄱ" {
		t.Fatalf("OnCommit got %q, want one call with the confirmed value", committed)
	}
	p.Click(ggui.Pt(295, 5))
	p.Key("enter")
	if len(committed) != 2 {
		t.Fatalf("Enter committed %d times in all, want a second OnCommit", len(committed))
	}
}

func TestTextInputSurroundingTextReplacementKeepsTheRest(t *testing.T) {
	t.Parallel()
	useFakeIME(t)
	value := ggui.State("teh cat")
	w := TextInput(value)
	w.ed.MoveTo(len("teh"), false)
	w.imeSession()
	// An autocorrect rewrote the text the session was told about.
	w.imeReplace("the", " cat", "")
	if got := ggui.Untrack(value.Get); got != "the cat" {
		t.Fatalf("replacement gave %q", got)
	}
	if w.ed.Caret != len("the cat") {
		t.Fatalf("caret at %d, want after the replaced text", w.ed.Caret)
	}
}

func TestTextInputTypingUndoesAWordAtATime(t *testing.T) {
	t.Parallel()
	w, p, value := editorProbe(t, "")
	p.Advance(0) // pins the probe's clock, which decides what undoes together
	f := w.ime.(*fakeIME)
	commit := func(s string) {
		f.commit = s
		p.Move(ggui.Pt(5, 5))
	}
	commit("a")
	commit("b")
	p.Advance(time.Second)
	commit("c")
	if got := ggui.Untrack(value.Get); got != "abc" {
		t.Fatalf("typed %q, want abc", got)
	}
	p.Key("cmd+z")
	if got := ggui.Untrack(value.Get); got != "ab" {
		t.Fatalf("undo after a pause gave %q, want ab: c was typed a second later", got)
	}
	p.Key("cmd+z")
	if got := ggui.Untrack(value.Get); got != "" {
		t.Fatalf("undo gave %q, want a and b, typed together, undone at once", got)
	}
	changes := 0
	w.OnChange(func(string) { changes++ })
	p.Key("cmd+z")
	if changes != 0 {
		t.Fatal("undo with nothing left wrote the value")
	}
	// Ctrl+Y redoes except on macOS, where Shift+Cmd+Z does.
	p.Key("cmd+y")
	want := "ab"
	if isDarwin {
		want = ""
	}
	if got := ggui.Untrack(value.Get); got != want {
		t.Fatalf("cmd+y gave %q, want %q on this platform", got, want)
	}
}

func TestTextInputClipboardKeys(t *testing.T) {
	t.Parallel()
	w, p, value := editorProbe(t, "cut me")
	p.Clipboard().Write("before")
	p.Key("cmd+c")
	if got := p.Clipboard().Read(); got != "before" {
		t.Fatalf("copy with nothing selected wrote %q", got)
	}
	p.Key("cmd+x")
	if got := ggui.Untrack(value.Get); got != "cut me" {
		t.Fatalf("cut with nothing selected left %q", got)
	}
	p.Key("alt+shift+left", "cmd+x")
	if got, clip := ggui.Untrack(value.Get), p.Clipboard().Read(); got != "cut " || clip != "me" {
		t.Fatalf("cut gave value %q, clipboard %q; want \"cut \" and \"me\"", got, clip)
	}
	p.Key("home", "cmd+v")
	if got := ggui.Untrack(value.Get); got != "mecut " || w.ed.Caret != 2 {
		t.Fatalf("paste gave %q with the caret at %d, want \"mecut \" at 2", got, w.ed.Caret)
	}
	// A key without the command modifier is not a clipboard shortcut.
	p.Key("alt+v")
	if got := ggui.Untrack(value.Get); got != "mecut " {
		t.Fatalf("Alt+V changed the value to %q", got)
	}
}

func TestTextInputPasswordIsNeverCopied(t *testing.T) {
	t.Parallel()
	_, p, value := editorProbe(t, "hunter2", func(w *TextInputWidget) { w.Password() })
	p.Clipboard().Write("before")
	p.Key("cmd+a", "cmd+c")
	if got := p.Clipboard().Read(); got != "before" {
		t.Fatalf("copying a password put %q on the clipboard", got)
	}
	p.Key("cmd+x")
	if got, clip := ggui.Untrack(value.Get), p.Clipboard().Read(); got != "" || clip != "before" {
		t.Fatalf("cutting a password left value %q, clipboard %q; want it deleted and not copied", got, clip)
	}
}

func TestTextInputMultilinePasteKeepsLineBreaks(t *testing.T) {
	t.Parallel()
	_, p, value := editorProbe(t, "", func(w *TextInputWidget) { w.Multiline() })
	p.Clipboard().Write("one\r\ntwo\rthree\nfour")
	p.Key("cmd+v")
	if got := ggui.Untrack(value.Get); got != "one\ntwo\nthree\nfour" {
		t.Fatalf("paste gave %q, want every CRLF and CR turned into a line feed", got)
	}
	// Meta+Up and Down reach the ends of the text: the macOS form.
	p.Type(ggui.Mods{Meta: true}, ggui.KeyArrowUp)
	if got := ggui.Untrack(value.Get); got != "one\ntwo\nthree\nfour" {
		t.Fatalf("Meta+Up edited the text: %q", got)
	}
}

func TestTextInputTripleClickSelectsAll(t *testing.T) {
	t.Parallel()
	w, p, _ := editorProbe(t, "one two")
	p.Advance(0)
	at := ggui.Pt(w.advance("o"), 5)
	p.Click(at)
	p.Click(at)
	if sel, _ := selection(w); sel != "one" {
		t.Fatalf("double click selected %q, want the word", sel)
	}
	p.Click(at)
	if sel, _ := selection(w); sel != "one two" {
		t.Fatalf("triple click selected %q, want everything", sel)
	}
	// A click after a pause starts over.
	p.Advance(time.Second)
	p.Click(at)
	if sel, caret := selection(w); sel != "" || caret != 1 {
		t.Fatalf("a later click selected %q with the caret at %d, want a caret at 1", sel, caret)
	}
}

// Ctrl jumps words on every platform: off macOS it is also Cmd, but no
// Cmd shortcut uses an arrow or a delete.
func TestTextInputWordKeysWithCtrl(t *testing.T) {
	t.Parallel()
	w, p, value := editorProbe(t, "hello big world")
	p.Type(ggui.Mods{Ctrl: true}, ggui.KeyArrowLeft)
	if w.ed.Caret != len("hello big ") {
		t.Fatalf("Ctrl+Left put the caret at %d, want the start of the last word", w.ed.Caret)
	}
	p.Type(ggui.Mods{Ctrl: true}, ggui.KeyBackspace)
	if got := ggui.Untrack(value.Get); got != "hello world" {
		t.Fatalf("Ctrl+Backspace left %q, want the word before the caret gone", got)
	}
}

func TestPasswordSelectionIsReportedInBullets(t *testing.T) {
	t.Parallel()
	w, _, _ := editorProbe(t, "héllo", func(w *TextInputWidget) { w.Password() })
	w.Act(ggui.Action{Kind: ggui.ActionSetSelection, SelStart: len(bullet), SelEnd: 3 * len(bullet)})
	if lo, hi := w.ed.Selection(); w.ed.Text[lo:hi] != "él" {
		t.Fatalf("selecting bullets 1 to 3 selected %q, want \"él\"", w.ed.Text[lo:hi])
	}
	n := w.Describe()
	if n.Value != "•••••" || n.SelStart != len(bullet) || n.SelEnd != 3*len(bullet) {
		t.Fatalf("described as %q selecting %d-%d, want five bullets selecting %d-%d", n.Value, n.SelStart, n.SelEnd, len(bullet), 3*len(bullet))
	}
}

// Meta+Left and Right move to the ends of the caret's line in a multiline
// field, as Cmd+Left and Right do on macOS, not to the ends of the text.
func TestTextInputMetaArrowsKeepToTheLine(t *testing.T) {
	t.Parallel()
	w, p, value := editorProbe(t, "one\ntwo\nthree", func(w *TextInputWidget) { w.Multiline() })
	w.Select(len("one\ntw"), len("one\ntw"))
	p.Type(ggui.Mods{Meta: true}, ggui.KeyArrowLeft)
	if lo, hi := w.ed.Anchor, w.ed.Caret; lo != len("one\n") || hi != lo {
		t.Fatalf("Meta+Left put the caret at %d..%d, want the start of the second line, %d", lo, hi, len("one\n"))
	}
	p.Type(ggui.Mods{Meta: true, Shift: true}, ggui.KeyArrowRight)
	if sel, _ := selection(w); sel != "two" {
		t.Fatalf("Shift+Meta+Right selected %q, want the line", sel)
	}
	if got := ggui.Untrack(value.Get); got != "one\ntwo\nthree" {
		t.Fatalf("moving edited the text: %q", got)
	}
}

// editorDoc is a text of n lines, some wrapping at the widths the tests
// lay it out at, some blank.
func editorDoc(n int) string {
	words := strings.Fields("the quick brown fox jumps over the lazy dog and keeps on running far away 한글 é")
	var lines []string
	for i := range n {
		if i%7 == 3 {
			lines = append(lines, "")
			continue
		}
		lines = append(lines, strings.Join(words[:1+i%len(words)], " "))
	}
	return strings.Join(lines, "\n")
}

// A multiline editor wraps an edit again line by line, keeping the lines
// it did not change; what it finds must be what wrapping the whole text
// afresh finds, after edits anywhere, edits that split and join lines,
// and a new width or size, which change every line.
func TestTextInputWrapsEditsAsTheWholeText(t *testing.T) {
	t.Parallel()
	useFakeIME(t)
	w := TextInput(ggui.State(editorDoc(60))).Multiline()
	layout := func(width float64) { w.Layout(ggui.Loose(ggui.Sz(width, ggui.Unbounded)), ggui.Env{}) }
	check := func(what string) {
		t.Helper()
		got := w.spans(w.ed.Text)
		if want := textlayout.WrapSpans(w.ed.Text, w.face(1), w.width); !slices.Equal(got, want) {
			t.Fatalf("after %s, the wrap is %v, want %v", what, got, want)
		}
	}
	layout(160)
	check("the first layout")
	edit := func(what string, at func() int, s string) {
		t.Helper()
		w.ed.MoveTo(at(), false)
		w.ed.Replace(s)
		layout(160)
		check(what)
	}
	edit("typing at the start", func() int { return 0 }, "x")
	edit("typing in the middle", func() int { return len(w.ed.Text) / 2 }, " more words to wrap ")
	edit("typing at the end", func() int { return len(w.ed.Text) }, " the end")
	edit("splitting a line", func() int { return len(w.ed.Text) / 3 }, "\n")
	edit("pasting lines", func() int { return len(w.ed.Text) / 4 }, "one\ntwo three four five six seven\n\nfour")
	// Joining lines: delete the line break after the middle.
	w.ed.MoveTo(strings.IndexByte(w.ed.Text[len(w.ed.Text)/2:], '\n')+len(w.ed.Text)/2+1, false)
	w.ed.Backspace(false)
	layout(160)
	check("joining two lines")
	w.ed.SelectAll()
	w.ed.Replace(editorDoc(20))
	layout(160)
	check("replacing the text")
	layout(97)
	check("narrowing")
	w.Style(ggui.TextStyle{Size: 23})
	layout(97)
	check("a new size")
	for i := range 300 { // edits leave lines no wrap uses, which go
		edit("typing", func() int { return 0 }, string(rune('a'+i%26)))
	}
	n, _ := w.layout.Held()
	if lines := strings.Count(w.ed.Text, "\n") + 1; n > 2*lines+64 {
		t.Errorf("%d lines kept for a text of %d", n, lines)
	}
}

// Edits anywhere, of any size, with and without line breaks, wrap as the
// whole text afresh would, wrapped to a width or at line breaks only; so
// does the text with a composition in it, asked for in turn with the text.
func TestTextInputWrapsRandomEditsAsTheWholeText(t *testing.T) {
	t.Parallel()
	useFakeIME(t)
	rng := rand.New(rand.NewPCG(1, 2))
	pieces := []string{"", "x", " ", "\n", "\n\n", "two words", "a line\nand another ", "한글 e\u0301", "שלום", strings.Repeat("long ", 12)}
	for _, noWrap := range []bool{false, true} {
		w := TextInput(ggui.State(editorDoc(25))).Multiline()
		if noWrap {
			w.NoWrap()
		}
		w.Layout(ggui.Loose(ggui.Sz(140, ggui.Unbounded)), ggui.Env{})
		want := func(s string) []textlayout.Span {
			if noWrap {
				return textlayout.BreakSpans(s)
			}
			return textlayout.WrapSpans(s, w.face(1), w.width)
		}
		for i := range 400 {
			lo := w.ed.Snap(rng.IntN(len(w.ed.Text) + 1))
			hi := w.ed.Snap(min(lo+rng.IntN(40), len(w.ed.Text)))
			w.ed.Anchor, w.ed.Caret = lo, hi
			w.ed.Replace(pieces[rng.IntN(len(pieces))])
			if got := w.spans(w.ed.Text); !slices.Equal(got, want(w.ed.Text)) {
				t.Fatalf("noWrap %v, edit %d: %q wraps as %v, want %v", noWrap, i, w.ed.Text, got, want(w.ed.Text))
			}
			if i%10 == 0 {
				w.composition = "조합"
				shown, _ := w.rendered()
				if got := w.spans(shown); !slices.Equal(got, want(shown)) {
					t.Fatalf("noWrap %v, edit %d: composed %q wraps as %v, want %v", noWrap, i, shown, got, want(shown))
				}
				w.composition = ""
			}
		}
	}
}

// While the IME composes, paint wraps the text with the composition in
// it and the rest the text without: each is wrapped once, as the whole
// text would wrap, however often the two are asked for in turn.
func TestTextInputComposingWrapsEachTextOnce(t *testing.T) {
	t.Parallel()
	useFakeIME(t)
	w := TextInput(ggui.State(editorDoc(30))).Multiline()
	w.Layout(ggui.Loose(ggui.Sz(160, ggui.Unbounded)), ggui.Env{})
	w.ed.MoveTo(len(w.ed.Text)/2, false)
	w.composition, w.compCaret = "한글 조합 중인 긴 글자", len("한글")
	shown, _ := w.rendered()
	plain, composed := w.spans(w.ed.Text), w.spans(shown)
	if want := textlayout.WrapSpans(shown, w.face(1), w.width); !slices.Equal(composed, want) {
		t.Fatalf("the composed text wraps as %v, want %v", composed, want)
	}
	for range 3 {
		if a, b := w.spans(w.ed.Text), w.spans(shown); &a[0] != &plain[0] || &b[0] != &composed[0] {
			t.Fatal("asking for the text and the composed text in turn wrapped them again")
		}
	}
}

// A keystroke and the frame after it in a multiline editor of 300 lines:
// the line typed in is wrapped again, not the text. A sentence is typed
// into the middle and the text put back, so the line stays a line's
// length however long the benchmark runs.
func BenchmarkTextInputKeystrokeFrame(b *testing.B) {
	doc := editorDoc(300)
	w := TextInput(ggui.State(doc)).Multiline()
	paintFrame(w, ggui.Sz(300, 400))
	const typed = "a few words typed "
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		if i%len(typed) == 0 {
			w.ed.SetText(doc)
			w.ed.MoveTo(len(doc)/2, false)
		}
		w.ed.Replace(typed[i%len(typed) : i%len(typed)+1])
		i++
		paintFrame(w, ggui.Sz(300, 400))
	}
}

// The same in a text of 10,000 lines: what a keystroke costs beside the
// line it changed, finding which lines those are and keeping the rest.
func BenchmarkTextInputKeystrokeFrameLongText(b *testing.B) {
	doc := editorDoc(10000)
	w := TextInput(ggui.State(doc)).Multiline()
	paintFrame(w, ggui.Sz(300, 400))
	const typed = "a few words typed "
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		if i%len(typed) == 0 {
			w.ed.SetText(doc)
			w.ed.MoveTo(len(doc)/2, false)
		}
		w.ed.Replace(typed[i%len(typed) : i%len(typed)+1])
		i++
		paintFrame(w, ggui.Sz(300, 400))
	}
}

// A frame of the same editor while the IME composes and nothing changes.
func BenchmarkTextInputComposingFrame(b *testing.B) {
	w := TextInput(ggui.State(editorDoc(300))).Multiline()
	paintFrame(w, ggui.Sz(300, 400))
	w.ed.MoveTo(len(w.ed.Text)/2, false)
	w.composition = "ㅎ"
	b.ReportAllocs()
	for b.Loop() {
		paintFrame(w, ggui.Sz(300, 400))
	}
}

// What the screen reader is told is measured again only when the text,
// its wrap or its style change: a scroll or a move only moves it.
func TestTextInputRunsFollowTheTextAndThePaint(t *testing.T) {
	t.Parallel()
	useFakeIME(t)
	w := TextInput(ggui.State(editorDoc(12))).Multiline()
	paintFrame(w, ggui.Sz(160, 80))
	same := func(what string) {
		t.Helper()
		got, want := w.runs(), w.measureRuns()
		for i := range want {
			want[i].Rect.Origin = want[i].Rect.Origin.Add(w.rect.Origin).Sub(ggui.Pt(0, w.scroll))
		}
		if !slices.EqualFunc(got, want, func(a, b ggui.TextRun) bool {
			return a.Start == b.Start && a.End == b.End && a.Rect == b.Rect && slices.Equal(a.Stops, b.Stops)
		}) {
			t.Fatalf("after %s, the runs are %v, want %v", what, got, want)
		}
	}
	measured := func() *ggui.TextRun { w.runs(); return &w.described.measured[0] }
	same("the first paint")
	if a, b := measured(), measured(); a != b {
		t.Fatal("the runs of an unchanged editor were measured again")
	}
	w.ed.MoveTo(0, false)
	w.ed.Replace("new words ")
	same("an edit")
	before := measured()
	w.scroll += 7
	same("a scroll")
	if measured() != before {
		t.Fatal("a scroll measured the runs again")
	}
	paintFrame(w, ggui.Sz(120, 80))
	same("a narrower paint")
}

// Right-to-left text draws right to left, and the caret, a click and the
// selection go where it is drawn: the caret before the first letter of a
// Hebrew word stands at its right, a click at its right puts the caret
// there, and a click at its left after the word. Each holds wherever the
// machine's fonts draw Hebrew, since the order is the characters'.
func TestTextInputPlacesTheCaretInRightToLeftText(t *testing.T) {
	t.Parallel()
	useFakeIME(t)
	for _, multiline := range []bool{false, true} {
		w := TextInput(ggui.State("שלום עולם")).Style(ggui.TextStyle{Size: 30})
		if multiline {
			w.Multiline()
		}
		paintFrame(w, ggui.Sz(600, 50))
		bl := w.bidiAt(w.wrap(w.ed.Text), 0)
		if bl == nil {
			t.Fatal("a Hebrew line was not laid out in visual order")
		}
		total := bl.Width()
		caretAt := func(b int) float64 {
			w.ed.MoveTo(b, false)
			paintFrame(w, ggui.Sz(600, 50))
			return float64(w.caretBounds.Min.X)
		}
		if x := caretAt(0); x < total-2 {
			t.Errorf("multiline %v: the caret at the start stands at %v, want the right end, %v", multiline, x, total)
		}
		if a, b := caretAt(len("ש")), caretAt(len("של")); !(b < a && a < total) {
			t.Errorf("multiline %v: the caret moves right as it goes through the word: %v then %v", multiline, a, b)
		}
		if i := w.indexAt(ggui.Pt(total-1, 10)); i != 0 {
			t.Errorf("multiline %v: a click at the right end puts the caret at %d, want 0", multiline, i)
		}
		if i := w.indexAt(ggui.Pt(1, 10)); i != len(w.ed.Text) {
			t.Errorf("multiline %v: a click at the left end puts the caret at %d, want %d", multiline, i, len(w.ed.Text))
		}
		// The first word selected is the right of the line.
		var xs []float64
		w.spanX(w.ed.Text, textlayout.Span{End: len(w.ed.Text)}, bl, 0, len("שלום"), 0, func(a, b float64) { xs = append(xs, a, b) })
		if len(xs) != 2 || xs[0] < total/3 || xs[1] < total-2 {
			t.Errorf("multiline %v: the first word selected covers %v of %v, want its right end", multiline, xs, total)
		}
	}
}

// An OnDescribe callback amends the node without being handed the runs
// kept for the next description, which it could otherwise change; runs it
// sets itself are the node's.
func TestTextInputDescribeKeepsItsRunsFromTheCallback(t *testing.T) {
	useFakeIME(t)
	a11y.SetWantsDetail(true)
	t.Cleanup(func() { a11y.SetWantsDetail(false) })
	saw := -1
	var own []ggui.TextRun
	w := TextInput(ggui.State("two words")).OnDescribe(func(n *ggui.Node) {
		saw = len(n.Runs)
		if own != nil {
			n.Runs = own
		}
	})
	paintFrame(w, ggui.Sz(200, 30))
	n := w.Describe()
	if saw != 0 || len(n.Runs) == 0 || &n.Runs[0] != &w.runs()[0] {
		t.Fatalf("the callback saw %d runs; the node has %d, kept: %v", saw, len(n.Runs), len(n.Runs) > 0 && &n.Runs[0] == &w.runs()[0])
	}
	own = []ggui.TextRun{{Start: 0, End: 3}}
	if n := w.Describe(); len(n.Runs) != 1 || n.Runs[0].End != 3 {
		t.Fatalf("the callback's own runs became %v", n.Runs)
	}
}

// Typing in a long line keeps no copy of the line per keystroke: a
// single-line field keeps none of its text, and a long paragraph's copies
// go once they add up to more than the text twice over.
func TestTextInputKeepsNoCopyOfALongLinePerKeystroke(t *testing.T) {
	t.Parallel()
	useFakeIME(t)
	long := strings.Repeat("a long line of words ", 500)
	for _, multiline := range []bool{false, true} {
		w := TextInput(ggui.State(long))
		if multiline {
			w.Multiline()
		}
		w.Layout(ggui.Loose(ggui.Sz(300, ggui.Unbounded)), ggui.Env{})
		w.ed.MoveTo(len(long)/2, false)
		for range 300 {
			w.ed.Replace("x")
			w.spans(w.ed.Text)
		}
		if lines, bytes := w.layout.Held(); bytes > 2*len(w.ed.Text)+4096 || !multiline && lines != 0 {
			t.Errorf("multiline %v: %d lines of %d bytes kept for a text of %d", multiline, lines, bytes, len(w.ed.Text))
		}
	}
}

// A frame of a 300-line editor with a screen reader attached, scrolled a
// little each time: the runs move, measured once.
func BenchmarkTextInputScrollFrameWithScreenReader(b *testing.B) {
	a11y.SetWantsDetail(true)
	b.Cleanup(func() { a11y.SetWantsDetail(false) })
	w := TextInput(ggui.State(editorDoc(300))).Multiline()
	paintFrame(w, ggui.Sz(300, 400))
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		w.scroll = float64(i % 500)
		i++
		w.Describe()
	}
}

// A keystroke and the frame after it in a single-line field holding a
// long text.
func BenchmarkTextInputKeystrokeFrameOneLine(b *testing.B) {
	doc := strings.Repeat("a long line of words ", 500)
	w := TextInput(ggui.State(doc))
	paintFrame(w, ggui.Sz(300, 30))
	const typed = "a few words typed "
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		if i%len(typed) == 0 {
			w.ed.SetText(doc)
			w.ed.MoveTo(len(doc)/2, false)
		}
		w.ed.Replace(typed[i%len(typed) : i%len(typed)+1])
		i++
		paintFrame(w, ggui.Sz(300, 30))
	}
}

// A keystroke's wrap learns where the text changed from the editor, by the
// name it gave the text the last wrap was made of, rather than comparing
// the texts; and undo, redo and a text the IME assigned outside the editor
// still wrap as the whole text would.
func TestTextInputWrapsFromTheEditorsNameForTheTextBefore(t *testing.T) {
	t.Parallel()
	useFakeIME(t)
	w := TextInput(ggui.State(editorDoc(80))).Multiline()
	layout := func() { w.Layout(ggui.Loose(ggui.Sz(150, ggui.Unbounded)), ggui.Env{}) }
	check := func(what string) {
		t.Helper()
		layout()
		if got, want := w.spans(w.ed.Text), textlayout.WrapSpans(w.ed.Text, w.face(1), w.width); !slices.Equal(got, want) {
			t.Fatalf("after %s, the wrap is %v, want %v", what, got, want)
		}
	}
	check("the first layout")
	prev := w.layout.Last()
	if prev.Rev() == 0 {
		t.Fatal("the wrap of the editor's text has no name for it")
	}
	w.ed.MoveTo(len(w.ed.Text)/2, false)
	w.ed.Replace("typed")
	at := w.ed.Caret - len("typed")
	if a, cut, ins, ok := w.ed.Changed(prev.Rev(), w.ed.Text); !ok || a != at || cut != 0 || ins != len("typed") {
		t.Fatalf("Changed from the last wrap = %d, %d, %d, %v; want %d, 0, 5, true", a, cut, ins, ok, at)
	}
	check("typing")
	w.ed.Undo()
	check("undo")
	w.ed.Redo()
	check("redo")
	w.imeStart, w.imeEnd = 10, 20
	w.imeReplace("a", "\nb", "c")
	check("an IME commit")
	w.ed.Replace("x")
	check("typing after an IME commit")
}

// The tests below were the kernel's while the editor was, and are the
// editor's: its style, emoji, tracking and accessibility.

// useTestEmoji makes the color emoji font in the module's assets/testdata the
// one the test goroutine's editors draw with.
func useTestEmoji(t *testing.T) {
	t.Helper()
	data, err := os.ReadFile("../assets/testdata/emoji-subset.ttf")
	if err != nil {
		t.Fatal(err)
	}
	ggui.SetEmojiFont(ggui.MustFont(data))
}

// trackingNear reports whether a and b are within a few 26.6 fixed-point steps, the
// grain advances are kept in: one step for each of n clusters.
func trackingNear(a, b float64, n int) bool {
	return math.Abs(a-b) <= float64(n+1)/64
}

func TestCoreStylesWorkWithoutTheme(t *testing.T) {
	t.Parallel()
	env := ggui.Env{}.With(ggui.EditorStyleKey, ggui.EditorStyle{Muted: color.White, Selection: color.Black}).
		With(ggui.ScrollStyleKey, ggui.ScrollStyle{Color: color.White, HoverColor: color.Black}).
		With(ggui.PopupDurationKey, time.Duration(0)).With(ggui.SpacingKey, 0.0)
	input := TextInput(ggui.State("text")).Disabled(true)
	input.Layout(ggui.Loose(ggui.Sz(200, 40)), env)
	if input.resolved.Color != color.White || input.selection != color.Black {
		t.Fatal("editor ignored explicit environment colors")
	}
	if env.ScrollStyle().Duration != 0 || env.PopupDuration() != 0 || env.Spacing() != 0 {
		t.Fatal("zero style settings were replaced by defaults")
	}
}

func TestEmojiPointerCaretAndBackspace(t *testing.T) {
	t.Parallel()
	for _, s := range []string{"👍🏽", "🇰🇷", "👩🏽‍💻", "👨‍👩‍👧‍👦", "1️⃣", "🏳️‍🌈", "❤️"} {
		t.Run(s, func(t *testing.T) {
			// A subtest runs on a goroutine of its own, whose settings are
			// its own.
			useTestEmoji(t)
			useFakeIME(t)
			value := ggui.State(s)
			w := TextInput(value)
			w.Layout(ggui.Tight(ggui.Sz(300, 30)), ggui.Untrack(ggui.UseEnv))
			for x := 0.0; x <= w.advance(s)+5; x += .5 {
				i := w.indexOnLine(w.wrap(s), 0, x)
				if i != 0 && i != len(s) {
					t.Fatalf("click at x=%v placed caret inside emoji at byte %d/%d", x, i, len(s))
				}
			}
			w.rect = ggui.Rct(ggui.Point{}, ggui.Sz(300, 30))
			w.HandlePointer(ggui.PointerEvent{Kind: ggui.PointerDown, Button: ggui.MouseButtonLeft, Pos: ggui.Pt(299, 10)})
			w.HandleKey(ggui.KeyEvent{Kind: ggui.KeyPress, Key: ggui.KeyBackspace})
			if ggui.Untrack(value.Get) != "" {
				t.Fatalf("backspace left emoji fragments: %q", ggui.Untrack(value.Get))
			}
		})
	}
}

func TestATextInputMeasuresItsCaretWithItsTracking(t *testing.T) {
	t.Parallel()
	plain := TextInput(ggui.State("")).Style(ggui.TextStyle{Size: 20})
	tracked := TextInput(ggui.State("")).Style(ggui.TextStyle{Size: 20, Tracking: 0.1})
	if got, want := tracked.advance("abc"), plain.advance("abc")+3*2; !trackingNear(got, want, 3) {
		t.Fatalf("caret after abc at %v, want %v", got, want)
	}
}

// axDetailed runs a frame with the detail a text field only freezes while
// something is reading it, and puts the flag back.
func axDetailed(t *testing.T, p *ggui.Probe) *ggui.SemTree {
	t.Helper()
	a11y.SetWantsDetail(true)
	t.Cleanup(func() { a11y.SetWantsDetail(false) })
	return p.Semantics()
}

func TestAXTextFieldSaysWhereItsCharactersAre(t *testing.T) {
	value := ggui.State("hello world")
	w := TextInput(value)
	p := ggui.NewProbe(w, ggui.Sz(300, 40))
	defer p.Close()

	field, ok := axDetailed(t, p).Find(ggui.RoleTextField, "")
	if !ok {
		t.Fatal("no text field in the tree")
	}
	if got := a11y.CharCount(field.Node); got != 11 {
		t.Errorf("character count = %d, want 11", got)
	}
	if got := a11y.StringForRange(field.Node, 6, 5); got != "world" {
		t.Errorf("string for range = %q, want %q", got, "world")
	}
	if len(field.Runs) != 1 {
		t.Fatalf("%d runs, want one line", len(field.Runs))
	}
	if n := len(field.Runs[0].Stops); n != 12 {
		t.Errorf("%d stops, want one per character boundary (12)", n)
	}
	// The caret sits at the end after TextInput loaded the value, and the
	// field is one line, so that is where the insertion point is.
	if got := a11y.InsertionLine(field.Node); got != 0 {
		t.Errorf("insertion line = %d, want 0", got)
	}
	if loc, length, has := a11y.RangeForLine(field.Node, 0); !has || loc != 0 || length != 11 {
		t.Errorf("line 0 = %d+%d, %v; want the whole field", loc, length, has)
	}
	if _, _, has := a11y.RangeForLine(field.Node, 3); has {
		t.Error("a field with one line answered for line 3")
	}
	// A character's box is inside the field, is not empty, and moves right
	// as the offset does.
	first := a11y.RectForRange(field, 0, 1)
	later := a11y.RectForRange(field, 6, 1)
	if first.Empty() || later.Empty() {
		t.Fatalf("character boxes = %v, %v; want real rectangles", first, later)
	}
	if later.Origin.X <= first.Origin.X {
		t.Errorf("character 6 at x=%g is not right of character 0 at x=%g", later.Origin.X, first.Origin.X)
	}
	if first.Origin.X < field.Full.Origin.X || later.Origin.X > field.Full.Origin.X+field.Full.Size.W {
		t.Errorf("character boxes fell outside the field %v", field.Full)
	}
}

// Not parallel: whether anything is reading is the process's, and the tests
// that turn it on with axDetailed run one at a time beside this one.
func TestAXTextDetailIsOnlyFrozenWhenSomethingIsReading(t *testing.T) {
	p := ggui.NewProbe(TextInput(ggui.State("hello")), ggui.Sz(300, 40))
	defer p.Close()
	field, ok := p.Semantics().Find(ggui.RoleTextField, "")
	if !ok {
		t.Fatal("no text field")
	}
	if field.Runs != nil {
		t.Error("the layout was frozen with nobody reading it; that is a measurement per character per frame")
	}
	// The selection is cheap and always there, so that the caret is known
	// the moment a bridge attaches.
	if field.SelEnd != len("hello") {
		t.Errorf("caret = %d, want the end of the text", field.SelEnd)
	}
	// Without a layout the field is still one line covering everything.
	if loc, length, has := a11y.RangeForLine(field.Node, 0); !has || loc != 0 || length != 5 {
		t.Errorf("line 0 = %d+%d, %v; want the whole field", loc, length, has)
	}
	if got := a11y.RectForRange(field, 0, 1); got != field.Full {
		t.Errorf("rect for range = %v, want the whole field %v", got, field.Full)
	}
}

func TestAXPasswordKeepsItsShapeToItself(t *testing.T) {
	p := ggui.NewProbe(TextInput(ggui.State("hunter2")).Password(), ggui.Sz(300, 40))
	defer p.Close()
	field, _ := axDetailed(t, p).Find(ggui.RoleTextField, "")
	if field.Runs != nil {
		t.Error("a password field froze its character positions")
	}
}

func TestAXSetSelectionMovesTheCaret(t *testing.T) {
	t.Parallel()
	w := TextInput(ggui.State("hello world"))
	p := ggui.NewProbe(w, ggui.Sz(300, 40))
	defer p.Close()
	field, ok := p.Semantics().Find(ggui.RoleTextField, "")
	if !ok {
		t.Fatal("no text field")
	}
	if !field.Actions.Has(ggui.ActionSetSelection) {
		t.Fatal("a text field does not offer its selection")
	}
	start, end := a11y.ByteRange(field.Node, 6, 5)
	p.PerformAction(field.ID, ggui.Action{Kind: ggui.ActionSetSelection, SelStart: start, SelEnd: end})
	after, _ := p.Semantics().Find(ggui.RoleTextField, "")
	if after.SelStart != 6 || after.SelEnd != 11 {
		t.Errorf("selection = %d..%d, want 6..11", after.SelStart, after.SelEnd)
	}
	if got := a11y.Selected(after.Node); got != "world" {
		t.Errorf("selected = %q, want %q", got, "world")
	}
}

func TestAXMultilineFieldReportsItsLines(t *testing.T) {
	w := TextInput(ggui.State("one\ntwo\nthree")).Multiline()
	p := ggui.NewProbe(w, ggui.Sz(300, 100))
	defer p.Close()
	field, ok := axDetailed(t, p).Find(ggui.RoleTextField, "")
	if !ok {
		t.Fatal("no text field")
	}
	if len(field.Runs) != 3 {
		t.Fatalf("%d runs, want three lines:\n%v", len(field.Runs), field.Runs)
	}
	for i, want := range []string{"one", "two", "three"} {
		loc, length, has := a11y.RangeForLine(field.Node, i)
		if !has {
			t.Fatalf("no line %d", i)
		}
		if got := a11y.StringForRange(field.Node, loc, length); got != want {
			t.Errorf("line %d = %q, want %q", i, got, want)
		}
	}
	// A byte on the second line is on line 1, and the lines go down the
	// screen in order.
	if got := a11y.LineForIndex(field.Node, 5); got != 1 {
		t.Errorf("line for offset 5 = %d, want 1", got)
	}
	if field.Runs[1].Rect.Origin.Y <= field.Runs[0].Rect.Origin.Y {
		t.Error("the second line is not below the first")
	}
	// The caret loaded at the end of the text, which is the last line.
	if got := a11y.InsertionLine(field.Node); got != 2 {
		t.Errorf("insertion line = %d, want 2", got)
	}
}

func TestCachedTextInputFollowsSignal(t *testing.T) {
	t.Parallel()
	v := ggui.State("a")
	in := TextInput(v)
	p := ggui.NewProbe(ggui.Cached(in), ggui.Sz(200, 30))
	p.Frame()
	v.Set("bbbbbbbb")
	p.Frame()
	p.Frame()
	if in.ed.Text != "bbbbbbbb" {
		t.Fatalf("editor shows %q", in.ed.Text)
	}
}

// A line is drawn in pieces, the stretches Colors gives in their colours
// and the rest in the style's, the stretches cut to the line, and one
// out of order left out.
func TestColorPiecesCoverTheLine(t *testing.T) {
	red, blue := color.NRGBA{R: 255, A: 255}, color.NRGBA{B: 255, A: 255}
	colors := []TextColor{{Start: 2, End: 6, Color: red}, {Start: 4, End: 5, Color: blue}, {Start: 8, End: 20, Color: blue}}
	got := colorPieces(nil, textlayout.Span{Start: 3, End: 10}, colors)
	want := []TextColor{{Start: 3, End: 6, Color: red}, {Start: 6, End: 8}, {Start: 8, End: 10, Color: blue}}
	if !slices.Equal(got, want) {
		t.Fatalf("pieces %v, want %v", got, want)
	}
	if got := colorPieces(nil, textlayout.Span{Start: 0, End: 4}, nil); !slices.Equal(got, []TextColor{{Start: 0, End: 4}}) {
		t.Fatalf("no colours: %v", got)
	}
}

// A placeholder's lines go on lines of a multiline field; the text drawer
// stacks none of its own, so drawn whole they overlapped on the first.
func TestPlaceholderLines(t *testing.T) {
	for _, c := range []struct {
		in        string
		multiline bool
		want      []string
	}{
		{"Search", false, []string{"Search"}},
		{"Search", true, []string{"Search"}},
		{"8080:80\n127.0.0.1:9000:9000", true, []string{"8080:80", "127.0.0.1:9000:9000"}},
		{"a\r\nb\n", true, []string{"a", "b", ""}},
		{"a\nb", false, []string{"a b"}},
	} {
		if got := placeholderLines(c.in, c.multiline); !slices.Equal(got, c.want) {
			t.Errorf("placeholderLines(%q, %v) = %q, want %q", c.in, c.multiline, got, c.want)
		}
	}
}

func TestTextInputShiftClickIMEReplaceAndClipboardCommit(t *testing.T) {
	t.Parallel()
	// 1. Shift+Click extends selection.
	v := ggui.State("hello world")
	w := TextInput(v)
	p := ggui.NewProbe(w, ggui.Sz(200, 40))
	defer p.Close()
	p.Frame()
	w.Select(0, 0)
	w.HandlePointer(ggui.PointerEvent{
		Kind:   ggui.PointerDown,
		Button: ggui.MouseButtonLeft,
		Pos:    ggui.Pt(w.rect.Origin.X+w.advance("hello"), w.rect.Origin.Y+5),
		Mods:   ggui.Mods{Shift: true},
	})
	if got := w.ed.Selected(); got != "hello" {
		t.Fatalf("Shift+Click selected %q, want \"hello\"", got)
	}

	// 2. imeReplace clamps stale imeStart/imeEnd and records undo.
	w.imeStart, w.imeEnd = -5, 999
	w.imeReplace("hi", "!", "")
	if w.ed.Text != "hi!" || w.ed.Caret != 3 {
		t.Fatalf("imeReplace with out-of-range bounds gave %q caret %d", w.ed.Text, w.ed.Caret)
	}
	if !w.ed.Undo() || w.ed.Text != "hello world" {
		t.Fatalf("Undo after imeReplace gave %q, want \"hello world\"", w.ed.Text)
	}

	// 3. Unmodified A/C/X/V and Cmd+A/Cmd+C do not trigger commit/filter.
	filterCalls := 0
	w.Filter(func(s string) string { filterCalls++; return s })
	w.key(ggui.KeyA, ggui.Mods{})
	w.key(ggui.KeyC, ggui.Mods{})
	w.key(ggui.KeyA, cmd)
	w.key(ggui.KeyC, cmd)
	w.key(ggui.KeyArrowLeft, ggui.Mods{})
	if filterCalls != 0 {
		t.Fatalf("non-mutating keys triggered commit/filter %d times, want 0", filterCalls)
	}

	// 4. Multiline layout grows for IME composition.
	mv := ggui.State("short")
	mw := TextInput(mv).Lines(1)
	h0 := mw.Layout(ggui.Loose(ggui.Sz(80, ggui.Unbounded)), ggui.Env{}).H
	mw.imeComposition(" a long preedit phrase that wraps across lines", 0)
	h1 := mw.Layout(ggui.Loose(ggui.Sz(80, ggui.Unbounded)), ggui.Env{}).H
	if h1 <= h0 {
		t.Fatalf("multiline height with IME composition %v <= without %v", h1, h0)
	}
}
