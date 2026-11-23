package updater

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCompareVersion(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		// Basic numeric and multi-digit ordering
		{"1.0.0", "1.0.0", 0},
		{"1.10.0", "1.2.0", 1},
		{"1.2.0", "1.10.0", -1},
		{"2.0.0", "1.99.99", 1},
		{"10.0.22631", "10.0.19041", 1},
		{"14.5", "14.6.1", -1},

		// Leading v/V, whitespace, trailing zeros, build metadata
		{"v1.2.0", "1.2.0", 0},
		{"V1.2.0", "1.2", 0},
		{"  v1.2  ", "1.2.0.0", 0},
		{"1.2.0.1", "1.2", 1},
		{"1.2.3+build.123", "1.2.3+build.456", 0},
		{"v1.2.3-rc.1+sha.abc", "1.2.3-rc.1+sha.def", 0},

		// SemVer prereleases
		{"1.2.0-rc.1", "1.2.0", -1},
		{"1.2.0", "1.2.0-rc.1", 1},
		{"1.2.0-alpha", "1.2.0-beta", -1},
		{"1.2.0-beta", "1.2.0-rc", -1},
		{"1.2.0-alpha", "1.2.0-alpha.1", -1},
		{"1.2.0-beta.2", "1.2.0-beta.10", -1},
		{"1.2.0-RC.1", "1.2.0-rc.1", 0},
		{"1.2.0-1", "1.2.0-alpha", -1},
		{"1.2.0-2", "1.2.0-10", -1},

		// Sparkle-style prereleases
		{"1.2a2", "1.2b1", -1},
		{"1.2b1", "1.2b2", -1},
		{"1.2b2", "1.2b10", -1},
		{"1.2b10", "1.2rc1", -1},
		{"1.2rc1", "1.2", -1},
		{"1.2rc1", "1.2.0-rc.1", 0},
		{"1.2.beta1", "1.2.beta2", -1},

		// Overflow-safe large numeric segments
		{"2026112412345678901234567890", "2026112412345678901234567889", 1},
		{"001.02.003", "1.2.3", 0},

		// Empty inputs
		{"", "", 0},
		{"", "0.0.0", -1},
		{"0.0.1", "", 1},
	}

	for _, tc := range cases {
		if got := CompareVersion(tc.a, tc.b); got != tc.want {
			t.Errorf("CompareVersion(%q, %q) = %d; want %d", tc.a, tc.b, got, tc.want)
		}
		// Check antisymmetry and reflexivity.
		if got := CompareVersion(tc.b, tc.a); got != -tc.want {
			t.Errorf("CompareVersion(%q, %q) = %d; want %d (antisymmetry)", tc.b, tc.a, got, -tc.want)
		}
		if got := CompareVersion(tc.a, tc.a); got != 0 {
			t.Errorf("CompareVersion(%q, %q) = %d; want 0 (reflexivity)", tc.a, tc.a, got)
		}
	}
}

