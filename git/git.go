package git

import (
	"fmt"
	"strings"

	"github.com/xheisenbugx/ksesh/shell"
)

type StatusSummary struct {
	Staged    int
	Unstaged  int
	Untracked int
	Deleted   int
}

type Git interface {
	ShowTopLevel(name string) (bool, string, error)
	GitCommonDir(name string) (bool, string, error)
	Clone(url string, cmdDir string, dir string, gitFlags ...string) (string, error)
	WorktreeList(name string) (bool, string, error)
	Fetch(repoPath string) (string, error)
	WorktreeAdd(repoPath, target, branch, base string) (string, error)
	WorktreeAddDetached(repoPath, target, base string) (string, error)
	Pull(repoPath string) (string, error)
	CurrentBranch(path string) (bool, string, error)
	StatusSummary(path string) (StatusSummary, error)
	// WorktreeAddBranch checks an existing local branch out in a new worktree.
	WorktreeAddBranch(repoPath, target, branch string) (string, error)
	// WorktreeAddTracking creates branch from remoteRef, tracking it, in a
	// new worktree.
	WorktreeAddTracking(repoPath, target, branch, remoteRef string) (string, error)
	WorktreeRemove(repoPath, target string, force bool) (string, error)
	DeleteBranch(repoPath, branch string, force bool) (string, error)
	// RefExists reports whether ref (refs/heads/x, refs/remotes/origin/x)
	// resolves to a commit.
	RefExists(repoPath, ref string) bool
	// DefaultBranch is the remote's default branch, e.g. "origin/main", read
	// from refs/remotes/origin/HEAD.
	DefaultBranch(repoPath string) (string, bool)
	// MergedBranches lists the local branches merged into base.
	MergedBranches(repoPath, base string) ([]string, error)
	// GoneBranches lists the local branches whose upstream was deleted, as a
	// merged pull request's branch is.
	GoneBranches(repoPath string) ([]string, error)
	// AheadBehind counts the commits path's HEAD has that base doesn't, and
	// the other way round.
	AheadBehind(path, base string) (int, int, error)
	FetchPrune(repoPath string) (string, error)
	// RevParse resolves a ref to its commit.
	RevParse(repoPath, ref string) (bool, string, error)
}

type RealGit struct {
	shell shell.Shell
}

func NewGit(shell shell.Shell) Git {
	return &RealGit{shell}
}

func (g *RealGit) ShowTopLevel(path string) (bool, string, error) {
	out, err := g.shell.Cmd("git", "-C", path, "rev-parse", "--show-toplevel")
	if err != nil {
		return false, "", err
	}
	return true, out, nil
}

func (g *RealGit) GitCommonDir(path string) (bool, string, error) {
	out, err := g.shell.Cmd("git", "-C", path, "rev-parse", "--git-common-dir")
	if err != nil {
		return false, "", err
	}
	return true, out, nil
}

func (g *RealGit) Clone(url string, cmdDir string, dir string, gitFlags ...string) (string, error) {
	args := []string{}

	if cmdDir != "" {
		args = append(args, "-C", cmdDir)
	}

	args = append(args, "clone")
	args = append(args, gitFlags...)
	args = append(args, url)

	if dir != "" {
		args = append(args, dir)
	}

	_, err := g.shell.CmdWithOutput("git", args...)
	if err != nil {
		return "", err
	}
	return "", nil
}

func (g *RealGit) CurrentBranch(path string) (bool, string, error) {
	out, err := g.shell.Cmd("git", "-C", path, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return false, "", err
	}
	return true, out, nil
}

func (g *RealGit) WorktreeList(path string) (bool, string, error) {
	out, err := g.shell.Cmd("git", "-C", path, "worktree", "list", "--porcelain")
	if err != nil {
		return false, "", err
	}
	return true, out, nil
}

func (g *RealGit) Fetch(repoPath string) (string, error) {
	return g.shell.CmdWithOutput("git", "-C", repoPath, "fetch")
}

func (g *RealGit) WorktreeAdd(repoPath, target, branch, base string) (string, error) {
	return g.shell.CmdWithOutput("git", "-C", repoPath, "worktree", "add", target, "-b", branch, "--no-track", base)
}

