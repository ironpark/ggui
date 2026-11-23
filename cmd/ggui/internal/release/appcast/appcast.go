// Package appcast generates, merges, parses, and signs Sparkle 2-compatible
// appcast.xml feeds using Ed25519 signatures over raw release artifact bytes.
package appcast

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ironpark/ggui/cmd/ggui/internal/release"
)

const (
	// SparkleNS is the Sparkle 2 XML namespace URI.
	SparkleNS = "http://www.andymatuschak.org/xml-namespaces/sparkle"
	// DublinCoreNS is the Dublin Core XML namespace URI included in RSS feeds.
	DublinCoreNS = "http://purl.org/dc/elements/1.1/"
	// DefaultFileName is the conventional name for an appcast feed file.
	DefaultFileName = "appcast.xml"
	// DefaultEnclosureType is the default MIME type for Sparkle 2 enclosures.
	DefaultEnclosureType = "application/octet-stream"
)

// ErrInvalidSignature is returned by VerifyFile when an Ed25519 signature does
// not match the file's contents and public key.
var ErrInvalidSignature = errors.New("invalid Ed25519 signature")

// GenerateKeyPair generates a new Ed25519 key pair in Sparkle 2 base64 format.
// pubBase64 is the 32-byte public key (SUPublicEDKey) and privSeedBase64 is the
// 32-byte private seed (compatible with Sparkle 2's generate_keys).
func GenerateKeyPair() (pubBase64, privSeedBase64 string, err error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("generating Ed25519 key pair: %w", err)
	}
	return base64.StdEncoding.EncodeToString(pub), base64.StdEncoding.EncodeToString(priv.Seed()), nil
}

func decodeBase64Key(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, errors.New("empty base64 value")
	}
	raw, err := base64.StdEncoding.DecodeString(s)
	if err != nil && !strings.Contains(s, "=") {
		raw, err = base64.RawStdEncoding.DecodeString(s)
	}
	if err != nil {
		return nil, errors.New("invalid base64 encoding")
	}
	return raw, nil
}

// ParsePublicKey decodes a base64-encoded 32-byte Ed25519 public key.
func ParsePublicKey(b64 string) (ed25519.PublicKey, error) {
	raw, err := decodeBase64Key(b64)
	if err != nil {
		return nil, fmt.Errorf("parsing Ed25519 public key: %w", err)
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("parsing Ed25519 public key: got %d bytes, want %d", len(raw), ed25519.PublicKeySize)
	}
	return ed25519.PublicKey(raw), nil
}

// ParsePrivateKey decodes a base64-encoded Ed25519 private key. It accepts both
// a 32-byte seed (Sparkle 2's format) and a 64-byte full Ed25519 private key.
// Returned errors never include the key material.
func ParsePrivateKey(b64 string) (ed25519.PrivateKey, error) {
	raw, err := decodeBase64Key(b64)
	if err != nil {
		return nil, fmt.Errorf("parsing Ed25519 private key: %w", err)
	}
	switch len(raw) {
	case ed25519.SeedSize:
		return ed25519.NewKeyFromSeed(raw), nil
	case ed25519.PrivateKeySize:
		derived := ed25519.NewKeyFromSeed(raw[:ed25519.SeedSize])
		if !bytes.Equal(derived, raw) {
			return nil, errors.New("parsing Ed25519 private key: embedded public key does not match seed")
		}
		return ed25519.PrivateKey(raw), nil
	default:
		return nil, fmt.Errorf("parsing Ed25519 private key: got %d bytes, want %d (seed) or %d (full key)",
			len(raw), ed25519.SeedSize, ed25519.PrivateKeySize)
	}
}

// PublicKeyFromPrivate derives the base64-encoded 32-byte Ed25519 public key
// corresponding to privBase64.
func PublicKeyFromPrivate(privBase64 string) (string, error) {
	priv, err := ParsePrivateKey(privBase64)
	if err != nil {
		return "", err
	}
	pub := priv.Public().(ed25519.PublicKey)
	return base64.StdEncoding.EncodeToString(pub), nil
}

// SignFile signs the exact raw bytes of path with privBase64 (Sparkle 2
// Ed25519 format) and returns the base64 signature, file size in bytes, and
// lowercase hex SHA-256 digest.
func SignFile(privBase64, path string) (sigBase64 string, size int64, sha256Hex string, err error) {
	priv, err := ParsePrivateKey(privBase64)
	if err != nil {
		return "", 0, "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", 0, "", err
	}
	sum := sha256.Sum256(data)
	sig := ed25519.Sign(priv, data)
	return base64.StdEncoding.EncodeToString(sig), int64(len(data)), hex.EncodeToString(sum[:]), nil
}

