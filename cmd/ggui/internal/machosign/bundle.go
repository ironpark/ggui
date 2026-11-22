package machosign

import (
	"crypto/sha1" //nolint:gosec // G505, G401: CodeResources version 1 "files" requires SHA-1
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/ironpark/zapp/pkg/plist"
)

// rules and rules2 are the resource rules codesign seals an app bundle
// with when it is given none, for macOS before 10.9 and after: which
// files of Contents are hashed, which may be missing, which are left
// out, and where nested code lives.
var (
	rules = map[string]any{
		`^Resources/`:                            true,
		`^Resources/.*\.lproj/`:                  map[string]any{"optional": true, "weight": 1000.0},
		`^Resources/.*\.lproj/locversion.plist$`: map[string]any{"omit": true, "weight": 1100.0},
		`^Resources/Base\.lproj/`:                map[string]any{"weight": 1010.0},
		`^version.plist$`:                        true,
	}
	rules2 = map[string]any{
		`.*\.dSYM($|/)`:      map[string]any{"weight": 11.0},
		`^(.*/)?\.DS_Store$`: map[string]any{"omit": true, "weight": 2000.0},
		`^(Frameworks|SharedFrameworks|PlugIns|Plug-ins|XPCServices|Helpers|MacOS|Library/(Automator|Spotlight|LoginItems))/`: map[string]any{"nested": true, "weight": 10.0},
		`^.*`:                                    true,
		`^Info\.plist$`:                          map[string]any{"omit": true, "weight": 20.0},
		`^PkgInfo$`:                              map[string]any{"omit": true, "weight": 20.0},
		`^Resources/`:                            map[string]any{"weight": 20.0},
		`^Resources/.*\.lproj/`:                  map[string]any{"optional": true, "weight": 1000.0},
		`^Resources/.*\.lproj/locversion.plist$`: map[string]any{"omit": true, "weight": 1100.0},
		`^Resources/Base\.lproj/`:                map[string]any{"weight": 1010.0},
		`^[^/]+$`:                                map[string]any{"nested": true, "weight": 10.0},
		`^embedded\.provisionprofile$`:           map[string]any{"weight": 20.0},
		`^version\.plist$`:                       map[string]any{"weight": 20.0},
	}
)

// rule is one resource rule: the files it matches, how much it counts
// against the others that match, and what it makes of them.
type rule struct {
	re                     *regexp.Regexp
	weight                 float64
	omit, optional, nested bool
}

func compileRules(m map[string]any) []rule {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	out := make([]rule, len(keys))
	for i, k := range keys {
		r := rule{re: regexp.MustCompile(k), weight: 1}
		if d, ok := m[k].(map[string]any); ok {
			if w, ok := d["weight"].(float64); ok {
				r.weight = w
			}
			r.omit, _ = d["omit"].(bool)
			r.optional, _ = d["optional"].(bool)
			r.nested, _ = d["nested"].(bool)
		}
		out[i] = r
	}
	return out
}

// match returns the heaviest rule matching name, a path in Contents.
func match(rules []rule, name string) (rule, bool) {
	var best rule
	found := false
	for _, r := range rules {
		if r.re.MatchString(name) && (!found || r.weight > best.weight) {
			best, found = r, true
		}
	}
	return best, found
}

// SignBundle signs the app bundle at app ad hoc, as codesign --force
// --sign - [--entitlements] does: the programs nested in Contents/MacOS,
// Contents/Frameworks and the other places for code first, each in its
// own right, then the seal of the bundle's resources,
// Contents/_CodeSignature/CodeResources, and last the main program,
// whose signature binds Info.plist and the seal and carries the
// entitlements. The identifier is the bundle's CFBundleIdentifier.
//
// Nested bundles, such as frameworks or helper apps, are not signed: a
// bundle holding one is refused.
func SignBundle(app string, entitlements []byte) error {
	contents := filepath.Join(app, "Contents")
	info, err := os.ReadFile(filepath.Join(contents, "Info.plist"))
	if err != nil {
		return fmt.Errorf("%s is not an app bundle: %w", app, err)
	}
	d, err := plist.ParseDict(info)
	if err != nil {
		return fmt.Errorf("%s: Info.plist: %w", app, err)
	}
	id, _ := d["CFBundleIdentifier"].(string)
	exe, _ := d["CFBundleExecutable"].(string)
	if id == "" || exe == "" {
		return fmt.Errorf("%s: Info.plist needs CFBundleIdentifier and CFBundleExecutable", app)
	}
	main := path.Join("MacOS", exe)

	seal, err := sealResources(contents, main)
	if err != nil {
		return fmt.Errorf("%s: %w", app, err)
	}
	dir := filepath.Join(contents, "_CodeSignature")
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "CodeResources"), seal, 0o644); err != nil {
		return err
	}
	return SignFile(filepath.Join(contents, filepath.FromSlash(main)), Options{
		Identifier: id, Entitlements: entitlements, InfoPlist: info, Resources: seal,
	})
}

