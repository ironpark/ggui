package i18n

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/ironpark/ggui"
)

var ko = &Catalog{
	Lang: "ko",
	Messages: map[string]string{
		"New sandbox":    "새 샌드박스",
		"Delete %s?":     "%s{을/를} 삭제할까요?",
		"Open":           "열림",
		"Open [verb]":    "열기",
		"%[1]s of %[2]s": "%[2]s의 %[1]s",
		"Moved to %s":    "%s{으로/로} 옮겼습니다",
		"Trash":          "휴지통",
	},
	Plurals: map[string]Forms{
		"%d sandbox running": {Other: "샌드박스 %d개 실행 중"},
	},
}

var pt = &Catalog{
	Lang:     "pt",
	Messages: map[string]string{"New sandbox": "Nova sandbox"},
}

var ptBR = &Catalog{
	Lang:     "pt-BR",
	Messages: map[string]string{"Trash": "Lixeira"},
}

var ru = &Catalog{
	Lang: "ru",
	Plurals: map[string]Forms{
		"%d file": {One: "%d файл", Few: "%d файла", Many: "%d файлов"},
	},
}

func TestMessagesResolveAndFallBack(t *testing.T) {
	t.Parallel()
	lang := ggui.State("ko")
	l := New(lang, ko, pt, ptBR, ru)
	cases := []struct {
		lang string
		m    Message
		want string
	}{
		{"ko", T("New sandbox"), "새 샌드박스"},
		{"ko-KR", T("New sandbox"), "새 샌드박스"},
		{"ko", T("Not translated %d", 3), "Not translated 3"},
		{"ko", T("Open"), "열림"},
		{"ko", T("Open").Context("verb"), "열기"},
		{"ko", T("Trash").Context("noun"), "휴지통"}, // no context entry: the plain key
		{"ko", T("%[1]s of %[2]s", "a", "b"), "b의 a"},
		{"ko", T("Moved to %s", T("Trash")), "휴지통으로 옮겼습니다"},
		{"ko", T("Delete %s?", ggui.State("사과")), "사과를 삭제할까요?"},
		{"ko", T("100%"), "100%"},
		{"pt-BR", T("New sandbox"), "Nova sandbox"}, // base language
		{"pt_BR", T("Trash"), "Lixeira"},
		{"pt", T("Trash"), "Trash"},
		{"de", T("New sandbox"), "New sandbox"},
		{"", T("Delete %s?", "x"), "Delete x?"},
		{"ko", N(3, "%d sandbox running", "%d sandboxes running"), "샌드박스 3개 실행 중"},
		{"de", N(1, "%d sandbox running", "%d sandboxes running"), "1 sandbox running"},
		{"de", N(2, "%d sandbox running", "%d sandboxes running"), "2 sandboxes running"},
		{"ru", N(1, "%d file", "%d files"), "1 файл"},
		{"ru", N(3, "%d file", "%d files"), "3 файла"},
		{"ru", N(11, "%d file", "%d files"), "11 файлов"},
		{"ru", N(21, "%d file", "%d files"), "21 файл"},
		{"ru", N(25, "%d file", "%d files"), "25 файлов"},
	}
	for _, c := range cases {
		lang.Set(c.lang)
		if got := l.message(c.m); got != c.want {
			t.Errorf("%q in %s = %q, want %q", c.m.String(), c.lang, got, c.want)
		}
	}
	lang.Set("ko")
	if got := l.S("Delete %s?", "수박"); got != "수박을 삭제할까요?" {
		t.Errorf("S = %q", got)
	}
	if got := l.N(2, "%d sandbox running", "%d sandboxes running"); got != "샌드박스 2개 실행 중" {
		t.Errorf("N = %q", got)
	}
	if got := T("Delete %s?", T("Trash")).String(); got != "Delete Trash?" {
		t.Errorf("String = %q, want the source language", got)
	}
}

func TestAMissingFormFallsBackToOtherThenToTheSource(t *testing.T) {
	t.Parallel()
	ar := &Catalog{Lang: "ar", Plurals: map[string]Forms{"%d day": {Zero: "لا أيام", Other: "%d يوم"}}}
	cs := &Catalog{Lang: "cs", Plurals: map[string]Forms{"%d day": {One: "%d den"}}}
	lang := ggui.State("ar")
	l := New(lang, ar, cs)
	for _, c := range []struct {
		lang string
		n    int
		want string
	}{
		{"ar", 0, "لا أيام"},
		{"ar", 5, "5 يوم"}, // few, missing: Other
		{"cs", 1, "1 den"},
		{"cs", 3, "3 days"}, // neither few nor other: the source
	} {
		lang.Set(c.lang)
		if got := l.N(c.n, "%d day", "%d days"); got != c.want {
			t.Errorf("%s %d = %q, want %q", c.lang, c.n, got, c.want)
		}
	}
}

