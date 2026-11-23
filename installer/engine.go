package installer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Progress reports installation or uninstallation progress to the UI or CLI.
type Progress struct {
	Fraction float64 // 0.0 to 1.0
	Message  string
}

// InstallOptions controls where and how a payload is installed.
type InstallOptions struct {
	InstallDir        string
	StartMenuShortcut bool
	DesktopShortcut   bool
}

// InstallManifest records the exact files, shortcuts, environment variables,
// PATH entries, and Windows Registry values placed by Install so Uninstall
// removes only what belongs to this application.
type InstallManifest struct {
	ID        string                  `json:"id"`
	Name      string                  `json:"name"`
	Version   string                  `json:"version"`
	Files     []string                `json:"files"`
	Shortcuts []string                `json:"shortcuts,omitempty"`
	Env       []ManifestEnvEntry      `json:"env,omitempty"`
	Path      []ManifestPathEntry     `json:"path,omitempty"`
	Registry  []ManifestRegistryEntry `json:"registry,omitempty"`
}

// installTx tracks active installation/uninstallation state so hook helpers
// on Context (WriteFile, TrackFile, TrackShortcut, CreateShortcut, SetEnv,
// AddToPath, SetRegistry*, ReportProgress) participate in atomic rollback and
// manifest recording.
type installTx struct {
	absDir           string
	backend          Backend
	report           func(float64, string)
	installedRel     *[]string
	createdFiles     *[]string
	createdShortcuts *[]string
	envEntries       *[]ManifestEnvEntry
	pathEntries      *[]ManifestPathEntry
	regEntries       *[]ManifestRegistryEntry
	rollbackFns      *[]func()
}

// Install extracts payload into opts.InstallDir, copies the running installer
// binary as Uninstall.exe, creates requested shortcuts, writes the install
// manifest, and registers the Windows Uninstall entry. If extraction fails
// partway through, newly written files are rolled back cleanly.
func Install(ctx context.Context, payload *Payload, opts InstallOptions, b Backend, onProgress func(Progress)) error {
	return install(ctx, payload, opts, b, nil, onProgress)
}

