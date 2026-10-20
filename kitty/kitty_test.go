package kitty

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xheisenbugx/ksesh/model"
	"github.com/xheisenbugx/ksesh/oswrap"
	"github.com/xheisenbugx/ksesh/shell"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func fixture(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "ls.json"))
	require.NoError(t, err)
	return string(data)
}

// newKitty builds a RealKitty that runs inside kitty window selfID (0 for
// outside kitty) and answers `kitten @ ls` with the fixture.
func newKitty(t *testing.T, selfID string, config model.Config) (*RealKitty, *shell.MockShell) {
	t.Helper()
	mOs := oswrap.NewMockOs(t)
	mOs.EXPECT().Getenv("KITTY_WINDOW_ID").Return(selfID).Maybe()
	mOs.EXPECT().Getenv("KITTY_LISTEN_ON").Return("").Maybe()
	mOs.EXPECT().Getenv("TMPDIR").Return("").Maybe()
	mShell := shell.NewMockShell(t)
	mShell.EXPECT().CmdCapture("kitten", "@", "ls").Return(fixture(t), nil).Maybe()
	return NewKitty(mOs, mShell, config).(*RealKitty), mShell
}

func TestListSessions(t *testing.T) {
	k, _ := newKitty(t, "4", model.Config{})

	sessions, err := k.ListSessions()

	require.NoError(t, err)
	require.Len(t, sessions, 2, "the window outside any session isn't a session")

	notes := sessions[0]
	assert.Equal(t, "Second Brain", notes.Name, "a rename is read from the user variable")
	assert.Equal(t, "notes", notes.NativeName)
	assert.Equal(t, 1, notes.Attached)
	assert.Equal(t, "/home/dev/notes/daily", notes.Path, "without a root variable the first window's live cwd is the root")
	assert.Equal(t, []string{"bell"}, notes.Alerts)
	assert.Equal(t, 4, notes.FocusWindowID, "the window sesh runs in is used when it is the session's only one")

	sesh := sessions[1]
	assert.Equal(t, "sesh", sesh.Name)
	assert.Equal(t, 0, sesh.Attached)
	assert.Equal(t, "/home/dev/c/sesh", sesh.Path)
	assert.Equal(t, 2, sesh.Windows)
	assert.Equal(t, 1, sesh.FocusWindowID)
	assert.Equal(t, []string{"activity"}, sesh.Alerts)
	require.NotNil(t, sesh.Created)
	assert.Equal(t, int64(1700000000), sesh.Created.Unix())
}

func TestCurrentSessionOutsideKitty(t *testing.T) {
	k, _ := newKitty(t, "", model.Config{})

	name, err := k.GetCurrentSession()

	require.NoError(t, err)
	assert.Equal(t, "Second Brain", name, "the focused window of the focused OS window decides")
}

func TestCurrentSessionFromAnOverlay(t *testing.T) {
	// An overlay opened by a key binding belongs to no session; the session
	// of its tab is the one the user is in.
	mOs := oswrap.NewMockOs(t)
	mOs.EXPECT().Getenv("KITTY_WINDOW_ID").Return("9").Maybe()
	mShell := shell.NewMockShell(t)
	mShell.EXPECT().CmdCapture("kitten", "@", "ls").Return(`[{"id":1,"is_active":true,"tabs":[
		{"id":1,"is_active":false,"windows":[{"id":1,"session_name":"api","last_focused_at":5}]},
		{"id":2,"is_active":true,"windows":[
			{"id":2,"session_name":"web","last_focused_at":3},
			{"id":9,"is_active":true,"session_name":"","last_focused_at":6}]}]}]`, nil)
	k := NewKitty(mOs, mShell, model.Config{})

	name, err := k.GetCurrentSession()

	require.NoError(t, err)
	assert.Equal(t, "web", name)
}

func TestListWindows(t *testing.T) {
	k, _ := newKitty(t, "4", model.Config{})

	tabs, err := k.ListWindows("sesh")

	require.NoError(t, err)
	assert.Equal(t, []*model.KittyTab{
		{Name: "nvim", Path: "/home/dev/c/sesh/lister", Index: 1, ID: 1, Active: true},
		{Name: "server", Path: "/home/dev/c/sesh", Index: 2, ID: 2, Active: false},
	}, tabs)

	_, err = k.ListWindows("nope")
	assert.ErrorContains(t, err, "can't find kitty session: nope")
}

