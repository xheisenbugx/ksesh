package seshcli

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/xheisenbugx/ksesh/lister"
)

func NewKillCommand(base *BaseDeps) *cobra.Command {
	return &cobra.Command{
		Use:     "kill [name]",
		Aliases: []string{"k"},
		Short:   "Kill a kitty session",
		Long: "Kill a kitty session by closing all of its windows: the session named by [name], " +
			"or the attached session. The name may carry the icon 'ksesh list -i' prints, so a " +
			"picker line can be passed as is.",
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, err := buildDeps(cmd, base)
			if err != nil {
				return err
			}
			return killSession(deps, strings.Join(args, " "))
		},
	}
}

// killSession ends the session and refreshes the cache, so the next
// 'ksesh list' (an fzf reload, say) doesn't offer the session it just killed.
func killSession(deps *Deps, name string) error {
	if _, err := deps.Kitty.KillSession(deps.Icon.RemoveIcon(name)); err != nil {
		return err
	}
	if deps.CachingLister != nil {
		deps.CachingLister.RefreshCache(lister.ListOptions{})
		deps.CachingLister.Wait()
	}
	return nil
}
