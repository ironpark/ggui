package updater

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type mockApplier struct {
	mu      sync.Mutex
	calls   int
	lastUpd *DownloadedUpdate
	lastOpt ApplyOptions
	err     error
}

func (m *mockApplier) Apply(_ context.Context, upd *DownloadedUpdate, opts ApplyOptions) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	m.lastUpd = upd
	m.lastOpt = opts
	return m.err
}

func TestNewValidation(t *testing.T) {
	pub, _, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}
	dir := t.TempDir()

	if _, err := New(Config{PublicKey: pub, StateDir: dir, CacheDir: dir}); err == nil {
		t.Errorf("New(empty FeedURL) = nil; want error")
	}
	if _, err := New(Config{FeedURL: "http://example.com/appcast.xml", PublicKey: pub, StateDir: dir, CacheDir: dir}); !errors.Is(err, ErrInsecureURL) {
		t.Errorf("New(http FeedURL) = %v; want ErrInsecureURL", err)
	}
	if _, err := New(Config{FeedURL: "https://example.com/appcast.xml", StateDir: dir, CacheDir: dir}); err == nil {
		t.Errorf("New(missing PublicKey) = nil; want error")
	}
	if _, err := New(Config{FeedURL: "https://example.com/appcast.xml", PublicKey: "bad-key", StateDir: dir, CacheDir: dir}); err == nil {
		t.Errorf("New(bad PublicKey) = nil; want error")
	}

	u, err := New(Config{
		FeedURL:        "https://example.com/appcast.xml",
		PublicKey:      pub,
		CurrentVersion: "1.0.0",
		OS:             "darwin",
		Arch:           "x86_64",
		StateDir:       dir,
		CacheDir:       dir,
	})
	if err != nil {
		t.Fatalf("New(valid) error = %v", err)
	}
	cfg := u.Config()
	if cfg.OS != "macos" || cfg.Arch != "amd64" {
		t.Errorf("normalized OS/Arch = (%q, %q); want (macos, amd64)", cfg.OS, cfg.Arch)
	}
	if cfg.CheckInterval != DefaultCheckInterval || cfg.MaxDownloadBytes != DefaultMaxDownloadBytes {
		t.Errorf("defaults mismatch: interval=%v maxBytes=%d", cfg.CheckInterval, cfg.MaxDownloadBytes)
	}
}

