package installer

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/runtime"
)

// Context holds the live reactive state, metadata, navigation controls, and
// system helpers for a running installer or uninstaller session.
//
// Custom steps, Header, BuildUI, and lifecycle hooks all receive *Context.
// Reactive fields may be bound directly to ggui/ui widgets. Inside lifecycle
// hooks (which run on a background goroutine), read reactive values with
// ggui.Peek and report progress via ReportProgress or Post.
type Context struct {
	InstallDir      *ggui.StateValue[string]
	StartMenu       *ggui.StateValue[bool]
	Desktop         *ggui.StateValue[bool]
	LaunchAfter     *ggui.StateValue[bool]
	LicenseAccepted *ggui.StateValue[bool]
	Progress        *ggui.StateValue[float64]
	Status          *ggui.StateValue[string]
	ErrText         *ggui.StateValue[string]
	StepIndex       *ggui.StateValue[int]
	Failed          *ggui.StateValue[bool]

	app         *App
	payload     *Payload
	backend     Backend
	opts        CLIOptions
	isPreview   bool
	isUpgrade   bool
	prevVersion string
	steps       []Step
	iconTex     *ggfx.Image

	post     func(func())
	dialogs  runtime.Dialogs
	closeWin func()

	optMu       sync.Mutex
	boolOpts    map[string]*ggui.StateValue[bool]
	stringOpts  map[string]*ggui.StateValue[string]
	boolSnap    map[string]bool
	stringSnap  map[string]string

	txMu     sync.Mutex
	activeTx *installTx
	inWorker atomic.Bool
	running  atomic.Bool
}

// NewContext constructs a Context for app, CLI args, and backend b (or the
// platform/preview default when b is nil). Useful for headless testing with
// ggui.Probe or embedding the installer in custom tooling.
func NewContext(app *App, args []string, b Backend) (*Context, error) {
	if app == nil {
		app = &App{}
	}
	opts, err := ParseArgs(args)
	if err != nil {
		return nil, err
	}
	payload, isPreview, err := resolvePayload(app)
	if err != nil {
		return nil, err
	}
	if b == nil {
		if isPreview {
			b = NewPreviewBackend()
		} else {
			b = defaultBackend()
		}
	}
	targetDir := resolveInstallDir(payload.Meta, opts, b)
	if opts.Uninstall {
		targetDir = resolveUninstallDir(payload.Meta, opts, b)
	}
	return newContext(app, payload, opts, targetDir, isPreview, b), nil
}

func newContext(app *App, payload *Payload, opts CLIOptions, targetDir string, isPreview bool, b Backend) *Context {
	meta := payload.Meta
	iopts := resolveInstallOptions(meta, opts, targetDir)
	launchAfter := meta.LaunchAfter
	if opts.LaunchSet {
		launchAfter = !opts.NoLaunch
	}
	_, prevVer, isUpgrade := b.ExistingInstall(meta)

	var steps []Step
	if opts.Uninstall {
		if len(app.UninstallSteps) > 0 {
			steps = append([]Step(nil), app.UninstallSteps...)
		} else {
			steps = DefaultUninstallSteps()
		}
	} else {
		if len(app.Steps) > 0 {
			steps = append([]Step(nil), app.Steps...)
		} else {
			steps = DefaultSteps()
		}
	}

	c := &Context{
		InstallDir:      ggui.State(iopts.InstallDir),
		StartMenu:       ggui.State(iopts.StartMenuShortcut),
		Desktop:         ggui.State(iopts.DesktopShortcut),
		LaunchAfter:     ggui.State(launchAfter),
		LicenseAccepted: ggui.State(false),
		Progress:        ggui.State(0.0),
		Status:          ggui.State(""),
		ErrText:         ggui.State(""),
		StepIndex:       ggui.State(0),
		Failed:          ggui.State(false),
		app:             app,
		payload:         payload,
		backend:         b,
		opts:            opts,
		isPreview:       isPreview,
		isUpgrade:       isUpgrade,
		prevVersion:     prevVer,
		steps:           steps,
	}
	if first := c.firstActiveStep(); first >= 0 {
		c.StepIndex.Set(first)
	}
	return c
}