// bundleExts are the extensions of bundles, which nested code may be
// and which SignBundle does not sign.
var bundleExts = []string{".app", ".framework", ".bundle", ".plugin", ".xpc", ".appex", ".kext", ".qlgenerator", ".mdimporter"}

// sealResources signs the nested code in contents and returns the seal
// of the bundle's files, CodeResources, leaving out main, the main
// program.
func sealResources(contents, main string) ([]byte, error) {
	v1, v2 := compileRules(rules), compileRules(rules2)
	files, files2 := map[string]any{}, map[string]any{}
	err := filepath.WalkDir(contents, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(contents, p)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(rel)
		switch {
		case name == ".":
			return nil
		case name == "_CodeSignature" && e.IsDir():
			return filepath.SkipDir
		case name == main:
			return nil
		}
		r2, ok2 := match(v2, name)
		if e.IsDir() {
			if ok2 && r2.nested && slices.Contains(bundleExts, strings.ToLower(path.Ext(name))) {
				return fmt.Errorf("nested bundle Contents/%s: only nested programs are signed, not bundles; sign the app on a Mac with codesign --deep, or leave it out", name)
			}
			return nil
		}
		if e.Type()&fs.ModeSymlink != 0 {
			if ok2 && !r2.omit {
				target, err := os.Readlink(p)
				if err != nil {
					return err
				}
				// The seal holds the target as macOS reads it, with
				// slashes: Windows keeps a link's target with
				// backslashes, and Readlink gives it back that way.
				files2[name] = withOptional(map[string]any{"symlink": filepath.ToSlash(target)}, r2.optional)
			}
			return nil
		}
		if !e.Type().IsRegular() {
			return fmt.Errorf("sealing Contents/%s: neither a file, a directory nor a symbolic link", name)
		}
		if ok2 && r2.nested && !r2.omit {
			nested, err := signNested(p)
			if err != nil {
				return fmt.Errorf("signing Contents/%s: %w", name, err)
			}
			files2[name] = nested
		}
		data, err := os.ReadFile(p) //nolint:gosec // G122: signs a bundle this process just assembled
		if err != nil {
			return err
		}
		if r1, ok := match(v1, name); ok && !r1.omit {
			h := sha1.Sum(data) //nolint:gosec // G505, G401: CodeResources version 1 "files" requires SHA-1
			if r1.optional {
				files[name] = map[string]any{"hash": h[:], "optional": true}
			} else {
				files[name] = h[:]
			}
		}
		if ok2 && !r2.omit && !r2.nested {
			h := sha256.Sum256(data)
			files2[name] = withOptional(map[string]any{"hash2": h[:]}, r2.optional)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return plist.MarshalXML(map[string]any{"files": files, "files2": files2, "rules": rules, "rules2": rules2})
}

func withOptional(d map[string]any, optional bool) map[string]any {
	if optional {
		d["optional"] = true
	}
	return d
}

// signNested signs the program at p, found where a bundle keeps code,
// and returns its entry in the seal: its CDHash and the requirement that
// it be the code with that hash, or for a universal program, any of its
// slices' hashes, the arm64 one first.
func signNested(p string) (map[string]any, error) {
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	if !IsMachO(data) {
		return nil, errors.New("is in a place for code but is not a program; codesign refuses it too: move it to Contents/Resources")
	}
	signed, err := Sign(data, Options{Identifier: nestedIdentifier(filepath.Base(p), data)})
	if err != nil {
		return nil, err
	}
	hashes, err := cdHashes(signed)
	if err != nil {
		return nil, err
	}
	if len(hashes) == 0 {
		return nil, errors.New("signed binary produced no code directory hash")
	}
	if err := replaceFile(p, signed); err != nil {
		return nil, err
	}
	reqs := make([]string, len(hashes))
	for i, h := range hashes {
		reqs[i] = fmt.Sprintf(`cdhash H"%x"`, h.hash)
	}
	return map[string]any{"cdhash": hashes[0].hash, "requirement": strings.Join(reqs, " or ")}, nil
}

// nestedIdentifier is the identifier codesign gives a program it signs
// without one: its name without the extension and, when that has no dot
// to make it look like a reverse domain name, made unique with the
// program's UUID.
func nestedIdentifier(name string, data []byte) string {
	id := strings.TrimSuffix(name, path.Ext(name))
	if strings.Contains(id, ".") {
		return id
	}
	if uuid := programUUID(data); uuid != nil {
		return id + "-" + hex.EncodeToString(append([]byte("UUID"), uuid...))
	}
	return id
}

// programUUID returns the UUID of a thin program, or of a universal
// one's arm64 slice or else its first.
func programUUID(data []byte) []byte {
	if !isFat(data) {
		if t, err := parseThin(data); err == nil {
			return t.uuid
		}
		return nil
	}
	ss, err := fatSlices(data)
	if err != nil || len(ss) == 0 {
		return nil
	}
	pick := ss[0]
	for _, s := range ss {
		if s.cpu == cpuARM64 {
			pick = s
		}
	}
	return programUUID(pick.data)
}
