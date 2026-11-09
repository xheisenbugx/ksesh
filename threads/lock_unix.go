//go:build unix

package threads

import (
	"os"
	"path/filepath"
	"syscall"
)

// lock takes the history's lock, held until the returned func is called.
// Every writer holds it for a read-modify-write of a record, which takes
// well under a millisecond.
func lock(dir string) (func(), error) {
	f, err := os.OpenFile(filepath.Join(dir, ".lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}
