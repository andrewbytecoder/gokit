//go:build ((unix && !android) || (js && wasm) || wasip1) && ((!cgo && !darwin) || osusergo)

package lockedfile

import (
	"os"

	"github.com/andrewbytecoder/gokit/lock/lockedfile/filelock"
)

func openFile(name string, flag int, perm os.FileMode) (*os.File, error) {
	// 如果flag里面有os.O_TRUNC，则将flag里面O_TRUNC去掉
	f, err := os.OpenFile(name, flag&^os.O_TRUNC, perm)
	if err != nil {
		return nil, err
	}

	switch flag & (os.O_RDONLY | os.O_WRONLY | os.O_RDWR) {
	// 写或者读写都需要添加写锁
	case os.O_WRONLY, os.O_RDWR:
		err = filelock.Lock(f)
	default:
		err = filelock.RLock(f)
	}
	if err != nil {
		f.Close()
		return nil, err
	}

	if flag&os.O_TRUNC == os.O_TRUNC {
		if err := f.Truncate(0); err != nil {
			// The documentation for os.O_TRUNC says “if possible, truncate file when
			// opened”, but doesn't define “possible” (golang.org/issue/28699).
			// We'll treat regular files (and symlinks to regular files) as “possible”
			// and ignore errors for the rest.
			if fi, statErr := f.Stat(); statErr != nil || fi.Mode().IsRegular() {
				filelock.Unlock(f)
				f.Close()
				return nil, err
			}
		}
	}

	return f, nil
}

func closeFile(f *os.File) error {
	// Since locking syscalls operator on file descriptors, we must unlock the file
	// while the descriptor still valid that is,before the file is closed
	// and avoid unlocking files that are already closed.
	if err := filelock.Unlock(f); err != nil {
		return err
	}
	return f.Close()
}
