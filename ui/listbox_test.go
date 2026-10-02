package ui_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

func fileList(n int, selected ggui.Binding[string]) *ui.ListBoxWidget[string, string] {
	files := make([]string, n)
	for i := range files {
		files[i] = fmt.Sprintf("file-%02d", i)
	}
	return ui.ListBox(ggui.Const(files), func(f string) string { return f },
		func(f ggui.Readable[string]) ggui.Widget { return ggui.TextOf(f).NoWrap() },
	).BindSelected(selected).Name("Files")
}

func selectedOptions(p *ggui.Probe) []string {
	var out []string
	for _, n := range p.Semantics().Nodes(ggui.RoleOption) {
		if n.Selected {
			out = append(out, n.Name)
		}
	}
	return out
}

// The list is one Tab stop, on the selected row; the arrows move the
// selection with the focus, and Enter and a click activate a row.
func TestListBoxKeyboard(t *testing.T) {
	t.Parallel()
	selected := ggui.State("file-01")
	var picked []string
	list := fileList(4, selected).OnSelect(func(f string) { picked = append(picked, f) })
	p := ggui.NewProbe(ggui.Column(ui.Button("Before", func() {}), list), ggui.Sz(200, 300))
	defer p.Close()
	p.Type(ggui.Mods{}, ggui.KeyTab, ggui.KeyTab)
	if got := focusedItem(p); got != "file-01" {
		t.Fatalf("Tab landed on %q, want the selected row", got)
	}
	p.Type(ggui.Mods{}, ggui.KeyArrowDown)
	p.Frame()
	if got := focusedItem(p); got != "file-02" || ggui.Untrack(selected.Get) != "file-02" {
		t.Fatalf("after Down: focus %q, selected %q", got, ggui.Untrack(selected.Get))
	}
	p.Type(ggui.Mods{}, ggui.KeyEnd)
	p.Frame()
	if got := selectedOptions(p); !slices.Equal(got, []string{"file-03"}) {
		t.Fatalf("after End the selected options are %q", got)
	}
	if len(picked) != 0 {
		t.Fatalf("arrow keys activated %q", picked)
	}
	p.Type(ggui.Mods{}, ggui.KeyEnter)
	p.Tap("file-00")
	if !slices.Equal(picked, []string{"file-03", "file-00"}) {
		t.Fatalf("picked %q, want Enter's then the click's", picked)
	}
	p.Type(ggui.Mods{}, ggui.KeyTab)
	p.Type(ggui.Mods{Shift: true}, ggui.KeyTab)
	if got := focusedItem(p); got != "file-00" {
		t.Fatalf("Shift+Tab came back to %q, want the row last focused", got)
	}
}

// A selection made elsewhere scrolls the list to its row and leaves the
// focus where it was.
func TestListBoxRevealsASelectionMadeElsewhere(t *testing.T) {
	t.Parallel()
	selected := ggui.State("file-00")
	offset := ggui.State(0.0)
	p := ggui.NewProbe(ggui.Column(ui.Button("Elsewhere", func() {}), ggui.Expanded(ggui.Scroll(fileList(40, selected)).BindOffset(offset))), ggui.Sz(200, 200))
	defer p.Close()
	p.Type(ggui.Mods{}, ggui.KeyTab)
	selected.Set("file-30")
	p.Frame()
	p.Frame()
	if ggui.Untrack(offset.Get) <= 0 {
		t.Fatal("the list did not scroll to the selected row")
	}
	n, ok := p.Semantics().Find(ggui.RoleOption, "file-30")
	if !ok || n.Rect.Origin.Y+n.Rect.Size.H > 200 || n.Rect.Origin.Y < 0 {
		t.Fatalf("file-30 is at %v, out of the window", n.Rect)
	}
	if got := focusedItem(p); got != "Elsewhere" {
		t.Fatalf("focus moved to %q", got)
	}
	// Scrolling away is not undone while the selection stays.
	revealed := ggui.Untrack(offset.Get)
	p.Scroll(ggui.Pt(100, 100), ggui.Pt(0, 200))
	p.Frame()
	moved := ggui.Untrack(offset.Get)
	if moved == revealed {
		t.Fatal("the wheel did not scroll the list")
	}
	p.Frame()
	if ggui.Untrack(offset.Get) != moved {
		t.Fatal("the list scrolled back to the selection by itself")
	}
}
