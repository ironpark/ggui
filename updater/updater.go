package updater

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/ironpark/ggui"
)

const (
	// DefaultCheckInterval is the default cadence between automatic update checks.
	DefaultCheckInterval = 24 * time.Hour

	// DefaultMaxDownloadBytes is the default maximum size of a downloaded update archive (512 MiB).
	DefaultMaxDownloadBytes = 512 << 20

	// defaultFeedTimeout caps appcast XML fetch duration when Config.HTTPClient is nil.
	defaultFeedTimeout = 30 * time.Second

	windowsInstallManifestName = "install-manifest.json"
)

// Config configures an Updater.
type Config struct {
	// FeedURL is the Sparkle 2 appcast XML URL (required).
	// Must use HTTPS unless AllowInsecureHTTP is true.
	FeedURL string

	// PublicKey is the standard base64-encoded 32-byte Ed25519 public key
	// (Sparkle's SUPublicEDKey). Required unless AllowUnsigned is true.
	PublicKey string

	// CurrentVersion is the currently running application version (e.g. "1.2.0").
	CurrentVersion string

	// Channel selects an optional update channel (e.g. "beta").
	// Empty or "stable" receives only default-channel releases.
	// Non-default channels receive both default-channel and matching-channel releases.
	Channel string

	// AppID identifies the application for state and cache directories.
	// Defaults to ggui.AppID().
	AppID string

	// AppName is the display name of the application. Defaults to AppID.
	AppName string

	// OS overrides the detected host OS ("macos", "windows", "linux").
	// Defaults to normalized runtime.GOOS ("darwin" -> "macos").
	OS string

	// Arch overrides the detected host architecture ("amd64", "arm64", "386", ...).
	// Defaults to normalized runtime.GOARCH.
	Arch string

	// OSVersion overrides the detected host OS version (e.g. "14.5", "10.0.22631").
	// Defaults to currentOSVersion().
	OSVersion string

	// CheckInterval is the minimum interval between automatic background checks.
	// Defaults to DefaultCheckInterval (24h).
	CheckInterval time.Duration

	// MaxDownloadBytes is the maximum permitted update download size in bytes.
	// Defaults to DefaultMaxDownloadBytes (512 MiB).
	MaxDownloadBytes int64

	// StateDir overrides where updater-state.json is stored.
	// Defaults to <os.UserConfigDir()>/<sanitized AppID>.
	StateDir string

	// CacheDir overrides where downloaded update archives are stored.
	// Defaults to <os.UserCacheDir()>/<sanitized AppID>/updates.
	CacheDir string

	// UserAgent overrides the HTTP User-Agent header sent for feed fetches and downloads.
	UserAgent string

	// HTTPClient optionally overrides the HTTP client used for network requests.
	HTTPClient *http.Client

	// Applier optionally overrides the platform update applier used by Apply.
	// When nil, Apply uses the registered platform default applier (if any).
	Applier Applier

	// PreferWindowsSetup, when true on Windows, prefers installer executables
	// (e.g. *-setup.exe) over portable zip archives when multiple enclosures
	// match. When false, Updater auto-detects whether install-manifest.json
	// exists beside the running executable and prefers setup installers if so,
	// or portable zips otherwise (falling back to whichever is available).
	PreferWindowsSetup bool

	// AllowInsecureHTTP allows plain http:// URLs (for local development/tests).
	AllowInsecureHTTP bool

	// AllowUnsigned allows missing Ed25519 keys/signatures (for local development/tests).
	AllowUnsigned bool

	// Now optionally overrides time.Now for deterministic testing.
	Now func() time.Time
}

// Release represents an eligible update selected from an Appcast.
type Release struct {
	Item      Item
	Enclosure Enclosure
}

// DisplayVersion returns the user-facing version string (ShortVersion if set,
// otherwise Version).
func (r Release) DisplayVersion() string {
	if s := strings.TrimSpace(r.Item.ShortVersion); s != "" {
		return s
	}
	return strings.TrimSpace(r.Item.Version)
}

// CompareVersion returns the canonical version string used for ordering and
// skip comparisons (Version if set, otherwise ShortVersion).
func (r Release) CompareVersion() string {
	return itemCompareVersion(r.Item)
}

// IsCriticalFor reports whether this release is marked as a critical update
// for an installation running currentVersion.
func (r Release) IsCriticalFor(currentVersion string) bool {
	return isCriticalFor(r.Item, currentVersion)
}

