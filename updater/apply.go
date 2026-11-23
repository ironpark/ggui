package updater

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
)

const (
	// defaultMaxExtractBytes caps total uncompressed bytes written when extracting
	// an update archive (2 GiB) to guard against zip/gzip bombs.
	defaultMaxExtractBytes int64 = 2 << 30

	// maxExtractEntries caps total archive entries extracted to guard against inode bombs.
	maxExtractEntries = 200_000

	// maxSymlinkTargetBytes caps the length of symlink targets inside archives.
	maxSymlinkTargetBytes = 4096

	// maxSymlinkHops caps symlink resolution depth during post-extraction audits.
	maxSymlinkHops = 40
)

func init() {
	defaultApplierFunc = DefaultApplier
}

// DefaultApplier returns the host platform's default update Applier.
func DefaultApplier() Applier {
	return newPlatformApplier()
}

// isSubpath reports whether target resides at or inside root after path
// canonicalization. It rejects empty paths, NUL bytes, and parent escapes.
func isSubpath(root, target string) bool {
	if root == "" || target == "" {
		return false
	}
	if strings.ContainsRune(root, '\x00') || strings.ContainsRune(target, '\x00') {
		return false
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absRoot, absTarget)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return false
	}
	return true
}

// copyFileAtomic copies src to dst with permissions mode via a temporary file
// in dst's directory and an atomic os.Rename. If mode is 0, src's permission
// bits are preserved (defaulting to 0644).
func copyFileAtomic(src, dst string, mode os.FileMode) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("updater: opening %s: %w", src, err)
	}
	defer srcFile.Close()

	if mode == 0 {
		fi, err := srcFile.Stat()
		if err != nil {
			return fmt.Errorf("updater: stating %s: %w", src, err)
		}
		mode = fi.Mode().Perm()
		if mode == 0 {
			mode = 0o644
		}
	}
	mode = mode.Perm()

	dstDir := filepath.Dir(dst)
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return fmt.Errorf("updater: creating directory %s: %w", dstDir, err)
	}

	tmp, err := os.CreateTemp(dstDir, "."+filepath.Base(dst)+".tmp-*")
	if err != nil {
		return fmt.Errorf("updater: creating temp file in %s: %w", dstDir, err)
	}
	tmpPath := tmp.Name()
	cleanup := true
	closed := false
	defer func() {
		if !closed {
			_ = tmp.Close()
		}
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := io.Copy(tmp, srcFile); err != nil {
		return fmt.Errorf("updater: copying %s to %s: %w", src, dst, err)
	}
	if err := tmp.Chmod(mode); err != nil {
		return fmt.Errorf("updater: setting mode on %s: %w", tmpPath, err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("updater: syncing %s: %w", tmpPath, err)
	}
	if err := tmp.Close(); err != nil {
		closed = true
		return fmt.Errorf("updater: closing %s: %w", tmpPath, err)
	}
	closed = true

	if err := os.Rename(tmpPath, dst); err != nil {
		return fmt.Errorf("updater: moving %s into place at %s: %w", tmpPath, dst, err)
	}
	cleanup = false
	return nil
}

// removeAllForce removes path and any children it contains, first restoring
// owner read/write/exec permissions on non-writable directories (and write bits
// on non-writable regular files) without following symlinks so read-only
// extracted trees can always be cleaned up.
func removeAllForce(path string) error {
	if path == "" {
		return nil
	}
	fi, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return os.RemoveAll(path)
	}
	if fi.Mode()&fs.ModeSymlink != 0 {
		return os.Remove(path)
	}
	if fi.IsDir() {
		if fi.Mode().Perm()&0o700 != 0o700 {
			_ = os.Chmod(path, fi.Mode().Perm()|0o700)
		}
		_ = filepath.WalkDir(path, func(p string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil || d == nil {
				return nil
			}
			if d.Type()&fs.ModeSymlink != 0 {
				return nil
			}
			info, infoErr := d.Info()
			if infoErr != nil {
				return nil
			}
			perm := info.Mode().Perm()
			if d.IsDir() {
				if perm&0o700 != 0o700 {
					_ = os.Chmod(p, perm|0o700)
				}
			} else if perm&0o200 == 0 {
				_ = os.Chmod(p, perm|0o200)
			}
			return nil
		})
	} else if fi.Mode().Perm()&0o200 == 0 {
		_ = os.Chmod(path, fi.Mode().Perm()|0o200)
	}
	return os.RemoveAll(path)
}

// copyDirPreserving recursively copies srcDir to dstDir, preserving regular
// file permissions, directory hierarchy, and validated internal symlinks while
// enforcing size, entry-count, and symlink-containment limits.
func copyDirPreserving(srcDir, dstDir string) error {
	return copyDirSafeWithLimit(srcDir, dstDir, defaultMaxExtractBytes, maxExtractEntries)
}

func copyDirSafeWithLimit(srcDir, dstDir string, maxBytes int64, maxEntries int) error {
	if maxBytes <= 0 {
		maxBytes = defaultMaxExtractBytes
	}
	if maxEntries <= 0 {
		maxEntries = maxExtractEntries
	}
	srcInfo, err := os.Lstat(srcDir)
	if err != nil {
		return fmt.Errorf("updater: stating %s: %w", srcDir, err)
	}
	if !srcInfo.IsDir() {
		return fmt.Errorf("updater: %s is not a directory", srcDir)
	}

	type dirModeFix struct {
		path string
		mode fs.FileMode
	}
	var dirFixes []dirModeFix

	srcPerm := srcInfo.Mode().Perm()
	if srcPerm == 0 {
		srcPerm = 0o755
	}
	if err := os.MkdirAll(dstDir, srcPerm|0o700); err != nil {
		return fmt.Errorf("updater: creating %s: %w", dstDir, err)
	}
	realRoot, err := prepareExtractRoot(dstDir)
	if err != nil {
		return err
	}
	if srcPerm != (srcPerm | 0o700) {
		dirFixes = append(dirFixes, dirModeFix{path: realRoot, mode: srcPerm})
	}

	var totalWritten int64
	var entries int

	err = filepath.WalkDir(srcDir, func(cur string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if cur == srcDir {
			return nil
		}
		entries++
		if entries > maxEntries {
			return fmt.Errorf("updater: directory copy exceeds maximum entry count (%d)", maxEntries)
		}

		rel, err := filepath.Rel(srcDir, cur)
		if err != nil {
			return err
		}
		dst, isRoot, err := validateArchiveEntryPath(realRoot, filepath.ToSlash(rel))
		if err != nil {
			return fmt.Errorf("updater: invalid path %q: %w", rel, err)
		}
		if isRoot {
			return nil
		}

		info, err := os.Lstat(cur)
		if err != nil {
			return err
		}
		mode := info.Mode()

		switch {
		case mode&fs.ModeSymlink != 0:
			if err := ensureSafeParent(realRoot, dst); err != nil {
				return fmt.Errorf("updater: symlink %q: %w", rel, err)
			}
			target, err := os.Readlink(cur)
			if err != nil {
				return fmt.Errorf("updater: reading symlink %s: %w", cur, err)
			}
			if len(target) > maxSymlinkTargetBytes {
				return fmt.Errorf("updater: symlink %q target too long", rel)
			}
			if err := validateSymlinkTarget(realRoot, dst, target); err != nil {
				return fmt.Errorf("updater: symlink %q -> %q: %w", rel, target, err)
			}
			if err := removeExistingNonDirOrSymlink(dst); err != nil {
				return err
			}
			if err := os.Symlink(target, dst); err != nil {
				return fmt.Errorf("updater: creating symlink %s: %w", dst, err)
			}
			if err := resolveLinkWithinRoot(realRoot, dst); err != nil {
				_ = os.Remove(dst)
				return fmt.Errorf("updater: symlink %q -> %q escapes destination: %w", rel, target, err)
			}
			return nil

		case mode.IsDir():
			if err := ensureSafeParent(realRoot, dst); err != nil {
				return fmt.Errorf("updater: dir %q: %w", rel, err)
			}
			if err := rejectExistingSymlink(dst); err != nil {
				return fmt.Errorf("updater: dir %q: %w", rel, err)
			}
			perm := mode.Perm()
			if perm == 0 {
				perm = 0o755
			}
			if err := os.MkdirAll(dst, perm|0o700); err != nil {
				return err
			}
			if perm != (perm | 0o700) {
				dirFixes = append(dirFixes, dirModeFix{path: dst, mode: perm})
			}
			return nil

		case mode.IsRegular():
			if err := ensureSafeParent(realRoot, dst); err != nil {
				return fmt.Errorf("updater: file %q: %w", rel, err)
			}
			if err := rejectExistingSymlink(dst); err != nil {
				return fmt.Errorf("updater: file %q: %w", rel, err)
			}
			remaining := maxBytes - totalWritten
			if remaining < 0 || info.Size() > remaining {
				return fmt.Errorf("updater: directory copy exceeds maximum allowed size (%d bytes)", maxBytes)
			}
			perm := mode.Perm()
			if perm == 0 {
				perm = 0o644
			}
			in, err := os.Open(cur)
			if err != nil {
				return err
			}
			written, writeErr := writeExtractedFile(in, dst, perm, remaining)
			_ = in.Close()
			if writeErr != nil {
				return fmt.Errorf("updater: copying file %q: %w", rel, writeErr)
			}
			totalWritten += written
			return nil

		default:
			return fmt.Errorf("updater: unsupported file mode %v for %s", mode, cur)
		}
	})
	if err != nil {
		return err
	}

	if err := verifyExtractedSymlinks(realRoot); err != nil {
		return err
	}

	// Restore read-only directory permissions deepest-first.
	for i := len(dirFixes) - 1; i >= 0; i-- {
		_ = os.Chmod(dirFixes[i].path, dirFixes[i].mode)
	}
	return nil
}

