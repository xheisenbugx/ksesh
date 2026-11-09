package sidebar

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xheisenbugx/ksesh/agent"
	"github.com/xheisenbugx/ksesh/model"
)

// repoModel is the sidebar with Repos, its threads in /w/<id>, on a clock
// the test moves.
func repoModel(t *testing.T, clock *time.Time) (Model, *MockRepos) {
	t.Helper()
	repos := NewMockRepos(t)
	m := New(Options{
		Backend:    NewMockBackend(t),
		Self:       9,
		ThreadOpts: agent.ThreadOpts{AutoSettleAfter: agent.DefaultAutoSettleAfter},
		StatePath:  filepath.Join(t.TempDir(), "sidebar.json"),
		Now:        func() time.Time { return *clock },
		Repos:      repos,
	})
	m, _ = update(m, tea.WindowSizeMsg{Width: 40, Height: 26})
	return m, repos
}

func inRepos(agents []*model.Agent) []*model.Agent {
	for _, a := range agents {
		a.Path = "/w/" + string(rune('0'+a.WindowID))
	}
	return agents
}

// answer runs the commands a load started and feeds their answers back.
func answer(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		for _, c := range msg {
			m = answer(t, m, c)
		}
	case branchMsg:
		m, _ = update(m, msg)
	}
	return m
}

func TestBranchesAreAskedAboutOffTheLoopAndCached(t *testing.T) {
	clock := now
	m, repos := repoModel(t, &clock)
	agents := inRepos(fixtures())[1:2] // Write docs, done, in /w/2
	repos.EXPECT().BranchStatus("/w/2").Return(model.BranchStatus{Branch: "docs/threads", Base: "origin/main", Ahead: 2, Behind: 1, Worktree: "/w/2"}, nil).Once()
	repos.EXPECT().PullRequest("/w/2", "docs/threads").Return(model.PullRequestState{Number: 12, State: model.PROpen}, true, nil).Once()

	m, cmd := update(m, loadedMsg{agents: agents, current: 1, shown: true, focused: true})
	assert.NotContains(t, ansi.Strip(m.View().Content), "docs/threads", "nothing known until git answers")
	m = answer(t, m, cmd)
	view := ansi.Strip(m.View().Content)
	assert.Contains(t, view, "docs/threads ↑2↓1 "+iconPR+" #12", "the branch, its commits and its pull request")

	clock = clock.Add(10 * time.Second)
	_, cmd = update(m, loadedMsg{agents: agents, current: 1, shown: true, focused: true})
	m = answer(t, m, cmd) // nothing to ask: the mocks would fail on a second call

	clock = clock.Add(gitTTL)
	repos.EXPECT().BranchStatus("/w/2").Return(model.BranchStatus{Branch: "docs/threads", Base: "origin/main", Ahead: 3}, nil).Once()
	m, cmd = update(m, loadedMsg{agents: agents, current: 1, shown: true, focused: true})
	m = answer(t, m, cmd)
	assert.Contains(t, ansi.Strip(m.View().Content), "docs/threads ↑3 "+iconPR+" #12", "git again, the pull request not yet")
}

func TestHiddenSidebarsDontAsk(t *testing.T) {
	clock := now
	m, _ := repoModel(t, &clock)
	m, _ = update(m, loadedMsg{agents: inRepos(fixtures()), current: 1, shown: false})
	assert.Empty(t, m.refreshBranches())
}

func TestABranchWithoutAPullRequestOrOnTheBase(t *testing.T) {
	clock := now
	m, repos := repoModel(t, &clock)
	agents := inRepos(fixtures())[1:3]
	repos.EXPECT().BranchStatus("/w/2").Return(model.BranchStatus{Branch: "main"}, nil).Once()
	repos.EXPECT().BranchStatus("/w/3").Return(model.BranchStatus{Branch: "wip", Base: "origin/main"}, nil).Once()
	repos.EXPECT().PullRequest("/w/3", "wip").Return(model.PullRequestState{}, false, nil).Once()

	m, cmd := update(m, loadedMsg{agents: agents, current: 1, shown: true, focused: true})
	m = answer(t, m, cmd)
	view := ansi.Strip(m.View().Content)
	assert.NotContains(t, view, "main", "nothing to say about the base branch")
	assert.Contains(t, view, "wip", "a branch of its own, even without a pull request")
}

func TestTheBranchGivesWayToWhatTheAgentNeeds(t *testing.T) {
	m, _ := newModel(t)
	a := thread(7, "Fix it", "waiting", time.Minute, model.Thread{})
	a.Path = "/w/7"
	m.branches["/w/7"] = &branchInfo{ok: true, status: model.BranchStatus{Branch: "fix", Base: "origin/main"}, hasPR: true, pr: model.PullRequestState{Number: 3, State: model.PRMerged}}

	a.Message = "Allow Bash: npm test -- --watch=false"
	sub := ansi.Strip(m.subtitle(a, agent.Active, agent.BadgeOf(a, now)))
	assert.NotContains(t, sub, "fix", "the permission it asks for comes first")
	assert.Contains(t, sub, "Allow Bash")

	a.Message = "Allow Bash"
	sub = ansi.Strip(m.subtitle(a, agent.Active, agent.BadgeOf(a, now)))
	assert.Contains(t, sub, "api · Allow Bash", "short enough for both")
	assert.True(t, strings.HasSuffix(sub, "fix "+iconPRMerged+" #3"), "the branch at the right: %q", sub)
	assert.LessOrEqual(t, ansi.StringWidth(sub), m.width-5)

	a.Status, a.Message = "idle", "Rewrote the whole session store and its tests"
	sub = ansi.Strip(m.subtitle(a, agent.Active, agent.BadgeOf(a, now)))
	assert.Contains(t, sub, "fix "+iconPRMerged+" #3", "anything else is cut to make room")
	assert.Contains(t, sub, "…")
	assert.LessOrEqual(t, ansi.StringWidth(sub), m.width-5)
}