func TestPluralCategories(t *testing.T) {
	t.Parallel()
	names := map[category]string{other: "other", zero: "zero", one: "one", two: "two", few: "few", many: "many"}
	cases := map[string]map[int]category{
		"en":    {0: other, 1: one, 2: other},
		"de":    {1: one, 5: other},
		"nl":    {1: one, 2: other},
		"sv":    {1: one, 0: other},
		"it":    {1: one, 2: other, 1000000: many},
		"es":    {1: one, 0: other, 2000000: many},
		"pt":    {0: one, 1: one, 2: other, 1000000: many},
		"pt-PT": {0: other, 1: one},
		"fr":    {0: one, 1: one, 2: other, 3000000: many},
		"ko":    {1: other, 2: other},
		"ja":    {1: other},
		"zh-TW": {1: other},
		"vi":    {1: other},
		"th":    {1: other},
		"id":    {1: other},
		"tr":    {1: one, 2: other},
		"ru":    {1: one, 2: few, 5: many, 11: many, 12: many, 21: one, 22: few, 111: many},
		"uk":    {1: one, 4: few, 14: many},
		"pl":    {1: one, 2: few, 5: many, 12: many, 21: many, 22: few},
		"cs":    {1: one, 3: few, 5: other},
		"ar":    {0: zero, 1: one, 2: two, 3: few, 10: few, 11: many, 99: many, 100: other, 102: other, 103: few},
		"he":    {1: one, 2: two, 3: other, 20: other},
		"xx":    {1: other},
	}
	for lang, want := range cases {
		for n, c := range want {
			if got := pluralOf(lang, n); got != c {
				t.Errorf("%s %d is %s, want %s", lang, n, names[got], names[c])
			}
		}
	}
}

func TestJosa(t *testing.T) {
	t.Parallel()
	cases := []struct{ word, a, b, want string }{
		{"사과", "을", "를", "를"},
		{"수박", "을", "를", "을"},
		{"책", "이", "가", "이"},
		{"나무", "이", "가", "가"},
		{"산", "은", "는", "은"},
		{"바다", "은", "는", "는"},
		{"빵", "과", "와", "과"},
		{"우유", "과", "와", "와"},
		{"서울", "으로", "로", "로"}, // ㄹ takes 로
		{"부산", "으로", "로", "으로"},
		{"학교", "으로", "로", "로"},
		{"물", "을", "를", "을"},     // ㄹ is a final consonant for 을
		{"3", "이", "가", "이"},     // 삼
		{"2", "이", "가", "가"},     // 이
		{"1", "으로", "로", "로"},    // 일
		{"10", "을", "를", "을"},    // 십
		{"1,000", "을", "를", "을"}, // 천
		{"20000", "은", "는", "은"}, // 만
		{"0", "을", "를", "을"},     // 영
		{"file", "을", "를", "를"},
		{"「사과」", "을", "를", "를"},
		{"", "을", "를", "를"},
	}
	for _, c := range cases {
		if got := Josa(c.word, c.a, c.b); got != c.want {
			t.Errorf("Josa(%q, %q, %q) = %q, want %q", c.word, c.a, c.b, got, c.want)
		}
	}
}

func TestKoreanFormatsPickParticles(t *testing.T) {
	t.Parallel()
	cases := []struct {
		format string
		args   []any
		want   string
	}{
		{"%s{을/를} 지웠습니다", []any{"파일"}, "파일을 지웠습니다"},
		{"%s{이/가} %s{을/를} 샀다", []any{"철수", "사과"}, "철수가 사과를 샀다"},
		{"%[2]s{은/는} %[1]d개", []any{3, "사과"}, "사과는 3개"},
		{"%d{이/가}", []any{3}, "3이"},
		{"%5.1f%%{을/를}", []any{1.25}, "  1.2%{을/를}"}, // a particle follows a verb only
		{"{을/를} 그대로", nil, "{을/를} 그대로"},
		{"%d{을/를", []any{3}, "3{을/를"},
		{"%*d{을/를}", []any{3, 1}, "  1을(를)"},
		{"%s %s", []any{"하나"}, "하나 %!s(MISSING)"},
	}
	for _, c := range cases {
		var got string
		if c.args == nil {
			got = c.format
		} else {
			got = sprintfKorean(c.format, c.args)
		}
		if got != c.want {
			t.Errorf("%q %v = %q, want %q", c.format, c.args, got, c.want)
		}
	}
	// Particles apply to Korean only.
	lang := ggui.State("en")
	l := New(lang, &Catalog{Lang: "en", Messages: map[string]string{"x %s": "%s{a/b}"}})
	if got := l.S("x %s", "y"); got != "y{a/b}" {
		t.Errorf("English got particles: %q", got)
	}
}

