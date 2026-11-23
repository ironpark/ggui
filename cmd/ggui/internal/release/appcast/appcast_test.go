package appcast

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ironpark/ggui/cmd/ggui/internal/release"
)

func TestKeyPairSignAndVerify(t *testing.T) {
	pubB64, privSeedB64, err := GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	pub, err := ParsePublicKey(pubB64)
	if err != nil {
		t.Fatal(err)
	}
	if len(pub) != ed25519.PublicKeySize {
		t.Fatalf("public key len = %d, want %d", len(pub), ed25519.PublicKeySize)
	}

	// ParsePrivateKey accepts 32-byte seed.
	privFromSeed, err := ParsePrivateKey(privSeedB64)
	if err != nil {
		t.Fatal(err)
	}
	derivedPub, err := PublicKeyFromPrivate(privSeedB64)
	if err != nil {
		t.Fatal(err)
	}
	if derivedPub != pubB64 {
		t.Fatalf("PublicKeyFromPrivate(seed) = %q, want %q", derivedPub, pubB64)
	}

	// ParsePrivateKey also accepts 64-byte full Ed25519 private key.
	fullPrivB64 := base64.StdEncoding.EncodeToString(privFromSeed)
	privFromFull, err := ParsePrivateKey(fullPrivB64)
	if err != nil {
		t.Fatal(err)
	}
	if !privFromFull.Equal(privFromSeed) {
		t.Fatal("64-byte full key did not match 32-byte seed key")
	}
	derivedFromFull, err := PublicKeyFromPrivate(fullPrivB64)
	if err != nil {
		t.Fatal(err)
	}
	if derivedFromFull != pubB64 {
		t.Fatalf("PublicKeyFromPrivate(full) = %q, want %q", derivedFromFull, pubB64)
	}

	// SignFile signs exact raw file bytes (Sparkle 2 format).
	dir := t.TempDir()
	artifact := filepath.Join(dir, "app.zip")
	payload := []byte("sparkle-2-raw-archive-bytes\x00\x01\x02")
	if err := os.WriteFile(artifact, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	sigB64, size, sumHex, err := SignFile(privSeedB64, artifact)
	if err != nil {
		t.Fatal(err)
	}
	if size != int64(len(payload)) {
		t.Errorf("size = %d, want %d", size, len(payload))
	}
	wantSum := sha256.Sum256(payload)
	if sumHex != hex.EncodeToString(wantSum[:]) {
		t.Errorf("sha256 = %q, want %q", sumHex, hex.EncodeToString(wantSum[:]))
	}
	// Independently verify raw Ed25519 signature.
	rawSig, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil {
		t.Fatal(err)
	}
	if !ed25519.Verify(pub, payload, rawSig) {
		t.Fatal("raw ed25519.Verify failed on SignFile output")
	}
	// Signing with 64-byte full key produces the identical deterministic signature.
	sigFull, _, _, err := SignFile(fullPrivB64, artifact)
	if err != nil {
		t.Fatal(err)
	}
	if sigFull != sigB64 {
		t.Errorf("signature mismatch between seed and full key")
	}

	// VerifyFile succeeds on valid file, fails on tampered file or wrong key.
	if err := VerifyFile(pubB64, artifact, sigB64); err != nil {
		t.Fatalf("VerifyFile valid: %v", err)
	}
	otherPub, _, err := GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyFile(otherPub, artifact, sigB64); !errors.Is(err, ErrInvalidSignature) {
		t.Errorf("VerifyFile wrong key: got %v, want ErrInvalidSignature", err)
	}
	tampered := filepath.Join(dir, "tampered.zip")
	if err := os.WriteFile(tampered, append(payload, '!'), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := VerifyFile(pubB64, tampered, sigB64); !errors.Is(err, ErrInvalidSignature) {
		t.Errorf("VerifyFile tampered: got %v, want ErrInvalidSignature", err)
	}

	// Corrupted 64-byte private key (public half altered) must be rejected.
	badFull := make([]byte, ed25519.PrivateKeySize)
	copy(badFull, privFromSeed)
	badFull[len(badFull)-1] ^= 0xff
	badFullB64 := base64.StdEncoding.EncodeToString(badFull)
	if _, err := ParsePrivateKey(badFullB64); err == nil {
		t.Error("corrupted 64-byte key was accepted")
	} else if strings.Contains(err.Error(), badFullB64) {
		t.Errorf("ParsePrivateKey error leaked key: %v", err)
	}

	// Invalid private key must never leak secret string in error.
	secretGarbage := "SUPER_SECRET_NOT_BASE64_KEY!!!"
	if _, err := ParsePrivateKey(secretGarbage); err == nil {
		t.Error("invalid base64 key was accepted")
	} else if strings.Contains(err.Error(), secretGarbage) {
		t.Errorf("ParsePrivateKey error leaked secret: %v", err)
	}
}

func TestMarshalAndParseRoundTrip(t *testing.T) {
	pubDate := time.Date(2026, 11, 23, 18, 50, 46, 0, time.UTC)
	feed := &Feed{
		Title:       "Hello World",
		Link:        "https://github.com/alice/hello",
		Description: "Updates for Hello & World",
		Language:    "en",
		Items: []Item{
			{
				Title:                 "Version 1.2.0",
				Version:               "1.2.0",
				ShortVersion:          "1.2.0",
				PubDate:               pubDate,
				MinimumSystemVersion:  "13.0",
				Channel:               "beta",
				PhasedRolloutInterval: 86400,
				CriticalUpdate:        true,
				ReleaseNotes:          "## What's new\n- Fix <bug> & \"quotes\"\n- Faster startup",
				ReleaseNotesLink:      "https://github.com/alice/hello/releases/tag/v1.2.0",
				Enclosure: Enclosure{
					URL:         "https://github.com/alice/hello/releases/download/v1.2.0/hello-1.2.0-macos-universal.zip",
					Length:      123456,
					Type:        DefaultEnclosureType,
					EdSignature: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==",
					SHA256:      strings.Repeat("ab", 32),
					OS:          "macos",
					Arch:        "universal",
				},
			},
			{
				Title:        "Version 1.2.0",
				Version:      "1.2.0",
				ShortVersion: "1.2.0",
				PubDate:      pubDate,
				Channel:      "stable", // omitted in XML and normalized to ""
				Enclosure: Enclosure{
					URL:                "https://github.com/alice/hello/releases/download/v1.2.0/hello-1.2.0-windows-amd64-setup.exe",
					Length:             654321,
					Type:               DefaultEnclosureType,
					EdSignature:        "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB==",
					SHA256:             strings.Repeat("cd", 32),
					OS:                 "windows",
					Arch:               "amd64",
					InstallerArguments: "--silent",
				},
			},
		},
	}

	raw, err := Marshal(feed)
	if err != nil {
		t.Fatal(err)
	}
	xmlStr := string(raw)
	for _, want := range []string{
		`<?xml version="1.0" encoding="utf-8"?>`,
		`xmlns:sparkle="http://www.andymatuschak.org/xml-namespaces/sparkle"`,
		`xmlns:dc="http://purl.org/dc/elements/1.1/"`,
		`<sparkle:version>1.2.0</sparkle:version>`,
		`<sparkle:shortVersionString>1.2.0</sparkle:shortVersionString>`,
		`<sparkle:minimumSystemVersion>13.0</sparkle:minimumSystemVersion>`,
		`<sparkle:channel>beta</sparkle:channel>`,
		`<sparkle:phasedRolloutInterval>86400</sparkle:phasedRolloutInterval>`,
		`<sparkle:criticalUpdate/>`,
		`<sparkle:releaseNotesLink>https://github.com/alice/hello/releases/tag/v1.2.0</sparkle:releaseNotesLink>`,
		`sparkle:os="macos"`,
		`sparkle:arch="universal"`,
		`sparkle:installerArguments="--silent"`,
	} {
		if !strings.Contains(xmlStr, want) {
			t.Errorf("marshaled XML lacks %q:\n%s", want, xmlStr)
		}
	}
	if strings.Contains(xmlStr, "<sparkle:channel>stable</sparkle:channel>") {
		t.Errorf("stable channel should be omitted:\n%s", xmlStr)
	}

	parsed, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse failed: %v\n%s", err, xmlStr)
	}
	if parsed.Title != feed.Title || parsed.Link != feed.Link || parsed.Description != feed.Description || parsed.Language != feed.Language {
		t.Errorf("feed header = %+v, want %+v", parsed, feed)
	}
	if len(parsed.Items) != 2 {
		t.Fatalf("parsed %d items, want 2", len(parsed.Items))
	}
	got0, want0 := parsed.Items[0], feed.Items[0]
	if got0.Title != want0.Title || got0.Version != want0.Version || got0.ShortVersion != want0.ShortVersion ||
		!got0.PubDate.Equal(want0.PubDate) || got0.MinimumSystemVersion != want0.MinimumSystemVersion ||
		got0.Channel != want0.Channel || got0.PhasedRolloutInterval != want0.PhasedRolloutInterval ||
		got0.CriticalUpdate != want0.CriticalUpdate || got0.ReleaseNotes != want0.ReleaseNotes ||
		got0.ReleaseNotesLink != want0.ReleaseNotesLink || got0.Enclosure != want0.Enclosure {
		t.Errorf("item 0 mismatch:\ngot  %+v\nwant %+v", got0, want0)
	}
	got1, want1 := parsed.Items[1], feed.Items[1]
	want1.Channel = "" // normalized
	if got1.Channel != "" || got1.CriticalUpdate || got1.Enclosure != want1.Enclosure {
		t.Errorf("item 1 mismatch:\ngot  %+v\nwant %+v", got1, want1)
	}
}

func TestParseExternalSparkleFeed(t *testing.T) {
	// Sparkle 1/2 feed using attributes on <enclosure>, CDATA description, and GMT pubDate.
	external := `<?xml version="1.0" encoding="utf-8"?>
<rss version="2.0" xmlns:sparkle="http://www.andymatuschak.org/xml-namespaces/sparkle">
  <channel>
    <title>External App</title>
    <item>
      <title>1.0.0</title>
      <pubDate>Mon, 02 Jan 2006 15:04:05 GMT</pubDate>
      <description><![CDATA[<p>Hello <b>world</b></p>]]></description>
      <guid isPermaLink="false">app-1.0.0</guid>
      <enclosure url="https://example.com/app-1.0.0.zip" length="42" type="application/octet-stream" sparkle:version="100" sparkle:shortVersionString="1.0.0" sparkle:edSignature="sig123" sparkle:os="macos"/>
    </item>
  </channel>
</rss>`
	feed, err := Parse([]byte(external))
	if err != nil {
		t.Fatal(err)
	}
	if len(feed.Items) != 1 {
		t.Fatalf("got %d items, want 1", len(feed.Items))
	}
	it := feed.Items[0]
	if it.Version != "100" || it.ShortVersion != "1.0.0" || it.ReleaseNotes != "<p>Hello <b>world</b></p>" || it.Enclosure.EdSignature != "sig123" || it.Enclosure.OS != "macos" {
		t.Errorf("parsed external item = %+v", it)
	}
}

func TestMerge(t *testing.T) {
	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	t3 := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

	existing := &Feed{
		Title:    "MyApp",
		Language: "en",
		Items: []Item{
			{Version: "1.0.0", PubDate: t1, Enclosure: Enclosure{URL: "https://ex/app-1.0.0-macos-universal.zip", OS: "macos", Arch: "universal", EdSignature: "old-1.0"}},
			{Version: "1.1.0", PubDate: t2, Enclosure: Enclosure{URL: "https://ex/app-1.1.0-macos-universal.zip", OS: "macos", Arch: "universal", EdSignature: "old-1.1"}},
			{Version: "1.1.0", PubDate: t2, Enclosure: Enclosure{URL: "https://ex/app-1.1.0-windows-amd64.zip", OS: "windows", Arch: "amd64", EdSignature: "old-1.1-win"}},
		},
	}

	incoming := []Item{
		// Re-release 1.1.0 macOS: should replace old-1.1
		{Version: "1.1.0", Channel: "stable", PubDate: t2, Enclosure: Enclosure{URL: "https://ex/app-1.1.0-macos-universal.zip", OS: "macos", Arch: "universal", EdSignature: "new-1.1"}},
		// New release 1.2.0 with both Windows .zip and -setup.exe plus macOS .zip
		{Version: "1.2.0", PubDate: t3, Enclosure: Enclosure{URL: "https://ex/app-1.2.0-macos-universal.zip", OS: "macos", Arch: "universal", EdSignature: "sig-1.2-mac"}},
		{Version: "1.2.0", PubDate: t3, Enclosure: Enclosure{URL: "https://ex/app-1.2.0-windows-amd64.zip", OS: "windows", Arch: "amd64", EdSignature: "sig-1.2-winzip"}},
		{Version: "1.2.0", PubDate: t3, Enclosure: Enclosure{URL: "https://ex/app-1.2.0-windows-amd64-setup.exe", OS: "windows", Arch: "amd64", EdSignature: "sig-1.2-setup", InstallerArguments: "--silent"}},
	}

	// Keep at most 2 distinct versions: 1.2.0 (3 items) and 1.1.0 (2 items); 1.0.0 pruned.
	merged := Merge(existing, incoming, 2)
	if merged.Title != "MyApp" || merged.Language != "en" {
		t.Errorf("metadata lost: %+v", merged)
	}
	if len(merged.Items) != 5 {
		t.Fatalf("merged has %d items, want 5: %+v", len(merged.Items), merged.Items)
	}
	// First 3 items must be 1.2.0, next 2 must be 1.1.0.
	for i := range 3 {
		if merged.Items[i].Version != "1.2.0" {
			t.Errorf("item[%d].Version = %q, want 1.2.0", i, merged.Items[i].Version)
		}
	}
	var sawReplacedMac bool
	for i := 3; i < 5; i++ {
		if merged.Items[i].Version != "1.1.0" {
			t.Errorf("item[%d].Version = %q, want 1.1.0", i, merged.Items[i].Version)
		}
		if merged.Items[i].Enclosure.OS == "macos" {
			sawReplacedMac = true
			if merged.Items[i].Enclosure.EdSignature != "new-1.1" {
				t.Errorf("re-released item not replaced: got sig %q, want new-1.1", merged.Items[i].Enclosure.EdSignature)
			}
		}
	}
	if !sawReplacedMac {
		t.Error("missing 1.1.0 macOS item after merge")
	}

	// Tie-breaking by semantic version when PubDate is equal (1.10.0 > 1.2.0 > 1.2.0-beta.1).
	sameTime := Merge(nil, []Item{
		{Version: "1.2.0-beta.1", PubDate: t1, Enclosure: Enclosure{URL: "https://ex/a.zip", OS: "macos", Arch: "arm64"}},
		{Version: "1.2.0", PubDate: t1, Enclosure: Enclosure{URL: "https://ex/b.zip", OS: "macos", Arch: "arm64"}},
		{Version: "1.10.0", PubDate: t1, Enclosure: Enclosure{URL: "https://ex/c.zip", OS: "macos", Arch: "arm64"}},
	}, 0)
	gotVers := []string{sameTime.Items[0].Version, sameTime.Items[1].Version, sameTime.Items[2].Version}
	wantVers := []string{"1.10.0", "1.2.0", "1.2.0-beta.1"}
	for i := range wantVers {
		if gotVers[i] != wantVers[i] {
			t.Errorf("version order = %v, want %v", gotVers, wantVers)
			break
		}
	}
}

func TestInferPlatformAndBuildFromAssets(t *testing.T) {
	for name, want := range map[string][2]string{
		"hello-1.2.0-macos-universal.zip":      {"macos", "universal"},
		"hello-1.2.0-macos-arm64.dmg":          {"macos", "arm64"},
		"hello-1.2.0-darwin-x86_64.zip":        {"macos", "amd64"},
		"hello-1.2.0-windows-amd64-setup.exe":  {"windows", "amd64"},
		"hello-1.2.0-windows-386.zip":          {"windows", "386"},
		"hello-1.2.0-linux-arm64.tar.gz":       {"linux", "arm64"},
		"hello-1.2.0-linux-riscv64.tar.gz":     {"linux", "riscv64"},
		"macos-helper-1.0.0-windows-amd64.zip": {"windows", "amd64"},
		"hello-1.2.0.dmg":                      {"macos", ""},
	} {
		gotOS, gotArch := InferPlatform(name)
		if gotOS != want[0] || gotArch != want[1] {
			t.Errorf("InferPlatform(%q) = (%q, %q), want (%q, %q)", name, gotOS, gotArch, want[0], want[1])
		}
	}

	pub, priv, err := GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	files := []string{
		"hello-1.2.0-macos-universal.zip",
		"hello-1.2.0-macos-universal.dmg",
		"hello-1.2.0-macos-universal.pkg", // skipped
		"hello-1.2.0-windows-amd64-setup.exe",
		"hello-1.2.0-windows-amd64.zip",
		"hello-1.2.0-linux-amd64.tar.gz",
		"hello_1.2.0_amd64.deb",    // skipped
		"hello-1.2.0-1.x86_64.rpm", // skipped
		release.SumsName,           // skipped
		DefaultFileName,            // skipped
	}
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("payload:"+name), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	assets, err := ScanDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	items, err := BuildFromAssets(assets, BuildOptions{
		Version:               "1.2.0",
		MinMacOSVersion:       "13.0",
		PhasedRolloutInterval: 3600,
		CriticalUpdate:        true,
		PrivateKey:            priv,
		BaseURL:               "https://example.com/releases/v1.2.0",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Expect 5 eligible items: mac zip, mac dmg, win setup.exe, win zip, linux tar.gz.
	if len(items) != 5 {
		t.Fatalf("got %d items, want 5: %+v", len(items), items)
	}
	byBase := map[string]Item{}
	for _, it := range items {
		base := urlBase(it.Enclosure.URL)
		byBase[base] = it
		// Every enclosure signature must verify against the actual file on disk.
		if err := VerifyFile(pub, filepath.Join(dir, base), it.Enclosure.EdSignature); err != nil {
			t.Errorf("VerifyFile(%s) failed: %v", base, err)
		}
		if it.Enclosure.OS == "macos" && it.MinimumSystemVersion != "13.0" {
			t.Errorf("%s MinimumSystemVersion = %q, want 13.0", base, it.MinimumSystemVersion)
		}
		if it.Enclosure.OS != "macos" && it.MinimumSystemVersion != "" {
			t.Errorf("%s MinimumSystemVersion = %q, want empty", base, it.MinimumSystemVersion)
		}
	}
	if setup, ok := byBase["hello-1.2.0-windows-amd64-setup.exe"]; !ok || setup.Enclosure.InstallerArguments != "--silent" {
		t.Errorf("windows setup item = %+v", setup)
	}
	for _, skipped := range []string{"hello-1.2.0-macos-universal.pkg", "hello_1.2.0_amd64.deb", "hello-1.2.0-1.x86_64.rpm", release.SumsName, DefaultFileName} {
		if _, ok := byBase[skipped]; ok {
			t.Errorf("asset %q should have been skipped", skipped)
		}
	}

	// Missing private key fails unless AllowUnsigned is true.
	if _, err := BuildFromAssets(assets, BuildOptions{Version: "1.2.0", BaseURL: "https://example.com"}); err == nil {
		t.Error("expected error when private key is empty and AllowUnsigned is false")
	}
	unsigned, err := BuildFromAssets(assets, BuildOptions{
		Version:       "1.2.0",
		BaseURL:       "https://example.com",
		AllowUnsigned: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(unsigned) != 5 || unsigned[0].Enclosure.EdSignature != "" || unsigned[0].Enclosure.SHA256 == "" {
		t.Errorf("unsigned build unexpected: %+v", unsigned[0])
	}
}

func TestLoadSource(t *testing.T) {
	// Missing local file returns (nil, false, nil).
	f, found, err := LoadSource(t.Context(), filepath.Join(t.TempDir(), "missing.xml"))
	if err != nil || found || f != nil {
		t.Fatalf("missing file: got (%v, %v, %v)", f, found, err)
	}

	sample, err := Marshal(&Feed{Title: "Demo", Language: "en"})
	if err != nil {
		t.Fatal(err)
	}
	localPath := filepath.Join(t.TempDir(), "appcast.xml")
	if err := os.WriteFile(localPath, sample, 0o644); err != nil {
		t.Fatal(err)
	}
	f, found, err = LoadSource(t.Context(), localPath)
	if err != nil || !found || f == nil || f.Title != "Demo" {
		t.Fatalf("local file: got (%+v, %v, %v)", f, found, err)
	}

	// HTTP 200, 404, 500.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok.xml":
			w.Header().Set("Content-Type", "application/xml")
			w.Write(sample)
		case "/missing.xml":
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	f, found, err = LoadSource(t.Context(), srv.URL+"/ok.xml")
	if err != nil || !found || f == nil || f.Title != "Demo" {
		t.Fatalf("HTTP 200: got (%+v, %v, %v)", f, found, err)
	}
	f, found, err = LoadSource(t.Context(), srv.URL+"/missing.xml")
	if err != nil || found || f != nil {
		t.Fatalf("HTTP 404: got (%v, %v, %v)", f, found, err)
	}
	if _, _, err := LoadSource(t.Context(), srv.URL+"/boom.xml"); err == nil {
		t.Error("HTTP 500 did not return an error")
	}
}

func TestCriticalBelowVersionRoundTrip(t *testing.T) {
	pubDate := time.Date(2026, 11, 23, 18, 50, 46, 0, time.UTC)
	feed := &Feed{
		Title:    "SecApp",
		Language: "en",
		Items: []Item{
			{
				Title:                "Version 2.0.0",
				Version:              "2.0.0",
				ShortVersion:         "2.0.0",
				PubDate:              pubDate,
				CriticalUpdate:       true,
				CriticalBelowVersion: "1.5.0",
				Enclosure: Enclosure{
					URL:    "https://example.com/app-2.0.0.zip",
					Length: 100,
					OS:     "macos",
				},
			},
			{
				Title:          "Version 2.1.0",
				Version:        "2.1.0",
				ShortVersion:   "2.1.0",
				PubDate:        pubDate.Add(time.Hour),
				CriticalUpdate: true,
				Enclosure: Enclosure{
					URL:    "https://example.com/app-2.1.0.zip",
					Length: 100,
					OS:     "macos",
				},
			},
		},
	}

	raw, err := Marshal(feed)
	if err != nil {
		t.Fatal(err)
	}
	xmlStr := string(raw)
	if !strings.Contains(xmlStr, `<sparkle:criticalUpdate sparkle:version="1.5.0"/>`) {
		t.Errorf("missing threshold criticalUpdate tag:\n%s", xmlStr)
	}
	if !strings.Contains(xmlStr, `<sparkle:criticalUpdate/>`) {
		t.Errorf("missing unconditional criticalUpdate tag:\n%s", xmlStr)
	}

	parsed, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Items) != 2 {
		t.Fatalf("got %d items, want 2", len(parsed.Items))
	}
	if !parsed.Items[0].CriticalUpdate || parsed.Items[0].CriticalBelowVersion != "1.5.0" {
		t.Errorf("item 0 critical = (%v, %q), want (true, 1.5.0)", parsed.Items[0].CriticalUpdate, parsed.Items[0].CriticalBelowVersion)
	}
	if !parsed.Items[1].CriticalUpdate || parsed.Items[1].CriticalBelowVersion != "" {
		t.Errorf("item 1 critical = (%v, %q), want (true, empty)", parsed.Items[1].CriticalUpdate, parsed.Items[1].CriticalBelowVersion)
	}

	// Bare version attribute and case-insensitive sparkle:VERSION attribute.
	bareXML := `<?xml version="1.0" encoding="utf-8"?>
<RSS version="2.0" xmlns:sparkle="http://www.andymatuschak.org/xml-namespaces/sparkle">
  <Channel>
    <title>  Demo App  </title>
    <link> https://example.com </link>
    <language> en </language>
    <item>
      <title>  Version 3.0.0  </title>
      <sparkle:Version>3.0.0</sparkle:Version>
      <sparkle:CriticalUpdate version="2.5.0"/>
      <enclosure url="https://example.com/app-3.0.0.zip" length="10" type="application/octet-stream" sparkle:EDSIGNATURE="sig3"/>
    </item>
    <item>
      <title>Version 3.1.0</title>
      <sparkle:version>3.1.0</sparkle:version>
      <sparkle:criticalUpdate sparkle:VERSION="3.0.0"/>
      <enclosure url="https://example.com/app-3.1.0.zip" length="10" type="application/octet-stream"/>
    </item>
  </Channel>
</RSS>`
	parsed2, err := Parse([]byte(bareXML))
	if err != nil {
		t.Fatalf("Parse case-insensitive XML: %v", err)
	}
	if parsed2.Title != "Demo App" || parsed2.Link != "https://example.com" || parsed2.Language != "en" {
		t.Errorf("untrimmed channel fields: %+v", parsed2)
	}
	if len(parsed2.Items) != 2 {
		t.Fatalf("got %d items, want 2", len(parsed2.Items))
	}
	if parsed2.Items[0].Title != "Version 3.0.0" || parsed2.Items[0].CriticalBelowVersion != "2.5.0" || parsed2.Items[0].Enclosure.EdSignature != "sig3" {
		t.Errorf("item 0 = %+v", parsed2.Items[0])
	}
	if parsed2.Items[1].CriticalBelowVersion != "3.0.0" {
		t.Errorf("item 1 CriticalBelowVersion = %q, want 3.0.0", parsed2.Items[1].CriticalBelowVersion)
	}
}

func TestParseAndMergeMultiEnclosureItem(t *testing.T) {
	xmlDoc := `<?xml version="1.0" encoding="utf-8"?>
<rss version="2.0" xmlns:sparkle="http://www.andymatuschak.org/xml-namespaces/sparkle">
  <channel>
    <title>MultiApp</title>
    <item>
      <title>Version 1.0.0</title>
      <pubDate>Mon, 02 Jan 2026 15:04:05 +0000</pubDate>
      <description>Multi-platform release</description>
      <enclosure url="https://ex/app-1.0.0-macos.zip" length="10" type="application/octet-stream" sparkle:version="100" sparkle:shortVersionString="1.0.0" sparkle:os="macos" sparkle:arch="universal" sparkle:edSignature="sig-mac"/>
      <enclosure url="https://ex/app-1.0.0-windows.zip" length="20" type="application/octet-stream" sparkle:version="999" sparkle:os="windows" sparkle:arch="amd64" sparkle:edSignature="sig-win"/>
      <enclosure url="https://ex/app-1.0.0-linux.tar.gz" length="30" type="application/octet-stream" sparkle:os="linux" sparkle:arch="amd64" sparkle:edSignature="sig-lin"/>
    </item>
    <item>
      <title>Notes Only</title>
      <sparkle:version>0.9.0</sparkle:version>
      <pubDate>Sun, 01 Jan 2026 15:04:05 +0000</pubDate>
    </item>
  </channel>
</rss>`

	feed, err := Parse([]byte(xmlDoc))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(feed.Items) != 4 {
		t.Fatalf("got %d items, want 4: %+v", len(feed.Items), feed.Items)
	}
	// First 3 items come from the multi-enclosure <item>, using first non-empty fallback version.
	wantOS := []string{"macos", "windows", "linux"}
	for i, osName := range wantOS {
		it := feed.Items[i]
		if it.Version != "100" || it.ShortVersion != "1.0.0" || it.Enclosure.OS != osName || it.ReleaseNotes != "Multi-platform release" {
			t.Errorf("item[%d] = %+v, want version=100 short=1.0.0 os=%s", i, it, osName)
		}
	}
	if feed.Items[3].Version != "0.9.0" || feed.Items[3].Enclosure.URL != "" {
		t.Errorf("item[3] = %+v, want 0.9.0 with empty enclosure", feed.Items[3])
	}

	// Merging a new release preserves all 3 enclosures from the 1.0.0 multi-enclosure item.
	merged := Merge(feed, []Item{{
		Title:   "Version 1.1.0",
		Version: "101",
		PubDate: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
		Enclosure: Enclosure{
			URL: "https://ex/app-1.1.0-macos.zip",
			OS:  "macos",
		},
	}}, 2)
	if len(merged.Items) != 4 {
		t.Fatalf("merged has %d items, want 4 (1 from 101 + 3 from 100): %+v", len(merged.Items), merged.Items)
	}
}

func TestChannelNormalizationAndPerChannelPruning(t *testing.T) {
	for in, want := range map[string]string{
		"":          "",
		"  ":        "",
		"stable":    "",
		"STABLE":    "",
		"default":   "",
		" Default ": "",
		"Beta":      "beta",
		" NIGHTLY ": "nightly",
	} {
		if got := normalizeChannel(in); got != want {
			t.Errorf("normalizeChannel(%q) = %q, want %q", in, got, want)
		}
	}

	// Marshal omits "default" and lowercases non-default channels.
	raw, err := Marshal(&Feed{
		Title: "App",
		Items: []Item{
			{Version: "1.0.0", Channel: "Default", Enclosure: Enclosure{URL: "https://ex/a.zip", OS: "macos"}},
			{Version: "1.1.0", Channel: "Beta", Enclosure: Enclosure{URL: "https://ex/b.zip", OS: "macos"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "default") || strings.Contains(string(raw), "Default") {
		t.Errorf("default channel should be omitted:\n%s", raw)
	}
	if !strings.Contains(string(raw), "<sparkle:channel>beta</sparkle:channel>") {
		t.Errorf("expected lowercase beta channel:\n%s", raw)
	}

	// Per-channel maxVersions: 4 newer beta builds must not evict older stable builds.
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	items := []Item{
		{Version: "0.9.0", Channel: "stable", PubDate: base, Enclosure: Enclosure{URL: "https://ex/0.9.zip", OS: "macos"}},
		{Version: "1.0.0", Channel: "default", PubDate: base.Add(24 * time.Hour), Enclosure: Enclosure{URL: "https://ex/1.0.zip", OS: "macos"}},
		{Version: "1.1.0", Channel: "", PubDate: base.Add(48 * time.Hour), Enclosure: Enclosure{URL: "https://ex/1.1.zip", OS: "macos"}},
		{Version: "1.2.0-beta.1", Channel: "beta", PubDate: base.Add(72 * time.Hour), Enclosure: Enclosure{URL: "https://ex/b1.zip", OS: "macos"}},
		{Version: "1.2.0-beta.2", Channel: "Beta", PubDate: base.Add(96 * time.Hour), Enclosure: Enclosure{URL: "https://ex/b2.zip", OS: "macos"}},
		{Version: "1.2.0-beta.3", Channel: "beta", PubDate: base.Add(120 * time.Hour), Enclosure: Enclosure{URL: "https://ex/b3.zip", OS: "macos"}},
		{Version: "1.2.0-beta.4", Channel: "BETA", PubDate: base.Add(144 * time.Hour), Enclosure: Enclosure{URL: "https://ex/b4.zip", OS: "macos"}},
	}
	merged := Merge(nil, items, 2)
	gotVers := make([]string, 0, len(merged.Items))
	for _, it := range merged.Items {
		gotVers = append(gotVers, it.Channel+":"+it.Version)
	}
	wantVers := []string{"beta:1.2.0-beta.4", "beta:1.2.0-beta.3", ":1.1.0", ":1.0.0"}
	if strings.Join(gotVers, ",") != strings.Join(wantVers, ",") {
		t.Errorf("per-channel Merge = %v, want %v", gotVers, wantVers)
	}
}

func TestCompareVersionsSparkle2(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"", "0.0.0", -1},
		{"0.0.0", "", 1},
		{"v1.2.0", "1.2.0", 0},
		{"V1.2.0", "1.2", 0},
		{"1.2.0+build.123", "1.2.0+build.999", 0},
		{"1.2a2", "1.2b1", -1},
		{"1.2b2", "1.2b10", -1},
		{"1.2rc1", "1.2.0-rc.1", 0},
		{"1.2.0-RC.1", "1.2.0-rc.1", 0},
		{"1.2.0-beta_2", "1.2.0-beta.2", 0},
		{"1.2rc1", "1.2.0", -1},
		{"1.10.0", "1.2.0", 1},
		{"99999999999999999999999999.0.0", "99999999999999999999999998.0.0", 1},
	}
	for _, tc := range cases {
		if got := compareVersions(tc.a, tc.b); got != tc.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestInferPlatformEdgeCases(t *testing.T) {
	for name, want := range map[string][2]string{
		"my-win-app-1.0.0.dmg":               {"macos", ""},
		"my-win-app-1.0.0-arm64.dmg":         {"macos", "arm64"},
		"my-win-app-1.0.0.pkg":               {"macos", ""},
		"darwin-notes-1.0.0-setup.exe":       {"windows", ""},
		"darwin-notes-1.0.0-amd64_setup.exe": {"windows", "amd64"},
		"darwin-notes-1.0.0-win64-setup.exe": {"windows", "amd64"},
		"setup.exe":                          {"windows", ""},
		"darwin-notes-1.0.0.exe":             {"windows", ""},
		"my-mac-app_1.0.0_amd64.deb":         {"linux", "amd64"},
		"my-win-app-1.0.0-1.x86_64.rpm":      {"linux", "amd64"},
		"install-all-1.0.0-linux.tar.gz":     {"linux", ""},
		"install-all-1.0.0-macos.dmg":        {"macos", ""},
		"app-1.0.0-macos-universal2.dmg":     {"macos", "universal"},
		"app-1.0.0-macos-fat.zip":            {"macos", "universal"},
		"fat-app-1.0.0-linux.tar.gz":         {"linux", ""},
		"universal-app-1.0.0-windows.zip":    {"windows", ""},
		"app-1.0.0-windows-ia32.zip":         {"windows", "386"},
	} {
		gotOS, gotArch := InferPlatform(name)
		if gotOS != want[0] || gotArch != want[1] {
			t.Errorf("InferPlatform(%q) = (%q, %q), want (%q, %q)", name, gotOS, gotArch, want[0], want[1])
		}
	}

	// BuildFromAssets preserves my-win-app.dmg, darwin-notes-setup.exe, _setup.exe, setup.exe,
	// and never emits arch="universal" on non-macOS targets.
	dir := t.TempDir()
	for _, name := range []string{
		"my-win-app-1.0.0.dmg",
		"darwin-notes-1.0.0-setup.exe",
		"app-1.0.0-amd64_setup.exe",
		"setup.exe",
		"install-all-1.0.0-linux.tar.gz",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("data:"+name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	assets, err := ScanDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Also test explicit Arch="universal" on Linux gets cleared.
	assets = append(assets, release.Asset{
		Name:   "explicit-linux.tar.gz",
		Path:   filepath.Join(dir, "install-all-1.0.0-linux.tar.gz"),
		Target: "linux",
		Arch:   "universal",
	})
	items, err := BuildFromAssets(assets, BuildOptions{
		Version:       "1.0.0",
		BaseURL:       "https://example.com/releases/v1.0.0",
		AllowUnsigned: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 6 {
		t.Fatalf("got %d items, want 6: %+v", len(items), items)
	}
	for _, it := range items {
		base := urlBase(it.Enclosure.URL)
		if it.Enclosure.OS != "macos" && it.Enclosure.Arch == "universal" {
			t.Errorf("%s has invalid non-macos universal arch", base)
		}
		if (strings.HasSuffix(base, "_setup.exe") || base == "setup.exe" || strings.HasSuffix(base, "-setup.exe")) &&
			it.Enclosure.InstallerArguments != "--silent" {
			t.Errorf("%s InstallerArguments = %q, want --silent", base, it.Enclosure.InstallerArguments)
		}
	}
}

func TestParsePubDateLayouts(t *testing.T) {
	dates := []string{
		"2026-11-23T18:50:46.123456789Z",
		"2 Jan 2006 15:04:05 -0700",
		"2 Jan 2006 15:04:05 GMT",
		"Mon, 02 Jan 2006 15:04 -0700",
		"Mon, 2 Jan 2006 15:04 -0700",
	}
	for _, d := range dates {
		tm, err := parsePubDate(d)
		if err != nil || tm.IsZero() {
			t.Errorf("parsePubDate(%q) = (%v, %v), want valid time", d, tm, err)
		}
	}
}

func TestFetchURLSecurity(t *testing.T) {
	sample, err := Marshal(&Feed{Title: "Secure", Language: "en"})
	if err != nil {
		t.Fatal(err)
	}

	httpSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/downgraded.xml":
			w.Write(sample)
		case "/loop":
			http.Redirect(w, r, "/loop", http.StatusFound)
		case "/huge.xml":
			w.Header().Set("Content-Type", "application/xml")
			chunk := make([]byte, 1<<20)
			for i := range chunk {
				chunk[i] = ' '
			}
			for range 17 {
				w.Write(chunk)
			}
		}
	}))
	defer httpSrv.Close()

	tlsSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/downgrade":
			http.Redirect(w, r, httpSrv.URL+"/downgraded.xml", http.StatusFound)
		case "/ok-redirect":
			http.Redirect(w, r, "/final.xml", http.StatusFound)
		case "/final.xml":
			w.Write(sample)
		}
	}))
	defer tlsSrv.Close()

	origTransport := fetchTransport
	fetchTransport = tlsSrv.Client().Transport
	t.Cleanup(func() { fetchTransport = origTransport })

	// 1. HTTPS -> HTTP downgrade must be blocked.
	if _, _, err := LoadSource(t.Context(), tlsSrv.URL+"/downgrade"); err == nil || !strings.Contains(err.Error(), "downgrade") {
		t.Errorf("https->http redirect: got %v, want downgrade error", err)
	}

	// 2. HTTPS -> HTTPS redirect succeeds.
	f, found, err := LoadSource(t.Context(), tlsSrv.URL+"/ok-redirect")
	if err != nil || !found || f == nil || f.Title != "Secure" {
		t.Errorf("https->https redirect: got (%+v, %v, %v)", f, found, err)
	}

	// 3. Too many redirects (> 10) rejected.
	if _, _, err := LoadSource(t.Context(), httpSrv.URL+"/loop"); err == nil || !strings.Contains(err.Error(), "redirects") {
		t.Errorf("redirect loop: got %v, want redirect limit error", err)
	}

	// 4. Feed exceeding 16 MiB returns explicit error.
	if _, _, err := LoadSource(t.Context(), httpSrv.URL+"/huge.xml"); err == nil || !strings.Contains(err.Error(), "exceeds maximum size") {
		t.Errorf("oversized feed: got %v, want size limit error", err)
	}
}
