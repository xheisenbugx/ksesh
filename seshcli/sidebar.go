package seshcli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/xheisenbugx/ksesh/agent"
	"github.com/xheisenbugx/ksesh/kitty"
	"github.com/xheisenbugx/ksesh/model"
	"github.com/xheisenbugx/ksesh/sidebar"
)

// threadOpts reads [sidebar]'s triage settings.
func threadOpts(cfg model.Config) agent.ThreadOpts {
	opts := agent.ThreadOpts{AutoSettleAfter: agent.DefaultAutoSettleAfter}
	if days := cfg.Sidebar.AutoSettleAfterDays; days != nil {
		opts.AutoSettleAfter = time.Duration(max(*days, 0)) * 24 * time.Hour
	}
	return opts
}

// sidebarOpts is how sidebars are docked, running this very binary.
func sidebarOpts(cmd *cobra.Command, cfg model.Config) (kitty.SidebarOpts, error) {
	exe, err := os.Executable()
	if err != nil {
		return kitty.SidebarOpts{}, err
	}
	command := []string{exe}
	if configPath, _ := cmd.Root().PersistentFlags().GetString("config"); configPath != "" {
		command = append(command, "--config", configPath)
	}
	return kitty.SidebarOpts{
		Edge:    cfg.Sidebar.Edge,
		Size:    cfg.Sidebar.Width,
		Command: append(command, "sidebar"),
	}, nil
}

func NewSidebarCommand(base *BaseDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "sidebar",
		Aliases: []string{"sb"},
		Short:   "A sidebar of every coding agent, triaged like T3 Code's threads",
		Long: `Run the thread sidebar: every coding agent in kitty as a thread, in sections

  Pinned    kept at the top, in your order
  (inbox)   everything else, the latest activity first
  Working   agents running that don't need you
  Snoozed   hidden until a time, or until the agent needs you
  Settled   done with: by hand, or after days without activity, and the
            closed threads, whose window is gone, to resume

It runs in a window docked to the edge of a kitty tab (kitty 0.50+), opened
with 'ksesh sidebar toggle'. Bind that and 'ksesh sidebar focus' in kitty.conf:

    map super+b       launch --type=background --allow-remote-control ksesh sidebar toggle
    map super+shift+b launch --type=background --allow-remote-control ksesh sidebar focus

Needs listen_on in kitty.conf: a TUI can't share its tty with remote control.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, err := buildDeps(cmd, base)
			if err != nil {
				return err
			}
			self, err := deps.Kitty.CurrentWindowID()
			if err != nil || !deps.Kitty.IsAttached() {
				return fmt.Errorf("the sidebar runs inside a kitty window: open it with 'ksesh sidebar toggle'")
			}
			if os.Getenv("KITTY_LISTEN_ON") == "" && deps.Config.KittySocket == "" {
				return fmt.Errorf("the sidebar needs kitty's listen_on (see the README's kitty setup)")
			}
			path, err := sidebar.StatePath()
			if err != nil {
				return err
			}
			exe, _ := os.Executable()
			m := sidebar.New(sidebar.Options{
				Backend:    deps.Threads,
				Self:       self,
				ThreadOpts: threadOpts(deps.Config),
				StatePath:  path,
				Changed:    rebuilt(exe),
			})
			prog := tea.NewProgram(m)
			// contrib/ksesh_agents.py sends SIGUSR1 when the sidebar's tab
			// comes to the front.
			refresh := make(chan os.Signal, 1)
			signal.Notify(refresh, syscall.SIGUSR1)
			defer signal.Stop(refresh)
			go func() {
				for range refresh {
					prog.Send(sidebar.RefreshMsg{})
				}
			}()
			final, err := prog.Run()
			if err != nil {
				return err
			}
			// Rebuilt: carry on as the new binary, in the same window.
			if m, ok := final.(sidebar.Model); ok && m.Restart() {
				return syscall.Exec(exe, os.Args, os.Environ())
			}
			return nil
		},
	}
	cmd.AddCommand(
		newSidebarToggleCommand(base),
		newSidebarFocusCommand(base),
		newSidebarStepCommand(base, "next", 1),
		newSidebarStepCommand(base, "prev", -1),
		newSidebarPrintCommand(base),
		newSidebarOverlayCommand(base),
	)
	return cmd
}

// rebuilt reports whether the binary at exe was replaced since the sidebar
// started, once the new one has been in place for a second (a build still
// writing it isn't done).
func rebuilt(exe string) func() bool {
	start, err := os.Stat(exe)
	if exe == "" || err != nil {
		return nil
	}
	return func() bool {
		now, err := os.Stat(exe)
		if err != nil || (now.ModTime().Equal(start.ModTime()) && now.Size() == start.Size()) {
			return false
		}
		return time.Since(now.ModTime()) > time.Second && now.Mode()&0o111 != 0
	}
}

func newSidebarToggleCommand(base *BaseDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "toggle [on|off]",
		Short: "Show the sidebar in every tab, or hide it",
		Long: `Show the thread sidebar, or hide it. Shown, the tab you are in gets one now,
and every tab you go to after gets its own, with contrib/ksesh_agents.py loaded
by kitty's watcher. Hidden, every sidebar closes.`,
		Args:      cobra.MaximumNArgs(1),
		ValidArgs: []string{"on", "off"},
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, err := buildDeps(cmd, base)
			if err != nil {
				return err
			}
			path, err := sidebar.StatePath()
			if err != nil {
				return err
			}
			on := !sidebar.LoadState(path).Enabled
			if len(args) == 1 {
				on = args[0] == "on"
			}
			if !on {
				if _, err := sidebar.UpdateState(path, func(s *sidebar.State) { s.Enabled = false }); err != nil {
					return err
				}
				return deps.Kitty.CloseSidebars()
			}
			opts, err := sidebarOpts(cmd, deps.Config)
			if err != nil {
				return err
			}
			if _, err := sidebar.UpdateState(path, func(s *sidebar.State) {
				s.Enabled = true
				s.Launch = kitty.SidebarLaunchArgs(opts)
			}); err != nil {
				return err
			}
			current, err := deps.Kitty.CurrentWindowID()
			if err != nil {
				return err
			}
			if id, err := deps.Kitty.SidebarTab(current); err != nil || id != 0 {
				return err
			}
			if _, err := deps.Kitty.OpenSidebar(current, opts); err != nil {
				return err
			}
			// The dock opens without the focus, but becomes its tab's active
			// window all the same: make sure you stay where you were.
			return deps.Kitty.FocusPlain(current)
		},
	}
}

