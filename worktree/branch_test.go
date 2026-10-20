package worktree

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/xheisenbugx/ksesh/connector"
	"github.com/xheisenbugx/ksesh/git"
	"github.com/xheisenbugx/ksesh/home"
	"github.com/xheisenbugx/ksesh/model"
	"github.com/xheisenbugx/ksesh/oswrap"
	"github.com/xheisenbugx/ksesh/pathwrap"
)

const porcelain = `worktree /code/app
HEAD 1111111111111111111111111111111111111111
branch refs/heads/main

worktree /code/app__worktrees/feat-login
HEAD 2222222222222222222222222222222222222222
branch refs/heads/feat/login

worktree /code/app__worktrees/old
HEAD 3333333333333333333333333333333333333333
branch refs/heads/old

worktree /code/app__worktrees/review
HEAD 4444444444444444444444444444444444444444
detached

worktree /code/bare.git
bare
`

func TestSlug(t *testing.T) {
	assert.Equal(t, "feat-login", Slug("feat/login"))
	assert.Equal(t, "fix-it", Slug(" fix it "))
	assert.Equal(t, "", Slug("/"))
}

func TestParseWorktreeList(t *testing.T) {
	worktrees := ParseWorktreeList(porcelain)
	require.Len(t, worktrees, 4, "the bare entry is left out")
	assert.Equal(t, "main", worktrees[0].Branch)
	assert.Equal(t, "feat/login", worktrees[1].Branch)
	assert.Equal(t, "/code/app__worktrees/feat-login", worktrees[1].Path)
	assert.True(t, worktrees[3].Detached)
	assert.Equal(t, "", worktrees[3].Branch)
}

func TestJoinCommands(t *testing.T) {
	assert.Equal(t, "npm i; claude 'go'", joinCommands(" npm i ", "", "claude 'go'"))
	assert.Equal(t, "", joinCommands("", " "))
	assert.Equal(t, `'it'"'"'s'`, shellQuote("it's"))
}

type branchMocks struct {
	git  *git.MockGit
	conn *connector.MockConnector
	os   *oswrap.MockOs
}

func newBranchWorktree(t *testing.T, config model.Config) (*RealWorktree, branchMocks) {
	m := branchMocks{git.NewMockGit(t), connector.NewMockConnector(t), oswrap.NewMockOs(t)}
	m.os.EXPECT().UserHomeDir().Return("/home/me", nil).Maybe()
	m.os.EXPECT().ExpandEnv(mock.Anything).RunAndReturn(func(s string) string { return s }).Maybe()
	m.os.EXPECT().Getwd().Return("/code/app/src", nil).Maybe()
	m.git.EXPECT().WorktreeList("/code/app/src").Return(true, porcelain, nil).Maybe()
	m.git.EXPECT().WorktreeList("/code/app").Return(true, porcelain, nil).Maybe()
	w := NewWorktree(config, m.git, nil, m.conn, nil, home.NewHome(m.os), m.os, pathwrap.NewPath(), nil).(*RealWorktree)
	return w, m
}

func TestNewCreatesABranchFromTheDefaultBranchNextToTheRepo(t *testing.T) {
	w, m := newBranchWorktree(t, model.Config{})
	target := "/code/app__worktrees/feat-new"
	m.os.EXPECT().Stat(target).Return(nil, os.ErrNotExist)
	m.os.EXPECT().MkdirAll("/code/app__worktrees", mock.Anything).Return(nil)
	m.git.EXPECT().RefExists("/code/app", "refs/heads/feat/new").Return(false)
	m.git.EXPECT().RefExists("/code/app", "refs/remotes/origin/feat/new").Return(false)
	m.git.EXPECT().DefaultBranch("/code/app").Return("origin/main", true)
	m.git.EXPECT().WorktreeAdd("/code/app", target, "feat/new", "origin/main").Return("", nil)
	m.conn.EXPECT().Connect(target, model.ConnectOpts{Command: "claude 'add a page'"}).Return("", nil)

	_, err := w.New(model.WorktreeNewOpts{Branch: "feat/new", Prompt: "add a page"})
	require.NoError(t, err)
}

func TestNewChecksOutARemoteBranchUnderTheConfiguredRoot(t *testing.T) {
	w, m := newBranchWorktree(t, model.Config{WorktreeConfigs: []model.WorktreeConfig{{
		Path: "/code/app", WorktreeDir: ".wk", Fetch: falsePtr(),
		CreateCommand: "npm i", AgentCommand: "codex",
	}}})
	target := "/code/app/.wk/feat-login"
	m.os.EXPECT().Stat(target).Return(nil, os.ErrNotExist)
	m.os.EXPECT().MkdirAll("/code/app/.wk", mock.Anything).Return(nil)
	m.git.EXPECT().RefExists("/code/app", "refs/heads/feat/login").Return(false)
	m.git.EXPECT().RefExists("/code/app", "refs/remotes/origin/feat/login").Return(true)
	m.git.EXPECT().WorktreeAddTracking("/code/app", target, "feat/login", "origin/feat/login").Return("", nil)
	m.conn.EXPECT().Connect(target, model.ConnectOpts{Switch: true, Command: "npm i; codex"}).Return("", nil)

	_, err := w.New(model.WorktreeNewOpts{Branch: "feat/login", StartAgent: true, Switch: true})
	require.NoError(t, err)
}