// Meta returns the resolved application metadata.
func (c *Context) Meta() Metadata { return c.payload.Meta }

// Payload returns the decoded installer archive.
func (c *Context) Payload() *Payload { return c.payload }

// Backend returns the active OS/preview backend.
func (c *Context) Backend() Backend { return c.backend }

// CLIOptions returns the parsed command-line options.
func (c *Context) CLIOptions() CLIOptions { return c.opts }

// IsUninstall reports whether the session is running in uninstall mode.
func (c *Context) IsUninstall() bool { return c.opts.Uninstall }

// IsUpgrade reports whether an existing installation of this app was detected.
func (c *Context) IsUpgrade() bool { return c.isUpgrade }

// PreviousVersion returns the version string of an existing installation, or "".
func (c *Context) PreviousVersion() string { return c.prevVersion }

// IsPreview reports whether the installer is running without an embedded
// payload (for example during `ggui dev`).
func (c *Context) IsPreview() bool { return c.isPreview }

// Dir returns the current install or uninstall directory path. During an active
// install/uninstall transaction (inside hooks), it returns the resolved
// absolute directory.
func (c *Context) Dir() string {
	c.txMu.Lock()
	tx := c.activeTx
	c.txMu.Unlock()
	if tx != nil && tx.absDir != "" {
		return tx.absDir
	}
	return strings.TrimSpace(ggui.Peek(c.InstallDir))
}

// Icon returns the lazily created GPU image for the embedded app icon, or nil.
func (c *Context) Icon() *ggfx.Image {
	if c.iconTex == nil && c.payload != nil && c.payload.Icon != nil {
		c.iconTex = ggfx.NewImageFromImage(c.payload.Icon)
	}
	return c.iconTex
}

// Steps returns a copy of the active wizard step list.
func (c *Context) Steps() []Step { return append([]Step(nil), c.steps...) }

// CurrentStep returns the currently displayed step.
func (c *Context) CurrentStep() (Step, bool) {
	idx := ggui.Peek(c.StepIndex)
	if idx < 0 || idx >= len(c.steps) {
		return Step{}, false
	}
	return c.steps[idx], true
}

func (c *Context) isSkipped(idx int) bool {
	if idx < 0 || idx >= len(c.steps) {
		return true
	}
	return c.steps[idx].Skip != nil && c.steps[idx].Skip(c)
}

func (c *Context) firstActiveStep() int {
	for i := range c.steps {
		if !c.isSkipped(i) {
			return i
		}
	}
	return -1
}

func (c *Context) prevActiveIndex(from int) int {
	for i := from - 1; i >= 0; i-- {
		if !c.isSkipped(i) && c.steps[i].Kind == StepNormal {
			return i
		}
	}
	return -1
}

func (c *Context) nextActiveIndex(from int) int {
	for i := from + 1; i < len(c.steps); i++ {
		if !c.isSkipped(i) {
			return i
		}
	}
	return -1
}

func (c *Context) findStepKind(kind StepKind) int {
	for i, s := range c.steps {
		if s.Kind == kind && !c.isSkipped(i) {
			return i
		}
	}
	return -1
}

// HasPrev reports whether there is a previous navigable step before the current one.
func (c *Context) HasPrev() bool {
	return c.prevActiveIndex(ggui.Peek(c.StepIndex)) >= 0
}

// HasNext reports whether there is a subsequent step after the current one.
func (c *Context) HasNext() bool {
	return c.nextActiveIndex(ggui.Peek(c.StepIndex)) >= 0
}

// NextIsProgress reports whether advancing from the current step enters a
// StepProgress step (used to label the primary button "Install"/"Update").
func (c *Context) NextIsProgress() bool {
	next := c.nextActiveIndex(ggui.Peek(c.StepIndex))
	return next >= 0 && c.steps[next].Kind == StepProgress
}