func newSidebarFocusCommand(base *BaseDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "focus",
		Short: "Go to the tab's sidebar, opening it if needed, or from it back to your window",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, err := buildDeps(cmd, base)
			if err != nil {
				return err
			}
			current, err := deps.Kitty.CurrentWindowID()
			if err != nil {
				return err
			}
			id, err := deps.Kitty.SidebarTab(current)
			if err != nil {
				return err
			}
			if id == current {
				back, err := deps.Kitty.LastWorkWindow(current)
				if err != nil || back == 0 {
					return err
				}
				return deps.Kitty.FocusPlain(back)
			}
			if id == 0 {
				opts, err := sidebarOpts(cmd, deps.Config)
				if err != nil {
					return err
				}
				if id, err = deps.Kitty.OpenSidebar(current, opts); err != nil {
					return err
				}
			}
			return deps.Kitty.FocusPlain(id)
		},
	}
}

// sidebarOrder is the threads in the order the sidebar lists them, leaving
// out the snoozed and settled ones, which it keeps out of the way.
func sidebarOrder(deps *Deps) ([]*model.Agent, error) {
	agents, err := deps.Kitty.ListAgents()
	if err != nil {
		return nil, err
	}
	board := agent.Arrange(agents, time.Now(), threadOpts(deps.Config))
	return board.Ordered(agent.Pinned, agent.Active, agent.SectionWorking), nil
}

func newSidebarStepCommand(base *BaseDeps, name string, delta int) *cobra.Command {
	short := "Go to the next thread down the sidebar"
	if delta < 0 {
		short = "Go to the thread above in the sidebar"
	}
	return &cobra.Command{
		Use:   name,
		Short: short,
		Long:  short + ", wrapping around: pinned, the inbox, then working.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, err := buildDeps(cmd, base)
			if err != nil {
				return err
			}
			threads, err := sidebarOrder(deps)
			if err != nil {
				return err
			}
			if len(threads) == 0 {
				fmt.Fprintln(cmd.ErrOrStderr(), "no threads")
				return nil
			}
			current, _ := deps.Kitty.CurrentWindowID()
			i := slices.IndexFunc(threads, func(a *model.Agent) bool { return a.WindowID == current })
			next := 0
			switch {
			case i >= 0:
				next = (i + delta + len(threads)) % len(threads)
			case delta < 0:
				next = len(threads) - 1
			}
			return deps.Kitty.FocusWindow(threads[next].WindowID)
		},
	}
}