func TestEd25519Signatures(t *testing.T) {
	pubB64, privSeedB64, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}

	pub, err := ParsePublicKey("  " + pubB64 + "\n")
	if err != nil {
		t.Fatalf("ParsePublicKey() error = %v", err)
	}
	if len(pub) != ed25519.PublicKeySize {
		t.Fatalf("public key len = %d; want %d", len(pub), ed25519.PublicKeySize)
	}

	privFromSeed, err := ParsePrivateKey(privSeedB64)
	if err != nil {
		t.Fatalf("ParsePrivateKey(32-byte seed) error = %v", err)
	}

	// Verify 64-byte full private key is also accepted and produces identical signature.
	privFullB64 := base64.StdEncoding.EncodeToString(privFromSeed)
	privFromFull, err := ParsePrivateKey(privFullB64)
	if err != nil {
		t.Fatalf("ParsePrivateKey(64-byte full) error = %v", err)
	}
	if !bytes.Equal(privFromSeed, privFromFull) {
		t.Fatalf("32-byte seed and 64-byte private key did not yield identical key")
	}

	payload := []byte("ggui sparkle 2 archive payload")
	sig1, err := SignBytes(privSeedB64, payload)
	if err != nil {
		t.Fatalf("SignBytes(seed) error = %v", err)
	}
	sig2, err := SignBytes(privFullB64, payload)
	if err != nil {
		t.Fatalf("SignBytes(full) error = %v", err)
	}
	if sig1 != sig2 {
		t.Fatalf("signatures from seed and full key differ: %q vs %q", sig1, sig2)
	}

	if err := VerifyBytes(pubB64, payload, sig1); err != nil {
		t.Fatalf("VerifyBytes(valid) error = %v", err)
	}

	// Tampered payload must fail with ErrInvalidSignature.
	tampered := append([]byte(nil), payload...)
	tampered[0] ^= 0xff
	if err := VerifyBytes(pubB64, tampered, sig1); !errors.Is(err, ErrInvalidSignature) {
		t.Errorf("VerifyBytes(tampered) = %v; want ErrInvalidSignature", err)
	}

	// Wrong public key must fail with ErrInvalidSignature.
	otherPub, _, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(other) error = %v", err)
	}
	if err := VerifyBytes(otherPub, payload, sig1); !errors.Is(err, ErrInvalidSignature) {
		t.Errorf("VerifyBytes(wrong key) = %v; want ErrInvalidSignature", err)
	}

	// Missing signature -> ErrMissingSignature.
	if err := VerifyBytes(pubB64, payload, "   "); !errors.Is(err, ErrMissingSignature) {
		t.Errorf("VerifyBytes(empty sig) = %v; want ErrMissingSignature", err)
	}

	// Malformed base64 & wrong length -> ErrInvalidSignature.
	if err := VerifyBytes(pubB64, payload, "!!!not-base64!!!"); !errors.Is(err, ErrInvalidSignature) {
		t.Errorf("VerifyBytes(bad b64) = %v; want ErrInvalidSignature", err)
	}
	shortSig := base64.StdEncoding.EncodeToString([]byte("too-short"))
	if err := VerifyBytes(pubB64, payload, shortSig); !errors.Is(err, ErrInvalidSignature) {
		t.Errorf("VerifyBytes(short sig) = %v; want ErrInvalidSignature", err)
	}

	// SignFile & VerifyFile round-trip.
	filePath := filepath.Join(t.TempDir(), "update.zip")
	if err := os.WriteFile(filePath, payload, 0o600); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}
	fileSig, fileSize, err := SignFile(privSeedB64, filePath)
	if err != nil {
		t.Fatalf("SignFile error = %v", err)
	}
	if fileSize != int64(len(payload)) {
		t.Errorf("SignFile size = %d; want %d", fileSize, len(payload))
	}
	if fileSig != sig1 {
		t.Errorf("SignFile sig = %q; want %q", fileSig, sig1)
	}
	if err := VerifyFile(pubB64, filePath, fileSig); err != nil {
		t.Fatalf("VerifyFile(valid) error = %v", err)
	}

	// Corrupted 64-byte private key should be rejected.
	badFull := make([]byte, ed25519.PrivateKeySize)
	_, _ = rand.Read(badFull)
	if _, err := ParsePrivateKey(base64.StdEncoding.EncodeToString(badFull)); err == nil {
		t.Errorf("ParsePrivateKey(inconsistent 64-byte) = nil; want error")
	}

	// Bad public key lengths / encodings.
	if _, err := ParsePublicKey(""); err == nil {
		t.Errorf("ParsePublicKey(empty) = nil; want error")
	}
	if _, err := ParsePublicKey(base64.StdEncoding.EncodeToString([]byte("short"))); err == nil {
		t.Errorf("ParsePublicKey(short) = nil; want error")
	}

	// Unpadded standard base64 (RawStdEncoding) accepted for keys and signatures.
	unpaddedPub := strings.TrimRight(pubB64, "=")
	unpaddedPrivSeed := strings.TrimRight(privSeedB64, "=")
	unpaddedPrivFull := strings.TrimRight(privFullB64, "=")
	unpaddedSig := strings.TrimRight(sig1, "=")

	if gotPub, err := ParsePublicKey(unpaddedPub); err != nil || !bytes.Equal(gotPub, pub) {
		t.Errorf("ParsePublicKey(unpadded) = (%v, %v); want match", len(gotPub), err)
	}
	if gotPriv, err := ParsePrivateKey(unpaddedPrivSeed); err != nil || !bytes.Equal(gotPriv, privFromSeed) {
		t.Errorf("ParsePrivateKey(unpadded seed) = (%v, %v); want match", len(gotPriv), err)
	}
	if gotPriv, err := ParsePrivateKey(unpaddedPrivFull); err != nil || !bytes.Equal(gotPriv, privFromFull) {
		t.Errorf("ParsePrivateKey(unpadded full) = (%v, %v); want match", len(gotPriv), err)
	}
	if err := VerifyBytes(unpaddedPub, payload, unpaddedSig); err != nil {
		t.Errorf("VerifyBytes(unpadded) error = %v", err)
	}
	if err := VerifyFile(unpaddedPub, filePath, unpaddedSig); err != nil {
		t.Errorf("VerifyFile(unpadded) error = %v", err)
	}
	// Malformed padding (contains '=' but invalid padding count) must still fail.
	if err := VerifyBytes(pubB64, payload, unpaddedSig+"="); !errors.Is(err, ErrInvalidSignature) {
		t.Errorf("VerifyBytes(bad padding) = %v; want ErrInvalidSignature", err)
	}
}

