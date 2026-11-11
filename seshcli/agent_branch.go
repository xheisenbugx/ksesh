package seshcli

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/xheisenbugx/ksesh/model"
)

// remoteBranch is what `ksesh agent branch` prints: where a directory's
// branch stands, and the URL of the remote it pushes to, which the sidebar
// on another machine asks GitHub about.
type remoteBranch struct {
	Status model.BranchStatus `json:"status"`
	Origin string             `json:"origin,omitempty"`
}

func newAgentBranchCommand(base *BaseDeps) *cobra.Command {
	return &cobra.Command{
		Use:    "branch <dir>",
		Short:  "Print where a directory's branch stands, as JSON, for a sidebar on another machine",
		Hidden: true,
		Long: `Print the branch checked out in a directory, how far it is ahead and behind
its base, and the URL of its origin, as JSON. A sidebar on another machine
runs it over ssh for the threads of agents running on this one.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, err := buildDeps(cmd, base)
			if err != nil {
				return err
			}
			dir, err := deps.Home.ExpandPath(args[0])
			if err != nil {
				return err
			}
			dir = filepath.Clean(dir)
			st, err := deps.Worktree.BranchStatus(dir)
			if err != nil {
				return err
			}
			b := remoteBranch{Status: st}
			if out, err := deps.Shell.CmdInDir(dir, "git", "remote", "get-url", "origin"); err == nil {
				b.Origin = strings.TrimSpace(out)
			}
			raw, err := json.Marshal(b)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), string(raw))
			return err
		},
	}
}
