package ggui

import (
	"slices"
	"testing"
)

func TestNormalizeLanguage(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"ko_KR.UTF-8@mod": "ko-KR",
		"ko_KR":           "ko-KR",
		"en-us":           "en-US",
		"zh_hant_tw":      "zh-Hant-TW",
		"es-419":          "es-419",
		"de":              "de",
		"sr_RS@latin":     "sr-RS",
		"C":               "",
		"C.UTF-8":         "",
		"POSIX":           "",
		"":                "",
		" fr_CA ":         "fr-CA", //nolint:gocritic // the spaces are what is tested
	} {
		if got := normalizeLanguage(in); got != want {
			t.Errorf("normalizeLanguage(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeLanguagesDropsRepeatsAndPOSIX(t *testing.T) {
	t.Parallel()
	got := normalizeLanguages([]string{"ko_KR.UTF-8", "C", "ko-KR", "en_US", "EN-us", ""})
	if want := []string{"ko-KR", "en-US"}; !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	if got := normalizeLanguages(nil); got == nil || len(got) != 0 {
		t.Fatalf("no languages = %#v, want an empty list", got)
	}
}

func TestPOSIXLanguages(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		env  map[string]string
		want []string
	}{
		{"language list then locale", map[string]string{"LANGUAGE": "ko_KR:ja::en", "LANG": "en_US.UTF-8"}, []string{"ko-KR", "ja", "en", "en-US"}},
		{"LC_ALL wins", map[string]string{"LC_ALL": "fr_FR.UTF-8", "LC_MESSAGES": "de_DE", "LANG": "en_US"}, []string{"fr-FR"}},
		{"LC_MESSAGES before LANG", map[string]string{"LC_MESSAGES": "de_DE", "LANG": "en_US"}, []string{"de-DE"}},
		{"POSIX locale ignores LANGUAGE", map[string]string{"LANGUAGE": "ko", "LANG": "C.UTF-8"}, []string{}},
		{"LANGUAGE alone", map[string]string{"LANGUAGE": "pt_BR"}, []string{"pt-BR"}},
		{"nothing", map[string]string{}, []string{}},
	} {
		got := normalizeLanguages(posixLanguages(func(k string) string { return c.env[k] }))
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestSystemLanguagesAreTags(t *testing.T) {
	t.Parallel()
	langs := SystemLanguages()
	if langs == nil {
		t.Fatal("SystemLanguages returned nil, want an empty list at least")
	}
	for _, l := range langs {
		if normalizeLanguage(l) != l {
			t.Errorf("%q is not a normalized tag", l)
		}
	}
	t.Logf("system languages: %q", langs)
}