func TestAppcastMarshalParseRoundTrip(t *testing.T) {
	pubTime := time.Date(2026, time.November, 20, 14, 30, 0, 0, time.UTC)
	want := &Appcast{
		Title:       "Example App Updates",
		Link:        "https://example.com/appcast.xml",
		Description: "Latest releases for Example App",
		Language:    "en",
		Items: []Item{
			{
				Title:                 "Version 2.1.0",
				Version:               "2.1.0",
				ShortVersion:          "2.1",
				PubDate:               pubTime,
				MinimumSystemVersion:  "14.0",
				Channel:               "beta",
				PhasedRolloutInterval: 48 * time.Hour,
				CriticalUpdate:        true,
				CriticalBelowVersion:  "2.0.0",
				ReleaseNotes:          "<h2>What's New</h2><ul><li>Faster startup &amp; fixes ]]> edge</li></ul>",
				ReleaseNotesLink:      "https://example.com/notes/2.1.0.html",
				Enclosures: []Enclosure{
					{
						URL:         "https://example.com/Example-2.1.0-macos.zip",
						Length:      12345678,
						Type:        "application/octet-stream",
						EdSignature: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==",
						SHA256:      "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
						OS:          "macos",
						Arch:        "universal",
					},
					{
						URL:                "https://example.com/Example-2.1.0-setup.exe",
						Length:             23456789,
						Type:               "application/octet-stream",
						EdSignature:        "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB==",
						OS:                 "windows",
						Arch:               "amd64",
						InstallerArguments: "/S /NORESTART",
					},
				},
			},
			{
				Title:        "Version 2.0.0",
				Version:      "2.0.0",
				ShortVersion: "2.0.0",
				ReleaseNotes: "Plain text release notes",
				Enclosures: []Enclosure{
					{
						URL:  "https://example.com/Example-2.0.0-linux.AppImage",
						OS:   "linux",
						Arch: "arm64",
					},
				},
			},
		},
	}

	raw, err := MarshalAppcast(want)
	if err != nil {
		t.Fatalf("MarshalAppcast error = %v", err)
	}
	if !strings.Contains(string(raw), `xmlns:sparkle="`+SparkleNamespace+`"`) {
		t.Fatalf("marshaled XML missing xmlns:sparkle:\n%s", raw)
	}
	if !strings.Contains(string(raw), `xmlns:dc="http://purl.org/dc/elements/1.1/"`) {
		t.Fatalf("marshaled XML missing xmlns:dc:\n%s", raw)
	}

	got, err := ParseAppcast(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ParseAppcast(round-trip) error = %v\nXML:\n%s", err, raw)
	}

	if got.Title != want.Title || got.Link != want.Link || got.Description != want.Description || got.Language != want.Language {
		t.Fatalf("channel mismatch: got %+v, want %+v", got, want)
	}
	if len(got.Items) != len(want.Items) {
		t.Fatalf("items len = %d; want %d", len(got.Items), len(want.Items))
	}

	gi := got.Items[0]
	wi := want.Items[0]
	if gi.Title != wi.Title ||
		gi.Version != wi.Version ||
		gi.ShortVersion != wi.ShortVersion ||
		!gi.PubDate.Equal(wi.PubDate) ||
		gi.MinimumSystemVersion != wi.MinimumSystemVersion ||
		gi.Channel != wi.Channel ||
		gi.PhasedRolloutInterval != wi.PhasedRolloutInterval ||
		gi.CriticalUpdate != wi.CriticalUpdate ||
		gi.CriticalBelowVersion != wi.CriticalBelowVersion ||
		gi.ReleaseNotes != wi.ReleaseNotes ||
		gi.ReleaseNotesLink != wi.ReleaseNotesLink {
		t.Fatalf("item[0] mismatch:\n got: %+v\nwant: %+v", gi, wi)
	}
	if len(gi.Enclosures) != 2 {
		t.Fatalf("item[0] enclosures = %d; want 2", len(gi.Enclosures))
	}
	if gi.Enclosures[0] != wi.Enclosures[0] {
		t.Errorf("enclosure[0] mismatch:\n got: %+v\nwant: %+v", gi.Enclosures[0], wi.Enclosures[0])
	}
	if gi.Enclosures[1] != wi.Enclosures[1] {
		t.Errorf("enclosure[1] mismatch:\n got: %+v\nwant: %+v", gi.Enclosures[1], wi.Enclosures[1])
	}
	if got.Items[1].ReleaseNotes != want.Items[1].ReleaseNotes {
		t.Errorf("item[1] release notes = %q; want %q", got.Items[1].ReleaseNotes, want.Items[1].ReleaseNotes)
	}
}