func newSidebarPrintCommand(base *BaseDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "print",
		Short: "Print the threads by section, as the sidebar has them",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, err := buildDeps(cmd, base)
			if err != nil {
				return err
			}
			agents, err := deps.Threads.ListAgents()
			if err != nil {
				return err
			}
			now := time.Now()
			opts := threadOpts(deps.Config)
			board := agent.Arrange(agents, now, opts)
			out := cmd.OutOrStdout()
			if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
				sections := map[agent.Section][]*model.Agent{}
				for _, s := range agent.Sections {
					sections[s] = board[s]
					if sections[s] == nil {
						sections[s] = []*model.Agent{}
					}
				}
				encoded, err := json.MarshalIndent(sections, "", "  ")
				if err != nil {
					return err
				}
				fmt.Fprintln(out, string(encoded))
				return nil
			}
			for _, s := range agent.Sections {
				if len(board[s]) == 0 {
					continue
				}
				fmt.Fprintf(out, "%s (%d)\n", s, len(board[s]))
				for _, a := range board[s] {
					fmt.Fprintf(out, "  %s\t%s\t%s\t%s\n", threadID(a), agent.BadgeOf(a, now), threadLabel(a), threadWhen(a, s, now, opts))
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolP("json", "j", false, "output as json, by section")
	return cmd
}

// threadID is how print names a thread: its window, or for a closed one its
// conversation, which 'ksesh agent resume' takes.
func threadID(a *model.Agent) string {
	if a.Closed {
		return a.Conversation
	}
	return strconv.Itoa(a.WindowID)
}

func threadLabel(a *model.Agent) string {
	if t := agent.CleanTitle(a.Title); t != "" {
		return t
	}
	return agentWhere(a)
}

func threadWhen(a *model.Agent, s agent.Section, now time.Time, opts agent.ThreadOpts) string {
	switch s {
	case agent.Snoozed:
		return "until " + a.Thread.SnoozedUntil.Format("Mon 15:04")
	case agent.Settled:
		at, _ := agent.SettledSince(a, now, opts)
		if a.Closed {
			return "closed, last active " + Age(&at, now) + " ago"
		}
		return "settled " + Age(&at, now) + " ago"
	}
	last := agent.LastActivity(a)
	if last.IsZero() {
		return ""
	}
	return Age(&last, now) + " ago"
}

// threadTarget finds the agent a thread command acts on: the window given,
// or the one you're in.
func threadTarget(deps *Deps, args []string) (*model.Agent, error) {
	if len(args) > 0 {
		return findAgent(deps, args[0])
	}
	current, err := deps.Kitty.CurrentWindowID()
	if err != nil {
		return nil, err
	}
	return findAgent(deps, fmt.Sprint(current))
}

func newThreadCommand(base *BaseDeps, use, short, long string, change func(deps *Deps, a *model.Agent, args []string) (model.Thread, string, error)) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Long:  long,
		Args:  cobra.RangeArgs(0, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, err := buildDeps(cmd, base)
			if err != nil {
				return err
			}
			// snooze takes its time first; the window, like every other
			// thread command's, is the optional last argument.
			target, rest := args, args
			if strings.HasPrefix(use, "snooze") {
				if len(args) == 0 {
					return fmt.Errorf("snooze until when? e.g. 1h, tomorrow, 15:04")
				}
				target, rest = args[1:], args[:1]
			} else if len(args) > 1 {
				return fmt.Errorf("takes at most one window id")
			}
			a, err := threadTarget(deps, target)
			if err != nil {
				return err
			}
			t, done, err := change(deps, a, rest)
			if err != nil {
				return err
			}
			if err := deps.Kitty.SetThread(a.WindowID, t); err != nil {
				return err
			}
			if err := deps.Threads.KeepThread(a.Conversation, t); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), done, threadLabel(a))
			return nil
		},
	}
}

// closedThread finds a closed thread by its conversation id, or a prefix
// of it no other one shares.
func closedThread(deps *Deps, id string) (*model.Agent, error) {
	agents, err := deps.Threads.ListAgents()
	if err != nil {
		return nil, err
	}
	var found []*model.Agent
	for _, a := range agents {
		if a.Conversation == id {
			return a, nil
		}
		if a.Closed && id != "" && strings.HasPrefix(a.Conversation, id) {
			found = append(found, a)
		}
	}
	switch len(found) {
	case 0:
		return nil, fmt.Errorf("no closed thread %s (see 'ksesh sidebar print')", id)
	case 1:
		return found[0], nil
	}
	return nil, fmt.Errorf("%d closed threads start with %s", len(found), id)
}