func TestAThreadWhosePullRequestWasMergedSettlesItself(t *testing.T) {
	clock := now
	m, repos := repoModel(t, &clock)
	b := m.opts.Backend.(*MockBackend)
	agents := inRepos(fixtures())
	merged := model.PullRequestState{Number: 12, State: model.PRMerged}
	for _, a := range agents {
		dir := a.Path
		repos.EXPECT().BranchStatus(dir).Return(model.BranchStatus{Branch: "b" + dir, Base: "origin/main"}, nil).Once()
		repos.EXPECT().PullRequest(dir, "b"+dir).Return(merged, true, nil).Once()
	}
	// Only the finished one: 1 waits on you, 3 is pinned, 4 works, 5 is
	// snoozed and 6 settled already, for inactivity.
	b.EXPECT().SetThread(2, model.Thread{SettledAt: now}).Return(nil).Once()

	m, cmd := update(m, loadedMsg{agents: agents, current: 1, shown: true, focused: true})
	var done []doneMsg
	var drain func(tea.Cmd)
	drain = func(c tea.Cmd) {
		if c == nil {
			return
		}
		switch msg := c().(type) {
		case tea.BatchMsg:
			for _, c := range msg {
				drain(c)
			}
		case branchMsg:
			next, more := update(m, msg)
			m = next
			drain(more)
		case doneMsg:
			done = append(done, msg)
		}
	}
	drain(cmd)
	require.Len(t, done, 1)
	assert.Equal(t, agent.Settled, agent.SectionOf(agents[1], now, m.opts.ThreadOpts), "shown settled at once")
	assert.Contains(t, done[0].flash, "its pull request was merged")

	// Taken back out by hand: it stays out.
	agents[1].Thread = model.Thread{Unsettled: true}
	assert.Empty(t, m.settleMerged())
}

func TestArchiveRemovesTheWorktreeThenClosesTheWindow(t *testing.T) {
	m, b := newModel(t)
	a, _ := m.selectedAgent()
	a.Path = "/w/login/src"
	var ran []string
	m.opts.Run = func(dir string, args ...string) error { ran = append([]string{dir}, args...); return nil }

	m, _ = press(t, m, "A")
	assert.Equal(t, modeNormal, m.mode, "nothing known of its worktree yet")
	assert.Contains(t, m.flash, "not in a worktree of its own")

	m.branches["/w/login/src"] = &branchInfo{ok: true, status: model.BranchStatus{Branch: "login", Base: "origin/main", Worktree: "/w/login"}}
	m, _ = update(m, tea.KeyPressMsg{Code: 'm', Text: "m"})
	assert.Contains(t, ansi.Strip(m.View().Content), "Archive worktree…", "in the menu")
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})

	m, _ = press(t, m, "A")
	require.Equal(t, modeConfirmArchive, m.mode)
	assert.Contains(t, ansi.Strip(m.View().Content), "Remove its worktree")
	b.EXPECT().CloseWindow(1).Return(nil).Once()
	_, cmd := press(t, m, "y")
	msg := cmd().(doneMsg)
	require.NoError(t, msg.err)
	assert.Equal(t, []string{"/w/login/src", "wt", "rm", "/w/login"}, ran)
	assert.Equal(t, "archived Fix the login, removed login", msg.flash)
}

func TestArchiveKeepsTheWindowWhenWtRmRefuses(t *testing.T) {
	m, _ := newModel(t)
	a, _ := m.selectedAgent()
	a.Path = "/w/login"
	m.branches["/w/login"] = &branchInfo{ok: true, status: model.BranchStatus{Branch: "login", Worktree: "/w/login"}}
	m.opts.Run = func(string, ...string) error {
		return errors.New("couldn't remove /w/login (uncommitted changes? --force removes it anyway): exit status 128")
	}
	m, _ = press(t, m, "A")
	m, cmd := press(t, m, "y")
	msg := cmd() // the mock would fail on CloseWindow
	m, _ = update(m, msg)
	footer := ansi.Strip(strings.Join(m.footer(), "\n"))
	assert.Contains(t, footer, "✗ couldn't remove", "what went wrong, in the status bar")
	assert.Contains(t, footer, "uncommitted", "on as many lines as it takes")
}

func TestPRIcons(t *testing.T) {
	icon, _ := prIcon(model.PullRequestState{State: model.PROpen, Draft: true})
	assert.Equal(t, iconPRDraft, icon)
	icon, _ = prIcon(model.PullRequestState{State: model.PRClosed})
	assert.Equal(t, iconPRClosed, icon)
}
