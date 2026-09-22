//go:build windows

package runlock

import (
	"fmt"
	"golang.org/x/sys/windows"
	"os"
)

// Acquire prevents concurrent runtimes/provisioners from writing the same state.
// OS-owned byte locks are released on crash; the empty lock file is retained.
func Acquire(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	overlap := &windows.Overlapped{}
	err = windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, overlap)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("runtime state is already in use or cannot be locked")
	}
	return func() { _ = windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, overlap); _ = f.Close() }, nil
}
