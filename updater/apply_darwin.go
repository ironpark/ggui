//go:build darwin

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

type darwinApplier struct{}

func newPlatformApplier() Applier {
	return darwinApplier{}
}

func isCrossDeviceErr(err error) bool {
	return errors.Is(err, syscall.EXDEV)
}

func (darwinApplier) Apply(ctx context.Context, upd *DownloadedUpdate, opts ApplyOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if upd == nil || strings.TrimSpace(upd.FilePath) == "" {
		return errors.New("updater: invalid downloaded update")
	}

	currentApp, err := resolveDarwinAppPath(opts.AppPath)
	if err != nil {
		return err
	}

	// Stage inside currentApp's parent directory whenever possible so the final
	// bundle swap is guaranteed to be an atomic same-filesystem rename.
	stageDir, err := os.MkdirTemp(filepath.Dir(currentApp), ".ggui-update-stage-*")
	if err != nil {
		stageDir, err = os.MkdirTemp("", "ggui-update-stage-*")
		if err != nil {
			return fmt.Errorf("updater: creating staging directory: %w", err)
		}
	}
	defer removeAllForce(stageDir)

	lowerPath := strings.ToLower(upd.FilePath)
	switch {
	case strings.HasSuffix(lowerPath, ".zip"):
		if err := extractZipSafe(upd.FilePath, stageDir); err != nil {
			return err
		}
	case strings.HasSuffix(lowerPath, ".dmg"):
		if err := extractDMGApp(ctx, upd.FilePath, stageDir); err != nil {
			return err
		}
	case strings.HasSuffix(lowerPath, ".tar.gz") || strings.HasSuffix(lowerPath, ".tgz"):
		if err := extractTarGzSafe(upd.FilePath, stageDir); err != nil {
			return err
		}
	default:
		return fmt.Errorf("updater: unsupported macOS update archive %q", filepath.Base(upd.FilePath))
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	stagedApp, err := locateExtractedApp(stageDir)
	if err != nil {
		return err
	}

	if err := verifyDarwinBundleIntegrity(ctx, currentApp, stagedApp); err != nil {
		return err
	}

	// Best-effort quarantine removal without following symlinks so macOS does not stall relaunch.
	_ = exec.CommandContext(ctx, "xattr", "-drs", "com.apple.quarantine", stagedApp).Run()

	if err := replaceAppBundle(currentApp, stagedApp); err != nil {
		return err
	}

	if opts.Relaunch {
		shArgs := []string{
			"-c",
			`pid="$1"; app="$2"; shift 2; i=0; while [ "$i" -lt 300 ] && kill -0 "$pid" 2>/dev/null; do sleep 0.1; i=$((i+1)); done; if [ "$#" -gt 0 ]; then open -n "$app" --args "$@"; else open -n "$app"; fi`,
			"_", strconv.Itoa(os.Getpid()), currentApp,
		}
		shArgs = append(shArgs, opts.RelaunchArgs...)
		cmd := exec.Command("/bin/sh", shArgs...)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		cmd.Stdin = nil
		cmd.Stdout = nil
		cmd.Stderr = nil
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("updater: scheduling macOS relaunch: %w", err)
		}
		_ = cmd.Process.Release()
	}

	if opts.QuitFunc != nil {
		opts.QuitFunc()
	}
	return nil
}