func TestParseAppcastSparkle1AndNormalization(t *testing.T) {
	xmlData := `<?xml version="1.0" encoding="utf-8"?>
<rss version="2.0" xmlns:sparkle="http://www.andymatuschak.org/xml-namespaces/sparkle">
  <channel>
    <title>Sparkle Feed</title>
    <item>
      <title>Sparkle 1 macOS Default OS</title>
      <pubDate>Mon, 2 Jan 2026 09:15:00 +0000</pubDate>
      <description><![CDATA[<p>Hello CDATA</p>]]></description>
      <enclosure
        url="https://example.com/App-1.5.zip"
        length="5000"
        type="application/octet-stream"
        sparkle:version="150"
        sparkle:shortVersionString="1.5.0"
        sparkle:edSignature="SIG1" />
      <sparkle:deltas>
        <enclosure url="https://example.com/App-1.4-to-1.5.delta" length="500" sparkle:edSignature="DELTA" />
      </sparkle:deltas>
    </item>
    <item>
      <title>Sparkle 2 Multi-Platform</title>
      <pubDate>2026-03-10T12:00:00Z</pubDate>
      <enclosure url="https://example.com/App-2.0-win64.exe" sparkle:os="windows-x64" sparkle:version="old" />
      <enclosure url="https://example.com/App-2.0-win32.exe" sparkle:os="windows-x86" />
      <enclosure url="https://example.com/App-2.0-winarm.exe" sparkle:os="windows-arm64" />
      <enclosure url="https://example.com/App-2.0-mac.zip" sparkle:os="osx" sparkle:arch="aarch64" />
      <enclosure url="https://example.com/App-2.0-lin.tar.gz" sparkle:os="linux" sparkle:arch="x86_64" />
      <sparkle:version>2.0.0</sparkle:version>
      <sparkle:shortVersionString>2.0</sparkle:shortVersionString>
      <sparkle:criticalUpdate />
    </item>
  </channel>
</rss>`

	feed, err := ParseAppcast(strings.NewReader(xmlData))
	if err != nil {
		t.Fatalf("ParseAppcast error = %v", err)
	}
	if len(feed.Items) != 2 {
		t.Fatalf("len(Items) = %d; want 2", len(feed.Items))
	}

	// Item 0: Sparkle 1 attributes on enclosure, omitted OS -> macos, deltas ignored.
	i0 := feed.Items[0]
	if i0.Version != "150" || i0.ShortVersion != "1.5.0" {
		t.Errorf("Sparkle 1 fallback versions = (%q, %q); want (150, 1.5.0)", i0.Version, i0.ShortVersion)
	}
	if i0.PubDate.IsZero() || i0.PubDate.Year() != 2026 || i0.PubDate.Day() != 2 {
		t.Errorf("PubDate = %v; want 2026-01-02", i0.PubDate)
	}
	if i0.ReleaseNotes != "<p>Hello CDATA</p>" {
		t.Errorf("ReleaseNotes = %q; want CDATA content", i0.ReleaseNotes)
	}
	if len(i0.Enclosures) != 1 {
		t.Fatalf("len(i0.Enclosures) = %d; want 1 (delta enclosure must be skipped)", len(i0.Enclosures))
	}
	if i0.Enclosures[0].OS != "macos" || i0.Enclosures[0].Arch != "" {
		t.Errorf("default macOS enclosure = (%q, %q); want (macos, '')", i0.Enclosures[0].OS, i0.Enclosures[0].Arch)
	}

	// Item 1: Sparkle 2 <sparkle:version> overrides enclosure attribute even when appearing later.
	i1 := feed.Items[1]
	if i1.Version != "2.0.0" || i1.ShortVersion != "2.0" {
		t.Errorf("Sparkle 2 versions = (%q, %q); want (2.0.0, 2.0)", i1.Version, i1.ShortVersion)
	}
	if !i1.CriticalUpdate || i1.CriticalBelowVersion != "" {
		t.Errorf("CriticalUpdate = (%v, %q); want (true, '')", i1.CriticalUpdate, i1.CriticalBelowVersion)
	}
	if i1.PubDate.Year() != 2026 || i1.PubDate.Month() != time.March || i1.PubDate.Day() != 10 {
		t.Errorf("RFC3339 PubDate = %v; want 2026-03-10", i1.PubDate)
	}
	wantPairs := [][2]string{
		{"windows", "amd64"},
		{"windows", "386"},
		{"windows", "arm64"},
		{"macos", "arm64"},
		{"linux", "amd64"},
	}
	if len(i1.Enclosures) != len(wantPairs) {
		t.Fatalf("len(i1.Enclosures) = %d; want %d", len(i1.Enclosures), len(wantPairs))
	}
	for idx, wp := range wantPairs {
		enc := i1.Enclosures[idx]
		if enc.OS != wp[0] || enc.Arch != wp[1] {
			t.Errorf("enclosure[%d] = (%q, %q); want (%q, %q)", idx, enc.OS, enc.Arch, wp[0], wp[1])
		}
	}
}

