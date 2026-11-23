//go:build linux

package updater

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

type linuxApplier struct{}

func newPlatformApplier() Applier {
	return linuxApplier{}
}

func isCrossDeviceErr(err error) bool {
	return errors.Is(err, syscall.EXDEV)
}

func readPlatformBundleID(appPath string) (string, error) {
	return readBundleID(appPath)
}

func (linuxApplier) Apply(ctx context.Context, upd *DownloadedUpdate, opts ApplyOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if upd == nil || strings.TrimSpace(upd.FilePath) == "" {
		return errors.New("updater: invalid downloaded update")
	}

	targetExe, err := resolveLinuxTargetExe(opts.AppPath)
	if err != nil {
		return err
	}

	pkgKind, err := detectLinuxPackageKind(upd.FilePath)
	if err != nil {
		return err
	}
	switch pkgKind {
	case linuxPackageTarGz:
		if err := applyLinuxArchive(ctx, upd.FilePath, targetExe, extractTarGzSafe); err != nil {
			return err
		}
	case linuxPackageZip:
		if err := applyLinuxArchive(ctx, upd.FilePath, targetExe, extractZipSafe); err != nil {
			return err
		}
	case linuxPackageRawExecutable:
		if err := replaceLinuxBinary(targetExe, upd.FilePath); err != nil {
			return err
		}
	default:
		return fmt.Errorf("updater: unsupported Linux update package %q", filepath.Base(upd.FilePath))
	}

	if opts.Relaunch {
		cmd := exec.Command("/bin/sh", "-c",
			`pid="$1"; bin="$2"; i=0; while [ "$i" -lt 150 ] && kill -0 "$pid" 2>/dev/null; do sleep 0.1; i=$((i+1)); done; exec "$bin"`,
			"_", strconv.Itoa(os.Getpid()), targetExe,
		)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		cmd.Stdin = nil
		cmd.Stdout = nil
		cmd.Stderr = nil
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("updater: scheduling Linux relaunch: %w", err)
		}
		_ = cmd.Process.Release()
	}

	if opts.QuitFunc != nil {
		opts.QuitFunc()
	}
	return nil
}

func resolveLinuxTargetExe(override string) (string, error) {
	if s := strings.TrimSpace(override); s != "" {
		cleaned := filepath.Clean(s)
		if resolved, err := filepath.EvalSymlinks(cleaned); err == nil {
			return resolved, nil
		}
		return cleaned, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("updater: locating current executable: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		return resolved, nil
	}
	return filepath.Clean(exe), nil
}

func applyLinuxArchive(
	ctx context.Context,
	archivePath, targetExe string,
	extractFn func(archivePath, dstDir string) error,
) error {
	stageDir, err := os.MkdirTemp("", "ggui-update-linux-*")
	if err != nil {
		return fmt.Errorf("updater: creating staging directory: %w", err)
	}
	defer removeAllForce(stageDir)

	if err := extractFn(archivePath, stageDir); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	newBin, err := locateLinuxBinary(stageDir, filepath.Base(targetExe))
	if err != nil {
		return err
	}

	// Transactionally update any bundled shared libraries (e.g. usr/lib/<pkg>/libggfx_driver.so)
	// and swap the main executable, rolling back all .so updates on any failure.
	libs := locateLinuxBundledLibs(stageDir, newBin, targetExe)
	return replaceLinuxArchiveFiles(libs, targetExe, newBin, copyFileAtomic, replaceLinuxBinary)
}
