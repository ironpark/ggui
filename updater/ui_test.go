package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/widget"
)

type uiTestApplier struct {
	mu      sync.Mutex
	calls   int
	lastUpd *DownloadedUpdate
	lastOpt ApplyOptions
	err     error
	onApply func(upd *DownloadedUpdate, opts ApplyOptions)
}

func (a *uiTestApplier) Apply(_ context.Context, upd *DownloadedUpdate, opts ApplyOptions) error {
	a.mu.Lock()
	a.calls++
	a.lastUpd = upd
	a.lastOpt = opts
	err := a.err
	fn := a.onApply
	a.mu.Unlock()
	if fn != nil {
		fn(upd, opts)
	}
	if err != nil {
		return err
	}
	if opts.QuitFunc != nil {
		opts.QuitFunc()
	}
	return nil
}

func (a *uiTestApplier) snapshot() (int, *DownloadedUpdate, ApplyOptions) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.calls, a.lastUpd, a.lastOpt
}

type testFeedServer struct {
	server *httptest.Server
	pubKey string

	mu        sync.Mutex
	feed      *Appcast
	payload   []byte
	feedHits  atomic.Int32
	dlDelay   time.Duration
	dlStarted chan struct{}
	dlRelease chan struct{}
}

func newTestFeedServer(t *testing.T, buildFeed func(baseURL, sig, sha string, size int64) *Appcast) *testFeedServer {
	t.Helper()
	pub, priv, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	payload := []byte(strings.Repeat("ggui-sparkle-signed-update-payload-", 256))
	sig, err := SignBytes(priv, payload)
	if err != nil {
		t.Fatalf("SignBytes: %v", err)
	}
	sum := sha256.Sum256(payload)
	shaHex := hex.EncodeToString(sum[:])

	s := &testFeedServer{
		pubKey:  pub,
		payload: payload,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/appcast.xml", func(w http.ResponseWriter, r *http.Request) {
		s.feedHits.Add(1)
		s.mu.Lock()
		feed := s.feed
		s.mu.Unlock()
		xmlBytes, err := MarshalAppcast(feed)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
		_, _ = w.Write(xmlBytes)
	})
	mux.HandleFunc("/update.zip", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		data := append([]byte(nil), s.payload...)
		delay := s.dlDelay
		started := s.dlStarted
		release := s.dlRelease
		s.mu.Unlock()

		if started != nil {
			select {
			case <-started:
			default:
				close(started)
			}
		}
		if release != nil {
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		}
		if delay > 0 {
			select {
			case <-time.After(delay):
			case <-r.Context().Done():
				return
			}
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(data)
	})

	s.server = httptest.NewServer(mux)
	t.Cleanup(s.server.Close)
	s.feed = buildFeed(s.server.URL, sig, shaHex, int64(len(payload)))
	return s
}

func waitFor(t *testing.T, probe *ggui.Probe, timeout time.Duration, desc string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		probe.Advance(30 * time.Millisecond)
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", desc)
}

func tapButton(t *testing.T, probe *ggui.Probe, label string) {
	t.Helper()
	// Advance once so any dialog PopIn transition settles before locating the button.
	probe.Advance(300 * time.Millisecond)
	f, ok := probe.FindRole(ggui.RoleButton, label)
	if !ok {
		t.Fatalf("button %q not found; semantics:\n%s", label, probe.Semantics())
	}
	probe.Click(f.Center())
	probe.Frame()
}