func (c *Context) enterStep(idx int) {
	if idx < 0 || idx >= len(c.steps) {
		return
	}
	c.Failed.Set(false)
	c.StepIndex.Set(idx)
	if onEnter := c.steps[idx].OnEnter; onEnter != nil {
		onEnter(c)
	}
}

// Next advances to the next non-skipped wizard step.
func (c *Context) Next() {
	if next := c.nextActiveIndex(ggui.Peek(c.StepIndex)); next >= 0 {
		c.enterStep(next)
	}
}

// Back returns to the previous non-skipped wizard step.
func (c *Context) Back() {
	if prev := c.prevActiveIndex(ggui.Peek(c.StepIndex)); prev >= 0 {
		c.enterStep(prev)
	}
}

// GoTo jumps to the step with the given ID, returning false if not found.
func (c *Context) GoTo(id string) bool {
	for i, s := range c.steps {
		if s.ID == id {
			c.enterStep(i)
			return true
		}
	}
	return false
}

// Post schedules fn to run on the UI goroutine (or runs fn directly when no
// GUI event loop is attached).
func (c *Context) Post(fn func()) {
	if fn == nil {
		return
	}
	if c.post != nil {
		c.post(fn)
		return
	}
	fn()
}

// Close closes the installer window.
func (c *Context) Close() {
	if c.closeWin != nil {
		c.closeWin()
	}
}

// Cancel closes the installer window without completing the installation.
func (c *Context) Cancel() { c.Close() }

// BrowseFolder opens a native folder picker and updates InstallDir.
func (c *Context) BrowseFolder() {
	if c.dialogs == nil {
		return
	}
	cur := ggui.Peek(c.InstallDir)
	go func() {
		picked, err := c.dialogs.PickFolder(runtime.FileDialog{
			Title:     "Choose install folder",
			Directory: cur,
		})
		if err != nil || strings.TrimSpace(picked) == "" {
			return
		}
		if !strings.EqualFold(filepath.Base(picked), c.payload.Meta.Name) {
			picked = filepath.Join(picked, c.payload.Meta.Name)
		}
		c.Post(func() { c.InstallDir.Set(picked) })
	}()
}

// StartInstall begins installation asynchronously, updating Progress, Status,
// ErrText, and advancing to the progress and finish steps.
func (c *Context) StartInstall() {
	if !c.running.CompareAndSwap(false, true) {
		return
	}
	dir := strings.TrimSpace(ggui.Peek(c.InstallDir))
	if dir == "" {
		c.running.Store(false)
		c.ErrText.Set("Please choose an install location.")
		c.Failed.Set(true)
		return
	}
	iopts := InstallOptions{
		InstallDir:        dir,
		StartMenuShortcut: ggui.Peek(c.StartMenu),
		DesktopShortcut:   ggui.Peek(c.Desktop),
	}
	c.Progress.Set(0)
	c.Status.Set("Preparing installation…")
	c.ErrText.Set("")
	c.Failed.Set(false)
	c.snapshotOptions()
	if pi := c.findStepKind(StepProgress); pi >= 0 && ggui.Peek(c.StepIndex) != pi {
		c.StepIndex.Set(pi)
	}

	go func() {
		c.inWorker.Store(true)
		defer func() {
			c.inWorker.Store(false)
			c.running.Store(false)
		}()
		err := install(context.Background(), c.payload, iopts, c.backend, c, func(p Progress) {
			c.Post(func() {
				c.Progress.Set(p.Fraction)
				c.Status.Set(p.Message)
			})
		})
		c.Post(func() {
			if err != nil {
				c.ErrText.Set(err.Error())
				c.Failed.Set(true)
				return
			}
			c.Progress.Set(1.0)
			c.Status.Set("Installation complete.")
			if fi := c.findStepKind(StepFinish); fi >= 0 {
				c.enterStep(fi)
			}
		})
	}()
}

