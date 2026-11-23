// Command updater is a self-contained runnable demo of the Sparkle 2-compatible
// ggui auto-updater.
//
// It starts an in-process local appcast server backed by a freshly generated
// Ed25519 key pair, serves signed stable (v2.0.0) and beta (v2.1.0-beta.1)
// releases with rich Markdown release notes, and uses an in-app simulated
// Applier so clicking "Install and Relaunch" verifies the Ed25519 signature and
// SHA-256 digest and updates the displayed app version live without exiting.
//
// Run directly:
//
//	go run ./docs/examples/updater
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
	"github.com/ironpark/ggui/ui/icons"
	uitheme "github.com/ironpark/ggui/ui/theme"
	"github.com/ironpark/ggui/updater"
	"github.com/ironpark/ggui/widget"
)

const stableNotes = `## Acme Notes 2.0.0

A major release focused on speed, Markdown polish, and reliable sync.

### Highlights
- **2x faster cold start** with incremental workspace indexing
- **Sparkle 2 Ed25519 updates** verified before installation
- Improved split-view editor with keyboard navigation (` + "`cmd+\\`" + `)
- Fixed dark-mode scrollbar contrast on macOS and Windows`

const betaNotes = `## Acme Notes 2.1.0-beta.1

Early preview of the upcoming 2.1 milestone on the **beta** channel.

### What's in this beta
- Experimental **command palette** with fuzzy command scoring
- Local SQLite full-text search index
- Known issue: custom themes reset after changing font scale`

type demoServer struct {
	server *httptest.Server
	pubKey string

	mu            sync.Mutex
	downloadDelay time.Duration
	onRequest     func(string)
}

func newDemoServer(downloadDelay time.Duration, onRequest func(string)) (*demoServer, error) {
	pub, priv, err := updater.GenerateKeyPair()
	if err != nil {
		return nil, err
	}

	stablePayload := bytes.Repeat([]byte("ACME-NOTES-2.0.0-SIGNED-ARCHIVE-"), 2048) // ~64 KB
	betaPayload := bytes.Repeat([]byte("ACME-NOTES-2.1.0-BETA.1-ARCHIVE-"), 2048)   // ~64 KB

	stableSig, err := updater.SignBytes(priv, stablePayload)
	if err != nil {
		return nil, err
	}
	betaSig, err := updater.SignBytes(priv, betaPayload)
	if err != nil {
		return nil, err
	}

	stableSum := sha256.Sum256(stablePayload)
	betaSum := sha256.Sum256(betaPayload)
	stableSHA := hex.EncodeToString(stableSum[:])
	betaSHA := hex.EncodeToString(betaSum[:])

	ds := &demoServer{
		pubKey:        pub,
		downloadDelay: downloadDelay,
		onRequest:     onRequest,
	}

	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/appcast.xml", func(w http.ResponseWriter, r *http.Request) {
		ds.notify("GET /appcast.xml (" + r.Header.Get("User-Agent") + ")")
		base := srv.URL
		feed := &updater.Appcast{
			Title:       "Acme Notes Updates",
			Description: "Local Sparkle 2 demo appcast feed",
			Language:    "en",
			Items: []updater.Item{
				{
					Title:        "Acme Notes 2.0.0",
					Version:      "2.0.0",
					ShortVersion: "2.0.0",
					PubDate:      time.Date(2026, time.November, 15, 12, 0, 0, 0, time.UTC),
					ReleaseNotes: stableNotes,
					Enclosures:   multiPlatformEnclosures(base+"/releases/acme-2.0.0.zip", int64(len(stablePayload)), stableSig, stableSHA),
				},
				{
					Title:        "Acme Notes 2.1.0-beta.1",
					Version:      "2.1.0-beta.1",
					ShortVersion: "2.1.0-beta.1",
					Channel:      "beta",
					PubDate:      time.Date(2026, time.November, 22, 12, 0, 0, 0, time.UTC),
					ReleaseNotes: betaNotes,
					Enclosures:   multiPlatformEnclosures(base+"/releases/acme-2.1.0-beta.1.zip", int64(len(betaPayload)), betaSig, betaSHA),
				},
			},
		}
		data, err := updater.MarshalAppcast(feed)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
		_, _ = w.Write(data)
	})

	serveArchive := func(name string, payload []byte) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			ds.notify(fmt.Sprintf("GET %s (%d bytes)", name, len(payload)))
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(payload)))

			ds.mu.Lock()
			delay := ds.downloadDelay
			ds.mu.Unlock()

			const chunk = 8192
			for off := 0; off < len(payload); off += chunk {
				end := min(off+chunk, len(payload))
				if _, err := w.Write(payload[off:end]); err != nil {
					return
				}
				if fl, ok := w.(http.Flusher); ok {
					fl.Flush()
				}
				if delay > 0 {
					select {
					case <-time.After(delay):
					case <-r.Context().Done():
						return
					}
				}
			}
		}
	}

	mux.HandleFunc("/releases/acme-2.0.0.zip", serveArchive("/releases/acme-2.0.0.zip", stablePayload))
	mux.HandleFunc("/releases/acme-2.1.0-beta.1.zip", serveArchive("/releases/acme-2.1.0-beta.1.zip", betaPayload))

	srv = httptest.NewServer(mux)
	ds.server = srv
	return ds, nil
}

