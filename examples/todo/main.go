// Command todo is a small but complete ggui app: a text field with IME
// input, a keyed list that keeps each row's widgets across edits, controls
// bound to signals, a tweened progress bar and a theme switch.
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/ironpark/ggui"
)

// Todo is a struct of signals: each row's title and done flag are cells of
// their own, so ticking one box re-runs only what read that box.
type Todo struct {
	ID    int
	Title *ggui.Signal[string]
	Done  *ggui.Signal[bool]
}

type filter int

const (
	all filter = iota
	active
	done
)

func main() {
	useSystemFont()

	todos := ggui.State([]*Todo{})
	draft := ggui.State("")
	show := ggui.State(all)
	dark := ggui.State(false)
	nextID := 1

	add := func(string) {
		title := strings.TrimSpace(draft.Peek())
		if title == "" {
			return
		}
		t := &Todo{ID: nextID, Title: ggui.State(title), Done: ggui.State(false)}
		nextID++
		todos.Update(func(ts []*Todo) []*Todo { return append(ts, t) })
		draft.Set("")
	}
	remove := func(id int) {
		todos.Update(func(ts []*Todo) []*Todo {
			out := ts[:0:0]
			for _, t := range ts {
				if t.ID != id {
					out = append(out, t)
				}
			}
			return out
		})
	}
	clearDone := func() {
		todos.Update(func(ts []*Todo) []*Todo {
			out := ts[:0:0]
			for _, t := range ts {
				if !t.Done.Peek() {
					out = append(out, t)
				}
			}
			return out
		})
	}

	// Derived views. visible follows the list, the filter and every Done flag.
	visible := ggui.Derived(func() []*Todo {
		f := show.Get()
		var out []*Todo
		for _, t := range todos.Get() {
			if f == all || (f == active) != t.Done.Get() {
				out = append(out, t)
			}
		}
		return out
	})
	counts := ggui.Derived(func() [2]int {
		var n, d int
		for _, t := range todos.Get() {
			n++
			if t.Done.Get() {
				d++
			}
		}
		return [2]int{n, d}
	})
	progress := ggui.Tween(0.0, 300*time.Millisecond)
	ggui.Watch(counts, func(c [2]int) {
		if c[0] == 0 {
			progress.Set(0)
			return
		}
		progress.Set(float64(c[1]) / float64(c[0]))
	})
	ggui.Watch(dark, func(on bool) {
		if on {
			ggui.SetTheme(ggui.DarkTheme())
		} else {
			ggui.SetTheme(ggui.DefaultTheme())
		}
	})

	row := func(item *ggui.Signal[*Todo]) ggui.Widget {
		td := item.Peek()
		return ggui.Row(
			ggui.Checkbox(td.Done, ""),
			ggui.Expanded(ggui.Reactive(func() ggui.Widget {
				t := ggui.UseTheme()
				text := ggui.Text(td.Title.Get())
				if td.Done.Get() {
					text.Color(t.Muted)
				}
				return text
			})),
			ggui.Button("×", func() { remove(td.ID) }).Secondary().Pad(2, 8),
		).Gap(8).Align(ggui.AlignCenter)
	}

	app := ggui.New(ggui.Config{Title: "ggui · todo", Width: 520, Height: 600, Resizable: true}, func() ggui.Widget {
		t := ggui.UseTheme()
		return ggui.Center(ggui.Box(ggui.Column(
			ggui.Row(
				ggui.Text("todo").Style(t.Title),
				ggui.Spacer(),
				ggui.Switch(dark, "Dark"),
			).Align(ggui.AlignCenter),
			ggui.Row(
				ggui.Expanded(ggui.TextField(draft).Placeholder("What needs doing?").OnSubmit(add)),
				ggui.Button("Add", func() { add("") }),
			).Gap(t.Space).Align(ggui.AlignCenter),
			ggui.Reactive(func() ggui.Widget {
				const width = 480 - 2*3*8 // the card's inner width
				return ggui.Stack(
					ggui.Box().Size(width, 4).Fill(t.Border).Radius(2),
					ggui.Box().Size(max(progress.Get()*width, 0), 4).Fill(t.Accent).Radius(2),
				)
			}),
			ggui.Expanded(ggui.Scroll(
				ggui.For(visible, func(td *Todo) int { return td.ID }, row).Gap(t.Space/2),
			)),
			ggui.Divider(),
			ggui.Row(
				ggui.Styled(ggui.Reactive(func() ggui.Widget {
					c := counts.Get()
					return ggui.Text(fmt.Sprintf("%d left", c[0]-c[1])).NoWrap()
				})).Color(t.Muted),
				ggui.Spacer(),
				ggui.Radio(show, all, "All"),
				ggui.Radio(show, active, "Active"),
				ggui.Radio(show, done, "Done"),
				ggui.Button("Clear done", clearDone).Secondary(),
			).Gap(t.Space).Align(ggui.AlignCenter),
		).Gap(t.Space*1.5)).Pad(t.Space*3).Fill(t.Surface).Radius(t.Radius*2).Size(480, 540))
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

// useSystemFont swaps in a font with CJK glyphs when the platform has one,
// so text typed through the IME shows up. Go Regular, the built-in default,
// covers Latin, Greek and Cyrillic only.
func useSystemFont() {
	var candidates []string
	switch runtime.GOOS {
	case "darwin":
		candidates = []string{
			"/System/Library/Fonts/AppleSDGothicNeo.ttc",
			"/System/Library/Fonts/Supplemental/AppleGothic.ttf",
		}
	case "windows":
		candidates = []string{filepath.Join(os.Getenv("WINDIR"), "Fonts", "malgun.ttf")}
	default:
		candidates = []string{
			"/usr/share/fonts/truetype/nanum/NanumGothic.ttf",
			"/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc",
			"/usr/share/fonts/noto-cjk/NotoSansCJK-Regular.ttc",
		}
	}
	for _, path := range candidates {
		if f, err := ggui.LoadFontFile(path); err == nil {
			ggui.SetDefaultFont(f)
			return
		}
	}
}
