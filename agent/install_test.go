package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const existingSettings = `{
  "permissions": {"defaultMode": "auto"},
  "model": "opus",
  "hooks": {
    "Stop": [
      {"hooks": [{"type": "command", "command": "if [ -n \"$TMUX\" ]; then workmux set-window-status done; fi"}]}
    ]
  },
  "zzz": true
}`

func TestInstallKeepsEverythingElseInOrder(t *testing.T) {
	out, err := InstallClaudeHooks([]byte(existingSettings), "/bin/ksesh")
	require.NoError(t, err)
	text := string(out)

	assert.Less(t, strings.Index(text, `"permissions"`), strings.Index(text, `"model"`))
	assert.Less(t, strings.Index(text, `"hooks"`), strings.Index(text, `"zzz"`))
	assert.Contains(t, text, `then workmux set-window-status done; fi`, "no HTML escaping of the user's commands")
	assert.NotContains(t, text, `&`)

	var parsed struct {
		Hooks map[string][]struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	require.NoError(t, json.Unmarshal(out, &parsed))
	require.Len(t, parsed.Hooks["Stop"], 2, "the existing Stop hook is kept beside ksesh's")
	assert.Equal(t, "/bin/ksesh agent hook", parsed.Hooks["Stop"][1].Hooks[0].Command)
	assert.Len(t, parsed.Hooks, len(ClaudeHookEvents))
	assert.Equal(t, "permission_prompt|elicitation_dialog|elicitation_url_dialog|agent_needs_input", parsed.Hooks["Notification"][0].Matcher)
}

func TestInstallIsIdempotentAndUninstallRestores(t *testing.T) {
	once, err := InstallClaudeHooks([]byte(existingSettings), "/bin/ksesh")
	require.NoError(t, err)
	twice, err := InstallClaudeHooks(once, "/other/ksesh")
	require.NoError(t, err)
	assert.Equal(t, strings.Count(string(once), "agent hook"), strings.Count(string(twice), "agent hook"))
	assert.NotContains(t, string(twice), "/bin/ksesh")

	removed, err := UninstallClaudeHooks(twice)
	require.NoError(t, err)
	assert.NotContains(t, string(removed), "agent hook")
	assert.Contains(t, string(removed), "workmux set-window-status done")
	assert.NotContains(t, string(removed), "UserPromptSubmit", "events left empty are dropped")
}

func TestInstallIntoNothing(t *testing.T) {
	out, err := InstallClaudeHooks(nil, "ksesh")
	require.NoError(t, err)
	assert.True(t, json.Valid(out))

	removed, err := UninstallClaudeHooks(out)
	require.NoError(t, err)
	assert.Equal(t, "{}\n", string(removed))
}

func TestInstallRejectsBrokenSettings(t *testing.T) {
	_, err := InstallClaudeHooks([]byte(`{"hooks": [}`), "ksesh")
	assert.Error(t, err)
}
