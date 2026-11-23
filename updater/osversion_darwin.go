//go:build darwin

package updater

import (
	"encoding/xml"
	"io"
	"os"
	"strings"
)

const systemVersionPlistPath = "/System/Library/CoreServices/SystemVersion.plist"

func currentOSVersion() string {
	f, err := os.Open(systemVersionPlistPath)
	if err != nil {
		return ""
	}
	defer f.Close()
	return parseSystemVersionPlist(f)
}

// parseSystemVersionPlist extracts ProductVersion from macOS SystemVersion.plist XML.
func parseSystemVersionPlist(r io.Reader) string {
	dec := xml.NewDecoder(r)
	wantString := false
	for {
		tok, err := dec.Token()
		if err != nil {
			return ""
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch se.Name.Local {
		case "key":
			var key string
			if err := dec.DecodeElement(&key, &se); err != nil {
				return ""
			}
			wantString = strings.TrimSpace(key) == "ProductVersion"
		case "string":
			if wantString {
				var val string
				if err := dec.DecodeElement(&val, &se); err != nil {
					return ""
				}
				return strings.TrimSpace(val)
			}
		default:
			wantString = false
		}
	}
}
