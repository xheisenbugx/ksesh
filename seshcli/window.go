package seshcli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/xheisenbugx/ksesh/model"
)

// addTargetFlags wires --target and the deprecated --session alias it replaced.
// --session lost its -s shorthand to --switch, which now means the same thing
// on every sesh command.
func addTargetFlags(cmd *cobra.Command) {
	cmd.Flags().StringP("target", "t", "", "target session (default: current attached session)")
	cmd.Flags().String("session", "", "target session")
	_ = cmd.Flags().MarkDeprecated("session", "use --target instead")
}

// targetFlag reads the target session, preferring --target over the --session
// alias it replaced.
func targetFlag(cmd *cobra.Command) string {
	if cmd.Flags().Changed("target") {
		target, _ := cmd.Flags().GetString("target")
		return target
	}
	target, _ := cmd.Flags().GetString("session")
	return target
}

func NewWindowCommand(base *BaseDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "tab",
		Aliases: []string{"window", "w"},
		Short:   "List or connect to the tabs of a kitty session (tmux windows)",
		Args:    cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			// `sesh window <name>` used to select-or-create that window.
			// Naming its replacement beats cobra's bare "unknown command".
			name := strings.Join(args, " ")
			return fmt.Errorf(
				"unknown command %q for %q, to connect to the %s tab run: ksesh tab connect %s",
				name, cmd.CommandPath(), name, name,
			)
		},
	}

	cmd.AddCommand(newWindowConnectCommand(base))
	cmd.AddCommand(newWindowListCommand(base))

	return cmd
}

func newWindowListCommand(base *BaseDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"l", "ls"},
		Short:   "List the tabs of a kitty session",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return listWindows(cmd, base)
		},
	}

	addTargetFlags(cmd)
	cmd.Flags().BoolP("json", "j", false, "output as json")

	return cmd
}

func newWindowConnectCommand(base *BaseDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "connect [name|directory]",
		Aliases: []string{"cn", "c"},
		Short:   "Select a tab, creating it if it doesn't exist",
		Long: "Select the tab with the given title in the target session, creating it if it isn't there.\n\n" +
			"With --command, a created tab runs the command as its process and closes when it\n" +
			"exits, while an existing tab has the command typed into whatever is already running\n" +
			"in it. Without a name there is nothing to match, so a tab is always created; --new\n" +
			"forces that for a name you want as a label rather than as something to reuse.\n\n" +
			"Prints the target of the tab, \"tab:<id>\" or \"window:<id>\".",
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return connectWindow(cmd, base, strings.Join(args, " "))
		},
	}

	addTargetFlags(cmd)
	cmd.Flags().StringP("command", "c", "", "command to run in the target")
	cmd.Flags().StringP("path", "p", "", "start directory (default: the target session's root)")
	cmd.Flags().BoolP("background", "b", false, "create without focusing it")
	cmd.Flags().BoolP("switch", "s", false, "Switch the session from outside kitty (kept for sesh compatibility)")
	cmd.Flags().Bool("new", false, "always create, even when the name matches a tab that exists")

	return cmd
}

func listWindows(cmd *cobra.Command, base *BaseDeps) error {
	deps, err := buildDeps(cmd, base)
	if err != nil {
		return err
	}

	targetSession := targetFlag(cmd)
	if err := requireSession(deps, targetSession); err != nil {
		return err
	}

	windows, err := deps.Kitty.ListWindows(targetSession)
	if err != nil {
		return err
	}

	if jsonOutput, _ := cmd.Flags().GetBool("json"); jsonOutput {
		out, err := json.Marshal(windows)
		if err != nil {
			return err
		}
		fmt.Println(string(out))
		return nil
	}
	for _, w := range windows {
		fmt.Println(w.Name)
	}
	return nil
}

func connectWindow(cmd *cobra.Command, base *BaseDeps, name string) error {
	deps, err := buildDeps(cmd, base)
	if err != nil {
		return err
	}

	command, _ := cmd.Flags().GetString("command")
	path, _ := cmd.Flags().GetString("path")
	background, _ := cmd.Flags().GetBool("background")
	switchFlag, _ := cmd.Flags().GetBool("switch")
	forceNew, _ := cmd.Flags().GetBool("new")

	target, err := deps.Connector.ConnectWindow(model.WindowConnectOpts{
		Name:       name,
		New:        forceNew,
		Session:    targetFlag(cmd),
		Path:       path,
		Command:    command,
		Background: background,
		Switch:     switchFlag,
	})
	if err != nil {
		return err
	}
	fmt.Println(target)
	return nil
}

// requireSession keeps the listing command honest about its target: an empty
// target only means "the current session" when there is one (the session ksesh
// runs in, or the focused one outside kitty), and a named session has to
// already be running to have anything to list.
func requireSession(deps *Deps, targetSession string) error {
	if targetSession == "" {
		if _, err := deps.Kitty.GetCurrentSession(); err != nil {
			return errors.New("not inside a kitty session, use --target to specify one")
		}
		return nil
	}
	sessions, err := deps.Kitty.ListSessions()
	if err != nil {
		return err
	}
	for _, s := range sessions {
		if s.Name == targetSession {
			return nil
		}
	}
	return fmt.Errorf("session '%s' not found", targetSession)
}
