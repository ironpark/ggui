package updater

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ironpark/ggui"
)

// Phase describes the current UI stage of an update Controller.
type Phase int

const (
	PhaseIdle Phase = iota
	PhaseChecking
	PhaseUpToDate
	PhaseAvailable
	PhaseDownloading
	PhaseReadyToInstall
	PhaseInstalling
	PhaseError
)

// String returns a human-readable name for p.
func (p Phase) String() string {
	switch p {
	case PhaseIdle:
		return "Idle"
	case PhaseChecking:
		return "Checking"
	case PhaseUpToDate:
		return "UpToDate"
	case PhaseAvailable:
		return "Available"
	case PhaseDownloading:
		return "Downloading"
	case PhaseReadyToInstall:
		return "ReadyToInstall"
	case PhaseInstalling:
		return "Installing"
	case PhaseError:
		return "Error"
	default:
		return fmt.Sprintf("Phase(%d)", int(p))
	}
}

// ControllerConfig configures a reactive Controller for an Updater.
type ControllerConfig struct {
	// Post marshals callbacks onto the UI goroutine (for example app.Post or
	// probe.Post). May also be wired later via Attach or SetPost.
	Post func(func())

	// Quit shuts down the running application when ApplyOptions.QuitFunc is
	// invoked (for example app.Quit or app.Close). May also be wired later via
	// Attach or SetQuit.
	Quit func()

	// Relaunch controls ApplyOptions.Relaunch. Defaults to true when nil.
	Relaunch *bool

	// SilentInstall controls ApplyOptions.Silent. Defaults to true when nil so
	// installer-based upgrades (such as Windows Name-Setup.exe) run seamlessly.
	SilentInstall *bool

	// OnLink handles link taps inside Markdown release notes and the full
	// release notes action. Only validated https (and http when
	// Config.AllowInsecureHTTP is true) and safe mailto URLs are forwarded;
	// unsafe schemes such as javascript: or file:// are rejected. When nil,
	// validated https (and allowed http) URLs are opened in the system browser.
	OnLink func(url string)
}

type failedOp int

const (
	failedNone failedOp = iota
	failedCheck
	failedDownload
	failedInstall
)

// Controller bridges an *Updater to reactive ggui state and UI controls.
//
// All exported *ggui.StateValue fields are mutated strictly on the UI goroutine
// (via Post) so builds with -race and -tags ggui_debug never race or panic.
type Controller struct {
	// Open controls update modal/window visibility and binds directly to ui.Dialog.
	Open *ggui.StateValue[bool]
	// Phase tracks the current stage of the update flow.
	Phase *ggui.StateValue[Phase]
	// Release holds the currently available update release, if any.
	Release *ggui.StateValue[*Release]
	// Downloaded holds the verified downloaded update ready to install, if any.
	Downloaded *ggui.StateValue[*DownloadedUpdate]
	// Progress reports download completion in the range [0, 1].
	Progress *ggui.StateValue[float64]
	// ProgressText holds a formatted status line during download and verification.
	ProgressText *ggui.StateValue[string]
	// ErrText holds the most recent user-visible error message when Phase is PhaseError.
	ErrText *ggui.StateValue[string]
	// AutoCheck mirrors Updater.AutoCheckEnabled() for binding to a checkbox.
	AutoCheck *ggui.StateValue[bool]
	// ManualCheck reports whether the current or most recent check was user-initiated.
	ManualCheck *ggui.StateValue[bool]

	updater *Updater

	mu               sync.Mutex
	post             func(func())
	pending          []func()
	draining         bool
	quit             func()
	relaunch         bool
	silentInstall    bool
	onLink           func(url string)
	opSeq            uint64
	cancelOp         context.CancelFunc
	silentSeq        uint64
	cancelSilent     context.CancelFunc
	lastFailed       failedOp
	retryAutoInstall bool
}

