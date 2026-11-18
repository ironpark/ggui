// Package robustio replaces and removes files in the face of what Windows
// does to a file that someone else has open.
package robustio

import (
	"os"
	"time"
)

// Rename renames oldpath to newpath as os.Rename does, replacing newpath,
// and on Windows tries again for a moment when the system refuses because
// one of them is open elsewhere.
//
// Windows will not replace or move a file that another handle has open,
// whatever that handle shares, and a file just written is opened at once
// by others: the antivirus that scans it, the indexer, a watcher reading
// what changed. Each is done in milliseconds. A program that saves by
// writing a file and renaming it over the old one therefore tries again,
// as editors do and as the go command does with its own files; without
// that, a save fails now and then for no reason the user can see.
func Rename(oldpath, newpath string) error {
	return retry(func() error { return os.Rename(oldpath, newpath) })
}

// Remove removes the named file as os.Remove does, trying again on Windows
// as Rename does.
func Remove(name string) error {
	return retry(func() error { return os.Remove(name) })
}

// retryFor is how long a refused operation is tried again: long past what
// a scan of a small file takes, and short enough that a refusal that is
// real, as for a directory that may not be written, is still said at once
// as far as a person can tell.
const retryFor = 500 * time.Millisecond

// retry runs op until it succeeds, fails for a reason that waiting does
// not change, or retryFor has passed, and returns its last error.
func retry(op func() error) error {
	err := op()
	if err == nil || !transient(err) {
		return err
	}
	wait := time.Millisecond
	for deadline := time.Now().Add(retryFor); time.Now().Before(deadline); {
		time.Sleep(wait)
		if wait < 32*time.Millisecond {
			wait *= 2
		}
		if err = op(); err == nil || !transient(err) {
			return err
		}
	}
	return err
}