// VerifyFile verifies that sigBase64 is a valid Sparkle 2 Ed25519 signature
// over the exact raw bytes of path under pubBase64.
func VerifyFile(pubBase64, path, sigBase64 string) error {
	pub, err := ParsePublicKey(pubBase64)
	if err != nil {
		return err
	}
	sig, err := decodeBase64Key(sigBase64)
	if err != nil {
		return fmt.Errorf("parsing Ed25519 signature: %w", err)
	}
	if len(sig) != ed25519.SignatureSize {
		return fmt.Errorf("parsing Ed25519 signature: got %d bytes, want %d", len(sig), ed25519.SignatureSize)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !ed25519.Verify(pub, data, sig) {
		return ErrInvalidSignature
	}
	return nil
}

// Feed is a Sparkle 2-compatible RSS 2.0 appcast document.
type Feed struct {
	Title       string
	Link        string
	Description string
	Language    string
	Items       []Item
}

// Item is a single release enclosure entry in an appcast feed.
type Item struct {
	Title                 string
	Version               string
	ShortVersion          string
	PubDate               time.Time
	MinimumSystemVersion  string
	Channel               string
	PhasedRolloutInterval int64
	CriticalUpdate        bool
	CriticalBelowVersion  string
	ReleaseNotes          string
	ReleaseNotesLink      string
	Enclosure             Enclosure
}

// Enclosure describes one downloadable update archive or installer.
type Enclosure struct {
	URL                string
	Length             int64
	Type               string
	EdSignature        string
	SHA256             string
	OS                 string
	Arch               string
	InstallerArguments string
}

// Marshal encodes f as a Sparkle 2-compatible RSS 2.0 XML document.
func Marshal(f *Feed) ([]byte, error) {
	if f == nil {
		return nil, errors.New("appcast: nil feed")
	}
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?>` + "\n")
	b.WriteString(`<rss version="2.0" xmlns:sparkle="` + SparkleNS + `" xmlns:dc="` + DublinCoreNS + `">` + "\n")
	b.WriteString("  <channel>\n")
	writeTextElement(&b, "    ", "title", f.Title)
	writeTextElement(&b, "    ", "link", f.Link)
	writeTextElement(&b, "    ", "description", f.Description)
	writeTextElement(&b, "    ", "language", f.Language)
	for _, it := range f.Items {
		b.WriteString("    <item>\n")
		writeTextElement(&b, "      ", "title", it.Title)
		writeTextElement(&b, "      ", "sparkle:version", it.Version)
		writeTextElement(&b, "      ", "sparkle:shortVersionString", it.ShortVersion)
		if !it.PubDate.IsZero() {
			writeTextElement(&b, "      ", "pubDate", it.PubDate.UTC().Format(time.RFC1123Z))
		}
		writeTextElement(&b, "      ", "sparkle:minimumSystemVersion", it.MinimumSystemVersion)
		if ch := normalizeChannel(it.Channel); ch != "" {
			writeTextElement(&b, "      ", "sparkle:channel", ch)
		}
		if it.PhasedRolloutInterval > 0 {
			writeTextElement(&b, "      ", "sparkle:phasedRolloutInterval", strconv.FormatInt(it.PhasedRolloutInterval, 10))
		}
		if it.CriticalUpdate {
			if v := strings.TrimSpace(it.CriticalBelowVersion); v != "" {
				b.WriteString("      <sparkle:criticalUpdate")
				writeAttr(&b, "sparkle:version", v, true)
				b.WriteString("/>\n")
			} else {
				b.WriteString("      <sparkle:criticalUpdate/>\n")
			}
		}
		writeTextElement(&b, "      ", "sparkle:releaseNotesLink", it.ReleaseNotesLink)
		writeTextElement(&b, "      ", "description", it.ReleaseNotes)
		writeEnclosure(&b, "      ", it.Enclosure)
		b.WriteString("    </item>\n")
	}
	b.WriteString("  </channel>\n")
	b.WriteString("</rss>\n")
	return b.Bytes(), nil
}

func writeTextElement(b *bytes.Buffer, indent, tag, value string) {
	if value == "" {
		return
	}
	b.WriteString(indent)
	b.WriteByte('<')
	b.WriteString(tag)
	b.WriteByte('>')
	writeEscapedText(b, value)
	b.WriteString("</")
	b.WriteString(tag)
	b.WriteString(">\n")
}

func writeEnclosure(b *bytes.Buffer, indent string, e Enclosure) {
	if e.URL == "" && e.Length == 0 && e.Type == "" && e.EdSignature == "" && e.SHA256 == "" && e.OS == "" && e.Arch == "" && e.InstallerArguments == "" {
		return
	}
	b.WriteString(indent)
	b.WriteString("<enclosure")
	writeAttr(b, "url", e.URL, true)
	writeAttr(b, "length", strconv.FormatInt(e.Length, 10), true)
	typ := e.Type
	if typ == "" {
		typ = DefaultEnclosureType
	}
	writeAttr(b, "type", typ, true)
	writeAttr(b, "sparkle:edSignature", e.EdSignature, false)
	writeAttr(b, "sparkle:sha256", e.SHA256, false)
	writeAttr(b, "sparkle:os", e.OS, false)
	writeAttr(b, "sparkle:arch", e.Arch, false)
	writeAttr(b, "sparkle:installerArguments", e.InstallerArguments, false)
	b.WriteString("/>\n")
}

func writeAttr(b *bytes.Buffer, name, val string, required bool) {
	if !required && val == "" {
		return
	}
	b.WriteByte(' ')
	b.WriteString(name)
	b.WriteString(`="`)
	writeEscapedAttr(b, val)
	b.WriteByte('"')
}

func writeEscapedText(b *bytes.Buffer, s string) {
	for i := 0; i < len(s); {
		r, w := utf8.DecodeRuneInString(s[i:])
		if !isValidXMLRune(r) {
			r = '\uFFFD'
		}
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '\r':
			b.WriteString("&#xD;")
		default:
			b.WriteRune(r)
		}
		i += w
	}
}