// NewController creates a reactive UI Controller wrapping u.
func NewController(u *Updater, cfg ...ControllerConfig) *Controller {
	if u == nil {
		panic("updater: NewController requires a non-nil *Updater")
	}
	var c ControllerConfig
	if len(cfg) > 0 {
		c = cfg[0]
	}
	relaunch := true
	if c.Relaunch != nil {
		relaunch = *c.Relaunch
	}
	silentInstall := true
	if c.SilentInstall != nil {
		silentInstall = *c.SilentInstall
	}

	return &Controller{
		Open:          ggui.State(false),
		Phase:         ggui.State(PhaseIdle),
		Release:       ggui.State[*Release](nil),
		Downloaded:    ggui.State[*DownloadedUpdate](nil),
		Progress:      ggui.State(0.0),
		ProgressText:  ggui.State(""),
		ErrText:       ggui.State(""),
		AutoCheck:     ggui.State(u.AutoCheckEnabled()),
		ManualCheck:   ggui.State(false),
		updater:       u,
		post:          c.Post,
		quit:          c.Quit,
		relaunch:      relaunch,
		silentInstall: silentInstall,
		onLink:        c.OnLink,
	}
}

// Attach wires app.Post and app.Quit into c and returns c for chaining.
func (c *Controller) Attach(app *ggui.App) *Controller {
	if app == nil {
		return c
	}
	c.mu.Lock()
	c.post = app.Post
	c.quit = app.Quit
	c.mu.Unlock()
	c.flushPending()
	return c
}

// SetPost sets the UI goroutine marshaler (for example probe.Post or window.Post).
func (c *Controller) SetPost(fn func(func())) {
	c.mu.Lock()
	c.post = fn
	c.mu.Unlock()
	c.flushPending()
}

// SetQuit sets the application quit callback passed to ApplyOptions.QuitFunc.
func (c *Controller) SetQuit(fn func()) {
	c.mu.Lock()
	c.quit = fn
	c.mu.Unlock()
}

// ensurePost sets Post only if none has been configured yet.
func (c *Controller) ensurePost(fn func(func())) {
	if fn == nil {
		return
	}
	c.mu.Lock()
	if c.post == nil {
		c.post = fn
	}
	c.mu.Unlock()
	c.flushPending()
}

// ensureQuit sets Quit only if none has been configured yet.
func (c *Controller) ensureQuit(fn func()) {
	if fn == nil {
		return
	}
	c.mu.Lock()
	if c.quit == nil {
		c.quit = fn
	}
	c.mu.Unlock()
}

// Updater returns the underlying *Updater.
func (c *Controller) Updater() *Updater {
	return c.updater
}

func (c *Controller) linkHandler() func(string) {
	c.mu.Lock()
	fn := c.onLink
	c.mu.Unlock()
	return fn
}

func (c *Controller) postUI(fn func()) {
	if fn == nil {
		return
	}
	c.mu.Lock()
	if c.post == nil || c.draining {
		c.pending = append(c.pending, fn)
		c.mu.Unlock()
		return
	}
	post := c.post
	c.mu.Unlock()
	post(fn)
}

func (c *Controller) flushPending() {
	c.mu.Lock()
	if c.post == nil || len(c.pending) == 0 || c.draining {
		c.mu.Unlock()
		return
	}
	c.draining = true
	c.mu.Unlock()

	done := false
	defer func() {
		if !done {
			c.mu.Lock()
			c.draining = false
			c.mu.Unlock()
		}
	}()

	for {
		c.mu.Lock()
		if c.post == nil || len(c.pending) == 0 {
			c.draining = false
			done = true
			c.mu.Unlock()
			return
		}
		post := c.post
		fn := c.pending[0]
		c.pending[0] = nil
		c.pending = c.pending[1:]
		if len(c.pending) == 0 {
			c.pending = nil
		}
		c.mu.Unlock()
		post(fn)
	}
}

func (c *Controller) beginOp() (uint64, context.Context) {
	c.mu.Lock()
	if c.cancelOp != nil {
		c.cancelOp()
	}
	if c.cancelSilent != nil {
		c.cancelSilent()
		c.cancelSilent = nil
	}
	c.silentSeq++
	c.opSeq++
	seq := c.opSeq
	ctx, cancel := context.WithCancel(context.Background())
	c.cancelOp = cancel
	c.mu.Unlock()
	return seq, ctx
}

func (c *Controller) isCurrentOp(seq uint64) bool {
	c.mu.Lock()
	ok := c.opSeq == seq
	c.mu.Unlock()
	return ok
}

func (c *Controller) clearOp(seq uint64) {
	c.mu.Lock()
	if c.opSeq == seq {
		if c.cancelOp != nil {
			c.cancelOp()
			c.cancelOp = nil
		}
	}
	c.mu.Unlock()
}

