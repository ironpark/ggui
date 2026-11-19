// Package fswatch reports changes to a set of files and directories.
//
// # A scan is the truth
//
// What decides that a path changed is a scan: a stat of every path in the
// set, compared with the last. It costs about a microsecond a path, so a
// few thousand are scanned in a few milliseconds, and it is indifferent
// to how a program saves: a write in place, a new file renamed over the
// old one and a delete followed by a create all leave a different
// modification time, size or existence behind. A directory in the set
// stands for its entries; when its own modification time moves, as an
// entry is added or removed, it is listed again, and the entries that came
// and went are reported.
//
// What the operating system says about changes only wakes the scan, so
// that a save is seen within milliseconds of being written; it never
// decides by itself what changed, so an event that is late, doubled,
// coalesced or about some other path does no harm. The sources are:
//
//   - macOS: FSEvents, with file-level events, on the fewest directories
//     whose trees hold the set, called through purego so that no cgo is
//     needed. A stream that says it dropped events, or that a subtree must
//     be scanned, or that a watched root moved, has everything rescanned.
//   - Linux: inotify, one watch for each directory of interest, never one
//     for each file. A queue overflow (IN_Q_OVERFLOW) has everything
//     rescanned; running out of watches (ENOSPC) falls back to polling,
//     with a hint to raise fs.inotify.max_user_watches.
//   - Windows: ReadDirectoryChangesW on the fewest directories whose trees
//     hold the set, on one I/O completion port. A read that overflowed its
//     buffer, which completes with no records, has everything rescanned.
//
// A slow scan runs every [Options.Rescan] whatever the system says, to
// catch a notification that never came, as on some network file systems.
// Where there are no notifications, or with [Options.NoNotify], the scan
// runs every [Options.Poll] instead.
//
// # What is reported
//
// Changes found close together are gathered for [Options.Settle] and sent
// as one [Batch]; changes found while nobody receives join the batch
// waiting to be sent. Each [Event] is a [Create], [Write] or [Remove] of a
// path. A rename is a Remove of the old name and a Create of the new one;
// a file saved by renaming another over it is a Write, as is a file
// deleted and made again between two scans.
//
// A file that is found empty, or still changing, is looked at again for
// up to [Options.Steady] before it is reported, since a save that
// truncates a file and then writes it may be caught in between. An
// [Options.Content] hook can read the files it picks and say whether an
// edit changed anything that matters to the caller, such as a source
// file's tokens; such an edit is reported with [Event.Same] set.
//
// [Ignore] is a ready-made [Options.Ignore]: editor temporary files,
// version control directories and node_modules by default, chosen
// directories, and, optionally, what git ignores.
package fswatch
