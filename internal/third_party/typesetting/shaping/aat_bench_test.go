//go:build darwin

package shaping

import (
	"testing"

	"github.com/ironpark/ggui/internal/third_party/typesetting/di"
	"github.com/ironpark/ggui/internal/third_party/typesetting/language"
	"golang.org/x/image/math/fixed"
)

// BenchmarkAATShaping shapes text in the AAT fonts ggui falls back on, its
// 'morx' tables driving most of the work, and reports the time a rune.
func BenchmarkAATShaping(b *testing.B) {
	for _, c := range []struct {
		name, path string
		script     language.Script
		text       string
	}{
		{"SDGothicNeo", "/System/Library/Fonts/AppleSDGothicNeo.ttc", language.Hangul, "안녕하세요 모든 사람에게 한국어 문장을 조판합니다 가나다라마바사아자차카타파하"},
		{"SDGothicNeo-jamo", "/System/Library/Fonts/AppleSDGothicNeo.ttc", language.Hangul, "나랏말ᄊᆞ미 듀ᇰ귁에 달아 문ᄍᆞᆼ와로 서르 ᄉᆞᄆᆞᆺ디"},
		{"Menlo", "/System/Library/Fonts/Menlo.ttc", language.Latin, "❯ ✳ ✻ func main() { return a != b && c <= d }"},
		{"AppleColorEmoji", "/System/Library/Fonts/Apple Color Emoji.ttc", language.Common, "👍🏽👨‍👩‍👧‍👦🇰🇷😀🙂1️⃣🏳️‍🌈"},
	} {
		b.Run(c.name, func(b *testing.B) {
			face := mapFaces(b, c.path)[0]
			text := []rune(c.text)
			in := Input{Text: text, RunEnd: len(text), Direction: di.DirectionLTR, Face: face, Size: fixed.I(16), Script: c.script}
			var shaper HarfbuzzShaper
			shaper.ShapeNoExtents(in)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				shaper.ShapeNoExtents(in)
			}
			b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*len(text)), "ns/rune")
		})
	}
}
