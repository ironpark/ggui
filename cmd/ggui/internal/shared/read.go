// Package shared reads files that others are about to change.
//
// The watcher of ggui dev reads a source file the moment it changes, often
// while the editor or tool that changed it is still at work. On Windows a
// file open without FILE_SHARE_DELETE cannot be deleted until it is closed,
// and os.Open does not share deletion; ReadFile does. Renaming over a file
// that is open still fails there whatever it shares, as MoveFileEx, which
// os.Rename uses, refuses to replace an open file; an editor that saves so
// tries again, and the read is over in a moment.
package shared

// ReadFile reads the whole of the named file, as os.ReadFile does, without
// keeping others from deleting it meanwhile.
func ReadFile(name string) ([]byte, error) { return readFile(name) }
