package updater

import "strings"

// CompareVersion compares two version strings using Sparkle 2 / SemVer-compatible
// ordering and returns -1 if a < b, 0 if a == b, or +1 if a > b.
//
// Rules:
//   - Leading/trailing whitespace is trimmed; an optional leading 'v' or 'V'
//     before a digit is stripped; build metadata after '+' is ignored.
//   - Main version numeric components are compared numerically, with trailing
//     ".0" components treated as equal ("1.2" == "1.2.0").
//   - Both SemVer prereleases ("1.2.0-rc.1", "1.2.0-beta.2") and Sparkle-style
//     prereleases ("1.2a2", "1.2b1", "1.2rc1") are supported.
//   - Any prerelease compares lower than the corresponding release ("1.2.0-rc.1" < "1.2.0").
//   - Prerelease identifiers are compared numerically when both are integers,
//     otherwise lexically case-insensitively.
func CompareVersion(a, b string) int {
	na := normalizeVersionRaw(a)
	nb := normalizeVersionRaw(b)
	if na == "" && nb == "" {
		return 0
	}
	if na == "" {
		return -1
	}
	if nb == "" {
		return 1
	}

	va := parseVersion(na)
	vb := parseVersion(nb)

	maxCore := max(len(va.core), len(vb.core))
	for i := 0; i < maxCore; i++ {
		pa := "0"
		if i < len(va.core) {
			pa = va.core[i]
		}
		pb := "0"
		if i < len(vb.core) {
			pb = vb.core[i]
		}
		if c := compareDecimalStrings(pa, pb); c != 0 {
			return c
		}
	}

	// Main numeric versions are equal; compare prerelease tags.
	aPre := len(va.pre) > 0
	bPre := len(vb.pre) > 0
	switch {
	case !aPre && !bPre:
		return 0
	case aPre && !bPre:
		return -1
	case !aPre && bPre:
		return 1
	}

	minPre := min(len(va.pre), len(vb.pre))
	for i := 0; i < minPre; i++ {
		if c := comparePreToken(va.pre[i], vb.pre[i]); c != 0 {
			return c
		}
	}
	switch {
	case len(va.pre) < len(vb.pre):
		return -1
	case len(va.pre) > len(vb.pre):
		return 1
	default:
		return 0
	}
}

type parsedVersion struct {
	core []string
	pre  []string
}

func normalizeVersionRaw(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 1 && (s[0] == 'v' || s[0] == 'V') && isASCIIDigit(s[1]) {
		s = s[1:]
	}
	if idx := strings.IndexByte(s, '+'); idx >= 0 {
		s = s[:idx]
	}
	return strings.TrimSpace(s)
}

func parseVersion(s string) parsedVersion {
	left := s
	right := ""
	if idx := strings.IndexByte(s, '-'); idx >= 0 {
		left = s[:idx]
		right = s[idx+1:]
	}

	var core []string
	var inlinePre string

	rem := left
	for rem != "" {
		seg := rem
		rest := ""
		if dot := strings.IndexByte(rem, '.'); dot >= 0 {
			seg = rem[:dot]
			rest = rem[dot+1:]
		}
		if seg == "" {
			rem = rest
			continue
		}
		// Count leading digits in this segment.
		digits := 0
		for digits < len(seg) && isASCIIDigit(seg[digits]) {
			digits++
		}
		if digits == len(seg) {
			core = append(core, seg)
			rem = rest
			continue
		}
		if digits > 0 {
			core = append(core, seg[:digits])
			tail := seg[digits:]
			if rest != "" {
				inlinePre = tail + "." + rest
			} else {
				inlinePre = tail
			}
		} else {
			inlinePre = rem
		}
		break
	}

	preRaw := inlinePre
	if right != "" {
		if preRaw != "" {
			preRaw += "." + right
		} else {
			preRaw = right
		}
	}

	return parsedVersion{
		core: core,
		pre:  tokenizePrerelease(preRaw),
	}
}

// tokenizePrerelease splits a prerelease string on separators ('.', '-', '_',
// whitespace) and on boundaries between ASCII digits and non-digits so that
// both "rc.1" and "rc1" tokenize identically as ["rc", "1"].
func tokenizePrerelease(s string) []string {
	var tokens []string
	i := 0
	for i < len(s) {
		if isPreSeparator(s[i]) {
			i++
			continue
		}
		start := i
		wantDigit := isASCIIDigit(s[i])
		for i < len(s) && !isPreSeparator(s[i]) && isASCIIDigit(s[i]) == wantDigit {
			i++
		}
		tokens = append(tokens, s[start:i])
	}
	return tokens
}

func comparePreToken(a, b string) int {
	aNum := isAllDigits(a)
	bNum := isAllDigits(b)
	if aNum && bNum {
		return compareDecimalStrings(a, b)
	}
	return strings.Compare(strings.ToLower(a), strings.ToLower(b))
}

// compareDecimalStrings compares two non-empty ASCII digit strings numerically
// without integer overflow.
func compareDecimalStrings(a, b string) int {
	a = trimLeadingZeros(a)
	b = trimLeadingZeros(b)
	if len(a) != len(b) {
		if len(a) < len(b) {
			return -1
		}
		return 1
	}
	return strings.Compare(a, b)
}

func trimLeadingZeros(s string) string {
	i := 0
	for i < len(s)-1 && s[i] == '0' {
		i++
	}
	return s[i:]
}

func isASCIIDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isASCIIDigit(s[i]) {
			return false
		}
	}
	return true
}

func isPreSeparator(c byte) bool {
	return c == '.' || c == '-' || c == '_' || c == ' ' || c == '\t'
}
