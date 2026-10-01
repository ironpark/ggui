package router

import (
	"errors"
	"strings"
	"testing"

	"github.com/ironpark/ggui"
)

// failingHistory is a memory history whose Push and Replace fail, as a
// browser adapter's may.
type failingHistory struct {
	*MemoryHistory
	err error
}

func (h failingHistory) Push(string) (Entry, error)    { return Entry{}, h.err }
func (h failingHistory) Replace(string) (Entry, error) { return Entry{}, h.err }

func TestAnInitialRedirectReplacesTheEntry(t *testing.T) {
	t.Parallel()
	h := Memory("/old?keep=1")
	r := New(Page("/", text("home")), Page("/new", text("new")), Redirect("/old", "/new"))
	r.History(h)
	p := ggui.NewProbe(r.View(), ggui.Sz(100, 100))
	defer p.Close()
	p.Frame()

	// A redirect goes to its literal destination: the query is not carried.
	if got := r.Location(); got.Path != "/new" || got.Entry != h.Current().ID {
		t.Errorf("Location = %+v, want /new on the current entry %d", got, h.Current().ID)
	}
	if got := h.Current().URL; got != "/new" {
		t.Errorf("history holds %q, want the redirect's destination /new", got)
	}
	if _, n := h.Position(); n != 1 {
		t.Errorf("history has %d entries, want the first one replaced rather than a second pushed", n)
	}
}

func TestAnUnparsableInitialEntryShowsTheBuiltinNotFound(t *testing.T) {
	t.Parallel()
	r := New(Page("/", text("home")))
	r.History(Memory("no-leading-slash"))
	p := ggui.NewProbe(r.View(), ggui.Sz(100, 100))
	defer p.Close()
	p.Frame()

	if got := r.Location().Path; got != "no-leading-slash" {
		t.Errorf("Location.Path = %q, want the entry's URL kept as it was", got)
	}
	if r.Current().Get() != nil {
		t.Error("Current should be nil on the built-in not-found screen")
	}
	if _, ok := p.Semantics().Find(ggui.RoleText, "Not found"); !ok {
		t.Error("the built-in not-found screen is not showing")
	}
}

func TestNavigateReportsBadURLsAndHistoryErrors(t *testing.T) {
	t.Parallel()
	boom := errors.New("history is full")
	h := failingHistory{Memory("/"), boom}
	r := New(Page("/", text("home")), Page("/a", text("a")))
	r.History(h)
	p := ggui.NewProbe(r.View(), ggui.Sz(100, 100))
	defer p.Close()
	p.Frame()

	for _, bad := range []string{"a", "//host/a", "/a//b", "/%zz", "mailto:x"} {
		if err := r.Navigate(bad); err == nil {
			t.Errorf("Navigate(%q) succeeded, want an error", bad)
		}
	}
	if err := r.Navigate("/a"); !errors.Is(err, boom) {
		t.Errorf("Navigate with a failing Push = %v, want %v", err, boom)
	}
	if err := r.Replace("/a"); !errors.Is(err, boom) {
		t.Errorf("Replace with a failing Replace = %v, want %v", err, boom)
	}
	p.Frame()
	if got := r.Location().Path; got != "/" {
		t.Errorf("a failed navigation moved the router to %s", got)
	}
}

func TestReadersBeforeMount(t *testing.T) {
	t.Parallel()
	home := Page("/", text("home"))
	r := New(home)
	if r.Current().Get() != nil || home.Active().Get() {
		t.Error("an unmounted router reports a matched page")
	}
	if r.CanBack().Get() || r.CanForward().Get() {
		t.Error("an unmounted router reports history to traverse")
	}
	// Back and Forward before mounting do nothing rather than panic.
	r.Back()
	r.Forward()
	if err := home.Navigate(nil); err != ErrNotMounted {
		t.Errorf("Target.Navigate before mount = %v, want ErrNotMounted", err)
	}
}

func TestActiveRejectsAnInvalidPath(t *testing.T) {
	t.Parallel()
	r := New(Page("/", text("home")))
	defer func() {
		if recover() == nil {
			t.Error("Active(\"relative\") did not panic")
		}
	}()
	r.Active("relative")
}