func TestListAllWindowNames(t *testing.T) {
	k, _ := newKitty(t, "4", model.Config{})

	names, err := k.ListAllWindowNames("{active_title}")

	require.NoError(t, err)
	assert.Equal(t, map[string][]string{"sesh": {"nvim"}, "Second Brain": {"notes"}}, names)
}

func TestListKittyWindows(t *testing.T) {
	t.Run("leaves out the window sesh runs in", func(t *testing.T) {
		k, _ := newKitty(t, "4", model.Config{})
		windows, err := k.ListKittyWindows()
		require.NoError(t, err)
		assert.Empty(t, windows)
	})

	t.Run("lists the current session's windows", func(t *testing.T) {
		k, _ := newKitty(t, "1", model.Config{})
		windows, err := k.ListKittyWindows()
		require.NoError(t, err)
		assert.Equal(t, []*model.KittyWindow{
			{TabIndex: 1, TabTitle: "nvim", ID: 2, Title: "zsh", Command: "zsh", Path: "/home/dev/c/sesh"},
			{TabIndex: 2, TabTitle: "server", ID: 3, Title: "server", Command: "zsh", Path: "/home/dev/c/sesh"},
		}, windows)
	})
}

func TestNewSession(t *testing.T) {
	t.Run("adds a tab to the session in the background", func(t *testing.T) {
		k, mShell := newKitty(t, "4", model.Config{})
		mShell.EXPECT().CmdCapture("kitten", "@", "launch", "--type=tab", "--keep-focus",
			"--add-to-session", "api", "--cwd", "/home/dev/c/api", "--var", "ksesh_root=/home/dev/c/api").
			Return("12\n", nil)

		target, err := k.NewSession("api", "/home/dev/c/api")

		require.NoError(t, err)
		assert.Equal(t, "window:12", target)
	})

	t.Run("opens an OS window with new_os_window", func(t *testing.T) {
		k, mShell := newKitty(t, "4", model.Config{NewOSWindow: true})
		mShell.EXPECT().CmdCapture("kitten", "@", "launch", "--type=os-window", "--keep-focus",
			"--add-to-session", "api", "--cwd", "/home/dev/c/api", "--var", "ksesh_root=/home/dev/c/api").
			Return("12", nil)

		_, err := k.NewSession("api", "/home/dev/c/api")

		require.NoError(t, err)
	})

	t.Run("goes through the configured socket outside kitty", func(t *testing.T) {
		mOs := oswrap.NewMockOs(t)
		mOs.EXPECT().Getenv("KITTY_WINDOW_ID").Return("")
		mOs.EXPECT().Getenv("KITTY_LISTEN_ON").Return("unix:/tmp/kitty-123")
		mShell := shell.NewMockShell(t)
		mShell.EXPECT().CmdCapture("kitten", "@", "--to", "unix:/tmp/kitty-123", "launch", "--type=tab", "--keep-focus",
			"--add-to-session", "api").Return("3", nil)
		k := NewKitty(mOs, mShell, model.Config{})

		_, err := k.NewSession("api", "")

		require.NoError(t, err)
	})

	t.Run("gives kitty a path-safe name and keeps the real one", func(t *testing.T) {
		k, mShell := newKitty(t, "4", model.Config{})
		mShell.EXPECT().CmdCapture("kitten", "@", "launch", "--type=tab", "--keep-focus",
			"--add-to-session", "webapp-backend", "--cwd", "/home/dev/webapp/backend",
			"--var", "ksesh_root=/home/dev/webapp/backend", "--var", "ksesh_name=webapp/backend").
			Return("12", nil)

		_, err := k.NewSession("webapp/backend", "/home/dev/webapp/backend")

		require.NoError(t, err)
	})

	t.Run("refuses names kitty reads specially", func(t *testing.T) {
		k, _ := newKitty(t, "4", model.Config{})
		_, err := k.NewSession(".", "/tmp")
		assert.Error(t, err)
	})
}

