//go:build !darwin && !windows && !linux

package updater

import "context"

type unsupportedApplier struct{}

func newPlatformApplier() Applier {
	return unsupportedApplier{}
}

func isCrossDeviceErr(error) bool {
	return false
}

func readPlatformBundleID(appPath string) (string, error) {
	return readBundleID(appPath)
}

func (unsupportedApplier) Apply(context.Context, *DownloadedUpdate, ApplyOptions) error {
	return ErrUnsupportedPlatform
}
