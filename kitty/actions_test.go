package kitty

import (
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/xheisenbugx/ksesh/agent"
	"github.com/xheisenbugx/ksesh/model"
	"github.com/xheisenbugx/ksesh/oswrap"
	"github.com/xheisenbugx/ksesh/shell"
)

// The fixture's sessions: sesh (windows 1, 2 and 3, focusing 1) and notes,
// renamed Second Brain (window 4); window 5 is in none.

func TestTargets(t *testing.T) {
	assert.Equal(t, "tab:7", TabTarget(7))
	assert.Equal(t, "window:7", WindowTarget(7))
}

func TestSelectWindow(t *testing.T) {
	for _, tc := range []struct {
		target string
		call   []any
	}{
		{"window:3", []any{"focus-window", "--match", "id:3"}},
		{"tab:2", []any{"focus-tab", "--match", "id:2"}},
		{"sesh", []any{"focus-window", "--match", "id:1"}},
		{"Second Brain", []any{"focus-window", "--match", "id:4"}},
		{"notes", []any{"focus-window", "--match", "id:4"}},
	} {
		t.Run(tc.target, func(t *testing.T) {
			k, mShell := newKitty(t, "4", model.Config{})
			mShell.EXPECT().CmdCapture("kitten", append([]any{"@"}, tc.call...)...).Return("", nil).Once()

			_, err := k.SelectWindow(tc.target)

			require.NoError(t, err)
		})
	}

	t.Run("a session that isn't running", func(t *testing.T) {
		k, _ := newKitty(t, "4", model.Config{})

		_, err := k.SelectWindow("gone")

		assert.EqualError(t, err, "can't find kitty session: gone")
	})
}

func TestSelectPane(t *testing.T) {
	k, mShell := newKitty(t, "4", model.Config{})
	mShell.EXPECT().CmdCapture("kitten", "@", "focus-window", "--match", "id:3").Return("", nil).Once()
	mShell.EXPECT().CmdCapture("kitten", "@", "focus-window", "--match", "id:8").Return("", assert.AnError).Once()

	out, err := k.SelectPane(3)
	require.NoError(t, err)
	assert.Equal(t, "focused window 3", out)

	_, err = k.SelectPane(8)
	assert.ErrorIs(t, err, assert.AnError)
	assert.ErrorContains(t, err, "failed to focus window 8: kitten @ focus-window:")
}

func TestSetSessionName(t *testing.T) {
	k, mShell := newKitty(t, "4", model.Config{})
	mShell.EXPECT().CmdCapture("kitten", "@", "set-user-vars", "--match", "id:1 or id:2 or id:3", "ksesh_name=webapp/backend").Return("", nil).Once()
	mShell.EXPECT().CmdCapture("kitten", "@", "set-user-vars", "--match", "id:4", "ksesh_name").Return("", nil).Once()

	require.NoError(t, k.SetSessionName("sesh", "webapp/backend"))
	require.NoError(t, k.SetSessionName("notes", "notes"), "its own name drops the override")
	assert.EqualError(t, k.SetSessionName("Second Brain", "x"), "can't find kitty session: Second Brain",
		"it goes by kitty's name, which a session file gives")
}

func TestCloseWindow(t *testing.T) {
	k, mShell := newKitty(t, "4", model.Config{})
	mShell.EXPECT().CmdCapture("kitten", "@", "close-window", "--ignore-no-match", "--match", "id:5").Return("", nil).Once()

	require.NoError(t, k.CloseWindow(5))
}

func TestGotoSessionFile(t *testing.T) {
	k, mShell := newKitty(t, "4", model.Config{})
	mShell.EXPECT().CmdCapture("kitten", "@", "action", `goto_session '/p/bob'"'"'s app.kitty-session'`).Return("", nil).Once()
	mShell.EXPECT().CmdCapture("kitten", "@", "action", "goto_session '/p/broken.session'").Return("", assert.AnError).Once()

	name, err := k.GotoSessionFile("/p/bob's app.kitty-session")
	require.NoError(t, err)
	assert.Equal(t, "bob's app", name)

	_, err = k.GotoSessionFile("/p/broken.session")
	assert.ErrorIs(t, err, assert.AnError)
	assert.ErrorContains(t, err, "failed to load kitty session file /p/broken.session")
}

func TestCurrentWindowID(t *testing.T) {
	k, _ := newKitty(t, "2", model.Config{})
	id, err := k.CurrentWindowID()
	require.NoError(t, err)
	assert.Equal(t, 2, id, "the window ksesh runs in")

	k, _ = newKitty(t, "", model.Config{})
	id, err = k.CurrentWindowID()
	require.NoError(t, err)
	assert.Equal(t, 4, id, "outside kitty, the focused one")
}

