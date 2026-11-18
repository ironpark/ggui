package reactive

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ironpark/ggui/internal/devstate"
)

// The tests here hand state over as ggui dev would, through a session of
// the process's own, so none of them is parallel.

// handOver starts a session with handed, its values JSON, and captures
// what it says on stderr.
func handOver(t *testing.T, handed map[string]string) *bytes.Buffer {
	t.Helper()
	st := devstate.State{}
	for k, v := range handed {
		st[k] = json.RawMessage(v)
	}
	t.Cleanup(devstate.Begin(st))
	var errs bytes.Buffer
	prev := devstate.Stderr
	devstate.Stderr = &errs
	t.Cleanup(func() { devstate.Stderr = prev })
	return &errs
}

func snapshot(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	for k, v := range devstate.Snapshot() {
		out[k] = string(v)
	}
	return out
}

type keptForm struct {
	Name  string
	Age   int
	Notes []string
}

// Without ggui dev Keep returns the state as it was and keeps nothing.
func TestKeepWithoutDev(t *testing.T) {
	s := State(3)
	if got := s.Keep("keep.off"); got != s || Untrack(s.Get) != 3 {
		t.Fatalf("Keep without a session = %p holding %d, want %p holding 3", got, Untrack(got.Get), s)
	}
	if st := devstate.Snapshot(); st != nil {
		t.Errorf("a snapshot without a session = %v, want nil", st)
	}
}

// The value handed over is restored when Keep is called, before it
// returns; what the state holds afterwards is what is handed on.
func TestKeepRestoresAndSaves(t *testing.T) {
	handOver(t, map[string]string{"count": "5", "form": `{"Name":"Ada","Notes":["a"]}`})
	count := State(0).Keep("count")
	if v := Untrack(count.Get); v != 5 {
		t.Fatalf("count kept as 5 restored as %d", v)
	}
	// A field the new build added keeps its initial value.
	form := State(keptForm{Age: 36}).Keep("form")
	if f := Untrack(form.Get); f.Name != "Ada" || f.Age != 36 || len(f.Notes) != 1 {
		t.Fatalf("form restored as %+v, want Ada with the initial Age 36", f)
	}
	count.Set(7)
	got := snapshot(t)
	if got["count"] != "7" || !strings.Contains(got["form"], `"Name":"Ada"`) {
		t.Errorf("snapshot = %v, want count 7 and the form", got)
	}
}

// A value claimed once is not handed to the next Keep of its key: a
// component mounted again starts fresh, as it would without ggui dev.
func TestKeepClaimsOnce(t *testing.T) {
	handOver(t, map[string]string{"tab": `"advanced"`})
	dispose := Root(func() {
		if v := Untrack(State("general").Keep("tab").Get); v != "advanced" {
			t.Errorf("tab restored as %q", v)
		}
	})
	dispose()
	if _, ok := snapshot(t)["tab"]; ok {
		t.Error("a state disposed with its owner is still kept")
	}
	if v := Untrack(State("general").Keep("tab").Get); v != "general" {
		t.Errorf("the second Keep of a claimed key restored %q, want its initial value", v)
	}
}

// A value whose type changed so that it no longer decodes is dropped, said
// once, and the state keeps its initial value; one handed over that no
// Keep asked for is handed on.
func TestKeepTypeChange(t *testing.T) {
	errs := handOver(t, map[string]string{
		"keep.changed": `"seven"`,
		"keep.later":   `{"Name":"Grace"}`,
	})
	n := State(1).Keep("keep.changed")
	if v := Untrack(n.Get); v != 1 {
		t.Errorf("a value that does not decode set the state to %d", v)
	}
	if !strings.Contains(errs.String(), `"keep.changed" no longer decodes`) {
		t.Errorf("stderr = %q, want the dropped value named", errs.String())
	}
	got := snapshot(t)
	if got["keep.changed"] != "1" {
		t.Errorf("snapshot keeps keep.changed as %s, want the state's own 1", got["keep.changed"])
	}
	if got["keep.later"] != `{"Name":"Grace"}` {
		t.Errorf("an unclaimed value was not handed on: %v", got)
	}
	// It is still there for a component that mounts later.
	later := State(keptForm{}).Keep("keep.later")
	if Untrack(later.Get).Name != "Grace" {
		t.Errorf("a Keep after startup restored %+v", Untrack(later.Get))
	}
}

// A value that does not encode is left out, said once, and the rest are
// kept.
func TestKeepEncodeFailure(t *testing.T) {
	errs := handOver(t, nil)
	State(func() {}).Keep("keep.func")
	State("ok").Keep("keep.fine")
	got := snapshot(t)
	if _, ok := got["keep.func"]; ok || got["keep.fine"] != `"ok"` {
		t.Errorf("snapshot = %v, want keep.fine alone", got)
	}
	if !strings.Contains(errs.String(), `"keep.func" does not encode`) {
		t.Errorf("stderr = %q, want the unencodable value named", errs.String())
	}
}

// A key kept twice is the later state's, and the earlier one letting go
// does not take it from the later.
func TestKeepDuplicateKeyLastWins(t *testing.T) {
	handOver(t, map[string]string{"dup": "1"})
	var first *StateValue[int]
	disposeFirst := Root(func() { first = State(0).Keep("dup") })
	second := State(0).Keep("dup")
	if Untrack(first.Get) != 1 || Untrack(second.Get) != 0 {
		t.Errorf("restored %d and %d, want the value to the first Keep alone", Untrack(first.Get), Untrack(second.Get))
	}
	second.Set(2)
	disposeFirst()
	if got := snapshot(t)["dup"]; got != "2" {
		t.Errorf("dup kept as %s, want the later state's 2", got)
	}
}

// A value decoded into is a copy: the initial value's slice is not
// written through.
func TestKeepRestoreCopies(t *testing.T) {
	handOver(t, map[string]string{"keep.slice": `["x","y"]`})
	initial := []string{"a", "b"}
	s := State(initial).Keep("keep.slice")
	if initial[0] != "a" || initial[1] != "b" {
		t.Errorf("restoring wrote into the initial slice: %v", initial)
	}
	if v := Untrack(s.Get); len(v) != 2 || v[0] != "x" {
		t.Errorf("restored %v", v)
	}
}