// extractZipSafe extracts zipPath into dstDir while blocking zip-slip path
// traversal, symlink escapes, and uncompressed size bombs.
func extractZipSafe(zipPath, dstDir string) error {
	return extractZipSafeWithLimit(zipPath, dstDir, defaultMaxExtractBytes)
}

func extractZipSafeWithLimit(zipPath, dstDir string, maxBytes int64) error {
	if maxBytes <= 0 {
		maxBytes = defaultMaxExtractBytes
	}
	realRoot, err := prepareExtractRoot(dstDir)
	if err != nil {
		return err
	}

	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("updater: opening zip %s: %w", zipPath, err)
	}
	defer zr.Close()

	if len(zr.File) > maxExtractEntries {
		return fmt.Errorf("updater: zip contains too many entries (%d > %d)", len(zr.File), maxExtractEntries)
	}

	type dirPermFix struct {
		path string
		perm fs.FileMode
	}
	var dirFixes []dirPermFix
	var totalWritten int64

	for _, f := range zr.File {
		target, isRoot, err := validateArchiveEntryPath(realRoot, f.Name)
		if err != nil {
			return fmt.Errorf("updater: invalid zip entry %q: %w", f.Name, err)
		}
		if isRoot {
			continue
		}

		mode := f.Mode()
		switch {
		case mode&fs.ModeSymlink != 0:
			if err := ensureSafeParent(realRoot, target); err != nil {
				return fmt.Errorf("updater: zip symlink %q: %w", f.Name, err)
			}
			rc, err := f.Open()
			if err != nil {
				return fmt.Errorf("updater: opening zip symlink %q: %w", f.Name, err)
			}
			linkBytes, readErr := io.ReadAll(io.LimitReader(rc, maxSymlinkTargetBytes+1))
			_ = rc.Close()
			if readErr != nil {
				return fmt.Errorf("updater: reading zip symlink %q: %w", f.Name, readErr)
			}
			if len(linkBytes) > maxSymlinkTargetBytes {
				return fmt.Errorf("updater: zip symlink %q target too long", f.Name)
			}
			linkTarget := string(linkBytes)
			if err := validateSymlinkTarget(realRoot, target, linkTarget); err != nil {
				return fmt.Errorf("updater: zip symlink %q -> %q: %w", f.Name, linkTarget, err)
			}
			if err := removeExistingNonDirOrSymlink(target); err != nil {
				return err
			}
			if err := os.Symlink(linkTarget, target); err != nil {
				return fmt.Errorf("updater: creating symlink %q: %w", f.Name, err)
			}
			if err := resolveLinkWithinRoot(realRoot, target); err != nil {
				_ = os.Remove(target)
				return fmt.Errorf("updater: zip symlink %q -> %q: %w", f.Name, linkTarget, err)
			}

		case f.FileInfo().IsDir() || strings.HasSuffix(f.Name, "/"):
			if err := ensureSafeParent(realRoot, target); err != nil {
				return fmt.Errorf("updater: zip dir %q: %w", f.Name, err)
			}
			if err := rejectExistingSymlink(target); err != nil {
				return fmt.Errorf("updater: zip dir %q: %w", f.Name, err)
			}
			perm := mode.Perm()
			if perm == 0 {
				perm = 0o755
			}
			if err := os.MkdirAll(target, perm|0o700); err != nil {
				return fmt.Errorf("updater: creating dir %q: %w", f.Name, err)
			}
			if perm != (perm | 0o700) {
				dirFixes = append(dirFixes, dirPermFix{path: target, perm: perm})
			}

		default:
			if err := ensureSafeParent(realRoot, target); err != nil {
				return fmt.Errorf("updater: zip file %q: %w", f.Name, err)
			}
			if err := rejectExistingSymlink(target); err != nil {
				return fmt.Errorf("updater: zip file %q: %w", f.Name, err)
			}
			remaining := maxBytes - totalWritten
			if remaining < 0 || f.UncompressedSize64 > uint64(remaining) {
				return fmt.Errorf("updater: zip extraction exceeds maximum allowed size (%d bytes)", maxBytes)
			}
			rc, err := f.Open()
			if err != nil {
				return fmt.Errorf("updater: opening zip file %q: %w", f.Name, err)
			}
			perm := mode.Perm()
			if perm == 0 {
				perm = 0o644
			}
			written, writeErr := writeExtractedFile(rc, target, perm, remaining)
			_ = rc.Close()
			if writeErr != nil {
				return fmt.Errorf("updater: extracting zip file %q: %w", f.Name, writeErr)
			}
			totalWritten += written
		}
	}

	if err := verifyExtractedSymlinks(realRoot); err != nil {
		return err
	}
	for i := len(dirFixes) - 1; i >= 0; i-- {
		_ = os.Chmod(dirFixes[i].path, dirFixes[i].perm)
	}
	return nil
}

// extractTarGzSafe extracts a gzipped tar archive into dstDir while blocking
// path traversal, symlink/hardlink escapes, unsafe device entries, and size bombs.
func extractTarGzSafe(tgzPath, dstDir string) error {
	return extractTarGzSafeWithLimit(tgzPath, dstDir, defaultMaxExtractBytes)
}

