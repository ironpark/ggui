package updater

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"
)

// SparkleNamespace is the official Sparkle XML namespace URI.
const SparkleNamespace = "http://www.andymatuschak.org/xml-namespaces/sparkle"

const (
	dcNamespace     = "http://purl.org/dc/elements/1.1/"
	maxAppcastBytes = 4 << 20 // 4 MiB
)

// Appcast represents a parsed Sparkle 2 RSS 2.0 update feed.
type Appcast struct {
	Title       string
	Link        string
	Description string
	Language    string
	Items       []Item
}

// Item represents a single release entry (<item>) in a Sparkle 2 appcast feed.
type Item struct {
	Title                 string
	Version               string        // sparkle:version
	ShortVersion          string        // sparkle:shortVersionString
	PubDate               time.Time     // RFC1123Z / RFC1123 / RFC822Z / RFC822 / RFC3339
	MinimumSystemVersion  string        // sparkle:minimumSystemVersion
	Channel               string        // sparkle:channel ("" = default/stable)
	PhasedRolloutInterval time.Duration // sparkle:phasedRolloutInterval (seconds)
	CriticalUpdate        bool          // <sparkle:criticalUpdate/>
	CriticalBelowVersion  string        // optional sparkle:version attr on <sparkle:criticalUpdate>
	ReleaseNotes          string        // <description>
	ReleaseNotesLink      string        // <sparkle:releaseNotesLink>
	Enclosures            []Enclosure
}

// Enclosure represents a downloadable update binary (<enclosure>) within an Item.
type Enclosure struct {
	URL                string
	Length             int64
	Type               string
	EdSignature        string // sparkle:edSignature
	SHA256             string // sparkle:sha256 (optional)
	OS                 string // normalized: "macos", "windows", "linux"
	Arch               string // normalized: "universal", "amd64", "arm64", "386", "riscv64", or ""
	InstallerArguments string // sparkle:installerArguments
}

// ParseAppcast parses a Sparkle 2 (and Sparkle 1-compatible) RSS 2.0 XML feed.
// Input is capped at 4 MiB to protect against excessive memory consumption.
func ParseAppcast(r io.Reader) (*Appcast, error) {
	if r == nil {
		return nil, errors.New("updater: nil appcast reader")
	}
	data, err := io.ReadAll(io.LimitReader(r, maxAppcastBytes+1))
	if err != nil {
		return nil, fmt.Errorf("updater: reading appcast: %w", err)
	}
	if len(data) > maxAppcastBytes {
		return nil, fmt.Errorf("updater: appcast exceeds maximum size of %d bytes", maxAppcastBytes)
	}

	dec := xml.NewDecoder(bytes.NewReader(data))
	var feed *Appcast
	sawRSS := false

	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("updater: parsing appcast XML: %w", err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if !strings.EqualFold(se.Name.Local, "rss") {
			return nil, fmt.Errorf("updater: unexpected root element <%s>, want <rss>", se.Name.Local)
		}
		sawRSS = true
		parsed, err := decodeRSS(dec)
		if err != nil {
			return nil, err
		}
		feed = parsed
		break
	}

	if !sawRSS || feed == nil {
		return nil, errors.New("updater: appcast missing <rss><channel> structure")
	}
	return feed, nil
}

func decodeRSS(dec *xml.Decoder) (*Appcast, error) {
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("updater: parsing <rss>: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if strings.EqualFold(t.Name.Local, "channel") && t.Name.Space == "" {
				return decodeChannel(dec)
			}
			if err := dec.Skip(); err != nil {
				return nil, fmt.Errorf("updater: skipping <%s>: %w", t.Name.Local, err)
			}
		case xml.EndElement:
			return nil, errors.New("updater: appcast <rss> missing <channel>")
		}
	}
}

