// Package filelock provides a platform-independent API for advisory file
// locking. Calls to functions in this package on platforms that do not support
// advisory locks will return errors for which IsNotSupported returns true.

//go:build ((unix && !android) || (js && wasm) || wasip1) && ((!cgo && !darwin) || osusergo)

package filelock

import (
	"errors"
	"os"
)

// A File provides the minimal set of methods required for file locking.
// File implementations must be usable as map keys.
// The usual implementations is *os.File.
type File interface {
	// Name returns the name of the file.
	Name() string

	// Fd returns the integer Unix file descriptor referencing the open file.
	// If the File is an *os.File, it must not be closed.
	Fd() uintptr

	// Stat returns the FileInfo structure describing file.
	Stat() (os.FileInfo, error)
}

// Lock places an advisory write lock on the file. blocking until it can be locked.
//
// If Lock returns nil, no other process will be able to place a read or write lock
// on the file until this process exits, closes f, or calls Unlock on it.
//
// If f's descriptor is already read or write-locked, the behavior of Lock is unspecified.
// Closing the file may or may not release the lock promptly. Callers should ensure that
// Unlock is always called when Lock succeeds.
func Lock(f File) error {
	return lock(f, writeLock)
}

// RLock places an advisory read lock on the file. blocking until it can be locked.
//
// If RLock returns nil, no other process will be able to place a write lock
// on the file until this process exists, closes f, or calls Unlock on it.
//
// If f is already read or write-locked, the behavior of RLock is unspecified.
//
// Closing the file may or may not release the lock promptly. Callers should ensure that
// Unlock is always called when RLock succeeds.
func RLock(f File) error {
	return lock(f, readLock)
}

// Unlock removes an advisory lock on the file by this process.
//
// The caller must not attempt to unlock a file that is not locked
func Unlock(f File) error {
	return unlock(f)
}

// String returns the name of the function corresponding to the lockType.
// Lock, RLock, and Unlock.
func (lt lockType) String() string {
	switch lt {
	case writeLock:
		return "Lock"
	case readLock:
		return "RLock"
	default:
		return "Unlock"
	}
}

// IsNotSupported returns a boolean indicating whether the error is known to
// report that a function is not supported (possibly for a specific input).
// It is satisfied by ErrNotSupported as well as some syscall errors.
func IsNotSupported(err error) bool {
	return isNotSupported(underlyingError(err))
}

var ErrNotSupported = errors.New("operation not supported")

// underlyingError returns the underlying error for known os error types.
func underlyingError(err error) error {
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return pathErr
	}

	var linkErr *os.LinkError
	if errors.As(err, &linkErr) {
		return linkErr
	}

	var syscallErr *os.SyscallError
	if errors.As(err, &syscallErr) {
		return syscallErr
	}

	return err
}
