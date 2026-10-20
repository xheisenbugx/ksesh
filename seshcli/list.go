package seshcli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/xheisenbugx/ksesh/lister"
	"github.com/xheisenbugx/ksesh/model"
)

func NewListCommand(base *BaseDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"l"},
		Short:   "List sessions",
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, err := buildDeps(cmd, base)
			if err != nil {
				return err
			}
			if deps.CachingLister != nil {
				defer deps.CachingLister.Wait()
			}

			config, _ := cmd.Flags().GetBool("config")
			jsonOutput, _ := cmd.Flags().GetBool("json")
			kitty, _ := cmd.Flags().GetBool("kitty")
			if legacy, _ := cmd.Flags().GetBool("tmux"); legacy {
				kitty = true
			}
			zoxide, _ := cmd.Flags().GetBool("zoxide")
			hideAttached, _ := cmd.Flags().GetBool("hide-attached")
			icons, _ := cmd.Flags().GetBool("icons")
			noColor, _ := cmd.Flags().GetBool("no-color")
			sessionfile, _ := cmd.Flags().GetBool("session-files")
			hideDuplicates, _ := cmd.Flags().GetBool("hide-duplicates")
			panes, _ := cmd.Flags().GetBool("panes")
			blacklisted, _ := cmd.Flags().GetBool("blacklisted")
			listFormat, _ := cmd.Flags().GetString("format")
			formatChanged := cmd.Flags().Changed("format")
			iconExcludes, _ := cmd.Flags().GetStringSlice("icons-exclude")

			if jsonOutput && formatChanged {
				return errors.New("--format cannot be used with --json")
			}

			if panes {
				if _, err := deps.Kitty.GetCurrentSession(); err != nil {
					return errors.New("--panes requires being inside a kitty session")
				}
			}

			sessions, err := deps.Lister.List(lister.ListOptions{
				Config:         config,
				HideAttached:   hideAttached,
				Icons:          icons,
				NoColor:        noColor,
				Json:           jsonOutput,
				Kitty:          kitty,
				Zoxide:         zoxide,
				SessionFile:    sessionfile,
				HideDuplicates: hideDuplicates,
				Panes:          panes,
				Blacklisted:    blacklisted,
				Format:         listFormat,
				FormatSet:      formatChanged,
				IconExcludes:   iconExcludes,
			})
			if err != nil {
				return fmt.Errorf("couldn't list sessions: %q", err)
			}

			if jsonOutput {
				sessionsArray := make([]model.SeshSession, 0, len(sessions.OrderedIndex))
				for _, i := range sessions.OrderedIndex {
					sessionsArray = append(sessionsArray, sessions.Directory[i])
				}
				fmt.Println(base.Json.EncodeSessions(sessionsArray))
				return nil
			}

			for _, i := range sessions.OrderedIndex {
				fmt.Println(sessions.Directory[i].Name)
			}

			return nil
		},
	}

	cmd.Flags().BoolP("config", "c", false, "show configured sessions")
	cmd.Flags().BoolP("json", "j", false, "output as json")
	cmd.Flags().BoolP("kitty", "t", false, "show running kitty sessions")
	cmd.Flags().Bool("tmux", false, "alias of --kitty, for scripts written for sesh")
	_ = cmd.Flags().MarkHidden("tmux")
	cmd.Flags().BoolP("zoxide", "z", false, "show zoxide results")
	cmd.Flags().BoolP("hide-attached", "H", false, "don't show currently attached sessions")
	cmd.Flags().BoolP("icons", "i", false, "show icons")
	cmd.Flags().BoolP("no-color", "n", false, "show icons without color (requires --icons)")
	cmd.Flags().BoolP("session-files", "T", false, "show kitty session files")
	cmd.Flags().BoolP("hide-duplicates", "d", false, "hide duplicate entries")
	cmd.Flags().BoolP("panes", "p", false, "show the kitty windows (panes) of the current session")
	cmd.Flags().BoolP("blacklisted", "b", false, "show blacklisted sessions")
	cmd.Flags().String("format", "", "format each session row ({name}, {session}, {source}, {path}, {active_window_name}, {active_window_name_prefix}; colors: {fg:yellow}...{/fg})")
	cmd.Flags().StringSlice("icons-exclude", nil, "don't show source icons for these session sources (for example: kitty)")

	return cmd
}