func decodeChannel(dec *xml.Decoder) (*Appcast, error) {
	feed := &Appcast{}
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("updater: parsing <channel>: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Space == "" {
				switch t.Name.Local {
				case "title":
					var s string
					if err := dec.DecodeElement(&s, &t); err != nil {
						return nil, fmt.Errorf("updater: parsing channel <title>: %w", err)
					}
					feed.Title = strings.TrimSpace(s)
					continue
				case "link":
					var s string
					if err := dec.DecodeElement(&s, &t); err != nil {
						return nil, fmt.Errorf("updater: parsing channel <link>: %w", err)
					}
					feed.Link = strings.TrimSpace(s)
					continue
				case "description":
					var s string
					if err := dec.DecodeElement(&s, &t); err != nil {
						return nil, fmt.Errorf("updater: parsing channel <description>: %w", err)
					}
					feed.Description = strings.TrimSpace(s)
					continue
				case "language":
					var s string
					if err := dec.DecodeElement(&s, &t); err != nil {
						return nil, fmt.Errorf("updater: parsing channel <language>: %w", err)
					}
					feed.Language = strings.TrimSpace(s)
					continue
				case "item":
					item, err := decodeItem(dec)
					if err != nil {
						return nil, err
					}
					feed.Items = append(feed.Items, item)
					continue
				}
			}
			if err := dec.Skip(); err != nil {
				return nil, fmt.Errorf("updater: skipping channel element <%s>: %w", t.Name.Local, err)
			}
		case xml.EndElement:
			return feed, nil
		}
	}
}

func decodeItem(dec *xml.Decoder) (Item, error) {
	var item Item
	var fallbackVersion string
	var fallbackShortVersion string

	for {
		tok, err := dec.Token()
		if err != nil {
			return Item{}, fmt.Errorf("updater: parsing <item>: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch {
			case t.Name.Space == "" && t.Name.Local == "title":
				var s string
				if err := dec.DecodeElement(&s, &t); err != nil {
					return Item{}, fmt.Errorf("updater: parsing item <title>: %w", err)
				}
				item.Title = strings.TrimSpace(s)

			case t.Name.Space == "" && t.Name.Local == "description":
				var s string
				if err := dec.DecodeElement(&s, &t); err != nil {
					return Item{}, fmt.Errorf("updater: parsing item <description>: %w", err)
				}
				item.ReleaseNotes = s

			case t.Name.Space == "" && t.Name.Local == "pubDate":
				var s string
				if err := dec.DecodeElement(&s, &t); err != nil {
					return Item{}, fmt.Errorf("updater: parsing item <pubDate>: %w", err)
				}
				if ts, ok := parseAppcastDate(s); ok {
					item.PubDate = ts
				}

			case t.Name.Space == "" && t.Name.Local == "enclosure":
				enc, encVer, encShortVer := parseEnclosureAttrs(t.Attr)
				if encVer != "" && fallbackVersion == "" {
					fallbackVersion = encVer
				}
				if encShortVer != "" && fallbackShortVersion == "" {
					fallbackShortVersion = encShortVer
				}
				item.Enclosures = append(item.Enclosures, enc)
				if err := dec.Skip(); err != nil {
					return Item{}, fmt.Errorf("updater: skipping <enclosure> body: %w", err)
				}

			case isSparkleName(t.Name, "version"):
				var s string
				if err := dec.DecodeElement(&s, &t); err != nil {
					return Item{}, fmt.Errorf("updater: parsing <sparkle:version>: %w", err)
				}
				item.Version = strings.TrimSpace(s)

			case isSparkleName(t.Name, "shortVersionString"):
				var s string
				if err := dec.DecodeElement(&s, &t); err != nil {
					return Item{}, fmt.Errorf("updater: parsing <sparkle:shortVersionString>: %w", err)
				}
				item.ShortVersion = strings.TrimSpace(s)

			case isSparkleName(t.Name, "minimumSystemVersion"):
				var s string
				if err := dec.DecodeElement(&s, &t); err != nil {
					return Item{}, fmt.Errorf("updater: parsing <sparkle:minimumSystemVersion>: %w", err)
				}
				item.MinimumSystemVersion = strings.TrimSpace(s)

			case isSparkleName(t.Name, "channel"):
				var s string
				if err := dec.DecodeElement(&s, &t); err != nil {
					return Item{}, fmt.Errorf("updater: parsing <sparkle:channel>: %w", err)
				}
				item.Channel = strings.TrimSpace(s)

			case isSparkleName(t.Name, "phasedRolloutInterval"):
				var s string
				if err := dec.DecodeElement(&s, &t); err != nil {
					return Item{}, fmt.Errorf("updater: parsing <sparkle:phasedRolloutInterval>: %w", err)
				}
				secs, parseErr := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
				if (parseErr == nil || errors.Is(parseErr, strconv.ErrRange)) && secs > 0 {
					const maxRolloutSeconds = math.MaxInt64 / int64(time.Second)
					if secs > maxRolloutSeconds {
						secs = maxRolloutSeconds
					}
					item.PhasedRolloutInterval = time.Duration(secs) * time.Second
				}

			case isSparkleName(t.Name, "criticalUpdate"):
				item.CriticalUpdate = true
				for _, attr := range t.Attr {
					if isSparkleName(attr.Name, "version") || (attr.Name.Space == "" && strings.EqualFold(attr.Name.Local, "version")) {
						item.CriticalBelowVersion = strings.TrimSpace(attr.Value)
					}
				}
				if err := dec.Skip(); err != nil {
					return Item{}, fmt.Errorf("updater: skipping <sparkle:criticalUpdate>: %w", err)
				}

			case isSparkleName(t.Name, "releaseNotesLink"):
				var s string
				if err := dec.DecodeElement(&s, &t); err != nil {
					return Item{}, fmt.Errorf("updater: parsing <sparkle:releaseNotesLink>: %w", err)
				}
				item.ReleaseNotesLink = strings.TrimSpace(s)

			default:
				if err := dec.Skip(); err != nil {
					return Item{}, fmt.Errorf("updater: skipping item element <%s>: %w", t.Name.Local, err)
				}
			}

		case xml.EndElement:
			if item.Version == "" {
				item.Version = fallbackVersion
			}
			if item.ShortVersion == "" {
				item.ShortVersion = fallbackShortVersion
			}
			return item, nil
		}
	}
}