func install(ctx context.Context, payload *Payload, opts InstallOptions, b Backend, ic *Context, onProgress func(Progress)) error {
	if payload == nil {
		return errors.New("installer: nil payload")
	}
	if b == nil {
		b = defaultBackend()
	}
	report := func(f float64, msg string) {
		if onProgress != nil {
			onProgress(Progress{Fraction: f, Message: msg})
		}
	}

	installDir := strings.TrimSpace(opts.InstallDir)
	if installDir == "" {
		return errors.New("install directory cannot be empty")
	}
	absDir, err := filepath.Abs(installDir)
	if err != nil {
		return fmt.Errorf("invalid install directory %q: %w", installDir, err)
	}
	if err := os.MkdirAll(absDir, 0o755); err != nil {
		return fmt.Errorf("creating install directory %s: %w", absDir, err)
	}

	var createdFiles []string
	var createdShortcuts []string
	var installedRel []string
	var envEntries []ManifestEnvEntry
	var pathEntries []ManifestPathEntry
	var regEntries []ManifestRegistryEntry
	var rollbackFns []func()
	rollback := func() {
		for i := len(rollbackFns) - 1; i >= 0; i-- {
			rollbackFns[i]()
		}
		for _, s := range createdShortcuts {
			_ = b.RemoveShortcut(s)
		}
		for i := len(createdFiles) - 1; i >= 0; i-- {
			_ = os.Remove(createdFiles[i])
		}
	}

	if ic != nil {
		ic.beginTx(&installTx{
			absDir:           absDir,
			backend:          b,
			report:           report,
			installedRel:     &installedRel,
			createdFiles:     &createdFiles,
			createdShortcuts: &createdShortcuts,
			envEntries:       &envEntries,
			pathEntries:      &pathEntries,
			regEntries:       &regEntries,
			rollbackFns:      &rollbackFns,
		})
		defer ic.endTx()
	}

	previewPause := func() error {
		if ic == nil || !ic.isPreview || ic.post == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(60 * time.Millisecond):
			return nil
		}
	}

	report(0.05, "Preparing installation…")
	if err := previewPause(); err != nil {
		rollback()
		return err
	}

	if ic != nil && ic.app != nil && ic.app.BeforeInstall != nil {
		if err := ic.app.BeforeInstall(ic); err != nil {
			rollback()
			return err
		}
	}

	var writtenBytes int64
	totalBytes := max(payload.TotalBytes, 1)

	for _, f := range payload.Files {
		if err := ctx.Err(); err != nil {
			rollback()
			return err
		}
		dst := filepath.Join(absDir, filepath.FromSlash(f.RelPath))
		if !isSubpath(absDir, dst) {
			rollback()
			return fmt.Errorf("file %q escapes install directory", f.RelPath)
		}
		report(0.05+0.70*float64(writtenBytes)/float64(totalBytes), "Installing "+f.RelPath+"…")
		existed := fileExists(dst)
		if err := writeFileAtomic(dst, f.Data, 0o755); err != nil {
			rollback()
			return fmt.Errorf("writing %s: %w (close %s if it is currently running and try again)", f.RelPath, err, payload.Meta.Name)
		}
		if !existed {
			createdFiles = append(createdFiles, dst)
		}
		installedRel = appendUnique(installedRel, f.RelPath)
		writtenBytes += int64(len(f.Data))
		if err := previewPause(); err != nil {
			rollback()
			return err
		}
	}

	// Copy this installer binary as Uninstall.exe inside the install directory.
	uninstallDst := filepath.Join(absDir, uninstallerExe)
	var uninstallBytes int64
	if selfExe, err := b.SelfExe(); err == nil && selfExe != "" && !samePath(selfExe, uninstallDst) {
		report(0.80, "Writing uninstaller…")
		existed := fileExists(uninstallDst)
		n, err := copyFileAtomic(selfExe, uninstallDst, 0o755)
		if err != nil {
			rollback()
			return fmt.Errorf("writing %s: %w", uninstallerExe, err)
		}
		if !existed {
			createdFiles = append(createdFiles, uninstallDst)
		}
		uninstallBytes = n
		installedRel = appendUnique(installedRel, uninstallerExe)
	}

	exePath := filepath.Join(absDir, filepath.FromSlash(payload.Meta.Executable))
	desc := payload.Meta.Description
	if desc == "" {
		desc = payload.Meta.Name
	}

	report(0.85, "Creating shortcuts…")
	if err := previewPause(); err != nil {
		rollback()
		return err
	}
	if opts.StartMenuShortcut {
		if dir := b.StartMenuDir(payload.Meta.NormalizedScope()); dir != "" {
			lnk := filepath.Join(dir, payload.Meta.Name+".lnk")
			if err := b.CreateShortcut(lnk, exePath, absDir, desc, exePath); err != nil {
				rollback()
				return fmt.Errorf("creating Start Menu shortcut: %w", err)
			}
			createdShortcuts = appendUnique(createdShortcuts, lnk)
		}
	}
	if opts.DesktopShortcut {
		if dir := b.DesktopDir(payload.Meta.NormalizedScope()); dir != "" {
			lnk := filepath.Join(dir, payload.Meta.Name+".lnk")
			if err := b.CreateShortcut(lnk, exePath, absDir, desc, exePath); err != nil {
				rollback()
				return fmt.Errorf("creating Desktop shortcut: %w", err)
			}
			createdShortcuts = appendUnique(createdShortcuts, lnk)
		}
	}

	// Apply declarative WindowsConfig PATH, environment variables, and registry entries.
	if ic != nil && ic.app != nil {
		if err := applyDeclarativeWindowsConfig(ic, payload.Meta, absDir, exePath); err != nil {
			rollback()
			return err
		}
	}

	if ic != nil && ic.app != nil && ic.app.AfterInstall != nil {
		report(0.90, "Running post-install tasks…")
		if err := ic.app.AfterInstall(ic); err != nil {
			rollback()
			return err
		}
	}

	// Preserve tracked PATH, environment, and registry entries from an earlier
	// manifest for this same application so silent upgrades keep clean uninstall
	// tracking for options the user chose during initial setup.
	mfPath := filepath.Join(absDir, manifestName)
	if prevRaw, err := os.ReadFile(mfPath); err == nil {
		var prev InstallManifest
		if json.Unmarshal(prevRaw, &prev) == nil && prev.ID == payload.Meta.ID {
			for _, pe := range prev.Path {
				if !slices.Contains(pathEntries, pe) {
					pathEntries = append(pathEntries, pe)
				}
			}
			for _, ee := range prev.Env {
				if !slices.Contains(envEntries, ee) {
					envEntries = append(envEntries, ee)
				}
			}
			for _, re := range prev.Registry {
				if !slices.Contains(regEntries, re) {
					regEntries = append(regEntries, re)
				}
			}
		}
	}

	// Persist manifest after declarative settings and AfterInstall so extra
	// files, shortcuts, env vars, PATH entries, and registry entries are
	// recorded for clean Uninstall.
	mf := InstallManifest{
		ID:        payload.Meta.ID,
		Name:      payload.Meta.Name,
		Version:   payload.Meta.Version,
		Files:     installedRel,
		Shortcuts: createdShortcuts,
		Env:       envEntries,
		Path:      pathEntries,
		Registry:  regEntries,
	}
	mfBytes, err := json.MarshalIndent(mf, "", "  ")
	if err != nil {
		rollback()
		return err
	}
	if err := writeFileAtomic(mfPath, mfBytes, 0o644); err != nil {
		rollback()
		return fmt.Errorf("writing %s: %w", manifestName, err)
	}
	createdFiles = append(createdFiles, mfPath)

	report(0.95, "Registering application…")
	if err := previewPause(); err != nil {
		rollback()
		return err
	}
	sizeKB := uint32((payload.TotalBytes + uninstallBytes + int64(len(mfBytes)) + 1023) / 1024)
	if sizeKB == 0 {
		sizeKB = 1
	}
	entry := UninstallEntry{
		Meta:              payload.Meta,
		InstallDir:        absDir,
		ExecutablePath:    exePath,
		UninstallExePath:  uninstallDst,
		EstimatedSizeKB:   sizeKB,
		UninstallCommand:  fmt.Sprintf(`"%s" --uninstall`, uninstallDst),
		QuietUninstallCmd: fmt.Sprintf(`"%s" --uninstall --silent`, uninstallDst),
	}
	if err := b.WriteUninstallEntry(entry); err != nil {
		rollback()
		return err
	}
	b.NotifyShell()

	report(1.0, "Installation complete.")
	return nil
}

