package installer

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

// PreviewBackend is a safe, sandboxed Backend used automatically when running
// an installer package directly (for example under `ggui dev ./cmd/installer`)
// without an embedded payload. All file operations stay inside a temporary
// directory and shortcuts, registry entries, environment variables, PATH
// updates, and app launches are recorded in memory rather than modifying the
// host machine.
type PreviewBackend struct {
	mu        sync.Mutex
	root      string
	ownedRoot bool
	shortcuts map[string]string
	regEntry  *UninstallEntry
	env       map[string]map[string]string
	pathDirs  map[string][]string
	registry  map[RegistryRoot]map[string]map[string]RegistryValue
	launched  []string
}

// NewPreviewBackend creates a PreviewBackend backed by a temporary sandbox
// directory. Call Close when finished to remove the sandbox directory.
func NewPreviewBackend() *PreviewBackend {
	root, err := os.MkdirTemp("", "ggui-installer-preview-*")
	if err != nil {
		root = filepath.Join(os.TempDir(), "ggui-installer-preview")
		_ = os.MkdirAll(root, 0o755)
	}
	return &PreviewBackend{
		root:      root,
		ownedRoot: true,
		shortcuts: map[string]string{},
		env:       map[string]map[string]string{},
		pathDirs:  map[string][]string{},
		registry:  map[RegistryRoot]map[string]map[string]RegistryValue{},
	}
}

// Root returns the temporary sandbox root directory used by the preview backend.
func (p *PreviewBackend) Root() string { return p.root }

// Close removes the temporary sandbox directory created by NewPreviewBackend.
func (p *PreviewBackend) Close() error {
	if !p.ownedRoot || p.root == "" {
		return nil
	}
	return os.RemoveAll(p.root)
}

func (p *PreviewBackend) DefaultInstallDir(meta Metadata) string {
	name := meta.Name
	if name == "" {
		name = "App"
	}
	return filepath.Join(p.root, "Programs", name)
}

func (p *PreviewBackend) ExistingInstall(meta Metadata) (string, string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.regEntry != nil && p.regEntry.Meta.ID == meta.ID {
		return p.regEntry.InstallDir, p.regEntry.Meta.Version, true
	}
	return "", "", false
}

func (p *PreviewBackend) StartMenuDir(string) string { return filepath.Join(p.root, "StartMenu") }
func (p *PreviewBackend) DesktopDir(string) string   { return filepath.Join(p.root, "Desktop") }

func (p *PreviewBackend) CreateShortcut(lnkPath, targetPath, _, _, _ string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.shortcuts[lnkPath] = targetPath
	return nil
}

func (p *PreviewBackend) RemoveShortcut(lnkPath string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.shortcuts, lnkPath)
	return nil
}

func (p *PreviewBackend) NotifyShell() {}

func (p *PreviewBackend) WriteUninstallEntry(entry UninstallEntry) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	cp := entry
	p.regEntry = &cp
	return nil
}

func (p *PreviewBackend) DeleteUninstallEntry(Metadata) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.regEntry = nil
	return nil
}

func (p *PreviewBackend) GetEnv(scope, name string) (string, bool, error) {
	n, err := validateEnvName(name)
	if err != nil {
		return "", false, err
	}
	s := normalizeEnvScope(scope)
	p.mu.Lock()
	defer p.mu.Unlock()
	for k, v := range p.env[s] {
		if strings.EqualFold(k, n) {
			return v, true, nil
		}
	}
	return "", false, nil
}

func (p *PreviewBackend) SetEnv(scope, name, value string) error {
	n, err := validateEnvName(name)
	if err != nil {
		return err
	}
	s := normalizeEnvScope(scope)
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.env[s] == nil {
		p.env[s] = map[string]string{}
	}
	for k := range p.env[s] {
		if strings.EqualFold(k, n) && k != n {
			delete(p.env[s], k)
		}
	}
	p.env[s][n] = value
	return nil
}

func (p *PreviewBackend) DeleteEnv(scope, name string) error {
	n, err := validateEnvName(name)
	if err != nil {
		return err
	}
	s := normalizeEnvScope(scope)
	p.mu.Lock()
	defer p.mu.Unlock()
	for k := range p.env[s] {
		if strings.EqualFold(k, n) {
			delete(p.env[s], k)
		}
	}
	return nil
}

func (p *PreviewBackend) AddToPath(scope, dir string) error {
	s := normalizeEnvScope(scope)
	p.mu.Lock()
	defer p.mu.Unlock()
	cur := strings.Join(p.pathDirs[s], ";")
	next, changed := modifyPathList(cur, dir, true)
	if changed {
		p.pathDirs[s] = splitNonEmpty(next, ";")
	}
	return nil
}

