package fswatch

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"
)

// On Linux the source is inotify, which watches one directory at a time:
// each directory the watcher names, once, never a file in it. For a
// recursive watch it also watches each directory created or moved into
// one of them, at once, so that a new directory's files are reported from
// the start. The descriptor is non-blocking and read through the
// runtime's poller, so closing it ends the reader.
//
// A queue overflow (IN_Q_OVERFLOW) is reported as lost events, which has
// the watcher rescan everything. Running out of watches (ENOSPC) is the
// source failing, which has the watcher poll instead, saying how to raise
// the limit.

const inotifyMask = syscall.IN_CREATE | syscall.IN_DELETE | syscall.IN_MODIFY | syscall.IN_CLOSE_WRITE |
	syscall.IN_ATTRIB | syscall.IN_MOVED_FROM | syscall.IN_MOVED_TO |
	syscall.IN_DELETE_SELF | syscall.IN_MOVE_SELF | syscall.IN_ONLYDIR

// errWatchLimit is inotify running out of watches.
var errWatchLimit = errors.New("inotify: the watch limit is reached (raise it with sysctl fs.inotify.max_user_watches=524288, " +
	"or in /etc/sysctl.d)")

type dirID struct {
	dev uint64
	ino uint64
}

type wdInfo struct {
	dir   string
	id    dirID
	hasID bool
}

func statDirID(dir string) (dirID, bool) {
	fi, err := os.Stat(dir)
	if err != nil {
		return dirID{}, false
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return dirID{}, false
	}
	return dirID{dev: uint64(st.Dev), ino: uint64(st.Ino)}, true
}

type inotify struct {
	queue
	f         *os.File
	recursive bool
	skip      func(path string) bool
	closing   atomic.Bool
	done      chan struct{} // closed when the reader returns

	mu    sync.Mutex
	wds   map[int32]wdInfo // a watch's directory and identity
	dirs  map[string]int32 // a directory's watch
	asked map[string]bool  // the directories watch named
}

func newSource(recursive bool, skip func(string) bool) (source, error) {
	fd, err := syscall.InotifyInit1(syscall.IN_CLOEXEC | syscall.IN_NONBLOCK)
	if err != nil {
		return nil, fmt.Errorf("inotify: %w", err)
	}
	s := &inotify{
		queue:     newQueue(),
		f:         os.NewFile(uintptr(fd), "inotify"),
		recursive: recursive,
		skip:      skip,
		done:      make(chan struct{}),
		wds:       map[int32]wdInfo{},
		dirs:      map[string]int32{},
		asked:     map[string]bool{},
	}
	go s.read()
	return s, nil
}

func (s *inotify) addWatch(dir string) (int, error) {
	raw, err := s.f.SyscallConn()
	if err != nil {
		return 0, err
	}
	var wd int
	var werr error
	if err := raw.Control(func(fd uintptr) {
		wd, werr = syscall.InotifyAddWatch(int(fd), dir, inotifyMask)
	}); err != nil {
		return 0, err
	}
	return wd, werr
}

func (s *inotify) rmWatch(wd int32) {
	raw, err := s.f.SyscallConn()
	if err != nil {
		return
	}
	_ = raw.Control(func(fd uintptr) {
		syscall.InotifyRmWatch(int(fd), uint32(wd))
	})
}

func (s *inotify) watch(dirs []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	asked := make(map[string]bool, len(dirs))
	for _, d := range dirs {
		asked[filepath.Clean(d)] = true
	}
	// A directory no longer asked for goes, unless it was added for
	// being created below one that still is.
	for d, wd := range s.dirs {
		if asked[d] {
			continue
		}
		if !s.asked[d] && s.below(asked, d) {
			continue
		}
		s.rmWatch(wd)
		delete(s.dirs, d)
		delete(s.wds, wd)
	}
	s.asked = asked
	for d := range asked {
		if err := s.add(d); err != nil {
			return err
		}
	}
	return nil
}

// below reports whether dir is below one of dirs.
func (s *inotify) below(dirs map[string]bool, dir string) bool {
	for d := filepath.Dir(dir); ; d = filepath.Dir(d) {
		if dirs[d] {
			return true
		}
		if d == filepath.Dir(d) {
			return false
		}
	}
}

// add watches dir; a directory that does not exist is not an error.
// s.mu is held.
func (s *inotify) add(dir string) error {
	if s.closing.Load() {
		return os.ErrClosed
	}
	if _, ok := s.dirs[dir]; ok {
		return nil
	}
	wd, err := s.addWatch(dir)
	switch {
	case errors.Is(err, syscall.ENOSPC):
		return errWatchLimit
	case errors.Is(err, syscall.ENOENT), errors.Is(err, syscall.ENOTDIR), errors.Is(err, syscall.EACCES):
		return nil
	case errors.Is(err, os.ErrClosed):
		return os.ErrClosed
	case err != nil:
		return fmt.Errorf("inotify: %s: %w", dir, err)
	}
	id, hasID := statDirID(dir)
	// The same directory by another name, through a link, has the same
	// watch.
	if old, ok := s.wds[int32(wd)]; ok {
		delete(s.dirs, old.dir)
	}
	s.wds[int32(wd)] = wdInfo{dir: dir, id: id, hasID: hasID}
	s.dirs[dir] = int32(wd)
	return nil
}