func TestSelectReleaseFiltering(t *testing.T) {
	pub, _, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}

	baseTime := time.Date(2026, time.November, 1, 0, 0, 0, 0, time.UTC)
	now := baseTime

	newUpdater := func(t *testing.T, mod func(*Config)) *Updater {
		t.Helper()
		dir := t.TempDir()
		cfg := Config{
			FeedURL:        "https://example.com/appcast.xml",
			PublicKey:      pub,
			CurrentVersion: "1.0.0",
			OS:             "macos",
			Arch:           "arm64",
			OSVersion:      "14.5",
			StateDir:       dir,
			CacheDir:       filepath.Join(dir, "cache"),
			Now:            func() time.Time { return now },
		}
		if mod != nil {
			mod(&cfg)
		}
		u, err := New(cfg)
		if err != nil {
			t.Fatalf("New error = %v", err)
		}
		return u
	}

	t.Run("channel filtering", func(t *testing.T) {
		feed := &Appcast{Items: []Item{
			{
				Version: "1.1.0",
				Enclosures: []Enclosure{
					{URL: "https://example.com/1.1.zip", OS: "macos", Arch: "arm64"},
				},
			},
			{
				Version: "1.2.0-beta.1",
				Channel: "beta",
				Enclosures: []Enclosure{
					{URL: "https://example.com/1.2-beta.zip", OS: "macos", Arch: "arm64"},
				},
			},
			{
				Version: "1.3.0-nightly.1",
				Channel: "nightly",
				Enclosures: []Enclosure{
					{URL: "https://example.com/1.3-nightly.zip", OS: "macos", Arch: "arm64"},
				},
			},
		}}

		uStable := newUpdater(t, func(c *Config) { c.Channel = "" })
		rel, ok := uStable.SelectRelease(feed, CheckOptions{})
		if !ok || rel.CompareVersion() != "1.1.0" {
			t.Fatalf("stable channel got (%v, %v); want 1.1.0", rel, ok)
		}

		uBeta := newUpdater(t, func(c *Config) { c.Channel = "beta" })
		rel, ok = uBeta.SelectRelease(feed, CheckOptions{})
		if !ok || rel.CompareVersion() != "1.2.0-beta.1" {
			t.Fatalf("beta channel got (%v, %v); want 1.2.0-beta.1", rel, ok)
		}

		// Beta user still receives higher stable release over older beta.
		feed.Items = append(feed.Items, Item{
			Version: "1.2.0",
			Enclosures: []Enclosure{
				{URL: "https://example.com/1.2.0.zip", OS: "macos", Arch: "arm64"},
			},
		})
		rel, ok = uBeta.SelectRelease(feed, CheckOptions{})
		if !ok || rel.CompareVersion() != "1.2.0" {
			t.Fatalf("beta channel with newer stable got (%v, %v); want 1.2.0", rel, ok)
		}
	})

	t.Run("minimum OS version", func(t *testing.T) {
		feed := &Appcast{Items: []Item{
			{
				Version:              "2.0.0",
				MinimumSystemVersion: "15.0",
				Enclosures:           []Enclosure{{URL: "https://example.com/2.0.zip", OS: "macos", Arch: "arm64"}},
			},
			{
				Version:              "1.5.0",
				MinimumSystemVersion: "14.0",
				Enclosures:           []Enclosure{{URL: "https://example.com/1.5.zip", OS: "macos", Arch: "arm64"}},
			},
		}}
		u := newUpdater(t, func(c *Config) { c.OSVersion = "14.5" })
		rel, ok := u.SelectRelease(feed, CheckOptions{})
		if !ok || rel.CompareVersion() != "1.5.0" {
			t.Fatalf("minOS filter got (%v, %v); want 1.5.0", rel, ok)
		}
	})

	t.Run("skipped version and critical update bypass", func(t *testing.T) {
		feed := &Appcast{Items: []Item{
			{
				Version:    "1.5.0",
				Enclosures: []Enclosure{{URL: "https://example.com/1.5.zip", OS: "macos", Arch: "arm64"}},
			},
		}}
		u := newUpdater(t, nil)
		if err := u.SkipVersion("1.5.0"); err != nil {
			t.Fatalf("SkipVersion error = %v", err)
		}
		if _, ok := u.SelectRelease(feed, CheckOptions{}); ok {
			t.Fatalf("auto check should skip 1.5.0")
		}
		if rel, ok := u.SelectRelease(feed, CheckOptions{Manual: true}); !ok || rel.CompareVersion() != "1.5.0" {
			t.Fatalf("manual check should bypass skip; got (%v, %v)", rel, ok)
		}
		if rel, ok := u.SelectRelease(feed, CheckOptions{IncludeSkipped: true}); !ok || rel.CompareVersion() != "1.5.0" {
			t.Fatalf("IncludeSkipped should bypass skip; got (%v, %v)", rel, ok)
		}

		// Critical update below 1.2.0 applies to 1.0.0 and bypasses skip.
		feed.Items[0].CriticalUpdate = true
		feed.Items[0].CriticalBelowVersion = "1.2.0"
		if rel, ok := u.SelectRelease(feed, CheckOptions{}); !ok || rel.CompareVersion() != "1.5.0" {
			t.Fatalf("critical update should bypass skip; got (%v, %v)", rel, ok)
		}

		// When CurrentVersion >= CriticalBelowVersion, skip still applies.
		uNewer := newUpdater(t, func(c *Config) { c.CurrentVersion = "1.2.0" })
		_ = uNewer.SkipVersion("1.5.0")
		if _, ok := uNewer.SelectRelease(feed, CheckOptions{}); ok {
			t.Fatalf("non-applicable criticalUpdate version should remain skipped")
		}

		// ClearSkippedVersion restores auto check eligibility.
		feed.Items[0].CriticalUpdate = false
		if err := u.ClearSkippedVersion(); err != nil {
			t.Fatalf("ClearSkippedVersion error = %v", err)
		}
		if rel, ok := u.SelectRelease(feed, CheckOptions{}); !ok || rel.CompareVersion() != "1.5.0" {
			t.Fatalf("after ClearSkippedVersion got (%v, %v); want 1.5.0", rel, ok)
		}
	})

	t.Run("phased rollout deterministic bucket", func(t *testing.T) {
		feed := &Appcast{Items: []Item{
			{
				Version:               "2.0.0",
				PubDate:               baseTime,
				PhasedRolloutInterval: 24 * time.Hour,
				Enclosures:            []Enclosure{{URL: "https://example.com/2.0.zip", OS: "macos", Arch: "arm64"}},
			},
		}}

		u := newUpdater(t, nil)
		if err := u.state.setRolloutSeed(0.5); err != nil {
			t.Fatalf("setRolloutSeed error = %v", err)
		}

		// Before threshold (11h < 12h): ineligible for auto, eligible for manual.
		now = baseTime.Add(11 * time.Hour)
		if _, ok := u.SelectRelease(feed, CheckOptions{}); ok {
			t.Fatalf("expected rollout ineligible at 11h for bucket 0.5")
		}
		if rel, ok := u.SelectRelease(feed, CheckOptions{Manual: true}); !ok || rel.CompareVersion() != "2.0.0" {
			t.Fatalf("manual check should bypass rollout; got (%v, %v)", rel, ok)
		}

		// Critical update bypasses rollout even before threshold.
		feed.Items[0].CriticalUpdate = true
		if rel, ok := u.SelectRelease(feed, CheckOptions{}); !ok || rel.CompareVersion() != "2.0.0" {
			t.Fatalf("critical update should bypass rollout; got (%v, %v)", rel, ok)
		}
		feed.Items[0].CriticalUpdate = false

		// At threshold (12h == 0.5 * 24h): eligible for auto check.
		now = baseTime.Add(12 * time.Hour)
		if rel, ok := u.SelectRelease(feed, CheckOptions{}); !ok || rel.CompareVersion() != "2.0.0" {
			t.Fatalf("expected rollout eligible at 12h for bucket 0.5; got (%v, %v)", rel, ok)
		}
	})

	t.Run("enclosure arch and Windows setup vs zip preference", func(t *testing.T) {
		// macOS prefers exact arch over universal over empty arch.
		macFeed := &Appcast{Items: []Item{
			{
				Version: "2.0.0",
				Enclosures: []Enclosure{
					{URL: "https://example.com/app-legacy.zip", OS: "macos", Arch: ""},
					{URL: "https://example.com/app-universal.zip", OS: "macos", Arch: "universal"},
					{URL: "https://example.com/app-amd64.zip", OS: "macos", Arch: "amd64"},
					{URL: "https://example.com/app-arm64.zip", OS: "macos", Arch: "arm64"},
				},
			},
		}}
		uArm := newUpdater(t, func(c *Config) { c.OS = "macos"; c.Arch = "arm64" })
		rel, ok := uArm.SelectRelease(macFeed, CheckOptions{})
		if !ok || rel.Enclosure.URL != "https://example.com/app-arm64.zip" {
			t.Fatalf("macOS arm64 got %v; want arm64 zip", rel)
		}

		// When exact arch missing, macOS falls back to universal.
		macFeed.Items[0].Enclosures = macFeed.Items[0].Enclosures[:3]
		rel, ok = uArm.SelectRelease(macFeed, CheckOptions{})
		if !ok || rel.Enclosure.URL != "https://example.com/app-universal.zip" {
			t.Fatalf("macOS arm64 fallback got %v; want universal zip", rel)
		}

		// Windows setup vs zip selection.
		winFeed := &Appcast{Items: []Item{
			{
				Version: "2.0.0",
				Enclosures: []Enclosure{
					{URL: "https://example.com/App-2.0.0-windows-amd64.zip", OS: "windows", Arch: "amd64"},
					{URL: "https://example.com/App-2.0.0-windows-amd64-setup.exe", OS: "windows", Arch: "amd64"},
				},
			},
		}}

		prevDetect := windowsInstalledFunc
		t.Cleanup(func() { windowsInstalledFunc = prevDetect })

		windowsInstalledFunc = func() bool { return false }
		uWinPortable := newUpdater(t, func(c *Config) { c.OS = "windows"; c.Arch = "amd64" })
		rel, ok = uWinPortable.SelectRelease(winFeed, CheckOptions{})
		if !ok || rel.Enclosure.URL != "https://example.com/App-2.0.0-windows-amd64.zip" {
			t.Fatalf("Windows portable got %v; want .zip", rel)
		}

		uWinPreferSetup := newUpdater(t, func(c *Config) { c.OS = "windows"; c.Arch = "amd64"; c.PreferWindowsSetup = true })
		rel, ok = uWinPreferSetup.SelectRelease(winFeed, CheckOptions{})
		if !ok || rel.Enclosure.URL != "https://example.com/App-2.0.0-windows-amd64-setup.exe" {
			t.Fatalf("Windows PreferWindowsSetup got %v; want -setup.exe", rel)
		}

		windowsInstalledFunc = func() bool { return true }
		rel, ok = uWinPortable.SelectRelease(winFeed, CheckOptions{})
		if !ok || rel.Enclosure.URL != "https://example.com/App-2.0.0-windows-amd64-setup.exe" {
			t.Fatalf("Windows manifest auto-detect got %v; want -setup.exe", rel)
		}
	})
}