func (p *PreviewBackend) RemoveFromPath(scope, dir string) error {
	s := normalizeEnvScope(scope)
	p.mu.Lock()
	defer p.mu.Unlock()
	cur := strings.Join(p.pathDirs[s], ";")
	next, changed := modifyPathList(cur, dir, false)
	if changed {
		p.pathDirs[s] = splitNonEmpty(next, ";")
	}
	return nil
}

// PathDirs returns a snapshot of the recorded PATH entries for scope.
func (p *PreviewBackend) PathDirs(scope string) []string {
	s := normalizeEnvScope(scope)
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.pathDirs[s])
}

func (p *PreviewBackend) findKeyLocked(root RegistryRoot, kp string) (string, bool) {
	keys := p.registry[root]
	if keys == nil {
		return "", false
	}
	for existing := range keys {
		if strings.EqualFold(existing, kp) {
			return existing, true
		}
	}
	return "", false
}

func (p *PreviewBackend) RegistryKeyExists(root RegistryRoot, keyPath string) (bool, error) {
	nr, err := normalizeRegistryRoot(root)
	if err != nil {
		return false, err
	}
	kp, err := normalizeRegistryKey(keyPath)
	if err != nil {
		return false, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	_, ok := p.findKeyLocked(nr, kp)
	return ok, nil
}

func (p *PreviewBackend) GetRegistryValue(root RegistryRoot, keyPath, name string) (RegistryValue, bool, error) {
	nr, err := normalizeRegistryRoot(root)
	if err != nil {
		return RegistryValue{}, false, err
	}
	kp, err := normalizeRegistryKey(keyPath)
	if err != nil {
		return RegistryValue{}, false, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	actualKey, ok := p.findKeyLocked(nr, kp)
	if !ok {
		return RegistryValue{}, false, nil
	}
	for k, v := range p.registry[nr][actualKey] {
		if strings.EqualFold(k, name) {
			return v, true, nil
		}
	}
	return RegistryValue{}, false, nil
}

func (p *PreviewBackend) SetRegistryValue(root RegistryRoot, keyPath, name string, val RegistryValue) error {
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
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.registry[nr] == nil {
		p.registry[nr] = map[string]map[string]RegistryValue{}
	}
	actualKey, ok := p.findKeyLocked(nr, kp)
	if !ok {
		actualKey = kp
		p.registry[nr][actualKey] = map[string]RegistryValue{}
	}
	for k := range p.registry[nr][actualKey] {
		if strings.EqualFold(k, name) && k != name {
			delete(p.registry[nr][actualKey], k)
		}
	}
	p.registry[nr][actualKey][name] = v
	return nil
}

func (p *PreviewBackend) DeleteRegistryValue(root RegistryRoot, keyPath, name string) error {
	nr, err := normalizeRegistryRoot(root)
	if err != nil {
		return err
	}
	kp, err := normalizeRegistryKey(keyPath)
	if err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	actualKey, ok := p.findKeyLocked(nr, kp)
	if !ok {
		return nil
	}
	for k := range p.registry[nr][actualKey] {
		if strings.EqualFold(k, name) {
			delete(p.registry[nr][actualKey], k)
		}
	}
	return nil
}

func (p *PreviewBackend) DeleteRegistryKey(root RegistryRoot, keyPath string, onlyIfEmpty bool) error {
	nr, err := normalizeRegistryRoot(root)
	if err != nil {
		return err
	}
	kp, err := normalizeRegistryKey(keyPath)
	if err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	keys := p.registry[nr]
	if keys == nil {
		return nil
	}
	prefix := strings.ToLower(kp) + `\`
	if onlyIfEmpty {
		actualKey, ok := p.findKeyLocked(nr, kp)
		if !ok {
			return nil
		}
		if len(keys[actualKey]) > 0 {
			return nil
		}
		for k := range keys {
			if strings.HasPrefix(strings.ToLower(k), prefix) {
				return nil
			}
		}
		delete(keys, actualKey)
		return nil
	}
	for k := range keys {
		lower := strings.ToLower(k)
		if lower == strings.ToLower(kp) || strings.HasPrefix(lower, prefix) {
			delete(keys, k)
		}
	}
	return nil
}

func (p *PreviewBackend) Launch(exePath string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.launched = append(p.launched, exePath)
	return nil
}

func (p *PreviewBackend) SelfExe() (string, error)                        { return "", os.ErrNotExist }
func (p *PreviewBackend) RelaunchForUninstall(string, bool) (bool, error) { return false, nil }
func (p *PreviewBackend) ScheduleTempDelete(string)                       {}

func splitNonEmpty(s, sep string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for part := range strings.SplitSeq(s, sep) {
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
