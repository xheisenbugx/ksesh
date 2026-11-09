package kitty

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/xheisenbugx/ksesh/model"
	"github.com/xheisenbugx/ksesh/oswrap"
	"github.com/xheisenbugx/ksesh/shell"
)

const testPID = 4242

var snapNow = time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC)

func unixSeconds(t time.Time) float64 {
	return float64(t.UnixNano()) / 1e9
}

// writeSnapshot writes a snapshot of kitty testPID holding the ls output,
// taken at the time given, as contrib/ksesh_snapshot.py would.
func writeSnapshot(t *testing.T, path string, ls string, at time.Time) {
	t.Helper()
	raw := fmt.Sprintf(`{"version":1,"pid":%d,"written_at":%f,"os_windows":%s}`, testPID, unixSeconds(at), ls)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(raw), 0o600))
}

// newSnapshotKitty builds a RealKitty in window selfID of kitty testPID that
// reads its snapshot, with the clock at snapNow. Its shell expects no calls:
// each test says which `kitten @` calls it allows.
func newSnapshotKitty(t *testing.T, selfID string) (*RealKitty, *shell.MockShell, string) {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	mOs := oswrap.NewMockOs(t)
	mOs.EXPECT().Getenv("KITTY_WINDOW_ID").Return(selfID).Maybe()
	mOs.EXPECT().Getenv("KITTY_LISTEN_ON").Return("").Maybe()
	mOs.EXPECT().Getenv("KITTY_PID").Return(fmt.Sprint(testPID)).Maybe()
	mShell := shell.NewMockShell(t)
	k := NewKitty(mOs, mShell, model.Config{}).(*RealKitty)
	k.clock = func() time.Time { return snapNow }
	path := k.UseSnapshot()
	require.NotEmpty(t, path)
	return k, mShell, path
}

func TestSnapshotPath(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/state")

	path, err := SnapshotPath(77)

	require.NoError(t, err)
	assert.Equal(t, "/state/ksesh/kitty-77.json", path, "contrib/ksesh_snapshot.py writes the same path")
}

func TestUseSnapshotOutsideKitty(t *testing.T) {
	mOs := oswrap.NewMockOs(t)
	mOs.EXPECT().Getenv("KITTY_PID").Return("")
	k := NewKitty(mOs, shell.NewMockShell(t), model.Config{}).(*RealKitty)

	assert.Empty(t, k.UseSnapshot())
}

func TestFreshSnapshotStandsInForLs(t *testing.T) {
	k, _, path := newSnapshotKitty(t, "4")
	writeSnapshot(t, path, fixture(t), snapNow.Add(-time.Second))

	sessions, err := k.ListSessions()

	require.NoError(t, err, "no `kitten @ ls`: the shell mock fails on any call")
	require.Len(t, sessions, 2)
	assert.Equal(t, "Second Brain", sessions[0].Name)
	assert.Equal(t, 1, sessions[0].Attached)
	assert.Equal(t, "sesh", sessions[1].Name)
	assert.Equal(t, "/home/dev/c/sesh", sessions[1].Path)
}

func TestSnapshotMatchesLs(t *testing.T) {
	fromLs, _ := newKitty(t, "4", model.Config{})
	fromSnap, _, path := newSnapshotKitty(t, "4")
	writeSnapshot(t, path, fixture(t), snapNow)

	want, err := fromLs.load()
	require.NoError(t, err)
	got, err := fromSnap.load()
	require.NoError(t, err)

	assert.Equal(t, want, got)
}

func TestSnapshotFallsBackToLs(t *testing.T) {
	tests := []struct {
		name  string
		write func(t *testing.T, path string)
	}{
		{"missing", func(t *testing.T, path string) {}},
		{"unparsable", func(t *testing.T, path string) {
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
			require.NoError(t, os.WriteFile(path, []byte(`{"version":1,"pid":42`), 0o600))
		}},
		{"stale", func(t *testing.T, path string) {
			writeSnapshot(t, path, fixture(t), snapNow.Add(-SnapshotMaxAge-time.Second))
		}},
		{"without the window ksesh runs in", func(t *testing.T, path string) {
			writeSnapshot(t, path, `[{"id":1,"tabs":[{"id":1,"windows":[{"id":1,"session_name":"x"}]}]}]`, snapNow)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k, mShell, path := newSnapshotKitty(t, "4")
			tt.write(t, path)
			mShell.EXPECT().CmdCapture("kitten", "@", "ls").Return(fixture(t), nil).Once()

			sessions, err := k.ListSessions()

			require.NoError(t, err)
			assert.Len(t, sessions, 2)
		})
	}
}