func TestFormats(t *testing.T) {
	ref := time.Date(2026, time.November, 18, 12, 0, 0, 0, time.UTC)
	now = func() time.Time { return ref }
	defer func() { now = time.Now }()
	de := &Catalog{Lang: "de", Messages: map[string]string{
		". [decimal separator]": ",",
		", [group separator]":   ".",
		"Jan 2, 2006":           "2. Jan 2006",
		"Nov [month]":           "Nov.",
	}}
	lang := ggui.State("en")
	l := New(lang, de)
	cases := []struct {
		lang string
		m    Message
		want string
	}{
		{"en", Since(ref.Add(-30 * time.Second)), "just now"},
		{"en", Since(ref.Add(-time.Minute)), "1 minute ago"},
		{"en", Since(ref.Add(-5 * 24 * time.Hour)), "5 days ago"},
		{"en", Since(ref.Add(3 * time.Hour)), "in 3 hours"},
		{"en", Since(ref.Add(-400 * 24 * time.Hour)), "1 year ago"},
		{"ko", Since(ref.Add(-5 * 24 * time.Hour)), "5일 전"},
		{"ko", Since(ref.Add(2 * 7 * 24 * time.Hour)), "2주 후"},
		{"ko", Since(ref), "방금"},
		{"de", Since(ref.Add(-time.Hour)), "1 hour ago"},
		{"en", Bytes(512), "512 bytes"},
		{"en", Bytes(1), "1 byte"},
		{"en", Bytes(1500), "1.5 KB"},
		{"en", Bytes(23_400_000), "23 MB"},
		{"en", Bytes(999_960), "1 MB"},
		{"de", Bytes(1500), "1,5 KB"},
		{"ko", Bytes(512), "512바이트"},
		{"en", Number(1234567), "1,234,567"},
		{"en", Number(-1234.5), "-1,234.5"},
		{"en", Number(uint8(12)), "12"},
		{"de", Number(1234567.25), "1.234.567,25"},
		{"ko", Number(1000), "1,000"},
		{"en", Date(ref, DateShort), "11/18/2026"},
		{"en", Date(ref, DateMedium), "Nov 18, 2026"},
		{"en", Date(ref, DateLong), "Wednesday, November 18, 2026"},
		{"ko", Date(ref, DateShort), "2026. 11. 18."},
		{"ko", Date(ref, DateMedium), "2026년 11월 18일"},
		{"ko", Date(ref, DateLong), "2026년 11월 18일 수요일"},
		{"de", Date(ref, DateMedium), "18. Nov. 2026"},
	}
	for _, c := range cases {
		lang.Set(c.lang)
		if got := l.message(c.m); got != c.want {
			t.Errorf("%s in %s = %q, want %q", c.m.String(), c.lang, got, c.want)
		}
	}
	if got := T("Size: %s", Bytes(2048)).String(); got != "Size: 2 KB" {
		t.Errorf("a format as an argument = %q", got)
	}
}