func TestFocusedSessionPath(t *testing.T) {
	k, _ := newKitty(t, "1", model.Config{})
	path, err := k.FocusedSessionPath()
	require.NoError(t, err)
	assert.Equal(t, "/home/dev/c/sesh", path)

	k, _ = newKitty(t, "5", model.Config{})
	_, err = k.FocusedSessionPath()
	assert.EqualError(t, err, "not inside a kitty session")
}

// kittyAt is a kitty reached from outside, with KITTY_LISTEN_ON as given.
func kittyAt(t *testing.T, listenOn string, config model.Config) (*RealKitty, *shell.MockShell) {
	t.Helper()
	mOs := oswrap.NewMockOs(t)
	mOs.EXPECT().Getenv("KITTY_WINDOW_ID").Return("").Maybe()
	mOs.EXPECT().Getenv("KITTY_LISTEN_ON").Return(listenOn).Maybe()
	mOs.EXPECT().Getenv("TMPDIR").Return(t.TempDir()).Maybe()
	mShell := shell.NewMockShell(t)
	return NewKitty(mOs, mShell, config).(*RealKitty), mShell
}

func TestRemoteControlAddress(t *testing.T) {
	t.Run("outside kitty, KITTY_LISTEN_ON", func(t *testing.T) {
		k, mShell := kittyAt(t, "unix:/run/kitty.sock", model.Config{KittySocket: "unix:@ignored"})
		mShell.EXPECT().CmdCapture("kitten", "@", "--to", "unix:/run/kitty.sock", "close-window", "--ignore-no-match", "--match", "id:1").Return("", nil).Once()

		assert.True(t, k.Reachable())
		require.NoError(t, k.CloseWindow(1))
	})

	t.Run("outside kitty, the configured socket", func(t *testing.T) {
		k, mShell := kittyAt(t, "", model.Config{KittySocket: "unix:@kitty", KittenCommand: "/opt/kitten"})
		mShell.EXPECT().CmdCapture("/opt/kitten", "@", "--to", "unix:@kitty", "close-window", "--ignore-no-match", "--match", "id:1").Return("", nil).Once()

		assert.True(t, k.Reachable())
		require.NoError(t, k.CloseWindow(1))
	})

	t.Run("outside kitty, with nowhere to go", func(t *testing.T) {
		k, _ := kittyAt(t, "", model.Config{})

		assert.False(t, k.Reachable())
		assert.False(t, k.IsAttached())
	})

	t.Run("inside kitty, the tty", func(t *testing.T) {
		k, _ := newKitty(t, "4", model.Config{KittySocket: "unix:@kitty"})

		assert.True(t, k.Reachable())
	})
}

func TestResolveSocketFindsKittysSocket(t *testing.T) {
	t.Run("relative to the temp directory", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "kitty.sock"), nil, 0o600))

		assert.Equal(t, "unix:"+filepath.Join(dir, "kitty.sock"), resolveSocket("unix:kitty.sock", dir))
	})

	t.Run("under home", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		require.NoError(t, os.WriteFile(filepath.Join(home, "kitty.sock"), nil, 0o600))

		assert.Equal(t, "unix:"+filepath.Join(home, "kitty.sock"), resolveSocket("unix:~/kitty.sock", ""))
	})

	t.Run("suffixed with kitty's pid: the newest socket", func(t *testing.T) {
		// Unix socket paths are short (104 bytes on macOS): t.TempDir()'s
		// can be too long for one.
		dir, err := os.MkdirTemp("/tmp", "ksesh-sock")
		if err != nil {
			t.Skip("no /tmp to make a socket in")
		}
		t.Cleanup(func() { os.RemoveAll(dir) })
		base := filepath.Join(dir, "k")
		listen := func(path string, age time.Duration) {
			l, err := net.Listen("unix", path)
			require.NoError(t, err)
			t.Cleanup(func() { l.Close() })
			at := time.Now().Add(-age)
			require.NoError(t, os.Chtimes(path, at, at))
		}
		listen(base+"-100", time.Hour)
		listen(base+"-200", time.Minute)
		listen(base+"-300", 2*time.Hour)
		require.NoError(t, os.WriteFile(base+"-400", nil, 0o600), "newer, but not a socket")

		assert.Equal(t, "unix:"+base+"-200", resolveSocket("unix:"+base, dir))
	})
}

func TestStateDir(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", "/home/dev")
	dir, err := StateDir()
	require.NoError(t, err)
	assert.Equal(t, "/home/dev/.local/state/ksesh", dir)

	t.Setenv("XDG_STATE_HOME", "/state")
	dir, err = StateDir()
	require.NoError(t, err)
	assert.Equal(t, "/state/ksesh", dir)
}