func TestParseAppcastWithoutNamespaceDeclarationAndErrors(t *testing.T) {
	// Undeclared sparkle: prefix should still work.
	noNS := `<rss version="2.0"><channel><item>
		<sparkle:version>3.0.0</sparkle:version>
		<enclosure url="https://example.com/a.zip" sparkle:edSignature="SIG" sparkle:os="darwin" sparkle:arch="x64"/>
	</item></channel></rss>`
	feed, err := ParseAppcast(strings.NewReader(noNS))
	if err != nil {
		t.Fatalf("ParseAppcast(no xmlns) error = %v", err)
	}
	if len(feed.Items) != 1 || feed.Items[0].Version != "3.0.0" || feed.Items[0].Enclosures[0].OS != "macos" || feed.Items[0].Enclosures[0].Arch != "amd64" {
		t.Fatalf("unexpected parse result: %+v", feed.Items)
	}

	// Non-RSS root element fails.
	if _, err := ParseAppcast(strings.NewReader(`<html><body>nope</body></html>`)); err == nil {
		t.Errorf("ParseAppcast(html) = nil; want error")
	}

	// Exceeding 4 MiB cap fails.
	huge := bytes.NewReader(bytes.Repeat([]byte("a"), maxAppcastBytes+10))
	if _, err := ParseAppcast(huge); err == nil {
		t.Errorf("ParseAppcast(>4MiB) = nil; want error")
	}
}