func parseEnclosureAttrs(attrs []xml.Attr) (enc Enclosure, fallbackVersion, fallbackShortVersion string) {
	var rawOS, rawArch string
	for _, attr := range attrs {
		val := strings.TrimSpace(attr.Value)
		switch {
		case attr.Name.Space == "" && attr.Name.Local == "url":
			enc.URL = val
		case attr.Name.Space == "" && attr.Name.Local == "length":
			if n, err := strconv.ParseInt(val, 10, 64); err == nil && n >= 0 {
				enc.Length = n
			}
		case attr.Name.Space == "" && attr.Name.Local == "type":
			enc.Type = val
		case isSparkleName(attr.Name, "edSignature"):
			enc.EdSignature = val
		case isSparkleName(attr.Name, "sha256"):
			enc.SHA256 = val
		case isSparkleName(attr.Name, "os"):
			rawOS = val
		case isSparkleName(attr.Name, "arch"):
			rawArch = val
		case isSparkleName(attr.Name, "installerArguments"):
			enc.InstallerArguments = attr.Value
		case isSparkleName(attr.Name, "version"):
			fallbackVersion = val
		case isSparkleName(attr.Name, "shortVersionString"):
			fallbackShortVersion = val
		}
	}
	enc.OS, enc.Arch = normalizeOSArch(rawOS, rawArch)
	return enc, fallbackVersion, fallbackShortVersion
}

func isSparkleName(name xml.Name, local string) bool {
	return strings.EqualFold(name.Local, local) &&
		(name.Space == SparkleNamespace || name.Space == "sparkle")
}

