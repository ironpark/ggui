package i18n

import (
	"encoding/json"
	"errors"
	"strings"
)

// Catalog is the translations of an app's text into one language, keyed by
// the source sentence. Write one as Go:
//
//	var ko = &i18n.Catalog{
//		Lang: "ko",
//		Messages: map[string]string{
//			"New sandbox":    "새 샌드박스",
//			"Delete %s?":     "%s{을/를} 삭제할까요?",
//			"Open [verb]":    "열기",
//			"%[1]s of %[2]s": "%[2]s의 %[1]s",
//		},
//		Plurals: map[string]i18n.Forms{
//			"%d sandbox running": {Other: "샌드박스 %d개 실행 중"},
//		},
//	}
//
// or as JSON, read with Parse, in the same shape:
//
//	{
//	  "lang": "ko",
//	  "messages": {"New sandbox": "새 샌드박스"},
//	  "plurals": {"%d sandbox running": {"other": "샌드박스 %d개 실행 중"}}
//	}
//
// A translation is a fmt format taking the same arguments as its source
// sentence; an explicit index such as %[2]s puts them in the order the
// language needs. A message given a Context is found under its sentence
// followed by a space and the context in brackets, "Open [verb]", and
// under the plain sentence when that is missing.
type Catalog struct {
	// Lang is the BCP 47 tag of the language, such as "ko" or "pt-BR".
	Lang string `json:"lang"`
	// Messages maps a source sentence to its translation.
	Messages map[string]string `json:"messages,omitempty"`
	// Plurals maps the singular source sentence of a plural message, the
	// one given to N, to its translated forms.
	Plurals map[string]Forms `json:"plurals,omitempty"`
}

// Forms is a plural message's translation for each CLDR plural category
// of its language. A language uses some of them: English One and Other,
// Korean Other alone, Arabic all six. A form left empty falls back to
// Other.
type Forms struct {
	Zero  string `json:"zero,omitempty"`
	One   string `json:"one,omitempty"`
	Two   string `json:"two,omitempty"`
	Few   string `json:"few,omitempty"`
	Many  string `json:"many,omitempty"`
	Other string `json:"other,omitempty"`
}

// pick returns the form for category c, or Other when that is empty.
func (f Forms) pick(c category) string {
	var s string
	switch c {
	case zero:
		s = f.Zero
	case one:
		s = f.One
	case two:
		s = f.Two
	case few:
		s = f.Few
	case many:
		s = f.Many
	}
	if s == "" {
		s = f.Other
	}
	return s
}

// Parse reads a catalog written as JSON, for one embedded in the binary:
//
//	//go:embed locales/ko.json
//	var koJSON []byte
//
//	ko, err := i18n.Parse(koJSON)
func Parse(data []byte) (*Catalog, error) {
	c := new(Catalog)
	if err := json.Unmarshal(data, c); err != nil {
		return nil, err
	}
	if strings.TrimSpace(c.Lang) == "" {
		return nil, errors.New("i18n: catalog has no lang")
	}
	return c, nil
}

// contextKey is the key a message with context ctx is found under.
func contextKey(key, ctx string) string { return key + " [" + ctx + "]" }
