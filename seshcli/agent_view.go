package seshcli

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/spf13/cobra"

	"github.com/xheisenbugx/ksesh/agent"
	"github.com/xheisenbugx/ksesh/model"
)

// maxWhere is the widest the "session › tab" column of 'ksesh agent list'
// gets; longer tab titles are cut.
const maxWhere = 44

// agentRows renders agents for 'ksesh agent list', one aligned row each:
//
//	waiting   backend › claude          4m  Needs permission: Bash
//
// With color the icon and status are colored by status and idle rows are
// dimmed, with no status word: the gray circle says it.
func agentRows(agents []*model.Agent, now time.Time, color bool) []string {
	width := 0
	wheres := make([]string, len(agents))
	for i, a := range agents {
		wheres[i] = truncate(agentWhere(a), maxWhere)
		width = max(width, utf8.RuneCountInString(wheres[i]))
	}
	// The status word goes where it adds something to the icon: everywhere
	// without color, and for everything but idle with it. Its column is only as
	// wide as the words in it, and gone when there are none.
	words := make([]string, len(agents))
	wordWidth := 0
	for i, a := range agents {
		if status := agent.Status(a.Status); !color || status != agent.Idle {
			words[i] = string(status)
		}
		wordWidth = max(wordWidth, len(words[i]))
	}
	rows := make([]string, len(agents))
	for i, a := range agents {
		status := agent.Status(a.Status)
		icn := " "
		g, hasGlyph := agent.GlyphFor(status)
		if hasGlyph {
			icn = g.Icon
		}
		head := icn
		if wordWidth > 0 {
			head += " " + pad(words[i], wordWidth)
		}
		age := ""
		if a.Since != nil {
			age = Age(a.Since, now)
		}
		rest := strings.TrimRight(fmt.Sprintf("  %s  %3s  %s", pad(wheres[i], width), age, agentDetail(a)), " ")
		switch {
		case color && status == agent.Idle:
			rows[i] = "\033[2m" + head + rest + "\033[22m"
		case color && hasGlyph:
			rows[i] = fmt.Sprintf("\033[%dm%s\033[39m", g.ColorCode, head) + rest
		default:
			rows[i] = head + rest
		}
	}
	return rows
}

// agentWhere is "session › tab title", without the status glyph agents put in
// front of their titles.
func agentWhere(a *model.Agent) string {
	where := a.Session
	if title := agent.CleanTitle(a.TabTitle); title != "" {
		if where != "" {
			where += " › "
		}
		where += title
	}
	if where == "" {
		where = "window " + strconv.Itoa(a.WindowID)
	}
	return where
}

// agentDetail is what an agent is on: what it needs or finished with, or the
// prompt it is working on.
func agentDetail(a *model.Agent) string {
	if a.Message == "" || agent.Status(a.Status) == agent.Working {
		return a.Prompt
	}
	return a.Message
}

func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n-1]) + "…"
}

func pad(s string, width int) string {
	return s + strings.Repeat(" ", max(width-utf8.RuneCountInString(s), 0))
}

// findAgent finds an agent by window id. The argument may be a line of
// 'ksesh agent list --ids', so fzf selections can be passed as they are.
func findAgent(deps *Deps, arg string) (*model.Agent, error) {
	id, err := windowIDArg(arg)
	if err != nil {
		return nil, err
	}
	agents, err := deps.Kitty.ListAgents()
	if err != nil {
		return nil, err
	}
	i := slices.IndexFunc(agents, func(a *model.Agent) bool { return a.WindowID == id })
	if i < 0 {
		return nil, fmt.Errorf("no agent in kitty window %d", id)
	}
	return agents[i], nil
}

func windowIDArg(arg string) (int, error) {
	field, _, _ := strings.Cut(strings.TrimSpace(arg), "\t")
	id, err := strconv.Atoi(strings.TrimSpace(field))
	if err != nil {
		return 0, fmt.Errorf("not a window id: %q", arg)
	}
	return id, nil
}

// ShowHeaderLines is how many lines 'ksesh agent show' prints before the
// screen, for fzf's sticky preview header (--preview-window ~N).
const ShowHeaderLines = 4

