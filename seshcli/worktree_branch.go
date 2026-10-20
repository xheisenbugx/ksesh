package seshcli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/xheisenbugx/ksesh/model"
)

// agentFlagDefault is what a bare --agent stands for: the repo's
// agent_command, which New resolves once it knows the repo.
const agentFlagDefault = "\x00default"

func newWorktreeNewCommand(base *BaseDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "new <branch>",
		Aliases: []string{"n", "add"},
		Short:   "Create a worktree for a branch and open it as a session",
		Long: `Create a worktree for a branch of the repo you're in and connect to it as a
kitty session, or connect to it when it exists.

The branch is checked out when it exists locally or on origin, and created from
--base (default: the [[worktree]] base_branch, else origin's default branch)
otherwise. Worktrees go under the [[worktree]] block's worktree_dir, or next to
the repo in <repo>__worktrees/<branch> without one.

A new worktree gets the files listed in .worktreeinclude and the block's copy
list copied in, and its symlink list linked, from the main worktree. The
session then runs create_command, followed by the agent with --agent (or
--prompt), e.g.:

    ksesh wt new feat/login --prompt "add a login page"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, err := buildDeps(cmd, base)
			if err != nil {
				return err
			}
			baseRef, _ := cmd.Flags().GetString("base")
			agentCmd, _ := cmd.Flags().GetString("agent")
			prompt, _ := cmd.Flags().GetString("prompt")
			fetch, _ := cmd.Flags().GetBool("fetch")
			switchFlag, _ := cmd.Flags().GetBool("switch")
			startAgent := agentCmd != "" || prompt != ""
			if agentCmd == agentFlagDefault {
				agentCmd = ""
			}
			_, err = deps.Worktree.New(model.WorktreeNewOpts{
				Branch:     args[0],
				Base:       baseRef,
				Fetch:      fetch,
				StartAgent: startAgent,
				Agent:      agentCmd,
				Prompt:     prompt,
				Switch:     switchFlag,
			})
			return err
		},
	}
	cmd.Flags().StringP("base", "b", "", "ref to start a new branch from")
	cmd.Flags().StringP("agent", "a", "", "start an agent in the session (bare: agent_command, default claude)")
	cmd.Flags().Lookup("agent").NoOptDefVal = agentFlagDefault
	cmd.Flags().StringP("prompt", "p", "", "start the agent with this prompt (implies --agent)")
	cmd.Flags().BoolP("fetch", "f", false, "git fetch first (always done for repos with fetch = true in [[worktree]])")
	cmd.Flags().BoolP("switch", "s", false, "Switch the session rather than attach (for use outside kitty)")
	return cmd
}

func newWorktreeStatusCommand(base *BaseDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "status",
		Aliases: []string{"st"},
		Short:   "Show every worktree of this repo: changes, commits, session and agent",
		Long: `Show every worktree of the repo you're in, one per line:

    <agent>  <branch>  <changes>  <commits vs base>  <path>

The first column is the status of the agents in the worktree's kitty session
(see 'ksesh agent'), or the kitty icon for an open session without one.
Changes count staged (+), unstaged (~) and untracked (?) files; commits are
ahead (↑) and behind (↓) the base branch.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, err := buildDeps(cmd, base)
			if err != nil {
				return err
			}
			worktrees, err := deps.Worktree.Worktrees(true)
			if err != nil {
				return err
			}
			sessions, _ := deps.Kitty.ListSessions()
			if jsonOutput, _ := cmd.Flags().GetBool("json"); jsonOutput {
				type entry struct {
					model.GitWorktree
					Session     string `json:"session,omitempty"`
					AgentStatus string `json:"agent_status,omitempty"`
				}
				entries := make([]entry, 0, len(worktrees))
				for _, wt := range worktrees {
					e := entry{GitWorktree: wt}
					if s := sessionAt(sessions, wt.Path); s != nil {
						e.Session, e.AgentStatus = s.Name, s.AgentStatus
					}
					entries = append(entries, e)
				}
				encoded, err := json.MarshalIndent(entries, "", "  ")
				if err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), string(encoded))
				return nil
			}
			width := 0
			for _, wt := range worktrees {
				width = max(width, len(branchLabel(wt)))
			}
			for _, wt := range worktrees {
				icn := " "
				if s := sessionAt(sessions, wt.Path); s != nil {
					icn = deps.Icon.AddIcon(model.SeshSession{Src: "kitty", AgentStatus: s.AgentStatus})
					icn = strings.TrimSpace(icn)
				}
				path, _ := deps.Home.ShortenHome(wt.Path)
				fmt.Fprintf(cmd.OutOrStdout(), "%s %-*s  %-12s %-8s %s\n",
					icn, width, branchLabel(wt), changesLabel(wt), commitsLabel(wt), path)
			}
			return nil
		},
	}
	cmd.Flags().BoolP("json", "j", false, "output as json")
	return cmd
}

func branchLabel(wt model.GitWorktree) string {
	label := wt.Branch
	if label == "" {
		label = "(" + shortHead(wt.Head) + ")"
	}
	if wt.Main {
		label += " *"
	}
	return label
}

func shortHead(head string) string {
	if len(head) > 7 {
		return head[:7]
	}
	return head
}