func TestUpdaterFullLifecycleAndDownloadVerification(t *testing.T) {
	pub, priv, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}

	payload := bytes.Repeat([]byte("ggui-update-binary-chunk-"), 4096) // ~100 KiB
	sig, err := SignBytes(priv, payload)
	if err != nil {
		t.Fatalf("SignBytes error = %v", err)
	}
	sum := sha256.Sum256(payload)
	wantSHA := hex.EncodeToString(sum[:])

	var srvURL string
	mux := http.NewServeMux()
	mux.HandleFunc("/appcast.xml", func(w http.ResponseWriter, r *http.Request) {
		feed := &Appcast{
			Title: "Test Feed",
			Items: []Item{
				{
					Title:        "Release 2.0.0",
					Version:      "2.0.0",
					ShortVersion: "2.0",
					PubDate:      time.Date(2026, time.November, 1, 0, 0, 0, 0, time.UTC),
					Enclosures: []Enclosure{
						{
							URL:         srvURL + "/downloads/App-2.0.0.zip",
							Length:      int64(len(payload)),
							Type:        "application/zip",
							EdSignature: sig,
							SHA256:      wantSHA,
							OS:          "macos",
							Arch:        "arm64",
						},
					},
				},
			},
		}
		xmlBytes, err := MarshalAppcast(feed)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write(xmlBytes)
	})
	mux.HandleFunc("/downloads/App-2.0.0.zip", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(payload)
	})
	mux.HandleFunc("/downloads/tampered.zip", func(w http.ResponseWriter, r *http.Request) {
		bad := append([]byte(nil), payload...)
		bad[len(bad)/2] ^= 0x01
		_, _ = w.Write(bad)
	})
	mux.HandleFunc("/downloads/huge.zip", func(w http.ResponseWriter, r *http.Request) {
		// Omit Content-Length by flushing chunked body to exercise streaming LimitReader.
		flusher, _ := w.(http.Flusher)
		chunk := bytes.Repeat([]byte("x"), 8192)
		for range 8 {
			_, _ = w.Write(chunk)
			if flusher != nil {
				flusher.Flush()
			}
		}
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()
	srvURL = srv.URL

	stateDir := t.TempDir()
	cacheDir := t.TempDir()
	applier := &mockApplier{}
	fixedNow := time.Date(2026, time.November, 20, 10, 0, 0, 0, time.UTC)

	u, err := New(Config{
		FeedURL:           srv.URL + "/appcast.xml",
		PublicKey:         pub,
		CurrentVersion:    "1.0.0",
		OS:                "macos",
		Arch:              "arm64",
		OSVersion:         "14.5",
		StateDir:          stateDir,
		CacheDir:          cacheDir,
		AllowInsecureHTTP: true,
		Applier:           applier,
		Now:               func() time.Time { return fixedNow },
	})
	if err != nil {
		t.Fatalf("New error = %v", err)
	}

	// 1. Check fetches feed, finds 2.0.0, and updates LastCheckTime.
	rel, ok, err := u.Check(context.Background())
	if err != nil {
		t.Fatalf("Check error = %v", err)
	}
	if !ok || rel.CompareVersion() != "2.0.0" || rel.DisplayVersion() != "2.0" {
		t.Fatalf("Check got (%+v, %v); want version 2.0.0 / display 2.0", rel, ok)
	}
	if !u.LastCheckTime().Equal(fixedNow) {
		t.Errorf("LastCheckTime = %v; want %v", u.LastCheckTime(), fixedNow)
	}

	// 2. Download streams, reports progress, verifies SHA256 + Ed25519, atomically renames.
	var progressEvents []DownloadProgress
	upd, err := u.Download(context.Background(), rel, func(p DownloadProgress) {
		progressEvents = append(progressEvents, p)
	})
	if err != nil {
		t.Fatalf("Download error = %v", err)
	}
	if !upd.Verified || upd.Size != int64(len(payload)) || upd.SHA256 != wantSHA {
		t.Fatalf("unexpected DownloadedUpdate: %+v", upd)
	}
	if len(progressEvents) == 0 {
		t.Errorf("expected download progress callbacks")
	} else {
		last := progressEvents[len(progressEvents)-1]
		if last.Downloaded != int64(len(payload)) || last.Fraction != 1.0 {
			t.Errorf("final progress = %+v; want downloaded=%d fraction=1.0", last, len(payload))
		}
	}
	if _, err := os.Stat(upd.FilePath + ".part"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected .part temp file to be removed/renamed, stat err = %v", err)
	}
	if leftover, _ := filepath.Glob(filepath.Join(cacheDir, "*.part")); len(leftover) != 0 {
		t.Errorf("expected no leftover .part files, got %v", leftover)
	}
	gotBytes, err := os.ReadFile(upd.FilePath)
	if err != nil || !bytes.Equal(gotBytes, payload) {
		t.Fatalf("downloaded file mismatch: err=%v len=%d", err, len(gotBytes))
	}

	// 3. Apply delegates to configured Applier after re-verifying file on disk.
	if err := u.Apply(context.Background(), upd, ApplyOptions{Relaunch: true, Silent: true}); err != nil {
		t.Fatalf("Apply error = %v", err)
	}
	if applier.calls != 1 || !applier.lastOpt.Relaunch || !applier.lastOpt.Silent {
		t.Errorf("applier received calls=%d opts=%+v", applier.calls, applier.lastOpt)
	}

	// 4. Apply refuses unverified update when AllowUnsigned is false.
	unverified := *upd
	unverified.Verified = false
	if err := u.Apply(context.Background(), &unverified, ApplyOptions{}); !errors.Is(err, ErrInvalidSignature) {
		t.Errorf("Apply(unverified) = %v; want ErrInvalidSignature", err)
	}

	// 4b. Cache-to-Apply TOCTOU: tampering the cached file on disk after Download
	// must be rejected by Apply before invoking Applier.
	tamperedOnDisk := append([]byte(nil), payload...)
	tamperedOnDisk[0] ^= 0xFF
	if err := os.WriteFile(upd.FilePath, tamperedOnDisk, 0o600); err != nil {
		t.Fatalf("tampering cached file: %v", err)
	}
	if err := u.Apply(context.Background(), upd, ApplyOptions{}); !errors.Is(err, ErrInvalidSignature) {
		t.Errorf("Apply(tampered on disk) = %v; want ErrInvalidSignature", err)
	}
	if applier.calls != 1 {
		t.Errorf("applier must not be called for tampered file on disk; calls=%d", applier.calls)
	}
	// Restore valid file and verify SHA-256 mismatch is also caught by Apply.
	if err := os.WriteFile(upd.FilePath, payload, 0o600); err != nil {
		t.Fatalf("restoring cached file: %v", err)
	}
	badSHAUpd := *upd
	badSHAUpd.Release.Enclosure.SHA256 = "0000000000000000000000000000000000000000000000000000000000000000"
	if err := u.Apply(context.Background(), &badSHAUpd, ApplyOptions{}); !errors.Is(err, ErrHashMismatch) {
		t.Errorf("Apply(SHA256 mismatch) = %v; want ErrHashMismatch", err)
	}
	if applier.calls != 1 {
		t.Errorf("applier must not be called on SHA256 mismatch; calls=%d", applier.calls)
	}

	// 5. Tampered bytes with matching SHA256 omitted -> ErrInvalidSignature & temp file deleted.
	tamperedRel := *rel
	tamperedRel.Enclosure.URL = srv.URL + "/downloads/tampered.zip"
	tamperedRel.Enclosure.SHA256 = ""
	if _, err := u.Download(context.Background(), &tamperedRel, nil); !errors.Is(err, ErrInvalidSignature) {
		t.Errorf("Download(tampered) = %v; want ErrInvalidSignature", err)
	}
	if leftover, _ := filepath.Glob(filepath.Join(cacheDir, "*.part")); len(leftover) != 0 {
		t.Errorf("tampered .part files should be deleted, got %v", leftover)
	}

	// 6. SHA-256 mismatch -> ErrHashMismatch & temp file deleted.
	badSHARel := *rel
	badSHARel.Enclosure.SHA256 = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	if _, err := u.Download(context.Background(), &badSHARel, nil); !errors.Is(err, ErrHashMismatch) {
		t.Errorf("Download(bad sha256) = %v; want ErrHashMismatch", err)
	}

	// 7. Missing EdSignature when AllowUnsigned is false -> ErrMissingSignature.
	unsignedRel := *rel
	unsignedRel.Enclosure.EdSignature = ""
	if _, err := u.Download(context.Background(), &unsignedRel, nil); !errors.Is(err, ErrMissingSignature) {
		t.Errorf("Download(unsigned) = %v; want ErrMissingSignature", err)
	}

	// 8. MaxDownloadBytes enforcement on streaming response -> ErrDownloadTooLarge.
	uSmall, err := New(Config{
		FeedURL:           srv.URL + "/appcast.xml",
		PublicKey:         pub,
		CurrentVersion:    "1.0.0",
		OS:                "macos",
		Arch:              "arm64",
		MaxDownloadBytes:  16 * 1024,
		StateDir:          t.TempDir(),
		CacheDir:          t.TempDir(),
		AllowInsecureHTTP: true,
	})
	if err != nil {
		t.Fatalf("New(small limit) error = %v", err)
	}
	hugeRel := *rel
	hugeRel.Enclosure.URL = srv.URL + "/downloads/huge.zip"
	hugeRel.Enclosure.Length = 0
	if _, err := uSmall.Download(context.Background(), &hugeRel, nil); !errors.Is(err, ErrDownloadTooLarge) {
		t.Errorf("Download(too large) = %v; want ErrDownloadTooLarge", err)
	}
}

func TestHTTPSRedirectDowngradeRejected(t *testing.T) {
	pub, priv, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}
	payload := []byte("secret-payload")
	sig, err := SignBytes(priv, payload)
	if err != nil {
		t.Fatalf("SignBytes error = %v", err)
	}

	plainHTTP := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer plainHTTP.Close()

	tlsSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, plainHTTP.URL+"/payload.zip", http.StatusFound)
	}))
	defer tlsSrv.Close()

	u, err := New(Config{
		FeedURL:        tlsSrv.URL + "/appcast.xml",
		PublicKey:      pub,
		CurrentVersion: "1.0.0",
		OS:             "macos",
		Arch:           "arm64",
		HTTPClient:     tlsSrv.Client(),
		StateDir:       t.TempDir(),
		CacheDir:       t.TempDir(),
	})
	if err != nil {
		t.Fatalf("New(TLS) error = %v", err)
	}

	// Feed fetch redirecting from HTTPS -> HTTP must be rejected.
	if _, _, err := u.Check(context.Background()); !errors.Is(err, ErrInsecureURL) {
		t.Errorf("Check(HTTPS->HTTP redirect) = %v; want ErrInsecureURL", err)
	}

	// Download redirecting from HTTPS -> HTTP must be rejected.
	rel := &Release{
		Item: Item{Version: "2.0.0"},
		Enclosure: Enclosure{
			URL:         tlsSrv.URL + "/download.zip",
			EdSignature: sig,
			OS:          "macos",
			Arch:        "arm64",
		},
	}
	if _, err := u.Download(context.Background(), rel, nil); !errors.Is(err, ErrInsecureURL) {
		t.Errorf("Download(HTTPS->HTTP redirect) = %v; want ErrInsecureURL", err)
	}
}