// Uninstall removes the files and shortcuts recorded in install-manifest.json
// (falling back to the app executable and Uninstall.exe), removes empty
// directories, and deletes the Windows Uninstall registry entry. Unrelated
// user files inside installDir are preserved.
func Uninstall(ctx context.Context, meta Metadata, installDir string, b Backend, onProgress func(Progress)) error {
	return uninstall(ctx, meta, installDir, b, nil, onProgress)
}

func uninstall(ctx context.Context, meta Metadata, installDir string, b Backend, ic *Context, onProgress func(Progress)) error {
	if b == nil {
		b = defaultBackend()
	}
	report := func(f float64, msg string) {
		if onProgress != nil {
			onProgress(Progress{Fraction: f, Message: msg})
		}
	}

	absDir, err := filepath.Abs(strings.TrimSpace(installDir))
	if err != nil {
		return fmt.Errorf("invalid install directory %q: %w", installDir, err)
	}

	if ic != nil {
		ic.beginTx(&installTx{
			absDir:  absDir,
			backend: b,
			report:  report,
		})
		defer ic.endTx()
	}

	report(0.1, "Reading install manifest…")
	mfPath := filepath.Join(absDir, manifestName)
	var mf InstallManifest
	if data, err := os.ReadFile(mfPath); err == nil {
		_ = json.Unmarshal(data, &mf)
	}
	if len(mf.Files) == 0 {
		mf.Files = []string{meta.Executable, uninstallerExe}
	}
	if len(mf.Shortcuts) == 0 {
		scope := meta.NormalizedScope()
		if d := b.StartMenuDir(scope); d != "" {
			mf.Shortcuts = append(mf.Shortcuts, filepath.Join(d, meta.Name+".lnk"))
		}
		if d := b.DesktopDir(scope); d != "" {
			mf.Shortcuts = append(mf.Shortcuts, filepath.Join(d, meta.Name+".lnk"))
		}
	}

	if ic != nil && ic.app != nil && ic.app.BeforeUninstall != nil {
		report(0.18, "Running pre-uninstall tasks…")
		if err := ic.app.BeforeUninstall(ic); err != nil {
			return err
		}
	}

	report(0.25, "Removing shortcuts…")
	for _, s := range mf.Shortcuts {
		_ = b.RemoveShortcut(s)
	}

	selfExe, _ := b.SelfExe()
	dirs := map[string]struct{}{}
	total := max(len(mf.Files), 1)
	for i, rel := range mf.Files {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !validRelPath(filepath.ToSlash(rel)) {
			continue
		}
		p := filepath.Join(absDir, filepath.FromSlash(rel))
		if !isSubpath(absDir, p) {
			continue
		}
		report(0.25+0.55*float64(i)/float64(total), "Removing "+rel+"…")
		if selfExe != "" && samePath(selfExe, p) {
			continue
		}
		if err := removeWithRetry(ctx, p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("removing %s: %w (close %s if it is currently running and try again)", rel, err, meta.Name)
		}
		for parent := filepath.Dir(p); parent != absDir && isSubpath(absDir, parent); parent = filepath.Dir(parent) {
			dirs[parent] = struct{}{}
		}
	}
	_ = os.Remove(mfPath)

	// Remove empty subdirectories deepest-first, then installDir itself if empty.
	dirList := make([]string, 0, len(dirs))
	for d := range dirs {
		dirList = append(dirList, d)
	}
	slices.SortFunc(dirList, func(a, b string) int { return len(b) - len(a) })
	for _, d := range dirList {
		_ = os.Remove(d)
	}
	_ = os.Remove(absDir)

	report(0.86, "Removing environment and registry entries…")
	for i := len(mf.Path) - 1; i >= 0; i-- {
		_ = b.RemoveFromPath(mf.Path[i].Scope, mf.Path[i].Dir)
	}
	for i := len(mf.Env) - 1; i >= 0; i-- {
		_ = b.DeleteEnv(mf.Env[i].Scope, mf.Env[i].Name)
	}
	for i := len(mf.Registry) - 1; i >= 0; i-- {
		re := mf.Registry[i]
		_ = b.DeleteRegistryValue(re.Root, re.Key, re.Name)
		if re.DeleteKey {
			_ = b.DeleteRegistryKey(re.Root, re.Key, true)
		}
	}

	report(0.90, "Removing registration…")
	if err := b.DeleteUninstallEntry(meta); err != nil {
		return err
	}
	b.NotifyShell()

	if ic != nil && ic.app != nil && ic.app.AfterUninstall != nil {
		report(0.95, "Finalizing uninstall…")
		if err := ic.app.AfterUninstall(ic); err != nil {
			return err
		}
	}

	report(1.0, "Uninstall complete.")
	return nil
}

