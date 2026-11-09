package seshcli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xheisenbugx/ksesh/cache"
	"github.com/xheisenbugx/ksesh/github"
	"github.com/xheisenbugx/ksesh/model"
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

func TestRunKsesh(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, runKsesh([]string{"/bin/sh", "-c"})(dir, `test "$(pwd -P)" = "$(cd "$0" && pwd -P)"`, dir), "runs in dir")
	err := runKsesh([]string{"/bin/sh", "-c"})(dir, "echo 'Error: it broke' >&2; exit 1")
	assert.EqualError(t, err, "it broke")
}
