package seshcli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/xheisenbugx/ksesh/setup"
)

func NewSetupCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Set kitty up for ksesh: its keys, sidebar, tab bar and agent hooks",
		Long: `Set kitty up for ksesh, the same on every machine:

- next to kitty.conf, the watcher, tab bar and pickers ksesh's kitty side
  needs (from contrib/), and ksesh.conf: its settings and keys (cmd+p
  sessions, cmd+a agents, cmd+b the sidebar, …), written for this machine's
  paths;
- at the end of kitty.conf, an include of ksesh.conf;
- the hooks that report Claude Code's status (when it is installed), and
  Codex's or Gemini CLI's with --codex or --gemini;
- then, inside kitty, it reloads kitty's config.

Run it again after updating ksesh: it only changes what is out of date. Files
you changed by hand are kept as <file>.ksesh-<time>.bak before being
replaced; kitty.conf always is. To change one of ksesh.conf's settings or
keys, put yours in ksesh-local.conf, next to it: kitty reads it last.

The keys use cmd (super), as on macOS; elsewhere, map others in
ksesh-local.conf.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			dir, _ := cmd.Flags().GetString("kitty-dir")
			if dir == "" {
				home, err := os.UserHomeDir()
				if err != nil {
					return err
				}
				dir = setup.KittyDir(os.Getenv, home, runtime.GOOS)
			}
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			now := time.Now()

			if uninstall, _ := cmd.Flags().GetBool("uninstall"); uninstall {
				changes, kept, err := setup.Uninstall(dir)
				if err != nil {
					return err
				}
				if err := applySetup(out, dir, changes, now, dryRun); err != nil {
					return err
				}
				for _, path := range kept {
					fmt.Fprintln(out, "kept", path, "(you changed it)")
				}
				if len(changes) == 0 {
					fmt.Fprintln(out, "ksesh isn't set up in", dir)
				} else if !dryRun {
					fmt.Fprintln(out, "restart kitty to finish. 'ksesh agent uninstall --all' removes the agents' hooks")
				}
				return nil
			}

			ksesh, _ := cmd.Flags().GetString("ksesh")
			if ksesh == "" {
				exe, err := os.Executable()
				if err != nil {
					return err
				}
				if resolved, err := filepath.EvalSymlinks(exe); err == nil {
					exe = resolved
				}
				ksesh = exe
			}
			ksesh, err := filepath.Abs(ksesh)
			if err != nil {
				return err
			}
			if strings.Contains(ksesh, "go-build") {
				return fmt.Errorf("ksesh runs from a temporary build (%s): install it first, e.g. 'just install', or pass --ksesh", ksesh)
			}

			opts := setup.Options{KittyDir: dir, Ksesh: ksesh, Path: setup.LaunchPath(ksesh, os.Getenv("PATH")), Now: now}
			changes, err := setup.Install(opts)
			if err != nil {
				return err
			}
			fmt.Fprintln(out, "kitty:", tildeHome(dir))
			if err := applySetup(out, dir, changes, now, dryRun); err != nil {
				return err
			}
			if len(changes) == 0 {
				fmt.Fprintln(out, "  up to date")
			}

			if noHooks, _ := cmd.Flags().GetBool("no-hooks"); !noHooks {
				if err := setupHooks(cmd, ksesh, dryRun); err != nil {
					return err
				}
			}
			checkTools(out)
			if dryRun {
				return nil
			}
			if noReload, _ := cmd.Flags().GetBool("no-reload"); !noReload {
				reloadKitty(out, dir, len(changes) > 0)
			}
			return nil
		},
	}
	cmd.Flags().String("kitty-dir", "", "the folder of kitty.conf (default: where kitty reads it from)")
	cmd.Flags().String("ksesh", "", "the ksesh binary kitty runs (default: this one)")
	cmd.Flags().Bool("codex", false, "also install Codex's hooks (you then trust them in Codex's /hooks)")
	cmd.Flags().Bool("gemini", false, "also install Gemini CLI's hooks")
	cmd.Flags().Bool("no-hooks", false, "leave the agents' hooks alone")
	cmd.Flags().Bool("no-reload", false, "don't reload kitty's config")
	cmd.Flags().BoolP("dry-run", "n", false, "say what would change, change nothing")
	cmd.Flags().Bool("uninstall", false, "take ksesh out of kitty.conf, and remove the files setup wrote")
	return cmd
}

// tildeHome is path with the home folder as ~.
func tildeHome(path string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if rel, err := filepath.Rel(home, path); err == nil && !strings.HasPrefix(rel, "..") {
			return filepath.Join("~", rel)
		}
	}
	return path
}

// applySetup makes the changes, saying what each is.
func applySetup(out io.Writer, dir string, changes []setup.Change, now time.Time, dryRun bool) error {
	verb := func(c setup.Change) string {
		switch {
		case c.New == nil:
			return "remove"
		case c.Old == nil:
			return "add"
		}
		return "update"
	}
	var backups map[string]string
	if !dryRun {
		var err error
		if backups, err = setup.Apply(changes, now); err != nil {
			return err
		}
	}
	for _, c := range changes {
		if filepath.Base(c.Path) == "ksesh-setup.json" {
			continue
		}
		name := c.Path
		if rel, err := filepath.Rel(dir, c.Path); err == nil && !strings.HasPrefix(rel, "..") {
			name = rel
		}
		line := "  " + verb(c) + " " + name
		if dryRun {
			line = "  would " + verb(c) + " " + name
		}
		if c.What != "" {
			line += " (" + c.What + ")"
		}
		fmt.Fprintln(out, line)
		if b, ok := backups[c.Path]; ok {
			fmt.Fprintln(out, "    previous version kept in", filepath.Base(b))
		} else if dryRun && c.Backup {
			fmt.Fprintln(out, "    would keep the previous version")
		}
	}
	return nil
}

// setupHooks installs the hooks of the agents asked for, Claude Code's when
// it is installed.
func setupHooks(cmd *cobra.Command, ksesh string, dryRun bool) error {
	out := cmd.OutOrStdout()
	var flags []string
	if _, err := exec.LookPath("claude"); err == nil {
		flags = append(flags, "--claude")
	} else if home, err := os.UserHomeDir(); err == nil {
		if _, err := os.Stat(filepath.Join(home, ".claude")); err == nil {
			flags = append(flags, "--claude")
		}
	}
	for _, agent := range []string{"codex", "gemini"} {
		if on, _ := cmd.Flags().GetBool(agent); on {
			flags = append(flags, "--"+agent)
		}
	}
	if len(flags) == 0 {
		fmt.Fprintln(out, "hooks: no agent to report on (Claude Code isn't installed; --codex, --gemini)")
		return nil
	}
	fmt.Fprintln(out, "hooks:")
	if dryRun {
		fmt.Fprintln(out, "  would run: ksesh agent install", strings.Join(flags, " "))
		return nil
	}
	install := newAgentInstallCommand()
	install.SetArgs(append(flags, "--command", ksesh))
	w := &indent{w: out}
	install.SetOut(w)
	install.SetErr(cmd.ErrOrStderr())
	return install.Execute()
}

// indent writes every line two spaces in.
type indent struct {
	w   io.Writer
	mid bool
}

func (i *indent) Write(p []byte) (int, error) {
	for _, line := range strings.SplitAfter(string(p), "\n") {
		if line == "" {
			continue
		}
		if !i.mid {
			if _, err := io.WriteString(i.w, "  "); err != nil {
				return 0, err
			}
		}
		if _, err := io.WriteString(i.w, line); err != nil {
			return 0, err
		}
		i.mid = !strings.HasSuffix(line, "\n")
	}
	return len(p), nil
}

// tools are the programs ksesh's kitty side runs, and what for.
var tools = []struct{ name, why string }{
	{"fzf", "contrib/ksesh-fzf and ksesh-agents-fzf, if you bind them"},
	{"zoxide", "sessions from the directories you use"},
	{"git", "worktrees and the sidebar's branches"},
	{"gh", "pull requests in the sidebar and 'ksesh wt'"},
}

// minKitty is the first kitty with tab docks, which the sidebar is.
var minKitty = [2]int{0, 50}

var kittyVersion = regexp.MustCompile(`(\d+)\.(\d+)`)

// checkTools says which of the programs ksesh needs are missing, and whether
// kitty is new enough for the sidebar.
func checkTools(out io.Writer) {
	var missing []string
	for _, t := range tools {
		if _, err := exec.LookPath(t.name); err != nil {
			missing = append(missing, t.name+" ("+t.why+")")
		}
	}
	if kitten, err := exec.LookPath("kitten"); err == nil {
		if v, err := exec.Command(kitten, "--version").Output(); err == nil {
			if m := kittyVersion.FindStringSubmatch(string(v)); m != nil {
				major, _ := strconv.Atoi(m[1])
				minor, _ := strconv.Atoi(m[2])
				if major < minKitty[0] || (major == minKitty[0] && minor < minKitty[1]) {
					fmt.Fprintf(out, "kitty %s.%s: the sidebar needs kitty %d.%d or later\n", m[1], m[2], minKitty[0], minKitty[1])
				}
			}
		}
	} else {
		missing = append(missing, "kitten (kitty's remote control: run setup from a kitty window, or add kitty to PATH)")
	}
	if len(missing) > 0 {
		fmt.Fprintln(out, "missing:")
		for _, m := range missing {
			fmt.Fprintln(out, "  "+m)
		}
	}
}

// reloadKitty has the kitty setup runs in read its config again, and the
// watcher and tab bar: what can be, at once. listen_on can't; without a
// socket yet, kitty has to be restarted.
func reloadKitty(out io.Writer, dir string, changed bool) {
	if os.Getenv("KITTY_WINDOW_ID") == "" {
		if changed {
			fmt.Fprintln(out, "start kitty again to load it")
		}
		return
	}
	if os.Getenv("KITTY_LISTEN_ON") == "" {
		fmt.Fprintln(out, "quit and start kitty again to finish: kitty only takes ksesh's socket (listen_on) at start")
		return
	}
	if !changed {
		return
	}
	kitten, err := exec.LookPath("kitten")
	if err != nil {
		fmt.Fprintln(out, "reload kitty's config (ctrl+shift+f5) to load it")
		return
	}
	if err := exec.Command(kitten, "@", "load-config").Run(); err != nil {
		fmt.Fprintln(out, "couldn't reload kitty's config:", err, "(ctrl+shift+f5 does)")
		return
	}
	if err := exec.Command(kitten, "@", "kitten", filepath.Join(dir, "ksesh_reload.py")).Run(); err != nil {
		fmt.Fprintln(out, "reloaded kitty's config; restart kitty for the new watcher and tab bar")
		return
	}
	fmt.Fprintln(out, "reloaded kitty's config, watcher and tab bar")
}
