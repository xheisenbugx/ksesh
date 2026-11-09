package seshcli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runAgentConfigCommand(t *testing.T, cmd *cobra.Command, args ...string) string {
	t.Helper()
	var out strings.Builder
	cmd.SetArgs(args)
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	require.NoError(t, cmd.Execute())
	return out.String()
}

func TestAgentInstallAll(t *testing.T) {
	dir := t.TempDir()
	claude := filepath.Join(dir, "claude", "settings.json")
	codex := filepath.Join(dir, "codex")
	gemini := filepath.Join(dir, "gemini", "settings.json")
	require.NoError(t, os.MkdirAll(codex, 0o755))
	config := "model = \"gpt-5.5\"\nnotify = [\"/bin/notify\"]\n"
	require.NoError(t, os.WriteFile(filepath.Join(codex, "config.toml"), []byte(config), 0o600))
	paths := []string{"--settings", claude, "--codex-home", codex, "--gemini-settings", gemini}

	out := runAgentConfigCommand(t, newAgentInstallCommand(), append(paths, "--all", "--command", "/bin/ksesh")...)
	assert.Contains(t, out, "open /hooks in Codex")

	for path, want := range map[string]string{
		claude:                             "/bin/ksesh agent hook\"",
		filepath.Join(codex, "hooks.json"): "/bin/ksesh agent hook --agent codex",
		gemini:                             "/bin/ksesh agent hook --agent gemini",
	} {
		content, err := os.ReadFile(path)
		require.NoError(t, err, path)
		assert.Contains(t, string(content), want, path)
	}
	kept, _ := os.ReadFile(filepath.Join(codex, "config.toml"))
	assert.Equal(t, config, string(kept), "notify and the rest of config.toml are left alone")
	backups, _ := filepath.Glob(filepath.Join(codex, "*.bak"))
	assert.Empty(t, backups, "nothing changed in config.toml, so nothing to back up")

	again := runAgentConfigCommand(t, newAgentInstallCommand(), append(paths, "--all", "--command", "/bin/ksesh")...)
	assert.Equal(t, 3, strings.Count(again, "unchanged"), again)
	backups, _ = filepath.Glob(filepath.Join(dir, "*", "*.bak"))
	assert.Empty(t, backups, "an install that changes nothing writes nothing")

	runAgentConfigCommand(t, newAgentUninstallCommand(), append(paths, "--codex", "--gemini")...)
	for _, path := range []string{filepath.Join(codex, "hooks.json"), gemini} {
		content, _ := os.ReadFile(path)
		assert.NotContains(t, string(content), "agent hook", path)
	}
	content, _ := os.ReadFile(claude)
	assert.Contains(t, string(content), "agent hook", "only the agents asked for are uninstalled")
	backups, _ = filepath.Glob(filepath.Join(dir, "*", "*.bak"))
	assert.Len(t, backups, 2)
}

func TestAgentInstallDefaultsToClaude(t *testing.T) {
	dir := t.TempDir()
	claude := filepath.Join(dir, "settings.json")
	codex := filepath.Join(dir, "codex")
	runAgentConfigCommand(t, newAgentInstallCommand(), "--settings", claude, "--codex-home", codex, "--command", "/bin/ksesh")
	_, err := os.Stat(claude)
	assert.NoError(t, err)
	_, err = os.Stat(codex)
	assert.True(t, os.IsNotExist(err))
}

func TestAgentUninstallCreatesNothing(t *testing.T) {
	dir := t.TempDir()
	runAgentConfigCommand(t, newAgentUninstallCommand(), "--all",
		"--settings", filepath.Join(dir, "c.json"), "--codex-home", filepath.Join(dir, "codex"), "--gemini-settings", filepath.Join(dir, "g.json"))
	entries, _ := os.ReadDir(dir)
	assert.Empty(t, entries)
}

func TestAgentInstallPrintsEveryFile(t *testing.T) {
	dir := t.TempDir()
	out := runAgentConfigCommand(t, newAgentInstallCommand(), "--print", "--codex", "--gemini", "--command", "/bin/ksesh",
		"--codex-home", filepath.Join(dir, "codex"), "--gemini-settings", filepath.Join(dir, "g.json"))
	assert.Contains(t, out, "==> "+filepath.Join(dir, "codex", "hooks.json")+" <==")
	assert.Contains(t, out, "==> "+filepath.Join(dir, "g.json")+" <==")
	entries, _ := os.ReadDir(dir)
	assert.Empty(t, entries, "--print writes nothing")
}

func TestAgentInstallLeavesEverythingOnAParseError(t *testing.T) {
	dir := t.TempDir()
	claude := filepath.Join(dir, "settings.json")
	gemini := filepath.Join(dir, "g.json")
	require.NoError(t, os.WriteFile(gemini, []byte("{ // comment\n}"), 0o600))
	cmd := newAgentInstallCommand()
	cmd.SetArgs([]string{"--claude", "--gemini", "--settings", claude, "--gemini-settings", gemini, "--command", "/bin/ksesh"})
	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&strings.Builder{})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Gemini CLI")
	_, statErr := os.Stat(claude)
	assert.True(t, os.IsNotExist(statErr), "Claude's settings aren't written when Gemini's can't be")
}