// StartUninstall begins uninstallation asynchronously, updating Progress,
// Status, ErrText, and advancing to the progress and finish steps.
func (c *Context) StartUninstall() {
	if !c.running.CompareAndSwap(false, true) {
		return
	}
	dir := strings.TrimSpace(ggui.Peek(c.InstallDir))
	c.Progress.Set(0)
	c.Status.Set("Removing files…")
	c.ErrText.Set("")
	c.Failed.Set(false)
	c.snapshotOptions()
	if pi := c.findStepKind(StepProgress); pi >= 0 && ggui.Peek(c.StepIndex) != pi {
		c.StepIndex.Set(pi)
	}

	go func() {
		c.inWorker.Store(true)
		defer func() {
			c.inWorker.Store(false)
			c.running.Store(false)
		}()
		err := uninstall(context.Background(), c.payload.Meta, dir, c.backend, c, func(p Progress) {
			c.Post(func() {
				c.Progress.Set(p.Fraction)
				c.Status.Set(p.Message)
			})
		})
		c.Post(func() {
			if err != nil {
				c.ErrText.Set(err.Error())
				c.Failed.Set(true)
				return
			}
			c.Progress.Set(1.0)
			c.Status.Set("Uninstall complete.")
			if fi := c.findStepKind(StepFinish); fi >= 0 {
				c.enterStep(fi)
			}
		})
	}()
}

// Retry retries the failed install or uninstall operation.
func (c *Context) Retry() {
	if c.IsUninstall() {
		c.StartUninstall()
		return
	}
	c.StartInstall()
}

// Finish launches the installed app if LaunchAfter is checked (during install)
// and closes the installer window.
func (c *Context) Finish() {
	if !c.IsUninstall() && ggui.Peek(c.LaunchAfter) {
		exe := filepath.Join(ggui.Peek(c.InstallDir), filepath.FromSlash(c.payload.Meta.Executable))
		_ = c.backend.Launch(exe)
	}
	c.Close()
}

func (c *Context) beginTx(tx *installTx) {
	c.txMu.Lock()
	c.activeTx = tx
	c.txMu.Unlock()
}

func (c *Context) endTx() {
	c.txMu.Lock()
	c.activeTx = nil
	c.txMu.Unlock()
}

// ReportProgress updates the active progress bar and status message safely
// from inside BeforeInstall, AfterInstall, BeforeUninstall, or AfterUninstall.
func (c *Context) ReportProgress(fraction float64, message string) {
	c.txMu.Lock()
	tx := c.activeTx
	c.txMu.Unlock()
	if tx != nil && tx.report != nil {
		tx.report(fraction, message)
		return
	}
	c.Post(func() {
		c.Progress.Set(fraction)
		c.Status.Set(message)
	})
}

