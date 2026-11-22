package main

import (
	"encoding/json"
	"go/parser"
	"go/token"
	"strings"
	"testing"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui/theme"
)

func playground(t *testing.T, width, height int) (*model, *ggui.Probe, func()) {
	t.Helper()
	now := time.Unix(100, 0)
	restore := ggui.SetClock(func() time.Time { return now })
	t.Cleanup(restore)
	var m *model
	p := ggui.ProbeBuilder(func() ggui.Widget { m = newModel(); return ggui.Reactive(m.build) }, ggui.Sz(width, height))
	t.Cleanup(p.Close)
	settle := func() {
		for range 3 {
			p.Frame()
			now = now.Add(time.Second)
		}
	}
	settle()
	return m, p, settle
}

func TestThemeEditorAndExport(t *testing.T) {
	m, p, settle := playground(t, 1440, 1000)
	m.preview.notes.Set("Keep this draft")
	p.Tap("Theme")
	settle()
	p.Tap("Violet")
	settle()
	if ggui.Peek(m.settings).Theme.Preset.Accent != theme.AccentViolet {
		t.Fatal("theme selector did not update configuration")
	}
	if ggui.Peek(m.preview.notes) != "Keep this draft" {
		t.Fatal("theme edit reset preview")
	}
	p.Tap("Toggle appearance")
	settle()
	if ggui.Peek(m.settings).Mode != theme.ModeLight {
		t.Fatal("appearance toggle did not change mode")
	}
	p.Tap("Get Code")
	settle()
	if !ggui.Peek(m.codeOpen) {
		t.Fatal("code dialog did not open")
	}
	p.Tap("Copy Go code")
	settle()
	code := p.Clipboard().Read()
	if !strings.Contains(code, "violet") && !strings.Contains(code, "AccentViolet") {
		t.Fatalf("export omits selected accent: %s", code)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "theme.go", code, parser.AllErrors); err != nil {
		t.Fatalf("invalid Go export: %v", err)
	}
	p.Tap("Done")
	settle()
	p.Tap("Menu")
	settle()
	p.Tap("Reset")
	settle()
	if ggui.Peek(m.settings) != defaults() {
		t.Fatal("reset did not restore defaults")
	}
}

func TestPresetRoundTripAndInvalidInput(t *testing.T) {
	m, p, settle := playground(t, 1440, 1000)
	want := defaults()
	want.Theme.Preset.Base = theme.BaseStone
	want.Theme.Chart = theme.AccentBlue
	want.Theme.Heading = theme.FontMono
	want.Theme.Icons = theme.IconsTabler
	want.Theme.MenuAccent = theme.MenuAccentBold
	want.Mode = theme.ModeSystem
	m.settings.Set(want)
	settle()
	p.Tap("Copy preset")
	settle()
	data := p.Clipboard().Read()
	m.reset()
	m.presetText.Set(data)
	m.importPreset()
	settle()
	if ggui.Peek(m.settings) != want {
		t.Fatal("preset round trip lost settings")
	}
	for _, input := range []string{`{"Mode":"dark","Theme":{"Icons":"lucide","Radius":"invalid"}}`, data + ` {}`, `{"Mode":"dark","Theme":{"Icons":"invalid"}}`, `{"Mode":"dusk","Theme":{"Icons":"lucide"}}`, `{"Unknown":1}`} {
		m.presetOpen.Set(true)
		m.presetText.Set(input)
		m.importPreset()
		settle()
		if !ggui.Peek(m.presetOpen) || ggui.Peek(m.presetError) == "" {
			t.Fatalf("invalid preset accepted: %s", input)
		}
		if ggui.Peek(m.settings) != want {
			t.Fatal("invalid preset changed active theme")
		}
	}
	var decoded customization
	if err := json.Unmarshal([]byte(data), &decoded); err != nil {
		t.Fatal(err)
	}
}

func TestNarrowCustomizerPreservesPreview(t *testing.T) {
	m, p, settle := playground(t, 390, 844)
	p.Tap("Customize")
	settle()
	if !ggui.Peek(m.settingsOpen) {
		t.Fatal("narrow settings did not open")
	}
	p.Tap("Base color")
	settle()
	p.Tap("Stone")
	settle()
	if ggui.Peek(m.settings).Theme.Preset.Base != theme.BaseStone {
		t.Fatal("narrow settings did not update")
	}
	m.settingsOpen.Set(false)
	settle()
	p.Resize(ggui.Sz(1440, 1000))
	settle()
	if _, ok := p.Semantics().Find(ggui.RoleSelect, "Base color"); !ok {
		t.Fatal("desktop controls missing after resize")
	}
}