func (c *Controller) cancelActiveOp() {
	c.mu.Lock()
	c.opSeq++
	if c.cancelOp != nil {
		c.cancelOp()
		c.cancelOp = nil
	}
	c.mu.Unlock()
}

func (c *Controller) beginSilentOp() (uint64, context.Context) {
	c.mu.Lock()
	if c.cancelSilent != nil {
		c.cancelSilent()
	}
	c.silentSeq++
	seq := c.silentSeq
	ctx, cancel := context.WithCancel(context.Background())
	c.cancelSilent = cancel
	c.mu.Unlock()
	return seq, ctx
}

func (c *Controller) isCurrentSilentOp(seq uint64) bool {
	c.mu.Lock()
	ok := c.silentSeq == seq
	c.mu.Unlock()
	return ok
}

func (c *Controller) clearSilentOp(seq uint64) {
	c.mu.Lock()
	if c.silentSeq == seq {
		if c.cancelSilent != nil {
			c.cancelSilent()
			c.cancelSilent = nil
		}
	}
	c.mu.Unlock()
}

func (c *Controller) setFailedOp(op failedOp, autoInstall bool) {
	c.mu.Lock()
	c.lastFailed = op
	c.retryAutoInstall = autoInstall
	c.mu.Unlock()
}

func (c *Controller) getFailedOp() (failedOp, bool) {
	c.mu.Lock()
	op, auto := c.lastFailed, c.retryAutoInstall
	c.mu.Unlock()
	return op, auto
}

// CheckNow initiates a user-driven update check (bypassing skipped versions and
// phased rollout delays), opens the update UI immediately in PhaseChecking, and
// transitions to PhaseAvailable, PhaseUpToDate, or PhaseError when complete.
func (c *Controller) CheckNow() {
	seq, ctx := c.beginOp()
	c.setFailedOp(failedNone, false)

	c.ManualCheck.Set(true)
	c.ErrText.Set("")
	c.Progress.Set(0)
	c.ProgressText.Set("")
	c.Phase.Set(PhaseChecking)
	c.Open.Set(true)

	u := c.updater
	go func() {
		rel, ok, err := u.CheckWithOptions(ctx, CheckOptions{Manual: true})
		c.postUI(func() {
			if !c.isCurrentOp(seq) {
				return
			}
			c.clearOp(seq)
			if err != nil {
				if errors.Is(err, context.Canceled) && ctx.Err() != nil {
					return
				}
				c.setFailedOp(failedCheck, false)
				c.ErrText.Set(err.Error())
				c.Phase.Set(PhaseError)
				return
			}
			c.setFailedOp(failedNone, false)
			if !ok || rel == nil {
				c.Release.Set(nil)
				c.Downloaded.Set(nil)
				c.Phase.Set(PhaseUpToDate)
				return
			}
			c.Release.Set(rel)
			c.Downloaded.Set(nil)
			c.Phase.Set(PhaseAvailable)
		})
	}()
}

// CheckSilent performs an automatic background check without opening the update
// UI while checking. Following Sparkle UX, it remains silent if the app is up
// to date or if a transient network error occurs, and opens the dialog only
// when an eligible update is discovered.
func (c *Controller) CheckSilent() {
	seq, ctx := c.beginSilentOp()
	u := c.updater

	go func() {
		rel, ok, err := u.CheckWithOptions(ctx, CheckOptions{Manual: false})
		if err != nil || !ok || rel == nil || ctx.Err() != nil {
			c.clearSilentOp(seq)
			return
		}
		c.postUI(func() {
			if !c.isCurrentSilentOp(seq) {
				return
			}
			c.clearSilentOp(seq)
			c.presentBackgroundReleaseOnUI(rel)
		})
	}()
}

// StartBackground launches periodic automatic checks via Updater.StartBackground
// and opens the update dialog on the UI goroutine whenever an eligible update
// is found. Call stop (or cancel ctx) to shut down the background goroutine.
func (c *Controller) StartBackground(ctx context.Context) (stop func()) {
	if ctx == nil {
		ctx = context.Background()
	}
	return c.updater.StartBackground(ctx, func(rel *Release) {
		if rel == nil {
			return
		}
		c.presentBackgroundRelease(rel)
	})
}

func (c *Controller) presentBackgroundRelease(rel *Release) {
	c.postUI(func() {
		c.presentBackgroundReleaseOnUI(rel)
	})
}