// addTree watches a directory that appeared, and those below it, and
// reports what is in them already: files made before the watch was in
// place.
func (s *inotify) addTree(dir string) {
	if s.closing.Load() {
		return
	}
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if s.closing.Load() {
			return filepath.SkipAll
		}
		if err != nil {
			return nil //nolint:nilerr // an unreadable entry is skipped; the walk goes on
		}
		if p != dir {
			s.push(p)
		}
		if !d.IsDir() {
			return nil
		}
		if p != dir && s.skip != nil && s.skip(p) {
			return filepath.SkipDir
		}
		s.mu.Lock()
		err = s.add(p)
		s.mu.Unlock()
		if errors.Is(err, errWatchLimit) {
			s.fail(err)
			return filepath.SkipAll
		}
		if err != nil {
			if s.closing.Load() || errors.Is(err, os.ErrClosed) {
				return filepath.SkipAll
			}
			s.lost() // the scan, and the safety net, still see changes
			return filepath.SkipAll
		}
		return nil
	})
}

func (s *inotify) read() {
	defer close(s.done)
	buf := make([]byte, 64<<10)
	for {
		n, err := s.f.Read(buf)
		if err != nil {
			if !s.closing.Load() {
				s.fail(fmt.Errorf("inotify: %w", err))
			}
			return
		}
		for off := 0; off+syscall.SizeofInotifyEvent <= n; {
			ev := (*syscall.InotifyEvent)(unsafe.Pointer(&buf[off]))
			avail := n - (off + syscall.SizeofInotifyEvent)
			if uint64(ev.Len) > uint64(avail) {
				s.lost()
				break
			}
			nameEnd := off + syscall.SizeofInotifyEvent + int(ev.Len)
			name := buf[off+syscall.SizeofInotifyEvent : nameEnd]
			off = nameEnd
			if i := bytes.IndexByte(name, 0); i >= 0 {
				name = name[:i]
			}
			s.event(ev.Wd, ev.Mask, string(name))
		}
	}
}

func (s *inotify) event(wd int32, mask uint32, name string) {
	if mask&syscall.IN_Q_OVERFLOW != 0 {
		s.lost()
		return
	}
	s.mu.Lock()
	info, ok := s.wds[wd]
	dir := info.dir
	if ok && mask&(syscall.IN_IGNORED|syscall.IN_DELETE_SELF) != 0 {
		// Gone, and its watch with it.
		s.forgetWatch(wd, dir)
	}
	readd := false
	if ok && mask&syscall.IN_MOVE_SELF != 0 {
		readd = s.movedSelf(wd, info, dir)
	}
	s.mu.Unlock()
	if !ok {
		return
	}
	path := dir
	if name != "" {
		path = filepath.Join(dir, name)
	}
	s.push(path)
	if readd && !s.closing.Load() {
		s.watchAgain(dir)
	}
	s.dirAppeared(path, mask, name)
}

// forgetWatch drops wd's watch of dir from the maps. s.mu is held.
func (s *inotify) forgetWatch(wd int32, dir string) {
	delete(s.wds, wd)
	if s.dirs[dir] == wd {
		delete(s.dirs, dir)
	}
}

// movedSelf answers whether a directory reported IN_MOVE_SELF must be
// watched again, and takes its stale watch out if so. A directory moved
// keeps its watch, which follows the directory, not its name. Moved
// within a watched directory, it was reported there by its new name,
// and adding that name found the same watch and renamed it, before
// IN_MOVE_SELF, so the name it has is right. Moved elsewhere, or
// replaced by another directory at the same path, its identity (Dev,
// Ino) no longer matches, so the stale watch goes and any replacement
// directory at dir is watched afresh. s.mu is held.
func (s *inotify) movedSelf(wd int32, info wdInfo, dir string) bool {
	cur, curOK := statDirID(dir)
	if !curOK || !info.hasID || cur != info.id {
		s.rmWatch(wd)
		s.forgetWatch(wd, dir)
		return curOK
	}
	return false
}

// watchAgain watches a moved directory afresh, and, when the watch is
// recursive, what reappeared below it.
func (s *inotify) watchAgain(dir string) {
	if s.recursive {
		s.addTree(dir)
		return
	}
	s.mu.Lock()
	err := s.add(dir)
	s.mu.Unlock()
	if errors.Is(err, errWatchLimit) {
		s.fail(err)
	} else if err != nil && !s.closing.Load() && !errors.Is(err, os.ErrClosed) {
		s.lost()
	}
}

// dirAppeared watches a directory made or moved into a watched one,
// with everything below it when the watch is recursive.
func (s *inotify) dirAppeared(path string, mask uint32, name string) {
	if s.recursive && mask&syscall.IN_ISDIR != 0 && mask&(syscall.IN_CREATE|syscall.IN_MOVED_TO) != 0 && name != "" {
		if s.skip == nil || !s.skip(path) {
			s.addTree(path)
		}
	}
}

func (s *inotify) close() {
	s.closing.Store(true)
	s.f.Close()
	<-s.done
}