// normalizeOSArch normalizes Sparkle OS and architecture identifiers.
// An omitted OS defaults to "macos", matching Sparkle 2 convention.
// Compound Windows/Linux/macOS tags such as "windows-x64" or "windows-arm64"
// normalize OS to "windows" and populate arch if rawArch was empty.
func normalizeOSArch(rawOS, rawArch string) (osName, arch string) {
	osRaw := strings.ToLower(strings.TrimSpace(rawOS))
	arch = normalizeArch(rawArch)

	var archHint string
	switch osRaw {
	case "", "macos", "osx", "darwin", "mac", "macosx":
		osName = "macos"
	case "windows", "win":
		osName = "windows"
	case "windows-x64", "windows-amd64", "win64":
		osName = "windows"
		archHint = "amd64"
	case "windows-x86", "windows-i386", "windows-386", "win32":
		osName = "windows"
		archHint = "386"
	case "windows-arm64", "windows-aarch64":
		osName = "windows"
		archHint = "arm64"
	case "linux":
		osName = "linux"
	default:
		if base, suffix, ok := strings.Cut(osRaw, "-"); ok {
			baseNorm, _ := normalizeOSArch(base, "")
			suffixArch := normalizeArch(suffix)
			if isKnownOS(baseNorm) && isKnownArch(suffixArch) {
				osName = baseNorm
				archHint = suffixArch
			} else {
				osName = osRaw
			}
		} else {
			osName = osRaw
		}
	}

	if arch == "" && archHint != "" {
		arch = archHint
	}
	return osName, arch
}

func normalizeArch(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		return ""
	case "x86_64", "x64", "amd64", "x86-64":
		return "amd64"
	case "aarch64", "arm64":
		return "arm64"
	case "x86", "i386", "i686", "386", "ia32":
		return "386"
	case "riscv64":
		return "riscv64"
	case "universal", "universal2", "fat":
		return "universal"
	default:
		return strings.ToLower(strings.TrimSpace(raw))
	}
}

func isKnownOS(osName string) bool {
	return osName == "macos" || osName == "windows" || osName == "linux"
}

func isKnownArch(arch string) bool {
	switch arch {
	case "amd64", "arm64", "386", "riscv64", "universal":
		return true
	default:
		return false
	}
}

var appcastDateLayouts = []string{
	time.RFC1123Z,
	time.RFC1123,
	"Mon, 2 Jan 2006 15:04:05 -0700",
	"Mon, 2 Jan 2006 15:04:05 MST",
	time.RFC822Z,
	time.RFC822,
	"2 Jan 2006 15:04:05 -0700",
	"2 Jan 2006 15:04:05 MST",
	"Mon, 02 Jan 2006 15:04 -0700",
	"Mon, 2 Jan 2006 15:04 -0700",
	time.RFC3339,
	time.RFC3339Nano,
}