func changesLabel(wt model.GitWorktree) string {
	var parts []string
	if wt.Staged > 0 {
		parts = append(parts, fmt.Sprintf("+%d", wt.Staged))
	}
	if wt.Unstaged > 0 {
		parts = append(parts, fmt.Sprintf("~%d", wt.Unstaged))
	}
	if wt.Untracked > 0 {
		parts = append(parts, fmt.Sprintf("?%d", wt.Untracked))
	}
	if len(parts) == 0 {
		return "clean"
	}
	return strings.Join(parts, " ")
}

func commitsLabel(wt model.GitWorktree) string {
	if wt.Base == "" {
		return ""
	}
	if wt.Ahead == 0 && wt.Behind == 0 {
		return "="
	}
	return fmt.Sprintf("↑%d ↓%d", wt.Ahead, wt.Behind)
}

// sessionAt finds the kitty session rooted in a worktree.
func sessionAt(sessions []*model.KittySession, path string) *model.KittySession {
	for _, s := range sessions {
		if s.Path == path || strings.HasPrefix(s.Path, path+string(filepath.Separator)) {
			return s
		}
	}
	return nil
}

func newWorktreeRemoveCommand(base *BaseDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "rm [worktree...]",
		Aliases: []string{"remove"},
		Short:   "Remove worktrees, their branches and their kitty sessions",
		Long: `Remove worktrees of the repo you're in, given by path, directory name or branch
(default: the one of the session you're in), along with their branch and the
kitty session open in them.

--merged picks every worktree whose branch is merged into the base branch, and
--gone every one whose upstream branch was deleted (a merged pull request);
both ask before removing anything unless -y. git refuses to remove a worktree
with uncommitted changes or delete an unmerged branch unless --force.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, err := buildDeps(cmd, base)
			if err != nil {
				return err
			}
			merged, _ := cmd.Flags().GetBool("merged")
			gone, _ := cmd.Flags().GetBool("gone")
			force, _ := cmd.Flags().GetBool("force")
			keepBranch, _ := cmd.Flags().GetBool("keep-branch")
			yes, _ := cmd.Flags().GetBool("yes")

			targets := args
			if len(targets) == 0 && !merged && !gone {
				path, err := deps.Kitty.FocusedSessionPath()
				if err != nil {
					return fmt.Errorf("name a worktree, or run it from a worktree's session: %w", err)
				}
				targets = []string{path}
			}
			candidates, err := deps.Worktree.RemoveCandidates(model.WorktreeRemoveOpts{
				Targets: targets, Merged: merged, Gone: gone,
			})
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if len(candidates) == 0 {
				fmt.Fprintln(out, "no worktree to remove")
				return nil
			}
			if (merged || gone) && !yes {
				for _, wt := range candidates {
					fmt.Fprintf(out, "  %s  %s  (%s)\n", branchLabel(wt), wt.Path, wt.Reason)
				}
				ok, err := confirm(cmd.InOrStdin(), out, fmt.Sprintf("remove %d worktree(s)?", len(candidates)))
				if err != nil || !ok {
					return err
				}
			}

			sessions, _ := deps.Kitty.ListSessions()
			current, _ := deps.Kitty.GetCurrentSession()
			var kill []string
			for _, wt := range candidates {
				if err := deps.Worktree.Remove(wt, force, keepBranch); err != nil {
					fmt.Fprintln(cmd.ErrOrStderr(), err)
					continue
				}
				fmt.Fprintln(out, "removed", wt.Path)
				if s := sessionAt(sessions, wt.Path); s != nil {
					kill = append(kill, s.Name)
				}
			}
			// The session ksesh runs in goes last: closing it ends ksesh.
			var errs []string
			for _, name := range append(without(kill, current), within(kill, current)...) {
				if err := killSession(deps, name); err != nil {
					errs = append(errs, err.Error())
				}
			}
			if len(errs) > 0 {
				return fmt.Errorf("%s", strings.Join(errs, "; "))
			}
			return nil
		},
	}
	cmd.Flags().Bool("merged", false, "remove every worktree whose branch is merged into the base")
	cmd.Flags().Bool("gone", false, "remove every worktree whose upstream branch was deleted")
	cmd.Flags().Bool("force", false, "remove despite uncommitted changes, delete unmerged branches")
	cmd.Flags().Bool("keep-branch", false, "keep the branch")
	cmd.Flags().BoolP("yes", "y", false, "don't ask before removing several")
	return cmd
}

func without(names []string, name string) []string {
	var kept []string
	for _, n := range names {
		if n != name {
			kept = append(kept, n)
		}
	}
	return kept
}

func within(names []string, name string) []string {
	for _, n := range names {
		if n == name {
			return []string{n}
		}
	}
	return nil
}

func confirm(in io.Reader, out io.Writer, question string) (bool, error) {
	if f, ok := in.(*os.File); ok && !term.IsTerminal(int(f.Fd())) {
		return false, fmt.Errorf("not asking without a terminal: pass -y")
	}
	fmt.Fprintf(out, "%s [y/N] ", question)
	answer, _ := bufio.NewReader(in).ReadString('\n')
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes", nil
}
