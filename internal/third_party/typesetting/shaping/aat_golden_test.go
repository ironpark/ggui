//go:build darwin

package shaping

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/ironpark/ggui/internal/third_party/typesetting/di"
	"github.com/ironpark/ggui/internal/third_party/typesetting/font"
	ot "github.com/ironpark/ggui/internal/third_party/typesetting/font/opentype"
	"github.com/ironpark/ggui/internal/third_party/typesetting/language"
	"golang.org/x/image/math/fixed"
)

var update = flag.Bool("update", false, "rewrite the golden shaping of testdata/aat_golden.txt")

// mappedFile is a font file mapped into memory, lending its bytes, as the
// files ggui reads its system fonts from are.
type mappedFile struct {
	*bytes.Reader
	b []byte
}

func (m mappedFile) Slice(off, n int64) ([]byte, bool) {
	if off < 0 || n < 0 || off+n > int64(len(m.b)) {
		return nil, false
	}
	return m.b[off : off+n : off+n], true
}

// Permanent implements opentype.PermanentSlicer: the file is never unmapped.
func (mappedFile) Permanent() bool { return true }

// mapFaces maps the font file at path, never unmapped, and parses its faces.
func mapFaces(t testing.TB, path string) []*font.Face {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Skip(err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	b, err := syscall.Mmap(int(f.Fd()), 0, int(st.Size()), syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		t.Fatal(err)
	}
	faces, err := font.ParseTTC(mappedFile{bytes.NewReader(b), b})
	if err != nil {
		t.Fatal(err)
	}
	return faces
}

// aatCorpus is what the AAT fonts ggui falls back on are shaped with, each
// font with text its 'morx' table works on.
var aatCorpus = []struct {
	path     string
	script   language.Script
	rtl      bool
	features []FontFeature
	lines    []string
}{
	{"/System/Library/Fonts/AppleSDGothicNeo.ttc", language.Hangul, false, nil, hangulLines},
	{"/System/Library/Fonts/Supplemental/AppleGothic.ttf", language.Hangul, false, nil, hangulLines},
	{"/System/Library/Fonts/Menlo.ttc", language.Latin, false, nil, menloLines},
	// Menlo's ligatures, fi and fl, are off unless asked for.
	{"/System/Library/Fonts/Menlo.ttc", language.Latin, false, []FontFeature{{Tag: ot.MustNewTag("liga"), Value: 1}}, menloLines},
	// Geeza Pro joins Arabic with its 'morx' table, and kerns it with a
	// state table of the old 'kern' format.
	{"/System/Library/Fonts/GeezaPro.ttc", language.Arabic, true, nil, []string{
		"بسم الله الرحمن الرحيم",
		"مرحبا بالعالم، كيف حالك؟",
		"لا إله إلا الله ﷲ لأ لإ لآ",
		"عَرَبِيٌّ مُشَكَّلٌ",
		"١٢٣ ٤٥٦ 123 كتب يكتب مكتوب",
	}},
	{"/System/Library/Fonts/Apple Color Emoji.ttc", language.Common, false, nil, []string{
		"👨‍👩‍👧‍👦 👨‍👩‍👦 👩‍👩‍👧 👨‍👧 👪",
		"👩🏽‍💻 🧑🏿‍🚀 👨🏻‍🍳 👩🏼‍🔬 🧑‍🎄",
		"🇰🇷🇺🇸🇯🇵🇫🇷🇩🇪🇬🇧🇺🇳🇪🇺",
		"👍🏻👍🏼👍🏽👍🏾👍🏿 ✌🏽 🤌🏿 👋🏻",
		"1️⃣ 2️⃣ #️⃣ *️⃣ 0️⃣ 🔟",
		"🏳️‍🌈 🏳️‍⚧️ 🏴‍☠️ 🏴󠁧󠁢󠁳󠁣󠁴󠁿 🏴󠁧󠁢󠁥󠁮󠁧󠁿",
		"❤️‍🔥 ❤️‍🩹 😮‍💨 😵‍💫 🐻‍❄️ 🐦‍⬛",
		"🧑‍🤝‍🧑 👨🏻‍🤝‍👨🏿 👩🏾‍🤝‍👨🏼 🫱🏻‍🫲🏿 💏🏽 👩🏻‍❤️‍👨🏾 👩‍❤️‍💋‍👩",
		"😀🙂☺️☺ ✨ ⭐ ❤ ❤️ 🙂‍↔️ 🙂‍↕️",
		"🏃‍♀️ 🏃🏽‍♂️ 🏃‍➡️ 🧎🏻‍♀️‍➡️ 👮🏿‍♀️",
	}},
}

// menloLines are the dingbats a terminal draws in Menlo, code, and
// letters with marks its 'morx' table composes.
var menloLines = []string{
	"❯ ✳ ✻ ✶ ✢ ✽ ⏺ ⎿ ● ○ ◐ ✓ ✗ ⚠",
	"func main() { return a != b && c <= d || e >= f }",
	"x := <-ch; y -> z => w === v !== u /* c */ // c",
	"0x1F 0o17 1_000 ffi fl fi ... ::",
	"┌─┬─┐│ ├─┼─┤ └─┴─┘ ━┃ ╭╮╰╯ ▏▎▍▌▋▊▉█",
	"→ ← ↑ ↓ ⇒ ⇐ ↵ ⌘ ⌥ ⇧ ⌃ ⎋",
	"àáâãäå çèéêë ñ ø ß ı ĳ œ ŀ",
	"e\u0301 a\u0308 o\u0302\u0323 n\u0303 i\u0307\u0301",
	"Ǆ ǅ ǆ Ǳ ǲ ǳ ﬀ ﬁ ﬂ ﬃ ﬄ",
	"Привет мир Γειά σου κόσμε",
}

// hangulLines are modern Korean, and the conjoining jamo of old Hangul,
// which the fonts' 'morx' tables, not a composed character, set as one.
var hangulLines = []string{
	"안녕하세요 모든 사람에게 한국어 문장을 조판합니다",
	"가나다라마바사아자차카타파하 ㄱㄴㄷ ㅏㅑㅓ",
	"\u1100\u1161\u11A8 \u1112\u1161\u11AB \u1100\u1173\u11AF",
	"나랏말\u110A\u119E미 \u1103\u1172\u11F0\u1100\u1171\u11A8에 달아",
	"문\u110D\u119E\u11BC와로 서르 \u1109\u119E\u1106\u119E\u11BA디 아니\u1112\u119E\u11AF\u110A\u11A1",
	"\u1100\u119E \u1140\u1161 \u114C\u1173 \u1159\u1175 \u112B\u1161\u11F9 \u1117\u1175",
	"\uA960\u1161 \uA97C\u1169 \u1100\uD7B0 \u1100\u1161\uD7CB \u1100\u1161\uD7FB",
	"\u1100\u1161\u302E \u1102\u1161\u302F \u1100\u119E\u11A8\u302E",
	"\u115F\u1161 \u1100\u1160 \u115F\u1160\u11A8 \u1100 \u1161 \u11A8",
	"\u1100\u1161\u1100\u1161\u11A8\u11A8 \u1100\u1100\u1161 \u1161\u1161",
	"ᄒᆞᆫ글 ᄃᆞᆯ ᄆᆞᄋᆞᆷ ᄫᆡ ᅀᅵ ᄬ ᅘ ᇂ",
	"Hangul 한글 mixed 한자 漢字 123",
}

const goldenPath = "testdata/aat_golden.txt"

// TestAATShapingGolden shapes text in the AAT fonts ggui falls back on, and
// checks the glyphs, clusters, advances and offsets against those recorded
// before the 'morx' tables were read in place. go test -update records
// them anew.
//
// The golden file begins with each font file's hash, and only the lines of
// the fonts the system has as they were recorded are checked: macOS 26, for
// one, sets Apple SD Gothic Neo with 'GSUB' and no 'morx' table.
func TestAATShapingGolden(t *testing.T) {
	got := fontHashes(t) + shapeAATCorpus(t)
	if *update {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatal(err)
	}
	if got == string(want) {
		return
	}
	gl, wl := strings.Split(got, "\n"), strings.Split(string(want), "\n")
	recorded := map[string]bool{}
	for _, l := range wl {
		if strings.HasPrefix(l, "# ") {
			recorded[l] = true
		}
	}
	var changed []string
	for _, l := range gl {
		if strings.HasPrefix(l, "# ") && !recorded[l] {
			name, _, _ := strings.Cut(strings.TrimPrefix(l, "# "), "\t")
			changed = append(changed, name+" ")
		}
	}
	if len(changed) > 0 {
		t.Logf("not checking the fonts other than recorded: %q", changed)
		gl, wl = keepLines(gl, changed), keepLines(wl, changed)
	}
	bad := 0
	for i := 0; i < max(len(gl), len(wl)); i++ {
		var g, w string
		if i < len(gl) {
			g = gl[i]
		}
		if i < len(wl) {
			w = wl[i]
		}
		if g != w {
			t.Errorf("line %d:\n got %s\nwant %s", i+1, g, w)
			if bad++; bad == 10 {
				t.Fatal("too many differences")
			}
		}
	}
}

// fontHashes are the hashes of the corpus's font files, a line each.
func fontHashes(t *testing.T) string {
	var b strings.Builder
	seen := map[string]bool{}
	for _, c := range aatCorpus {
		if seen[c.path] {
			continue
		}
		seen[c.path] = true
		data, err := os.ReadFile(c.path)
		if err != nil {
			t.Skip(err)
		}
		fmt.Fprintf(&b, "# %s\t%x\n", filepath.Base(c.path), sha256.Sum256(data))
	}
	return b.String()
}

// keepLines are lines but those of the fonts named by a prefix of changed,
// and the fonts' hashes.
func keepLines(lines, changed []string) []string {
	var out []string
	for _, l := range lines {
		if strings.HasPrefix(l, "# ") {
			continue
		}
		keep := true
		for _, p := range changed {
			if strings.HasPrefix(l, p) {
				keep = false
				break
			}
		}
		if keep {
			out = append(out, l)
		}
	}
	return out
}

// glyphString is o's glyphs: ID, cluster, advance and offsets each.
func glyphString(o Output) string {
	var b strings.Builder
	for _, g := range o.Glyphs {
		fmt.Fprintf(&b, " %d/%d/%d/%d/%d", g.GlyphID, g.ClusterIndex, g.XAdvance, g.XOffset, g.YOffset)
	}
	return b.String()
}

func shapeAATCorpus(t *testing.T) string {
	var out bytes.Buffer
	w := bufio.NewWriter(&out)
	var shaper HarfbuzzShaper
	mapped := map[string][]*font.Face{}
	transformed := map[string]int{}
	var order []string
	for _, c := range aatCorpus {
		faces, ok := mapped[c.path]
		if !ok {
			faces = mapFaces(t, c.path)
			mapped[c.path] = faces
		}
		for fi, face := range faces {
			if len(face.Morx) == 0 {
				continue // as some of Geeza Pro's faces
			}
			key := fmt.Sprintf("%s %d", filepath.Base(c.path), fi)
			if _, ok := transformed[key]; !ok {
				order = append(order, key)
				transformed[key] = 0
			}
			// The same font with no 'morx' table, to tell the lines the
			// table works on.
			plain := *face.Font
			plain.Morx = nil
			plainFace := font.NewFace(&plain)
			var feats []string
			for _, f := range c.features {
				feats = append(feats, fmt.Sprintf("+%s", f.Tag))
			}
			dir := di.DirectionLTR
			if c.rtl {
				dir = di.DirectionRTL
			}
			for li, line := range c.lines {
				text := []rune(line)
				in := Input{
					Text:         text,
					RunStart:     0,
					RunEnd:       len(text),
					Direction:    dir,
					Face:         face,
					FontFeatures: c.features,
					Size:         fixed.I(1000),
					Script:       c.script,
				}
				gs := glyphString(shaper.ShapeNoExtents(in))
				in.Face = plainFace
				mark := " "
				if gs != glyphString(shaper.ShapeNoExtents(in)) {
					mark = "*"
					transformed[key]++
				}
				fmt.Fprintf(w, "%s%s %d%s%s\n", key, strings.Join(feats, ""), li, mark, gs)
			}
		}
	}
	// The corpus is to exercise the table: some of it must be set
	// otherwise with it than without.
	for _, key := range order {
		if transformed[key] == 0 {
			t.Errorf("%s: no line the morx table changes", key)
		}
	}
	w.Flush()
	return out.String()
}