// CheckOptions configures update filtering during CheckWithOptions and SelectRelease.
type CheckOptions struct {
	// Manual indicates a user-initiated check, bypassing phased rollout delays
	// and skipped-version suppression.
	Manual bool

	// IncludeSkipped includes versions previously skipped via SkipVersion.
	IncludeSkipped bool
}

// DownloadProgress reports streaming download progress.
type DownloadProgress struct {
	Downloaded int64
	Total      int64   // -1 when unknown
	Fraction   float64 // 0..1 when Total > 0, else 0
}

// DownloadedUpdate represents a downloaded update archive in CacheDir.
type DownloadedUpdate struct {
	Release  Release
	FilePath string
	Size     int64
	SHA256   string
	Verified bool
}

// ApplyOptions configures how an Applier installs and optionally relaunches an update.
type ApplyOptions struct {
	// Relaunch requests restarting the application after the update is installed.
	Relaunch bool

	// Silent requests unattended installation where supported (e.g. Windows setup exe).
	Silent bool

	// QuitFunc, if non-nil, is called right before handing off to the installer/helper
	// so the running application can shut down gracefully.
	QuitFunc func()

	// AppPath optionally overrides the target application bundle or executable path.
	AppPath string

	// ExtraArgs appends additional arguments for installer-based updates.
	ExtraArgs []string
}

// Applier installs a DownloadedUpdate on the host platform.
type Applier interface {
	Apply(ctx context.Context, upd *DownloadedUpdate, opts ApplyOptions) error
}

// defaultApplierFunc is registered by platform-specific apply*.go files in init().
var defaultApplierFunc func() Applier

// windowsInstalledFunc is overridden in tests to exercise Windows manifest detection.
var windowsInstalledFunc = isWindowsInstalledBesideExe

// Directory resolution hooks overridden in tests to exercise fallback paths.
var (
	userConfigDirFunc = os.UserConfigDir
	userCacheDirFunc  = os.UserCacheDir
	userHomeDirFunc   = os.UserHomeDir
)

// Updater checks a Sparkle 2 appcast feed, downloads and verifies updates,
// tracks persistent state, and hands verified updates to an Applier.
type Updater struct {
	cfg   Config
	state *stateStore
}

// New validates cfg, applies defaults, loads persistent state, and returns a ready Updater.
func New(cfg Config) (*Updater, error) {
	cfg.FeedURL = strings.TrimSpace(cfg.FeedURL)
	if cfg.FeedURL == "" {
		return nil, errors.New("updater: FeedURL is required")
	}
	if _, err := validateURLString(cfg.FeedURL, cfg.AllowInsecureHTTP); err != nil {
		return nil, err
	}

	cfg.PublicKey = strings.TrimSpace(cfg.PublicKey)
	if cfg.PublicKey == "" {
		if !cfg.AllowUnsigned {
			return nil, fmt.Errorf("%w: PublicKey is required unless AllowUnsigned is true", ErrMissingSignature)
		}
	} else {
		if _, err := ParsePublicKey(cfg.PublicKey); err != nil {
			return nil, err
		}
	}

	if cfg.OS == "" {
		cfg.OS = runtime.GOOS
	}
	cfg.OS, _ = normalizeOSArch(cfg.OS, "")

	if cfg.Arch == "" {
		cfg.Arch = runtime.GOARCH
	}
	cfg.Arch = normalizeArch(cfg.Arch)

	if cfg.OSVersion == "" {
		cfg.OSVersion = currentOSVersion()
	}

	if cfg.AppID == "" {
		cfg.AppID = ggui.AppID()
	}
	if cfg.AppName == "" {
		cfg.AppName = cfg.AppID
	}

	safeID := sanitizePathComponent(cfg.AppID)
	if safeID == "" {
		safeID = "ggui"
	}

	if cfg.StateDir == "" {
		if dir, err := userConfigDirFunc(); err == nil && strings.TrimSpace(dir) != "" {
			cfg.StateDir = filepath.Join(dir, safeID)
		} else if home, err := userHomeDirFunc(); err == nil && strings.TrimSpace(home) != "" {
			cfg.StateDir = filepath.Join(home, ".config", safeID)
			_ = os.MkdirAll(cfg.StateDir, 0o700)
		} else {
			cfg.StateDir = filepath.Join(os.TempDir(), safeID)
			_ = os.MkdirAll(cfg.StateDir, 0o700)
		}
	}
	if cfg.CacheDir == "" {
		if dir, err := userCacheDirFunc(); err == nil && strings.TrimSpace(dir) != "" {
			cfg.CacheDir = filepath.Join(dir, safeID, "updates")
		} else if home, err := userHomeDirFunc(); err == nil && strings.TrimSpace(home) != "" {
			cfg.CacheDir = filepath.Join(home, ".cache", safeID, "updates")
			_ = os.MkdirAll(cfg.CacheDir, 0o700)
		} else {
			cfg.CacheDir = filepath.Join(os.TempDir(), safeID, "updates")
			_ = os.MkdirAll(cfg.CacheDir, 0o700)
		}
	}

	if cfg.CheckInterval <= 0 {
		cfg.CheckInterval = DefaultCheckInterval
	}
	if cfg.MaxDownloadBytes <= 0 {
		cfg.MaxDownloadBytes = DefaultMaxDownloadBytes
	}
	if cfg.UserAgent == "" {
		ver := strings.TrimSpace(cfg.CurrentVersion)
		if ver == "" {
			ver = "0"
		}
		cfg.UserAgent = fmt.Sprintf("%s/%s (%s; %s)", cfg.AppName, ver, cfg.OS, cfg.Arch)
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}

	store, err := loadState(cfg.StateDir)
	if err != nil {
		return nil, err
	}

	return &Updater{
		cfg:   cfg,
		state: store,
	}, nil
}