func TestKittyOutOfReach(t *testing.T) {
	mOs := oswrap.NewMockOs(t)
	mOs.EXPECT().Getenv("KITTY_WINDOW_ID").Return("4").Maybe()
	mShell := shell.NewMockShell(t)
	mShell.EXPECT().CmdCapture("kitten", "@", "ls").Return("", assert.AnError)
	k := NewKitty(mOs, mShell, model.Config{}).(*RealKitty)

	sessions, err := k.ListSessions()
	require.NoError(t, err, "no kitty is no sessions, as no tmux server is")
	assert.Empty(t, sessions)
	windows, err := k.ListKittyWindows()
	require.NoError(t, err)
	assert.Empty(t, windows)

	for name, call := range map[string]func() error{
		"GetCurrentSession": func() error { _, err := k.GetCurrentSession(); return err },
		"CurrentWindowID":   func() error { _, err := k.CurrentWindowID(); return err },
		"ListAgents":        func() error { _, err := k.ListAgents(); return err },
		"FocusWindow":       func() error { return k.FocusWindow(1) },
		"SidebarTab":        func() error { _, err := k.SidebarTab(1); return err },
		"CloseSidebars":     func() error { return k.CloseSidebars() },
		"LastWorkWindow":    func() error { _, err := k.LastWorkWindow(1); return err },
		"WindowState":       func() error { _, _, err := k.WindowState(1); return err },
		"OverlayOn":         func() error { return k.OverlayOn(1, nil, []string{"sh"}) },
		"SetSessionName":    func() error { return k.SetSessionName("sesh", "x") },
		"ListAllWindows":    func() error { _, err := k.ListAllWindows(); return err },
	} {
		err := call()
		assert.ErrorIs(t, err, assert.AnError, name)
		assert.ErrorContains(t, err, "kitten @ ls:", name)
	}
}

func TestUnparseableLs(t *testing.T) {
	mOs := oswrap.NewMockOs(t)
	mOs.EXPECT().Getenv("KITTY_WINDOW_ID").Return("4").Maybe()
	mShell := shell.NewMockShell(t)
	mShell.EXPECT().CmdCapture("kitten", "@", "ls").Return("Error: remote control is disabled", nil)
	k := NewKitty(mOs, mShell, model.Config{}).(*RealKitty)

	_, err := k.GetCurrentSession()

	assert.ErrorContains(t, err, "kitten @ ls")
}

func TestLaunchOutputKsehCantRead(t *testing.T) {
	k, mShell := newSidebarKitty(t, "")
	mShell.EXPECT().CmdCapture("kitten", "@", "launch", "--match", "window_id:3", "--type=tab-dock", "--dock-edge=right", "--dock-size=20%",
		"--var=ksesh_role=sidebar", "--keep-focus", "--title=threads", "--", "ksesh", "sidebar").Return("Error: unknown option --type=tab-dock", nil).Once()

	_, err := k.OpenSidebar(3, SidebarOpts{Edge: "right", Size: "20%", Command: []string{"ksesh", "sidebar"}})

	assert.EqualError(t, err, `unexpected output from kitten @ launch: "Error: unknown option --type=tab-dock"`, "kitty older than 0.50")
}

func TestOpenSidebarReportsKittysError(t *testing.T) {
	k, mShell := newSidebarKitty(t, "")
	mShell.EXPECT().CmdCapture("kitten", "@", "launch", "--match", "window_id:3", "--type=tab-dock", "--dock-edge=left", "--dock-size=38",
		"--var=ksesh_role=sidebar", "--keep-focus", "--title=threads", "--", "ksesh", "sidebar").Return("", assert.AnError).Once()

	_, err := k.OpenSidebar(3, SidebarOpts{Command: []string{"ksesh", "sidebar"}})

	assert.ErrorIs(t, err, assert.AnError)
}

func TestSidebarLaunchArgs(t *testing.T) {
	assert.Equal(t, []string{"launch", "--type=tab-dock", "--dock-edge=left", "--dock-size=38", "--var=ksesh_role=sidebar",
		"--keep-focus", "--title=threads", "--"}, SidebarLaunchArgs(SidebarOpts{}), "the defaults, and no command")
	assert.Equal(t, []string{"launch", "--type=tab-dock", "--dock-edge=right", "--dock-size=25%", "--var=ksesh_role=sidebar",
		"--keep-focus", "--title=threads", "--", "/bin/ksesh", "--config", "/c.toml", "sidebar"},
		SidebarLaunchArgs(SidebarOpts{Edge: "right", Size: "25%", Command: []string{"/bin/ksesh", "--config", "/c.toml", "sidebar"}}))
}

func TestSidebarLookupsOfAWindowKittyDoesntHave(t *testing.T) {
	k, _ := newSidebarKitty(t, "")

	id, err := k.SidebarTab(99)
	require.NoError(t, err)
	assert.Zero(t, id)

	id, err = k.LastWorkWindow(99)
	require.NoError(t, err)
	assert.Zero(t, id)

	shown, focused, err := k.WindowState(99)
	require.NoError(t, err)
	assert.False(t, shown)
	assert.False(t, focused)
}

