package textedit

import (
	"strings"
	"testing"
	"time"
)

// A document of mixed scripts, emoji and combining marks, 16 KiB long.
var benchDoc = func() string {
	line := "The quick brown fox — 빠른 갈색 여우 — 素早い茶色の狐 👩‍👩‍👧 é\n"
	return strings.Repeat(line, 16<<10/len(line))
}()

func BenchmarkBounds(b *testing.B) {
	b.SetBytes(int64(len(benchDoc)))
	b.ReportAllocs()
	var dst []int
	for b.Loop() {
		dst = AppendBounds(dst[:0], benchDoc)
	}
}

// Walking the caret across the document by grapheme, as an arrow key held
// down does.
func BenchmarkNextGrapheme(b *testing.B) {
	b.SetBytes(int64(len(benchDoc)))
	b.ReportAllocs()
	for b.Loop() {
		for i := 0; i < len(benchDoc); {
			i = NextGrapheme(benchDoc, i)
		}
	}
}

// Typing into the middle of the document, a key at a time, each one
// joining the undo step of the last. Putting the document back between
// runs is measured too; it costs little beside the typing.
func BenchmarkTyping(b *testing.B) {
	now := time.Unix(0, 0)
	e := Editor{Now: func() time.Time { return now }}
	mid := len(benchDoc) / 2
	b.ReportAllocs()
	for b.Loop() {
		e.SetText(benchDoc)
		e.MoveTo(e.Snap(mid), false)
		for _, r := range "hello, world" {
			e.Replace(string(r))
			now = now.Add(10 * time.Millisecond)
		}
	}
}

// Moving by word from one end of the document to the other.
func BenchmarkMoveByWord(b *testing.B) {
	var e Editor
	e.SetText(benchDoc)
	b.ReportAllocs()
	for b.Loop() {
		e.MoveTo(0, false)
		for e.Caret < len(e.Text) {
			e.MoveBy(1, true, false)
		}
	}
}

// Undoing and redoing a long history.
func BenchmarkUndoRedo(b *testing.B) {
	now := time.Unix(0, 0)
	e := Editor{Now: func() time.Time { return now }}
	e.SetText(benchDoc)
	for i := range 100 {
		e.MoveTo(e.Snap(i*97%len(e.Text)), false)
		e.Replace("x")
		now = now.Add(time.Second) // each its own step
	}
	b.ReportAllocs()
	for b.Loop() {
		for e.Undo() {
		}
		for e.Redo() {
		}
	}
}
