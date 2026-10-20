package kitty

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/xheisenbugx/ksesh/agent"
	"github.com/xheisenbugx/ksesh/model"
	"github.com/xheisenbugx/ksesh/oswrap"
	"github.com/xheisenbugx/ksesh/shell"
)

const agentsLs = `[{"id":1,"is_active":true,"is_focused":true,"tabs":[
	{"id":10,"is_active":true,"title":"code","windows":[
		{"id":1,"is_active":true,"session_name":"api","last_focused_at":50,"cwd":"/c/api",
		 "user_vars":{"ksesh_agent":"claude","ksesh_status":"working","ksesh_status_at":"1700000000","ksesh_prompt":"fix it"}},
		{"id":2,"session_name":"api","last_focused_at":40,"cwd":"/c/api",
		 "user_vars":{"ksesh_agent":"claude","ksesh_status":"waiting","ksesh_status_at":"1700000100","ksesh_msg":"Needs permission: Bash"}}
	]},
	{"id":11,"title":"web","windows":[
		{"id":3,"session_name":"web","last_focused_at":30,"cwd":"/c/web",
		 "foreground_processes":[{"cmdline":["codex"],"cwd":"/c/web"}]},
		{"id":4,"session_name":"web","last_focused_at":20,"cwd":"/c/web",
		 "foreground_processes":[{"cmdline":["-zsh"],"cwd":"/c/web"}]}
	]}
]}]`

func newAgentKitty(t *testing.T, selfID string) (*RealKitty, *shell.MockShell) {
	t.Helper()
	mOs := oswrap.NewMockOs(t)
	mOs.EXPECT().Getenv("KITTY_WINDOW_ID").Return(selfID).Maybe()
	mOs.EXPECT().Getenv("KITTY_LISTEN_ON").Return("").Maybe()
	mOs.EXPECT().Getenv("TMPDIR").Return("").Maybe()
	mShell := shell.NewMockShell(t)
	mShell.EXPECT().CmdCapture("kitten", "@", "ls").Return(agentsLs, nil).Maybe()
	return NewKitty(mOs, mShell, model.Config{}).(*RealKitty), mShell
}

func TestListAgents(t *testing.T) {
	k, _ := newAgentKitty(t, "")

	agents, err := k.ListAgents()

	require.NoError(t, err)
	require.Len(t, agents, 3, "the plain shell isn't an agent")

	working := agents[0]
	assert.Equal(t, 1, working.WindowID)
	assert.Equal(t, "claude", working.Name)
	assert.Equal(t, "working", working.Status)
	assert.Equal(t, "fix it", working.Prompt)
	assert.Equal(t, "api", working.Session)
	assert.Equal(t, "code", working.TabTitle)
	assert.True(t, working.Focused)
	require.NotNil(t, working.Since)
	assert.Equal(t, int64(1700000000), working.Since.Unix())

	assert.Equal(t, "Needs permission: Bash", agents[1].Message)

	codex := agents[2]
	assert.Equal(t, "codex", codex.Name, "found by its program, without hooks")
	assert.Equal(t, "idle", codex.Status)
	assert.Equal(t, "web", codex.Session)
}

func TestSessionsCarryTheirMostUrgentAgent(t *testing.T) {
	k, _ := newAgentKitty(t, "")

	sessions, err := k.ListSessions()

	require.NoError(t, err)
	byName := map[string]string{}
	for _, s := range sessions {
		byName[s.Name] = s.AgentStatus
	}
	assert.Equal(t, "waiting", byName["api"])
	assert.Equal(t, "", byName["web"], "an agent found by its program reports nothing")
}

func TestSetAgentStatus(t *testing.T) {
	k, mShell := newAgentKitty(t, "7")
	mShell.EXPECT().CmdCapture("kitten", "@", "set-user-vars", "--match", "id:7",
		"ksesh_agent=claude", "ksesh_msg=Needs permission", mock.MatchedBy(func(s string) bool {
			return strings.HasPrefix(s, "ksesh_status_at=")
		}), "ksesh_status=waiting").Return("", nil)

	err := k.SetAgentStatus(0, agent.Update{Agent: "claude", Status: agent.Waiting, Message: "Needs permission", SetMessage: true})

	require.NoError(t, err)
}

func TestSetAgentStatusClears(t *testing.T) {
	k, mShell := newAgentKitty(t, "7")
	mShell.EXPECT().CmdCapture("kitten", "@", "set-user-vars", "--match", "id:3",
		"ksesh_agent", "ksesh_status", "ksesh_status_at", "ksesh_msg", "ksesh_prompt").Return("", nil)

	require.NoError(t, k.SetAgentStatus(3, agent.Update{Clear: true}))
}

func TestFocusWindowGoesThroughItsSession(t *testing.T) {
	k, mShell := newAgentKitty(t, "")
	mShell.EXPECT().CmdCapture("kitten", "@", "action", "goto_session '/web.kitty-session'").Return("", nil)
	mShell.EXPECT().CmdCapture("kitten", "@", "focus-window", "--match", "id:3").Return("", nil)

	require.NoError(t, k.FocusWindow(3))
}

func TestFocusWindowInTheCurrentSession(t *testing.T) {
	k, mShell := newAgentKitty(t, "")
	mShell.EXPECT().CmdCapture("kitten", "@", "focus-window", "--match", "id:2").Return("", nil)

	require.NoError(t, k.FocusWindow(2))
}
