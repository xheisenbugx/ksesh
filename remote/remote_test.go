package remote

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xheisenbugx/ksesh/model"
	"github.com/xheisenbugx/ksesh/shell"
)

func TestQuoting(t *testing.T) {
	assert.Equal(t, "plain/path-1.2", Quote("plain/path-1.2"))
	assert.Equal(t, "'two words'", Quote("two words"))
	assert.Equal(t, `'it'"'"'s'`, Quote("it's"))
	assert.Equal(t, "''", Quote(""))
	assert.Equal(t, "ksesh agent diff --dir '/c/my app'", Join("ksesh", "agent", "diff", "--dir", "/c/my app"))
	assert.Equal(t, `"$HOME"'/c/my app'`, ShellDir("~/c/my app"))
	assert.Equal(t, `/srv/api`, ShellDir("/srv/api"))
	assert.Equal(t, "$HOME/c", RemoteDir("~/c"))
}

func TestSessionName(t *testing.T) {
	assert.Equal(t, "api@devbox", SessionName("devbox", "/home/me/c/api/"))
	assert.Equal(t, "c@devbox", SessionName("devbox", "~/c"))
}

func TestWindowForwardsRemoteControl(t *testing.T) {
	assert.Equal(t, []string{"kitten", "ssh",
		"--kitten", "forward_remote_control=yes",
		"--kitten", "env=KSESH_HOST=devbox",
		"--kitten", "env=KITTY_WINDOW_ID=_kitty_copy_env_var_",
		"--kitten", "cwd=$HOME/c/api",
		"devbox"}, Window("devbox", "$HOME/c/api"))
}

func TestRunGoesThroughALoginShell(t *testing.T) {
	sh := shell.NewMockShell(t)
	sh.EXPECT().CmdCapture("ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=5", "devbox",
		`exec "${SHELL:-/bin/sh}" -lc 'cd /c/api && ksesh agent branch /c/api'`).Return("{}", nil)
	out, err := NewRemote(sh, nil).Run("devbox", "/c/api", "ksesh", "agent", "branch", "/c/api")
	require.NoError(t, err)
	assert.Equal(t, "{}", out)
}

func TestDirs(t *testing.T) {
	sh := shell.NewMockShell(t)
	sh.EXPECT().CmdCapture("ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=5", "devbox",
		`exec "${SHELL:-/bin/sh}" -lc 'printf '"'"'%s\n'"'"' "$HOME"; command -v zoxide >/dev/null && zoxide query -l'`).
		Return("/home/me\n/home/me/c/api\n/srv/logs\n", nil)
	hosts := []model.HostConfig{{Name: "devbox", Paths: []string{"~/c/web", "/srv/logs"}}}

	home, dirs, err := NewRemote(sh, hosts).Dirs("devbox")
	require.NoError(t, err)
	assert.Equal(t, "/home/me", home)
	assert.Equal(t, []string{"/home/me/c/web", "/srv/logs", "/home/me/c/api"}, dirs, "configured paths first, ~ expanded, no duplicates")
}

func TestDirsWithoutZoxide(t *testing.T) {
	sh := shell.NewMockShell(t)
	sh.EXPECT().CmdCapture("ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=5", "box",
		`exec "${SHELL:-/bin/sh}" -lc 'printf '"'"'%s\n'"'"' "$HOME"'`).Return("/root\n", nil)
	no := false
	_, dirs, err := NewRemote(sh, []model.HostConfig{{Name: "box", Zoxide: &no, Paths: []string{"/srv"}}}).Dirs("box")
	require.NoError(t, err)
	assert.Equal(t, []string{"/srv"}, dirs)
}
