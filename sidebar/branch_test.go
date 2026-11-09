package sidebar

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"

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

func TestPRIcons(t *testing.T) {
	icon, _ := prIcon(model.PullRequestState{State: model.PROpen, Draft: true})
	assert.Equal(t, iconPRDraft, icon)
	icon, _ = prIcon(model.PullRequestState{State: model.PRClosed})
	assert.Equal(t, iconPRClosed, icon)
}
