package i18n_test

import (
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/i18n"
	"github.com/ironpark/ggui/widget"
)

var (
	ko = &i18n.Catalog{Lang: "ko", Messages: map[string]string{"New sandbox": "새 샌드박스", "Quit": "종료"}}
	ja = &i18n.Catalog{Lang: "ja", Messages: map[string]string{"New sandbox": "新しいサンドボックス", "Quit": "終了"}}
	ar = &i18n.Catalog{Lang: "ar", Messages: map[string]string{"New sandbox": "صندوق جديد"}}
)

// shown reports whether p shows text.
func shown(p *ggui.Probe, text string) bool {
	if _, ok := p.Find(text); ok {
		return true
	}
	for _, n := range p.Semantics().All() {
		if n.Name == text || n.Value == text {
			return true
		}
	}
	return false
}

// TestATextFollowsTheLanguageWithoutARebuild shows a message under a
// Provide and under a Provide of another Localizer inside it: each shows
// its own language at once, and switching the outer language lays the
// text out again in it without building the tree again.
func TestATextFollowsTheLanguageWithoutARebuild(t *testing.T) {
	t.Parallel()
	lang := ggui.State("ko")
	preview := ggui.State("ja")
	builds := 0
	var env ggui.Env
	p := ggui.ProbeBuilder(func() ggui.Widget {
		builds++
		probe := ggui.FromFuncs(func(c ggui.Constraints, e ggui.Env) ggui.Size {
			env = e
			return ggui.Size{}
		}, func(*ggui.Canvas, ggui.Rect) {})
		return i18n.Provide(i18n.New(lang, ko, ja, ar), widget.Column(
			widget.TextOf(i18n.T("New sandbox")),
			probe,
			i18n.Provide(i18n.New(preview, ko, ja), widget.TextOf(i18n.T("Quit"))),
		))
	}, ggui.Sz(400, 200))
	defer p.Close()
	p.Frame()
	if !shown(p, "새 샌드박스") || !shown(p, "終了") {
		t.Fatalf("not each subtree in its own language:\n%s", p.Semantics())
	}
	if env.Language() != "ko" || env.Direction() != ggui.DirLTR {
		t.Errorf("Env language %q direction %v, want ko, left to right", env.Language(), env.Direction())
	}
	lang.Set("ja")
	p.Frame()
	if !shown(p, "新しいサンドボックス") || shown(p, "새 샌드박스") {
		t.Errorf("the switch did not reach the text:\n%s", p.Semantics())
	}
	lang.Set("ar")
	p.Frame()
	if !shown(p, "صندوق جديد") || env.Language() != "ar" || env.Direction() != ggui.DirRTL {
		t.Errorf("Arabic: language %q direction %v\n%s", env.Language(), env.Direction(), p.Semantics())
	}
	lang.Set("")
	p.Frame()
	if !shown(p, "New sandbox") || env.Language() != "" || env.Direction() != ggui.DirLTR {
		t.Errorf("reset to source: language %q direction %v\n%s", env.Language(), env.Direction(), p.Semantics())
	}
	preview.Set("ko")
	p.Frame()
	if !shown(p, "종료") {
		t.Errorf("the inner switch did not reach its text:\n%s", p.Semantics())
	}
	if builds != 1 {
		t.Errorf("the tree was built %d times; a switch should only lay it out again", builds)
	}
}

// TestInstallTranslatesOffTheTree installs a Localizer for the
// application: a message read outside layout, and a text no Provide
// covers, are translated by it, and Watch runs again on a switch.
func TestInstallTranslatesOffTheTree(t *testing.T) {
	t.Parallel()
	lang := ggui.State("ko")
	loc := i18n.New(lang, ko, ja)
	var title string
	var watched []string
	p := ggui.ProbeBuilder(func() ggui.Widget {
		i18n.Install(loc)
		ggui.Effect(func() ggui.Cleanup {
			title = i18n.T("Quit").Get()
			return nil
		})
		i18n.Watch(loc, func(l *i18n.Localizer) { watched = append(watched, l.S("Quit")) })
		return widget.TextOf(i18n.T("New sandbox"))
	}, ggui.Sz(400, 100))
	defer p.Close()
	p.Frame()
	if title != "종료" || !shown(p, "새 샌드박스") {
		t.Fatalf("installed: title %q\n%s", title, p.Semantics())
	}
	lang.Set("ja")
	p.Frame()
	if title != "終了" || !shown(p, "新しいサンドボックス") {
		t.Errorf("after the switch: title %q\n%s", title, p.Semantics())
	}
	if len(watched) != 2 || watched[0] != "종료" || watched[1] != "終了" {
		t.Errorf("Watch saw %q, want 종료 then 終了", watched)
	}
}

// TestFromWithoutAProviderIsTheSourceLanguage reads a message in an Env
// no Localizer was provided in.
func TestFromWithoutAProviderIsTheSourceLanguage(t *testing.T) {
	t.Parallel()
	if got := i18n.T("Delete %s?", "x").In(ggui.Env{}); got != "Delete x?" {
		t.Errorf("In an empty Env = %q", got)
	}
	if i18n.From(ggui.Env{}) == nil {
		t.Error("From returned nil")
	}
}
