package seshcli

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Without path flags, install writes each agent's own config: where its
// environment variable says, else under the home directory. HOME is a temp
// directory here, so no real agent config is touched.
func TestAgentInstallFindsEachAgentsConfig(t *testing.T) {
	for _, tc := range []struct {
		name  string
		env   map[string]string
		files []string // relative to the temp dir
	}{
		{"under home", nil, []string{
			"home/.claude/settings.json", "home/.codex/hooks.json", "home/.gemini/settings.json",
		}},
		{"where the agents' variables say", map[string]string{
			"CLAUDE_CONFIG_DIR": "claude-dir", "CODEX_HOME": "codex-dir", "GEMINI_CLI_HOME": "gemini-home",
		}, []string{
			"claude-dir/settings.json", "codex-dir/hooks.json", "gemini-home/.gemini/settings.json",
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("HOME", filepath.Join(dir, "home"))
			for _, v := range []string{"CLAUDE_CONFIG_DIR", "CODEX_HOME", "GEMINI_CLI_HOME"} {
				t.Setenv(v, "")
			}
			for k, v := range tc.env {
				t.Setenv(k, filepath.Join(dir, v))
			}

			runAgentConfigCommand(t, newAgentInstallCommand(), "--all", "--command", "/bin/ksesh")

			for _, f := range tc.files {
				assert.FileExists(t, filepath.Join(dir, f))
			}
		})
	}
}
