package seshcli

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xheisenbugx/ksesh/dir"
	"github.com/xheisenbugx/ksesh/execwrap"
	"github.com/xheisenbugx/ksesh/git"
	"github.com/xheisenbugx/ksesh/github"
	"github.com/xheisenbugx/ksesh/home"
	"github.com/xheisenbugx/ksesh/oswrap"
	"github.com/xheisenbugx/ksesh/shell"
)

// gitShell runs git for real, in the test's temp repos, and leaves every
// other command (kitten) to the mock.
type gitShell struct {
	shell.Shell
	real shell.Shell
}

func (s gitShell) Cmd(cmd string, args ...string) (string, error) {
	if cmd == "git" {
		return s.real.Cmd(cmd, args...)
	}
	return s.Shell.Cmd(cmd, args...)
}

func (s gitShell) CmdWithOutput(cmd string, args ...string) (string, error) {
	if cmd == "git" {
		return s.real.CmdWithOutput(cmd, args...)
	}
	return s.Shell.CmdWithOutput(cmd, args...)
}

// withRealGit has the commands run git, against repos the test makes.
func (c *cli) withRealGit(t *testing.T) *cli {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("needs git")
	}
	// Leave the user's git config out of it.
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, v := range []string{"GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME"} {
		t.Setenv(v, "ksesh test")
	}
	for _, v := range []string{"GIT_AUTHOR_EMAIL", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(v, "test@ksesh.invalid")
	}
	sh := gitShell{Shell: c.sh, real: shell.NewShell(execwrap.NewExec(), home.NewHome(oswrap.NewOs()))}
	c.base.Shell = sh
	c.base.Git = git.NewGit(sh)
	c.base.Github = github.NewGithub(sh, c.base.Git)
	c.base.Dir = dir.NewDir(c.base.Os, c.base.Git, c.base.Path)
	return c
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	require.NoError(t, err, string(out))
}

// TestWorktreeRemoveCarriesOnPastARefusal: git refusing one worktree fails
// the command, but only once the others are removed, branch and session
// with them.
func TestWorktreeRemoveCarriesOnPastARefusal(t *testing.T) {
	c := newCLI(t, "").withRealGit(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	repo := filepath.Join(root, "repo")
	require.NoError(t, os.MkdirAll(repo, 0o755))
	runGit(t, repo, "init", "-q", "-b", "main")
	runGit(t, repo, "commit", "-q", "--allow-empty", "-m", "first")
	done, dirty := filepath.Join(root, "done"), filepath.Join(root, "dirty")
	runGit(t, repo, "worktree", "add", "-q", "-b", "feat/done", done)
	runGit(t, repo, "worktree", "add", "-q", "-b", "feat/dirty", dirty)
	require.NoError(t, os.WriteFile(filepath.Join(dirty, "wip.txt"), []byte("wip"), 0o644))
	t.Chdir(repo)

	c.sh.EXPECT().CmdCapture("kitten", "@", "ls").Return(kittyLs(
		tab{id: 10, active: true, wins: []win{{id: 1, session: "repo", cwd: repo, active: true, vars: map[string]string{"ksesh_root": repo}}}},
		tab{id: 11, wins: []win{
			{id: 5, session: "done", cwd: done, vars: map[string]string{"ksesh_root": done}},
			{id: 6, session: "done", cwd: done, vars: map[string]string{"ksesh_root": done}},
		}},
	), nil)
	c.kitten("", "close-window", "--ignore-no-match", "--match", "id:5 or id:6")

	stdout, _, err := c.run("worktree", "rm", done, "dirty")

	assert.EqualError(t, err, "couldn't remove "+dirty+" (uncommitted changes? --force removes it anyway): exit status 128")
	assert.Equal(t, "removed "+done+"\n", stdout)
	assert.NoDirExists(t, done)
	assert.FileExists(t, filepath.Join(dirty, "wip.txt"), "what git refused is left alone")
	branches, err := exec.Command("git", "-C", repo, "branch", "--format=%(refname:short)").Output()
	require.NoError(t, err)
	assert.Equal(t, "feat/dirty\nmain\n", string(branches), "the removed worktree's branch goes with it")
}

func TestWorktreeRemoveNeedsAWorktree(t *testing.T) {
	t.Run("not in a session, and none named", func(t *testing.T) {
		c := newCLI(t, kittyLs(tab{id: 10, active: true, wins: []win{{id: 1, active: true}}}))

		_, _, err := c.run("worktree", "rm")

		assert.EqualError(t, err, "name a worktree, or run it from a worktree's session: not inside a kitty session")
	})

	t.Run("one that isn't a linked worktree of the repo", func(t *testing.T) {
		c := newCLI(t, "").withRealGit(t)
		repo, err := filepath.EvalSymlinks(t.TempDir())
		require.NoError(t, err)
		runGit(t, repo, "init", "-q", "-b", "main")
		runGit(t, repo, "commit", "-q", "--allow-empty", "-m", "first")
		t.Chdir(repo)

		_, _, err = c.run("worktree", "rm", "nope")

		assert.EqualError(t, err, "nope is not a linked worktree of "+repo)
	})
}
