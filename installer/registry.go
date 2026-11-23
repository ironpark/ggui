package installer

import (
	"fmt"
	"slices"
	"strings"
)

// RegistryRoot identifies a Windows Registry root hive.
type RegistryRoot string

const (
	// RegistryHKCU targets HKEY_CURRENT_USER (per-user, no elevation needed).
	RegistryHKCU RegistryRoot = "HKCU"
	// RegistryHKLM targets HKEY_LOCAL_MACHINE (machine-wide, requires admin).
	RegistryHKLM RegistryRoot = "HKLM"
	// RegistryHKCR targets HKEY_CLASSES_ROOT (file associations / protocols).
	RegistryHKCR RegistryRoot = "HKCR"
)

// RegistryValueType identifies the Windows Registry value representation.
type RegistryValueType string

const (
	RegSZ       RegistryValueType = "SZ"
	RegExpandSZ RegistryValueType = "EXPAND_SZ"
	RegDWORD    RegistryValueType = "DWORD"
	RegQWORD    RegistryValueType = "QWORD"
	RegBinary   RegistryValueType = "BINARY"
	RegMultiSZ  RegistryValueType = "MULTI_SZ"
)

// RegistryValue holds a typed Windows Registry value.
type RegistryValue struct {
	Type   RegistryValueType
	String string
	DWORD  uint32
	QWORD  uint64
	Binary []byte
	Multi  []string
}

// StringValue returns a REG_SZ RegistryValue.
func StringValue(s string) RegistryValue { return RegistryValue{Type: RegSZ, String: s} }

// ExpandStringValue returns a REG_EXPAND_SZ RegistryValue (environment
// variables such as %LOCALAPPDATA% are expanded by Windows consumers).
func ExpandStringValue(s string) RegistryValue { return RegistryValue{Type: RegExpandSZ, String: s} }

// DWORDValue returns a 32-bit REG_DWORD RegistryValue.
func DWORDValue(v uint32) RegistryValue { return RegistryValue{Type: RegDWORD, DWORD: v} }

// QWORDValue returns a 64-bit REG_QWORD RegistryValue.
func QWORDValue(v uint64) RegistryValue { return RegistryValue{Type: RegQWORD, QWORD: v} }

// BinaryValue returns a REG_BINARY RegistryValue.
func BinaryValue(b []byte) RegistryValue {
	return RegistryValue{Type: RegBinary, Binary: slices.Clone(b)}
}

// MultiStringValue returns a REG_MULTI_SZ RegistryValue.
func MultiStringValue(ss []string) RegistryValue {
	return RegistryValue{Type: RegMultiSZ, Multi: slices.Clone(ss)}
}

// RegistryEntry declares a Windows Registry value to write during installation
// (via WindowsConfig.Registry) and remove automatically on uninstall.
type RegistryEntry struct {
	// Root is RegistryHKCU, RegistryHKLM, or RegistryHKCR. Empty defaults to
	// RegistryHKLM when Scope == ScopeMachine, else RegistryHKCU.
	Root RegistryRoot
	// Key is the subkey path (for example `Software\Example\MyApp`).
	Key string
	// Name is the value name; empty string writes the key's default value.
	Name string
	// Value is the typed value to write. String / ExpandString / MultiString
	// values may reference %INSTALL_DIR%, %EXE_PATH%, %APP_NAME%, %APP_ID%,
	// and %VERSION%.
	Value RegistryValue
}

// ManifestEnvEntry records an environment variable written during install so
// Uninstall removes it cleanly.
type ManifestEnvEntry struct {
	Scope string `json:"scope"`
	Name  string `json:"name"`
}

// ManifestPathEntry records a PATH directory added during install so
// Uninstall removes it cleanly.
type ManifestPathEntry struct {
	Scope string `json:"scope"`
	Dir   string `json:"dir"`
}

// ManifestRegistryEntry records a Windows Registry value written during
// install so Uninstall removes it cleanly (and deletes Key only if it did not
// exist before install and is empty after removing tracked values).
type ManifestRegistryEntry struct {
	Root      RegistryRoot `json:"root"`
	Key       string       `json:"key"`
	Name      string       `json:"name"`
	DeleteKey bool         `json:"delete_key,omitempty"`
}

func defaultRegistryRoot(scope string) RegistryRoot {
	if strings.EqualFold(strings.TrimSpace(scope), ScopeMachine) {
		return RegistryHKLM
	}
	return RegistryHKCU
}

func normalizeRegistryRoot(root RegistryRoot) (RegistryRoot, error) {
	switch RegistryRoot(strings.ToUpper(strings.TrimSpace(string(root)))) {
	case RegistryHKCU, "HKEY_CURRENT_USER":
		return RegistryHKCU, nil
	case RegistryHKLM, "HKEY_LOCAL_MACHINE":
		return RegistryHKLM, nil
	case RegistryHKCR, "HKEY_CLASSES_ROOT":
		return RegistryHKCR, nil
	default:
		return "", fmt.Errorf("installer: unsupported registry root %q (use HKCU, HKLM, or HKCR)", root)
	}
}

func normalizeRegistryKey(keyPath string) (string, error) {
	k := strings.TrimSpace(keyPath)
	k = strings.ReplaceAll(k, "/", `\`)
	k = strings.Trim(k, `\`)
	if k == "" || strings.ContainsRune(k, 0) {
		return "", fmt.Errorf("installer: invalid registry key path %q", keyPath)
	}
	return k, nil
}

func normalizeEnvScope(scope string) string {
	if strings.EqualFold(strings.TrimSpace(scope), ScopeMachine) {
		return ScopeMachine
	}
	return ScopeUser
}

func validateEnvName(name string) (string, error) {
	n := strings.TrimSpace(name)
	if n == "" || strings.ContainsAny(n, "=\x00") {
		return "", fmt.Errorf("installer: invalid environment variable name %q", name)
	}
	return n, nil
}

func normalizeRegistryValue(v RegistryValue) (RegistryValue, error) {
	if v.Type == "" {
		v.Type = RegSZ
	}
	switch v.Type {
	case RegSZ, RegExpandSZ, RegDWORD, RegQWORD, RegBinary, RegMultiSZ:
		return v, nil
	default:
		return RegistryValue{}, fmt.Errorf("installer: unsupported registry value type %q", v.Type)
	}
}

func expandInstallTokens(s string, meta Metadata, absDir, exePath string) string {
	if !strings.Contains(s, "%") {
		return s
	}
	r := strings.NewReplacer(
		"%INSTALL_DIR%", absDir,
		"%EXE_PATH%", exePath,
		"%APP_NAME%", meta.Name,
		"%APP_ID%", meta.ID,
		"%VERSION%", meta.Version,
	)
	return r.Replace(s)
}

func expandRegistryValue(v RegistryValue, meta Metadata, absDir, exePath string) RegistryValue {
	switch v.Type {
	case "", RegSZ, RegExpandSZ:
		v.String = expandInstallTokens(v.String, meta, absDir, exePath)
	case RegMultiSZ:
		if len(v.Multi) > 0 {
			out := make([]string, len(v.Multi))
			for i, item := range v.Multi {
				out[i] = expandInstallTokens(item, meta, absDir, exePath)
			}
			v.Multi = out
		}
	}
	return v
}
