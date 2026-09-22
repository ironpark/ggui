package theme_test

import (
	"image/color"
	"testing"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
	"github.com/ironpark/ggui/ui/theme"
)

func preserveEnv(t *testing.T) {
	t.Helper()
	old := ggui.Untrack(ggui.UseEnv)
	t.Cleanup(func() { ggui.SetEnv(old) })
}

func TestBindUpdatesCachedLayoutAndTrackedReaders(t *testing.T) {
	preserveEnv(t)
	dark := ggui.State(true)
	var seen theme.Theme
	layouts, builds := 0, 0
	child := ggui.FromFuncs(func(c ggui.Constraints, env ggui.Env) ggui.Size {
		layouts++
		seen = theme.From(env)
		return c.Constrain(ggui.Sz(10, 10))
	}, func(*ggui.Canvas, ggui.Rect) {})
	p := ggui.NewProbe(ggui.Cached(child), ggui.Sz(20, 20)).Setup(func() {
		theme.Bind(dark, theme.Dark(), theme.Default())
		ggui.Effect(func() ggui.Cleanup { _ = theme.Use(); builds++; return nil })
	})
	defer p.Close()
	p.Frame()
	if seen.Bg != theme.Dark().Bg || builds != 1 {
		t.Fatalf("initial theme: %v, builds: %d", seen.Bg, builds)
	}
	initialLayouts := layouts
	p.Frame()
	if layouts != initialLayouts {
		t.Fatal("unchanged frame invalidated cached layout")
	}
	dark.Set(false)
	p.Frame()
	if seen.Bg != theme.Default().Bg || builds != 2 || layouts != initialLayouts+1 {
		t.Fatalf("theme change: %v, builds: %d, layouts: %d", seen.Bg, builds, layouts)
	}
	p.Close()
	dark.Set(true)
	// Closing the owner must dispose Bind; a new frame flushes pending work.
	next := ggui.NewProbe(ggui.Box(), ggui.Sz(10, 10))
	defer next.Close()
	next.Frame()
	if ggui.Untrack(theme.Use).Bg != theme.Default().Bg {
		t.Fatal("closed binding still changes the theme")
	}
}

func TestLocalThemeMapsCoreStylesAndPreservesPreferences(t *testing.T) {
	preserveEnv(t)
	theme.Set(theme.Default())
	local := theme.Dark()
	local.Space, local.MotionFast = 13, 97*time.Millisecond
	local.Title.Size = 31
	custom := ggui.NewEnvKey[string]("custom")
	parent := theme.Default().Apply(ggui.Env{}).With(custom, "kept").With(ggui.ReducedMotionKey, true)
	var seen ggui.Env
	child := ggui.FromFuncs(func(c ggui.Constraints, env ggui.Env) ggui.Size {
		seen = env
		return c.Constrain(ggui.Sz(10, 10))
	}, func(*ggui.Canvas, ggui.Rect) {})
	tree := theme.With(local, child)
	tree.Layout(ggui.Loose(ggui.Sz(100, 100)), parent)
	if theme.From(seen).Bg != local.Bg || seen.Background() != local.Bg || seen.Spacing() != 13 {
		t.Fatal("subtree did not receive local theme and core settings")
	}
	if seen.Text().Color != local.Fg || seen.EditorStyle().Selection != local.Selection || seen.ScrollStyle().Color != local.MutedFg || seen.PopupDuration() != local.MotionFast {
		t.Fatal("theme did not reach core text, editor, scrollbar, and popup styles")
	}
	if value, _ := seen.Get(custom); value != "kept" || !seen.ReducedMotion() || seen.Motion(seen.PopupDuration()) != 0 {
		t.Fatal("applying a theme lost inherited environment values")
	}
	if theme.From(parent).Bg != theme.Default().Bg || ggui.Untrack(theme.Use).Bg != theme.Default().Bg {
		t.Fatal("subtree theme changed its parent or global theme")
	}
	title := ui.Title("Heading")
	if localSize, plainSize := title.Layout(ggui.Loose(ggui.Sz(500, 100)), seen), title.Layout(ggui.Loose(ggui.Sz(500, 100)), parent); localSize.H <= plainSize.H {
		t.Fatal("prebuilt title did not resolve the local named style")
	}
}

func TestSetResetsTextAndPreservesOtherRootValues(t *testing.T) {
	preserveEnv(t)
	custom := ggui.NewEnvKey[int]("custom")
	ggui.SetEnv(ggui.Untrack(ggui.UseEnv).With(custom, 42).With(ggui.TextScaleKey, 1.5))
	first := theme.Default()
	first.Text.Font = new(ggui.Font) // only inspect identity; no font rendering
	first.Text.Color = color.Black
	theme.Set(first)
	theme.Set(theme.Default())
	env := ggui.Untrack(ggui.UseEnv)
	if env.Text().Font != nil {
		t.Fatal("previous theme font leaked into the next theme")
	}
	if v, _ := env.Get(custom); v != 42 || env.TextScale() != 1.5 {
		t.Fatal("theme change erased application preferences")
	}
}

func TestLocalThemeKeepsCacheAcrossFrames(t *testing.T) {
	layouts := 0
	child := ggui.Cached(ggui.FromFuncs(func(c ggui.Constraints, _ ggui.Env) ggui.Size {
		layouts++
		return c.Constrain(ggui.Sz(10, 10))
	}, func(*ggui.Canvas, ggui.Rect) {}))
	p := ggui.NewProbe(theme.With(theme.Dark(), child), ggui.Sz(20, 20))
	defer p.Close()
	p.Frame()
	// Force an unrelated layout without changing the environment.
	state := ggui.State(0)
	state.Set(1)
	p.Frame()
	if layouts != 1 {
		t.Fatalf("local theme invalidated stable cached child %d times", layouts)
	}
}