func writeEscapedAttr(b *bytes.Buffer, s string) {
	for i := 0; i < len(s); {
		r, w := utf8.DecodeRuneInString(s[i:])
		if !isValidXMLRune(r) {
			r = '\uFFFD'
		}
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		case '\t':
			b.WriteString("&#x9;")
		case '\n':
			b.WriteString("&#xA;")
		case '\r':
			b.WriteString("&#xD;")
		default:
			b.WriteRune(r)
		}
		i += w
	}
}

func isValidXMLRune(r rune) bool {
	return r == 0x09 || r == 0x0A || r == 0x0D ||
		(r >= 0x20 && r <= 0xD7FF) ||
		(r >= 0xE000 && r <= 0xFFFD) ||
		(r >= 0x10000 && r <= 0x10FFFF)
}

func isSparkleSpace(space string) bool {
	return space == SparkleNS || space == "sparkle"
}

func isSparkleName(name xml.Name, local string) bool {
	return isSparkleSpace(name.Space) && strings.EqualFold(name.Local, local)
}

// Parse parses a Sparkle 2 RSS 2.0 appcast XML document.
func Parse(data []byte) (*Feed, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	var rootFound bool
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parsing appcast XML: %w", err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if !strings.EqualFold(se.Name.Local, "rss") {
			return nil, fmt.Errorf("parsing appcast XML: root element is <%s>, want <rss>", se.Name.Local)
		}
		rootFound = true
		return parseRSS(dec)
	}
	if !rootFound {
		return nil, errors.New("parsing appcast XML: empty document")
	}
	return nil, errors.New("parsing appcast XML: missing <rss> element")
}

func parseRSS(dec *xml.Decoder) (*Feed, error) {
	var feed *Feed
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("parsing appcast XML: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Space == "" && strings.EqualFold(t.Name.Local, "channel") {
				f, err := parseChannel(dec)
				if err != nil {
					return nil, err
				}
				feed = f
			} else {
				if err := dec.Skip(); err != nil {
					return nil, fmt.Errorf("parsing appcast XML: %w", err)
				}
			}
		case xml.EndElement:
			if feed == nil {
				return nil, errors.New("parsing appcast XML: missing <channel> element")
			}
			return feed, nil
		}
	}
}

func parseChannel(dec *xml.Decoder) (*Feed, error) {
	feed := &Feed{}
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("parsing appcast XML: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch {
			case t.Name.Space == "" && strings.EqualFold(t.Name.Local, "title"):
				if feed.Title, err = readCharData(dec); err != nil {
					return nil, err
				}
				feed.Title = strings.TrimSpace(feed.Title)
			case t.Name.Space == "" && strings.EqualFold(t.Name.Local, "link"):
				if feed.Link, err = readCharData(dec); err != nil {
					return nil, err
				}
				feed.Link = strings.TrimSpace(feed.Link)
			case t.Name.Space == "" && strings.EqualFold(t.Name.Local, "description"):
				if feed.Description, err = readCharData(dec); err != nil {
					return nil, err
				}
				feed.Description = strings.TrimSpace(feed.Description)
			case t.Name.Space == "" && strings.EqualFold(t.Name.Local, "language"):
				if feed.Language, err = readCharData(dec); err != nil {
					return nil, err
				}
				feed.Language = strings.TrimSpace(feed.Language)
			case t.Name.Space == "" && strings.EqualFold(t.Name.Local, "item"):
				items, err := parseItem(dec)
				if err != nil {
					return nil, err
				}
				feed.Items = append(feed.Items, items...)
			default:
				if err := dec.Skip(); err != nil {
					return nil, fmt.Errorf("parsing appcast XML: %w", err)
				}
			}
		case xml.EndElement:
			return feed, nil
		}
	}
}