func TestNewWindowInSession(t *testing.T) {
	k, mShell := newKitty(t, "4", model.Config{})
	mShell.EXPECT().CmdCapture("kitten", "@", "launch", "--type=tab",
		"--add-to-session", "notes", "--match", "id:4", "--keep-focus",
		"--tab-title", "agent", "--cwd", "/home/dev/notes", "--var", "ksesh_root=/home/dev/notes/daily",
		"--var", "ksesh_name=Second Brain", "--", mock.Anything, "-c", "claude").
		Return("9", nil)

	target, err := k.NewWindowInSession(model.KittyTabOpts{
		Name:          "agent",
		StartDir:      "/home/dev/notes",
		TargetSession: "Second Brain",
		Command:       "claude",
		Background:    true,
	})

	require.NoError(t, err)
	assert.Equal(t, "window:9", target)
}

func TestSendKeys(t *testing.T) {
	k, mShell := newKitty(t, "4", model.Config{})
	mShell.EXPECT().CmdCapture("kitten", "@", "send-text", "--match", "id:1", "--", `printf 'a\\nb'\r`).Return("", nil)
	mShell.EXPECT().CmdCapture("kitten", "@", "send-text", "--match", "id:7", "--", `ls\r`).Return("", nil)
	mShell.EXPECT().CmdCapture("kitten", "@", "send-text", "--match-tab", "id:2", "--", `ls\r`).Return("", nil)

	_, err := k.SendKeys("sesh", `printf 'a\nb'`)
	require.NoError(t, err)
	_, err = k.SendKeys("window:7", "ls")
	require.NoError(t, err)
	_, err = k.SendKeys("tab:2", "ls")
	require.NoError(t, err)
}

func TestSwitchOrAttach(t *testing.T) {
	t.Run("uses goto_session so kitty's session history follows", func(t *testing.T) {
		k, mShell := newKitty(t, "4", model.Config{})
		mShell.EXPECT().CmdCapture("kitten", "@", "action", "goto_session '/sesh.kitty-session'").Return("", nil)

		msg, err := k.SwitchOrAttach("sesh", model.ConnectOpts{})

		require.NoError(t, err)
		assert.Equal(t, "switching to kitty session: sesh", msg)
	})

	t.Run("focuses the window for names a path can't carry", func(t *testing.T) {
		mOs := oswrap.NewMockOs(t)
		mOs.EXPECT().Getenv("KITTY_WINDOW_ID").Return("1").Maybe()
		mShell := shell.NewMockShell(t)
		mShell.EXPECT().CmdCapture("kitten", "@", "ls").Return(`[{"id":1,"is_active":true,"tabs":[{"id":1,"is_active":true,"windows":[
			{"id":1,"is_active":true,"session_name":"repo/main","last_focused_at":1},
			{"id":2,"is_active":false,"session_name":"repo/feat","last_focused_at":0.5}]}]}]`, nil)
		mShell.EXPECT().CmdCapture("kitten", "@", "focus-window", "--match", "id:2").Return("", nil)
		k := NewKitty(mOs, mShell, model.Config{})

		_, err := k.SwitchOrAttach("repo/feat", model.ConnectOpts{})

		require.NoError(t, err)
	})
}

func TestRenameSession(t *testing.T) {
	k, mShell := newKitty(t, "4", model.Config{})
	mShell.EXPECT().CmdCapture("kitten", "@", "set-user-vars", "--match", "id:1 or id:2 or id:3", "ksesh_name=sesh — fix the bug").Return("", nil)
	mShell.EXPECT().CmdCapture("kitten", "@", "set-user-vars", "--match", "id:4", "ksesh_name").Return("", nil)

	_, err := k.RenameSession("sesh", "sesh — fix the bug")
	require.NoError(t, err)
	// Renaming back to kitty's own name drops the override.
	_, err = k.RenameSession("Second Brain", "notes")
	require.NoError(t, err)
}

func TestKillSession(t *testing.T) {
	k, mShell := newKitty(t, "4", model.Config{})
	mShell.EXPECT().CmdCapture("kitten", "@", "close-window", "--ignore-no-match", "--match", "id:1 or id:2 or id:3").Return("", nil)

	_, err := k.KillSession("sesh")

	require.NoError(t, err)
}

