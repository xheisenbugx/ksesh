package worktree

import (
	"cmp"
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"

	"github.com/xheisenbugx/ksesh/model"
)

// Branch worktrees: `ksesh worktree new <branch>` and friends, for any git
// repo, with or without a [[worktree]] block. The block for the repo, when
// there is one, supplies the worktree root, base branch, files to copy and the
// commands to run; without one, worktrees go next to the repo in
// "<repo>__worktrees/<branch>".

// DefaultAgentCommand is the agent `worktree new --agent` starts when neither
// the flag nor agent_command name one.
const DefaultAgentCommand = "claude"

// Slug turns a branch name into a directory name: "feat/login" -> "feat-login".
func Slug(branch string) string {
	slug := strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ' ', ':':
			return '-'
		}
		return r
	}, strings.TrimSpace(branch))
	return strings.Trim(slug, "-.")
}

// repoContext is the repo a branch worktree command runs against.
type repoContext struct {
	main string               // the main worktree
	cfg  model.WorktreeConfig // its [[worktree]] block, zero when none
	has  bool                 // whether there is one
}

func (w *RealWorktree) repoAt(dir string) (repoContext, error) {
	main, err := w.mainWorktree(dir)
	if err != nil {
		return repoContext{}, err
	}
	ctx := repoContext{main: main}
	for _, c := range w.config.WorktreeConfigs {
		expanded, err := w.home.ExpandPath(c.Path)
		if err != nil {
			continue
		}
		if w.sameDir(expanded, main) {
			ctx.cfg, ctx.has = c, true
			break
		}
	}
	return ctx, nil
}

// root is where the repo's branch worktrees go.
func (r repoContext) root() string {
	if r.has {
		return r.cfg.Root(r.main)
	}
	return filepath.Join(filepath.Dir(r.main), filepath.Base(r.main)+"__worktrees")
}

// base is the ref new branches start from and worktrees are compared to.
func (w *RealWorktree) base(r repoContext, override string) string {
	if override != "" {
		return override
	}
	if r.cfg.BaseBranch != "" {
		return r.cfg.BaseBranch
	}
	if def, ok := w.git.DefaultBranch(r.main); ok {
		return def
	}
	if ok, branch, err := w.git.CurrentBranch(r.main); err == nil && ok && branch != "HEAD" {
		return branch
	}
	return "HEAD"
}

func (w *RealWorktree) New(opts model.WorktreeNewOpts) (string, error) {
	if Slug(opts.Branch) == "" {
		return "", fmt.Errorf("invalid branch name %q", opts.Branch)
	}
	cwd, err := w.os.Getwd()
	if err != nil {
		return "", err
	}
	repo, err := w.repoAt(cwd)
	if err != nil {
		return "", err
	}
	target := filepath.Join(repo.root(), Slug(opts.Branch))

	created := false
	if _, err := w.os.Stat(target); err != nil {
		if (repo.has && fetchEnabled(repo.cfg)) || opts.Fetch {
			w.git.Fetch(repo.main) // best-effort; a failed fetch shouldn't block
		}
		if err := w.os.MkdirAll(repo.root(), 0o755); err != nil {
			return "", err
		}
		switch {
		case w.git.RefExists(repo.main, "refs/heads/"+opts.Branch):
			_, err = w.git.WorktreeAddBranch(repo.main, target, opts.Branch)
		case w.git.RefExists(repo.main, "refs/remotes/origin/"+opts.Branch):
			_, err = w.git.WorktreeAddTracking(repo.main, target, opts.Branch, "origin/"+opts.Branch)
		default:
			_, err = w.git.WorktreeAdd(repo.main, target, opts.Branch, w.base(repo, opts.Base))
		}
		if err != nil {
			return "", fmt.Errorf("couldn't create the worktree for %s: %w", opts.Branch, err)
		}
		created = true
		if err := w.populate(repo, target); err != nil {
			// The worktree is there and usable; a missing .env is for the
			// user to sort out, not a reason to leave them without a session.
			slog.Warn("worktree new: couldn't copy files into the worktree", "error", err)
		}
	}

	agent := ""
	if opts.StartAgent || opts.Prompt != "" {
		agent = cmp.Or(opts.Agent, repo.cfg.AgentCommand, DefaultAgentCommand)
	}
	if agent != "" && opts.Prompt != "" {
		agent += " " + shellQuote(opts.Prompt)
	}
	return w.connector.Connect(target, model.ConnectOpts{
		Switch:  opts.Switch,
		Command: joinCommands(connectCommand(repo.cfg, created), agent),
	})
}

