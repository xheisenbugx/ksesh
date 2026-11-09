package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const existingGeminiSettings = `{
  "general": {"vimMode": true},
  "hooksConfig": {"enabled": true},
  "hooks": {
    "BeforeTool": [
      {"matcher": "write_file|replace", "hooks": [{"name": "guard", "type": "command", "command": "./guard.sh && echo ok", "timeout": 5000}]}
    ]
  },
  "ui": {"theme": "Dracula"}
}`

func TestInstallGeminiHooks(t *testing.T) {
	out, err := InstallGeminiHooks([]byte(existingGeminiSettings), "/bin/ksesh")
	require.NoError(t, err)
	text := string(out)
	assert.Less(t, strings.Index(text, `"general"`), strings.Index(text, `"hooksConfig"`))
	assert.Less(t, strings.Index(text, `"hooks"`), strings.Index(text, `"ui"`))
	assert.Contains(t, text, "./guard.sh && echo ok", "no HTML escaping of the user's commands")

	var parsed struct {
		Hooks map[string][]struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Name    string `json:"name"`
				Command string `json:"command"`
				Timeout int    `json:"timeout"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	require.NoError(t, json.Unmarshal(out, &parsed))
	assert.Len(t, parsed.Hooks, len(GeminiHookEvents))
	tool := parsed.Hooks["BeforeTool"]
	require.Len(t, tool, 2)
	assert.Equal(t, "guard", tool[0].Hooks[0].Name)
	assert.Equal(t, "ksesh", tool[1].Hooks[0].Name)
	assert.Equal(t, "/bin/ksesh agent hook --agent gemini", tool[1].Hooks[0].Command)
	assert.Equal(t, 5000, tool[1].Hooks[0].Timeout, "Gemini's timeouts are in milliseconds")

	twice, err := InstallGeminiHooks(out, "/bin/ksesh")
	require.NoError(t, err)
	assert.Equal(t, text, string(twice))

	removed, err := UninstallGeminiHooks(twice)
	require.NoError(t, err)
	assert.NotContains(t, string(removed), "agent hook")
	assert.Contains(t, string(removed), "./guard.sh")
	assert.NotContains(t, string(removed), "AfterAgent")
}

func TestInstallGeminiHooksRefusesComments(t *testing.T) {
	_, err := InstallGeminiHooks([]byte("{\n  // mine\n  \"ui\": {}\n}"), "/bin/ksesh")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "comments")
}