func resolveDarwinAppPath(override string) (string, error) {
	if s := strings.TrimSpace(override); s != "" {
		cleaned := filepath.Clean(s)
		if resolved, err := filepath.EvalSymlinks(cleaned); err == nil {
			cleaned = resolved
		}
		if app, ok := findAppBundle(cleaned); ok {
			fi, err := os.Stat(app)
			if err != nil {
				return "", fmt.Errorf("updater: app bundle %s: %w", app, err)
			}
			if !fi.IsDir() {
				return "", fmt.Errorf("updater: app bundle %s is not a directory", app)
			}
			return app, nil
		}
		return "", fmt.Errorf("updater: AppPath %q is not a macOS .app bundle", override)
	}

	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("updater: locating current executable: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	app, ok := findAppBundle(exe)
	if !ok {
		return "", fmt.Errorf("updater: current executable %q is not running inside a macOS .app bundle", exe)
	}
	return app, nil
}

func extractDMGApp(ctx context.Context, dmgPath, stageDir string) error {
	mntDir, err := os.MkdirTemp("", "ggui-dmg-mount-*")
	if err != nil {
		return fmt.Errorf("updater: creating dmg mountpoint: %w", err)
	}
	defer removeAllForce(mntDir)

	attach := exec.CommandContext(ctx, "hdiutil", "attach", "-nobrowse", "-readonly", "-mountpoint", mntDir, dmgPath)
	if out, err := attach.CombinedOutput(); err != nil {
		return fmt.Errorf("updater: mounting dmg %s: %w (%s)", dmgPath, err, strings.TrimSpace(string(out)))
	}
	defer func() {
		_ = exec.Command("hdiutil", "detach", "-force", "-quiet", mntDir).Run()
	}()

	dmgApp, err := locateExtractedApp(mntDir)
	if err != nil {
		return fmt.Errorf("updater: locating .app in mounted dmg: %w", err)
	}

	dstApp := filepath.Join(stageDir, filepath.Base(dmgApp))
	if err := copyDirPreserving(dmgApp, dstApp); err != nil {
		return fmt.Errorf("updater: copying .app from dmg: %w", err)
	}
	return nil
}

func verifyDarwinBundleIntegrity(ctx context.Context, currentApp, stagedApp string) error {
	curID, err := readDarwinBundleID(ctx, currentApp)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("updater: reading current bundle ID: %w", err)
	}
	newID, err := readDarwinBundleID(ctx, stagedApp)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("updater: reading update bundle ID: %w", err)
	}
	if curID != "" && (newID == "" || curID != newID) {
		return fmt.Errorf("updater: bundle identifier mismatch (current=%q, update=%q)", curID, newID)
	}

	if _, err := os.Stat("/usr/bin/codesign"); err != nil {
		return nil
	}

	curSigned, curAdHoc, curTeamID, curAuthorities, err := inspectDarwinCodeSign(ctx, currentApp)
	if err != nil {
		return fmt.Errorf("updater: inspecting current code signature: %w", err)
	}
	newSigned, newAdHoc, newTeamID, newAuthorities, err := inspectDarwinCodeSign(ctx, stagedApp)
	if err != nil {
		return fmt.Errorf("updater: inspecting update code signature: %w", err)
	}

	if newSigned {
		cmd := exec.CommandContext(ctx, "/usr/bin/codesign", "--verify", "--deep", "--strict", stagedApp)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("updater: update code signature verification failed: %w (%s)", err, strings.TrimSpace(string(out)))
		}
	}

	if curSigned && !curAdHoc {
		if !newSigned {
			return errors.New("updater: refusing to replace a signed application bundle with an unsigned update")
		}
		if newAdHoc {
			return errors.New("updater: refusing to replace a signed application bundle with an ad-hoc signed update")
		}
		if err := verifyDarwinSigningIdentity(curTeamID, curAuthorities, newTeamID, newAuthorities); err != nil {
			return err
		}
	}
	return nil
}

func readPlatformBundleID(appPath string) (string, error) {
	return readDarwinBundleID(context.Background(), appPath)
}

func readDarwinBundleID(ctx context.Context, appPath string) (string, error) {
	id, err := readBundleID(appPath)
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return id, err
	}
	// Fallback for binary plists on macOS via plutil.
	plistPath := filepath.Join(appPath, "Contents", "Info.plist")
	out, convErr := exec.CommandContext(ctx, "/usr/bin/plutil", "-convert", "xml1", "-o", "-", plistPath).Output()
	if convErr != nil {
		return "", err
	}
	return parsePlistBundleID(out)
}

func inspectDarwinCodeSign(ctx context.Context, appPath string) (signed bool, adHoc bool, teamID string, authorities []string, err error) {
	sigPath := filepath.Join(appPath, "Contents", "_CodeSignature", "CodeResources")
	_, sigStatErr := os.Lstat(sigPath)
	hasCodeResources := sigStatErr == nil
	if sigStatErr != nil && !errors.Is(sigStatErr, os.ErrNotExist) {
		return false, false, "", nil, fmt.Errorf("updater: checking code signature resources: %w", sigStatErr)
	}

	out, cmdErr := exec.CommandContext(ctx, "/usr/bin/codesign", "-dvv", appPath).CombinedOutput()
	outStr := string(out)
	if cmdErr != nil {
		if !hasCodeResources && strings.Contains(strings.ToLower(outStr), "not signed at all") {
			return false, false, "", nil, nil
		}
		return false, false, "", nil, fmt.Errorf("updater: codesign -dvv failed: %w (%s)", cmdErr, strings.TrimSpace(outStr))
	}

	adHoc, teamID, authorities = parseDarwinCodeSignOutput(outStr)
	return true, adHoc, teamID, authorities, nil
}