func TestApplyDefaultApplierHook(t *testing.T) {
	pub, priv, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}

	prev := defaultApplierFunc
	t.Cleanup(func() { defaultApplierFunc = prev })

	defaultApplierFunc = nil
	u, err := New(Config{
		FeedURL:        "https://example.com/appcast.xml",
		PublicKey:      pub,
		CurrentVersion: "1.0.0",
		StateDir:       t.TempDir(),
		CacheDir:       t.TempDir(),
	})
	if err != nil {
		t.Fatalf("New error = %v", err)
	}

	payload := []byte("signed-hook-payload")
	sig, err := SignBytes(priv, payload)
	if err != nil {
		t.Fatalf("SignBytes error = %v", err)
	}
	filePath := filepath.Join(t.TempDir(), "update.zip")
	if err := os.WriteFile(filePath, payload, 0o600); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	upd := &DownloadedUpdate{
		Release: Release{
			Enclosure: Enclosure{
				EdSignature: sig,
			},
		},
		FilePath: filePath,
		Verified: true,
	}
	err = u.Apply(context.Background(), upd, ApplyOptions{})
	if !errors.Is(err, ErrUnsupportedPlatform) || !errors.Is(err, errors.ErrUnsupported) {
		t.Errorf("Apply(nil defaultApplierFunc) = %v; want ErrUnsupportedPlatform / errors.ErrUnsupported", err)
	}

	mock := &mockApplier{}
	defaultApplierFunc = func() Applier { return mock }
	if err := u.Apply(context.Background(), upd, ApplyOptions{Relaunch: true}); err != nil {
		t.Fatalf("Apply(registered defaultApplierFunc) error = %v", err)
	}
	if mock.calls != 1 {
		t.Errorf("defaultApplierFunc calls = %d; want 1", mock.calls)
	}
}