func parseItem(dec *xml.Decoder) ([]Item, error) {
	var it Item
	var enclosures []Enclosure
	var encVersion, encShortVersion string
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("parsing appcast XML: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch {
			case t.Name.Space == "" && strings.EqualFold(t.Name.Local, "title"):
				if it.Title, err = readCharData(dec); err != nil {
					return nil, err
				}
				it.Title = strings.TrimSpace(it.Title)
			case t.Name.Space == "" && strings.EqualFold(t.Name.Local, "description"):
				if it.ReleaseNotes, err = readCharData(dec); err != nil {
					return nil, err
				}
			case t.Name.Space == "" && strings.EqualFold(t.Name.Local, "pubDate"):
				s, err := readCharData(dec)
				if err != nil {
					return nil, err
				}
				s = strings.TrimSpace(s)
				if s != "" {
					tm, err := parsePubDate(s)
					if err != nil {
						return nil, err
					}
					it.PubDate = tm
				}
			case t.Name.Space == "" && strings.EqualFold(t.Name.Local, "enclosure"):
				enc, v, sv, err := parseEnclosure(t, dec)
				if err != nil {
					return nil, err
				}
				if v != "" && encVersion == "" {
					encVersion = v
				}
				if sv != "" && encShortVersion == "" {
					encShortVersion = sv
				}
				enclosures = append(enclosures, enc)
			case isSparkleName(t.Name, "version"):
				if it.Version, err = readCharData(dec); err != nil {
					return nil, err
				}
				it.Version = strings.TrimSpace(it.Version)
			case isSparkleName(t.Name, "shortVersionString"):
				if it.ShortVersion, err = readCharData(dec); err != nil {
					return nil, err
				}
				it.ShortVersion = strings.TrimSpace(it.ShortVersion)
			case isSparkleName(t.Name, "minimumSystemVersion"):
				if it.MinimumSystemVersion, err = readCharData(dec); err != nil {
					return nil, err
				}
				it.MinimumSystemVersion = strings.TrimSpace(it.MinimumSystemVersion)
			case isSparkleName(t.Name, "channel"):
				if it.Channel, err = readCharData(dec); err != nil {
					return nil, err
				}
				it.Channel = normalizeChannel(it.Channel)
			case isSparkleName(t.Name, "phasedRolloutInterval"):
				s, err := readCharData(dec)
				if err != nil {
					return nil, err
				}
				s = strings.TrimSpace(s)
				if s != "" {
					n, err := strconv.ParseInt(s, 10, 64)
					if err != nil {
						return nil, fmt.Errorf("parsing sparkle:phasedRolloutInterval %q: %w", s, err)
					}
					it.PhasedRolloutInterval = n
				}
			case isSparkleName(t.Name, "criticalUpdate"):
				it.CriticalUpdate = true
				for _, attr := range t.Attr {
					if isSparkleName(attr.Name, "version") ||
						(attr.Name.Space == "" && strings.EqualFold(attr.Name.Local, "version")) {
						it.CriticalBelowVersion = strings.TrimSpace(attr.Value)
					}
				}
				if err := dec.Skip(); err != nil {
					return nil, fmt.Errorf("parsing appcast XML: %w", err)
				}
			case isSparkleName(t.Name, "releaseNotesLink"):
				if it.ReleaseNotesLink, err = readCharData(dec); err != nil {
					return nil, err
				}
				it.ReleaseNotesLink = strings.TrimSpace(it.ReleaseNotesLink)
			default:
				if err := dec.Skip(); err != nil {
					return nil, fmt.Errorf("parsing appcast XML: %w", err)
				}
			}
		case xml.EndElement:
			if it.Version == "" && encVersion != "" {
				it.Version = encVersion
			}
			if it.ShortVersion == "" && encShortVersion != "" {
				it.ShortVersion = encShortVersion
			}
			if len(enclosures) == 0 {
				return []Item{it}, nil
			}
			out := make([]Item, len(enclosures))
			for i, enc := range enclosures {
				cp := it
				cp.Enclosure = enc
				out[i] = cp
			}
			return out, nil
		}
	}
}

