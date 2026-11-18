package main

import (
	"bytes"
	"encoding/json"
	"image/gif"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ironpark/ggui/cmd/ggui/internal/config"
	"github.com/ironpark/ggui/cmd/ggui/internal/scaffold"
	"github.com/ironpark/ggui/cmd/ggui/internal/ui"
)

// The request and the report are what package record reads and writes;
// its test checks the same strings (record/record_test.go).
const (
	goldenRecordRequest = `{"v":1,"tour":"intro","format":"gif","out":"/tmp/tour.gif","fps":10,"width":640,"colors":128,"dither":0.25,"loop":2,"frame":"mac","backdrop":"#20242a","no_cursor":true,"report":"/tmp/report.json"}`
	goldenRecordReport  = `{"v":1,"tours":["","intro"],"frames":42,"duration_ms":4200,"width":640,"height":400}`
)

func TestRecordProtocolGolden(t *testing.T) {
	req := recordRequest{V: 1, Tour: "intro", Format: "gif", Out: "/tmp/tour.gif", FPS: 10, Width: 640, Colors: 128, Dither: 0.25, Loop: 2, Frame: "mac", Backdrop: "#20242a", NoCursor: true, Report: "/tmp/report.json"}
	if got, _ := json.Marshal(req); string(got) != goldenRecordRequest {
		t.Errorf("request\n%s\nwant\n%s", got, goldenRecordRequest)
	}
	var rep recordReport
	if err := json.Unmarshal([]byte(goldenRecordReport), &rep); err != nil {
		t.Fatal(err)
	}
	if rep.V != 1 || len(rep.Tours) != 2 || rep.Tours[1] != "intro" || rep.Frames != 42 || rep.DurationMS != 4200 || rep.Width != 640 || rep.Height != 400 {
		t.Errorf("report read as %+v", rep)
	}
	if got, _ := json.Marshal(rep); string(got) != goldenRecordReport {
		t.Errorf("report\n%s\nwant\n%s", got, goldenRecordReport)
	}
}

func TestRecordFlags(t *testing.T) {
	for _, args := range [][]string{
		{"record", "a", "b"},
		{"record", "-o", "x.gif", "--frames", "dir"},
		{"record", "--colors", "300"},
		{"record", "--dither", "2"},
	} {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr); code == 0 {
			t.Errorf("%v exited 0", args)
		}
	}
}

// tourApp writes a module, beside this checkout's ggui, whose main
// package is body, and returns its directory.
func tourApp(t *testing.T, root, body string) string {
	t.Helper()
	dir := t.TempDir()
	mod := "module example.com/tour\n\ngo 1.27\n\nrequire " + scaffold.GGUI + " v0.0.0\n\nreplace " + scaffold.GGUI + " => " + filepath.ToSlash(root) + "\n"
	sum, err := os.ReadFile(filepath.Join(root, "go.sum"))
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{
		"go.mod":        []byte(mod),
		"go.sum":        sum,
		"main.go":       []byte(body),
		config.FileName: []byte("[app]\nname = \"Tour\"\n"),
	} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

const tourMain = `package main

import (
	"image/color"
	"log"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/record"
	"github.com/ironpark/ggui/widget"
)

func build(c *ggui.StateValue[color.Color]) ggui.Widget {
	return widget.Column(
		ggui.View(c, func(c color.Color) ggui.Widget { return widget.Box().Size(80, 30).Fill(c) }),
		widget.Text("tour"),
	)
}

func main() {
	c := ggui.State(color.Color(color.RGBA{R: 0xff, A: 0xff}))
	for _, name := range []string{"", "second"} {
		record.Register(record.Tour{
			Name:  name,
			Size:  ggui.Sz(80, 50),
			Build: func() ggui.Widget { return build(c) },
			Script: func(r *record.Recorder) {
				r.Hold(time.Second)
				r.Then(time.Second, func() { c.Set(color.RGBA{B: 0xff, A: 0xff}) })
			},
		})
	}
	if err := ggui.Run(ggui.Config{Title: "tour"}, func() ggui.Widget { return build(c) }); err != nil {
		log.Fatal(err)
	}
}
`

const plainMain = `package main

import (
	"log"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/widget"
)

func main() {
	if err := ggui.Run(ggui.Config{Title: "plain"}, func() ggui.Widget { return widget.Text("plain") }); err != nil {
		log.Fatal(err)
	}
}
`

// recordIn runs ggui record with args in dir and returns the exit status,
// what it printed, and what ggui and the app said.
func recordIn(t *testing.T, dir string, args ...string) (int, string, string) {
	t.Helper()
	t.Chdir(dir)
	var said, stdout, stderr bytes.Buffer
	saved := ui.Stderr
	ui.Stderr = ui.New(&said)
	defer func() { ui.Stderr = saved }()
	code := run(append([]string{"record"}, args...), &stdout, &stderr)
	return code, stdout.String(), said.String() + stderr.String()
}

// ggui record builds an app that registers tours, lists them, and draws
// one into a GIF; an app that registers none says so.
func TestRecordApp(t *testing.T) {
	if testing.Short() {
		t.Skip("builds apps against this checkout's ggui")
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(root, "go.mod")); err != nil || !strings.HasPrefix(string(data), "module "+scaffold.GGUI+"\n") {
		t.Skipf("not inside a ggui checkout: %s", root)
	}
	t.Setenv("GOWORK", "off")
	t.Setenv("GOFLAGS", "-mod=mod")

	t.Run("plain", func(t *testing.T) {
		dir := tourApp(t, root, plainMain)
		code, _, said := recordIn(t, dir)
		if code == 0 || !strings.Contains(said, "does not import github.com/ironpark/ggui/record") {
			t.Fatalf("exit %d:\n%s", code, said)
		}
	})

	dir := tourApp(t, root, tourMain)
	t.Run("list", func(t *testing.T) {
		// The names go to the process's standard output, as the path
		// of what ggui builds does, which run does not take.
		code, _, said := recordIn(t, dir, "--list")
		if code != 0 || !strings.Contains(said, "Listed the tours") {
			t.Fatalf("exit %d:\n%s", code, said)
		}
	})
	t.Run("unknown", func(t *testing.T) {
		code, _, said := recordIn(t, dir, "third")
		if code == 0 || !strings.Contains(said, `no tour named "third"; it has "" (the default), "second"`) {
			t.Fatalf("exit %d:\n%s", code, said)
		}
	})
	t.Run("gif", func(t *testing.T) {
		if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
			t.Skip("draws with the renderer, which Linux has only where EGL is installed")
		}
		out := filepath.Join(t.TempDir(), "out.gif")
		code, _, said := recordIn(t, dir, "second", "-o", out, "--fps", "5")
		if code != 0 {
			t.Fatalf("exit %d:\n%s", code, said)
		}
		if !strings.Contains(said, "2 frames, 2.0s, 80×50") {
			t.Errorf("the report:\n%s", said)
		}
		f, err := os.Open(out)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		g, err := gif.DecodeAll(f)
		if err != nil {
			t.Fatal(err)
		}
		if len(g.Image) != 2 || g.Config.Width != 80 || g.Config.Height != 50 || g.Delay[0] != 100 {
			t.Fatalf("%d frames of %d×%d, delays %v", len(g.Image), g.Config.Width, g.Config.Height, g.Delay)
		}
	})
}