func TestStatePersistenceAndBackgroundLoop(t *testing.T) {
	pub, _, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}

	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		feed := &Appcast{
			Title: "BG Feed",
			Items: []Item{
				{
					Version: "2.0.0",
					Enclosures: []Enclosure{
						{URL: "https://example.com/app-2.0.0.zip", OS: "macos", Arch: "arm64"},
					},
				},
			},
		}
		b, _ := MarshalAppcast(feed)
		_, _ = w.Write(b)
	}))
	defer srv.Close()

	stateDir := t.TempDir()
	cacheDir := t.TempDir()

	u1, err := New(Config{
		FeedURL:           srv.URL + "/appcast.xml",
		PublicKey:         pub,
		CurrentVersion:    "1.0.0",
		OS:                "macos",
		Arch:              "arm64",
		CheckInterval:     30 * time.Millisecond,
		StateDir:          stateDir,
		CacheDir:          cacheDir,
		AllowInsecureHTTP: true,
	})
	if err != nil {
		t.Fatalf("New(u1) error = %v", err)
	}

	// Default AutoCheck is true, ShouldAutoCheck is true before first check.
	if !u1.AutoCheckEnabled() || !u1.ShouldAutoCheck() {
		t.Fatalf("expected AutoCheckEnabled=true and ShouldAutoCheck=true initially")
	}
	seed1 := u1.state.rolloutSeed()
	if err := u1.SkipVersion("1.5.0"); err != nil {
		t.Fatalf("SkipVersion error = %v", err)
	}
	if err := u1.SetAutoCheckEnabled(false); err != nil {
		t.Fatalf("SetAutoCheckEnabled(false) error = %v", err)
	}
	if u1.AutoCheckEnabled() || u1.ShouldAutoCheck() {
		t.Fatalf("expected AutoCheckEnabled=false and ShouldAutoCheck=false")
	}

	// Reload from the same StateDir and confirm persistence of skipped version,
	// auto-check preference, and deterministic rollout seed.
	u2, err := New(Config{
		FeedURL:           srv.URL + "/appcast.xml",
		PublicKey:         pub,
		CurrentVersion:    "1.0.0",
		OS:                "macos",
		Arch:              "arm64",
		CheckInterval:     30 * time.Millisecond,
		StateDir:          stateDir,
		CacheDir:          cacheDir,
		AllowInsecureHTTP: true,
	})
	if err != nil {
		t.Fatalf("New(u2) error = %v", err)
	}
	if u2.SkippedVersion() != "1.5.0" {
		t.Errorf("reloaded SkippedVersion = %q; want 1.5.0", u2.SkippedVersion())
	}
	if u2.AutoCheckEnabled() {
		t.Errorf("reloaded AutoCheckEnabled = true; want false")
	}
	if u2.state.rolloutSeed() != seed1 {
		t.Errorf("reloaded rolloutSeed = %v; want %v", u2.state.rolloutSeed(), seed1)
	}

	// Re-enable auto-check and verify StartBackground invokes onUpdate and stops cleanly.
	if err := u2.SetAutoCheckEnabled(true); err != nil {
		t.Fatalf("SetAutoCheckEnabled(true) error = %v", err)
	}

	gotUpdate := make(chan string, 4)
	stop := u2.StartBackground(context.Background(), func(rel *Release) {
		select {
		case gotUpdate <- rel.CompareVersion():
		default:
		}
	})

	select {
	case ver := <-gotUpdate:
		if ver != "2.0.0" {
			t.Errorf("background update version = %q; want 2.0.0", ver)
		}
	case <-time.After(2 * time.Second):
		stop()
		t.Fatalf("timed out waiting for background update callback")
	}

	// Stopping twice must be safe and stop further checks.
	stop()
	stop()
	afterStop := hits.Load()
	time.Sleep(80 * time.Millisecond)
	if hits.Load() != afterStop {
		t.Errorf("background checks continued after stop(): before=%d after=%d", afterStop, hits.Load())
	}
}