func TestUpdateDialogFullFlow(t *testing.T) {
	srv := newTestFeedServer(t, func(baseURL, sig, sha string, size int64) *Appcast {
		return &Appcast{
			Title: "Demo App Updates",
			Items: []Item{{
				Title:        "Version 2.0.0",
				Version:      "2.0.0",
				ShortVersion: "2.0.0",
				ReleaseNotes: "## What's New\n\n- **Faster startup** across all platforms\n- Fixed crash on sleep resume",
				Enclosures: []Enclosure{{
					URL:         baseURL + "/update.zip",
					Length:      size,
					Type:        "application/octet-stream",
					EdSignature: sig,
					SHA256:      sha,
					OS:          "macos",
					Arch:        "arm64",
				}},
			}},
		}
	})

	applier := &uiTestApplier{}
	dir := t.TempDir()
	u, err := New(Config{
		FeedURL:           srv.server.URL + "/appcast.xml",
		PublicKey:         srv.pubKey,
		CurrentVersion:    "1.0.0",
		AppID:             "com.example.demo",
		AppName:           "Demo App",
		OS:                "macos",
		Arch:              "arm64",
		StateDir:          dir,
		CacheDir:          filepath.Join(dir, "cache"),
		Applier:           applier,
		AllowInsecureHTTP: true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	var quitCalled atomic.Bool
	quitUIState := ggui.State(false)
	ctrl := NewController(u, ControllerConfig{
		Quit: func() {
			// Touching a ggui.StateValue here proves Quit runs on the UI
			// goroutine under -race -tags ggui_debug.
			quitUIState.Set(true)
			quitCalled.Store(true)
		},
	})

	var probe *ggui.Probe
	probe = ggui.ProbeBuilder(func() ggui.Widget {
		ctrl.SetPost(probe.Post)
		return widget.Box(Dialog(ctrl))
	}, ggui.Sz(700, 540))
	defer probe.Close()

	probe.Frame()
	if ggui.Peek(ctrl.Open) {
		t.Fatalf("dialog should start closed")
	}

	// 1. CheckNow -> PhaseAvailable with rendered Markdown release notes.
	ctrl.CheckNow()
	waitFor(t, probe, 3*time.Second, "PhaseAvailable", func() bool {
		return ggui.Peek(ctrl.Phase) == PhaseAvailable
	})
	probe.Advance(300 * time.Millisecond)

	sem := probe.Semantics().String()
	for _, want := range []string{
		"A new version of Demo App is available!",
		"Demo App 2.0.0 is now available",
		"What's New",
		"Faster startup",
	} {
		if !strings.Contains(sem, want) {
			t.Errorf("PhaseAvailable semantics missing %q:\n%s", want, sem)
		}
	}

	// Toggle auto-check checkbox and confirm persistence.
	cb, ok := probe.FindRole(ggui.RoleCheckbox, "Automatically check for updates")
	if !ok {
		t.Fatalf("auto-check checkbox not found:\n%s", sem)
	}
	probe.Click(cb.Center())
	probe.Frame()
	if u.AutoCheckEnabled() {
		t.Errorf("AutoCheckEnabled = true after unchecking; want false")
	}

	// 2. Click "Install Update" -> downloads, verifies, reaches PhaseReadyToInstall.
	tapButton(t, probe, "Install Update")
	waitFor(t, probe, 3*time.Second, "PhaseReadyToInstall", func() bool {
		return ggui.Peek(ctrl.Phase) == PhaseReadyToInstall
	})
	probe.Advance(300 * time.Millisecond)

	upd := ggui.Peek(ctrl.Downloaded)
	if upd == nil || !upd.Verified {
		t.Fatalf("Downloaded = %+v; want verified update", upd)
	}

	// 3. Click "Install and Relaunch" -> invokes Applier & Quit callback on UI goroutine.
	tapButton(t, probe, "Install and Relaunch")
	waitFor(t, probe, 3*time.Second, "applier and quit invoked", func() bool {
		calls, _, _ := applier.snapshot()
		return calls == 1 && quitCalled.Load()
	})

	calls, gotUpd, gotOpt := applier.snapshot()
	if calls != 1 {
		t.Fatalf("applier calls = %d; want 1", calls)
	}
	if gotUpd == nil || !gotUpd.Verified || gotUpd.FilePath != upd.FilePath {
		t.Errorf("applier got update %+v; want verified %s", gotUpd, upd.FilePath)
	}
	if !gotOpt.Relaunch || !gotOpt.Silent {
		t.Errorf("ApplyOptions = %+v; want Relaunch=true, Silent=true", gotOpt)
	}
	if !quitCalled.Load() || !ggui.Peek(quitUIState) {
		t.Errorf("Quit callback was not invoked on UI goroutine")
	}
}

func TestUpdateDialogSkipAndSilentCheck(t *testing.T) {
	srv := newTestFeedServer(t, func(baseURL, sig, sha string, size int64) *Appcast {
		return &Appcast{
			Items: []Item{{
				Version:      "1.5.0",
				ShortVersion: "1.5.0",
				ReleaseNotes: "Minor polish and fixes.",
				Enclosures: []Enclosure{{
					URL:         baseURL + "/update.zip",
					Length:      size,
					EdSignature: sig,
					OS:          "macos",
					Arch:        "arm64",
				}},
			}},
		}
	})

	dir := t.TempDir()
	u, err := New(Config{
		FeedURL:           srv.server.URL + "/appcast.xml",
		PublicKey:         srv.pubKey,
		CurrentVersion:    "1.0.0",
		AppID:             "com.example.skip",
		AppName:           "SkipApp",
		OS:                "macos",
		Arch:              "arm64",
		StateDir:          dir,
		CacheDir:          filepath.Join(dir, "cache"),
		AllowInsecureHTTP: true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctrl := NewController(u)
	var probe *ggui.Probe
	probe = ggui.ProbeBuilder(func() ggui.Widget {
		ctrl.SetPost(probe.Post)
		return widget.Box(Dialog(ctrl))
	}, ggui.Sz(700, 540))
	defer probe.Close()

	// Manual check finds 1.5.0 -> click "Skip This Version".
	ctrl.CheckNow()
	waitFor(t, probe, 3*time.Second, "PhaseAvailable", func() bool {
		return ggui.Peek(ctrl.Phase) == PhaseAvailable
	})
	tapButton(t, probe, "Skip This Version")

	if got := u.SkippedVersion(); got != "1.5.0" {
		t.Fatalf("SkippedVersion = %q; want 1.5.0", got)
	}
	if ggui.Peek(ctrl.Open) {
		t.Fatalf("dialog should close after Skip")
	}

	// CheckSilent must hit the feed and NOT open the dialog for skipped version.
	beforeHits := srv.feedHits.Load()
	ctrl.CheckSilent()
	waitFor(t, probe, 3*time.Second, "silent check feed hit", func() bool {
		return srv.feedHits.Load() > beforeHits
	})
	// Pump a few more frames to verify no delayed open occurs.
	for range 5 {
		probe.Advance(30 * time.Millisecond)
		time.Sleep(5 * time.Millisecond)
	}
	if ggui.Peek(ctrl.Open) {
		t.Fatalf("CheckSilent opened dialog for skipped version 1.5.0")
	}

	// CheckNow (manual) MUST bypass skip and open the dialog.
	ctrl.CheckNow()
	waitFor(t, probe, 3*time.Second, "manual check opens skipped version", func() bool {
		return ggui.Peek(ctrl.Open) && ggui.Peek(ctrl.Phase) == PhaseAvailable
	})
	if rel := ggui.Peek(ctrl.Release); rel == nil || rel.CompareVersion() != "1.5.0" {
		t.Fatalf("manual check Release = %+v; want 1.5.0", rel)
	}
}

func TestUpdateDialogUpToDate(t *testing.T) {
	srv := newTestFeedServer(t, func(baseURL, sig, sha string, size int64) *Appcast {
		return &Appcast{
			Items: []Item{{
				Version:      "2.0.0",
				ShortVersion: "2.0.0",
				Enclosures: []Enclosure{{
					URL:         baseURL + "/update.zip",
					Length:      size,
					EdSignature: sig,
					OS:          "macos",
					Arch:        "arm64",
				}},
			}},
		}
	})

	dir := t.TempDir()
	u, err := New(Config{
		FeedURL:           srv.server.URL + "/appcast.xml",
		PublicKey:         srv.pubKey,
		CurrentVersion:    "2.0.0",
		AppID:             "com.example.uptodate",
		AppName:           "FreshApp",
		OS:                "macos",
		Arch:              "arm64",
		StateDir:          dir,
		CacheDir:          filepath.Join(dir, "cache"),
		AllowInsecureHTTP: true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctrl := NewController(u)
	var probe *ggui.Probe
	probe = ggui.ProbeBuilder(func() ggui.Widget {
		ctrl.SetPost(probe.Post)
		return widget.Box(Dialog(ctrl))
	}, ggui.Sz(700, 540))
	defer probe.Close()

	ctrl.CheckNow()
	waitFor(t, probe, 3*time.Second, "PhaseUpToDate", func() bool {
		return ggui.Peek(ctrl.Phase) == PhaseUpToDate
	})
	probe.Advance(300 * time.Millisecond)

	sem := probe.Semantics().String()
	if !strings.Contains(sem, "You're up to date!") || !strings.Contains(sem, "FreshApp 2.0.0 is currently the newest version available.") {
		t.Errorf("unexpected UpToDate semantics:\n%s", sem)
	}

	tapButton(t, probe, "OK")
	if ggui.Peek(ctrl.Open) {
		t.Errorf("OK did not close UpToDate dialog")
	}
}

func TestUpdateDialogTamperedSignatureAndRetry(t *testing.T) {
	srv := newTestFeedServer(t, func(baseURL, sig, sha string, size int64) *Appcast {
		return &Appcast{
			Items: []Item{{
				Version:      "3.0.0",
				ShortVersion: "3.0.0",
				ReleaseNotes: "Security release.",
				Enclosures: []Enclosure{{
					URL:         baseURL + "/update.zip",
					Length:      size,
					EdSignature: sig,
					OS:          "macos",
					Arch:        "arm64",
				}},
			}},
		}
	})

	// Tamper the served payload so Ed25519 signature verification fails.
	srv.mu.Lock()
	validPayload := append([]byte(nil), srv.payload...)
	tampered := append([]byte(nil), srv.payload...)
	tampered[0] ^= 0xFF
	srv.payload = tampered
	srv.mu.Unlock()

	dir := t.TempDir()
	u, err := New(Config{
		FeedURL:           srv.server.URL + "/appcast.xml",
		PublicKey:         srv.pubKey,
		CurrentVersion:    "2.0.0",
		AppID:             "com.example.tamper",
		AppName:           "SecureApp",
		OS:                "macos",
		Arch:              "arm64",
		StateDir:          dir,
		CacheDir:          filepath.Join(dir, "cache"),
		AllowInsecureHTTP: true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctrl := NewController(u)
	var probe *ggui.Probe
	probe = ggui.ProbeBuilder(func() ggui.Widget {
		ctrl.SetPost(probe.Post)
		return widget.Box(Dialog(ctrl))
	}, ggui.Sz(700, 540))
	defer probe.Close()

	ctrl.CheckNow()
	waitFor(t, probe, 3*time.Second, "PhaseAvailable", func() bool {
		return ggui.Peek(ctrl.Phase) == PhaseAvailable
	})

	// Attempt download of tampered payload -> PhaseError showing "Update Failed".
	tapButton(t, probe, "Install Update")
	waitFor(t, probe, 3*time.Second, "PhaseError", func() bool {
		return ggui.Peek(ctrl.Phase) == PhaseError
	})
	probe.Advance(300 * time.Millisecond)

	sem := probe.Semantics().String()
	if !strings.Contains(sem, "Update Failed") {
		t.Errorf("error UI missing Update Failed alert:\n%s", sem)
	}
	if !strings.Contains(ggui.Peek(ctrl.ErrText), "ed25519") {
		t.Errorf("ErrText = %q; want ed25519 verification error", ggui.Peek(ctrl.ErrText))
	}

	// Restore valid signed payload and click Retry -> reaches PhaseReadyToInstall!
	srv.mu.Lock()
	srv.payload = validPayload
	srv.mu.Unlock()

	tapButton(t, probe, "Retry")
	waitFor(t, probe, 3*time.Second, "PhaseReadyToInstall after retry", func() bool {
		return ggui.Peek(ctrl.Phase) == PhaseReadyToInstall
	})
	if upd := ggui.Peek(ctrl.Downloaded); upd == nil || !upd.Verified {
		t.Fatalf("Downloaded after retry = %+v; want verified", upd)
	}
}

func TestUpdateUICriticalAndCancelDownload(t *testing.T) {
	srv := newTestFeedServer(t, func(baseURL, sig, sha string, size int64) *Appcast {
		return &Appcast{
			Items: []Item{{
				Version:        "2.0.1",
				ShortVersion:   "2.0.1",
				CriticalUpdate: true,
				ReleaseNotes:   "Critical security hotfix.",
				Enclosures: []Enclosure{{
					URL:         baseURL + "/update.zip",
					Length:      size,
					EdSignature: sig,
					OS:          "macos",
					Arch:        "arm64",
				}},
			}},
		}
	})

	started := make(chan struct{})
	release := make(chan struct{})
	srv.mu.Lock()
	srv.dlStarted = started
	srv.dlRelease = release
	srv.mu.Unlock()
	defer close(release)

	dir := t.TempDir()
	u, err := New(Config{
		FeedURL:           srv.server.URL + "/appcast.xml",
		PublicKey:         srv.pubKey,
		CurrentVersion:    "2.0.0",
		AppID:             "com.example.critical",
		AppName:           "CritApp",
		OS:                "macos",
		Arch:              "arm64",
		StateDir:          dir,
		CacheDir:          filepath.Join(dir, "cache"),
		AllowInsecureHTTP: true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctrl := NewController(u)
	var probe *ggui.Probe
	probe = ggui.ProbeBuilder(func() ggui.Widget {
		ctrl.SetPost(probe.Post)
		return widget.Box(Dialog(ctrl))
	}, ggui.Sz(700, 540))
	defer probe.Close()

	ctrl.CheckNow()
	waitFor(t, probe, 3*time.Second, "PhaseAvailable", func() bool {
		return ggui.Peek(ctrl.Phase) == PhaseAvailable
	})
	probe.Advance(300 * time.Millisecond)

	// Critical updates display the "Critical" badge and hide "Skip This Version".
	if _, ok := probe.FindRole(ggui.RoleButton, "Skip This Version"); ok {
		t.Errorf("Skip This Version button should be hidden for critical updates")
	}
	if !strings.Contains(probe.Semantics().String(), "Critical") {
		t.Errorf("expected Critical badge in semantics:\n%s", probe.Semantics())
	}

	// Start download, wait for request to reach server, then Cancel.
	tapButton(t, probe, "Install Update")
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatalf("download never started")
	}
	waitFor(t, probe, 3*time.Second, "PhaseDownloading", func() bool {
		return ggui.Peek(ctrl.Phase) == PhaseDownloading
	})
	tapButton(t, probe, "Cancel")
	if got := ggui.Peek(ctrl.Phase); got != PhaseAvailable {
		t.Fatalf("phase after CancelDownload = %v; want PhaseAvailable", got)
	}
}

func TestControllerStartBackgroundAndDownloadAndInstall(t *testing.T) {
	srv := newTestFeedServer(t, func(baseURL, sig, sha string, size int64) *Appcast {
		return &Appcast{
			Items: []Item{{
				Version:      "2.5.0",
				ShortVersion: "2.5.0",
				ReleaseNotes: "Auto-discovered release.",
				Enclosures: []Enclosure{{
					URL:         baseURL + "/update.zip",
					Length:      size,
					EdSignature: sig,
					OS:          "macos",
					Arch:        "arm64",
				}},
			}},
		}
	})

	applier := &uiTestApplier{}
	dir := t.TempDir()
	u, err := New(Config{
		FeedURL:           srv.server.URL + "/appcast.xml",
		PublicKey:         srv.pubKey,
		CurrentVersion:    "2.0.0",
		AppID:             "com.example.bg",
		AppName:           "BgApp",
		OS:                "macos",
		Arch:              "arm64",
		CheckInterval:     time.Hour,
		StateDir:          dir,
		CacheDir:          filepath.Join(dir, "cache"),
		Applier:           applier,
		AllowInsecureHTTP: true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctrl := NewController(u)
	var probe *ggui.Probe
	probe = ggui.ProbeBuilder(func() ggui.Widget {
		ctrl.SetPost(probe.Post)
		return widget.Box(Panel(ctrl))
	}, ggui.Sz(520, 420))
	defer probe.Close()

	stop := ctrl.StartBackground(context.Background())
	defer stop()

	waitFor(t, probe, 3*time.Second, "background update discovered", func() bool {
		return ggui.Peek(ctrl.Open) && ggui.Peek(ctrl.Phase) == PhaseAvailable
	})
	stop()

	// Exercise one-step DownloadAndInstall.
	ctrl.DownloadAndInstall()
	waitFor(t, probe, 3*time.Second, "DownloadAndInstall invokes applier", func() bool {
		calls, _, _ := applier.snapshot()
		return calls == 1
	})
	_, upd, _ := applier.snapshot()
	if upd == nil || !upd.Verified {
		t.Fatalf("DownloadAndInstall applier update = %+v; want verified", upd)
	}
}

func TestUpdateDialogReleaseNotesLinkSecurity(t *testing.T) {
	dir := t.TempDir()
	u, err := New(Config{
		FeedURL:           "https://updates.example.com/appcast.xml",
		CurrentVersion:    "1.0.0",
		AppID:             "com.example.links",
		AppName:           "LinkApp",
		OS:                "macos",
		Arch:              "arm64",
		StateDir:          dir,
		CacheDir:          filepath.Join(dir, "cache"),
		AllowUnsigned:     true,
		AllowInsecureHTTP: false,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	var opened []string
	ctrl := NewController(u, ControllerConfig{
		OnLink: func(raw string) {
			opened = append(opened, raw)
		},
	})

	var probe *ggui.Probe
	probe = ggui.ProbeBuilder(func() ggui.Widget {
		ctrl.SetPost(probe.Post)
		return widget.Box(Dialog(ctrl))
	}, ggui.Sz(700, 600))
	defer probe.Close()

	showRelease := func(notes, link string) {
		ctrl.Release.Set(&Release{
			Item: Item{
				Version:          "2.0.0",
				ShortVersion:     "2.0.0",
				ReleaseNotes:     notes,
				ReleaseNotesLink: link,
			},
			Enclosure: Enclosure{
				URL: "https://updates.example.com/app-2.0.0.zip",
				OS:  "macos",
			},
		})
		ctrl.Phase.Set(PhaseAvailable)
		ctrl.Open.Set(true)
		probe.Advance(300 * time.Millisecond)
	}

	// Unsafe ReleaseNotesLink schemes (javascript:, file://, and http:// when
	// AllowInsecureHTTP is false) must not render the full release notes action.
	for _, badLink := range []string{
		"javascript:alert(1)",
		"file:///etc/passwd",
		"smb://evil.example/share",
		"http://insecure.example.com/notes",
		"ms-msdt:/id PCWDiagnostic",
	} {
		showRelease("Safe notes.", badLink)
		if _, ok := probe.FindRole(ggui.RoleButton, "View full release notes"); ok {
			t.Errorf("unsafe ReleaseNotesLink %q rendered a button", badLink)
		}
	}

	// A ReleaseNotesLink containing Markdown syntax (e.g. ')') must never inject
	// Markdown links, and Markdown release notes links must block unsafe schemes.
	injectionLink := "https://example.com/notes)[Injected Link](javascript:alert(1)"
	notes := strings.Join([]string{
		"[Bad JS](javascript:alert(1))",
		"[Bad File](file:///etc/passwd)",
		"[Bad HTTP](http://insecure.example.com/changelog)",
		"[Bad SMB](smb://evil.example/share)",
		"[Safe HTTPS](https://example.com/changelog)",
		"[Safe Mail](mailto:support@example.com)",
	}, "\n\n")
	showRelease(notes, injectionLink)

	sem := probe.Semantics().String()
	if strings.Contains(sem, "Injected Link") {
		t.Fatalf("ReleaseNotesLink injected Markdown into UI semantics:\n%s", sem)
	}

	tapButton(t, probe, "View full release notes")
	if len(opened) != 1 || !strings.HasPrefix(opened[0], "https://example.com/notes") {
		t.Fatalf("clicking View full release notes opened %v; want parsed https://example.com/notes...", opened)
	}
	opened = nil

	clickLink := func(label string) {
		f, ok := probe.Find(label)
		if !ok {
			t.Fatalf("markdown link %q not found; semantics:\n%s", label, probe.Semantics())
		}
		probe.Click(f.Center())
		probe.Frame()
	}

	for _, blocked := range []string{"Bad JS", "Bad File", "Bad HTTP", "Bad SMB"} {
		clickLink(blocked)
	}
	if len(opened) != 0 {
		t.Fatalf("unsafe Markdown links reached OnLink: %v", opened)
	}

	clickLink("Safe HTTPS")
	clickLink("Safe Mail")
	wantOpened := []string{"https://example.com/changelog", "mailto:support@example.com"}
	if !slices.Equal(opened, wantOpened) {
		t.Fatalf("safe Markdown links opened = %v; want %v", opened, wantOpened)
	}

	// Verify direct scheme policy when OnLink is nil (allowMailto=false).
	if _, ok := safeLinkURL("http://example.com", false, false); ok {
		t.Errorf("http:// should be rejected when allowInsecureHTTP=false")
	}
	if _, ok := safeLinkURL("http://example.com", true, false); !ok {
		t.Errorf("http:// should be allowed when allowInsecureHTTP=true")
	}
	if _, ok := safeLinkURL("https://example.com/ok", false, false); !ok {
		t.Errorf("https:// should be allowed")
	}
	if _, ok := safeLinkURL("mailto:help@example.com", false, false); ok {
		t.Errorf("mailto: should not be opened when allowMailto=false")
	}
	if _, ok := safeLinkURL("javascript:alert(1)", true, true); ok {
		t.Errorf("javascript: must always be rejected")
	}
}

func TestUpdateDialogEscapeBlockedDuringDownloadAndInstall(t *testing.T) {
	srv := newTestFeedServer(t, func(baseURL, sig, sha string, size int64) *Appcast {
		return &Appcast{
			Items: []Item{{
				Version:      "2.0.0",
				ShortVersion: "2.0.0",
				ReleaseNotes: "Release notes.",
				Enclosures: []Enclosure{{
					URL:         baseURL + "/update.zip",
					Length:      size,
					EdSignature: sig,
					OS:          "macos",
					Arch:        "arm64",
				}},
			}},
		}
	})

	dlStarted := make(chan struct{})
	dlRelease := make(chan struct{})
	srv.mu.Lock()
	srv.dlStarted = dlStarted
	srv.dlRelease = dlRelease
	srv.mu.Unlock()
	var dlReleaseOnce sync.Once
	releaseDownload := func() { dlReleaseOnce.Do(func() { close(dlRelease) }) }
	defer releaseDownload()

	applyStarted := make(chan struct{})
	applyRelease := make(chan struct{})
	var applyReleaseOnce sync.Once
	releaseApply := func() { applyReleaseOnce.Do(func() { close(applyRelease) }) }
	defer releaseApply()

	applier := &uiTestApplier{
		onApply: func(_ *DownloadedUpdate, _ ApplyOptions) {
			select {
			case <-applyStarted:
			default:
				close(applyStarted)
			}
			<-applyRelease
		},
	}

	dir := t.TempDir()
	u, err := New(Config{
		FeedURL:           srv.server.URL + "/appcast.xml",
		PublicKey:         srv.pubKey,
		CurrentVersion:    "1.0.0",
		AppID:             "com.example.escape",
		AppName:           "EscApp",
		OS:                "macos",
		Arch:              "arm64",
		StateDir:          dir,
		CacheDir:          filepath.Join(dir, "cache"),
		Applier:           applier,
		AllowInsecureHTTP: true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctrl := NewController(u)
	var probe *ggui.Probe
	probe = ggui.ProbeBuilder(func() ggui.Widget {
		ctrl.SetPost(probe.Post)
		return widget.Box(Dialog(ctrl))
	}, ggui.Sz(700, 540))
	defer probe.Close()

	// 1. CheckNow -> PhaseAvailable. Escape MUST dismiss when available.
	ctrl.CheckNow()
	waitFor(t, probe, 3*time.Second, "PhaseAvailable", func() bool {
		return ggui.Peek(ctrl.Phase) == PhaseAvailable
	})
	probe.Advance(300 * time.Millisecond)

	probe.Key("escape")
	probe.Advance(300 * time.Millisecond)
	if ggui.Peek(ctrl.Open) {
		t.Fatalf("Escape should close dialog during PhaseAvailable")
	}

	// 2. Re-open and start download -> Escape & scrim clicks MUST NOT close dialog.
	ctrl.Open.Set(true)
	probe.Advance(300 * time.Millisecond)
	tapButton(t, probe, "Install Update")
	select {
	case <-dlStarted:
	case <-time.After(3 * time.Second):
		t.Fatalf("download never started")
	}
	waitFor(t, probe, 3*time.Second, "PhaseDownloading", func() bool {
		return ggui.Peek(ctrl.Phase) == PhaseDownloading
	})
	probe.Advance(300 * time.Millisecond)

	if _, ok := probe.FindRole(ggui.RoleButton, "Close"); ok {
		t.Errorf("dialog close button should be hidden during PhaseDownloading")
	}
	probe.Key("escape")
	probe.Advance(100 * time.Millisecond)
	if !ggui.Peek(ctrl.Open) || ggui.Peek(ctrl.Phase) != PhaseDownloading {
		t.Fatalf("Escape dismissed dialog during PhaseDownloading (open=%v, phase=%v)", ggui.Peek(ctrl.Open), ggui.Peek(ctrl.Phase))
	}
	probe.Click(ggui.Pt(5, 5))
	probe.Advance(100 * time.Millisecond)
	if !ggui.Peek(ctrl.Open) || ggui.Peek(ctrl.Phase) != PhaseDownloading {
		t.Fatalf("scrim click dismissed dialog during PhaseDownloading (open=%v, phase=%v)", ggui.Peek(ctrl.Open), ggui.Peek(ctrl.Phase))
	}

	// 3. Finish download -> start Install -> Escape & scrim clicks MUST NOT close dialog.
	releaseDownload()
	waitFor(t, probe, 3*time.Second, "PhaseReadyToInstall", func() bool {
		return ggui.Peek(ctrl.Phase) == PhaseReadyToInstall
	})
	tapButton(t, probe, "Install and Relaunch")
	select {
	case <-applyStarted:
	case <-time.After(3 * time.Second):
		t.Fatalf("install never started")
	}
	probe.Advance(300 * time.Millisecond)

	probe.Key("escape")
	probe.Advance(100 * time.Millisecond)
	if !ggui.Peek(ctrl.Open) || ggui.Peek(ctrl.Phase) != PhaseInstalling {
		t.Fatalf("Escape dismissed dialog during PhaseInstalling (open=%v, phase=%v)", ggui.Peek(ctrl.Open), ggui.Peek(ctrl.Phase))
	}
	probe.Click(ggui.Pt(5, 5))
	probe.Advance(100 * time.Millisecond)
	if !ggui.Peek(ctrl.Open) || ggui.Peek(ctrl.Phase) != PhaseInstalling {
		t.Fatalf("scrim click dismissed dialog during PhaseInstalling (open=%v, phase=%v)", ggui.Peek(ctrl.Open), ggui.Peek(ctrl.Phase))
	}
	releaseApply()

	// 4. Constructing Dialog(ctrl) outside a reactive owner (e.g. via NewProbe)
	// must still sync dismissibility/Escape without leaking an Observe onto baseRuntime.
	ctrl2 := NewController(u)
	ctrl2.Release.Set(&Release{Item: Item{Version: "2.0.0", ShortVersion: "2.0.0"}})
	ctrl2.Phase.Set(PhaseInstalling)
	ctrl2.Open.Set(true)
	dlg := Dialog(ctrl2)
	probe2 := ggui.NewProbe(widget.Box(dlg), ggui.Sz(700, 540))
	defer probe2.Close()
	probe2.Advance(300 * time.Millisecond)

	probe2.Key("escape")
	probe2.Advance(100 * time.Millisecond)
	if !ggui.Peek(ctrl2.Open) {
		t.Fatalf("standalone Dialog closed on Escape during PhaseInstalling")
	}
	ctrl2.Phase.Set(PhaseAvailable)
	probe2.Advance(100 * time.Millisecond)
	probe2.Key("escape")
	probe2.Advance(100 * time.Millisecond)
	if ggui.Peek(ctrl2.Open) {
		t.Fatalf("standalone Dialog did not close on Escape after returning to PhaseAvailable")
	}
}

func TestControllerInstallDismissCancelGuards(t *testing.T) {
	dir := t.TempDir()
	applyStarted := make(chan struct{})
	applyRelease := make(chan struct{})
	var applyReleaseOnce sync.Once
	releaseApply := func() { applyReleaseOnce.Do(func() { close(applyRelease) }) }
	defer releaseApply()

	applier := &uiTestApplier{
		onApply: func(_ *DownloadedUpdate, _ ApplyOptions) {
			select {
			case <-applyStarted:
			default:
				close(applyStarted)
			}
			<-applyRelease
		},
	}

	u, err := New(Config{
		FeedURL:           "https://updates.example.com/appcast.xml",
		CurrentVersion:    "1.0.0",
		AppID:             "com.example.guards",
		AppName:           "GuardApp",
		OS:                "macos",
		Arch:              "arm64",
		StateDir:          dir,
		CacheDir:          filepath.Join(dir, "cache"),
		Applier:           applier,
		AllowUnsigned:     true,
		AllowInsecureHTTP: false,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	quitState := ggui.State(false)
	ctrl := NewController(u, ControllerConfig{
		Quit: func() { quitState.Set(true) },
	})

	var probe *ggui.Probe
	probe = ggui.ProbeBuilder(func() ggui.Widget {
		ctrl.SetPost(probe.Post)
		return widget.Box(Dialog(ctrl))
	}, ggui.Sz(700, 540))
	defer probe.Close()
	probe.Frame()

	rel := &Release{
		Item:      Item{Version: "2.0.0", ShortVersion: "2.0.0"},
		Enclosure: Enclosure{URL: "https://updates.example.com/update.zip", OS: "macos"},
	}
	upd := &DownloadedUpdate{
		Release:  *rel,
		FilePath: filepath.Join(dir, "update.zip"),
		Verified: true,
	}
	ctrl.Release.Set(rel)
	ctrl.Downloaded.Set(upd)
	ctrl.Phase.Set(PhaseReadyToInstall)
	ctrl.Open.Set(true)
	probe.Frame()

	// CancelDownload outside PhaseDownloading must be a no-op.
	ctrl.CancelDownload()
	if got := ggui.Peek(ctrl.Phase); got != PhaseReadyToInstall {
		t.Fatalf("CancelDownload during PhaseReadyToInstall changed phase to %v", got)
	}

	// Start Install, then try double Install, Dismiss, and CancelDownload while PhaseInstalling.
	ctrl.Install()
	select {
	case <-applyStarted:
	case <-time.After(3 * time.Second):
		t.Fatalf("first Install never started")
	}

	ctrl.Install()
	ctrl.Dismiss()
	ctrl.CancelDownload()

	if !ggui.Peek(ctrl.Open) {
		t.Fatalf("Dismiss hid dialog during PhaseInstalling")
	}
	if got := ggui.Peek(ctrl.Phase); got != PhaseInstalling {
		t.Fatalf("Phase during install = %v; want PhaseInstalling", got)
	}

	releaseApply()
	waitFor(t, probe, 3*time.Second, "Quit posted to UI thread", func() bool {
		return ggui.Peek(quitState)
	})
	if calls, _, _ := applier.snapshot(); calls != 1 {
		t.Fatalf("applier calls = %d; want 1 after double Install", calls)
	}

	// Install error path must surface PhaseError AND re-open the dialog.
	applierErr := &uiTestApplier{err: errors.New("disk full")}
	uErr, err := New(Config{
		FeedURL:        "https://updates.example.com/appcast.xml",
		CurrentVersion: "1.0.0",
		AppID:          "com.example.guards-err",
		AppName:        "GuardErrApp",
		OS:             "macos",
		Arch:           "arm64",
		StateDir:       t.TempDir(),
		CacheDir:       filepath.Join(t.TempDir(), "cache"),
		Applier:        applierErr,
		AllowUnsigned:  true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctrlErr := NewController(uErr)
	ctrlErr.SetPost(probe.Post)
	ctrlErr.Downloaded.Set(upd)
	ctrlErr.Phase.Set(PhaseReadyToInstall)
	ctrlErr.Install()
	// Simulate dialog hidden before the async Apply error posts back.
	ctrlErr.Open.Set(false)
	waitFor(t, probe, 3*time.Second, "install error surfaces", func() bool {
		return ggui.Peek(ctrlErr.Phase) == PhaseError
	})
	if !ggui.Peek(ctrlErr.Open) {
		t.Fatalf("Install error did not set Open=true")
	}
	if !strings.Contains(ggui.Peek(ctrlErr.ErrText), "disk full") {
		t.Fatalf("ErrText = %q; want disk full", ggui.Peek(ctrlErr.ErrText))
	}
}

func TestControllerCheckSilentSkipRace(t *testing.T) {
	srv := newTestFeedServer(t, func(baseURL, sig, sha string, size int64) *Appcast {
		return &Appcast{
			Items: []Item{{
				Version:      "2.0.0",
				ShortVersion: "2.0.0",
				ReleaseNotes: "Version 2.0.0 notes.",
				Enclosures: []Enclosure{{
					URL:         baseURL + "/update.zip",
					Length:      size,
					EdSignature: sig,
					OS:          "macos",
					Arch:        "arm64",
				}},
			}},
		}
	})

	dir := t.TempDir()
	u, err := New(Config{
		FeedURL:           srv.server.URL + "/appcast.xml",
		PublicKey:         srv.pubKey,
		CurrentVersion:    "1.0.0",
		AppID:             "com.example.skiprace",
		AppName:           "RaceApp",
		OS:                "macos",
		Arch:              "arm64",
		StateDir:          dir,
		CacheDir:          filepath.Join(dir, "cache"),
		AllowInsecureHTTP: true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctrl := NewController(u)
	var probe *ggui.Probe
	probe = ggui.ProbeBuilder(func() ggui.Widget {
		return widget.Box(Dialog(ctrl))
	}, ggui.Sz(700, 540))
	defer probe.Close()
	probe.Frame()

	// Intercept CheckSilent's postUI callback so we can invoke Skip() while the
	// already-fetched 2.0.0 release is in flight to the UI goroutine.
	intercepted := make(chan func(), 4)
	ctrl.SetPost(func(fn func()) {
		intercepted <- fn
	})

	ctrl.CheckSilent()
	var silentCB func()
	select {
	case silentCB = <-intercepted:
	case <-time.After(3 * time.Second):
		t.Fatalf("CheckSilent never posted result")
	}

	// User skips version 2.0.0 before the silent check callback runs on UI thread.
	ctrl.Release.Set(&Release{Item: Item{Version: "2.0.0", ShortVersion: "2.0.0"}})
	ctrl.Phase.Set(PhaseAvailable)
	ctrl.Open.Set(true)
	ctrl.Skip()
	if got := u.SkippedVersion(); got != "2.0.0" {
		t.Fatalf("SkippedVersion = %q; want 2.0.0", got)
	}

	// Deliver the stale silent callback on the UI goroutine via probe.Post.
	ctrl.SetPost(probe.Post)
	probe.Post(silentCB)
	probe.Advance(100 * time.Millisecond)

	if ggui.Peek(ctrl.Open) {
		t.Fatalf("in-flight CheckSilent re-opened dialog for skipped version 2.0.0")
	}
	if got := ggui.Peek(ctrl.Phase); got != PhaseIdle {
		t.Fatalf("Phase = %v; want PhaseIdle", got)
	}

	// Verify presentBackgroundRelease also suppresses skipped versions directly,
	// while still allowing critical updates and newer versions.
	ctrl.presentBackgroundRelease(&Release{Item: Item{Version: "2.0.0", ShortVersion: "2.0.0"}})
	probe.Advance(50 * time.Millisecond)
	if ggui.Peek(ctrl.Open) {
		t.Fatalf("presentBackgroundRelease opened dialog for skipped 2.0.0")
	}

	ctrl.presentBackgroundRelease(&Release{Item: Item{Version: "2.0.0", ShortVersion: "2.0.0", CriticalUpdate: true}})
	probe.Advance(50 * time.Millisecond)
	if !ggui.Peek(ctrl.Open) || ggui.Peek(ctrl.Phase) != PhaseAvailable {
		t.Fatalf("presentBackgroundRelease should still open dialog for critical update")
	}
}

func TestControllerPendingPostFlushesOnUIThread(t *testing.T) {
	srv := newTestFeedServer(t, func(baseURL, sig, sha string, size int64) *Appcast {
		return &Appcast{
			Items: []Item{{
				Version:      "2.0.0",
				ShortVersion: "2.0.0",
				ReleaseNotes: "Queued release.",
				Enclosures: []Enclosure{{
					URL:         baseURL + "/update.zip",
					Length:      size,
					EdSignature: sig,
					OS:          "macos",
					Arch:        "arm64",
				}},
			}},
		}
	})

	dir := t.TempDir()
	u, err := New(Config{
		FeedURL:           srv.server.URL + "/appcast.xml",
		PublicKey:         srv.pubKey,
		CurrentVersion:    "1.0.0",
		AppID:             "com.example.pending",
		AppName:           "PendingApp",
		OS:                "macos",
		Arch:              "arm64",
		StateDir:          dir,
		CacheDir:          filepath.Join(dir, "cache"),
		AllowInsecureHTTP: true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Controller intentionally starts with Post == nil.
	customQuit := 0
	ctrl := NewController(u, ControllerConfig{
		Quit: func() { customQuit++ },
	})
	// ensureQuit must not overwrite an existing Quit callback.
	ctrl.ensureQuit(func() { customQuit += 100 })
	ctrl.mu.Lock()
	q := ctrl.quit
	ctrl.mu.Unlock()
	q()
	if customQuit != 1 {
		t.Fatalf("ensureQuit clobbered existing Quit callback (customQuit=%d; want 1)", customQuit)
	}

	probe := ggui.ProbeBuilder(func() ggui.Widget {
		return widget.Box()
	}, ggui.Sz(400, 300))
	defer probe.Close()
	// Mark this goroutine as the UI goroutine under -tags ggui_debug.
	probe.Frame()

	orderState := ggui.State([]int(nil))
	var order []int
	doneEnqueue := make(chan struct{})
	go func() {
		defer close(doneEnqueue)
		ctrl.postUI(func() {
			order = append(order, 1)
			orderState.Set(slices.Clone(order))
		})
		ctrl.postUI(func() {
			order = append(order, 2)
			orderState.Set(slices.Clone(order))
		})
	}()
	<-doneEnqueue

	// Trigger CheckSilent while Post is still nil; the background goroutine must
	// queue its UI update instead of writing StateValues off-thread.
	ctrl.CheckSilent()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		ctrl.mu.Lock()
		n := len(ctrl.pending)
		ctrl.mu.Unlock()
		if n >= 3 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	ctrl.mu.Lock()
	queuedCount := len(ctrl.pending)
	ctrl.mu.Unlock()
	if queuedCount < 3 {
		t.Fatalf("expected >= 3 queued pending callbacks, got %d", queuedCount)
	}
	if got := ggui.Peek(ctrl.Phase); got != PhaseIdle {
		t.Fatalf("Phase mutated off-UI-thread while Post==nil: got %v", got)
	}

	// Wiring SetPost must drain queued callbacks in FIFO order onto the UI thread.
	ctrl.SetPost(probe.Post)
	waitFor(t, probe, 3*time.Second, "pending callbacks flushed", func() bool {
		return ggui.Peek(ctrl.Open) && ggui.Peek(ctrl.Phase) == PhaseAvailable
	})
	if !slices.Equal(ggui.Peek(orderState), []int{1, 2}) {
		t.Fatalf("pending callbacks ran in order %v; want [1 2]", ggui.Peek(orderState))
	}
}