func TestTargetNavigateReportsMissingParameters(t *testing.T) {
	t.Parallel()
	item := Page("/items/:id", text("item"))
	r := New(Page("/", text("home")), item)
	p := ggui.NewProbe(r.View(), ggui.Sz(100, 100))
	defer p.Close()
	p.Frame()
	err := item.Navigate(nil)
	if err == nil || !strings.Contains(err.Error(), `missing parameter "id"`) {
		t.Errorf("Navigate(nil) on /items/:id = %v, want a missing parameter error", err)
	}
	if err := item.Navigate(Params{"id": "9"}); err != nil || r.Location().Path != "/items/9" {
		t.Errorf("Navigate(id=9) = %v at %s, want /items/9", err, r.Location().Path)
	}
}

func TestMemoryHistoryGoIgnoresZeroAndOutOfRange(t *testing.T) {
	t.Parallel()
	h := Memory("/a")
	h.Push("/b")
	h.Push("/c")
	var got []string
	stop := h.Subscribe(func(e Entry) { got = append(got, e.URL) })
	for _, d := range []int{0, 1, -3, 5} {
		h.Go(d)
	}
	if len(got) != 0 {
		t.Errorf("Go past either end or by zero notified %v", got)
	}
	h.Go(-2)
	if i, n := h.Position(); i != 0 || n != 3 || len(got) != 1 || got[0] != "/a" {
		t.Errorf("Go(-2) from the end: position %d of %d, notified %v; want 0 of 3 and /a", i, n, got)
	}
	stop()
	h.Go(1)
	if len(got) != 1 {
		t.Errorf("a stopped subscriber was notified: %v", got)
	}
	// Pushing from the middle discards the forward entries.
	h.Push("/d")
	if i, n := h.Position(); i != 2 || n != 3 || h.Current().URL != "/d" {
		t.Errorf("after Push from entry 1: position %d of %d at %s, want 2 of 3 at /d", i, n, h.Current().URL)
	}
}

func TestPatternErrorsNameTheProblem(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"/a//b": "empty segment",
		"/:":    "unnamed parameter",
		"/a/*":  "unnamed catch-all",
	}
	for path, want := range cases {
		mustPanic(t, want, func() { New(Page(path, text(""))) })
	}
	for path, want := range map[string]string{"": "/", "/": "/", "a/:b/": "/a/:b", "/x/*rest": "/x/*rest"} {
		p, err := parsePattern(path)
		if err != nil {
			t.Errorf("parsePattern(%q): %v", path, err)
			continue
		}
		if got := patternString(p); got != want {
			t.Errorf("patternString(parsePattern(%q)) = %q, want %q", path, got, want)
		}
	}
}

// strayRoute is a Route the compiler does not know how to place.
type strayRoute struct{}

func (strayRoute) route() {}

func TestCompileRejectsMalformedTrees(t *testing.T) {
	t.Parallel()
	mustPanic(t, "nil route in /a", func() { New(Group("/a", nil)) })
	mustPanic(t, "nil layout at /a", func() { New(Group("/a", Layout("/b", nil))) })
	mustPanic(t, "nil not-found page at /a", func() { New(Group("/a", NotFound(nil))) })
	mustPanic(t, "unknown route node", func() { New(strayRoute{}) })
	mustPanic(t, `redirect /a → "a"`, func() { New(Page("/", text("")), Redirect("/a", "a")) })
	mustPanic(t, "unnamed parameter", func() { New(Group("/:", Page("/", text("")))) })
}

func TestRedirectChainsAndTheBuiltinNotFound(t *testing.T) {
	t.Parallel()
	r := New(Page("/c", text("c")), Redirect("/a", "/b"), Redirect("/b", "/c"))
	h := Memory("/a")
	r.History(h)
	p := ggui.NewProbe(r.View(), ggui.Sz(100, 100))
	defer p.Close()
	p.Frame()
	if got := r.Location().Path; got != "/c" || h.Current().URL != "/c" {
		t.Errorf("a two-step redirect landed on %s with history at %s, want /c for both", got, h.Current().URL)
	}
	if err := r.Navigate("/nowhere"); err != nil {
		t.Fatal(err)
	}
	if r.Current().Get() != nil {
		t.Error("an unknown URL without a NotFound matched a page")
	}
	if _, ok := p.Semantics().Find(ggui.RoleText, "Not found"); !ok {
		t.Error("the built-in not-found screen is not showing")
	}
}

func TestAViewMountsOnlyOnce(t *testing.T) {
	t.Parallel()
	r := New(Page("/", text("home")))
	first := ggui.NewProbe(r.View(), ggui.Sz(100, 100))
	defer first.Close()
	first.Frame()
	second := ggui.NewProbe(r.View(), ggui.Sz(100, 100))
	defer second.Close()
	mustPanic(t, "already mounted", func() { second.Frame() })
}
