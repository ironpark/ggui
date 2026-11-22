package hangul

import (
	"slices"
	"testing"
)

func TestKeysAreTheJamoTyped(t *testing.T) {
	for in, want := range map[rune]string{'셋': "ㅅㅔㅅ", '왜': "ㅇㅗㅐ", '닭': "ㄷㅏㄹㄱ", 'ㅘ': "ㅗㅏ", 'a': ""} {
		if got := string(Keys(nil, in)); got != want {
			t.Errorf("Keys(%q) = %q, want %q", in, got, want)
		}
	}
}

// A syllable still being typed takes what it will become; a lone
// consonant before it takes a syllable it begins.
func TestMatchTakesWhatIsBeingTyped(t *testing.T) {
	s := []rune("세션 닫기")
	for _, tc := range []struct {
		i    int
		q    rune
		last bool
		want int
	}{
		{0, '셋', true, 2},  // its final the next syllable's initial
		{0, '세', true, 1},  // the syllable itself
		{0, 'ㅅ', true, 1},  // its initial
		{3, '다', true, 1},  // 닫 begun
		{0, '사', true, 0},  // another vowel
		{0, 'ㅅ', false, 1}, // an initial before the last rune
		{0, 'ㄷ', false, 0},
	} {
		var last []rune
		if tc.last {
			last = Keys(nil, tc.q)
		}
		if got := Match(s, tc.i, tc.q, last); got != tc.want {
			t.Errorf("Match(%q at %d, %q, last %v) = %d, want %d", string(s), tc.i, tc.q, tc.last, got, tc.want)
		}
	}
}

func TestQwertyReadsTheKeysTyped(t *testing.T) {
	for in, want := range map[string]string{"ㄴㄷㅅ샤": "setti", "tab 닫": "tab eke", "plain": "plain"} {
		got, had := Qwerty(in)
		if got != want || had != (in != "plain") {
			t.Errorf("Qwerty(%q) = %q, %v", in, got, had)
		}
	}
	if !slices.Equal([]rune("ㄲ"), Keys(nil, 'ㄲ')) {
		t.Error("a doubled consonant is one key")
	}
}