// Config returns a copy of the resolved Updater configuration.
func (u *Updater) Config() Config {
	return u.cfg
}

// Check fetches the configured appcast feed and selects the best eligible update
// using default automatic check options.
func (u *Updater) Check(ctx context.Context) (*Release, bool, error) {
	return u.CheckWithOptions(ctx, CheckOptions{})
}

// CheckWithOptions fetches the configured appcast feed, updates LastCheckTime,
// and selects the best eligible update according to opts.
func (u *Updater) CheckWithOptions(ctx context.Context, opts CheckOptions) (*Release, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.cfg.FeedURL, nil)
	if err != nil {
		return nil, false, fmt.Errorf("updater: creating appcast request: %w", redactURLError(err))
	}
	if u.cfg.UserAgent != "" {
		req.Header.Set("User-Agent", u.cfg.UserAgent)
	}
	req.Header.Set("Accept", "application/rss+xml, application/xml, text/xml;q=0.9, */*;q=0.1")

	client := u.httpClient(defaultFeedTimeout)
	resp, err := client.Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("updater: fetching appcast: %w", redactURLError(err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("updater: appcast %s returned HTTP %d", redactURLString(u.cfg.FeedURL), resp.StatusCode)
	}

	feed, err := ParseAppcast(resp.Body)
	if err != nil {
		return nil, false, err
	}

	_ = u.state.setLastCheck(u.now())

	rel, ok := u.SelectRelease(feed, opts)
	return rel, ok, nil
}

// SelectRelease filters feed according to channel, version, minimum OS version,
// skipped version, phased rollout, and platform enclosure matching, returning
// the highest-version eligible release.
func (u *Updater) SelectRelease(feed *Appcast, opts CheckOptions) (*Release, bool) {
	if feed == nil || len(feed.Items) == 0 {
		return nil, false
	}

	wantChannel := normalizeChannel(u.cfg.Channel)
	skipped := strings.TrimSpace(u.SkippedVersion())
	now := u.now()
	bucket := u.state.rolloutSeed()
	preferSetup := u.cfg.PreferWindowsSetup || (u.cfg.OS == "windows" && windowsInstalledFunc())

	var best *Release
	bestScore := -1

	for _, item := range feed.Items {
		// 1. Channel filter: default/stable only sees default items;
		// non-default channel sees default OR matching channel.
		itemChannel := normalizeChannel(item.Channel)
		if wantChannel == "" {
			if itemChannel != "" {
				continue
			}
		} else if itemChannel != "" && !strings.EqualFold(itemChannel, wantChannel) {
			continue
		}

		// 2. Version strictly greater than CurrentVersion.
		ver := itemCompareVersion(item)
		if ver == "" {
			continue
		}
		if CompareVersion(ver, u.cfg.CurrentVersion) <= 0 {
			continue
		}

		// 3. Minimum system version check.
		if minOS := strings.TrimSpace(item.MinimumSystemVersion); minOS != "" && u.cfg.OSVersion != "" {
			if CompareVersion(u.cfg.OSVersion, minOS) < 0 {
				continue
			}
		}

		critical := isCriticalFor(item, u.cfg.CurrentVersion)

		// 4. Skipped version check (bypassed by manual, IncludeSkipped, or critical update).
		if !opts.Manual && !opts.IncludeSkipped && !critical && skipped != "" {
			if CompareVersion(ver, skipped) <= 0 {
				continue
			}
		}

		// 5. Phased rollout check (bypassed by manual or critical update).
		if !opts.Manual && !critical {
			if !isRolloutEligible(now, item.PubDate, item.PhasedRolloutInterval, bucket) {
				continue
			}
		}

		// 6. Matching enclosure for host OS & Arch.
		enc, score, ok := selectBestEnclosure(item.Enclosures, u.cfg.OS, u.cfg.Arch, preferSetup)
		if !ok {
			continue
		}

		if best == nil {
			cand := Release{Item: item, Enclosure: enc}
			best = &cand
			bestScore = score
			continue
		}

		cmp := CompareVersion(ver, best.CompareVersion())
		if cmp > 0 || (cmp == 0 && score > bestScore) {
			cand := Release{Item: item, Enclosure: enc}
			best = &cand
			bestScore = score
		}
	}

	if best == nil {
		return nil, false
	}
	return best, true
}

