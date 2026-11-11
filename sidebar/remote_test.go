package sidebar

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xheisenbugx/ksesh/kitty"
	"github.com/xheisenbugx/ksesh/model"
	"github.com/xheisenbugx/ksesh/remote"
)

func TestARemoteThreadsBranchIsAskedAboutOnItsHost(t *testing.T) {
	clock := now
	m, repos := repoModel(t, &clock)
	agents := inRepos(fixtures())[1:2]
	agents[0].Host = "devbox"
	repos.EXPECT().BranchStatus("devbox", "/w/2").Return(model.BranchStatus{Branch: "feat/far", Base: "origin/main", Ahead: 1}, nil).Once()
	repos.EXPECT().PullRequest("devbox", "/w/2", "feat/far", false).Return(model.PullRequestState{Number: 9, State: model.PROpen, URL: "u"}, true, nil).Once()

	m, cmd := update(m, loadedMsg{agents: agents, current: 1, shown: true, focused: true})
	m = answer(t, m, cmd)
	pr, ok := m.pullRequest(agents[0])
	require.True(t, ok)
	assert.Equal(t, 9, pr.Number)
	_, here := m.branches["/w/2"]
	assert.False(t, here, "kept apart from the same directory on this machine")
}

func TestANewThreadNextToARemoteOneRunsOnItsHost(t *testing.T) {
	m, b := newModel(t)
	a, _ := m.selectedAgent()
	a.Path, a.Host = "/home/me/c/api", "devbox"

	m, _ = press(t, m, "n")
	m = typeText(t, m, "deploy")
	b.EXPECT().NewWindowInSession(model.KittyTabOpts{TargetSession: "api", Background: true,
		Command: remote.Join(remote.Window("devbox", "/home/me/c/api")...),
		Vars:    map[string]string{kitty.HostVar: "devbox", kitty.CwdVar: "/home/me/c/api"},
	}).Return("window:42", nil).Once()
	b.EXPECT().WaitForPrompt("window:42", 20*time.Second).Return(nil).Once()
	b.EXPECT().SendKeys("window:42", "claude 'deploy'").Return("", nil).Once()
	b.EXPECT().FocusWindow(42).Return(nil).Once()
	_, cmd := press(t, m, "enter")
	require.NoError(t, cmd().(doneMsg).err)
}

func TestARemoteThreadCantBranchOrBeArchivedFromHere(t *testing.T) {
	m, _ := newModel(t)
	m.opts.Run = func(string, ...string) error { return nil }
	a, _ := m.selectedAgent()
	a.Path, a.Host = "/home/me/c/api", "devbox"
	m.branches[branchKey("devbox", a.Path)] = &branchInfo{ok: true, status: model.BranchStatus{Branch: "x", Worktree: "/home/me/wt/x"}}

	_, ok := m.archivable(a)
	assert.False(t, ok, "ksesh wt rm runs here")
	m, _ = press(t, m, "N")
	assert.Contains(t, m.flash, "worktrees on devbox aren't supported yet")
}
