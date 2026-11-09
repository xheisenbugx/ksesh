package seshcli

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/xheisenbugx/ksesh/agent"
)

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

// codexHome is where Codex keeps its config.toml and hooks.json.
func codexHome() (string, error) {
	if dir := os.Getenv("CODEX_HOME"); dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".codex"), nil
}

func geminiSettingsPath() (string, error) {
	home := os.Getenv("GEMINI_CLI_HOME")
	if home == "" {
		var err error
		if home, err = os.UserHomeDir(); err != nil {
			return "", err
		}
	}
	return filepath.Join(home, ".gemini", "settings.json"), nil
}

func newAgentInstallCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Add ksesh's hooks to Claude Code, Codex or Gemini CLI",
		Long: `Add the hooks that report an agent's status to kitty: to Claude Code's
~/.claude/settings.json (or --settings) by default, to Codex's ~/.codex/hooks.json
with --codex, to Gemini CLI's ~/.gemini/settings.json with --gemini, or to all
three with --all. Your other settings and hooks are kept; running it again
replaces the hooks it added before. The previous file is kept next to it as
<file>.ksesh-<time>.bak.

Codex keeps hooks in hooks.json, unless config.toml already has [hooks] of its
own: then ksesh's join them at the end of config.toml. Codex's notify setting is
left alone. Codex only runs a hook you trusted: open /hooks in Codex once after
installing and trust ksesh's.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			command, _ := cmd.Flags().GetString("command")
			if command == "" {
				exe, err := os.Executable()
				if err != nil {
					return err
				}
				command = exe
			}
			return editAgentConfigs(cmd, true, func(t hookTarget, files agentFiles) (agentFiles, error) {
				return t.install(files, command)
			})
		},
	}
	agentConfigFlags(cmd)
	cmd.Flags().String("command", "", "how the hooks run ksesh (default: this binary's path)")
	return cmd
}

func newAgentUninstallCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove ksesh's hooks from Claude Code, Codex or Gemini CLI",
		Long: `Remove the hooks 'ksesh agent install' added, from Claude Code by default, or