func parseEnclosure(start xml.StartElement, dec *xml.Decoder) (enc Enclosure, version, shortVersion string, err error) {
	for _, a := range start.Attr {
		switch {
		case a.Name.Space == "" && strings.EqualFold(a.Name.Local, "url"):
			enc.URL = strings.TrimSpace(a.Value)
		case a.Name.Space == "" && strings.EqualFold(a.Name.Local, "length"):
			s := strings.TrimSpace(a.Value)
			if s != "" {
				n, err := strconv.ParseInt(s, 10, 64)
				if err != nil {
					return Enclosure{}, "", "", fmt.Errorf("parsing enclosure length %q: %w", s, err)
				}
				enc.Length = n
			}
		case a.Name.Space == "" && strings.EqualFold(a.Name.Local, "type"):
			enc.Type = strings.TrimSpace(a.Value)
		case isSparkleName(a.Name, "edSignature"):
			enc.EdSignature = strings.TrimSpace(a.Value)
		case isSparkleName(a.Name, "sha256"):
			enc.SHA256 = strings.TrimSpace(a.Value)
		case isSparkleName(a.Name, "os"):
			enc.OS = strings.TrimSpace(a.Value)
		case isSparkleName(a.Name, "arch"):
			enc.Arch = strings.TrimSpace(a.Value)
		case isSparkleName(a.Name, "installerArguments"):
			enc.InstallerArguments = a.Value
		case isSparkleName(a.Name, "version"):
			version = strings.TrimSpace(a.Value)
		case isSparkleName(a.Name, "shortVersionString"):
			shortVersion = strings.TrimSpace(a.Value)
		}
	}
	if err := dec.Skip(); err != nil {
		return Enclosure{}, "", "", fmt.Errorf("parsing appcast XML: %w", err)
	}
	return enc, version, shortVersion, nil
}

func readCharData(dec *xml.Decoder) (string, error) {
	var b strings.Builder
	depth := 1
	for depth > 0 {
		tok, err := dec.Token()
		if err != nil {
			return "", fmt.Errorf("parsing appcast XML: %w", err)
		}
		switch t := tok.(type) {
		case xml.CharData:
			b.Write(t)
		case xml.StartElement:
			depth++
		case xml.EndElement:
			depth--
		}
	}
	return b.String(), nil
}