func (c *Controller) presentBackgroundReleaseOnUI(rel *Release) {
	if rel == nil {
		return
	}
	switch ggui.Peek(c.Phase) {
	case PhaseChecking, PhaseDownloading, PhaseReadyToInstall, PhaseInstalling:
		return
	}
	skipped := strings.TrimSpace(c.updater.SkippedVersion())
	if skipped != "" && !rel.IsCriticalFor(c.updater.Config().CurrentVersion) && CompareVersion(rel.CompareVersion(), skipped) <= 0 {
		return
	}
	c.setFailedOp(failedNone, false)
	c.ManualCheck.Set(false)
	c.Release.Set(rel)
	c.Downloaded.Set(nil)
	c.Progress.Set(0)
	c.ProgressText.Set("")
	c.ErrText.Set("")
	c.Phase.Set(PhaseAvailable)
	c.Open.Set(true)
}

// Download downloads and verifies the currently selected Release in the
// background, updating Progress and ProgressText on the UI goroutine and
// transitioning to PhaseReadyToInstall on success.
func (c *Controller) Download() {
	c.startDownload(false)
}

// DownloadAndInstall downloads and verifies the currently selected Release and
// immediately begins installation upon verification.
func (c *Controller) DownloadAndInstall() {
	c.startDownload(true)
}

func (c *Controller) startDownload(autoInstall bool) {
	rel := ggui.Peek(c.Release)
	if rel == nil {
		return
	}

	seq, ctx := c.beginOp()
	c.setFailedOp(failedNone, false)

	c.Downloaded.Set(nil)
	c.ErrText.Set("")
	c.Progress.Set(0)
	c.ProgressText.Set("Starting download…")
	c.Phase.Set(PhaseDownloading)
	c.Open.Set(true)

	u := c.updater
	go func() {
		const minUpdateInterval = 33 * time.Millisecond
		var lastReport time.Time
		var lastText string

		onProgress := func(p DownloadProgress) {
			if ctx.Err() != nil {
				return
			}
			now := time.Now()
			done := p.Total > 0 && p.Downloaded >= p.Total
			if !done && !lastReport.IsZero() && now.Sub(lastReport) < minUpdateInterval {
				return
			}
			lastReport = now

			frac := p.Fraction
			if frac < 0 {
				frac = 0
			} else if frac > 1 {
				frac = 1
			}
			text := formatProgressText(p)
			if done && text == lastText {
				return
			}
			lastText = text

			c.postUI(func() {
				if !c.isCurrentOp(seq) {
					return
				}
				c.Progress.Set(frac)
				c.ProgressText.Set(text)
			})
		}

		upd, err := u.Download(ctx, rel, onProgress)
		c.postUI(func() {
			if !c.isCurrentOp(seq) {
				return
			}
			c.clearOp(seq)
			if err != nil {
				if errors.Is(err, context.Canceled) && ctx.Err() != nil {
					return
				}
				c.setFailedOp(failedDownload, autoInstall)
				c.ErrText.Set(err.Error())
				c.Phase.Set(PhaseError)
				return
			}
			c.setFailedOp(failedNone, false)
			c.Progress.Set(1.0)
			if upd.Verified {
				c.ProgressText.Set("Signature verified.")
			} else {
				c.ProgressText.Set("Download complete.")
			}
			c.Downloaded.Set(upd)
			if autoInstall {
				c.Install()
				return
			}
			c.Phase.Set(PhaseReadyToInstall)
		})
	}()
}

// CancelDownload aborts an active download and returns to PhaseAvailable when a
// release is still selected (or PhaseIdle otherwise).
func (c *Controller) CancelDownload() {
	if ggui.Peek(c.Phase) != PhaseDownloading {
		return
	}
	c.cancelActiveOp()
	c.Progress.Set(0)
	c.ProgressText.Set("")
	c.ErrText.Set("")
	if ggui.Peek(c.Release) != nil {
		c.Phase.Set(PhaseAvailable)
		return
	}
	c.Phase.Set(PhaseIdle)
}