func (ds *demoServer) notify(msg string) {
	ds.mu.Lock()
	fn := ds.onRequest
	ds.mu.Unlock()
	if fn != nil {
		fn(msg)
	}
}

func (ds *demoServer) Close() {
	if ds.server != nil {
		ds.server.Close()
	}
}

func multiPlatformEnclosures(rawURL string, length int64, sig, sha string) []updater.Enclosure {
	platforms := []string{"macos", "windows", "linux"}
	out := make([]updater.Enclosure, 0, len(platforms))
	for _, osName := range platforms {
		out = append(out, updater.Enclosure{
			URL:         rawURL,
			Length:      length,
			Type:        "application/octet-stream",
			EdSignature: sig,
			SHA256:      sha,
			OS:          osName,
		})
	}
	return out
}

type demoApplier struct {
	demo *demoApp
}

func (a *demoApplier) Apply(_ context.Context, upd *updater.DownloadedUpdate, opts updater.ApplyOptions) error {
	newVer := upd.Release.DisplayVersion()
	shortSHA := upd.SHA256
	if len(shortSHA) > 12 {
		shortSHA = shortSHA[:12]
	}
	a.demo.logAsync(fmt.Sprintf(
		"Verified Ed25519 signature (verified=%v) & SHA-256 %s… (%d bytes)",
		upd.Verified, shortSHA, upd.Size,
	))
	a.demo.logAsync(fmt.Sprintf(
		"Simulating in-app update to v%s (relaunch=%v, silent=%v)",
		newVer, opts.Relaunch, opts.Silent,
	))
	a.demo.postUI(func() {
		a.demo.currentVersion.Set(newVer)
		a.demo.rebuildController()
		a.demo.appendLog(fmt.Sprintf("Application updated live to v%s!", newVer))
	})
	return nil
}

type demoApp struct {
	server   *demoServer
	tempDir  string
	stateDir string
	cacheDir string

	currentVersion *ggui.StateValue[string]
	channel        *ggui.StateValue[string]
	ctrl           *ggui.StateValue[*updater.Controller]
	logs           *ggui.StateValue[[]string]

	postMu sync.Mutex
	post   func(func())
}

