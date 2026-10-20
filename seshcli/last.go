package seshcli

import (
	"fmt"

	"github.com/xheisenbugx/ksesh/model"
	"github.com/spf13/cobra"
)

func NewLastCommand(base *BaseDeps) *cobra.Command {
	return &cobra.Command{
		Use:     "last",
		Aliases: []string{"L"},
		Short:   "Connect to the last kitty session",
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, err := buildDeps(cmd, base)
			if err != nil {
				return err
			}

			lastSession, exists := deps.Lister.GetLastKittySession()
			if !exists {
				// TODO: silently fail?
				return fmt.Errorf("No last session found")
			}
			if _, err := deps.Kitty.SwitchOrAttach(lastSession.Name, model.ConnectOpts{}); err != nil {
				return err
			}
			return nil
		},
	}
}