func newAgentShowCommand(base *BaseDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "show <window-id>",
		Short: "Print an agent's status, worktree and screen (the agent picker's preview)",
		Long: `Print an agent's status, where it runs, its git branch and changes, what it is
on, and then its screen with the trailing blank lines dropped. The header is
always 4 lines, so fzf can keep it in view with --preview-window '~4,follow'.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, err := buildDeps(cmd, base)
			if err != nil {
				return err
			}
			a, err := findAgent(deps, args[0])
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			for _, line := range showHeader(deps, a, time.Now()) {
				fmt.Fprintln(out, line)
			}
			screen, err := deps.Kitty.CaptureWindow(a.WindowID)
			if err != nil {
				return err
			}
			fmt.Fprintln(out, strings.TrimRight(screen, " \n"))
			return nil
		},
	}
}

func showHeader(deps *Deps, a *model.Agent, now time.Time) []string {
	status := agent.Status(a.Status)
	first := string(status)
	if g, ok := agent.GlyphFor(status); ok {
		first = fmt.Sprintf("\033[%dm%s %s\033[39m", g.ColorCode, g.Icon, status)
	}
	if a.Since != nil {
		first += " · " + Age(a.Since, now)
	}
	first += " · " + agentWhere(a)
	if a.Name != "" {
		first += " · " + a.Name
	}

	path, _ := deps.Home.ShortenHome(a.Path)
	second := "\033[2m" + path
	if ok, branch, err := deps.Git.CurrentBranch(a.Path); err == nil && ok {
		second += "  " + branch
		if s, err := deps.Git.StatusSummary(a.Path); err == nil {
			changes := changesLabel(model.GitWorktree{Staged: s.Staged, Unstaged: s.Unstaged, Untracked: s.Untracked})
			second += "  " + changes
		}
	}
	second += "\033[22m"

	third := ""
	switch {
	case a.Message != "" && status != agent.Working:
		third = a.Message
	case a.Prompt != "":
		third = "› " + a.Prompt
	}
	return []string{first, second, third, "\033[2m" + strings.Repeat("─", 40) + "\033[22m"}
}

func newAgentSendCommand(base *BaseDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "send <window-id> [text...]",
		Short: "Type a reply into an agent, or press keys in it, without switching to it",
		Long: `Type text into an agent's window followed by enter, or press keys there with
--key (kitty key names: enter, escape, ctrl+c, 1, …), without leaving the
window you're in. --if-status only acts when the agent is in one of the given
statuses, so a key meant for a permission prompt can't land in the middle of
something else:

    ksesh agent send 42 --key enter --if-status waiting   # approve
    ksesh agent send 42 --key escape                      # interrupt or deny
    ksesh agent send 42 "use the existing helper instead"`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, err := buildDeps(cmd, base)
			if err != nil {
				return err
			}
			a, err := findAgent(deps, args[0])
			if err != nil {
				return err
			}
			only, _ := cmd.Flags().GetStringSlice("if-status")
			if len(only) > 0 && !slices.Contains(only, a.Status) {
				return fmt.Errorf("the agent is %s, not %s", a.Status, strings.Join(only, " or "))
			}
			keys, _ := cmd.Flags().GetStringSlice("key")
			text := strings.Join(args[1:], " ")
			if text == "" && len(keys) == 0 {
				return fmt.Errorf("nothing to send: give text or --key")
			}
			if text != "" {
				if err := deps.Kitty.SendText(a.WindowID, text, true); err != nil {
					return err
				}
			}
			if len(keys) > 0 {
				return deps.Kitty.SendKey(a.WindowID, keys...)
			}
			return nil
		},
	}
	cmd.Flags().StringSliceP("key", "k", nil, "press these keys (repeatable)")
	cmd.Flags().StringSlice("if-status", nil, "only when the agent is in one of these statuses")
	return cmd
}

func newAgentPathCommand(base *BaseDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "path <window-id>",
		Short: "Print the directory an agent works in",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, err := buildDeps(cmd, base)
			if err != nil {
				return err
			}
			a, err := findAgent(deps, args[0])
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), a.Path)
			return nil
		},
	}
}
