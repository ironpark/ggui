// Package plist writes XML property lists, the format of a bundle's
// Info.plist, from Go values.
package plist

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
)

const header = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
`

// Encode writes v as a property list. v and what it holds may be strings,
// booleans, integers, floats, slices and maps with string keys; a map's
// keys are written sorted, so the same values always make the same file.
func Encode(w io.Writer, v any) error {
	var b bytes.Buffer
	b.WriteString(header)
	if err := value(&b, v, 0); err != nil {
		return err
	}
	b.WriteString("</plist>\n")
	_, err := w.Write(b.Bytes())
	return err
}

// Marshal returns v as a property list.
func Marshal(v any) ([]byte, error) {
	var b bytes.Buffer
	err := Encode(&b, v)
	return b.Bytes(), err
}

func value(b *bytes.Buffer, v any, depth int) error {
	indent := strings.Repeat("\t", depth)
	switch v := v.(type) {
	case string:
		b.WriteString(indent + "<string>")
		xml.EscapeText(b, []byte(v))
		b.WriteString("</string>\n")
	case bool:
		if v {
			b.WriteString(indent + "<true/>\n")
		} else {
			b.WriteString(indent + "<false/>\n")
		}
	case int:
		fmt.Fprintf(b, "%s<integer>%d</integer>\n", indent, v)
	case int64:
		fmt.Fprintf(b, "%s<integer>%d</integer>\n", indent, v)
	case float64:
		if math.IsInf(v, 0) || math.IsNaN(v) {
			return fmt.Errorf("plist: %v has no plist form", v)
		}
		fmt.Fprintf(b, "%s<real>%s</real>\n", indent, strconv.FormatFloat(v, 'g', -1, 64))
	case []string:
		return array(b, len(v), func(i int) any { return v[i] }, depth)
	case []any:
		return array(b, len(v), func(i int) any { return v[i] }, depth)
	case []map[string]any:
		return array(b, len(v), func(i int) any { return v[i] }, depth)
	case map[string]any:
		if len(v) == 0 {
			b.WriteString(indent + "<dict/>\n")
			return nil
		}
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteString(indent + "<dict>\n")
		for _, k := range keys {
			b.WriteString(indent + "\t<key>")
			xml.EscapeText(b, []byte(k))
			b.WriteString("</key>\n")
			if err := value(b, v[k], depth+1); err != nil {
				return fmt.Errorf("%s: %w", k, err)
			}
		}
		b.WriteString(indent + "</dict>\n")
	default:
		return fmt.Errorf("plist: a %T has no plist form", v)
	}
	return nil
}

func array(b *bytes.Buffer, n int, at func(int) any, depth int) error {
	indent := strings.Repeat("\t", depth)
	if n == 0 {
		b.WriteString(indent + "<array/>\n")
		return nil
	}
	b.WriteString(indent + "<array>\n")
	for i := 0; i < n; i++ {
		if err := value(b, at(i), depth+1); err != nil {
			return err
		}
	}
	b.WriteString(indent + "</array>\n")
	return nil
}