func (g *RealGit) WorktreeAddDetached(repoPath, target, base string) (string, error) {
	return g.shell.CmdWithOutput("git", "-C", repoPath, "worktree", "add", "--detach", target, base)
}

func (g *RealGit) Pull(repoPath string) (string, error) {
	return g.shell.CmdWithOutput("git", "-C", repoPath, "pull", "--ff-only")
}

func (g *RealGit) StatusSummary(path string) (StatusSummary, error) {
	out, err := g.shell.Cmd("git", "-C", path, "status", "--porcelain")
	if err != nil {
		return StatusSummary{}, err
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
		return StatusSummary{}, nil
	}
	var s StatusSummary
	for _, line := range lines {
		if strings.HasPrefix(line, "?? ") {
			s.Untracked++
			continue
		}
		first := line[0]
		second := line[1]
		if first != ' ' {
			s.Staged++
		}
		if second == 'M' {
			s.Unstaged++
		}
		if first == 'D' || second == 'D' {
			s.Deleted++
		}
	}
	return s, nil
}

func (g *RealGit) WorktreeAddBranch(repoPath, target, branch string) (string, error) {
	return g.shell.CmdWithOutput("git", "-C", repoPath, "worktree", "add", target, branch)
}

func (g *RealGit) WorktreeAddTracking(repoPath, target, branch, remoteRef string) (string, error) {
	return g.shell.CmdWithOutput("git", "-C", repoPath, "worktree", "add", "--track", "-b", branch, target, remoteRef)
}

func (g *RealGit) WorktreeRemove(repoPath, target string, force bool) (string, error) {
	args := []string{"-C", repoPath, "worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	return g.shell.CmdWithOutput("git", append(args, target)...)
}

func (g *RealGit) DeleteBranch(repoPath, branch string, force bool) (string, error) {
	flag := "-d"
	if force {
		flag = "-D"
	}
	return g.shell.CmdWithOutput("git", "-C", repoPath, "branch", flag, branch)
}

func (g *RealGit) RefExists(repoPath, ref string) bool {
	_, err := g.shell.Cmd("git", "-C", repoPath, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	return err == nil
}

func (g *RealGit) DefaultBranch(repoPath string) (string, bool) {
	out, err := g.shell.Cmd("git", "-C", repoPath, "symbolic-ref", "--short", "refs/remotes/origin/HEAD")
	if err != nil || strings.TrimSpace(out) == "" {
		return "", false
	}
	return strings.TrimSpace(out), true
}

func (g *RealGit) MergedBranches(repoPath, base string) ([]string, error) {
	out, err := g.shell.Cmd("git", "-C", repoPath, "branch", "--format=%(refname:short)", "--merged", base)
	if err != nil {
		return nil, err
	}
	return nonEmptyLines(out), nil
}

func (g *RealGit) GoneBranches(repoPath string) ([]string, error) {
	out, err := g.shell.Cmd("git", "-C", repoPath, "for-each-ref", "--format=%(refname:short)\t%(upstream:track)", "refs/heads")
	if err != nil {
		return nil, err
	}
	var gone []string
	for _, line := range nonEmptyLines(out) {
		branch, track, _ := strings.Cut(line, "\t")
		if track == "[gone]" {
			gone = append(gone, branch)
		}
	}
	return gone, nil
}

func (g *RealGit) AheadBehind(path, base string) (int, int, error) {
	out, err := g.shell.Cmd("git", "-C", path, "rev-list", "--left-right", "--count", "HEAD..."+base)
	if err != nil {
		return 0, 0, err
	}
	var ahead, behind int
	if _, err := fmt.Sscanf(strings.TrimSpace(out), "%d %d", &ahead, &behind); err != nil {
		return 0, 0, err
	}
	return ahead, behind, nil
}

func (g *RealGit) FetchPrune(repoPath string) (string, error) {
	return g.shell.Cmd("git", "-C", repoPath, "fetch", "--prune", "--quiet")
}

func nonEmptyLines(out string) []string {
	var lines []string
	for _, line := range strings.Split(out, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func (g *RealGit) RevParse(repoPath, ref string) (bool, string, error) {
	out, err := g.shell.Cmd("git", "-C", repoPath, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	if err != nil {
		return false, "", err
	}
	return true, strings.TrimSpace(out), nil
}