// joinCommands runs commands one after the other, the next one whether or not
// the previous one failed: a failed install shouldn't keep the agent from
// starting.
func joinCommands(commands ...string) string {
	var parts []string
	for _, c := range commands {
		if c = strings.TrimSpace(c); c != "" {
			parts = append(parts, c)
		}
	}
	return strings.Join(parts, "; ")
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

// Worktrees lists the repo's worktrees, the main one first, with their state
// when withStatus is set.
func (w *RealWorktree) Worktrees(withStatus bool) ([]model.GitWorktree, error) {
	cwd, err := w.os.Getwd()
	if err != nil {
		return nil, err
	}
	repo, err := w.repoAt(cwd)
	if err != nil {
		return nil, err
	}
	worktrees, err := w.list(repo)
	if err != nil || !withStatus {
		return worktrees, err
	}
	base := w.base(repo, "")
	for i := range worktrees {
		wt := &worktrees[i]
		if s, err := w.git.StatusSummary(wt.Path); err == nil {
			wt.Staged, wt.Unstaged, wt.Untracked = s.Staged, s.Unstaged, s.Untracked
		}
		if !wt.Main || wt.Branch != strings.TrimPrefix(base, "origin/") {
			if ahead, behind, err := w.git.AheadBehind(wt.Path, base); err == nil {
				wt.Base, wt.Ahead, wt.Behind = base, ahead, behind
			}
		}
	}
	return worktrees, nil
}

func (w *RealWorktree) list(repo repoContext) ([]model.GitWorktree, error) {
	ok, out, err := w.git.WorktreeList(repo.main)
	if err != nil || !ok {
		return nil, fmt.Errorf("couldn't list the worktrees of %s: %v", repo.main, err)
	}
	worktrees := ParseWorktreeList(out)
	for i := range worktrees {
		worktrees[i].Main = w.sameDir(worktrees[i].Path, repo.main)
		worktrees[i].Repo = repo.main
	}
	return worktrees, nil
}

// ParseWorktreeList reads `git worktree list --porcelain`. Bare entries are
// left out: there is nothing checked out in them to work in.
func ParseWorktreeList(out string) []model.GitWorktree {
	var worktrees []model.GitWorktree
	var cur *model.GitWorktree
	bare := false
	flush := func() {
		if cur != nil && !bare {
			worktrees = append(worktrees, *cur)
		}
		cur, bare = nil, false
	}
	for line := range strings.SplitSeq(out, "\n") {
		key, value, _ := strings.Cut(strings.TrimSpace(line), " ")
		switch key {
		case "worktree":
			flush()
			cur = &model.GitWorktree{Path: value}
		case "HEAD":
			if cur != nil {
				cur.Head = value
			}
		case "branch":
			if cur != nil {
				cur.Branch = strings.TrimPrefix(value, "refs/heads/")
			}
		case "detached":
			if cur != nil {
				cur.Detached = true
			}
		case "bare":
			bare = true
		}
	}
	flush()
	return worktrees
}

// RemoveCandidates resolves what `worktree rm` would remove. The main
// worktree is never one of them.
func (w *RealWorktree) RemoveCandidates(opts model.WorktreeRemoveOpts) ([]model.GitWorktree, error) {
	cwd, err := w.os.Getwd()
	if err != nil {
		return nil, err
	}
	repo, err := w.repoAt(cwd)
	if err != nil {
		return nil, err
	}
	worktrees, err := w.list(repo)
	if err != nil {
		return nil, err
	}
	linked := slices.DeleteFunc(worktrees, func(wt model.GitWorktree) bool { return wt.Main })

	var picked []model.GitWorktree
	add := func(wt model.GitWorktree, reason string) {
		if slices.ContainsFunc(picked, func(p model.GitWorktree) bool { return p.Path == wt.Path }) {
			return
		}
		wt.Reason = reason
		picked = append(picked, wt)
	}
	for _, target := range opts.Targets {
		wt, ok := w.matchWorktree(linked, target)
		if !ok {
			return nil, fmt.Errorf("%s is not a linked worktree of %s", target, repo.main)
		}
		add(wt, "")
	}
	if opts.Gone {
		w.git.FetchPrune(repo.main) // best-effort: without it nothing is gone yet
		gone, err := w.git.GoneBranches(repo.main)
		if err != nil {
			return nil, err
		}
		for _, wt := range linked {
			if wt.Branch != "" && slices.Contains(gone, wt.Branch) {
				add(wt, "gone")
			}
		}
	}
	if opts.Merged {
		base := w.base(repo, "")
		merged, err := w.git.MergedBranches(repo.main, base)
		if err != nil {
			return nil, err
		}
		for _, wt := range linked {
			// A branch just created from the base is "merged" into it too, but
			// it is new work, not finished work: only count branches with
			// commits of their own.
			if wt.Branch == "" || !slices.Contains(merged, wt.Branch) || wt.Head == w.headOf(repo.main, base) {
				continue
			}
			add(wt, "merged")
		}
	}
	return picked, nil
}

func (w *RealWorktree) headOf(repoPath, ref string) string {
	ok, out, err := w.git.RevParse(repoPath, ref)
	if err != nil || !ok {
		return ""
	}
	return out
}

// matchWorktree finds a worktree by its path (or a path inside it), its
// directory name or its branch.
func (w *RealWorktree) matchWorktree(worktrees []model.GitWorktree, target string) (model.GitWorktree, bool) {
	abs := target
	if expanded, err := w.home.ExpandPath(target); err == nil {
		abs = expanded
	}
	abs = w.resolveSymlinks(abs)
	for _, wt := range worktrees {
		root := w.resolveSymlinks(wt.Path)
		if rel, err := filepath.Rel(root, abs); err == nil && filepath.IsAbs(abs) && !strings.HasPrefix(rel, "..") {
			return wt, true
		}
	}
	for _, wt := range worktrees {
		if wt.Branch == target || filepath.Base(wt.Path) == target {
			return wt, true
		}
	}
	return model.GitWorktree{}, false
}

// Remove removes a worktree and, unless keepBranch, its branch. A branch whose
// upstream is gone is deleted even though git can't see it merged, which is
// what a squash-merged pull request looks like.
func (w *RealWorktree) Remove(wt model.GitWorktree, force bool, keepBranch bool) error {
	if wt.Main || wt.Repo == "" {
		return fmt.Errorf("%s is not a linked worktree", wt.Path)
	}
	if _, err := w.git.WorktreeRemove(wt.Repo, wt.Path, force); err != nil {
		return fmt.Errorf("couldn't remove %s (uncommitted changes? --force removes it anyway): %w", wt.Path, err)
	}
	if keepBranch || wt.Branch == "" {
		return nil
	}
	if _, err := w.git.DeleteBranch(wt.Repo, wt.Branch, force || wt.Reason == "gone"); err != nil {
		return fmt.Errorf("removed %s but kept branch %s (not merged? --force deletes it): %w", wt.Path, wt.Branch, err)
	}
	return nil
}
