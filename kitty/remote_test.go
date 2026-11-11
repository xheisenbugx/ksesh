package kitty

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/xheisenbugx/ksesh/model"
)

func TestSSHDestination(t *testing.T) {
	cases := map[string][]string{
		"devbox":         {"ssh", "devbox"},
		"me@devbox":      {"/usr/bin/ssh", "-p", "2222", "-o", "ControlMaster=auto", "-tt", "me@devbox", "ls"},
		"box":            {"ssh", "-p2222", "box"},
		"devbox.example": {"/Applications/kitty.app/Contents/MacOS/kitten", "ssh", "--kitten", "cwd=/x", "--kitten=env=A=b", "devbox.example"},
		"after":          {"ssh", "-v", "--", "after"},
	}
	for want, cmdline := range cases {
		got, ok := SSHDestination(cmdline)
		assert.True(t, ok, cmdline)
		assert.Equal(t, want, got, cmdline)
	}
	_, ok := SSHDestination([]string{"claude", "--resume", "x"})
	assert.False(t, ok, "not ssh")
	_, ok = SSHDestination([]string{"ssh", "-p", "22"})
	assert.False(t, ok, "no destination")
}

func TestAnAgentOnAnotherMachine(t *testing.T) {
	w := lsWindow{ID: 3, Cwd: "/Users/me", UserVars: map[string]string{
		StatusVar: "waiting", AgentVar: "claude", HostVar: "ip-10-0-1-5", CwdVar: "/home/me/c/api",
	}, ForegroundProcesses: []lsProcess{{Cmdline: []string{"kitten", "ssh", "devbox"}, Cwd: "/Users/me"}}}
	a, ok := agentOf(w)
	assert.True(t, ok)
	assert.Equal(t, "devbox", a.Host, "named as ssh reaches it, not by its hostname")
	assert.Equal(t, "/home/me/c/api", a.Path, "its directory there, not the window's here")

	w.ForegroundProcesses = nil
	a, _ = agentOf(w)
	assert.Equal(t, "ip-10-0-1-5", a.Host, "the hook's name when the window doesn't say")

	w.UserVars[HostVar] = thisHost()
	a, _ = agentOf(w)
	assert.Empty(t, a.Host, "this machine")
	assert.Equal(t, "/Users/me", a.Path)
}

func TestSameHost(t *testing.T) {
	assert.True(t, sameHost("box", "box.local"))
	assert.True(t, sameHost("Box.local", "box"))
	assert.False(t, sameHost("box", "devbox"))
	assert.False(t, sameHost("", ""))
}

func TestThreadVars(t *testing.T) {
	at := time.Unix(1700000000, 0)
	vars := ThreadVars(model.Thread{Pin: 2, SettledAt: at})
	assert.Equal(t, "2", vars[PinVar])
	assert.Equal(t, "1700000000", vars[SettledAtVar])
	_, unset := vars[SnoozeUntilVar]
	assert.False(t, unset, "only what is set")
}