func TestNewReusesAnExistingWorktree(t *testing.T) {
	w, m := newBranchWorktree(t, model.Config{})
	target := "/code/app__worktrees/feat-login"
	m.os.EXPECT().Stat(target).Return(nil, nil)
	m.conn.EXPECT().Connect(target, model.ConnectOpts{}).Return("", nil)

	_, err := w.New(model.WorktreeNewOpts{Branch: "feat/login"})
	require.NoError(t, err)
}

func TestRemoveCandidates(t *testing.T) {
	w, m := newBranchWorktree(t, model.Config{})
	m.git.EXPECT().FetchPrune("/code/app").Return("", nil)
	m.git.EXPECT().GoneBranches("/code/app").Return([]string{"old"}, nil)
	m.git.EXPECT().DefaultBranch("/code/app").Return("origin/main", true)
	m.git.EXPECT().MergedBranches("/code/app", "origin/main").Return([]string{"main", "old", "feat/login"}, nil)
	m.git.EXPECT().RevParse("/code/app", "origin/main").Return(true, "2222222222222222222222222222222222222222", nil)

	picked, err := w.RemoveCandidates(model.WorktreeRemoveOpts{Merged: true, Gone: true})

	require.NoError(t, err)
	require.Len(t, picked, 1, "main is never removed, and feat/login has no commits of its own yet")
	assert.Equal(t, "old", picked[0].Branch)
	assert.Equal(t, "gone", picked[0].Reason)
	assert.Equal(t, "/code/app", picked[0].Repo)
}

func TestRemoveCandidatesByName(t *testing.T) {
	w, _ := newBranchWorktree(t, model.Config{})

	picked, err := w.RemoveCandidates(model.WorktreeRemoveOpts{Targets: []string{"feat/login", "/code/app__worktrees/old/sub", "review"}})
	require.NoError(t, err)
	var paths []string
	for _, wt := range picked {
		paths = append(paths, wt.Path)
	}
	assert.Equal(t, []string{"/code/app__worktrees/feat-login", "/code/app__worktrees/old", "/code/app__worktrees/review"}, paths)

	_, err = w.RemoveCandidates(model.WorktreeRemoveOpts{Targets: []string{"main"}})
	assert.ErrorContains(t, err, "not a linked worktree")
}

func TestRemoveDeletesAGoneBranchEvenUnmerged(t *testing.T) {
	w, m := newBranchWorktree(t, model.Config{})
	wt := model.GitWorktree{Path: "/code/app__worktrees/old", Branch: "old", Repo: "/code/app", Reason: "gone"}
	m.git.EXPECT().WorktreeRemove("/code/app", wt.Path, false).Return("", nil)
	m.git.EXPECT().DeleteBranch("/code/app", "old", true).Return("", nil)

	require.NoError(t, w.Remove(wt, false, false))
}

func TestRemoveKeepsTheBranch(t *testing.T) {
	w, m := newBranchWorktree(t, model.Config{})
	wt := model.GitWorktree{Path: "/code/app__worktrees/old", Branch: "old", Repo: "/code/app"}
	m.git.EXPECT().WorktreeRemove("/code/app", wt.Path, true).Return("", nil)

	require.NoError(t, w.Remove(wt, true, true))
	assert.Error(t, w.Remove(model.GitWorktree{Path: "/code/app", Main: true, Repo: "/code/app"}, false, false))
}

func TestPopulateCopiesAndLinksWithoutOverwriting(t *testing.T) {
	main, target := t.TempDir(), t.TempDir()
	write := func(path, content string) {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
	write(filepath.Join(main, IncludeFile), "# secrets\n.env\nconfig/*.local\n!ignored\n../outside\n")
	write(filepath.Join(main, ".env"), "SECRET=1")
	write(filepath.Join(main, "config", "db.local"), "db")
	write(filepath.Join(main, "config", "db.yml"), "tracked")
	write(filepath.Join(main, "tools", "bin", "x"), "x")
	write(filepath.Join(main, "node_modules", "pkg", "index.js"), "js")
	write(filepath.Join(target, "config", "db.local"), "branch's own")

	w := &RealWorktree{}
	repo := repoContext{main: main, cfg: model.WorktreeConfig{Copy: []string{"tools/"}, Symlink: []string{"node_modules"}}}
	require.NoError(t, w.populate(repo, target))

	read := func(rel string) string {
		data, err := os.ReadFile(filepath.Join(target, rel))
		require.NoError(t, err, rel)
		return string(data)
	}
	assert.Equal(t, "SECRET=1", read(".env"))
	assert.Equal(t, "branch's own", read("config/db.local"), "nothing is overwritten")
	assert.NoFileExists(t, filepath.Join(target, "config", "db.yml"))
	assert.Equal(t, "x", read("tools/bin/x"), "directories are copied whole")
	link, err := os.Readlink(filepath.Join(target, "node_modules"))
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(main, "node_modules"), link)
}