func extractTarGzSafeWithLimit(tgzPath, dstDir string, maxBytes int64) error {
	if maxBytes <= 0 {
		maxBytes = defaultMaxExtractBytes
	}
	realRoot, err := prepareExtractRoot(dstDir)
	if err != nil {
		return err
	}

	f, err := os.Open(tgzPath)
	if err != nil {
		return fmt.Errorf("updater: opening tar.gz %s: %w", tgzPath, err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("updater: reading gzip stream %s: %w", tgzPath, err)
	}
	defer gz.Close()

	streamLimited := &io.LimitedReader{R: gz, N: tarStreamLimit(maxBytes)}
	tr := tar.NewReader(streamLimited)

	type dirPermFix struct {
		path string
		perm fs.FileMode
	}
	var dirFixes []dirPermFix
	var totalWritten int64
	var entries int

	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			if streamLimited.N <= 0 {
				return fmt.Errorf("updater: tar.gz decompressed stream exceeds maximum allowed size (%d bytes)", maxBytes)
			}
			break
		}
		if err != nil {
			if streamLimited.N <= 0 {
				return fmt.Errorf("updater: tar.gz decompressed stream exceeds maximum allowed size (%d bytes)", maxBytes)
			}
			return fmt.Errorf("updater: reading tar entry: %w", err)
		}
		entries++
		if entries > maxExtractEntries {
			return fmt.Errorf("updater: tar.gz contains too many entries (> %d)", maxExtractEntries)
		}

		// Ignore PAX extended metadata headers that archive/tar surfaces.
		if hdr.Typeflag == tar.TypeXHeader || hdr.Typeflag == tar.TypeXGlobalHeader {
			continue
		}

		target, isRoot, err := validateArchiveEntryPath(realRoot, hdr.Name)
		if err != nil {
			return fmt.Errorf("updater: invalid tar entry %q: %w", hdr.Name, err)
		}
		if isRoot {
			continue
		}

		perm := fs.FileMode(hdr.Mode).Perm()
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := ensureSafeParent(realRoot, target); err != nil {
				return fmt.Errorf("updater: tar dir %q: %w", hdr.Name, err)
			}
			if err := rejectExistingSymlink(target); err != nil {
				return fmt.Errorf("updater: tar dir %q: %w", hdr.Name, err)
			}
			if perm == 0 {
				perm = 0o755
			}
			if err := os.MkdirAll(target, perm|0o700); err != nil {
				return fmt.Errorf("updater: creating dir %q: %w", hdr.Name, err)
			}
			if perm != (perm | 0o700) {
				dirFixes = append(dirFixes, dirPermFix{path: target, perm: perm})
			}

		case tar.TypeReg:
			if hdr.Size < 0 {
				return fmt.Errorf("updater: tar file %q has negative size %d", hdr.Name, hdr.Size)
			}
			if err := ensureSafeParent(realRoot, target); err != nil {
				return fmt.Errorf("updater: tar file %q: %w", hdr.Name, err)
			}
			if err := rejectExistingSymlink(target); err != nil {
				return fmt.Errorf("updater: tar file %q: %w", hdr.Name, err)
			}
			remaining := maxBytes - totalWritten
			if remaining < 0 || hdr.Size > remaining {
				return fmt.Errorf("updater: tar.gz extraction exceeds maximum allowed size (%d bytes)", maxBytes)
			}
			if perm == 0 {
				perm = 0o644
			}
			written, err := writeExtractedFile(tr, target, perm, remaining)
			if err != nil {
				return fmt.Errorf("updater: extracting tar file %q: %w", hdr.Name, err)
			}
			totalWritten += written

		case tar.TypeSymlink:
			if err := ensureSafeParent(realRoot, target); err != nil {
				return fmt.Errorf("updater: tar symlink %q: %w", hdr.Name, err)
			}
			if len(hdr.Linkname) > maxSymlinkTargetBytes {
				return fmt.Errorf("updater: tar symlink %q target too long", hdr.Name)
			}
			if err := validateSymlinkTarget(realRoot, target, hdr.Linkname); err != nil {
				return fmt.Errorf("updater: tar symlink %q -> %q: %w", hdr.Name, hdr.Linkname, err)
			}
			if err := removeExistingNonDirOrSymlink(target); err != nil {
				return err
			}
			if err := os.Symlink(hdr.Linkname, target); err != nil {
				return fmt.Errorf("updater: creating symlink %q: %w", hdr.Name, err)
			}
			if err := resolveLinkWithinRoot(realRoot, target); err != nil {
				_ = os.Remove(target)
				return fmt.Errorf("updater: tar symlink %q -> %q: %w", hdr.Name, hdr.Linkname, err)
			}

		case tar.TypeLink:
			if err := ensureSafeParent(realRoot, target); err != nil {
				return fmt.Errorf("updater: tar hardlink %q: %w", hdr.Name, err)
			}
			linkDst, linkIsRoot, err := validateArchiveEntryPath(realRoot, hdr.Linkname)
			if err != nil || linkIsRoot {
				return fmt.Errorf("updater: invalid tar hardlink target %q", hdr.Linkname)
			}
			if err := resolveLinkWithinRoot(realRoot, linkDst); err != nil {
				return fmt.Errorf("updater: tar hardlink target %q: %w", hdr.Linkname, err)
			}
			if err := rejectExistingSymlink(linkDst); err != nil {
				return fmt.Errorf("updater: tar hardlink target %q: %w", hdr.Linkname, err)
			}
			if err := removeExistingNonDirOrSymlink(target); err != nil {
				return err
			}
			if err := os.Link(linkDst, target); err != nil {
				return fmt.Errorf("updater: creating hardlink %q -> %q: %w", hdr.Name, hdr.Linkname, err)
			}

		default:
			return fmt.Errorf("updater: unsupported tar entry type %q for %q", hdr.Typeflag, hdr.Name)
		}
	}

	if err := verifyExtractedSymlinks(realRoot); err != nil {
		return err
	}
	for i := len(dirFixes) - 1; i >= 0; i-- {
		_ = os.Chmod(dirFixes[i].path, dirFixes[i].perm)
	}
	return nil
}

func tarStreamLimit(maxBytes int64) int64 {
	const (
		minTarStreamOverhead int64 = 64 << 10 // 64 KiB minimum header allowance for small limits
		maxTarStreamOverhead int64 = 64 << 20 // 64 MiB cap on tar/PAX header overhead
	)
	overhead := maxBytes
	if overhead < minTarStreamOverhead {
		overhead = minTarStreamOverhead
	}
	if overhead > maxTarStreamOverhead {
		overhead = maxTarStreamOverhead
	}
	if maxBytes > math.MaxInt64-overhead-1 {
		return math.MaxInt64
	}
	return maxBytes + overhead + 1
}

func prepareExtractRoot(dstDir string) (string, error) {
	if strings.TrimSpace(dstDir) == "" {
		return "", errors.New("updater: empty extraction destination directory")
	}
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return "", fmt.Errorf("updater: creating extraction dir %s: %w", dstDir, err)
	}
	absRoot, err := filepath.Abs(dstDir)
	if err != nil {
		return "", fmt.Errorf("updater: resolving extraction dir %s: %w", dstDir, err)
	}
	realRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return "", fmt.Errorf("updater: canonicalizing extraction dir %s: %w", dstDir, err)
	}
	return realRoot, nil
}