// Install hands the verified DownloadedUpdate to Updater.Apply in the
// background, transitioning to PhaseInstalling and reporting any error in
// PhaseError.
func (c *Controller) Install() {
	if ggui.Peek(c.Phase) == PhaseInstalling {
		return
	}
	upd := ggui.Peek(c.Downloaded)
	if upd == nil {
		return
	}

	c.mu.Lock()
	quit := c.quit
	opts := ApplyOptions{
		Relaunch: c.relaunch,
		Silent:   c.silentInstall,
	}
	c.mu.Unlock()
	if quit != nil {
		opts.QuitFunc = func() {
			c.postUI(quit)
		}
	}

	seq, ctx := c.beginOp()
	c.setFailedOp(failedNone, false)

	c.ErrText.Set("")
	c.Phase.Set(PhaseInstalling)
	c.Open.Set(true)

	u := c.updater
	go func() {
		err := u.Apply(ctx, upd, opts)
		c.postUI(func() {
			if !c.isCurrentOp(seq) {
				return
			}
			c.clearOp(seq)
			if err != nil {
				if errors.Is(err, context.Canceled) && ctx.Err() != nil {
					return
				}
				c.setFailedOp(failedInstall, false)
				c.ErrText.Set(err.Error())
				c.Phase.Set(PhaseError)
				c.Open.Set(true)
			}
		})
	}()
}

// Skip persists the current release's version as skipped, resets the controller
// to PhaseIdle, and closes the update UI.
func (c *Controller) Skip() {
	c.cancelActiveOp()
	if rel := ggui.Peek(c.Release); rel != nil {
		_ = c.updater.SkipVersion(rel.CompareVersion())
	}
	c.setFailedOp(failedNone, false)
	c.Release.Set(nil)
	c.Downloaded.Set(nil)
	c.Progress.Set(0)
	c.ProgressText.Set("")
	c.ErrText.Set("")
	c.Phase.Set(PhaseIdle)
	c.Open.Set(false)
}

// RemindLater dismisses the update prompt without skipping the release.
func (c *Controller) RemindLater() {
	c.Dismiss()
}

// Dismiss cancels any in-flight check or download and closes the update UI.
func (c *Controller) Dismiss() {
	p := ggui.Peek(c.Phase)
	if p == PhaseInstalling {
		return
	}
	if p == PhaseChecking || p == PhaseDownloading {
		c.cancelActiveOp()
		c.Progress.Set(0)
		c.ProgressText.Set("")
		if p == PhaseDownloading && ggui.Peek(c.Release) != nil {
			c.Phase.Set(PhaseAvailable)
		} else {
			c.Phase.Set(PhaseIdle)
		}
	} else if p == PhaseUpToDate || p == PhaseError {
		c.Phase.Set(PhaseIdle)
	}
	c.Open.Set(false)
}

// ToggleAutoCheck persists the automatic update check preference and updates
// the reactive AutoCheck state.
func (c *Controller) ToggleAutoCheck(enabled bool) {
	_ = c.updater.SetAutoCheckEnabled(enabled)
	c.AutoCheck.Set(enabled)
}

// Retry retries the most recently failed check, download, or install operation.
func (c *Controller) Retry() {
	op, autoInstall := c.getFailedOp()
	switch op {
	case failedCheck:
		c.CheckNow()
	case failedDownload:
		if autoInstall {
			c.DownloadAndInstall()
			return
		}
		c.Download()
	case failedInstall:
		c.Install()
	default:
		switch {
		case ggui.Peek(c.Downloaded) != nil:
			c.Install()
		case ggui.Peek(c.Release) != nil:
			c.Download()
		default:
			c.CheckNow()
		}
	}
}

func formatProgressText(p DownloadProgress) string {
	if p.Total > 0 && p.Downloaded >= p.Total {
		return "Verifying signature…"
	}
	if p.Total > 0 {
		return fmt.Sprintf("%s / %s", formatBytes(p.Downloaded), formatBytes(p.Total))
	}
	if p.Downloaded > 0 {
		return fmt.Sprintf("Downloaded %s", formatBytes(p.Downloaded))
	}
	return "Downloading…"
}

func formatBytes(n int64) string {
	if n < 0 {
		n = 0
	}
	const (
		kb = 1024.0
		mb = 1024.0 * 1024.0
		gb = 1024.0 * 1024.0 * 1024.0
	)
	f := float64(n)
	switch {
	case f >= gb:
		return fmt.Sprintf("%.2f GB", f/gb)
	case f >= mb:
		return fmt.Sprintf("%.1f MB", f/mb)
	case f >= kb:
		return fmt.Sprintf("%.1f KB", f/kb)
	default:
		return fmt.Sprintf("%d B", n)
	}
}