func TestParseAppcastRolloutOverflowClamp(t *testing.T) {
	const wantMax = time.Duration(math.MaxInt64/int64(time.Second)) * time.Second
	pub := time.Date(2026, time.November, 1, 0, 0, 0, 0, time.UTC)

	for _, rawSecs := range []string{"10000000000", "9223372036854775807", "99999999999999999999999999"} {
		feedXML := `<?xml version="1.0" encoding="utf-8"?>
<rss version="2.0" xmlns:sparkle="http://www.andymatuschak.org/xml-namespaces/sparkle">
  <channel><item>
    <sparkle:version>2.0.0</sparkle:version>
    <pubDate>Sun, 01 Nov 2026 00:00:00 +0000</pubDate>
    <sparkle:phasedRolloutInterval>` + rawSecs + `</sparkle:phasedRolloutInterval>
    <enclosure url="https://example.com/app.zip" length="10" type="application/octet-stream" />
  </item></channel>
</rss>`
		feed, err := ParseAppcast(strings.NewReader(feedXML))
		if err != nil {
			t.Fatalf("ParseAppcast(%s) error = %v", rawSecs, err)
		}
		got := feed.Items[0].PhasedRolloutInterval
		if got != wantMax || got <= 0 {
			t.Errorf("phasedRolloutInterval(%s) = %v; want clamped positive %v", rawSecs, got, wantMax)
		}
		if isRolloutEligible(pub.Add(24*time.Hour), pub, got, 0.5) {
			t.Errorf("overflow rollout(%s) bypassed phased rollout eligibility", rawSecs)
		}
	}
}

func TestMarshalAppcastNormalizationAndDefaults(t *testing.T) {
	feed := &Appcast{
		Title: "Interop Feed",
		Items: []Item{
			{
				Version: "1.0.0",
				Channel: "stable",
				Enclosures: []Enclosure{{
					URL: "https://example.com/app-1.0.0.zip",
				}},
			},
			{
				Version: "1.1.0",
				Channel: " Default ",
				Enclosures: []Enclosure{{
					URL:    "https://example.com/app-1.1.0.zip",
					Length: 0,
				}},
			},
			{
				Version: "1.2.0-beta.1",
				Channel: " BETA ",
				Enclosures: []Enclosure{{
					URL:    "https://example.com/app-1.2.0-beta.1.zip",
					Length: 42,
					Type:   "application/zip",
				}},
			},
		},
	}

	raw, err := MarshalAppcast(feed)
	if err != nil {
		t.Fatalf("MarshalAppcast error = %v", err)
	}
	xmlStr := string(raw)
	if strings.Contains(xmlStr, "<sparkle:channel>stable</sparkle:channel>") ||
		strings.Contains(xmlStr, "<sparkle:channel>default</sparkle:channel>") ||
		strings.Contains(xmlStr, "Default") {
		t.Errorf("stable/default channels must be omitted from XML:\n%s", xmlStr)
	}
	if !strings.Contains(xmlStr, "<sparkle:channel>beta</sparkle:channel>") {
		t.Errorf("beta channel should be normalized and emitted:\n%s", xmlStr)
	}
	if !strings.Contains(xmlStr, `length="0"`) {
		t.Errorf("enclosure must always emit length attribute even when 0:\n%s", xmlStr)
	}
	if !strings.Contains(xmlStr, `type="application/octet-stream"`) {
		t.Errorf("enclosure with empty Type must default to application/octet-stream:\n%s", xmlStr)
	}
}
