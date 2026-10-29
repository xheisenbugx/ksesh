package seshcli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/xheisenbugx/ksesh/agent"
	"github.com/xheisenbugx/ksesh/kitty"
	"github.com/xheisenbugx/ksesh/model"
)

func NewAgentCommand(base *BaseDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "agent",
		Aliases: []string{"a"},
		Short:   "Track the coding agents running in kitty and jump to the ones that need you",
		Long: `Track the coding agents (Claude Code, Codex, …) running in kitty windows.

An agent reports its status (working, waiting, done, error, idle) through user
variables on its own kitty window, set by 'ksesh agent hook' from its hooks:
'ksesh agent install' adds them to Claude Code. The status then shows in
'ksesh agent list', next to the session in 'ksesh list -i' and the picker, and,
with contrib/ksesh_agents.py, in kitty's tab bar and desktop notifications.`,
	}
	cmd.AddCommand(
		newAgentHookCommand(base),
		newAgentSetCommand(base),
		newAgentListCommand(base),
		newAgentNextCommand(base),
		newAgentLastCommand(base),
		newAgentFocusCommand(base),
		newAgentShowCommand(base),
		newAgentSendCommand(base),
		newAgentPathCommand(base),
		newAgentDiffCommand(base),
		newAgentInstallCommand(),
		newAgentUninstallCommand(),
	)
	return cmd
}

func newAgentHookCommand(base *BaseDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "hook",
		Short: "Record a Claude Code hook event (reads the hook's JSON on stdin)",
		Long: `Record a Claude Code hook event on the kitty window Claude runs in.

It reads the hook's JSON from stdin, never prints anything (Claude would read it
as context) and always succeeds, so a kitty that can't be reached never gets in
Claude's way. Outside kitty, or without a listen_on socket, it does nothing:
remote control over the tty would write into Claude's own terminal.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			input, _ := io.ReadAll(io.LimitReader(cmd.InOrStdin(), 1<<20))
			if os.Getenv("KITTY_WINDOW_ID") == "" || os.Getenv("KITTY_LISTEN_ON") == "" {
				return nil
			}
			update, ok := agent.FromClaudeHook(input)
			if !ok {
				return nil
			}
			if err := quietKitty(cmd, base).SetAgentStatus(0, update); err != nil {
				slog.Debug("agent hook", "error", err)
			}
			return nil
		},
	}
}

// quietKitty builds the kitty client without buildDeps, which prints config
// errors to stdout: from a hook, that would end up in Claude's context.
func quietKitty(cmd *cobra.Command, base *BaseDeps) kitty.Kitty {
	configPath, _ := cmd.Root().PersistentFlags().GetString("config")
	deps, err := base.BuildAll(configPath)
	if err != nil {
		return kitty.NewKitty(base.Os, base.Shell, model.Config{})
	}
	return deps.Kitty
}

func newAgentSetCommand(base *BaseDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set <working|waiting|done|error|idle|clear>",
		Short: "Set the status of the agent in this kitty window",
		Long: `Set the status of the agent in this kitty window, or another one with --window.

For agents without Claude Code's hooks: call it from their own notification
hooks (Codex's notify, a wrapper script, …). 'clear' drops the status.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, err := buildDeps(cmd, base)
			if err != nil {
				return err
			}
			name, _ := cmd.Flags().GetString("agent")
			msg, _ := cmd.Flags().GetString("message")
			windowID, _ := cmd.Flags().GetInt("window")
			update := agent.Update{Agent: name}
			if args[0] == "clear" {
				update.Clear = true
			} else {
				status, ok := agent.Parse(args[0])
				if !ok {
					return fmt.Errorf("unknown status %q", args[0])
				}
				update.Status = status
				update.Message, update.SetMessage = agent.Summarize(msg), true
			}
			return deps.Kitty.SetAgentStatus(windowID, update)
		},
	}
	cmd.Flags().StringP("agent", "a", "", "name of the agent, e.g. codex")
	cmd.Flags().StringP("message", "m", "", "what the agent needs or finished with")
	cmd.Flags().IntP("window", "w", 0, "kitty window id (default: this window)")
	return cmd
}

func newAgentListCommand(base *BaseDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"l", "ls"},
		Short:   "List the agents in kitty, the ones that need you first",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, err := buildDeps(cmd, base)
			if err != nil {
				return err
			}
			agents, err := deps.Kitty.ListAgents()
			if err != nil {
				return err
			}
			agent.Sort(agents)
			if attention, _ := cmd.Flags().GetBool("attention"); attention {
				kept := agents[:0]
				for _, a := range agents {
					if agent.Status(a.Status).NeedsAttention() {
						kept = append(kept, a)
					}
				}
				agents = kept
			}
			if jsonOutput, _ := cmd.Flags().GetBool("json"); jsonOutput {
				if agents == nil {
					agents = []*model.Agent{}
				}
				encoded, err := json.MarshalIndent(agents, "", "  ")
				if err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), string(encoded))
				return nil
			}
			noColor, _ := cmd.Flags().GetBool("no-color")
			withIDs, _ := cmd.Flags().GetBool("ids")
			sep := "  "
			if withIDs {
				sep = "\t "
			}
			for i, row := range agentRows(agents, time.Now(), !noColor, sep) {
				if withIDs {
					row = strconv.Itoa(agents[i].WindowID) + "\t" + agents[i].Status + "\t" + row
				}
				fmt.Fprintln(cmd.OutOrStdout(), row)
			}
			return nil
		},
	}
	cmd.Flags().BoolP("json", "j", false, "output as json")
	cmd.Flags().BoolP("attention", "A", false, "only the agents waiting, done or failed")
	cmd.Flags().Bool("no-color", false, "don't color the status icons")
	cmd.Flags().Bool("ids", false, "start each line with the window id and the status, and end the session and tab with a tab too, for fzf and 'ksesh agent focus'")
	return cmd
}