// Apply verifies that upd has been verified and re-verifies the downloaded file
// on disk against the configured Ed25519 public key (and optional SHA-256
// digest) unless AllowUnsigned is set, then delegates installation to
// Config.Applier or the registered platform default applier.
func (u *Updater) Apply(ctx context.Context, upd *DownloadedUpdate, opts ApplyOptions) error {
	if upd == nil || strings.TrimSpace(upd.FilePath) == "" {
		return errors.New("updater: invalid downloaded update")
	}
	if !u.cfg.AllowUnsigned {
		if !upd.Verified {
			return fmt.Errorf("%w: refusing to apply unverified update", ErrInvalidSignature)
		}
		if err := VerifyFile(u.cfg.PublicKey, upd.FilePath, upd.Release.Enclosure.EdSignature); err != nil {
			return err
		}
		if wantSHA := normalizeSHA256(upd.Release.Enclosure.SHA256); wantSHA != "" {
			gotSHA, err := sha256File(upd.FilePath)
			if err != nil {
				return fmt.Errorf("updater: hashing update file for verification: %w", err)
			}
			if subtle.ConstantTimeCompare([]byte(gotSHA), []byte(wantSHA)) != 1 {
				return fmt.Errorf("%w: got %s, want %s", ErrHashMismatch, gotSHA, wantSHA)
			}
		}
	}

	applier := u.cfg.Applier
	if applier == nil && defaultApplierFunc != nil {
		applier = defaultApplierFunc()
	}
	if applier == nil {
		return ErrUnsupportedPlatform
	}
	return applier.Apply(ctx, upd, opts)
}

// SkipVersion records version as skipped so automatic checks will not prompt for it.
func (u *Updater) SkipVersion(version string) error {
	return u.state.setSkippedVersion(strings.TrimSpace(version))
}

// SkippedVersion returns the currently skipped version string, or "" if none.
func (u *Updater) SkippedVersion() string {
	return u.state.skippedVersion()
}

// ClearSkippedVersion clears any previously skipped version.
func (u *Updater) ClearSkippedVersion() error {
	return u.state.setSkippedVersion("")
}

// AutoCheckEnabled reports whether automatic background update checks are enabled.
func (u *Updater) AutoCheckEnabled() bool {
	return u.state.autoCheck()
}

// SetAutoCheckEnabled enables or disables automatic background update checks.
func (u *Updater) SetAutoCheckEnabled(enabled bool) error {
	return u.state.setAutoCheck(enabled)
}

// LastCheckTime returns the timestamp of the last successful appcast check.
func (u *Updater) LastCheckTime() time.Time {
	return u.state.lastCheck()
}

// ShouldAutoCheck reports whether an automatic check is currently due.
func (u *Updater) ShouldAutoCheck() bool {
	if !u.AutoCheckEnabled() {
		return false
	}
	last := u.LastCheckTime()
	if last.IsZero() {
		return true
	}
	return u.now().Sub(last) >= u.cfg.CheckInterval
}