var pubDateLayouts = []string{
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

func parsePubDate(s string) (time.Time, error) {
	for _, layout := range pubDateLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("parsing pubDate %q: unrecognized date format", s)
}

func normalizeChannel(ch string) string {
	ch = strings.TrimSpace(strings.ToLower(ch))
	if ch == "" || ch == "stable" || ch == "default" {
		return ""
	}
	return ch
}

type dedupKey struct {
	version string
	channel string
	os      string
	arch    string
	base    string
}

func itemKey(it Item) dedupKey {
	return dedupKey{
		version: it.Version,
		channel: normalizeChannel(it.Channel),
		os:      strings.ToLower(it.Enclosure.OS),
		arch:    strings.ToLower(it.Enclosure.Arch),
		base:    urlBase(it.Enclosure.URL),
	}
}

func urlBase(rawURL string) string {
	if u, err := url.Parse(rawURL); err == nil && u.Path != "" {
		return path.Base(u.Path)
	}
	return filepath.Base(rawURL)
}

// Merge merges incoming items into existing (which may be nil).
//
// Matching entries are deduplicated by (Version, Channel, Enclosure.OS,
// Enclosure.Arch, base filename of URL), with incoming replacing existing so
// re-releasing updates entries in place. Items are sorted newest first
// (PubDate desc, then Version desc) and when maxVersions > 0, older distinct
// versions beyond maxVersions are pruned per channel.
func Merge(existing *Feed, incoming []Item, maxVersions int) *Feed {
	out := &Feed{}
	byKey := make(map[dedupKey]Item)
	if existing != nil {
		out.Title = existing.Title
		out.Link = existing.Link
		out.Description = existing.Description
		out.Language = existing.Language
		for _, it := range existing.Items {
			it.Channel = normalizeChannel(it.Channel)
			byKey[itemKey(it)] = it
		}
	}
	for _, it := range incoming {
		it.Channel = normalizeChannel(it.Channel)
		byKey[itemKey(it)] = it
	}
	items := make([]Item, 0, len(byKey))
	for _, it := range byKey {
		items = append(items, it)
	}
	slices.SortFunc(items, compareItemsDesc)
	if maxVersions > 0 {
		seenByChannel := make(map[string]map[string]bool)
		kept := make([]Item, 0, len(items))
		for _, it := range items {
			ch := normalizeChannel(it.Channel)
			seen := seenByChannel[ch]
			if seen == nil {
				seen = make(map[string]bool)
				seenByChannel[ch] = seen
			}
			if !seen[it.Version] {
				if len(seen) >= maxVersions {
					continue
				}
				seen[it.Version] = true
			}
			kept = append(kept, it)
		}
		items = kept
	}
	out.Items = items
	return out
}

func compareItemsDesc(a, b Item) int {
	// PubDate descending; non-zero dates sort ahead of zero dates.
	aZero, bZero := a.PubDate.IsZero(), b.PubDate.IsZero()
	if aZero != bZero {
		if aZero {
			return 1
		}
		return -1
	}
	if !aZero && !a.PubDate.Equal(b.PubDate) {
		if a.PubDate.After(b.PubDate) {
			return -1
		}
		return 1
	}
	// Version descending.
	if c := compareVersions(a.Version, b.Version); c != 0 {
		return -c
	}
	// Deterministic tie-breakers within the same release.
	if c := strings.Compare(a.Channel, b.Channel); c != 0 {
		return c
	}
	if c := strings.Compare(a.Enclosure.OS, b.Enclosure.OS); c != 0 {
		return c
	}
	if c := strings.Compare(a.Enclosure.Arch, b.Enclosure.Arch); c != 0 {
		return c
	}
	return strings.Compare(urlBase(a.Enclosure.URL), urlBase(b.Enclosure.URL))
}

// compareVersions compares version strings a and b using Sparkle 2 / SemVer-compatible
// ordering (matching updater.CompareVersion), returning -1, 0, or 1.
func compareVersions(a, b string) int {
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
	for i := range maxCore {
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
	for i := range minPre {
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
	for i := range len(s) {
		if !isASCIIDigit(s[i]) {
			return false
		}
	}
	return true
}

func isPreSeparator(c byte) bool {
	return c == '.' || c == '-' || c == '_' || c == ' ' || c == '\t'
}

// BuildOptions controls how BuildFromAssets builds and signs appcast items.
type BuildOptions struct {
	Version               string
	ShortVersion          string
	Title                 string
	PubDate               time.Time
	MinimumSystemVersion  string // applied to all items unless overridden by MinMacOSVersion
	MinMacOSVersion       string // applied only to macOS items
	Channel               string
	PhasedRolloutInterval int64
	CriticalUpdate        bool
	ReleaseNotes          string
	ReleaseNotesLink      string
	PrivateKey            string // base64 Ed25519 private key (seed or full)
	AllowUnsigned         bool
	DownloadURL           func(release.Asset) string
	BaseURL               string // used when DownloadURL is nil
	AppcastFile           string // skipped if encountered in assets; default "appcast.xml"
}

// BuildFromAssets selects eligible updatable assets, signs them with Ed25519
// (unless AllowUnsigned is set and PrivateKey is empty), and builds appcast
// items ready for Merge and Marshal.
//
// Eligible artifacts:
//   - macOS: .zip and .dmg (.pkg is skipped)
//   - Windows: -setup.exe (with installerArguments="--silent") and .zip
//   - Linux: .tar.gz (.deb and .rpm are skipped)
//
// SHA256SUMS and the appcast XML file itself are always skipped.
func BuildFromAssets(assets []release.Asset, opts BuildOptions) ([]Item, error) {
	if strings.TrimSpace(opts.Version) == "" {
		return nil, errors.New("appcast: version is required")
	}
	privKey := strings.TrimSpace(opts.PrivateKey)
	if privKey == "" && !opts.AllowUnsigned {
		return nil, errors.New("appcast: Ed25519 private key is required")
	}
	if privKey != "" {
		if _, err := ParsePrivateKey(privKey); err != nil {
			return nil, err
		}
	}
	if opts.DownloadURL == nil && strings.TrimSpace(opts.BaseURL) == "" {
		return nil, errors.New("appcast: download URL or base URL is required")
	}

	shortVer := opts.ShortVersion
	if shortVer == "" {
		shortVer = opts.Version
	}
	title := opts.Title
	if title == "" {
		title = "Version " + opts.Version
	}
	pubDate := opts.PubDate
	if pubDate.IsZero() {
		pubDate = time.Now().UTC()
	}
	skipFile := opts.AppcastFile
	if skipFile == "" {
		skipFile = DefaultFileName
	}
	baseURL := strings.TrimRight(opts.BaseURL, "/")
	channel := normalizeChannel(opts.Channel)

	var items []Item
	for _, a := range assets {
		if shouldSkipAsset(a.Name, skipFile) {
			continue
		}
		osName, arch := a.Target, a.Arch
		if osName == "" || arch == "" {
			inferredOS, inferredArch := InferPlatform(a.Name)
			if osName == "" {
				osName = inferredOS
			}
			if arch == "" {
				arch = inferredArch
			}
		}
		osName = normalizeOS(osName)
		arch = normalizeArch(arch)
		if arch == "universal" && osName != "macos" {
			arch = ""
		}

		eligible, installerArgs := assetEligibility(osName, a.Name)
		if !eligible {
			continue
		}

		var (
			sig  string
			size int64
			sum  string
			err  error
		)
		if privKey != "" {
			sig, size, sum, err = SignFile(privKey, a.Path)
			if err != nil {
				return nil, fmt.Errorf("signing %s: %w", a.Name, err)
			}
		} else {
			cp := a
			if cp.SHA256 == "" || cp.Size == 0 {
				if err := cp.Hash(); err != nil {
					return nil, fmt.Errorf("hashing %s: %w", a.Name, err)
				}
			}
			size, sum = cp.Size, cp.SHA256
		}

		dlURL := ""
		if opts.DownloadURL != nil {
			dlURL = opts.DownloadURL(a)
		} else {
			dlURL = baseURL + "/" + url.PathEscape(a.Name)
		}

		minOS := opts.MinimumSystemVersion
		if osName == "macos" && opts.MinMacOSVersion != "" {
			minOS = opts.MinMacOSVersion
		}

		items = append(items, Item{
			Title:                 title,
			Version:               opts.Version,
			ShortVersion:          shortVer,
			PubDate:               pubDate.UTC(),
			MinimumSystemVersion:  minOS,
			Channel:               channel,
			PhasedRolloutInterval: opts.PhasedRolloutInterval,
			CriticalUpdate:        opts.CriticalUpdate,
			ReleaseNotes:          opts.ReleaseNotes,
			ReleaseNotesLink:      opts.ReleaseNotesLink,
			Enclosure: Enclosure{
				URL:                dlURL,
				Length:             size,
				Type:               DefaultEnclosureType,
				EdSignature:        sig,
				SHA256:             sum,
				OS:                 osName,
				Arch:               arch,
				InstallerArguments: installerArgs,
			},
		})
	}
	return items, nil
}

func shouldSkipAsset(name, appcastFile string) bool {
	base := filepath.Base(name)
	lower := strings.ToLower(base)
	return base == release.SumsName ||
		strings.EqualFold(base, appcastFile) ||
		strings.HasSuffix(lower, ".xml")
}

func assetEligibility(osName, name string) (ok bool, installerArgs string) {
	lower := strings.ToLower(filepath.Base(name))
	switch osName {
	case "macos":
		return strings.HasSuffix(lower, ".zip") || strings.HasSuffix(lower, ".dmg"), ""
	case "windows":
		if strings.HasSuffix(lower, "-setup.exe") || strings.HasSuffix(lower, "_setup.exe") || lower == "setup.exe" {
			return true, "--silent"
		}
		return strings.HasSuffix(lower, ".zip"), ""
	case "linux":
		return strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz"), ""
	default:
		return false, ""
	}
}

// InferPlatform infers the target OS ("macos", "windows", "linux") and
// architecture ("universal", "amd64", "arm64", "386", "riscv64") from a
// release filename.
func InferPlatform(name string) (osName, arch string) {
	lower := strings.ToLower(filepath.Base(name))
	stem := lower
	var authOS, fallbackOS string
	switch {
	case strings.HasSuffix(lower, "-setup.exe"):
		stem = strings.TrimSuffix(lower, "-setup.exe")
		authOS = "windows"
	case strings.HasSuffix(lower, "_setup.exe"):
		stem = strings.TrimSuffix(lower, "_setup.exe")
		authOS = "windows"
	case lower == "setup.exe":
		stem = ""
		authOS = "windows"
	case strings.HasSuffix(lower, ".exe"):
		stem = strings.TrimSuffix(lower, ".exe")
		authOS = "windows"
	case strings.HasSuffix(lower, ".dmg"):
		stem = strings.TrimSuffix(lower, ".dmg")
		authOS = "macos"
	case strings.HasSuffix(lower, ".pkg"):
		stem = strings.TrimSuffix(lower, ".pkg")
		authOS = "macos"
	case strings.HasSuffix(lower, ".deb"), strings.HasSuffix(lower, ".rpm"):
		stem = strings.TrimSuffix(lower, filepath.Ext(lower))
		authOS = "linux"
	case strings.HasSuffix(lower, ".tar.gz"):
		stem = strings.TrimSuffix(lower, ".tar.gz")
		fallbackOS = "linux"
	case strings.HasSuffix(lower, ".tgz"):
		stem = strings.TrimSuffix(lower, ".tgz")
		fallbackOS = "linux"
	case strings.HasSuffix(lower, ".zip"):
		stem = strings.TrimSuffix(lower, ".zip")
	}

	osName = authOS
	tokens := strings.FieldsFunc(stem, func(r rune) bool {
		return r == '-' || r == '_' || r == '.'
	})
	// Scan right to left so app names containing OS-like words do not shadow
	// the platform suffix (e.g. macos-helper-1.0.0-windows-amd64.zip).
	for i := len(tokens) - 1; i >= 0; i-- {
		tok := tokens[i]
		if arch == "" {
			if tok == "64" && i > 0 && tokens[i-1] == "x86" {
				arch = "amd64"
				i--
				continue
			}
			if a := normalizeArch(tok); a != "" {
				if a != "universal" || authOS == "" || authOS == "macos" {
					arch = a
					continue
				}
			}
		}
		if tok == "win64" || tok == "win32" {
			if osName == "" {
				osName = "windows"
			}
			if osName == "windows" && arch == "" {
				if tok == "win64" {
					arch = "amd64"
				} else {
					arch = "386"
				}
			}
		} else if osName == "" {
			if o := normalizeOS(tok); o != "" {
				osName = o
			}
		}
		if osName != "" && arch != "" {
			break
		}
	}
	if osName == "" {
		osName = fallbackOS
	}
	if arch == "universal" && osName != "macos" {
		arch = ""
	}
	return osName, arch
}

func normalizeOS(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "macos", "darwin", "osx":
		return "macos"
	case "windows", "win":
		return "windows"
	case "linux":
		return "linux"
	default:
		return ""
	}
}

func normalizeArch(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "universal", "universal2", "fat":
		return "universal"
	case "amd64", "x86_64", "x86-64", "x64":
		return "amd64"
	case "arm64", "aarch64":
		return "arm64"
	case "386", "i386", "i686", "x86", "ia32":
		return "386"
	case "riscv64":
		return "riscv64"
	default:
		return ""
	}
}

// ScanDir reads regular files from dir into release.Asset entries with OS and
// architecture inferred from their filenames.
func ScanDir(dir string) ([]release.Asset, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var assets []release.Asset
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			continue
		}
		osName, arch := InferPlatform(e.Name())
		assets = append(assets, release.Asset{
			Name:   e.Name(),
			Path:   filepath.Join(dir, e.Name()),
			Target: osName,
			Arch:   arch,
		})
	}
	return assets, nil
}

