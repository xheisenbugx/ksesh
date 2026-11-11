package seshcli

import (
	"github.com/spf13/cobra"

	"github.com/xheisenbugx/ksesh/icon"
	"github.com/xheisenbugx/ksesh/lister"
	"github.com/xheisenbugx/ksesh/model"
	"github.com/xheisenbugx/ksesh/picker"
)

func NewPickerCommand(base *BaseDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "picker",
		Aliases: []string{"pick", "pk"},
		Short:   "Interactive session picker",
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, err := buildDeps(cmd, base)
			if err != nil {
				return err
			}
			if deps.CachingLister != nil {
				defer deps.CachingLister.Wait()
			}

			config, _ := cmd.Flags().GetBool("config")
			kitty, _ := cmd.Flags().GetBool("kitty")
			if legacy, _ := cmd.Flags().GetBool("tmux"); legacy {
				kitty = true
			}
			zoxide, _ := cmd.Flags().GetBool("zoxide")
			hideAttached, _ := cmd.Flags().GetBool("hide-attached")
			sessionfile, _ := cmd.Flags().GetBool("session-files")
			hideDuplicates, _ := cmd.Flags().GetBool("hide-duplicates")

			listerOpts := lister.ListOptions{
				Config:         config,
				HideAttached:   hideAttached,
				Kitty:          kitty,
				Zoxide:         zoxide,
				SessionFile:    sessionfile,
				HideDuplicates: hideDuplicates,
			}
			fetchFunc := func() (model.SeshSessions, error) {
				return deps.Lister.List(listerOpts)
			}

			var pickerOpts picker.PickerOptions
			// Without a source flag, the picker lists everything and switches
			// between the sources with a key, as contrib/ksesh-fzf does.
			if !config && !kitty && !zoxide && !sessionfile {
				pickerOpts.Sources = pickerSources(deps.Lister, listerOpts, len(deps.Config.Hosts) > 0)
			}
			if cmd.Flags().Changed("icons") {
				showIcons := true
				pickerOpts.ShowIcons = &showIcons
			} else {
				showIcons := deps.Config.TUI.ShowIcons
				pickerOpts.ShowIcons = &showIcons
			}
			if cmd.Flags().Changed("separator-aware") {
				separatorAware := true
				pickerOpts.SeparatorAware = &separatorAware
			}
			if cmd.Flags().Changed("prompt") {
				prompt, _ := cmd.Flags().GetString("prompt")
				pickerOpts.Prompt = &prompt
			} else if deps.Config.TUI.Prompt != "" {
				pickerOpts.Prompt = &deps.Config.TUI.Prompt
			}
			if cmd.Flags().Changed("no-alias-auto") {
				disableAliasAutoConnect, _ := cmd.Flags().GetBool("no-alias-auto")
				pickerOpts.DisableAliasAutoConnect = &disableAliasAutoConnect
			}
			// --no-preview wins over --preview so it can override a --preview
			// baked into an alias or wrapper script.
			if cmd.Flags().Changed("preview") {
				preview, _ := cmd.Flags().GetBool("preview")
				pickerOpts.Preview = &preview
			}
			if cmd.Flags().Changed("no-preview") {
				preview := false
				pickerOpts.Preview = &preview
			}
			if cmd.Flags().Changed("placeholder") {
				placeholder, _ := cmd.Flags().GetString("placeholder")
				pickerOpts.Placeholder = &placeholder
			} else if deps.Config.TUI.Placeholder != "" {
				pickerOpts.Placeholder = &deps.Config.TUI.Placeholder
			}
			if cmd.Flags().Changed("query") {
				query, _ := cmd.Flags().GetString("query")
				pickerOpts.Query = &query
			}

			unzoom := func() {}
			if zoom, _ := cmd.Flags().GetBool("zoom"); zoom {
				unzoom = zoomTab(deps.Shell)
			}
			chosen, err := deps.Picker.Pick(fetchFunc, pickerOpts)
			// Back to the tab's own layout before going anywhere.
			unzoom()
			if err != nil {
				return err
			}

			if chosen == "" {
				return nil
			}

			if _, err := deps.Connector.Connect(chosen, model.ConnectOpts{}); err != nil {
				return err
			}

			return nil
		},
	}

	cmd.Flags().BoolP("config", "c", false, "show configured sessions")
	cmd.Flags().BoolP("kitty", "t", false, "show running kitty sessions")
	cmd.Flags().Bool("tmux", false, "alias of --kitty, for scripts written for sesh")
	_ = cmd.Flags().MarkHidden("tmux")
	cmd.Flags().BoolP("zoxide", "z", false, "show zoxide results")
	cmd.Flags().BoolP("hide-attached", "H", false, "don't show currently attached sessions")
	cmd.Flags().BoolP("icons", "i", false, "show icons")
	cmd.Flags().BoolP("session-files", "T", false, "show kitty session files")
	cmd.Flags().BoolP("hide-duplicates", "d", false, "hide duplicate entries")
	cmd.Flags().BoolP("separator-aware", "s", false, "match spaces to separators (-_/\\)")
	cmd.Flags().StringP("prompt", "p", "", "prompt shown in the picker TUI")
	cmd.Flags().String("placeholder", "", "placeholder text in the picker TUI")
	cmd.Flags().StringP("query", "q", "", "prefill the picker's filter with this query")
	cmd.Flags().Bool("no-alias-auto", false, "don't auto-connect when an alias is fully typed")
	cmd.Flags().Bool("preview", false, "show a preview of the highlighted session beside the list")
	cmd.Flags().Bool("no-preview", false, "hide the preview pane")
	cmd.Flags().Bool("zoom", false, "show the tab's stack layout while picking, so an overlay fills the tab")

	return cmd
}

// pickerSources are the lists the picker switches between, all of them first:
// ^a everything, ^t kitty's sessions, ^g the configured ones, ^s the session
// files and ^z zoxide's directories (^x kills or removes the selected row).
func pickerSources(l lister.Lister, base lister.ListOptions, hosts bool) []picker.Source {
	source := func(key, label, src string, set func(*lister.ListOptions)) picker.Source {
		opts := base
		set(&opts)
		return picker.Source{
			Key:   key,
			Label: label,
			Icon:  icon.Glyphs[src].Icon,
			Fetch: func() (model.SeshSessions, error) { return l.List(opts) },
		}
	}
	sources := []picker.Source{
		source("ctrl+a", "all", "", func(*lister.ListOptions) {}),
		source("ctrl+t", "kitty", "kitty", func(o *lister.ListOptions) { o.Kitty = true }),
		source("ctrl+g", "configs", "config", func(o *lister.ListOptions) { o.Config = true }),
		source("ctrl+s", "files", "sessionfile", func(o *lister.ListOptions) { o.SessionFile = true }),
		source("ctrl+z", "zoxide", "zoxide", func(o *lister.ListOptions) { o.Zoxide = true }),
	}
	if hosts {
		// Directories on the [[host]]s, opened over ssh. Only on their own
		// tab: each host is an ssh round trip.
		sources = append(sources, source("ctrl+r", "hosts", "host", func(o *lister.ListOptions) { o.Hosts = true }))
	}
	return sources
}