func applyDeclarativeWindowsConfig(ic *Context, meta Metadata, absDir, exePath string) error {
	wc := ic.app.Windows
	if wc.AddToPath {
		if err := ic.AddToPath(absDir); err != nil {
			return err
		}
	}
	if len(wc.Env) > 0 {
		keys := make([]string, 0, len(wc.Env))
		for k := range wc.Env {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		for _, k := range keys {
			val := expandInstallTokens(wc.Env[k], meta, absDir, exePath)
			if err := ic.SetEnv(k, val); err != nil {
				return err
			}
		}
	}
	for _, entry := range wc.Registry {
		root := entry.Root
		if root == "" {
			root = defaultRegistryRoot(meta.NormalizedScope())
		}
		val := expandRegistryValue(entry.Value, meta, absDir, exePath)
		if err := ic.SetRegistryValue(root, entry.Key, entry.Name, val); err != nil {
			return err
		}
	}
	return nil
}

func appendUnique(list []string, v string) []string {
	if slices.Contains(list, v) {
		return list
	}
	return append(list, v)
}

func isSubpath(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func samePath(a, b string) bool {
	aa, err1 := filepath.Abs(a)
	bb, err2 := filepath.Abs(b)
	if err1 != nil || err2 != nil {
		return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
	}
	return strings.EqualFold(aa, bb)
}

func writeFileAtomic(dst string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".ggui-install-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := renameWithRetry(tmpName, dst); err != nil {
		return err
	}
	ok = true
	return nil
}

// renameWithRetry atomically replaces dst with src, retrying briefly if dst
// is transiently locked on Windows (for example while an updated application
// finishes exiting right before a silent upgrade).
func renameWithRetry(src, dst string) error {
	var err error
	for attempt := range 20 {
		if err = os.Rename(src, dst); err == nil {
			return nil
		}
		if rmErr := os.Remove(dst); rmErr == nil {
			if err = os.Rename(src, dst); err == nil {
				return nil
			}
		}
		if attempt == 19 {
			return err
		}
		time.Sleep(150 * time.Millisecond)
	}
	return err
}

func copyFileAtomic(src, dst string, mode os.FileMode) (int64, error) {
	in, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return 0, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".ggui-copy-*")
	if err != nil {
		return 0, err
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmpName)
		}
	}()
	n, err := io.Copy(tmp, in)
	if err != nil {
		_ = tmp.Close()
		return 0, err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return 0, err
	}
	if err := tmp.Close(); err != nil {
		return 0, err
	}
	if err := os.Rename(tmpName, dst); err != nil {
		if rmErr := os.Remove(dst); rmErr == nil {
			if err2 := os.Rename(tmpName, dst); err2 == nil {
				ok = true
				return n, nil
			}
		}
		return 0, err
	}
	ok = true
	return n, nil
}

// removeWithRetry retries os.Remove briefly to allow a parent Uninstall.exe
// process that just relaunched from %TEMP% to finish exiting and release its
// Windows file lock.
func removeWithRetry(ctx context.Context, path string) error {
	var err error
	for attempt := range 10 {
		err = os.Remove(path)
		if err == nil || errors.Is(err, os.ErrNotExist) {
			return err
		}
		if attempt == 9 {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	return err
}