func TestWindowStateHasTheFocusOnlyInTheFocusedOSWindow(t *testing.T) {
	mOs := oswrap.NewMockOs(t)
	mOs.EXPECT().Getenv("KITTY_WINDOW_ID").Return("").Maybe()
	mOs.EXPECT().Getenv("KITTY_LISTEN_ON").Return("").Maybe()
	mOs.EXPECT().Getenv("TMPDIR").Return("").Maybe()
	mShell := shell.NewMockShell(t)
	mShell.EXPECT().CmdCapture("kitten", "@", "ls").Return(`[
		{"id":1,"is_focused":true,"tabs":[{"id":10,"is_active":true,"is_focused":true,"windows":[{"id":1,"is_focused":true}]}]},
		{"id":2,"is_focused":false,"tabs":[{"id":20,"is_active":true,"is_focused":true,"windows":[{"id":2,"is_focused":true}]}]}
	]`, nil)
	k := NewKitty(mOs, mShell, model.Config{}).(*RealKitty)

	for id, want := range map[int][2]bool{1: {true, true}, 2: {true, false}} {
		shown, focused, err := k.WindowState(id)
		require.NoError(t, err)
		assert.Equal(t, want, [2]bool{shown, focused}, "window %d", id)
	}
}

func TestUpdateAgentNeedsAWindow(t *testing.T) {
	t.Run("outside kitty, without one given", func(t *testing.T) {
		k, _ := newAgentKitty(t, "")

		_, err := k.UpdateAgent(0, agent.Update{Status: agent.Done})

		assert.EqualError(t, err, "not inside a kitty window")
	})

	t.Run("one kitty doesn't have", func(t *testing.T) {
		k, mShell := newAgentKitty(t, "7")
		mShell.EXPECT().CmdCapture("kitten", "@", "ls", "--match", "id:7").Return(`[]`, nil)

		err := k.SetAgentStatus(0, agent.Update{Status: agent.Waiting})

		assert.EqualError(t, err, "no kitty window 7")
	})

	t.Run("an answer that isn't ls's", func(t *testing.T) {
		k, mShell := newAgentKitty(t, "7")
		mShell.EXPECT().CmdCapture("kitten", "@", "ls", "--match", "id:7").Return(`No matching windows`, nil)

		err := k.SetAgentStatus(0, agent.Update{Status: agent.Error})

		assert.ErrorContains(t, err, "couldn't parse kitten @ ls output")
	})
}

func TestStartKitty(t *testing.T) {
	start := func(t *testing.T, ls string) (int, error) {
		t.Helper()
		t.Setenv("TMPDIR", t.TempDir())
		socket := filepath.Join(os.TempDir(), "ksesh-"+strconv.Itoa(os.Getpid())+".sock")
		k, mShell := kittyAt(t, "", model.Config{KittyCommand: "/opt/kitty"})
		mShell.EXPECT().CmdCapture("/opt/kitty", "--detach", "-o", "allow_remote_control=socket-only",
			"--listen-on", "unix:"+socket, "--directory", "/c/api").
			RunAndReturn(func(string, ...string) (string, error) {
				// kitty, starting up, opens its socket.
				return "", os.WriteFile(socket, nil, 0o600)
			}).Once()
		mShell.EXPECT().CmdCapture("kitten", "@", "--to", "unix:"+socket, "ls").Return(ls, nil).Once()
		return k.StartKitty("/c/api")
	}

	t.Run("returns its lone window, for the caller to close", func(t *testing.T) {
		id, err := start(t, `[{"id":1,"tabs":[{"id":1,"windows":[{"id":3}]}]}]`)

		require.NoError(t, err)
		assert.Equal(t, 3, id)
	})

	t.Run("keeps a startup session", func(t *testing.T) {
		id, err := start(t, `[{"id":1,"tabs":[{"id":1,"windows":[{"id":3,"session_name":"work"}]}]}]`)

		require.NoError(t, err)
		assert.Zero(t, id)
	})

	t.Run("keeps several windows", func(t *testing.T) {
		id, err := start(t, `[{"id":1,"tabs":[{"id":1,"windows":[{"id":3},{"id":4}]}]}]`)

		require.NoError(t, err)
		assert.Zero(t, id)
	})

	t.Run("kitty failing to start", func(t *testing.T) {
		k, mShell := kittyAt(t, "", model.Config{})
		mShell.EXPECT().CmdCapture("kitty", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("", assert.AnError).Once()

		_, err := k.StartKitty("")

		assert.ErrorIs(t, err, assert.AnError)
		assert.ErrorContains(t, err, "failed to start kitty")
	})
}