func parseAppcastDate(raw string) (time.Time, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range appcastDateLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// MarshalAppcast serializes an Appcast to valid Sparkle 2 RSS 2.0 XML.
func MarshalAppcast(feed *Appcast) ([]byte, error) {
	if feed == nil {
		return nil, errors.New("updater: nil appcast")
	}

	doc := xmlAppcast{
		Version:      "2.0",
		XmlnsSparkle: SparkleNamespace,
		XmlnsDC:      dcNamespace,
		Channel: xmlChannel{
			Title:       feed.Title,
			Link:        feed.Link,
			Description: feed.Description,
			Language:    feed.Language,
			Items:       make([]xmlItem, 0, len(feed.Items)),
		},
	}

	for _, item := range feed.Items {
		xi := xmlItem{
			Title:                item.Title,
			Version:              item.Version,
			ShortVersion:         item.ShortVersion,
			MinimumSystemVersion: item.MinimumSystemVersion,
			Channel:              normalizeChannel(item.Channel),
			ReleaseNotesLink:     item.ReleaseNotesLink,
		}
		if !item.PubDate.IsZero() {
			xi.PubDate = item.PubDate.Format(time.RFC1123Z)
		}
		if item.PhasedRolloutInterval > 0 {
			secs := int64(item.PhasedRolloutInterval / time.Second)
			if secs > 0 {
				xi.PhasedRolloutInterval = strconv.FormatInt(secs, 10)
			}
		}
		if item.CriticalUpdate {
			xi.CriticalUpdate = &xmlCriticalUpdate{
				Version: item.CriticalBelowVersion,
			}
		}
		if item.ReleaseNotes != "" {
			if strings.ContainsAny(item.ReleaseNotes, "<>&") {
				xi.Description = xmlCDATA{Value: item.ReleaseNotes}
			} else {
				xi.Description = item.ReleaseNotes
			}
		}
		if len(item.Enclosures) > 0 {
			xi.Enclosures = make([]xmlEnclosure, 0, len(item.Enclosures))
			for _, enc := range item.Enclosures {
				encType := strings.TrimSpace(enc.Type)
				if encType == "" {
					encType = "application/octet-stream"
				}
				length := enc.Length
				if length < 0 {
					length = 0
				}
				xe := xmlEnclosure{
					URL:                enc.URL,
					Length:             length,
					Type:               encType,
					EdSignature:        enc.EdSignature,
					SHA256:             enc.SHA256,
					OS:                 enc.OS,
					Arch:               enc.Arch,
					InstallerArguments: enc.InstallerArguments,
				}
				xi.Enclosures = append(xi.Enclosures, xe)
			}
		}
		doc.Channel.Items = append(doc.Channel.Items, xi)
	}

	body, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("updater: marshaling appcast XML: %w", err)
	}
	var out bytes.Buffer
	out.WriteString(xml.Header)
	out.Write(body)
	out.WriteByte('\n')
	return out.Bytes(), nil
}

type xmlAppcast struct {
	XMLName      xml.Name   `xml:"rss"`
	Version      string     `xml:"version,attr"`
	XmlnsSparkle string     `xml:"xmlns:sparkle,attr"`
	XmlnsDC      string     `xml:"xmlns:dc,attr"`
	Channel      xmlChannel `xml:"channel"`
}

type xmlChannel struct {
	Title       string    `xml:"title,omitempty"`
	Link        string    `xml:"link,omitempty"`
	Description string    `xml:"description,omitempty"`
	Language    string    `xml:"language,omitempty"`
	Items       []xmlItem `xml:"item"`
}

type xmlItem struct {
	Title                 string             `xml:"title,omitempty"`
	Version               string             `xml:"sparkle:version,omitempty"`
	ShortVersion          string             `xml:"sparkle:shortVersionString,omitempty"`
	PubDate               string             `xml:"pubDate,omitempty"`
	MinimumSystemVersion  string             `xml:"sparkle:minimumSystemVersion,omitempty"`
	Channel               string             `xml:"sparkle:channel,omitempty"`
	PhasedRolloutInterval string             `xml:"sparkle:phasedRolloutInterval,omitempty"`
	CriticalUpdate        *xmlCriticalUpdate `xml:"sparkle:criticalUpdate,omitempty"`
	ReleaseNotesLink      string             `xml:"sparkle:releaseNotesLink,omitempty"`
	Description           any                `xml:"description,omitempty"`
	Enclosures            []xmlEnclosure     `xml:"enclosure"`
}

type xmlCriticalUpdate struct {
	Version string `xml:"sparkle:version,attr,omitempty"`
}

type xmlCDATA struct {
	Value string `xml:",cdata"`
}

type xmlEnclosure struct {
	URL                string `xml:"url,attr"`
	Length             int64  `xml:"length,attr"`
	Type               string `xml:"type,attr,omitempty"`
	EdSignature        string `xml:"sparkle:edSignature,attr,omitempty"`
	SHA256             string `xml:"sparkle:sha256,attr,omitempty"`
	OS                 string `xml:"sparkle:os,attr,omitempty"`
	Arch               string `xml:"sparkle:arch,attr,omitempty"`
	InstallerArguments string `xml:"sparkle:installerArguments,attr,omitempty"`
}