func TestSnapshotUsable(t *testing.T) {
	ls := []lsOSWindow{{ID: 1, Tabs: []lsTab{{ID: 1, Windows: []lsWindow{{ID: 4}}}}}}
	fresh := snapshotFile{Version: 1, PID: testPID, WrittenAt: unixSeconds(snapNow.Add(-time.Second)), OSWindows: ls}
	with := func(change func(*snapshotFile)) snapshotFile {
		s := fresh
		change(&s)
		return s
	}

	assert.NoError(t, snapshotUsable(fresh, testPID, 4, time.Time{}, snapNow))
	assert.NoError(t, snapshotUsable(fresh, testPID, 0, time.Time{}, snapNow), "outside kitty there is no window to find")
	assert.NoError(t, snapshotUsable(fresh, testPID, 4, snapNow.Add(-2*time.Second), snapNow), "written after ksesh's last change")

	assert.Error(t, snapshotUsable(fresh, testPID, 4, snapNow.Add(-time.Second), snapNow), "written as ksesh changed kitty: it may not show the change")
	assert.Error(t, snapshotUsable(fresh, testPID, 4, snapNow.Add(-500*time.Millisecond), snapNow), "written before ksesh's last change")
	assert.Error(t, snapshotUsable(fresh, testPID+1, 4, time.Time{}, snapNow), "another kitty's")
	assert.Error(t, snapshotUsable(fresh, testPID, 5, time.Time{}, snapNow), "doesn't know the window ksesh runs in yet")
	assert.Error(t, snapshotUsable(with(func(s *snapshotFile) { s.Version = 2 }), testPID, 4, time.Time{}, snapNow))
	assert.Error(t, snapshotUsable(with(func(s *snapshotFile) { s.WrittenAt = unixSeconds(snapNow.Add(-SnapshotMaxAge - time.Millisecond)) }), testPID, 4, time.Time{}, snapNow),
		"kitty stopped keeping it up")
	assert.NoError(t, snapshotUsable(with(func(s *snapshotFile) { s.WrittenAt = unixSeconds(snapNow.Add(-SnapshotMaxAge + time.Millisecond)) }), testPID, 4, time.Time{}, snapNow))
}

func TestSnapshotAfterAChange(t *testing.T) {
	// Right after ksesh changes kitty, the snapshot doesn't show it yet:
	// `ls` is used until kitty writes a new one.
	k, mShell, path := newSnapshotKitty(t, "4")
	writeSnapshot(t, path, fixture(t), snapNow.Add(-time.Second))
	mShell.EXPECT().CmdCapture("kitten", "@", "focus-window", "--match", "id:3").Return("", nil).Once()
	mShell.EXPECT().CmdCapture("kitten", "@", "ls").Return(fixture(t), nil).Once()

	_, err := k.ListSessions()
	require.NoError(t, err)
	require.NoError(t, k.FocusPlain(3))
	_, err = k.ListSessions()
	require.NoError(t, err)

	k.clock = func() time.Time { return snapNow.Add(200 * time.Millisecond) }
	writeSnapshot(t, path, fixture(t), snapNow.Add(100*time.Millisecond))
	k.Refresh()
	_, err = k.ListSessions()
	require.NoError(t, err)
	mShell.AssertNumberOfCalls(t, "CmdCapture", 2)
}

func TestSnapshotNotUsedWithoutAskingForIt(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	path, err := SnapshotPath(testPID)
	require.NoError(t, err)
	writeSnapshot(t, path, `[]`, time.Now())
	k, mShell := newKitty(t, "4", model.Config{})

	sessions, err := k.ListSessions()

	require.NoError(t, err)
	assert.Len(t, sessions, 2, "a command that runs once asks kitty")
	mShell.AssertCalled(t, "CmdCapture", "kitten", "@", "ls")
}

func TestSnapshotWatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kitty-1.json")
	w := NewSnapshotWatch(path)
	clock := time.Now()
	w.now = func() time.Time { return clock }

	changed, live := w.Changed()
	assert.False(t, changed)
	assert.False(t, live, "no snapshot: the watcher isn't loaded")

	require.NoError(t, os.WriteFile(path, []byte("{}"), 0o600))
	changed, live = w.Changed()
	assert.True(t, changed)
	assert.True(t, live)
	changed, live = w.Changed()
	assert.False(t, changed, "nothing new since")
	assert.True(t, live)

	// Rewritten the way kitty does it: a new file renamed over the old one.
	tmp := path + ".tmp"
	require.NoError(t, os.WriteFile(tmp, []byte("{}"), 0o600))
	require.NoError(t, os.Rename(tmp, path))
	changed, _ = w.Changed()
	assert.True(t, changed)

	clock = clock.Add(SnapshotMaxAge + time.Second)
	changed, live = w.Changed()
	assert.False(t, changed)
	assert.False(t, live, "kitty stopped rewriting it")

	require.NoError(t, os.Remove(path))
	changed, live = w.Changed()
	assert.False(t, changed)
	assert.False(t, live)
}

// BenchmarkLoad compares a sidebar's refresh read from the snapshot with one
// from `kitten @ ls`, without the cost of running kitten itself (35-42ms a
// call against kitty 0.50 with 6 windows, on an M1 Ultra).
func BenchmarkLoad(b *testing.B) {
	ls := benchLs(b, 30)
	b.Run("snapshot", func(b *testing.B) {
		b.Setenv("XDG_STATE_HOME", b.TempDir())
		mOs := oswrap.NewMockOs(b)
		mOs.EXPECT().Getenv(mock.Anything).RunAndReturn(func(name string) string {
			return map[string]string{"KITTY_PID": fmt.Sprint(testPID), "KITTY_WINDOW_ID": "1"}[name]
		}).Maybe()
		k := NewKitty(mOs, shell.NewMockShell(b), model.Config{}).(*RealKitty)
		path := k.UseSnapshot()
		raw := fmt.Sprintf(`{"version":1,"pid":%d,"written_at":%f,"os_windows":%s}`, testPID, unixSeconds(time.Now().Add(time.Hour)), ls)
		require.NoError(b, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(b, os.WriteFile(path, []byte(raw), 0o600))
		k.clock = func() time.Time { return time.Now().Add(time.Hour) }
		b.ResetTimer()
		for b.Loop() {
			k.Refresh()
			if _, err := k.ListAgents(); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("ls-parse-only", func(b *testing.B) {
		for b.Loop() {
			if _, err := parseState(ls, 1); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// benchLs is `kitten @ ls` for n windows, each in a tab of its own with an
// agent's user variables.
func benchLs(b *testing.B, n int) string {
	b.Helper()
	var tabs []map[string]any
	for i := 1; i <= n; i++ {
		tabs = append(tabs, map[string]any{
			"id": i, "is_active": i == 1, "title": "tab",
			"windows": []map[string]any{{
				"id": i, "is_active": true, "title": "✳ Fix the login", "cwd": "/home/dev/c/api",
				"cmdline": []string{"/bin/zsh"}, "last_focused_at": float64(i), "created_at": 1700000000000000000 + i,
				"session_name": fmt.Sprintf("s%d", i%5), "pid": 1000 + i,
				"user_vars":            map[string]string{"ksesh_agent": "claude", "ksesh_status": "working", "ksesh_status_at": "1700000000", "ksesh_msg": "Allow Bash: npm test"},
				"foreground_processes": []map[string]any{{"cmdline": []string{"claude"}, "cwd": "/home/dev/c/api", "pid": 2000 + i}},
			}},
		})
	}
	raw, err := json.Marshal([]map[string]any{{"id": 1, "is_active": true, "is_focused": true, "tabs": tabs}})
	require.NoError(b, err)
	return string(raw)
}