func newThreadHistoryCommands(base *BaseDeps) []*cobra.Command {
	return []*cobra.Command{
		{
			Use:   "resume <conversation>",
			Short: "Resume a closed thread in a new window",
			Long: `Resume a closed thread, one whose window is gone: a new window in the
thread's session (started again if it isn't running) runs its agent on the same
conversation, in the directory it ran in, e.g. 'claude --resume <id>'. The
window takes on the thread's pin, out of Settled. A thread still open in a
window is gone to instead.

The conversation is the id 'ksesh sidebar print' lists closed threads by, or a
prefix of it.`,
			Args: cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				deps, err := buildDeps(cmd, base)
				if err != nil {
					return err
				}
				a, err := closedThread(deps, args[0])
				if err != nil {
					return err
				}
				return deps.Threads.ResumeThread(a.Conversation)
			},
		},
		{
			Use:   "forget <conversation>",
			Short: "Drop a closed thread from the thread history",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				deps, err := buildDeps(cmd, base)
				if err != nil {
					return err
				}
				a, err := closedThread(deps, args[0])
				if err != nil {
					return err
				}
				if err := deps.Threads.ForgetThread(a.Conversation); err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), "forgot", threadLabel(a))
				return nil
			},
		},
	}
}

func newThreadCommands(base *BaseDeps) []*cobra.Command {
	return []*cobra.Command{
		newThreadCommand(base, "pin [window-id]", "Pin an agent's thread to the top of the sidebar, or unpin it",
			"Pin the thread of the agent in this window (or the one given), or unpin it.\nPinning takes it out of Snoozed and Settled.",
			func(deps *Deps, a *model.Agent, _ []string) (model.Thread, string, error) {
				agents, err := deps.Kitty.ListAgents()
				if err != nil {
					return a.Thread, "", err
				}
				now := time.Now()
				done := "pinned"
				if a.Thread.Pin != 0 {
					done = "unpinned"
				}
				return agent.TogglePin(a, now, threadOpts(deps.Config), agent.NextPin(agents, now)), done, nil
			}),
		newThreadCommand(base, "settle [window-id]", "Settle an agent's thread, or take it back to the inbox",
			"Settle the thread of the agent in this window (or the one given): it moves to\nthe Settled shelf until the agent does something again. Run on a settled\nthread, it goes back to the inbox. An agent still working or waiting on you\ncan't be settled.",
			func(deps *Deps, a *model.Agent, _ []string) (model.Thread, string, error) {
				opts := threadOpts(deps.Config)
				_, was := agent.SettledSince(a, time.Now(), opts)
				t, err := agent.ToggleSettle(a, time.Now(), opts)
				done := "settled"
				if was {
					done = "un-settled"
				}
				return t, done, err
			}),
		newThreadCommand(base, "snooze <when> [window-id]", "Snooze an agent's thread until later",
			`Hide the thread of the agent in this window (or the one given) until when:

  1h, 3h, evening, tomorrow (9:00), next-week (Monday 9:00)
  a duration: 90m, 2h30m, 2d
  a time: 15:04 (tomorrow once it has passed), 2026-12-01 09:00

It comes back early, marked as woken, when the agent starts waiting on you or
fails, and for good when you send it a prompt. Finishing doesn't wake it.`,
			func(deps *Deps, a *model.Agent, args []string) (model.Thread, string, error) {
				now := time.Now()
				until, err := agent.ParseSnooze(args[0], now)
				if err != nil {
					return a.Thread, "", err
				}
				t, err := agent.Snooze(a, now, until)
				return t, "snoozed until " + until.Format("Mon 15:04") + ":", err
			}),
		newThreadCommand(base, "wake [window-id]", "Bring a snoozed thread back now",
			"Bring the snoozed thread of the agent in this window (or the one given) back\nnow, or dismiss the woken mark of one already back.",
			func(deps *Deps, a *model.Agent, _ []string) (model.Thread, string, error) {
				return agent.Wake(a), "woke", nil
			}),
	}
}

func newSidebarOverlayCommand(base *BaseDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "overlay -- command [args...]",
		Short: "Run a command as an overlay over your window, even from the sidebar",
		Long: `Run a command as an overlay over the window you are in. In the sidebar, too
narrow for a picker, it opens over the window you were in before it instead:
bind a picker's key to this for when the sidebar has the focus.

  map --when-focus-on var:ksesh_role=sidebar super+a launch --type=background \
      --env PATH=... ksesh sidebar overlay -- ~/.config/kitty/ksesh-agents-fzf

The command gets this one's PATH.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, err := buildDeps(cmd, base)
			if err != nil {
				return err
			}
			target, err := deps.Kitty.CurrentWindowID()
			if err != nil {
				return err
			}
			if id, err := deps.Kitty.SidebarTab(target); err != nil {
				return err
			} else if id == target {
				if target, err = deps.Kitty.LastWorkWindow(target); err != nil {
					return err
				}
			}
			if target == 0 {
				return fmt.Errorf("no window to open over")
			}
			var env []string
			if path := os.Getenv("PATH"); path != "" {
				env = append(env, "PATH="+path)
			}
			return deps.Kitty.OverlayOn(target, env, args)
		},
	}
}