func TestCapturePane(t *testing.T) {
	k, mShell := newKitty(t, "4", model.Config{})
	mShell.EXPECT().CmdCapture("kitten", "@", "get-text", "--ansi", "--match", "id:1").Return("$ nvim", nil)

	out, err := k.CapturePane("sesh")

	require.NoError(t, err)
	assert.Equal(t, "$ nvim", out)
}

func TestFreeze(t *testing.T) {
	t.Run("serves the snapshot and defers kills while a TUI owns the tty", func(t *testing.T) {
		k, mShell := newKitty(t, "4", model.Config{})
		mShell.EXPECT().CmdCapture("kitten", "@", "get-text", "--ansi", "--match", "id:4").Return("notes screen", nil).Once()
		mShell.EXPECT().CmdCapture("kitten", "@", "get-text", "--ansi", "--match", "id:1").Return("sesh screen", nil).Once()

		require.NoError(t, k.Freeze(true))

		out, err := k.CapturePane("sesh")
		require.NoError(t, err)
		assert.Equal(t, "sesh screen", out)

		_, err = k.KillSession("sesh")
		require.NoError(t, err)
		sessions, err := k.ListSessions()
		require.NoError(t, err)
		require.Len(t, sessions, 1, "a killed session leaves the snapshot at once")

		mShell.EXPECT().CmdCapture("kitten", "@", "close-window", "--ignore-no-match", "--match", "id:1 or id:2 or id:3").Return("", nil).Once()
		require.NoError(t, k.Thaw())
		mShell.AssertNumberOfCalls(t, "CmdCapture", 4)
	})

	t.Run("does nothing over a socket", func(t *testing.T) {
		mOs := oswrap.NewMockOs(t)
		mOs.EXPECT().Getenv("KITTY_WINDOW_ID").Return("4")
		mOs.EXPECT().Getenv("KITTY_LISTEN_ON").Return("unix:/tmp/kitty-1")
		k := NewKitty(mOs, shell.NewMockShell(t), model.Config{}).(*RealKitty)

		require.NoError(t, k.Freeze(true))
		assert.False(t, k.isFrozen())
	})
}

func TestFormatTab(t *testing.T) {
	tab := model.KittyTab{Name: "nvim", Path: "/home/dev/c/sesh", Index: 2, ID: 7, Active: true}
	assert.Equal(t, "nvim", FormatTab("", tab))
	assert.Equal(t, "2:nvim* sesh", FormatTab("{index}:{title}{active} {cwd_name}", tab))
	assert.Equal(t, "2 nvim /home/dev/c/sesh", FormatTab("#{window_index} #{window_name} #{pane_current_path}", tab))
	tab.Active = false
	assert.Equal(t, "", FormatTab("{active_title}", tab))
}

func TestNativeName(t *testing.T) {
	assert.Equal(t, "sesh", NativeName("sesh"))
	assert.Equal(t, "Downloads 📥", NativeName("Downloads 📥"))
	assert.Equal(t, "webapp-backend", NativeName("webapp/backend"))
	assert.Equal(t, "it-s", NativeName("it's"))
	assert.Equal(t, "rc", NativeName("-rc"))
	assert.Equal(t, "session", NativeName("/"))
}

func TestSessionFileName(t *testing.T) {
	assert.Equal(t, "work", SessionFileName("/p/work.kitty-session"))
	assert.Equal(t, "work", SessionFileName("/p/work.kitty_session"))
	assert.Equal(t, "my.proj", SessionFileName("/p/my.proj.session"))
	assert.Equal(t, "notes.txt", SessionFileName("/p/notes.txt"))
}

func TestResolveSocket(t *testing.T) {
	dir := t.TempDir()
	assert.Equal(t, "", resolveSocket("", dir))
	assert.Equal(t, "tcp:localhost:5000", resolveSocket("tcp:localhost:5000", dir))
	assert.Equal(t, "unix:@kitty", resolveSocket("unix:@kitty", dir))
	assert.Equal(t, "", resolveSocket("unix:"+filepath.Join(dir, "missing"), dir))
}
