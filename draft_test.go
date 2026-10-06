package ggui

import (
	"errors"
	"testing"
)

// A text input bound to a Draft shows the model until edited, commits on
// Enter, keeps a refused edit with its error, and drops an edit on Escape.
func TestTextInputCommitsAndRevertsADraft(t *testing.T) {
	saved := State("ada")
	commits := 0
	d := Draft[string](saved, func(v string) error {
		commits++
		if v == "" {
			return errors.New("a name is required")
		}
		saved.Set(v)
		return nil
	})
	input := TextInput(d)
	p := NewProbe(input, Sz(200, 30))
	defer p.Close()
	escape := KeyEvent{Kind: KeyPress, Key: KeyEscape}
	p.Click(Pt(10, 10))
	p.Key("end")
	p.Text("!")
	if Untrack(saved.Get) != "ada" || Untrack(d.Get) != "ada!" || !Untrack(d.Dirty().Get) {
		t.Fatalf("while typing: saved %q, draft %q", Untrack(saved.Get), Untrack(d.Get))
	}
	p.Key("enter")
	if Untrack(saved.Get) != "ada!" || Untrack(d.Dirty().Get) || commits != 1 {
		t.Fatalf("after Enter: saved %q, dirty %v, %d commits", Untrack(saved.Get), Untrack(d.Dirty().Get), commits)
	}

	p.Key("cmd+a", "backspace", "enter")
	if Untrack(d.Error().Get) != "a name is required" || Untrack(d.Get) != "" || Untrack(saved.Get) != "ada!" {
		t.Fatalf("refused: error %q, draft %q, saved %q", Untrack(d.Error().Get), Untrack(d.Get), Untrack(saved.Get))
	}
	p.Key("escape")
	if Untrack(d.Get) != "ada!" || Untrack(d.Error().Get) != "" || !input.ConsumesKey(escape) {
		t.Fatalf("after Escape: draft %q, error %q, Escape kept %v", Untrack(d.Get), Untrack(d.Error().Get), input.ConsumesKey(escape))
	}
	p.Key("escape")
	if input.ConsumesKey(escape) {
		t.Fatal("Escape on a clean field stayed with it, so a dialog around it would not close")
	}

	// The model changing under a clean draft shows through.
	saved.Set("grace")
	p.Frame()
	if Untrack(d.Get) != "grace" {
		t.Fatalf("draft %q after the model changed, want grace", Untrack(d.Get))
	}
}
