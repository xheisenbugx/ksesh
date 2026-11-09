package kitty

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Every sidebar used to run `kitten @ ls` every second, one per tab. Instead,
// contrib/ksesh_snapshot.py, loaded into kitty by the watcher, writes what
// `ls` would say to a file whenever something the sidebars show changes
// (a user variable, a title, the focus, a window or tab opening or closing,
// a command starting), debounced, and on a heartbeat every couple of seconds
// for what changes without an event (the program running in a window). The
// sidebars read the file instead of asking kitty, and watch it to know when to
// look again.
//
// The file is $XDG_STATE_HOME/ksesh/kitty-<pid>.json, one per kitty instance
// (pid is kitty's, which its windows see as $KITTY_PID):
//
//	{"version": 1, "pid": 4242, "written_at": 1700000000.25, "os_windows": [...]}
//
// where os_windows is `kitten @ ls`'s output, less what ksesh doesn't read
// (environment variables, layouts). It is only trusted while the watcher keeps
// it up: see snapshotUsable.

const (
	snapshotVersion = 1
	// SnapshotMaxAge is how old a snapshot can be and still be read. The
	// watcher rewrites it every 2 seconds, so a missed heartbeat or two is
	// tolerated, but one kitty has stopped writing (it quit, or its watcher
	// isn't loaded or broke) soon falls back to `kitten @ ls`.
	SnapshotMaxAge = 6 * time.Second
)

type snapshotFile struct {
	Version int `json:"version"`
	PID     int `json:"pid"`
	// WrittenAt is when kitty read the state it holds, in unix seconds.
	WrittenAt float64      `json:"written_at"`
	OSWindows []lsOSWindow `json:"os_windows"`
}

func (s snapshotFile) writtenAt() time.Time {
	sec, frac := math.Modf(s.WrittenAt)
	return time.Unix(int64(sec), int64(frac*1e9))
}

// StateDir is where ksesh keeps its state: $XDG_STATE_HOME/ksesh,
// ~/.local/state/ksesh by default. contrib's kitty modules agree on it.
func StateDir() (string, error) {
	dir := os.Getenv("XDG_STATE_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(dir, "ksesh"), nil
}

// SnapshotPath is where the kitty whose pid is given writes its snapshot.
func SnapshotPath(pid int) (string, error) {
	dir, err := StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "kitty-"+strconv.Itoa(pid)+".json"), nil
}

// UseSnapshot has ksesh read kitty's state from its snapshot, for a program
// that keeps asking, like the sidebar. It takes the snapshot of the kitty ksesh
// runs in, and does nothing outside kitty. Commands that run once keep asking
// kitty, which is exact. It returns the snapshot's path, "" when there is none.
func (k *RealKitty) UseSnapshot() string {
	pid, err := strconv.Atoi(k.os.Getenv("KITTY_PID"))
	if err != nil || pid <= 0 {
		return ""
	}
	path, err := SnapshotPath(pid)
	if err != nil {
		return ""
	}
	k.mu.Lock()
	k.snapshotPath, k.snapshotPID = path, pid
	k.mu.Unlock()
	return path
}

func (k *RealKitty) now() time.Time {
	if k.clock != nil {
		return k.clock()
	}
	return time.Now()
}

// loadSnapshot reads kitty's state from the snapshot, if it can be trusted.
func (k *RealKitty) loadSnapshot() (*state, bool) {
	k.mu.Lock()
	path, pid, mutatedAt := k.snapshotPath, k.snapshotPID, k.mutatedAt
	k.mu.Unlock()
	if path == "" {
		return nil, false
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		k.snapshotMiss(err)
		return nil, false
	}
	var snap snapshotFile
	if err := json.Unmarshal(raw, &snap); err != nil {
		k.snapshotMiss(fmt.Errorf("couldn't parse %s: %w", path, err))
		return nil, false
	}
	selfID := k.selfWindowID()
	if err := snapshotUsable(snap, pid, selfID, mutatedAt, k.now()); err != nil {
		k.snapshotMiss(err)
		return nil, false
	}
	k.mu.Lock()
	k.snapshotErr = ""
	k.mu.Unlock()
	return buildState(snap.OSWindows, selfID), true
}

// snapshotUsable decides whether a snapshot can stand in for `kitten @ ls`:
// it must come from the kitty ksesh runs in, be recent enough that kitty is
// still keeping it up, be newer than the last change ksesh made itself (kitty
// writes it a moment after a change; until then `ls` is exact), and know the
// window ksesh runs in (a sidebar that just opened may predate it).
func snapshotUsable(snap snapshotFile, pid, selfID int, mutatedAt, now time.Time) error {
	if snap.Version != snapshotVersion {
		return fmt.Errorf("snapshot version %d, want %d", snap.Version, snapshotVersion)
	}
	if snap.PID != pid {
		return fmt.Errorf("snapshot of kitty %d, not %d", snap.PID, pid)
	}
	at := snap.writtenAt()
	if age := now.Sub(at); age > SnapshotMaxAge || age < -SnapshotMaxAge {
		return fmt.Errorf("snapshot is %s old", age.Round(time.Millisecond))
	}
	if !mutatedAt.IsZero() && !at.After(mutatedAt) {
		return errors.New("snapshot predates ksesh's last change")
	}
	if selfID != 0 && !hasWindow(snap.OSWindows, selfID) {
		return fmt.Errorf("snapshot predates window %d", selfID)
	}
	return nil
}

func hasWindow(osWindows []lsOSWindow, id int) bool {
	for _, osw := range osWindows {
		for _, tab := range osw.Tabs {
			for _, w := range tab.Windows {
				if w.ID == id {
					return true
				}
			}
		}
	}
	return false
}

// snapshotMiss logs why the snapshot wasn't used, once per reason: a sidebar
// asks often, and a snapshot right after a change is expected to be old.
func (k *RealKitty) snapshotMiss(err error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if msg := err.Error(); msg != k.snapshotErr {
		k.snapshotErr = msg
		slog.Debug("kitty: not using the snapshot, asking kitty", "reason", msg)
	}
}

// SnapshotWatch tells a program that keeps asking kitty, like the sidebar,
// when the snapshot changes, so it can look again then instead of on a timer.
type SnapshotWatch struct {
	path string
	last os.FileInfo
	now  func() time.Time
}

func NewSnapshotWatch(path string) *SnapshotWatch {
	return &SnapshotWatch{path: path, now: time.Now}
}

// Changed reports whether the snapshot was rewritten since the last call, and
// whether it is live: kitty is keeping it up, so a change will show up there.
// A stat, so cheap enough to call a few times a second.
func (w *SnapshotWatch) Changed() (changed, live bool) {
	info, err := os.Stat(w.path)
	if err != nil {
		w.last = nil
		return false, false
	}
	changed = w.last == nil || !os.SameFile(w.last, info) || !info.ModTime().Equal(w.last.ModTime()) || info.Size() != w.last.Size()
	w.last = info
	return changed, w.now().Sub(info.ModTime()) <= SnapshotMaxAge
}
