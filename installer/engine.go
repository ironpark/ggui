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

// InstallManifest records the exact files and shortcuts placed by Install so
// Uninstall removes only what belongs to this application.
type InstallManifest struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Version   string   `json:"version"`
	Files     []string `json:"files"`
	Shortcuts []string `json:"shortcuts,omitempty"`
}

// Install extracts payload into opts.InstallDir, copies the running installer
// binary as Uninstall.exe, creates requested shortcuts, writes the install
// manifest, and registers the Windows Uninstall entry. If extraction fails
// partway through, newly written files are rolled back cleanly.
func Install(ctx context.Context, payload *Payload, opts InstallOptions, b Backend, onProgress func(Progress)) error {
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
	rollback := func() {
		for _, s := range createdShortcuts {
			_ = b.RemoveShortcut(s)
		}
		for i := len(createdFiles) - 1; i >= 0; i-- {
			_ = os.Remove(createdFiles[i])
		}
	}

	report(0.05, "Preparing installation…")

	var installedRel []string
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
		report(0.05+0.75*float64(writtenBytes)/float64(totalBytes), "Installing "+f.RelPath+"…")
		existed := fileExists(dst)
		if err := writeFileAtomic(dst, f.Data, 0o755); err != nil {
			rollback()
			return fmt.Errorf("writing %s: %w (close %s if it is currently running and try again)", f.RelPath, err, payload.Meta.Name)
		}
		if !existed {
			createdFiles = append(createdFiles, dst)
		}
		installedRel = append(installedRel, f.RelPath)
		writtenBytes += int64(len(f.Data))
	}

	// Copy this installer binary as Uninstall.exe inside the install directory.
	uninstallDst := filepath.Join(absDir, uninstallerExe)
	var uninstallBytes int64
	if selfExe, err := b.SelfExe(); err == nil && selfExe != "" && !samePath(selfExe, uninstallDst) {
		report(0.84, "Writing uninstaller…")
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
		installedRel = append(installedRel, uninstallerExe)
	}

	exePath := filepath.Join(absDir, filepath.FromSlash(payload.Meta.Executable))
	desc := payload.Meta.Description
	if desc == "" {
		desc = payload.Meta.Name
	}

	report(0.90, "Creating shortcuts…")
	if opts.StartMenuShortcut {
		if dir := b.StartMenuDir(payload.Meta.NormalizedScope()); dir != "" {
			lnk := filepath.Join(dir, payload.Meta.Name+".lnk")
			if err := b.CreateShortcut(lnk, exePath, absDir, desc, exePath); err != nil {
				rollback()
				return fmt.Errorf("creating Start Menu shortcut: %w", err)
			}
			createdShortcuts = append(createdShortcuts, lnk)
		}
	}
	if opts.DesktopShortcut {
		if dir := b.DesktopDir(payload.Meta.NormalizedScope()); dir != "" {
			lnk := filepath.Join(dir, payload.Meta.Name+".lnk")
			if err := b.CreateShortcut(lnk, exePath, absDir, desc, exePath); err != nil {
				rollback()
				return fmt.Errorf("creating Desktop shortcut: %w", err)
			}
			createdShortcuts = append(createdShortcuts, lnk)
		}
	}

	// Persist manifest so Uninstall removes only files/shortcuts we placed.
	mf := InstallManifest{
		ID:        payload.Meta.ID,
		Name:      payload.Meta.Name,
		Version:   payload.Meta.Version,
		Files:     installedRel,
		Shortcuts: createdShortcuts,
	}
	mfBytes, err := json.MarshalIndent(mf, "", "  ")
	if err != nil {
		rollback()
		return err
	}
	mfPath := filepath.Join(absDir, manifestName)
	if err := writeFileAtomic(mfPath, mfBytes, 0o644); err != nil {
		rollback()
		return fmt.Errorf("writing %s: %w", manifestName, err)
	}
	createdFiles = append(createdFiles, mfPath)

	report(0.95, "Registering application…")
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

	report(0.90, "Removing registration…")
	if err := b.DeleteUninstallEntry(meta); err != nil {
		return err
	}
	b.NotifyShell()

	report(1.0, "Uninstall complete.")
	return nil
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
	if err := os.Rename(tmpName, dst); err != nil {
		// On Windows, os.Rename onto an existing file succeeds in Go 1.24+,
		// or fallback to removing first if an older FS rejects it.
		if rmErr := os.Remove(dst); rmErr == nil {
			if err2 := os.Rename(tmpName, dst); err2 == nil {
				ok = true
				return nil
			}
		}
		return err
	}
	ok = true
	return nil
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