func TestShuffleRespectsTileLocks(t *testing.T) {
	m, p, settle := playground(t, 1440, 1000)
	style, ok := p.Semantics().Find(ggui.RoleSelect, "Style")
	if !ok {
		t.Fatal("missing style tile")
	}
	if style.Full.Size.H < 48 {
		t.Fatal("setting is not a two-line tile", style.Full)
	}
	p.PerformAction(style.ID, ggui.Action{Kind: ggui.ActionFocus})
	settle()
	p.Tap("Lock Style")
	settle()
	if !ggui.Peek(m.locked)[lockStyle] {
		t.Fatal("lock action did not update")
	}
	want := ggui.Peek(m.settings).Theme.Preset.Style
	for range 8 {
		m.shuffle()
		settle()
		if ggui.Peek(m.settings).Theme.Preset.Style != want {
			t.Fatal("shuffle changed locked style")
		}
	}
	p.Tap("Unlock Style")
	settle()
	if ggui.Peek(m.locked)[lockStyle] {
		t.Fatal("unlock action did not update")
	}
	locks := [lockCount]bool{}
	for i := range locks {
		locks[i] = true
	}
	m.locked.Set(locks)
	before := ggui.Peek(m.settings)
	m.shuffle()
	settle()
	if ggui.Peek(m.settings) != before {
		t.Fatal("shuffle changed locked configuration")
	}
	m.reset()
	settle()
	if ggui.Peek(m.locked) != ([lockCount]bool{}) {
		t.Fatal("reset retained locks")
	}
}

func TestReferencePayoutAmountDoesNotCrowdItsLabel(t *testing.T) {
	_, p, _ := playground(t, 1440, 1000)
	label, ok := p.Semantics().Find(ggui.RoleText, "Minimum Payout Amount")
	if !ok {
		t.Fatal("missing amount label")
	}
	value, ok := p.Semantics().Find(ggui.RoleText, "$3000.00")
	if !ok {
		t.Fatal("missing amount value")
	}
	if label.Full.Origin.X+label.Full.Size.W+8 > value.Full.Origin.X {
		t.Fatalf("payout amount crowds label: label=%v amount=%v", label.Full, value.Full)
	}
}

func TestSourceCodeFontRegistrations(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		font       theme.FontFamily
		heading    theme.FontFamily
		want       []string
		wantAbsent []string
		wantCount  map[string]int
	}{
		{
			name:    "default omits font imports and registrations",
			font:    theme.FontDefault,
			heading: theme.FontDefault,
			wantAbsent: []string{
				"github.com/ironpark/ggui/fonts/",
				"theme.RegisterFont(",
			},
		},
		{
			name:    "inter deduplicates import and registration",
			font:    theme.FontInter,
			heading: theme.FontInter,
			wantCount: map[string]int{
				`"github.com/ironpark/ggui/fonts/inter"`:  1,
				`theme.RegisterFont("Inter", inter.Font)`: 1,
			},
			wantAbsent: []string{
				"github.com/ironpark/ggui/fonts/geist",
				"github.com/ironpark/ggui/fonts/gofont",
			},
		},
		{
			name:    "geist and geist-mono share one import",
			font:    theme.FontGeist,
			heading: theme.FontGeistMono,
			want: []string{
				`theme.RegisterFont("Geist", geist.Font)`,
				`theme.RegisterFont("Geist Mono", geist.Mono)`,
			},
			wantCount: map[string]int{
				`"github.com/ironpark/ggui/fonts/geist"`: 1,
			},
			wantAbsent: []string{
				"github.com/ironpark/ggui/fonts/inter",
				"github.com/ironpark/ggui/fonts/gofont",
			},
		},
		{
			name:    "sans and mono share gofont import",
			font:    theme.FontSans,
			heading: theme.FontMono,
			want: []string{
				`theme.RegisterFont("Go", gofont.Font)`,
				`theme.RegisterFont("Go Mono", gofont.Mono)`,
			},
			wantCount: map[string]int{
				`"github.com/ironpark/ggui/fonts/gofont"`: 1,
			},
			wantAbsent: []string{
				"github.com/ironpark/ggui/fonts/inter",
				"github.com/ironpark/ggui/fonts/geist",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := defaults()
			s.Theme.Font = tc.font
			s.Theme.Heading = tc.heading
			code := sourceCode(s)
			if _, err := parser.ParseFile(token.NewFileSet(), "theme.go", code, parser.AllErrors); err != nil {
				t.Fatalf("invalid generated Go:\n%s\nerr: %v", code, err)
			}
			for _, sub := range tc.want {
				if !strings.Contains(code, sub) {
					t.Errorf("generated Go missing %q:\n%s", sub, code)
				}
			}
			for sub, count := range tc.wantCount {
				if got := strings.Count(code, sub); got != count {
					t.Errorf("generated Go has %d occurrences of %q, want %d:\n%s", got, sub, count, code)
				}
			}
			for _, sub := range tc.wantAbsent {
				if strings.Contains(code, sub) {
					t.Errorf("generated Go unexpectedly contains %q:\n%s", sub, code)
				}
			}
		})
	}
}