func newDemoApp(downloadDelay time.Duration) (*demoApp, error) {
	tempDir, err := os.MkdirTemp("", "ggui-updater-demo-*")
	if err != nil {
		return nil, err
	}

	d := &demoApp{
		tempDir:        tempDir,
		stateDir:       filepath.Join(tempDir, "state"),
		cacheDir:       filepath.Join(tempDir, "cache"),
		currentVersion: ggui.State("1.0.0"),
		channel:        ggui.State("stable"),
		ctrl:           ggui.State[*updater.Controller](nil),
		logs:           ggui.State([]string{}),
	}

	srv, err := newDemoServer(downloadDelay, func(msg string) {
		d.logAsync("Server: " + msg)
	})
	if err != nil {
		_ = os.RemoveAll(tempDir)
		return nil, err
	}
	d.server = srv

	d.appendLog("Generated ephemeral Ed25519 key pair (SUPublicEDKey: " + srv.pubKey[:16] + "…)")
	d.appendLog("Local Sparkle 2 appcast server listening at " + srv.server.URL + "/appcast.xml")

	if err := d.rebuildController(); err != nil {
		srv.Close()
		_ = os.RemoveAll(tempDir)
		return nil, err
	}
	return d, nil
}

func (d *demoApp) Close() {
	if d.server != nil {
		d.server.Close()
	}
	if d.tempDir != "" {
		_ = os.RemoveAll(d.tempDir)
	}
}

func (d *demoApp) setPost(fn func(func())) {
	d.postMu.Lock()
	d.post = fn
	d.postMu.Unlock()
	if ctrl := ggui.Peek(d.ctrl); ctrl != nil {
		ctrl.SetPost(fn)
	}
}

func (d *demoApp) postUI(fn func()) {
	d.postMu.Lock()
	post := d.post
	d.postMu.Unlock()
	if post != nil {
		post(fn)
		return
	}
	fn()
}

func (d *demoApp) appendLog(entry string) {
	stamp := time.Now().Format("15:04:05")
	ggui.Append(d.logs, fmt.Sprintf("[%s] %s", stamp, entry))
}

func (d *demoApp) logAsync(entry string) {
	d.postUI(func() { d.appendLog(entry) })
}

func (d *demoApp) rebuildController() error {
	ver := ggui.Peek(d.currentVersion)
	ch := ggui.Peek(d.channel)

	u, err := updater.New(updater.Config{
		FeedURL:           d.server.server.URL + "/appcast.xml",
		PublicKey:         d.server.pubKey,
		CurrentVersion:    ver,
		Channel:           ch,
		AppID:             "com.example.acme-notes",
		AppName:           "Acme Notes",
		StateDir:          d.stateDir,
		CacheDir:          d.cacheDir,
		Applier:           &demoApplier{demo: d},
		AllowInsecureHTTP: true,
	})
	if err != nil {
		return err
	}

	d.postMu.Lock()
	post := d.post
	d.postMu.Unlock()

	if prev := ggui.Peek(d.ctrl); prev != nil {
		prev.Dismiss()
	}
	ctrl := updater.NewController(u, updater.ControllerConfig{
		Post: post,
	})
	d.ctrl.Set(ctrl)
	return nil
}

func (d *demoApp) checkNow() {
	ctrl := ggui.Peek(d.ctrl)
	if ctrl == nil {
		return
	}
	d.appendLog(fmt.Sprintf("Manual check started (current=v%s, channel=%s)", ggui.Peek(d.currentVersion), ggui.Peek(d.channel)))
	ctrl.CheckNow()
}

func (d *demoApp) checkSilent() {
	ctrl := ggui.Peek(d.ctrl)
	if ctrl == nil {
		return
	}
	skipped := ctrl.Updater().SkippedVersion()
	if skipped != "" {
		d.appendLog(fmt.Sprintf("Background check started (skipped=v%s)", skipped))
	} else {
		d.appendLog("Background check started")
	}
	ctrl.CheckSilent()
}

