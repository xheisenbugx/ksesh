package seshcli

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xheisenbugx/ksesh/cache"
	"github.com/xheisenbugx/ksesh/github"
	"github.com/xheisenbugx/ksesh/kitty"
	"github.com/xheisenbugx/ksesh/model"
	"github.com/xheisenbugx/ksesh/sidebar"
)

func TestCommandError(t *testing.T) {
	assert.Equal(t, "invalid branch name \"/\"", commandError("Error: invalid branch name \"/\"\nUsage:\n  ksesh worktree new <branch> [flags]\n"))
	assert.Equal(t, "couldn't remove /w (uncommitted changes?)", commandError("removed /a\ncouldn't remove /w (uncommitted changes?)\n"))
}

func TestSidebarReposSharePullRequestsThroughTheirCache(t *testing.T) {
	dir := t.TempDir()
	gh := github.NewMockGithub(t)
	gh.EXPECT().PrState("/w/login").Return(model.PullRequestState{Number: 12, State: model.PRMerged}, true, nil).Once()
	gh.EXPECT().PrState("/w/new").Return(model.PullRequestState{}, false, nil).Once()
	repos := func() sidebarRepos {
		return sidebarRepos{gh: gh, prs: cache.NewNamespaceInDir[model.PullRequestState](dir, prCacheName, prCacheVersion, prCacheTTL)}
	}

	pr, found, err := repos().PullRequest("/w/login", "feat/login")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, 12, pr.Number)
	_, found, _ = repos().PullRequest("/w/new", "new")
	assert.False(t, found)

	// Another sidebar: gh isn't asked again.
	pr, found, _ = repos().PullRequest("/w/login", "feat/login")
	assert.True(t, found)
	assert.Equal(t, model.PRMerged, pr.State)
	_, found, _ = repos().PullRequest("/w/new", "new")
	assert.False(t, found, "no pull request is remembered too")
}

func TestGotoFollowsTheSidebarsOrder(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ago := func(d time.Duration) *time.Time { at := time.Now().Add(-d); return &at }
	agents := []*model.Agent{
		{WindowID: 1, Session: "api", Status: "idle", Since: ago(time.Minute)},
		{WindowID: 2, Session: "web", Status: "done", Since: ago(2 * time.Minute)},
		{WindowID: 3, Session: "api", Status: "idle", Since: ago(3 * time.Minute)},
		{WindowID: 4, Session: "api", Status: "idle", Since: ago(time.Hour), Thread: model.Thread{Pin: 1}},
		{WindowID: 5, Session: "api", Status: "idle", Since: ago(time.Minute), Thread: model.Thread{SnoozedUntil: time.Now().Add(time.Hour)}},
	}
	k := kitty.NewMockKitty(t)
	k.EXPECT().ListAgents().Return(agents, nil)
	deps := &Deps{Kitty: k}

	k.EXPECT().FocusWindow(4).Return(nil).Once()
	require.NoError(t, gotoThread(deps, 1), "pinned first")
	k.EXPECT().FocusWindow(2).Return(nil).Once()
	require.NoError(t, gotoThread(deps, 3), "then the inbox, newest first")
	assert.EqualError(t, gotoThread(deps, 5), "no thread 5: the sidebar has 4", "the snoozed one is left out")

	path, err := sidebar.StatePath()
	require.NoError(t, err)
	_, err = sidebar.UpdateState(path, func(s *sidebar.State) { s.Group = true })
	require.NoError(t, err)
	k.EXPECT().FocusWindow(3).Return(nil).Once()
	require.NoError(t, gotoThread(deps, 3), "grouped by session, as the sidebar shows it")

	_, err = sidebar.UpdateState(path, func(s *sidebar.State) { s.Session = true })
	require.NoError(t, err)
	k.EXPECT().GetCurrentSession().Return("web", nil)
	k.EXPECT().FocusWindow(2).Return(nil).Once()
	require.NoError(t, gotoThread(deps, 1), "only this session's")
}

func TestRunKsesh(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, runKsesh([]string{"/bin/sh", "-c"})(dir, `test "$(pwd -P)" = "$(cd "$0" && pwd -P)"`, dir), "runs in dir")
	err := runKsesh([]string{"/bin/sh", "-c"})(dir, "echo 'Error: it broke' >&2; exit 1")
	assert.EqualError(t, err, "it broke")
}