func TestParseReadsTheCatalogShape(t *testing.T) {
	t.Parallel()
	c, err := Parse([]byte(`{"lang":"ko","messages":{"New sandbox":"새 샌드박스"},"plurals":{"%d sandbox running":{"other":"샌드박스 %d개"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if c.Lang != "ko" || c.Messages["New sandbox"] != "새 샌드박스" || c.Plurals["%d sandbox running"].Other != "샌드박스 %d개" {
		t.Fatalf("Parse = %+v", c)
	}
	if _, err := Parse([]byte(`{"messages":{}}`)); err == nil {
		t.Error("a catalog without lang parsed")
	}
	if _, err := Parse([]byte(`{`)); err == nil {
		t.Error("bad JSON parsed")
	}
}

func TestMatch(t *testing.T) {
	t.Parallel()
	supported := []string{"en", "ko", "pt-BR", "zh-Hant", "fr"}
	cases := []struct {
		preferred []string
		want      string
	}{
		{[]string{"ko-KR", "en"}, "ko"},
		{[]string{"pt_BR"}, "pt-BR"},
		{[]string{"pt-PT"}, "pt-BR"},
		{[]string{"zh-Hant-TW"}, "zh-Hant"},
		{[]string{"de", "fr-CA"}, "fr"},
		{[]string{"de"}, "en"},
		{nil, "en"},
		{[]string{"en-GB", "ko"}, "en"},
	}
	for _, c := range cases {
		if got := Match(supported, c.preferred); got != c.want {
			t.Errorf("Match(%v) = %q, want %q", c.preferred, got, c.want)
		}
	}
	if Match(nil, []string{"ko"}) != "" {
		t.Error("Match with nothing supported is not empty")
	}
}

func TestDirection(t *testing.T) {
	t.Parallel()
	for tag, want := range map[string]ggui.TextDirection{
		"ar": ggui.DirRTL, "he-IL": ggui.DirRTL, "fa": ggui.DirRTL, "ur": ggui.DirRTL,
		"en": ggui.DirLTR, "ko": ggui.DirLTR, "": ggui.DirLTR,
		"az-Arab": ggui.DirRTL, "ku-Latn": ggui.DirLTR,
	} {
		if got := Direction(tag); got != want {
			t.Errorf("Direction(%q) = %v, want %v", tag, got, want)
		}
	}
}

func TestMessagesWithoutArgumentsCompareEqual(t *testing.T) {
	t.Parallel()
	a, b := any(T("New sandbox")), any(T("New sandbox"))
	c, d := any(T("Open").Context("verb")), any(T("Open").Context("verb"))
	if a != b || c != d {
		t.Error("equal messages without arguments differ")
	}
	if T("Open") == T("Open").Context("verb") {
		t.Error("a context does not tell messages apart")
	}
	var _ ggui.Readable[string] = T("x")
}

func TestLocalizerRegressions(t *testing.T) {
	t.Parallel()
	// Nil *Localizer does not panic.
	var nilLoc *Localizer
	if got := nilLoc.S("Hello %s", "world"); got != "Hello world" {
		t.Errorf("nil.S = %q", got)
	}
	if got := nilLoc.N(-1, "%d item", "%d items"); got != "-1 item" {
		t.Errorf("nil.N(-1) = %q, want %q", got, "-1 item")
	}

	// Empty plural entry in specific catalog falls back to base catalog,
	// using the matched catalog's plural rules.
	basePT := &Catalog{Lang: "pt", Plurals: map[string]Forms{"%d file": {One: "%d arquivo", Other: "%d arquivos"}}}
	emptyPTPT := &Catalog{Lang: "pt-PT", Plurals: map[string]Forms{"%d file": {Other: ""}}}
	lang := ggui.State("pt-PT")
	l := New(lang, basePT, emptyPTPT)
	// pt-PT entry is empty -> falls back to pt (where 0 is one).
	if got := l.N(0, "%d file", "%d files"); got != "0 arquivo" {
		t.Errorf("pt-PT empty plural fallback = %q, want %q", got, "0 arquivo")
	}

	// base() normalizes case and underscores.
	if got := base("KO_KR"); got != "ko" {
		t.Errorf("base(KO_KR) = %q, want ko", got)
	}

	// Romanian 101..119 is few; 1 is one.
	if got := pluralOf("ro", 1); got != one {
		t.Errorf("ro 1 = %v, want one", got)
	}
	if got := pluralOf("ro", 101); got != few {
		t.Errorf("ro 101 = %v, want few", got)
	}
	if got := pluralOf("ro", 20); got != other {
		t.Errorf("ro 20 = %v, want other", got)
	}

	// math.MinInt does not overflow negative in pluralOf.
	if got := pluralOf("fr", math.MinInt); got != other {
		t.Errorf("fr MinInt = %v, want other", got)
	}

	// with() resets language and direction when lang switches back to "".
	arEnv := with(ggui.Env{}, l, "ar")
	if arEnv.Direction() != ggui.DirRTL || arEnv.Language() != "ar" {
		t.Fatalf("ar env = (%q, %v)", arEnv.Language(), arEnv.Direction())
	}
	resetEnv := with(arEnv, l, "")
	if resetEnv.Direction() != ggui.DirLTR || resetEnv.Language() != "" {
		t.Errorf("reset env = (%q, %v), want (\"\", DirLTR)", resetEnv.Language(), resetEnv.Direction())
	}
}

func TestSinceFarFutureNoOverflow(t *testing.T) {
	ref := time.Date(2026, time.November, 18, 12, 0, 0, 0, time.UTC)
	now = func() time.Time { return ref }
	defer func() { now = time.Now }()
	l := New(ggui.State("en"))
	// More than 292 years ahead saturates Sub to math.MinInt64.
	got := l.message(Since(ref.AddDate(500, 0, 0)))
	if !strings.HasPrefix(got, "in ") || !strings.HasSuffix(got, " years") {
		t.Errorf("Since(+500y) = %q, want 'in N years'", got)
	}
}