func validateArchiveEntryPath(realRoot, rawName string) (target string, isRoot bool, err error) {
	if strings.ContainsRune(rawName, '\x00') {
		return "", false, errors.New("path contains NUL byte")
	}
	if rawName == "" {
		return "", false, errors.New("empty archive entry path")
	}
	if strings.HasPrefix(rawName, "/") || strings.HasPrefix(rawName, `\`) {
		return "", false, errors.New("absolute path not allowed")
	}
	if hasWindowsDrivePrefix(rawName) {
		return "", false, errors.New("drive-qualified path not allowed")
	}

	norm := strings.ReplaceAll(rawName, `\`, "/")
	for _, seg := range strings.Split(norm, "/") {
		if seg == ".." {
			return "", false, errors.New("path traversal ('..') not allowed")
		}
	}

	cleaned := path.Clean(norm)
	if cleaned == "." || cleaned == "" {
		return realRoot, true, nil
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.HasPrefix(cleaned, "/") {
		return "", false, errors.New("path escapes extraction directory")
	}

	target = filepath.Join(realRoot, filepath.FromSlash(cleaned))
	if !isSubpath(realRoot, target) {
		return "", false, errors.New("target escapes extraction directory")
	}
	return target, false, nil
}

func ensureSafeParent(realRoot, target string) error {
	parent := filepath.Dir(target)
	if !isSubpath(realRoot, parent) {
		return errors.New("parent escapes extraction directory")
	}

	// Verify the deepest existing ancestor resolves inside realRoot before creating directories.
	existing := parent
	for {
		if _, err := os.Lstat(existing); err == nil {
			break
		}
		next := filepath.Dir(existing)
		if next == existing {
			return errors.New("cannot locate existing parent directory")
		}
		existing = next
	}
	resolvedExisting, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return fmt.Errorf("resolving parent %s: %w", existing, err)
	}
	if !isSubpath(realRoot, resolvedExisting) {
		return errors.New("parent symlink escapes extraction directory")
	}

	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("creating parent directory: %w", err)
	}
	resolvedParent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return fmt.Errorf("resolving created parent: %w", err)
	}
	if !isSubpath(realRoot, resolvedParent) {
		return errors.New("resolved parent escapes extraction directory")
	}
	return nil
}

func validateSymlinkTarget(realRoot, linkPath, linkTarget string) error {
	if linkTarget == "" {
		return errors.New("empty symlink target")
	}
	if strings.ContainsRune(linkTarget, '\x00') {
		return errors.New("symlink target contains NUL byte")
	}
	if strings.HasPrefix(linkTarget, "/") || strings.HasPrefix(linkTarget, `\`) || filepath.IsAbs(linkTarget) || hasWindowsDrivePrefix(linkTarget) {
		return errors.New("absolute symlink target not allowed")
	}

	normTarget := filepath.FromSlash(strings.ReplaceAll(linkTarget, `\`, "/"))
	lexDest := filepath.Join(filepath.Dir(linkPath), normTarget)
	if !isSubpath(realRoot, lexDest) {
		return errors.New("symlink target escapes extraction directory")
	}

	resolvedParent, err := filepath.EvalSymlinks(filepath.Dir(linkPath))
	if err != nil {
		return fmt.Errorf("resolving symlink parent: %w", err)
	}
	if !isSubpath(realRoot, resolvedParent) {
		return errors.New("symlink parent escapes extraction directory")
	}
	realDest := filepath.Join(resolvedParent, normTarget)
	if !isSubpath(realRoot, realDest) {
		return errors.New("resolved symlink target escapes extraction directory")
	}
	return nil
}

func verifyExtractedSymlinks(realRoot string) error {
	return filepath.WalkDir(realRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&fs.ModeSymlink == 0 {
			return nil
		}
		if err := resolveLinkWithinRoot(realRoot, p); err != nil {
			return fmt.Errorf("updater: extracted symlink %q escapes extraction root: %w", p, err)
		}
		return nil
	})
}

func resolveLinkWithinRoot(realRoot, startPath string) error {
	if !isSubpath(realRoot, startPath) {
		return errors.New("path outside root")
	}
	rel, err := filepath.Rel(realRoot, startPath)
	if err != nil {
		return err
	}
	if rel == "." {
		return nil
	}

	// Resolve step-by-step following POSIX left-to-right symlink semantics so
	// "symlink/.." is never collapsed lexically before following "symlink".
	queue := strings.Split(filepath.ToSlash(rel), "/")
	var components []string
	hops := 0

	for len(queue) > 0 {
		seg := queue[0]
		queue = queue[1:]

		switch seg {
		case "", ".":
			continue
		case "..":
			if len(components) == 0 {
				return errors.New("resolved path escapes root")
			}
			components = components[:len(components)-1]
			continue
		}

		candidate := filepath.Join(append([]string{realRoot}, append(components, seg)...)...)
		fi, err := os.Lstat(candidate)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				components = append(components, seg)
				continue
			}
			return err
		}
		if fi.Mode()&fs.ModeSymlink != 0 {
			hops++
			if hops > maxSymlinkHops {
				return errors.New("too many symlink levels")
			}
			lt, err := os.Readlink(candidate)
			if err != nil {
				return err
			}
			if lt == "" || strings.ContainsRune(lt, '\x00') ||
				strings.HasPrefix(lt, "/") || strings.HasPrefix(lt, `\`) ||
				filepath.IsAbs(lt) || hasWindowsDrivePrefix(lt) {
				return errors.New("absolute or invalid symlink target")
			}
			linkParts := strings.Split(strings.ReplaceAll(lt, `\`, "/"), "/")
			nextQueue := make([]string, 0, len(linkParts)+len(queue))
			nextQueue = append(nextQueue, linkParts...)
			nextQueue = append(nextQueue, queue...)
			queue = nextQueue
			continue
		}
		components = append(components, seg)
	}
	return nil
}

func rejectExistingSymlink(path string) error {
	fi, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if fi.Mode()&fs.ModeSymlink != 0 {
		return errors.New("refusing to overwrite through existing symlink")
	}
	return nil
}

func removeExistingNonDirOrSymlink(path string) error {
	fi, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if fi.IsDir() {
		return fmt.Errorf("cannot replace directory %s with link", path)
	}
	return os.Remove(path)
}

func writeExtractedFile(r io.Reader, target string, perm fs.FileMode, remaining int64) (int64, error) {
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return 0, err
	}
	n, copyErr := io.Copy(out, io.LimitReader(r, remaining+1))
	if copyErr != nil {
		_ = out.Close()
		_ = os.Remove(target)
		return n, copyErr
	}
	if n > remaining {
		_ = out.Close()
		_ = os.Remove(target)
		return n, errors.New("archive extraction exceeds maximum allowed size")
	}
	if err := out.Chmod(perm); err != nil {
		_ = out.Close()
		return n, err
	}
	if err := out.Close(); err != nil {
		return n, err
	}
	return n, nil
}

func hasWindowsDrivePrefix(s string) bool {
	if len(s) < 2 || s[1] != ':' {
		return false
	}
	c := s[0]
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// parseCommandArgs splits an Enclosure.InstallerArguments string into argument
// tokens. It supports double-quoted segments (preserving Windows path
// backslashes such as --dir "C:\My App" while treating \" as an escaped quote)
// and single-quoted segments.
func parseCommandArgs(s string) []string {
	var args []string
	var cur strings.Builder
	inToken := false
	inDouble := false
	inSingle := false

	for i := 0; i < len(s); {
		ch := s[i]
		switch {
		case inDouble:
			if ch == '\\' {
				// Count consecutive backslashes; they only escape a following '"'.
				bs := 0
				for i+bs < len(s) && s[i+bs] == '\\' {
					bs++
				}
				if i+bs < len(s) && s[i+bs] == '"' {
					for k := 0; k < bs/2; k++ {
						cur.WriteByte('\\')
					}
					if bs%2 == 1 {
						cur.WriteByte('"')
						i += bs + 1
					} else {
						// Even number of backslashes precedes closing quote.
						i += bs
					}
					continue
				}
				// Backslashes not preceding '"' are literal (e.g. "C:\My App").
				for k := 0; k < bs; k++ {
					cur.WriteByte('\\')
				}
				i += bs
				continue
			}
			if ch == '"' {
				inDouble = false
				i++
				continue
			}
			cur.WriteByte(ch)
			i++

		case inSingle:
			if ch == '\'' {
				inSingle = false
				i++
				continue
			}
			cur.WriteByte(ch)
			i++

		default:
			if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
				if inToken {
					args = append(args, cur.String())
					cur.Reset()
					inToken = false
				}
				i++
				continue
			}
			if ch == '"' {
				inDouble = true
				inToken = true
				i++
				continue
			}
			if ch == '\'' {
				inSingle = true
				inToken = true
				i++
				continue
			}
			cur.WriteByte(ch)
			inToken = true
			i++
		}
	}

	if inToken {
		args = append(args, cur.String())
	}
	return args
}

func randomHex(nBytes int) string {
	buf := make([]byte, nBytes)
	if _, err := rand.Read(buf); err != nil {
		return "00000000"
	}
	return hex.EncodeToString(buf)
}

// -----------------------------------------------------------------------------
// Cross-platform macOS .app bundle helpers (exercised by apply_darwin.go and tests)
// -----------------------------------------------------------------------------

// findAppBundle walks up from exePath looking for an enclosing macOS .app bundle
// (matching *.app/Contents/MacOS/<exe>, or exePath itself if it ends with .app).
func findAppBundle(exePath string) (string, bool) {
	p := strings.TrimSpace(exePath)
	if p == "" {
		return "", false
	}
	p = filepath.Clean(p)
	if strings.HasSuffix(strings.ToLower(filepath.Base(p)), ".app") {
		return p, true
	}

	// Walk upward checking for <App>.app/Contents/MacOS/...
	cur := filepath.Dir(p)
	for {
		if strings.EqualFold(filepath.Base(cur), "MacOS") {
			contentsDir := filepath.Dir(cur)
			if strings.EqualFold(filepath.Base(contentsDir), "Contents") {
				appDir := filepath.Dir(contentsDir)
				if strings.HasSuffix(strings.ToLower(filepath.Base(appDir)), ".app") {
					return appDir, true
				}
			}
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	return "", false
}

// readBundleID reads CFBundleIdentifier from <appPath>/Contents/Info.plist using
// pure-Go XML plist parsing.
func readBundleID(appPath string) (string, error) {
	plistPath := filepath.Join(appPath, "Contents", "Info.plist")
	data, err := os.ReadFile(plistPath)
	if err != nil {
		return "", err
	}
	return parsePlistBundleID(data)
}

func parsePlistBundleID(data []byte) (string, error) {
	if bytes.HasPrefix(data, []byte("bplist")) {
		return "", errors.New("updater: binary Info.plist format requires XML conversion")
	}

	dec := xml.NewDecoder(bytes.NewReader(data))
	// Advance to <plist>, then top-level <dict>.
	inPlist := false
	inTopDict := false
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			if !inPlist {
				return "", errors.New("updater: invalid Info.plist: missing <plist> element")
			}
			return "", nil
		}
		if err != nil {
			return "", fmt.Errorf("updater: parsing Info.plist XML: %w", err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if !inPlist {
			if se.Name.Local == "plist" {
				inPlist = true
			}
			continue
		}
		if !inTopDict {
			if se.Name.Local == "dict" {
				inTopDict = true
				break
			}
			if err := dec.Skip(); err != nil {
				return "", fmt.Errorf("updater: skipping Info.plist element: %w", err)
			}
		}
	}

	var curKey string
	hasKey := false
	for {
		tok, err := dec.Token()
		if err != nil {
			return "", fmt.Errorf("updater: reading Info.plist dict: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "key" {
				var k string
				if err := dec.DecodeElement(&k, &t); err != nil {
					return "", fmt.Errorf("updater: decoding Info.plist key: %w", err)
				}
				curKey = strings.TrimSpace(k)
				hasKey = true
				continue
			}
			if hasKey && curKey == "CFBundleIdentifier" && t.Name.Local == "string" {
				var val string
				if err := dec.DecodeElement(&val, &t); err != nil {
					return "", fmt.Errorf("updater: decoding CFBundleIdentifier: %w", err)
				}
				return strings.TrimSpace(val), nil
			}
			// Skip any other value element (including nested <dict> or <array>)
			// so nested keys cannot match top-level CFBundleIdentifier.
			if err := dec.Skip(); err != nil {
				return "", fmt.Errorf("updater: skipping Info.plist value <%s>: %w", t.Name.Local, err)
			}
			hasKey = false
			curKey = ""

		case xml.EndElement:
			if t.Name.Local == "dict" {
				return "", nil
			}
		}
	}
}

// locateExtractedApp searches dir for a macOS .app bundle directory, skipping
// __MACOSX metadata directories and never descending into nested helper .app
// bundles inside an outer .app.
func locateExtractedApp(dir string) (string, error) {
	type candidate struct {
		path     string
		depth    int
		hasPlist bool
	}
	var found []candidate

	var scan func(cur string, depth int) error
	scan = func(cur string, depth int) error {
		if depth > 4 {
			return nil
		}
		entries, err := os.ReadDir(cur)
		if err != nil {
			return err
		}
		for _, e := range entries {
			name := e.Name()
			if name == "__MACOSX" || strings.HasPrefix(name, ".") {
				continue
			}
			if !e.IsDir() {
				continue
			}
			full := filepath.Join(cur, name)
			if strings.HasSuffix(strings.ToLower(name), ".app") {
				_, plistErr := os.Stat(filepath.Join(full, "Contents", "Info.plist"))
				found = append(found, candidate{
					path:     full,
					depth:    depth,
					hasPlist: plistErr == nil,
				})
				// Do not descend into a .app bundle (avoids embedded helper apps).
				continue
			}
			if err := scan(full, depth+1); err != nil {
				return err
			}
		}
		return nil
	}

	if err := scan(dir, 0); err != nil {
		return "", fmt.Errorf("updater: scanning %s for .app bundle: %w", dir, err)
	}
	if len(found) == 0 {
		return "", fmt.Errorf("updater: no .app bundle found in %s", dir)
	}

	sort.Slice(found, func(i, j int) bool {
		if found[i].depth != found[j].depth {
			return found[i].depth < found[j].depth
		}
		if found[i].hasPlist != found[j].hasPlist {
			return found[i].hasPlist
		}
		return found[i].path < found[j].path
	})

	// Reject ambiguous results at the shallowest level when multiple bundles exist.
	best := found[0]
	var sameDepthWithPlist int
	var sameDepthTotal int
	for _, c := range found {
		if c.depth == best.depth {
			sameDepthTotal++
			if c.hasPlist {
				sameDepthWithPlist++
			}
		}
	}
	if sameDepthTotal > 1 && !(best.hasPlist && sameDepthWithPlist == 1) {
		return "", fmt.Errorf("updater: multiple .app bundles found in %s", dir)
	}
	return best.path, nil
}

// replaceAppBundle verifies bundle identifiers and atomically swaps currentApp
// with newApp using a same-directory backup and immediate rollback on failure.
func replaceAppBundle(currentApp, newApp string) error {
	return replaceAppBundleWithRename(currentApp, newApp, os.Rename)
}

func replaceAppBundleWithRename(currentApp, newApp string, renameFn func(oldpath, newpath string) error) error {
	curInfo, err := os.Stat(currentApp)
	if err != nil {
		return fmt.Errorf("updater: current app bundle %s: %w", currentApp, err)
	}
	if !curInfo.IsDir() {
		return fmt.Errorf("updater: current app path %s is not a directory", currentApp)
	}
	newInfo, err := os.Stat(newApp)
	if err != nil {
		return fmt.Errorf("updater: new app bundle %s: %w", newApp, err)
	}
	if !newInfo.IsDir() {
		return fmt.Errorf("updater: new app path %s is not a directory", newApp)
	}

	if err := verifyMatchingBundleIDs(currentApp, newApp); err != nil {
		return err
	}

	parentDir := filepath.Dir(currentApp)
	baseName := filepath.Base(currentApp)
	suffix := randomHex(6)
	backupPath := filepath.Join(parentDir, "."+baseName+".backup-"+suffix)

	stagedApp := newApp
	createdStaging := false
	if filepath.Clean(filepath.Dir(newApp)) != filepath.Clean(parentDir) {
		stagingPath := filepath.Join(parentDir, "."+baseName+".staging-"+suffix)
		if err := os.Rename(newApp, stagingPath); err != nil {
			if !isCrossDeviceErr(err) {
				return fmt.Errorf("updater: staging new app bundle into %s: %w", parentDir, err)
			}
			if copyErr := copyDirPreserving(newApp, stagingPath); copyErr != nil {
				return fmt.Errorf("updater: copying new app bundle into %s: %w", parentDir, copyErr)
			}
		}
		stagedApp = stagingPath
		createdStaging = true
	}
	defer func() {
		if createdStaging {
			_ = removeAllForce(stagedApp)
		}
	}()

	// Step 1: Atomically move current bundle aside to backup in same directory.
	if err := renameFn(currentApp, backupPath); err != nil {
		return fmt.Errorf("updater: backing up current app bundle: %w", err)
	}

	// Step 2: Atomically move staged bundle into currentApp's location.
	if err := renameFn(stagedApp, currentApp); err != nil {
		rbErr := os.Rename(backupPath, currentApp)
		if rbErr != nil {
			return fmt.Errorf("updater: swapping new app bundle failed (%w) and rollback failed (%v)", err, rbErr)
		}
		return fmt.Errorf("updater: swapping new app bundle (rolled back): %w", err)
	}
	createdStaging = false

	// Step 3: Best-effort cleanup of backup directory after successful swap.
	_ = removeAllForce(backupPath)
	return nil
}

func verifyMatchingBundleIDs(currentApp, newApp string) error {
	curID, curErr := readPlatformBundleID(currentApp)
	if curErr != nil && !errors.Is(curErr, os.ErrNotExist) {
		return fmt.Errorf("updater: reading current Info.plist: %w", curErr)
	}
	newID, newErr := readPlatformBundleID(newApp)
	if newErr != nil && !errors.Is(newErr, os.ErrNotExist) {
		return fmt.Errorf("updater: reading new Info.plist: %w", newErr)
	}
	if curID != "" && (newID == "" || curID != newID) {
		return fmt.Errorf("updater: bundle identifier mismatch (current=%q, update=%q)", curID, newID)
	}
	return nil
}

func parseDarwinCodeSignOutput(output string) (adHoc bool, teamID string, authorities []string) {
	hasAdHocFlag := false
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "Signature="):
			if strings.EqualFold(strings.TrimSpace(strings.TrimPrefix(line, "Signature=")), "adhoc") {
				hasAdHocFlag = true
			}
		case strings.HasPrefix(line, "TeamIdentifier="):
			teamID = strings.TrimSpace(strings.TrimPrefix(line, "TeamIdentifier="))
		case strings.HasPrefix(line, "Authority="):
			if auth := strings.TrimSpace(strings.TrimPrefix(line, "Authority=")); auth != "" {
				authorities = append(authorities, auth)
			}
		}
	}
	adHoc = hasAdHocFlag || (len(authorities) == 0 && (teamID == "" || strings.EqualFold(teamID, "not set")))
	return adHoc, teamID, authorities
}

func verifyDarwinSigningIdentity(curTeamID string, curAuthorities []string, newTeamID string, newAuthorities []string) error {
	if curTeamID != "" && !strings.EqualFold(curTeamID, "not set") {
		if newTeamID != curTeamID {
			return fmt.Errorf("updater: update code signing TeamIdentifier mismatch (current=%q, update=%q)", curTeamID, newTeamID)
		}
		return nil
	}
	if len(curAuthorities) > 0 {
		if len(newAuthorities) == 0 || !slices.Equal(curAuthorities, newAuthorities) {
			return fmt.Errorf("updater: update code signing Authority mismatch (current=%v, update=%v)", curAuthorities, newAuthorities)
		}
	}
	return nil
}

// -----------------------------------------------------------------------------
// Cross-platform Windows update helpers (exercised by apply_windows.go and tests)
// -----------------------------------------------------------------------------

// buildWindowsInstallerArgs builds the command-line argument slice for a Windows
// installer executable from untrusted Enclosure.InstallerArguments (filtered
// through a strict boolean-flag allowlist), caller-trusted ApplyOptions.ExtraArgs,
// opts.Silent, and installDir.
func buildWindowsInstallerArgs(installerArgs, installDir string, opts ApplyOptions) []string {
	var args []string
	for _, tok := range parseCommandArgs(installerArgs) {
		if isAllowedFeedInstallerArg(tok) {
			args = append(args, tok)
		}
	}
	if len(opts.ExtraArgs) > 0 {
		args = append(args, opts.ExtraArgs...)
	}
	if opts.Silent && !hasWindowsSilentFlag(args) {
		args = append(args, "--silent")
	}
	if strings.TrimSpace(installDir) != "" && !hasWindowsDirFlag(opts.ExtraArgs) {
		args = append(args, "--dir", installDir)
	}
	return args
}

func isAllowedFeedInstallerArg(arg string) bool {
	switch strings.ToLower(arg) {
	case "--silent", "-s", "/s", "/silent", "/verysilent", "/quiet", "/qn", "-silent",
		"--no-desktop", "--no-start-menu", "--no-launch":
		return true
	default:
		return false
	}
}

func hasWindowsSilentFlag(args []string) bool {
	for _, a := range args {
		switch strings.ToLower(strings.TrimSpace(a)) {
		case "--silent", "-s", "/s", "/silent", "/verysilent", "/quiet", "/qn", "-silent":
			return true
		}
	}
	return false
}

func hasWindowsDirFlag(args []string) bool {
	for _, a := range args {
		low := strings.ToLower(strings.TrimSpace(a))
		if low == "--dir" || low == "-d" ||
			strings.HasPrefix(low, "--dir=") ||
			strings.HasPrefix(low, "/d=") ||
			strings.HasPrefix(low, "/dir=") {
			return true
		}
	}
	return false
}

const (
	// windowsSilentRelaunchScript is a static PowerShell script (zero string
	// interpolation) that waits for GGUI_UPDATER_PID to exit (up to 30s), runs
	// GGUI_UPDATER_SETUP with GGUI_UPDATER_ARGS using ProcessStartInfo
	// (UseShellExecute = $false), and launches GGUI_UPDATER_TARGET if ExitCode == 0.
	windowsSilentRelaunchScript = `$ErrorActionPreference = 'SilentlyContinue'; $pidVal = 0; if ([int]::TryParse($env:GGUI_UPDATER_PID, [ref]$pidVal) -and $pidVal -gt 0) { try { $p = [System.Diagnostics.Process]::GetProcessById($pidVal); $p.WaitForExit(30000) | Out-Null } catch {} }; $setup = $env:GGUI_UPDATER_SETUP; if (-not [string]::IsNullOrEmpty($setup)) { $psi = New-Object System.Diagnostics.ProcessStartInfo; $psi.FileName = $setup; $psi.Arguments = [string]$env:GGUI_UPDATER_ARGS; $psi.UseShellExecute = $false; $proc = [System.Diagnostics.Process]::Start($psi); if ($null -ne $proc) { $proc.WaitForExit(); if ($proc.ExitCode -eq 0 -and -not [string]::IsNullOrEmpty($env:GGUI_UPDATER_TARGET)) { $rpsi = New-Object System.Diagnostics.ProcessStartInfo; $rpsi.FileName = $env:GGUI_UPDATER_TARGET; $rpsi.UseShellExecute = $false; [System.Diagnostics.Process]::Start($rpsi) | Out-Null } } }`

	// windowsPortableHelperScript is a static PowerShell script (zero string
	// interpolation) that waits for GGUI_UPDATER_PID to exit (up to 30s),
	// launches GGUI_UPDATER_TARGET if set, and retries Remove-Item -LiteralPath
	// on each JSON-encoded path in GGUI_UPDATER_CLEANUP.
	windowsPortableHelperScript = `$ErrorActionPreference = 'SilentlyContinue'; $pidVal = 0; if ([int]::TryParse($env:GGUI_UPDATER_PID, [ref]$pidVal) -and $pidVal -gt 0) { try { $p = [System.Diagnostics.Process]::GetProcessById($pidVal); $p.WaitForExit(30000) | Out-Null } catch {} }; if (-not [string]::IsNullOrEmpty($env:GGUI_UPDATER_TARGET)) { $rpsi = New-Object System.Diagnostics.ProcessStartInfo; $rpsi.FileName = $env:GGUI_UPDATER_TARGET; $rpsi.UseShellExecute = $false; [System.Diagnostics.Process]::Start($rpsi) | Out-Null }; if (-not [string]::IsNullOrEmpty($env:GGUI_UPDATER_CLEANUP)) { $items = @(ConvertFrom-Json $env:GGUI_UPDATER_CLEANUP); for ($i = 0; $i -lt 20; $i++) { $rem = @(); foreach ($item in $items) { if (-not [string]::IsNullOrEmpty($item) -and (Test-Path -LiteralPath $item)) { Remove-Item -LiteralPath $item -Force -ErrorAction SilentlyContinue; if (Test-Path -LiteralPath $item) { $rem += $item } } }; if ($rem.Count -eq 0) { break }; $items = $rem; Start-Sleep -Milliseconds 250 } }`
)

type windowsHelperSpec struct {
	Exe  string
	Args []string
	Env  []string
}

func windowsPowerShellExe() string {
	return resolveWindowsPowerShellExe(os.Getenv("SystemRoot"))
}

func resolveWindowsPowerShellExe(systemRoot string) string {
	root := strings.TrimSpace(systemRoot)
	if root == "" || strings.ContainsAny(root, "\x00\r\n") {
		root = `C:\Windows`
	}
	root = strings.TrimRight(root, `\/`)
	if root == "" {
		root = `C:\Windows`
	}
	return root + `\System32\WindowsPowerShell\v1.0\powershell.exe`
}

func validateWindowsHelperValue(name, val string) error {
	if strings.ContainsAny(val, "\x00\r\n") {
		return fmt.Errorf("updater: %s contains forbidden control character", name)
	}
	return nil
}

// quoteWin32Arg quotes a single argument according to Win32 CommandLineToArgvW
// rules and rejects NUL/CR/LF control characters.
func quoteWin32Arg(arg string) (string, error) {
	if err := validateWindowsHelperValue("argument", arg); err != nil {
		return "", err
	}
	if arg == "" {
		return `""`, nil
	}
	if !strings.ContainsAny(arg, " \t\"") {
		return arg, nil
	}
	var b strings.Builder
	b.WriteByte('"')
	bs := 0
	for i := 0; i < len(arg); i++ {
		switch arg[i] {
		case '\\':
			bs++
		case '"':
			for k := 0; k < bs*2+1; k++ {
				b.WriteByte('\\')
			}
			b.WriteByte('"')
			bs = 0
		default:
			for k := 0; k < bs; k++ {
				b.WriteByte('\\')
			}
			bs = 0
			b.WriteByte(arg[i])
		}
	}
	for k := 0; k < bs*2; k++ {
		b.WriteByte('\\')
	}
	b.WriteByte('"')
	return b.String(), nil
}

// formatWin32Args formats args as a single Win32 CommandLineToArgvW-quoted
// string suitable for System.Diagnostics.ProcessStartInfo.Arguments.
func formatWin32Args(args []string) (string, error) {
	if len(args) == 0 {
		return "", nil
	}
	quoted := make([]string, 0, len(args))
	for _, a := range args {
		q, err := quoteWin32Arg(a)
		if err != nil {
			return "", err
		}
		quoted = append(quoted, q)
	}
	return strings.Join(quoted, " "), nil
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

// buildWindowsSilentRelaunchHelper builds a static PowerShell helper invocation
// that passes all dynamic paths/arguments exclusively through environment variables.
func buildWindowsSilentRelaunchHelper(pid int, setupPath string, args []string, targetExe string) (*windowsHelperSpec, error) {
	if strings.TrimSpace(setupPath) == "" {
		return nil, errors.New("updater: empty Windows setup path")
	}
	if err := validateWindowsHelperValue("Windows setup path", setupPath); err != nil {
		return nil, err
	}
	if err := validateWindowsHelperValue("Windows target executable path", targetExe); err != nil {
		return nil, err
	}
	formattedArgs, err := formatWin32Args(args)
	if err != nil {
		return nil, err
	}
	return &windowsHelperSpec{
		Exe: windowsPowerShellExe(),
		Args: []string{
			"-NoProfile",
			"-NonInteractive",
			"-WindowStyle", "Hidden",
			"-Command", windowsSilentRelaunchScript,
		},
		Env: []string{
			"GGUI_UPDATER_PID=" + strconv.Itoa(pid),
			"GGUI_UPDATER_SETUP=" + setupPath,
			"GGUI_UPDATER_ARGS=" + formattedArgs,
			"GGUI_UPDATER_TARGET=" + targetExe,
		},
	}, nil
}

// buildWindowsPortableHelper builds a static PowerShell helper invocation that
// waits for pid to exit, optionally launches targetExe, and cleans up oldFiles
// via JSON-encoded GGUI_UPDATER_CLEANUP with Remove-Item -LiteralPath.
func buildWindowsPortableHelper(pid int, targetExe string, oldFiles []string) (*windowsHelperSpec, error) {
	if strings.TrimSpace(targetExe) == "" && len(oldFiles) == 0 {
		return nil, nil
	}
	if err := validateWindowsHelperValue("Windows target executable path", targetExe); err != nil {
		return nil, err
	}
	for _, p := range oldFiles {
		if err := validateWindowsHelperValue("Windows cleanup path", p); err != nil {
			return nil, err
		}
	}
	cleanupJSON := ""
	if len(oldFiles) > 0 {
		encoded, err := json.Marshal(oldFiles)
		if err != nil {
			return nil, fmt.Errorf("updater: encoding Windows cleanup paths: %w", err)
		}
		cleanupJSON = string(encoded)
	}
	return &windowsHelperSpec{
		Exe: windowsPowerShellExe(),
		Args: []string{
			"-NoProfile",
			"-NonInteractive",
			"-WindowStyle", "Hidden",
			"-Command", windowsPortableHelperScript,
		},
		Env: []string{
			"GGUI_UPDATER_PID=" + strconv.Itoa(pid),
			"GGUI_UPDATER_TARGET=" + targetExe,
			"GGUI_UPDATER_CLEANUP=" + cleanupJSON,
		},
	}, nil
}

type portableReplacement struct {
	Src string
	Dst string
}

// locateWindowsPortableReplacements finds the updated executable matching
// targetExe's filename (or the single .exe in the archive) plus any sibling
// .dll files (such as ggfx_driver.dll) inside extractedDir.
func locateWindowsPortableReplacements(extractedDir, targetExe string) ([]portableReplacement, error) {
	wantName := strings.ToLower(filepath.Base(targetExe))
	dstDir := filepath.Dir(targetExe)

	type exeCandidate struct {
		path  string
		depth int
		exact bool
	}
	var candidates []exeCandidate

	err := filepath.WalkDir(extractedDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != extractedDir && (d.Name() == "__MACOSX" || strings.HasPrefix(d.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		low := strings.ToLower(d.Name())
		if !strings.HasSuffix(low, ".exe") {
			return nil
		}
		rel, relErr := filepath.Rel(extractedDir, p)
		if relErr != nil {
			return nil
		}
		depth := strings.Count(filepath.ToSlash(rel), "/")
		candidates = append(candidates, exeCandidate{
			path:  p,
			depth: depth,
			exact: low == wantName,
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("updater: scanning portable archive: %w", err)
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("updater: no executable found in portable update archive")
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].exact != candidates[j].exact {
			return candidates[i].exact
		}
		if candidates[i].depth != candidates[j].depth {
			return candidates[i].depth < candidates[j].depth
		}
		return candidates[i].path < candidates[j].path
	})

	best := candidates[0]
	if !best.exact {
		sameDepth := 0
		for _, c := range candidates {
			if c.depth == best.depth {
				sameDepth++
			}
		}
		if sameDepth > 1 {
			return nil, fmt.Errorf("updater: multiple executables found in portable update archive (none matching %q)", filepath.Base(targetExe))
		}
	}

	newExe := best.path
	srcDir := filepath.Dir(newExe)

	replacements := []portableReplacement{
		{Src: newExe, Dst: filepath.Join(dstDir, filepath.Base(targetExe))},
	}

	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return nil, fmt.Errorf("updater: reading extracted directory %s: %w", srcDir, err)
	}
	for _, e := range entries {
		if e.IsDir() || !e.Type().IsRegular() {
			continue
		}
		if strings.HasSuffix(strings.ToLower(e.Name()), ".dll") {
			replacements = append(replacements, portableReplacement{
				Src: filepath.Join(srcDir, e.Name()),
				Dst: filepath.Join(dstDir, e.Name()),
			})
		}
	}
	return replacements, nil
}

// replacePortableFiles replaces each target file by renaming any existing file
// to <dst>.old and installing the new file into <dst>. On any failure, all
// renamed backups are restored and any newly created files are removed.
func replacePortableFiles(replacements []portableReplacement) ([]string, error) {
	return replacePortableFilesWithInstaller(replacements, installReplacementFile)
}

func installReplacementFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	return copyFileAtomic(src, dst, 0)
}

func replacePortableFilesWithInstaller(
	replacements []portableReplacement,
	installFn func(src, dst string) error,
) ([]string, error) {
	if len(replacements) == 0 {
		return nil, errors.New("updater: no files to replace")
	}

	type backupRecord struct {
		dst string
		old string
	}
	var backups []backupRecord
	var created []string

	rollback := func() {
		for i := len(created) - 1; i >= 0; i-- {
			_ = os.Remove(created[i])
		}
		for i := len(backups) - 1; i >= 0; i-- {
			b := backups[i]
			_ = os.Remove(b.dst)
			_ = os.Rename(b.old, b.dst)
		}
	}

	for _, r := range replacements {
		if _, err := os.Stat(r.Src); err != nil {
			rollback()
			return nil, fmt.Errorf("updater: source replacement %s: %w", r.Src, err)
		}

		hadExisting := false
		if _, err := os.Lstat(r.Dst); err == nil {
			hadExisting = true
			oldPath := r.Dst + ".old"
			if _, stErr := os.Lstat(oldPath); stErr == nil {
				if rmErr := os.Remove(oldPath); rmErr != nil {
					oldPath = r.Dst + ".old-" + randomHex(4)
				}
			}
			if err := os.Rename(r.Dst, oldPath); err != nil {
				rollback()
				return nil, fmt.Errorf("updater: backing up %s to %s: %w", r.Dst, oldPath, err)
			}
			backups = append(backups, backupRecord{dst: r.Dst, old: oldPath})
		} else if !errors.Is(err, os.ErrNotExist) {
			rollback()
			return nil, fmt.Errorf("updater: checking %s: %w", r.Dst, err)
		}

		if err := installFn(r.Src, r.Dst); err != nil {
			_ = os.Remove(r.Dst)
			rollback()
			return nil, fmt.Errorf("updater: installing %s: %w", r.Dst, err)
		}
		if !hadExisting {
			created = append(created, r.Dst)
		}
	}

	oldFiles := make([]string, 0, len(backups))
	for _, b := range backups {
		oldFiles = append(oldFiles, b.old)
	}
	return oldFiles, nil
}

// -----------------------------------------------------------------------------
// Cross-platform Linux update helpers (exercised by apply_linux.go and tests)
// -----------------------------------------------------------------------------

type linuxPackageKind int

const (
	linuxPackageUnknown linuxPackageKind = iota
	linuxPackageTarGz
	linuxPackageZip
	linuxPackageRawExecutable
)

// detectLinuxPackageKind identifies a Linux update package by extension and
// magic bytes. System packages (.deb/.rpm) and unrecognized file formats are
// rejected so they can never overwrite the target executable.
func detectLinuxPackageKind(filePath string) (linuxPackageKind, error) {
	lowerPath := strings.ToLower(filePath)
	switch {
	case strings.HasSuffix(lowerPath, ".deb"):
		return linuxPackageUnknown, fmt.Errorf("updater: unsupported Linux update package %q (.deb packages must be installed via system package manager)", filepath.Base(filePath))
	case strings.HasSuffix(lowerPath, ".rpm"):
		return linuxPackageUnknown, fmt.Errorf("updater: unsupported Linux update package %q (.rpm packages must be installed via system package manager)", filepath.Base(filePath))
	case strings.HasSuffix(lowerPath, ".tar.gz") || strings.HasSuffix(lowerPath, ".tgz"):
		return linuxPackageTarGz, nil
	case strings.HasSuffix(lowerPath, ".zip"):
		return linuxPackageZip, nil
	}

	f, err := os.Open(filePath)
	if err != nil {
		return linuxPackageUnknown, fmt.Errorf("updater: inspecting Linux update package %s: %w", filePath, err)
	}
	defer f.Close()

	var magic [4]byte
	n, readErr := io.ReadFull(f, magic[:])
	if readErr != nil && !errors.Is(readErr, io.EOF) && !errors.Is(readErr, io.ErrUnexpectedEOF) {
		return linuxPackageUnknown, fmt.Errorf("updater: reading Linux update package header %s: %w", filePath, readErr)
	}
	hdr := magic[:n]
	switch {
	case len(hdr) >= 2 && hdr[0] == 0x1f && hdr[1] == 0x8b:
		return linuxPackageTarGz, nil
	case len(hdr) >= 4 && hdr[0] == 'P' && hdr[1] == 'K' && ((hdr[2] == 0x03 && hdr[3] == 0x04) || (hdr[2] == 0x05 && hdr[3] == 0x06)):
		return linuxPackageZip, nil
	case len(hdr) >= 4 && hdr[0] == 0x7f && hdr[1] == 'E' && hdr[2] == 'L' && hdr[3] == 'F':
		return linuxPackageRawExecutable, nil
	case len(hdr) >= 2 && hdr[0] == '#' && hdr[1] == '!':
		return linuxPackageRawExecutable, nil
	default:
		return linuxPackageUnknown, fmt.Errorf("updater: unsupported Linux update package %q (unrecognized format)", filepath.Base(filePath))
	}
}

// locateLinuxBinary locates the updated Linux executable inside an extracted
// archive tree. It supports ggui's usr/bin/<exe> layout (with or without a
// single wrapper directory) as well as flat archives containing an executable.
func locateLinuxBinary(extractedDir, exeName string) (string, error) {
	type candidate struct {
		path  string
		score int // higher is better
		depth int
	}
	var candidates []candidate

	err := filepath.WalkDir(extractedDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != extractedDir && (d.Name() == "__MACOSX" || strings.HasPrefix(d.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, relErr := filepath.Rel(extractedDir, p)
		if relErr != nil {
			return nil
		}
		slashRel := filepath.ToSlash(rel)
		name := d.Name()
		low := strings.ToLower(name)
		// Skip non-binary metadata/asset files.
		if strings.HasSuffix(low, ".so") || strings.Contains(low, ".so.") ||
			strings.HasSuffix(low, ".desktop") || strings.HasSuffix(low, ".png") ||
			strings.HasSuffix(low, ".svg") || strings.HasSuffix(low, ".xml") ||
			strings.HasSuffix(low, ".txt") || strings.HasSuffix(low, ".md") ||
			strings.HasSuffix(low, ".json") {
			return nil
		}

		info, statErr := d.Info()
		if statErr != nil {
			return nil
		}
		isExec := info.Mode()&0o111 != 0
		inUsrBin := strings.HasPrefix(slashRel, "usr/bin/") || strings.Contains(slashRel, "/usr/bin/")
		exactName := exeName != "" && name == exeName

		score := 0
		switch {
		case inUsrBin && exactName:
			score = 100
		case exactName && isExec:
			score = 90
		case inUsrBin && isExec:
			score = 80
		case inUsrBin:
			score = 70
		case exactName:
			score = 60
		case isExec:
			score = 40
		default:
			return nil
		}

		depth := strings.Count(slashRel, "/")
		candidates = append(candidates, candidate{
			path:  p,
			score: score,
			depth: depth,
		})
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("updater: scanning extracted Linux archive: %w", err)
	}
	if len(candidates) == 0 {
		return "", fmt.Errorf("updater: no Linux executable found in %s", extractedDir)
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		if candidates[i].depth != candidates[j].depth {
			return candidates[i].depth < candidates[j].depth
		}
		return candidates[i].path < candidates[j].path
	})
	return candidates[0].path, nil
}

// locateLinuxBundledLibs finds optional shared libraries (such as
// usr/lib/<pkg>/libggfx_driver.so) inside extractedDir corresponding to a
// usr/bin/<exe> layout and maps them onto the target installation prefix.
func locateLinuxBundledLibs(extractedDir, newBin, targetExe string) []portableReplacement {
	// Case 1: ggui Linux tarball layout (<tree>/usr/bin/<exe> and <tree>/usr/lib/<subdir>/*.so*)
	binDir := filepath.Dir(newBin)
	usrDir := filepath.Dir(binDir)
	targetBinDir := filepath.Dir(targetExe)
	exeBase := filepath.Base(targetExe)
	var out []portableReplacement

	if filepath.Base(binDir) == "bin" && filepath.Base(usrDir) == "usr" && filepath.Base(targetBinDir) == "bin" {
		srcLibRoot := filepath.Join(usrDir, "lib")
		if fi, err := os.Stat(srcLibRoot); err == nil && fi.IsDir() {
			targetPrefix := filepath.Dir(targetBinDir)
			targetLibRoot := filepath.Join(targetPrefix, "lib")
			_ = filepath.WalkDir(srcLibRoot, func(p string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() || !d.Type().IsRegular() {
					return nil
				}
				low := strings.ToLower(d.Name())
				if !strings.HasSuffix(low, ".so") && !strings.Contains(low, ".so.") {
					return nil
				}
				rel, relErr := filepath.Rel(srcLibRoot, p)
				if relErr != nil {
					return nil
				}
				slashRel := filepath.ToSlash(rel)
				subdir, rest, hasSubdir := strings.Cut(slashRel, "/")
				if !hasSubdir || subdir == "" || subdir == "." || subdir == ".." || rest == "" {
					return nil
				}
				allowedSubdir := subdir == exeBase
				if !allowedSubdir {
					if sfi, sErr := os.Stat(filepath.Join(targetLibRoot, subdir)); sErr == nil && sfi.IsDir() {
						allowedSubdir = true
					}
				}
				if !allowedSubdir {
					return nil
				}
				dst := filepath.Join(targetLibRoot, rel)
				if !isSubpath(targetLibRoot, dst) {
					return nil
				}
				out = append(out, portableReplacement{
					Src: p,
					Dst: dst,
				})
				return nil
			})
			return out
		}
	}

	// Case 2: flat archive with sibling *.so* beside the executable.
	entries, err := os.ReadDir(binDir)
	if err != nil {
		return nil
	}
	for _, e := range entries {
		if e.IsDir() || !e.Type().IsRegular() {
			continue
		}
		low := strings.ToLower(e.Name())
		if strings.HasSuffix(low, ".so") || strings.Contains(low, ".so.") {
			dst := filepath.Join(targetBinDir, e.Name())
			if !isSubpath(targetBinDir, dst) {
				continue
			}
			out = append(out, portableReplacement{
				Src: filepath.Join(binDir, e.Name()),
				Dst: dst,
			})
		}
	}
	return out
}

// replaceLinuxArchiveFiles transactionally updates any bundled .so libraries
// and targetExe. Existing .so libraries are backed up with unique .old-<hex>
// names and newly created .so files are tracked; if any library update or
// binary replacement fails, all .so changes are rolled back.
func replaceLinuxArchiveFiles(
	libs []portableReplacement,
	targetExe, newBin string,
	copyLibFn func(src, dst string, mode os.FileMode) error,
	replaceBinFn func(targetExe, newBin string) error,
) error {
	if copyLibFn == nil {
		copyLibFn = copyFileAtomic
	}
	if replaceBinFn == nil {
		replaceBinFn = replaceLinuxBinary
	}

	type libBackup struct {
		dst string
		old string
	}
	var backups []libBackup
	var created []string

	rollback := func() {
		for i := len(created) - 1; i >= 0; i-- {
			_ = os.Remove(created[i])
		}
		for i := len(backups) - 1; i >= 0; i-- {
			b := backups[i]
			_ = os.Rename(b.old, b.dst)
		}
	}

	for _, lib := range libs {
		hadExisting := false
		if fi, err := os.Lstat(lib.Dst); err == nil {
			hadExisting = true
			oldPath := filepath.Join(filepath.Dir(lib.Dst), "."+filepath.Base(lib.Dst)+".old-"+randomHex(6))
			if fi.Mode().IsRegular() {
				if linkErr := os.Link(lib.Dst, oldPath); linkErr != nil {
					if cpErr := copyFileAtomic(lib.Dst, oldPath, fi.Mode().Perm()); cpErr != nil {
						rollback()
						return fmt.Errorf("updater: backing up shared library %s: %w", lib.Dst, cpErr)
					}
				}
			} else if mvErr := os.Rename(lib.Dst, oldPath); mvErr != nil {
				rollback()
				return fmt.Errorf("updater: backing up shared library %s: %w", lib.Dst, mvErr)
			}
			backups = append(backups, libBackup{dst: lib.Dst, old: oldPath})
		} else if !errors.Is(err, os.ErrNotExist) {
			rollback()
			return fmt.Errorf("updater: checking shared library %s: %w", lib.Dst, err)
		}

		if err := copyLibFn(lib.Src, lib.Dst, 0o755); err != nil {
			rollback()
			return fmt.Errorf("updater: updating shared library %s: %w", lib.Dst, err)
		}
		if !hadExisting {
			created = append(created, lib.Dst)
		}
	}

	if err := replaceBinFn(targetExe, newBin); err != nil {
		rollback()
		return err
	}

	for _, b := range backups {
		_ = os.Remove(b.old)
	}
	return nil
}

// replaceLinuxBinary atomically replaces targetExe with newBin by staging
// .<name>.new-<hex> with mode 0755 next to targetExe and replacing targetExe
// in a single atomic POSIX rename.
func replaceLinuxBinary(targetExe, newBin string) error {
	return replaceLinuxBinaryWithRename(targetExe, newBin, os.Rename)
}

func replaceLinuxBinaryWithRename(targetExe, newBin string, renameFn func(oldpath, newpath string) error) error {
	info, err := os.Stat(newBin)
	if err != nil {
		return fmt.Errorf("updater: reading new binary %s: %w", newBin, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("updater: new binary %s is not a regular file", newBin)
	}

	dir := filepath.Dir(targetExe)
	base := filepath.Base(targetExe)
	newPath := filepath.Join(dir, "."+base+".new-"+randomHex(6))

	if err := copyFileAtomic(newBin, newPath, 0o755); err != nil {
		return fmt.Errorf("updater: staging new binary %s: %w", newPath, err)
	}
	if err := os.Chmod(newPath, 0o755); err != nil {
		_ = os.Remove(newPath)
		return fmt.Errorf("updater: setting executable mode on %s: %w", newPath, err)
	}

	cleanupNew := true
	defer func() {
		if cleanupNew {
			_ = os.Remove(newPath)
		}
	}()

	if err := renameFn(newPath, targetExe); err != nil {
		return fmt.Errorf("updater: installing %s: %w", targetExe, err)
	}
	cleanupNew = false
	return nil
}