// WriteFile writes a file atomically inside the install directory (at
// slash-separated relPath) and records it in install-manifest.json and the
// active rollback set so Uninstall removes it cleanly.
func (c *Context) WriteFile(relPath string, data []byte, mode os.FileMode) error {
	clean := path.Clean(strings.ReplaceAll(relPath, "\\", "/"))
	if !validRelPath(clean) {
		return fmt.Errorf("installer: invalid relative file path %q", relPath)
	}
	root := c.Dir()
	if root == "" {
		return fmt.Errorf("installer: install directory is empty")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	dst := filepath.Join(absRoot, filepath.FromSlash(clean))
	if !isSubpath(absRoot, dst) {
		return fmt.Errorf("installer: file %q escapes install directory", relPath)
	}
	existed := fileExists(dst)
	if err := writeFileAtomic(dst, data, mode); err != nil {
		return err
	}
	c.txMu.Lock()
	if c.activeTx != nil {
		if !existed && c.activeTx.createdFiles != nil {
			*c.activeTx.createdFiles = append(*c.activeTx.createdFiles, dst)
		}
		if c.activeTx.installedRel != nil {
			*c.activeTx.installedRel = appendUnique(*c.activeTx.installedRel, clean)
		}
	}
	c.txMu.Unlock()
	return nil
}

// TrackFile records an existing file (relative to the install directory) in
// install-manifest.json so Uninstall removes it cleanly.
func (c *Context) TrackFile(relPath string) error {
	clean := path.Clean(strings.ReplaceAll(relPath, "\\", "/"))
	if !validRelPath(clean) {
		return fmt.Errorf("installer: invalid relative file path %q", relPath)
	}
	c.txMu.Lock()
	if c.activeTx != nil && c.activeTx.installedRel != nil {
		*c.activeTx.installedRel = appendUnique(*c.activeTx.installedRel, clean)
	}
	c.txMu.Unlock()
	return nil
}

// TrackShortcut records a shortcut path in install-manifest.json so Uninstall
// removes it cleanly.
func (c *Context) TrackShortcut(lnkPath string) {
	if strings.TrimSpace(lnkPath) == "" {
		return
	}
	c.txMu.Lock()
	if c.activeTx != nil && c.activeTx.createdShortcuts != nil {
		*c.activeTx.createdShortcuts = appendUnique(*c.activeTx.createdShortcuts, lnkPath)
	}
	c.txMu.Unlock()
}

// CreateShortcut creates a Windows .lnk shortcut via the active backend and
// records it in install-manifest.json (and the active rollback set).
func (c *Context) CreateShortcut(lnkPath, targetPath, workDir, desc, iconPath string) error {
	if err := c.backend.CreateShortcut(lnkPath, targetPath, workDir, desc, iconPath); err != nil {
		return err
	}
	c.TrackShortcut(lnkPath)
	return nil
}

// EnvScope returns the installer's normalized scope (ScopeUser or ScopeMachine),
// which determines whether environment variable and PATH helpers target HKCU or HKLM.
func (c *Context) EnvScope() string {
	return c.payload.Meta.NormalizedScope()
}

// RegistryRoot returns RegistryHKLM when Scope is ScopeMachine, else RegistryHKCU.
func (c *Context) RegistryRoot() RegistryRoot {
	return defaultRegistryRoot(c.payload.Meta.NormalizedScope())
}

// GetEnv reads an environment variable from the installer's scope
// (HKCU\Environment for ScopeUser, HKLM\...\Environment for ScopeMachine).
func (c *Context) GetEnv(name string) (string, bool, error) {
	return c.GetEnvForScope(c.EnvScope(), name)
}

// GetEnvForScope reads an environment variable from the specified scope
// (ScopeUser or ScopeMachine).
func (c *Context) GetEnvForScope(scope, name string) (string, bool, error) {
	return c.backend.GetEnv(normalizeEnvScope(scope), name)
}

// SetEnv sets an environment variable in the installer's scope, broadcasts
// WM_SETTINGCHANGE on Windows, and records it in install-manifest.json (and
// the active rollback stack) so Uninstall removes it automatically.
func (c *Context) SetEnv(name, value string) error {
	return c.SetEnvForScope(c.EnvScope(), name, value)
}

// SetEnvForScope sets an environment variable in the specified scope
// (ScopeUser or ScopeMachine) and tracks it in install-manifest.json and rollback.
func (c *Context) SetEnvForScope(scope, name, value string) error {
	s := normalizeEnvScope(scope)
	n, err := validateEnvName(name)
	if err != nil {
		return err
	}
	prev, hadPrev, _ := c.backend.GetEnv(s, n)
	if err := c.backend.SetEnv(s, n, value); err != nil {
		return err
	}
	c.txMu.Lock()
	if c.activeTx != nil {
		if c.activeTx.rollbackFns != nil {
			b := c.backend
			*c.activeTx.rollbackFns = append(*c.activeTx.rollbackFns, func() {
				if hadPrev {
					_ = b.SetEnv(s, n, prev)
				} else {
					_ = b.DeleteEnv(s, n)
				}
			})
		}
		if c.activeTx.envEntries != nil {
			*c.activeTx.envEntries = appendEnvUnique(*c.activeTx.envEntries, ManifestEnvEntry{Scope: s, Name: n})
		}
	}
	c.txMu.Unlock()
	return nil
}

// UnsetEnv removes an environment variable from the installer's scope.
func (c *Context) UnsetEnv(name string) error {
	return c.UnsetEnvForScope(c.EnvScope(), name)
}

// UnsetEnvForScope removes an environment variable from the specified scope.
func (c *Context) UnsetEnvForScope(scope, name string) error {
	return c.backend.DeleteEnv(normalizeEnvScope(scope), name)
}

// AddToPath appends dir (defaulting to ctx.Dir() when empty) to PATH in the
// installer's scope (user or machine) and tracks it in install-manifest.json
// and rollback so Uninstall removes it automatically.
func (c *Context) AddToPath(dir string) error {
	return c.addToPathScope(c.EnvScope(), dir)
}

// RemoveFromPath removes dir (defaulting to ctx.Dir() when empty) from PATH in
// the installer's scope (user or machine).
func (c *Context) RemoveFromPath(dir string) error {
	return c.removeFromPathScope(c.EnvScope(), dir)
}

// AddToUserPath appends dir (defaulting to ctx.Dir() when empty) to the
// current user's PATH (HKCU\Environment) and tracks it for rollback/uninstall.
func (c *Context) AddToUserPath(dir string) error {
	return c.addToPathScope(ScopeUser, dir)
}

// RemoveFromUserPath removes dir (defaulting to ctx.Dir() when empty) from the
// current user's PATH (HKCU\Environment).
func (c *Context) RemoveFromUserPath(dir string) error {
	return c.removeFromPathScope(ScopeUser, dir)
}

// AddToMachinePath appends dir (defaulting to ctx.Dir() when empty) to the
// system PATH (HKLM\...\Environment, requires elevation) and tracks it for
// rollback/uninstall.
func (c *Context) AddToMachinePath(dir string) error {
	return c.addToPathScope(ScopeMachine, dir)
}

// RemoveFromMachinePath removes dir (defaulting to ctx.Dir() when empty) from
// the system PATH (HKLM\...\Environment).
func (c *Context) RemoveFromMachinePath(dir string) error {
	return c.removeFromPathScope(ScopeMachine, dir)
}

func (c *Context) addToPathScope(scope, dir string) error {
	if dir == "" {
		dir = c.Dir()
	}
	s := normalizeEnvScope(scope)
	if err := c.backend.AddToPath(s, dir); err != nil {
		return err
	}
	c.txMu.Lock()
	if c.activeTx != nil {
		if c.activeTx.rollbackFns != nil {
			b := c.backend
			*c.activeTx.rollbackFns = append(*c.activeTx.rollbackFns, func() {
				_ = b.RemoveFromPath(s, dir)
			})
		}
		if c.activeTx.pathEntries != nil {
			*c.activeTx.pathEntries = appendPathUnique(*c.activeTx.pathEntries, ManifestPathEntry{Scope: s, Dir: dir})
		}
	}
	c.txMu.Unlock()
	return nil
}

func (c *Context) removeFromPathScope(scope, dir string) error {
	if dir == "" {
		dir = c.Dir()
	}
	return c.backend.RemoveFromPath(normalizeEnvScope(scope), dir)
}

// GetRegistryValue reads a typed value from the Windows Registry.
func (c *Context) GetRegistryValue(root RegistryRoot, keyPath, name string) (RegistryValue, bool, error) {
	if root == "" {
		root = c.RegistryRoot()
	}
	return c.backend.GetRegistryValue(root, keyPath, name)
}

// SetRegistryValue writes a typed value to the Windows Registry and records it
// in install-manifest.json and the active rollback stack so Uninstall removes
// it cleanly (and deletes the key only if newly created and empty).
func (c *Context) SetRegistryValue(root RegistryRoot, keyPath, name string, val RegistryValue) error {
	if root == "" {
		root = c.RegistryRoot()
	}
	nr, err := normalizeRegistryRoot(root)
	if err != nil {
		return err
	}
	kp, err := normalizeRegistryKey(keyPath)
	if err != nil {
		return err
	}
	v, err := normalizeRegistryValue(val)
	if err != nil {
		return err
	}
	keyExisted, _ := c.backend.RegistryKeyExists(nr, kp)
	prevVal, hadPrev, _ := c.backend.GetRegistryValue(nr, kp, name)
	if err := c.backend.SetRegistryValue(nr, kp, name, v); err != nil {
		return err
	}
	c.txMu.Lock()
	if c.activeTx != nil {
		if c.activeTx.rollbackFns != nil {
			b := c.backend
			*c.activeTx.rollbackFns = append(*c.activeTx.rollbackFns, func() {
				if hadPrev {
					_ = b.SetRegistryValue(nr, kp, name, prevVal)
				} else {
					_ = b.DeleteRegistryValue(nr, kp, name)
					if !keyExisted {
						_ = b.DeleteRegistryKey(nr, kp, true)
					}
				}
			})
		}
		if c.activeTx.regEntries != nil {
			*c.activeTx.regEntries = appendRegUnique(*c.activeTx.regEntries, ManifestRegistryEntry{
				Root:      nr,
				Key:       kp,
				Name:      name,
				DeleteKey: !keyExisted,
			})
		}
	}
	c.txMu.Unlock()
	return nil
}

// SetRegistryString writes a REG_SZ string value (empty name sets the key's default value).
func (c *Context) SetRegistryString(root RegistryRoot, keyPath, name, value string) error {
	return c.SetRegistryValue(root, keyPath, name, StringValue(value))
}

// SetRegistryExpandString writes a REG_EXPAND_SZ expandable string value.
func (c *Context) SetRegistryExpandString(root RegistryRoot, keyPath, name, value string) error {
	return c.SetRegistryValue(root, keyPath, name, ExpandStringValue(value))
}

// SetRegistryDWORD writes a 32-bit REG_DWORD value.
func (c *Context) SetRegistryDWORD(root RegistryRoot, keyPath, name string, value uint32) error {
	return c.SetRegistryValue(root, keyPath, name, DWORDValue(value))
}

// SetRegistryQWORD writes a 64-bit REG_QWORD value.
func (c *Context) SetRegistryQWORD(root RegistryRoot, keyPath, name string, value uint64) error {
	return c.SetRegistryValue(root, keyPath, name, QWORDValue(value))
}

// SetRegistryBinary writes a REG_BINARY value.
func (c *Context) SetRegistryBinary(root RegistryRoot, keyPath, name string, value []byte) error {
	return c.SetRegistryValue(root, keyPath, name, BinaryValue(value))
}

// SetRegistryMultiString writes a REG_MULTI_SZ string slice value.
func (c *Context) SetRegistryMultiString(root RegistryRoot, keyPath, name string, values []string) error {
	return c.SetRegistryValue(root, keyPath, name, MultiStringValue(values))
}

// DeleteRegistryValue removes a single value from a Windows Registry key.
func (c *Context) DeleteRegistryValue(root RegistryRoot, keyPath, name string) error {
	if root == "" {
		root = c.RegistryRoot()
	}
	return c.backend.DeleteRegistryValue(root, keyPath, name)
}

// DeleteRegistryKey recursively deletes a Windows Registry key and its subkeys.
func (c *Context) DeleteRegistryKey(root RegistryRoot, keyPath string) error {
	if root == "" {
		root = c.RegistryRoot()
	}
	return c.backend.DeleteRegistryKey(root, keyPath, false)
}

func appendEnvUnique(list []ManifestEnvEntry, e ManifestEnvEntry) []ManifestEnvEntry {
	for _, cur := range list {
		if cur.Scope == e.Scope && strings.EqualFold(cur.Name, e.Name) {
			return list
		}
	}
	return append(list, e)
}

func appendPathUnique(list []ManifestPathEntry, e ManifestPathEntry) []ManifestPathEntry {
	for _, cur := range list {
		if cur.Scope == e.Scope && strings.EqualFold(normalizePathEntry(cur.Dir), normalizePathEntry(e.Dir)) {
			return list
		}
	}
	return append(list, e)
}

func appendRegUnique(list []ManifestRegistryEntry, e ManifestRegistryEntry) []ManifestRegistryEntry {
	for i, cur := range list {
		if cur.Root == e.Root && strings.EqualFold(cur.Key, e.Key) && strings.EqualFold(cur.Name, e.Name) {
			// Keep DeleteKey true if the key was newly created on the first write.
			if cur.DeleteKey {
				list[i].DeleteKey = true
			}
			return list
		}
	}
	return append(list, e)
}

// Bool returns a persistent reactive bool option identified by key, creating it
// with defaultVal on first access. Bind it to ui.Checkbox in custom steps and
// read its snapshotted value safely inside lifecycle hooks with GetBool(key).
func (c *Context) Bool(key string, defaultVal bool) *ggui.StateValue[bool] {
	c.optMu.Lock()
	defer c.optMu.Unlock()
	if c.boolOpts == nil {
		c.boolOpts = map[string]*ggui.StateValue[bool]{}
	}
	if s, ok := c.boolOpts[key]; ok {
		return s
	}
	s := ggui.State(defaultVal)
	c.boolOpts[key] = s
	if c.boolSnap == nil {
		c.boolSnap = map[string]bool{}
	}
	c.boolSnap[key] = defaultVal
	return s
}

// GetBool returns the current value of a named bool option created via Bool.
// Safe to call from both UI code and worker-goroutine lifecycle hooks.
func (c *Context) GetBool(key string) bool {
	if !c.inWorker.Load() {
		c.optMu.Lock()
		s := c.boolOpts[key]
		c.optMu.Unlock()
		if s != nil {
			return ggui.Peek(s)
		}
	}
	c.optMu.Lock()
	defer c.optMu.Unlock()
	return c.boolSnap[key]
}

// String returns a persistent reactive string option identified by key,
// creating it with defaultVal on first access. Bind it to ui.TextField in
// custom steps and read it safely inside lifecycle hooks with GetString(key).
func (c *Context) String(key string, defaultVal string) *ggui.StateValue[string] {
	c.optMu.Lock()
	defer c.optMu.Unlock()
	if c.stringOpts == nil {
		c.stringOpts = map[string]*ggui.StateValue[string]{}
	}
	if s, ok := c.stringOpts[key]; ok {
		return s
	}
	s := ggui.State(defaultVal)
	c.stringOpts[key] = s
	if c.stringSnap == nil {
		c.stringSnap = map[string]string{}
	}
	c.stringSnap[key] = defaultVal
	return s
}

// GetString returns the current value of a named string option created via
// String. Safe to call from both UI code and worker-goroutine lifecycle hooks.
func (c *Context) GetString(key string) string {
	if !c.inWorker.Load() {
		c.optMu.Lock()
		s := c.stringOpts[key]
		c.optMu.Unlock()
		if s != nil {
			return ggui.Peek(s)
		}
	}
	c.optMu.Lock()
	defer c.optMu.Unlock()
	return c.stringSnap[key]
}

func (c *Context) snapshotOptions() {
	c.optMu.Lock()
	defer c.optMu.Unlock()
	if len(c.boolOpts) > 0 {
		if c.boolSnap == nil {
			c.boolSnap = make(map[string]bool, len(c.boolOpts))
		}
		for k, s := range c.boolOpts {
			c.boolSnap[k] = ggui.Peek(s)
		}
	}
	if len(c.stringOpts) > 0 {
		if c.stringSnap == nil {
			c.stringSnap = make(map[string]string, len(c.stringOpts))
		}
		for k, s := range c.stringOpts {
			c.stringSnap[k] = ggui.Peek(s)
		}
	}
}

// Value reads r safely from any goroutine, including BeforeInstall and
// AfterInstall hooks running off the UI thread. When called from the worker
// goroutine while a GUI event loop is active, the read is marshaled onto the
// UI goroutine via ctx.Post.
func Value[T any](ctx *Context, r ggui.Readable[T]) T {
	if ctx == nil || !ctx.inWorker.Load() || ctx.post == nil {
		return ggui.Peek(r)
	}
	ch := make(chan T, 1)
	ctx.post(func() {
		ch <- ggui.Peek(r)
	})
	return <-ch
}
