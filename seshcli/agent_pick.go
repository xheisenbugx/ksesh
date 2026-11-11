package seshcli

import (
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/xheisenbugx/ksesh/agent"
	"github.com/xheisenbugx/ksesh/kitty"
	"github.com/xheisenbugx/ksesh/model"
	"github.com/xheisenbugx/ksesh/review"
	"github.com/xheisenbugx/ksesh/shell"
	"github.com/xheisenbugx/ksesh/sidebar"
)

// pickPreviewMaxFileLines folds the diff of files with more changed lines
// than this in the picker's preview, as contrib/ksesh-agents-fzf did.
const pickPreviewMaxFileLines = 200

func newAgentPickCommand(base *BaseDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "pick",
		Aliases: []string{"picker", "p"},
		Short:   "Pick an agent: the sidebar's threads full window, with each one's screen or diff",
		Long: `Pick an agent from every thread in kitty, as the sidebar triages them, with
the selected one's screen beside the list (its diff after ctrl+g). Typing
filters; enter goes to the thread, or resumes a closed one, and quits. Most
things you'd go to an agent for are a key away without leaving your window:
ctrl+y approves, ctrl+x denies or interrupts, ctrl+o replies; f1 lists them all.

It is meant for an overlay over your window; --zoom makes the overlay the
whole tab while it is up, an overlay only covering its own window:

    map super+a launch --type=overlay --cwd=current ksesh agent pick --zoom

Needs listen_on in kitty.conf, as the sidebar does.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, err := buildDeps(cmd, base)
			if err != nil {
				return err
			}
			self, err := deps.Kitty.CurrentWindowID()
			if err != nil || !deps.Kitty.IsAttached() {
				return fmt.Errorf("the agent picker runs inside a kitty window")
			}
			if os.Getenv("KITTY_LISTEN_ON") == "" && deps.Config.KittySocket == "" {
				return fmt.Errorf("the agent picker needs kitty's listen_on (see the README's kitty setup)")
			}
			path, err := sidebar.StatePath()
			if err != nil {
				return err
			}
			ksesh, _ := kseshCommand(cmd)
			var env []string
			if p := os.Getenv("PATH"); p != "" {
				env = append(env, "PATH="+p)
			}
			shelves, err := pickerShelves(deps.Config)
			if err != nil {
				return err
			}
			opts := sidebar.Options{
				Picker:       true,
				OpenShelves:  shelves,
				Backend:      deps.Threads,
				Self:         self,
				ThreadOpts:   threadOpts(deps.Config),
				StatePath:    path,
				Ksesh:        ksesh,
				Env:          env,
				AgentCommand: deps.Worktree.AgentCommand,
				Run:          runKsesh(ksesh),
				Repos:        newSidebarRepos(deps),
				OpenURL:      deps.Browser.Open,
				Preview:      agentPreview(deps),
			}
			if k, ok := deps.Kitty.(interface{ UseSnapshot() string }); ok {
				if snapshot := k.UseSnapshot(); snapshot != "" {
					opts.Watch = kitty.NewSnapshotWatch(snapshot).Changed
				}
			}
			if zoom, _ := cmd.Flags().GetBool("zoom"); zoom {
				defer zoomTab(deps.Shell)()
			}
			_, err = tea.NewProgram(sidebar.New(opts)).Run()
			return err
		},
	}
	cmd.Flags().Bool("zoom", false, "show the tab's stack layout while picking, so an overlay fills the tab")
	return cmd
}

// pickerShelves are the shelves [picker] open_shelves opens the picker with:
// Working alone when it isn't set.
func pickerShelves(cfg model.Config) ([]agent.Section, error) {
	names := []string{string(agent.SectionWorking)}
	if cfg.Picker.OpenShelves != nil {
		names = *cfg.Picker.OpenShelves
	}
	shelves := []agent.Section{}
	for _, name := range names {
		switch s := agent.Section(strings.ToLower(strings.TrimSpace(name))); s {
		case agent.SectionWorking, agent.Snoozed, agent.Settled:
			shelves = append(shelves, s)
		default:
			return nil, fmt.Errorf("[picker] open_shelves: no shelf %q (working, snoozed or settled)", name)
		}
	}
	return shelves, nil
}

// agentPreview draws a thread for the picker's preview: what 'ksesh agent
// show' prints (its status, where it runs, then its screen), or its review
// with diff. A closed thread has no screen: what is known of it instead.
func agentPreview(deps *Deps) func(a *model.Agent, diff bool, width int) (sidebar.Preview, error) {
	return func(a *model.Agent, diff bool, width int) (sidebar.Preview, error) {
		if diff {
			if a.Path == "" {
				return sidebar.Preview{}, fmt.Errorf("it doesn't say where it runs")
			}
			text, err := reviewText(deps, a.Host, a.Path, "", review.Options{Width: width, MaxFileLines: pickPreviewMaxFileLines}, false)
			return sidebar.Preview{Body: text}, err
		}
		if a.Closed {
			return sidebar.Preview{Body: closedPreview(deps, a)}, nil
		}
		screen, err := deps.Kitty.CaptureWindow(a.WindowID)
		if err != nil {
			return sidebar.Preview{}, err
		}
		header := showHeader(deps, a, time.Now())
		// The rule under it is the pane's own width.
		header[len(header)-1] = "\033[2m" + strings.Repeat("─", max(width, 1)) + "\033[22m"
		return sidebar.Preview{Header: strings.Join(header, "\n"), Body: strings.TrimRight(screen, " \n")}, nil
	}
}

// closedPreview is what the picker shows of a closed thread.
func closedPreview(deps *Deps, a *model.Agent) string {
	where, _ := deps.Home.ShortenHome(a.Path)
	lines := []string{
		"\033[1m" + agent.Title(a) + "\033[22m",
		"\033[2mclosed · " + strings.Join(nonEmpty(a.Session, where, a.Name), " · ") + "\033[22m",
		"",
	}
	if a.Prompt != "" {
		lines = append(lines, "› "+a.Prompt)
	}
	if a.Message != "" {
		lines = append(lines, a.Message)
	}
	return strings.Join(append(lines, "", "\033[2menter resumes it in a new tab of its session\033[22m"), "\n")
}

func nonEmpty(items ...string) []string {
	var out []string
	for _, s := range items {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// zoomTab switches the tab to the stack layout while a picker is up, unless
// it is already: an overlay only covers its own window, so in a tab split
// into several the picker would be squeezed into one of them. It returns
// what puts the layout back.
func zoomTab(sh shell.Shell) func() {
	ls, err := sh.CmdCapture("kitten", "@", "ls", "--self")
	if err != nil || strings.Contains(ls, `"layout": "stack"`) {
		return func() {}
	}
	if _, err := sh.CmdCapture("kitten", "@", "action", "--self", "toggle_layout", "stack"); err != nil {
		return func() {}
	}
	return func() { _, _ = sh.CmdCapture("kitten", "@", "action", "--self", "toggle_layout", "stack") }
}