func newAgentFocusCommand(base *BaseDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "focus <window-id>",
		Short: "Go to an agent's kitty window",
		Long: "Go to a kitty window, switching to its session first. The argument may be a\n" +
			"line of 'ksesh agent list --ids', so an fzf selection can be passed as is.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := windowIDArg(args[0])
			if err != nil {
				return err
			}
			deps, err := buildDeps(cmd, base)
			if err != nil {
				return err
			}
			return deps.Kitty.FocusWindow(id)
		},
	}
}

// Age renders how long an agent has been in its status, in its largest unit.
func Age(since *time.Time, now time.Time) string {
	if since == nil {
		return "-"
	}
	d := now.Sub(*since)
	switch {
	case d < time.Minute:
		return strconv.Itoa(max(int(d.Seconds()), 0)) + "s"
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())) + "m"
	case d < 24*time.Hour:
		return strconv.Itoa(int(d.Hours())) + "h"
	}
	return strconv.Itoa(int(d.Hours()/24)) + "d"
}

func newAgentNextCommand(base *BaseDeps) *cobra.Command {
	return &cobra.Command{
		Use:     "next",
		Aliases: []string{"n"},
		Short:   "Go to the agent that has waited on you the longest",
		Long: `Go to the agent that needs you most: waiting on you first, then failed, then
done, the longest in that state first. Run again from there, it moves on to the
next one, so it cycles through all of them.

Bind it in kitty.conf with a background launch, so it runs against the window
you're in:

    map super+j launch --type=background ksesh agent next`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return focusAgent(cmd, base, agent.Next, "no agent needs you")
		},
	}
}

func newAgentLastCommand(base *BaseDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "last",
		Short: "Go back to the agent you were with before this one",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return focusAgent(cmd, base, agent.Last, "no other agent")
		},
	}
}

func focusAgent(cmd *cobra.Command, base *BaseDeps, pick func([]*model.Agent, int) (*model.Agent, bool), none string) error {
	deps, err := buildDeps(cmd, base)
	if err != nil {
		return err
	}
	agents, err := deps.Kitty.ListAgents()
	if err != nil {
		return err
	}
	current, _ := deps.Kitty.CurrentWindowID()
	target, ok := pick(agents, current)
	if !ok {
		fmt.Fprintln(cmd.ErrOrStderr(), none)
		return nil
	}
	return deps.Kitty.FocusWindow(target.WindowID)
}

func claudeSettingsPath() (string, error) {
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, "settings.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "settings.json"), nil
}

func newAgentInstallCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Add ksesh's hooks to Claude Code's settings.json",
		Long: `Add the hooks that report Claude Code's status to kitty to ~/.claude/settings.json
(or --settings). Your other settings and hooks are kept; running it again
replaces the hooks it added before. The previous file is kept next to it as
settings.json.ksesh-<time>.bak.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return editClaudeSettings(cmd, func(settings []byte) ([]byte, error) {
				command, _ := cmd.Flags().GetString("command")
				if command == "" {
					exe, err := os.Executable()
					if err != nil {
						return nil, err
					}
					command = exe
				}
				return agent.InstallClaudeHooks(settings, command)
			})
		},
	}
	cmd.Flags().String("settings", "", "Claude settings file (default ~/.claude/settings.json)")
	cmd.Flags().Bool("print", false, "print the new settings instead of writing them")
	cmd.Flags().String("command", "", "how the hooks run ksesh (default: this binary's path)")
	return cmd
}

func newAgentUninstallCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove ksesh's hooks from Claude Code's settings.json",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return editClaudeSettings(cmd, agent.UninstallClaudeHooks)
		},
	}
	cmd.Flags().String("settings", "", "Claude settings file (default ~/.claude/settings.json)")
	cmd.Flags().Bool("print", false, "print the new settings instead of writing them")
	return cmd
}

func editClaudeSettings(cmd *cobra.Command, edit func([]byte) ([]byte, error)) error {
	path, _ := cmd.Flags().GetString("settings")
	if path == "" {
		var err error
		if path, err = claudeSettingsPath(); err != nil {
			return err
		}
	}
	old, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	updated, err := edit(old)
	if err != nil {
		return err
	}
	if printOnly, _ := cmd.Flags().GetBool("print"); printOnly {
		_, err := cmd.OutOrStdout().Write(updated)
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	mode := fs.FileMode(0o600)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	backup := ""
	if old != nil {
		// A fresh name every time: an existing backup may be the user's own.
		stamp := fmt.Sprintf("%s.ksesh-%s", path, time.Now().Format("20060102-150405"))
		var f *os.File
		for i := 0; f == nil; i++ {
			backup = stamp + ".bak"
			if i > 0 {
				backup = fmt.Sprintf("%s-%d.bak", stamp, i)
			}
			f, err = os.OpenFile(backup, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
			if err != nil && (!errors.Is(err, fs.ErrExist) || i > 100) {
				return fmt.Errorf("couldn't back up %s: %w", path, err)
			}
		}
		_, werr := f.Write(old)
		if err := errors.Join(werr, f.Close()); err != nil {
			return fmt.Errorf("couldn't back up %s: %w", path, err)
		}
	}
	if err := os.WriteFile(path, updated, mode); err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), "updated", path)
	if backup != "" {
		fmt.Fprintln(cmd.OutOrStdout(), "previous version kept in", backup)
	}
	return nil
}
