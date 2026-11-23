//go:build !darwin && !windows

package updater

func currentOSVersion() string {
	return ""
}
