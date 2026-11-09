// Package threads is the thread history: what ksesh keeps of every
// conversation with an agent that names it (Claude Code's session_id), so a
// thread outlives its kitty window. Its window gone, it is a closed thread,
// settled in the sidebar, and can be resumed in a new window (see
// agent/thread.go for the rules, and Kitty for how the sidebar gets them).
//
// Each conversation is one small JSON file, $XDG_STATE_HOME/ksesh/threads/
// <conversation>.json. The hooks of several agents fire at once, and every
// sidebar reads the history: a write replaces the file whole by a rename, so
// a reader never sees half of one, and the read-modify-write of an update is
// done under a lock on the directory, so two writers don't lose each
// other's changes.
package threads

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/xheisenbugx/ksesh/model"
)

const ext = ".json"

// DefaultDir is $XDG_STATE_HOME/ksesh/threads, ~/.local/state by default.
func DefaultDir() (string, error) {
	dir := os.Getenv("XDG_STATE_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(dir, "ksesh", "threads"), nil
}

// Store is the thread history in a directory.
type Store struct {
	dir string

	// cache keeps what List read, by file, so a sidebar refreshing every
	// second only parses the records that changed.
	mu    sync.Mutex
	cache map[string]cached
}

type cached struct {
	mod  time.Time
	size int64
	rec  model.ThreadRecord
}

// NewStore is the history in dir. Without a dir (no home to keep it in)
// there is no history: nothing is kept, and nothing listed.
func NewStore(dir string) *Store {
	return &Store{dir: dir, cache: map[string]cached{}}
}

// fileName is a conversation's file. Conversation ids are UUIDs; anything
// else that could leave the directory is refused.
func fileName(conversation string) (string, bool) {
	if conversation == "" || len(conversation) > 128 || strings.HasPrefix(conversation, ".") {
		return "", false
	}
	for _, r := range conversation {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.') {
			return "", false
		}
	}
	return conversation + ext, true
}

var errBadID = errors.New("not a conversation id")

// Get reads a conversation's record.
func (s *Store) Get(conversation string) (model.ThreadRecord, bool) {
	name, ok := fileName(conversation)
	if !ok || s.dir == "" {
		return model.ThreadRecord{}, false
	}
	return s.read(filepath.Join(s.dir, name))
}

func (s *Store) read(path string) (model.ThreadRecord, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return model.ThreadRecord{}, false
	}
	var rec model.ThreadRecord
	if err := json.Unmarshal(raw, &rec); err != nil || rec.Conversation == "" {
		return model.ThreadRecord{}, false
	}
	return rec, true
}

// Update changes a conversation's record, creating it if there is none, and
// writes it back. It does nothing if change leaves the record as it was.
func (s *Store) Update(conversation string, change func(*model.ThreadRecord)) error {
	if s.dir == "" {
		return nil
	}
	name, ok := fileName(conversation)
	if !ok {
		return errBadID
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	unlock, err := lock(s.dir)
	if err != nil {
		return err
	}
	defer unlock()
	path := filepath.Join(s.dir, name)
	old, _ := s.read(path)
	rec := old
	change(&rec)
	rec.Conversation = conversation
	if Same(rec, old) {
		return nil
	}
	return writeFile(path, rec)
}

// Same reports whether two records say the same: the same times, in
// whatever location each was read in, count as the same.
func Same(a, b model.ThreadRecord) bool {
	ra, erra := json.Marshal(a)
	rb, errb := json.Marshal(b)
	return erra == nil && errb == nil && bytes.Equal(ra, rb)
}

// writeFile writes a record whole: to a temporary file, renamed over the
// record's.
func writeFile(path string, rec model.ThreadRecord) error {
	raw, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".thread-*.tmp")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(append(raw, '\n'))
	if err := errors.Join(werr, tmp.Close()); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}

// List reads every record, in no particular order. A missing directory is
// an empty history.
func (s *Store) List() ([]model.ThreadRecord, error) {
	if s.dir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(s.dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	seen := map[string]bool{}
	var records []model.ThreadRecord
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || strings.HasPrefix(name, ".") || !strings.HasSuffix(name, ext) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		seen[name] = true
		if c, ok := s.cache[name]; ok && c.mod.Equal(info.ModTime()) && c.size == info.Size() {
			records = append(records, c.rec)
			continue
		}
		rec, ok := s.read(filepath.Join(s.dir, name))
		if !ok {
			delete(s.cache, name)
			continue
		}
		s.cache[name] = cached{mod: info.ModTime(), size: info.Size(), rec: rec}
		records = append(records, rec)
	}
	for name := range s.cache {
		if !seen[name] {
			delete(s.cache, name)
		}
	}
	return records, nil
}

// Forget deletes a conversation's record.
func (s *Store) Forget(conversation string) error {
	if s.dir == "" {
		return nil
	}
	name, ok := fileName(conversation)
	if !ok {
		return errBadID
	}
	unlock, err := lock(s.dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	defer unlock()
	if err := os.Remove(filepath.Join(s.dir, name)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// Retention is how much of the history is kept: at most Keep closed
// threads, none of them inactive for longer than MaxAge (0 for no limit).
type Retention struct {
	Keep   int
	MaxAge time.Duration
}

const (
	DefaultKeep   = 200
	DefaultMaxAge = 30 * 24 * time.Hour
)

// LastActive is the last time a record's thread did anything.
func LastActive(r model.ThreadRecord) time.Time {
	last := r.StatusAt
	for _, t := range []time.Time{r.UpdatedAt, r.TurnAt, r.Thread.SettledAt} {
		if t.After(last) {
			last = t
		}
	}
	return last
}

// Expired picks the records past retention: of those not open in a window
// (open), every one inactive for longer than MaxAge, and past the Keep most
// recently active.
func Expired(records []model.ThreadRecord, open map[string]bool, now time.Time, keep Retention) []string {
	var closed []model.ThreadRecord
	for _, r := range records {
		if !open[r.Conversation] {
			closed = append(closed, r)
		}
	}
	slices.SortFunc(closed, func(a, b model.ThreadRecord) int {
		return cmp.Or(LastActive(b).Compare(LastActive(a)), strings.Compare(a.Conversation, b.Conversation))
	})
	var expired []string
	for i, r := range closed {
		if i >= max(keep.Keep, 0) || (keep.MaxAge > 0 && now.Sub(LastActive(r)) > keep.MaxAge) {
			expired = append(expired, r.Conversation)
		}
	}
	return expired
}

// Prune forgets the records past retention, keeping every conversation open
// in a window. It returns how many it forgot.
func (s *Store) Prune(open map[string]bool, now time.Time, keep Retention) (int, error) {
	records, err := s.List()
	if err != nil {
		return 0, err
	}
	expired := Expired(records, open, now, keep)
	var errs []error
	for _, conv := range expired {
		errs = append(errs, s.Forget(conv))
	}
	return len(expired), errors.Join(errs...)
}