func (d *demoApp) clearSkipped() {
	ctrl := ggui.Peek(d.ctrl)
	if ctrl == nil {
		return
	}
	prev := ctrl.Updater().SkippedVersion()
	if err := ctrl.Updater().ClearSkippedVersion(); err != nil {
		d.appendLog("Error clearing skipped version: " + err.Error())
		return
	}
	if prev == "" {
		d.appendLog("No skipped version was recorded")
	} else {
		d.appendLog("Cleared skipped version v" + prev)
	}
}

func (d *demoApp) onChannelChange(ch string) {
	if err := d.rebuildController(); err != nil {
		d.appendLog("Error switching channel: " + err.Error())
		return
	}
	d.appendLog("Switched update channel to " + ch)
}

func (d *demoApp) buildUI() ggui.Widget {
	if h, ok := ggui.LookupHost(); ok {
		d.setPost(h.Post)
	}

	versionBadge := ggui.View(d.currentVersion, func(ver string) ggui.Widget {
		return ui.Badge("v" + ver).Secondary()
	})

	header := widget.Row(
		widget.Box(widget.Center(ui.Icon(icons.Download).Size(20).Color(uitheme.Primary))).
			Size(40, 40).Radius(10).Fill(uitheme.Muted),
		widget.Expanded(widget.Column(
			widget.Row(
				ui.Title("Acme Notes"),
				versionBadge,
			).Space(1).Align(widget.AlignCenter),
			ui.Caption("Pure-Go Sparkle 2 auto-updater demo with local Ed25519 signing"),
		).Space(0.25)),
	).Space(1.5).Align(widget.AlignCenter)

	channelPicker := ui.Select(d.channel).
		Name("Update Channel").
		Options([]string{"stable", "beta"}).
		MinWidth(130).
		OnChange(d.onChannelChange)

	controls := ui.Card(
		ui.CardHeader("Update Controls", "Test manual checks, silent background checks, channels, and skipped versions."),
		ui.CardContent(widget.Column(
			widget.Row(
				ui.Caption("Release channel:"),
				channelPicker,
				widget.Spacer(),
			).Space(1).Align(widget.AlignCenter),
			widget.Row(
				ui.Button("Check for Updates…", d.checkNow).Icon(ui.Icon(icons.Download)),
				ui.Button("Simulate Background Check", d.checkSilent).Outline(),
				ui.Button("Clear Skipped Version", d.clearSkipped).Outline(),
			).Space(1),
		).Space(1.25).Align(widget.AlignStretch)),
	)

	logView := ggui.View(d.logs, func(entries []string) ggui.Widget {
		if len(entries) == 0 {
			return ui.Caption("No update activity yet.")
		}
		return ui.Mono(strings.Join(entries, "\n"))
	})

	activityCard := ui.Card(
		ui.CardHeader("Activity Log", "Appcast requests, Ed25519 signature checks, and simulated installs."),
		ui.CardContent(
			widget.Box(
				widget.Scroll(widget.Box(logView).Pad(10)).FollowEnd(),
			).Height(190).Border(1, uitheme.Border).Radius(6).Fill(uitheme.Bg).Clip(),
		),
	)

	updateDialog := ggui.View(d.ctrl, func(ctrl *updater.Controller) ggui.Widget {
		if ctrl == nil {
			return widget.Box()
		}
		return updater.Dialog(ctrl)
	})

	return widget.Box(
		widget.Column(
			header,
			controls,
			activityCard,
			updateDialog,
		).Space(1.5).Align(widget.AlignStretch),
	).Pad(24).Fill(uitheme.Bg)
}

func main() {
	demo, err := newDemoApp(25 * time.Millisecond)
	if err != nil {
		log.Fatal(err)
	}
	defer demo.Close()

	app := ggui.New(ggui.Config{
		Title:     "ggui · Sparkle 2 Auto-Updater Demo",
		Width:     720,
		Height:    600,
		Resizable: true,
	}, demo.buildUI)
	app.Setup(func() {
		uitheme.Bind(ggui.SystemDark(), uitheme.Dark(), uitheme.Default())
	})
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