func TestCurrentOSVersion(t *testing.T) {
	ver := currentOSVersion()
	if runtime.GOOS == "darwin" && ver == "" {
		t.Errorf("currentOSVersion() on darwin = %q; want non-empty macOS version", ver)
	}
}

func TestDownloadUnpredictableTempPartAndSanitizeReservedNames(t *testing.T) {
	// Reserved Windows device names must be prefixed with '_'.
	reserved := map[string]string{
		"CON":       "_CON",
		"prn.txt":   "_prn.txt",
		"AUX":       "_AUX",
		"NUL":       "_NUL",
		"NUL.exe":   "_NUL.exe",
		"com1":      "_com1",
		"COM9.zip":  "_COM9.zip",
		"LPT1.bin":  "_LPT1.bin",
		"lpt9":      "_lpt9",
		"NUL .exe":  "_NUL .exe",
		"COM0":      "COM0",
		"COM10":     "COM10",
		"console":   "console",
		"app-1.zip": "app-1.zip",
		"":          "",
	}
	for in, want := range reserved {
		if got := sanitizePathComponent(in); got != want {
			t.Errorf("sanitizePathComponent(%q) = %q; want %q", in, got, want)
		}
	}

	// Pre-placed symlink at predictable <finalPath>.part must never be followed.
	if runtime.GOOS == "windows" {
		return
	}
	pub, priv, err := GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("safe-download-body")
	sig, err := SignBytes(priv, payload)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	victim := filepath.Join(t.TempDir(), "victim.txt")
	if err := os.WriteFile(victim, []byte("untouched"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, filepath.Join(cacheDir, "app.zip.part")); err != nil {
		t.Fatal(err)
	}

	u, err := New(Config{
		FeedURL:           srv.URL + "/appcast.xml",
		PublicKey:         pub,
		CurrentVersion:    "1.0.0",
		StateDir:          t.TempDir(),
		CacheDir:          cacheDir,
		AllowInsecureHTTP: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	rel := &Release{
		Item: Item{Version: "2.0.0"},
		Enclosure: Enclosure{
			URL:         srv.URL + "/app.zip",
			EdSignature: sig,
		},
	}
	if _, err := u.Download(context.Background(), rel, nil); err != nil {
		t.Fatalf("Download error = %v", err)
	}
	gotVictim, err := os.ReadFile(victim)
	if err != nil || string(gotVictim) != "untouched" {
		t.Errorf("victim file modified through predictable .part symlink: (%q, %v)", string(gotVictim), err)
	}
}

func TestUpdaterRedactsURLsInErrors(t *testing.T) {
	pub, priv, err := GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	sig, err := SignBytes(priv, []byte("x"))
	if err != nil {
		t.Fatal(err)
	}

	// 1. Insecure URL error redacts userinfo, query, and fragment.
	_, err = New(Config{
		FeedURL:   "http://alice:supersecret@example.com/appcast.xml?token=leakme#fragsecret",
		PublicKey: pub,
		StateDir:  t.TempDir(),
		CacheDir:  t.TempDir(),
	})
	if !errors.Is(err, ErrInsecureURL) {
		t.Fatalf("New(insecure) = %v; want ErrInsecureURL", err)
	}
	for _, secret := range []string{"alice", "supersecret", "leakme", "fragsecret"} {
		if strings.Contains(err.Error(), secret) {
			t.Errorf("New error leaked %q: %v", secret, err)
		}
	}

	// 2. HTTP status errors in Check and Download redact credentials/query/fragment.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	feedURLWithSecrets := strings.Replace(srv.URL, "http://", "http://user:pw123@", 1) + "/appcast.xml?api_key=k999#hash999"
	u, err := New(Config{
		FeedURL:           feedURLWithSecrets,
		PublicKey:         pub,
		CurrentVersion:    "1.0.0",
		StateDir:          t.TempDir(),
		CacheDir:          t.TempDir(),
		AllowInsecureHTTP: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := u.Check(context.Background()); err == nil {
		t.Fatalf("Check(HTTP 500) = nil; want error")
	} else {
		for _, secret := range []string{"user", "pw123", "k999", "hash999"} {
			if strings.Contains(err.Error(), secret) {
				t.Errorf("Check error leaked %q: %v", secret, err)
			}
		}
	}

	dlURLWithSecrets := strings.Replace(srv.URL, "http://", "http://dluser:dlpw@", 1) + "/pkg.zip?sig=dlsecret#dlfrag"
	if _, err := u.Download(context.Background(), &Release{
		Item:      Item{Version: "2.0.0"},
		Enclosure: Enclosure{URL: dlURLWithSecrets, EdSignature: sig},
	}, nil); err == nil {
		t.Fatalf("Download(HTTP 500) = nil; want error")
	} else {
		for _, secret := range []string{"dluser", "dlpw", "dlsecret", "dlfrag"} {
			if strings.Contains(err.Error(), secret) {
				t.Errorf("Download error leaked %q: %v", secret, err)
			}
		}
	}
}

func TestUpdaterDirFallbacksAndBackgroundClockJump(t *testing.T) {
	pub, _, err := GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}

	prevCfg, prevCache, prevHome := userConfigDirFunc, userCacheDirFunc, userHomeDirFunc
	t.Cleanup(func() {
		userConfigDirFunc = prevCfg
		userCacheDirFunc = prevCache
		userHomeDirFunc = prevHome
	})

	// Home fallback when UserConfigDir / UserCacheDir fail.
	fakeHome := t.TempDir()
	userConfigDirFunc = func() (string, error) { return "", errors.New("no config dir") }
	userCacheDirFunc = func() (string, error) { return "", errors.New("no cache dir") }
	userHomeDirFunc = func() (string, error) { return fakeHome, nil }

	uHome, err := New(Config{
		FeedURL:   "https://example.com/appcast.xml",
		PublicKey: pub,
		AppID:     "com.example.fallback",
	})
	if err != nil {
		t.Fatalf("New(home fallback) error = %v", err)
	}
	wantState := filepath.Join(fakeHome, ".config", "com.example.fallback")
	wantCache := filepath.Join(fakeHome, ".cache", "com.example.fallback", "updates")
	if uHome.Config().StateDir != wantState || uHome.Config().CacheDir != wantCache {
		t.Errorf("home fallback dirs = (%q, %q); want (%q, %q)", uHome.Config().StateDir, uHome.Config().CacheDir, wantState, wantCache)
	}
	if runtime.GOOS != "windows" {
		if fi, err := os.Stat(wantState); err != nil || fi.Mode().Perm() != 0o700 {
			t.Errorf("home fallback StateDir perm = (%v, %v); want 0700", fi, err)
		}
		if fi, err := os.Stat(wantCache); err != nil || fi.Mode().Perm() != 0o700 {
			t.Errorf("home fallback CacheDir perm = (%v, %v); want 0700", fi, err)
		}
	}

	// Backward system clock jump clamping in StartBackground initial delay.
	now := time.Date(2026, time.November, 20, 12, 0, 0, 0, time.UTC)
	uClock, err := New(Config{
		FeedURL:       "https://example.com/appcast.xml",
		PublicKey:     pub,
		CheckInterval: 2 * time.Hour,
		StateDir:      t.TempDir(),
		CacheDir:      t.TempDir(),
		Now:           func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	// LastCheck in the future (1 year ahead due to backward clock jump) must clamp to CheckInterval.
	if err := uClock.state.setLastCheck(now.Add(365 * 24 * time.Hour)); err != nil {
		t.Fatal(err)
	}
	if got := uClock.initialBackgroundDelay(); got != 2*time.Hour {
		t.Errorf("initialBackgroundDelay(future lastCheck) = %v; want clamped 2h", got)
	}
	// Half-elapsed interval returns remaining time.
	if err := uClock.state.setLastCheck(now.Add(-30 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	if got := uClock.initialBackgroundDelay(); got != 90*time.Minute {
		t.Errorf("initialBackgroundDelay(half elapsed) = %v; want 90m", got)
	}
}