from Codex (--codex), Gemini CLI (--gemini) or all three (--all).`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return editAgentConfigs(cmd, false, func(t hookTarget, files agentFiles) (agentFiles, error) {
				return t.uninstall(files)
			})
		},
	}
	agentConfigFlags(cmd)
	return cmd
}

func agentConfigFlags(cmd *cobra.Command) {
	cmd.Flags().Bool("claude", false, "Claude Code (the default without --codex or --gemini)")
	cmd.Flags().Bool("codex", false, "Codex")
	cmd.Flags().Bool("gemini", false, "Gemini CLI")
	cmd.Flags().Bool("all", false, "Claude Code, Codex and Gemini CLI")
	cmd.Flags().String("settings", "", "Claude settings file (default ~/.claude/settings.json)")
	cmd.Flags().String("codex-home", "", "Codex's folder (default $CODEX_HOME or ~/.codex)")
	cmd.Flags().String("gemini-settings", "", "Gemini CLI settings file (default ~/.gemini/settings.json)")
	cmd.Flags().Bool("print", false, "print the new files instead of writing them")
}

// agentFiles are the config files of one agent, by path; a nil content is a
// file that doesn't exist, or, as a result, that stays as it is.
type agentFiles map[string][]byte

// hookTarget is an agent ksesh can install its hooks into.
type hookTarget struct {
	name      string
	paths     func(cmd *cobra.Command) ([]string, error)
	install   func(files agentFiles, command string) (agentFiles, error)
	uninstall func(files agentFiles) (agentFiles, error)
	// notes are said after installing, about what else it takes.
	notes func(files agentFiles) []string
}

// pathFlag is a path from a flag, or its default.
func pathFlag(cmd *cobra.Command, flag string, def func() (string, error)) (string, error) {
	if p, _ := cmd.Flags().GetString(flag); p != "" {
		return p, nil
	}
	return def()
}

var claudeTarget = hookTarget{
	name: "Claude Code",
	paths: func(cmd *cobra.Command) ([]string, error) {
		p, err := pathFlag(cmd, "settings", claudeSettingsPath)
		return []string{p}, err
	},
	install: func(files agentFiles, command string) (agentFiles, error) {
		return editOne(files, func(b []byte) ([]byte, error) { return agent.InstallClaudeHooks(b, command) })
	},
	uninstall: func(files agentFiles) (agentFiles, error) {
		return editOne(files, agent.UninstallClaudeHooks)
	},
}

var geminiTarget = hookTarget{
	name: "Gemini CLI",
	paths: func(cmd *cobra.Command) ([]string, error) {
		p, err := pathFlag(cmd, "gemini-settings", geminiSettingsPath)
		return []string{p}, err
	},
	install: func(files agentFiles, command string) (agentFiles, error) {
		return editOne(files, func(b []byte) ([]byte, error) { return agent.InstallGeminiHooks(b, command) })
	},
	uninstall: func(files agentFiles) (agentFiles, error) {
		return editOne(files, agent.UninstallGeminiHooks)
	},
}

var codexTarget = hookTarget{
	name: "Codex",
	paths: func(cmd *cobra.Command) ([]string, error) {
		home, err := pathFlag(cmd, "codex-home", codexHome)
		return []string{filepath.Join(home, "hooks.json"), filepath.Join(home, "config.toml")}, err
	},
	install: func(files agentFiles, command string) (agentFiles, error) {
		return editCodex(files, func(in agent.CodexFiles) (agent.CodexFiles, error) {
			return agent.InstallCodexHooks(in, command)
		})
	},
	uninstall: func(files agentFiles) (agentFiles, error) {
		return editCodex(files, agent.UninstallCodexHooks)
	},
	notes: func(files agentFiles) []string {
		notes := []string{"Codex runs a hook only once you trust it: open /hooks in Codex and trust ksesh's"}
		for path, content := range files {
			if filepath.Base(path) == "config.toml" && agent.CodexHooksDisabled(content) {
				notes = append(notes, "Codex's hooks are turned off in "+path+" ([features] hooks = false)")
			}
		}
		return notes
	},
}

// editOne edits an agent's only file.
func editOne(files agentFiles, edit func([]byte) ([]byte, error)) (agentFiles, error) {
	out := agentFiles{}
	for path, content := range files {
		updated, err := edit(content)
		if err != nil {
			return nil, err
		}
		out[path] = updated
	}
	return out, nil
}

func editCodex(files agentFiles, edit func(agent.CodexFiles) (agent.CodexFiles, error)) (agentFiles, error) {
	var hooksPath, configPath string
	for path := range files {
		if filepath.Base(path) == "hooks.json" {
			hooksPath = path
		} else {
			configPath = path
		}
	}
	out, err := edit(agent.CodexFiles{HooksJSON: files[hooksPath], ConfigTOML: files[configPath]})
	if err != nil {
		return nil, err
	}
	return agentFiles{hooksPath: out.HooksJSON, configPath: out.ConfigTOML}, nil
}

func selectedTargets(cmd *cobra.Command) []hookTarget {
	if all, _ := cmd.Flags().GetBool("all"); all {
		return []hookTarget{claudeTarget, codexTarget, geminiTarget}
	}
	var targets []hookTarget
	for _, t := range []struct {
		flag   string
		target hookTarget
	}{{"claude", claudeTarget}, {"codex", codexTarget}, {"gemini", geminiTarget}} {
		if on, _ := cmd.Flags().GetBool(t.flag); on {
			targets = append(targets, t.target)
		}
	}
	if len(targets) == 0 {
		return []hookTarget{claudeTarget}
	}
	return targets
}

// editAgentConfigs edits the config files of every agent asked for. Each
// agent's edits are worked out before anything is written, so a file ksesh
// can't parse leaves all of them as they were.
func editAgentConfigs(cmd *cobra.Command, installing bool, edit func(hookTarget, agentFiles) (agentFiles, error)) error {
	type change struct {
		path     string
		old, new []byte
	}
	var changes []change
	var notes []string
	for _, t := range selectedTargets(cmd) {
		paths, err := t.paths(cmd)
		if err != nil {
			return err
		}
		files := agentFiles{}
		for _, path := range paths {
			content, err := os.ReadFile(path)
			if err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
			files[path] = content
		}
		updated, err := edit(t, files)
		if err != nil {
			return fmt.Errorf("%s: %w", t.name, err)
		}
		for _, path := range paths {
			// Uninstalling never creates a file.
			if updated[path] == nil || (files[path] == nil && !installing) {
				continue
			}
			changes = append(changes, change{path, files[path], updated[path]})
		}
		if installing && t.notes != nil {
			notes = append(notes, t.notes(files)...)
		}
	}
	if printOnly, _ := cmd.Flags().GetBool("print"); printOnly {
		for _, c := range changes {
			if len(changes) > 1 {
				fmt.Fprintf(cmd.OutOrStdout(), "==> %s <==\n", c.path)
			}
			if _, err := cmd.OutOrStdout().Write(c.new); err != nil {
				return err
			}
		}
		return nil
	}
	for _, c := range changes {
		if c.old != nil && bytes.Equal(c.old, c.new) {
			// Only worth saying of a file that has ksesh's hooks: Codex's
			// config.toml is read every time, but rarely where they go.
			if bytes.Contains(c.new, []byte(agent.HookMarker)) {
				fmt.Fprintln(cmd.OutOrStdout(), "unchanged", c.path)
			}
			continue
		}
		if err := writeWithBackup(cmd, c.path, c.old, c.new); err != nil {
			return err
		}
	}
	for _, note := range notes {
		fmt.Fprintln(cmd.OutOrStdout(), "note:", note)
	}
	return nil
}

// writeWithBackup writes a config file, keeping its previous version beside it.
func writeWithBackup(cmd *cobra.Command, path string, old, updated []byte) error {
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
		var err error
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