// StartBackground launches a background goroutine that periodically runs
// automatic update checks when ShouldAutoCheck is true and invokes onUpdate
// when an update is found. Call the returned stop function (or cancel ctx) to
// stop the loop and wait for the goroutine to exit cleanly.
func (u *Updater) StartBackground(ctx context.Context, onUpdate func(*Release)) (stop func()) {
	bgCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})

	go func() {
		defer close(done)

		runOnce := func() {
			if bgCtx.Err() != nil || !u.ShouldAutoCheck() {
				return
			}
			rel, ok, err := u.Check(bgCtx)
			if err == nil && ok && bgCtx.Err() == nil && onUpdate != nil {
				onUpdate(rel)
			}
		}

		// Initial delay: run immediately if already due, otherwise wait remaining interval
		// clamped to CheckInterval to guard against backward system clock jumps.
		initialDelay := u.initialBackgroundDelay()

		if initialDelay == 0 {
			runOnce()
		} else {
			timer := time.NewTimer(initialDelay)
			select {
			case <-bgCtx.Done():
				timer.Stop()
				return
			case <-timer.C:
				runOnce()
			}
		}

		ticker := time.NewTicker(u.cfg.CheckInterval)
		defer ticker.Stop()

		for {
			select {
			case <-bgCtx.Done():
				return
			case <-ticker.C:
				runOnce()
			}
		}
	}()

	var once sync.Once
	return func() {
		once.Do(func() {
			cancel()
			<-done
		})
	}
}

func (u *Updater) initialBackgroundDelay() time.Duration {
	if !u.AutoCheckEnabled() {
		return u.cfg.CheckInterval
	}
	last := u.LastCheckTime()
	if last.IsZero() {
		return 0
	}
	rem := u.cfg.CheckInterval - u.now().Sub(last)
	if rem <= 0 {
		return 0
	}
	if rem > u.cfg.CheckInterval {
		return u.cfg.CheckInterval
	}
	return rem
}

func (u *Updater) now() time.Time {
	return u.cfg.Now()
}

func itemCompareVersion(item Item) string {
	if s := strings.TrimSpace(item.Version); s != "" {
		return s
	}
	return strings.TrimSpace(item.ShortVersion)
}

func normalizeChannel(ch string) string {
	ch = strings.TrimSpace(strings.ToLower(ch))
	if ch == "" || ch == "stable" || ch == "default" {
		return ""
	}
	return ch
}

func isCriticalFor(item Item, currentVersion string) bool {
	if !item.CriticalUpdate {
		return false
	}
	below := strings.TrimSpace(item.CriticalBelowVersion)
	if below == "" {
		return true
	}
	return CompareVersion(currentVersion, below) < 0
}

func selectBestEnclosure(enclosures []Enclosure, hostOS, hostArch string, preferWinSetup bool) (Enclosure, int, bool) {
	var best Enclosure
	bestScore := -1
	for _, enc := range enclosures {
		score, ok := scoreEnclosure(enc, hostOS, hostArch, preferWinSetup)
		if !ok {
			continue
		}
		if score > bestScore {
			best = enc
			bestScore = score
		}
	}
	if bestScore < 0 {
		return Enclosure{}, 0, false
	}
	return best, bestScore, true
}

func scoreEnclosure(enc Enclosure, hostOS, hostArch string, preferWinSetup bool) (int, bool) {
	if strings.TrimSpace(enc.URL) == "" {
		return 0, false
	}
	encOS, encArch := normalizeOSArch(enc.OS, enc.Arch)
	if encOS != hostOS {
		return 0, false
	}

	archScore := 0
	switch {
	case encArch != "" && encArch == hostArch:
		archScore = 30
	case encArch == "universal" && hostOS == "macos":
		archScore = 20
	case encArch == "":
		archScore = 10
	default:
		return 0, false
	}

	pkgScore := 0
	if hostOS == "windows" {
		pkgScore = scoreWindowsPackage(enc.URL, preferWinSetup)
	}
	return archScore + pkgScore, true
}

func scoreWindowsPackage(rawURL string, preferSetup bool) int {
	name := ""
	if u, err := url.Parse(rawURL); err == nil {
		name = strings.ToLower(path.Base(u.Path))
	} else {
		name = strings.ToLower(rawURL)
	}

	isSetupExe := strings.HasSuffix(name, "-setup.exe") || strings.HasSuffix(name, "_setup.exe") || name == "setup.exe"
	isInstaller := strings.HasSuffix(name, ".exe") || strings.HasSuffix(name, ".msi") || strings.HasSuffix(name, ".msix")
	isZip := strings.HasSuffix(name, ".zip")

	if preferSetup {
		switch {
		case isSetupExe:
			return 4
		case isInstaller:
			return 3
		case isZip:
			return 1
		default:
			return 2
		}
	}

	switch {
	case isZip:
		return 4
	case isSetupExe:
		return 2
	case isInstaller:
		return 1
	default:
		return 2
	}
}

func isWindowsInstalledBesideExe() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	info, err := os.Stat(filepath.Join(filepath.Dir(exe), windowsInstallManifestName))
	return err == nil && !info.IsDir()
}
