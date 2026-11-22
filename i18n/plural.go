package i18n

// category is a CLDR plural category.
type category uint8

const (
	other category = iota
	zero
	one
	two
	few
	many
)

// The CLDR cardinal plural rules of each language, for whole numbers: the
// category a count n, never negative, falls in. A language missing here
// has Other alone.
var pluralRules = map[string]func(n uint) category{
	// One for 1 alone.
	"en": oneIsOne, "de": oneIsOne, "nl": oneIsOne, "sv": oneIsOne,
	"da": oneIsOne, "nb": oneIsOne, "no": oneIsOne, "nn": oneIsOne,
	"fi": oneIsOne, "et": oneIsOne, "el": oneIsOne, "hu": oneIsOne,
	"tr": oneIsOne, "bg": oneIsOne, "ka": oneIsOne, "az": oneIsOne,
	"ur": oneIsOne, "sw": oneIsOne, "mr": oneIsOne, "ca": oneIsOne, "gl": oneIsOne,

	// One for 1 and many for millions: "1 millón de archivos".
	"es": oneMillions, "it": oneMillions, "pt-pt": oneMillions,

	// One for 0 and 1, many for millions.
	"fr": zeroOneMillions, "pt": zeroOneMillions,

	// One for 0 and 1.
	"hi": zeroOne, "bn": zeroOne, "fa": zeroOne, "gu": zeroOne, "kn": zeroOne,
	"zu": zeroOne, "am": zeroOne,

	// No distinction.
	"ko": none, "ja": none, "zh": none, "vi": none, "th": none, "id": none,
	"ms": none, "lo": none, "my": none, "km": none,

	"ru": eastSlavic, "uk": eastSlavic, "be": eastSlavic,
	"pl": polish,
	"cs": westSlavic, "sk": westSlavic,
	"lt": lithuanian,
	"lv": latvian,
	"ro": romanian,
	"hr": southSlavic, "sr": southSlavic, "bs": southSlavic,
	"sl": slovenian,
	"ar": arabic,
	"he": hebrew, "iw": hebrew,
	"ga": irish,
	"cy": welsh,
}

func none(uint) category { return other }

func oneIsOne(n uint) category {
	if n == 1 {
		return one
	}
	return other
}

func zeroOne(n uint) category {
	if n <= 1 {
		return one
	}
	return other
}

func oneMillions(n uint) category {
	switch {
	case n == 1:
		return one
	case n != 0 && n%1000000 == 0:
		return many
	}
	return other
}

func zeroOneMillions(n uint) category {
	switch {
	case n <= 1:
		return one
	case n%1000000 == 0:
		return many
	}
	return other
}

func eastSlavic(n uint) category {
	switch d, h := n%10, n%100; {
	case d == 1 && h != 11:
		return one
	case d >= 2 && d <= 4 && (h < 12 || h > 14):
		return few
	}
	return many
}

func polish(n uint) category {
	switch d, h := n%10, n%100; {
	case n == 1:
		return one
	case d >= 2 && d <= 4 && (h < 12 || h > 14):
		return few
	}
	return many
}

func westSlavic(n uint) category {
	switch {
	case n == 1:
		return one
	case n >= 2 && n <= 4:
		return few
	}
	return other
}

func lithuanian(n uint) category {
	switch d, h := n%10, n%100; {
	case d == 1 && (h < 11 || h > 19):
		return one
	case d >= 2 && (h < 11 || h > 19):
		return few
	}
	return other
}

func latvian(n uint) category {
	switch d, h := n%10, n%100; {
	case d == 0 || (h >= 11 && h <= 19):
		return zero
	case d == 1 && h != 11:
		return one
	}
	return other
}

func romanian(n uint) category {
	switch h := n % 100; {
	case n == 1:
		return one
	case n == 0 || (h >= 1 && h <= 19):
		return few
	}
	return other
}

func southSlavic(n uint) category {
	switch d, h := n%10, n%100; {
	case d == 1 && h != 11:
		return one
	case d >= 2 && d <= 4 && (h < 12 || h > 14):
		return few
	}
	return other
}

func slovenian(n uint) category {
	switch h := n % 100; {
	case h == 1:
		return one
	case h == 2:
		return two
	case h == 3 || h == 4:
		return few
	}
	return other
}

func arabic(n uint) category {
	switch h := n % 100; {
	case n == 0:
		return zero
	case n == 1:
		return one
	case n == 2:
		return two
	case h >= 3 && h <= 10:
		return few
	case h >= 11:
		return many
	}
	return other
}

func hebrew(n uint) category {
	switch n {
	case 1:
		return one
	case 2:
		return two
	}
	return other
}

func irish(n uint) category {
	switch {
	case n == 1:
		return one
	case n == 2:
		return two
	case n >= 3 && n <= 6:
		return few
	case n >= 7 && n <= 10:
		return many
	}
	return other
}

func welsh(n uint) category {
	switch n {
	case 0:
		return zero
	case 1:
		return one
	case 2:
		return two
	case 3:
		return few
	case 6:
		return many
	}
	return other
}

// pluralOf returns the category of n in the language tag: by the rule of
// the most specific of its fallbacks that has one, and Other in a language
// with none.
func pluralOf(tag string, n int) category {
	var u uint
	if n < 0 {
		u = uint(-(n + 1)) + 1
	} else {
		u = uint(n)
	}
	for _, t := range fallbacks(tag) {
		if rule, ok := pluralRules[t]; ok {
			return rule(u)
		}
	}
	return other
}
