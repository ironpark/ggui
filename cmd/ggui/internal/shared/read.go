// Package shared reads files that others are about to change.
//
// The watcher of ggui dev reads a source file the moment it changes, often
// while the editor that saved it is still at work. On Windows a file open
// without FILE_SHARE_DELETE cannot be deleted or renamed over until it is
// closed, which is how many editors save; os.Open does not share deletion.
// ReadFile does, so reading a file never makes a save fail.
package shared

// ReadFile reads the whole of the named file, as os.ReadFile does, without
// keeping others from deleting it or renaming over it meanwhile.
func ReadFile(name string) ([]byte, error) { return readFile(name) }
