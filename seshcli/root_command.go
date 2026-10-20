package seshcli

import (
	"github.com/spf13/cobra"
)

func NewRootCommand(version string) *cobra.Command {
	base := NewBaseDeps()

	rootCmd := &cobra.Command{
		Use:              "ksesh",
		Version:          version,
		Short:            "Smart session manager for kitty",
		Long:             "ksesh is a smart session manager for the kitty terminal that helps you create and manage kitty sessions quickly and easily using zoxide. It is a port of sesh, with kitty's native sessions in place of tmux.",
		TraverseChildren: true,
	}

	rootCmd.PersistentFlags().StringP("config", "C", "", "path to config file")

	rootCmd.AddCommand(
		NewCacheCommand(base),
		NewListCommand(base),
		NewLastCommand(base),
		NewConnectCommand(base),
		NewCloneCommand(base),
		NewMkdirCommand(base),
		NewRootSessionCommand(base),
		NewRenameCommand(base),
		NewKillCommand(base),
		NewPreviewCommand(base),
		NewPickerCommand(base),
		NewWindowCommand(base),
		NewWorktreeCommand(base),
	)

	return rootCmd
}