const (
	maxFetchBytes     = 16 << 20 // 16 MiB
	maxFetchRedirects = 10
)

// fetchTransport optionally overrides the HTTP transport used by fetchURL in tests.
var fetchTransport http.RoundTripper

// LoadSource loads an existing appcast feed from a local path or an http(s) URL
// (with a 15-second timeout). If the local file does not exist or the HTTP URL
// responds with 404 Not Found, it returns (nil, false, nil) so callers can warn
// and start a fresh feed.
func LoadSource(ctx context.Context, source string) (*Feed, bool, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return nil, false, nil
	}
	lower := strings.ToLower(source)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return fetchURL(ctx, source)
	}
	data, err := os.ReadFile(source)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	feed, err := Parse(data)
	if err != nil {
		return nil, false, fmt.Errorf("reading %s: %w", source, err)
	}
	return feed, true, nil
}

func fetchURL(ctx context.Context, rawURL string) (*Feed, bool, error) {
	initialURL, err := url.Parse(rawURL)
	if err != nil || initialURL.Host == "" {
		return nil, false, fmt.Errorf("fetching appcast %s: invalid URL", redactURL(rawURL))
	}
	requireHTTPS := strings.EqualFold(initialURL.Scheme, "https")
	client := &http.Client{
		Timeout:   15 * time.Second,
		Transport: fetchTransport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxFetchRedirects {
				return fmt.Errorf("stopped after %d redirects", maxFetchRedirects)
			}
			scheme := strings.ToLower(req.URL.Scheme)
			if scheme != "http" && scheme != "https" {
				return fmt.Errorf("disallowed redirect scheme %q", req.URL.Scheme)
			}
			if requireHTTPS && scheme != "https" {
				return fmt.Errorf("refusing redirect downgrade from https to %s (%s)", req.URL.Scheme, redactURL(req.URL.String()))
			}
			for _, prev := range via {
				if strings.EqualFold(prev.URL.Scheme, "https") && scheme != "https" {
					return fmt.Errorf("refusing redirect downgrade from https to %s (%s)", req.URL.Scheme, redactURL(req.URL.String()))
				}
			}
			return nil
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, false, fmt.Errorf("fetching appcast %s: %w", redactURL(rawURL), err)
	}
	req.Header.Set("User-Agent", "ggui-release")
	resp, err := client.Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("fetching appcast %s: %w", redactURL(rawURL), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, false, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, false, fmt.Errorf("fetching appcast %s: HTTP %d %s", redactURL(rawURL), resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchBytes+1))
	if err != nil {
		return nil, false, fmt.Errorf("reading appcast %s: %w", redactURL(rawURL), err)
	}
	if len(data) > maxFetchBytes {
		return nil, false, fmt.Errorf("reading appcast %s: feed exceeds maximum size of %d bytes", redactURL(rawURL), maxFetchBytes)
	}
	feed, err := Parse(data)
	if err != nil {
		return nil, false, fmt.Errorf("parsing appcast from %s: %w", redactURL(rawURL), err)
	}
	return feed, true, nil
}

func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "(invalid URL)"
	}
	u.User = nil
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}
