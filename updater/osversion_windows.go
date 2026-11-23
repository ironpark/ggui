//go:build windows

package updater

import (
	"fmt"
	"strings"

	"golang.org/x/sys/windows/registry"
)

func currentOSVersion() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()

	build, _, _ := k.GetStringValue("CurrentBuildNumber")
	build = strings.TrimSpace(build)

	major, _, errMaj := k.GetIntegerValue("CurrentMajorVersionNumber")
	minor, _, errMin := k.GetIntegerValue("CurrentMinorVersionNumber")
	if errMaj == nil && errMin == nil {
		if build != "" {
			return fmt.Sprintf("%d.%d.%s", major, minor, build)
		}
		return fmt.Sprintf("%d.%d", major, minor)
	}

	ver, _, err := k.GetStringValue("CurrentVersion")
	if err != nil {
		return ""
	}
	ver = strings.TrimSpace(ver)
	if ver == "" {
		return ""
	}
	if build != "" {
		return ver + "." + build
	}
	return ver
}
