//go:build windows

package updater

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

const (
	winCreateNoWindow   uint32        = 0x08000000
	winErrNotSameDevice syscall.Errno = 17 // ERROR_NOT_SAME_DEVICE
)

type windowsApplier struct{}

func newPlatformApplier() Applier {
	return windowsApplier{}
}

func isCrossDeviceErr(err error) bool {
	return errors.Is(err, winErrNotSameDevice)
}

func readPlatformBundleID(appPath string) (string, error) {
	return readBundleID(appPath)
}

func (windowsApplier) Apply(ctx context.Context, upd *DownloadedUpdate, opts ApplyOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if upd == nil || strings.TrimSpace(upd.FilePath) == "" {
		return errors.New("updater: invalid downloaded update")
	}

	targetExe, err := resolveWindowsTargetExe(opts.AppPath)
	if err != nil {
		return err
	}

	lowerPath := strings.ToLower(upd.FilePath)
	switch {
	case strings.HasSuffix(lowerPath, ".exe"):
		return applyWindowsInstaller(upd, targetExe, opts)
	case strings.HasSuffix(lowerPath, ".zip"):
		return applyWindowsPortable(ctx, upd, targetExe, opts)
	default:
		return fmt.Errorf("updater: unsupported Windows update package %q", filepath.Base(upd.FilePath))
	}
}

func resolveWindowsTargetExe(override string) (string, error) {
	if s := strings.TrimSpace(override); s != "" {
		return filepath.Clean(s), nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("updater: locating current executable: %w", err)
	}
	return filepath.Clean(exe), nil
}

func applyWindowsInstaller(upd *DownloadedUpdate, targetExe string, opts ApplyOptions) error {
	installDir := filepath.Dir(targetExe)
	args := buildWindowsInstallerArgs(upd.Release.Enclosure.InstallerArguments, installDir, opts)
	isSilent := hasWindowsSilentFlag(args)

	if opts.Relaunch && isSilent {
		spec, err := buildWindowsSilentRelaunchHelperWithArgs(os.Getpid(), upd.FilePath, args, targetExe, opts.RelaunchArgs)
		if err != nil {
			return err
		}
		cmd := exec.Command(spec.Exe, spec.Args...)
		cmd.Env = mergeEnv(os.Environ(), spec.Env)
		cmd.SysProcAttr = &syscall.SysProcAttr{
			HideWindow:    true,
			CreationFlags: winCreateNoWindow | syscall.CREATE_NEW_PROCESS_GROUP,
		}
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("updater: launching silent installer helper: %w", err)
		}
		_ = cmd.Process.Release()
	} else {
		if err := validateWindowsHelperValue("Windows setup path", upd.FilePath); err != nil {
			return err
		}
		for _, a := range args {
			if err := validateWindowsHelperValue("Windows installer argument", a); err != nil {
				return err
			}
		}
		cmd := exec.Command(upd.FilePath, args...)
		cmd.SysProcAttr = &syscall.SysProcAttr{
			CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
		}
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("updater: launching installer %s: %w", upd.FilePath, err)
		}
		_ = cmd.Process.Release()
	}

	if opts.QuitFunc != nil {
		opts.QuitFunc()
	}
	return nil
}

func applyWindowsPortable(ctx context.Context, upd *DownloadedUpdate, targetExe string, opts ApplyOptions) error {
	stageDir, err := os.MkdirTemp("", "ggui-update-win-*")
	if err != nil {
		return fmt.Errorf("updater: creating staging directory: %w", err)
	}
	defer removeAllForce(stageDir)

	if err := extractZipSafe(upd.FilePath, stageDir); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	replacements, err := locateWindowsPortableReplacements(stageDir, targetExe)
	if err != nil {
		return err
	}

	oldBackups, err := replacePortableFiles(replacements)
	if err != nil {
		return err
	}

	relaunchTarget := ""
	if opts.Relaunch {
		relaunchTarget = targetExe
	}

	// Always spawn the detached helper before calling QuitFunc so process exit
	// cannot race ahead of scheduling relaunch and delayed .old cleanup.
	if relaunchTarget != "" || len(oldBackups) > 0 {
		spec, specErr := buildWindowsPortableHelperWithArgs(os.Getpid(), relaunchTarget, opts.RelaunchArgs, oldBackups)
		if specErr != nil {
			if opts.Relaunch {
				return specErr
			}
		} else if spec != nil {
			helperCmd := exec.Command(spec.Exe, spec.Args...)
			helperCmd.Env = mergeEnv(os.Environ(), spec.Env)
			helperCmd.SysProcAttr = &syscall.SysProcAttr{
				HideWindow:    true,
				CreationFlags: winCreateNoWindow | syscall.CREATE_NEW_PROCESS_GROUP,
			}
			if startErr := helperCmd.Start(); startErr != nil {
				if opts.Relaunch {
					return fmt.Errorf("updater: scheduling Windows relaunch: %w", startErr)
				}
			} else {
				_ = helperCmd.Process.Release()
			}
		}
	}

	if opts.QuitFunc != nil {
		opts.QuitFunc()
	}
	return nil
}

func mergeEnv(base, overrides []string) []string {
	overrideKeys := make(map[string]struct{}, len(overrides))
	for _, kv := range overrides {
		if k, _, ok := strings.Cut(kv, "="); ok {
			overrideKeys[strings.ToUpper(k)] = struct{}{}
		}
	}
	out := make([]string, 0, len(base)+len(overrides))
	for _, kv := range base {
		if k, _, ok := strings.Cut(kv, "="); ok {
			if _, drop := overrideKeys[strings.ToUpper(k)]; drop {
				continue
			}
		}
		out = append(out, kv)
	}
	out = append(out, overrides...)
	return out
}
