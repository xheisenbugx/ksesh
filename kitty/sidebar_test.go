package kitty

import (
	"strconv"
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

// sidebarLs is a session's tab with an agent, a shell and a docked sidebar
// that kitty added to the session, and another tab without a sidebar.
const sidebarLs = `[{"id":1,"is_active":true,"is_focused":true,"tabs":[
	{"id":10,"is_active":true,"title":"code","windows":[
		{"id":1,"session_name":"api","last_focused_at":50,"cwd":"/c/api","title":"✳ Fix the login","created_at":1700000000000000000,
		 "user_vars":{"ksesh_agent":"claude","ksesh_status":"idle","ksesh_status_at":"1700000000",
		              "ksesh_pin":"5","ksesh_snooze_until":"1700003600","ksesh_snoozed_at":"1700000000","ksesh_woke":"1"}},
		{"id":2,"session_name":"api","last_focused_at":40,"cwd":"/c/api"},
		{"id":9,"is_active":true,"session_name":"api","last_focused_at":60,"cwd":"/",
		 "user_vars":{"ksesh_role":"sidebar"},"foreground_processes":[{"cmdline":["claude"],"cwd":"/"}]}
	]},
	{"id":11,"title":"web","windows":[
		{"id":3,"session_name":"web","last_focused_at":30,"cwd":"/c/web"}
	]}
]}]`

func newSidebarKitty(t *testing.T, selfID string) (*RealKitty, *shell.MockShell) {
	t.Helper()
	mOs := oswrap.NewMockOs(t)
	mOs.EXPECT().Getenv("KITTY_WINDOW_ID").Return(selfID).Maybe()
	mOs.EXPECT().Getenv("KITTY_LISTEN_ON").Return("").Maybe()
	mOs.EXPECT().Getenv("TMPDIR").Return("").Maybe()
	mShell := shell.NewMockShell(t)
	mShell.EXPECT().CmdCapture("kitten", "@", "ls").Return(sidebarLs, nil).Maybe()
	return NewKitty(mOs, mShell, model.Config{}).(*RealKitty), mShell
}

func TestListAgentsReadsThreads(t *testing.T) {
	k, _ := newSidebarKitty(t, "")

	agents, err := k.ListAgents()

	require.NoError(t, err)
	require.Len(t, agents, 1, "the sidebar is never an agent, whatever it runs")
	a := agents[0]
	assert.Equal(t, "✳ Fix the login", a.Title)
	assert.Equal(t, time.Unix(1700000000, 0), a.Created)
	assert.Equal(t, model.Thread{
		Pin:          5,
		SnoozedUntil: time.Unix(1700003600, 0),
		SnoozedAt:    time.Unix(1700000000, 0),
		Woke:         true,
	}, a.Thread)
}

func TestSidebarIsNotPartOfTheSession(t *testing.T) {
	k, _ := newSidebarKitty(t, "9")

	tabs, err := k.ListAllWindows()
	require.NoError(t, err)
	assert.Len(t, tabs["api"], 1)

	session, err := k.GetCurrentSession()
	require.NoError(t, err)
	assert.Equal(t, "api", session, "the sidebar is in the session of its tab")

	windows, err := k.ListKittyWindows()
	require.NoError(t, err)
	for _, w := range windows {
		assert.NotEqual(t, 9, w.ID)
	}
}

func TestThreadAssignmentsRoundTrip(t *testing.T) {
	th := model.Thread{Pin: 7, SnoozedUntil: time.Unix(200, 0), SnoozedAt: time.Unix(100, 0), SettledAt: time.Unix(50, 0), Unsettled: true, Woke: true}
	vars := map[string]string{}
	for _, a := range threadAssignments(th) {
		name, value, _ := strings.Cut(a, "=")
		vars[name] = value
	}
	assert.Equal(t, th, threadOf(vars))
	assert.Equal(t, []string{"ksesh_pin", "ksesh_snooze_until", "ksesh_snoozed_at", "ksesh_settled_at", "ksesh_unsettled", "ksesh_woke"},
		threadAssignments(model.Thread{}), "an empty thread unsets every variable")
}

func TestSetThread(t *testing.T) {
	k, mShell := newSidebarKitty(t, "")
	mShell.EXPECT().CmdCapture("kitten", "@", "set-user-vars", "--match", "id:1",
		"ksesh_pin=3", "ksesh_snooze_until", "ksesh_snoozed_at", "ksesh_settled_at", "ksesh_unsettled", "ksesh_woke").Return("", nil)

	require.NoError(t, k.SetThread(1, model.Thread{Pin: 3}))
}

func TestWaitingWakesASnoozedThread(t *testing.T) {
	k, mShell := newSidebarKitty(t, "7")
	until := time.Now().Add(time.Hour).Unix()
	mShell.EXPECT().CmdCapture("kitten", "@", "ls", "--match", "id:7").
		Return(windowLs(`"ksesh_pin":"4","ksesh_snooze_until":"`+strconv.FormatInt(until, 10)+`","ksesh_snoozed_at":"1"`), nil)
	statusAt := mock.MatchedBy(func(s string) bool { return strings.HasPrefix(s, "ksesh_status_at=") })
	mShell.EXPECT().CmdCapture("kitten", "@", "set-user-vars", "--match", "id:7",
		"ksesh_pin=4", "ksesh_snooze_until", "ksesh_snoozed_at", "ksesh_settled_at", "ksesh_unsettled", "ksesh_woke=1",
		"ksesh_msg=Allow Bash", statusAt, "ksesh_status=waiting").Return("", nil)

	require.NoError(t, k.SetAgentStatus(0, agent.Update{Status: agent.Waiting, Message: "Allow Bash", SetMessage: true}))
}

func TestFinishingLeavesTheThreadAlone(t *testing.T) {
	k, mShell := newSidebarKitty(t, "7")
	statusAt := mock.MatchedBy(func(s string) bool { return strings.HasPrefix(s, "ksesh_status_at=") })
	// No ls: finishing can't move a thread, so it isn't read.
	mShell.EXPECT().CmdCapture("kitten", "@", "set-user-vars", "--match", "id:7",
		"ksesh_msg=ok", statusAt, "ksesh_status=done").Return("", nil)

	require.NoError(t, k.SetAgentStatus(0, agent.Update{Status: agent.Done, Message: "ok", SetMessage: true}))
}

func TestSidebarTab(t *testing.T) {
	k, _ := newSidebarKitty(t, "")

	id, err := k.SidebarTab(2)
	require.NoError(t, err)
	assert.Equal(t, 9, id)

	id, err = k.SidebarTab(3)
	require.NoError(t, err)
	assert.Equal(t, 0, id, "the web tab has none")
}

func TestOpenSidebar(t *testing.T) {
	k, mShell := newSidebarKitty(t, "")
	mShell.EXPECT().CmdCapture("kitten", "@", "launch", "--match", "window_id:3",
		"--type=tab-dock", "--dock-edge=left", "--dock-size=38", "--var=ksesh_role=sidebar", "--keep-focus", "--title=threads",
		"--", "/bin/ksesh", "sidebar").Return("12\n", nil)

	id, err := k.OpenSidebar(3, SidebarOpts{Command: []string{"/bin/ksesh", "sidebar"}})

	require.NoError(t, err)
	assert.Equal(t, 12, id)
}

func TestCloseSidebars(t *testing.T) {
	k, mShell := newSidebarKitty(t, "")
	mShell.EXPECT().CmdCapture("kitten", "@", "close-window", "--ignore-no-match", "--match", "id:9").Return("", nil)

	require.NoError(t, k.CloseSidebars())
}

func TestLastWorkWindowSkipsTheSidebar(t *testing.T) {
	k, _ := newSidebarKitty(t, "")

	id, err := k.LastWorkWindow(9)

	require.NoError(t, err)
	assert.Equal(t, 1, id, "the sidebar was focused last, but it is where you come from")
}

func TestWindowState(t *testing.T) {
	k, _ := newSidebarKitty(t, "")

	shown, focused, err := k.WindowState(9)
	require.NoError(t, err)
	assert.True(t, shown)
	assert.False(t, focused)

	shown, _, err = k.WindowState(3)
	require.NoError(t, err)
	assert.False(t, shown)
}

func TestOverlayOnCoversTheWindowInItsDirectory(t *testing.T) {
	k, mShell := newSidebarKitty(t, "")
	mShell.EXPECT().CmdCapture("kitten", "@", "launch", "--match", "id:1", "--next-to", "id:1", "--type=overlay",
		"--cwd=/c/api", "--env", "PATH=/bin", "--", "picker", "-a").Return("", nil).Once()

	require.NoError(t, k.OverlayOn(1, []string{"PATH=/bin"}, []string{"picker", "-a"}))
}

func TestOverlayOnFollowsTheProgramsDirectory(t *testing.T) {
	k, mShell := newSidebarKitty(t, "")
	mShell.EXPECT().CmdCapture("kitten", "@", "launch", "--match", "id:9", "--next-to", "id:9", "--type=overlay",
		"--cwd=/", "--", "picker").Return("", nil).Once()

	require.NoError(t, k.OverlayOn(9, nil, []string{"picker"}))
}
