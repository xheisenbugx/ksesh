package kitty

import (
	"strings"
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

// windowLs is `kitten @ ls --match id:7` for a window with these user vars.
func windowLs(vars string) string {
	return `[{"id":1,"tabs":[{"id":10,"windows":[{"id":7,"user_vars":{` + vars + `}}]}]}]`
}

func TestSetAgentStatus(t *testing.T) {
	k, mShell := newAgentKitty(t, "7")
	mShell.EXPECT().CmdCapture("kitten", "@", "ls", "--match", "id:7").Return(windowLs(``), nil)
	mShell.EXPECT().CmdCapture("kitten", "@", "set-user-vars", "--match", "id:7",
		"ksesh_agent=claude", "ksesh_msg=Needs permission", mock.MatchedBy(func(s string) bool {
			return strings.HasPrefix(s, "ksesh_status_at=")
		}), "ksesh_status=waiting").Return("", nil)

	err := k.SetAgentStatus(0, agent.Update{Agent: "claude", Status: agent.Waiting, Message: "Needs permission", SetMessage: true})

	require.NoError(t, err)
}

func TestSetAgentStatusStartsATurn(t *testing.T) {
	k, mShell := newAgentKitty(t, "7")
	at := mock.MatchedBy(func(s string) bool { return strings.HasPrefix(s, "ksesh_turn_at=") })
	statusAt := mock.MatchedBy(func(s string) bool { return strings.HasPrefix(s, "ksesh_status_at=") })
	mShell.EXPECT().CmdCapture("kitten", "@", "ls", "--match", "id:7").Return(windowLs(``), nil)
	mShell.EXPECT().CmdCapture("kitten", "@", "set-user-vars", "--match", "id:7",
		"ksesh_agent=claude", "ksesh_msg", "ksesh_prompt=fix it", at, statusAt, "ksesh_status=working").Return("", nil)

	err := k.SetAgentStatus(0, agent.Update{Agent: "claude", Status: agent.Working, SetMessage: true, Prompt: "fix it", SetPrompt: true, StartTurn: true})

	require.NoError(t, err)
}

func TestSetAgentStatusClears(t *testing.T) {
	k, mShell := newAgentKitty(t, "7")
	mShell.EXPECT().CmdCapture("kitten", "@", "set-user-vars", "--match", "id:3",
		"ksesh_agent", "ksesh_status", "ksesh_status_at", "ksesh_msg", "ksesh_prompt", "ksesh_turn_at", "ksesh_conv",
		"ksesh_pin", "ksesh_snooze_until", "ksesh_snoozed_at", "ksesh_settled_at", "ksesh_unsettled", "ksesh_woke").Return("", nil)

	require.NoError(t, k.SetAgentStatus(3, agent.Update{Clear: true}))
}

func TestUpdateAgentRecordsTheConversation(t *testing.T) {
	k, mShell := newAgentKitty(t, "7")
	statusAt := mock.MatchedBy(func(s string) bool { return strings.HasPrefix(s, "ksesh_status_at=") })
	mShell.EXPECT().CmdCapture("kitten", "@", "set-user-vars", "--match", "id:7",
		"ksesh_conv=abc", "ksesh_agent=claude", statusAt, "ksesh_status=working").Return("", nil)

	seen, err := k.UpdateAgent(0, agent.Update{Agent: "claude", Status: agent.Working, Conversation: "abc"})

	require.NoError(t, err)
	assert.Nil(t, seen, "a tool call doesn't pay for reading the window")
}

func TestUpdateAgentCarriesAResumedThreadOver(t *testing.T) {
	k, mShell := newAgentKitty(t, "7")
	statusAt := mock.MatchedBy(func(s string) bool { return strings.HasPrefix(s, "ksesh_status_at=") })
	mShell.EXPECT().CmdCapture("kitten", "@", "ls", "--match", "id:7").Return(
		`[{"id":1,"tabs":[{"id":10,"windows":[{"id":7,"title":"✳ Fix the login","session_name":"api-native","cwd":"/c/api",
		  "user_vars":{"ksesh_name":"api"}}]}]}]`, nil)
	mShell.EXPECT().CmdCapture("kitten", "@", "set-user-vars", "--match", "id:7",
		"ksesh_pin=5", "ksesh_snooze_until", "ksesh_snoozed_at", "ksesh_settled_at", "ksesh_unsettled=1", "ksesh_woke",
		"ksesh_conv=abc", "ksesh_agent=claude", "ksesh_msg", "ksesh_prompt", statusAt, "ksesh_status=idle").Return("", nil)

	carry := model.Thread{Pin: 5, Unsettled: true}
	seen, err := k.UpdateAgent(0, agent.Update{Agent: "claude", Status: agent.Idle, SetMessage: true, SetPrompt: true,
		Start: true, Conversation: "abc", Carry: &carry})

	require.NoError(t, err)
	require.NotNil(t, seen)
	assert.Equal(t, 7, seen.WindowID)
	assert.Equal(t, "✳ Fix the login", seen.Title)
	assert.Equal(t, "api", seen.Session, "the session by its ksesh name")
	assert.Equal(t, carry, seen.Thread)
}

func TestUpdateAgentKeepsTheThreadOfItsOwnConversation(t *testing.T) {
	k, mShell := newAgentKitty(t, "7")
	mShell.EXPECT().CmdCapture("kitten", "@", "ls", "--match", "id:7").Return(windowLs(`"ksesh_conv":"abc","ksesh_pin":"3"`), nil)
	mShell.EXPECT().CmdCapture("kitten", "@", "set-user-vars", "--match", "id:7",
		"ksesh_conv=abc", "ksesh_agent=claude", mock.Anything, "ksesh_status=idle").Return("", nil)

	carry := model.Thread{SettledAt: time.Unix(1, 0)}
	seen, err := k.UpdateAgent(0, agent.Update{Agent: "claude", Status: agent.Idle, Start: true, Conversation: "abc", Carry: &carry})

	require.NoError(t, err)
	assert.Equal(t, model.Thread{Pin: 3}, seen.Thread, "compacting restarts the session, not the thread")
}

func TestUpdateAgentStartsAFreshThreadForANewConversation(t *testing.T) {
	k, mShell := newAgentKitty(t, "7")
	mShell.EXPECT().CmdCapture("kitten", "@", "ls", "--match", "id:7").Return(windowLs(`"ksesh_conv":"old","ksesh_pin":"3"`), nil)
	mShell.EXPECT().CmdCapture("kitten", "@", "set-user-vars", "--match", "id:7",
		"ksesh_pin", "ksesh_snooze_until", "ksesh_snoozed_at", "ksesh_settled_at", "ksesh_unsettled", "ksesh_woke",
		"ksesh_conv=new", "ksesh_agent=claude", mock.Anything, "ksesh_status=idle").Return("", nil)

	seen, err := k.UpdateAgent(0, agent.Update{Agent: "claude", Status: agent.Idle, Start: true, Conversation: "new"})

	require.NoError(t, err)
	assert.Equal(t, model.Thread{}, seen.Thread)
}

func TestUpdateAgentSeesTheThreadBeforeClearingIt(t *testing.T) {
	k, mShell := newAgentKitty(t, "7")
	mShell.EXPECT().CmdCapture("kitten", "@", "ls", "--match", "id:7").Return(windowLs(`"ksesh_conv":"abc","ksesh_settled_at":"1700000000"`), nil)
	mShell.EXPECT().CmdCapture("kitten", "@", "set-user-vars", "--match", "id:7",
		"ksesh_agent", "ksesh_status", "ksesh_status_at", "ksesh_msg", "ksesh_prompt", "ksesh_turn_at", "ksesh_conv",
		"ksesh_pin", "ksesh_snooze_until", "ksesh_snoozed_at", "ksesh_settled_at", "ksesh_unsettled", "ksesh_woke").Return("", nil)

	seen, err := k.UpdateAgent(0, agent.Update{Agent: "claude", Clear: true, Conversation: "abc"})

	require.NoError(t, err)
	assert.Equal(t, int64(1700000000), seen.Thread.SettledAt.Unix(), "what the thread history keeps of it")
}

func TestListAgentsReadsTheConversation(t *testing.T) {
	k, _ := newAgentKitty(t, "")
	st, err := parseState(windowLs(`"ksesh_agent":"claude","ksesh_status":"idle","ksesh_conv":"abc"`), 0)
	require.NoError(t, err)
	k.state = st

	agents, err := k.ListAgents()

	require.NoError(t, err)
	require.Len(t, agents, 1)
	assert.Equal(t, "abc", agents[0].Conversation)
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

func TestSendTextAndKeys(t *testing.T) {
	k, mShell := newAgentKitty(t, "")
	mShell.EXPECT().CmdCapture("kitten", "@", "send-text", "--match", "id:2", "--", `use C:\\tmp\r`).Return("", nil)
	mShell.EXPECT().CmdCapture("kitten", "@", "send-key", "--match", "id:2", "enter").Return("", nil)
	mShell.EXPECT().CmdCapture("kitten", "@", "get-text", "--ansi", "--match", "id:2").Return("screen", nil)

	require.NoError(t, k.SendText(2, `use C:\tmp`, true))
	require.NoError(t, k.SendKey(2, "enter"))
	screen, err := k.CaptureWindow(2)
	require.NoError(t, err)
	assert.Equal(t, "screen", screen)
}

func TestSetTitle(t *testing.T) {
	k, mShell := newAgentKitty(t, "")
	mShell.EXPECT().CmdCapture("kitten", "@", "set-user-vars", "--match", "id:2", "ksesh_title=Auth rework").Return("", nil).Once()
	mShell.EXPECT().CmdCapture("kitten", "@", "set-user-vars", "--match", "id:2", "ksesh_title").Return("", nil).Once()

	require.NoError(t, k.SetTitle(2, "Auth rework"))
	require.NoError(t, k.SetTitle(2, ""), "empty unsets it")

	a, ok := agentOf(lsWindow{ID: 2, Title: "claude", UserVars: map[string]string{StatusVar: "idle", TitleVar: "Auth rework", SeenAtVar: "1700000000"}})
	require.True(t, ok)
	assert.Equal(t, "Auth rework", a.CustomTitle)
	assert.Equal(t, int64(1700000000), a.SeenAt.Unix(), "when the watcher saw you at it")
}
