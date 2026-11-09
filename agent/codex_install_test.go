package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A hooks.json the way other tools leave it, keys in their own order.
const existingCodexHooks = `{
  "hooks": {
    "PermissionRequest": [
      {"hooks": [{"command": "workmux set-window-status waiting", "type": "command"}]}
    ],
    "SessionStart": [
      {"hooks": [{"command": "workmux register-agent", "type": "command"}], "matcher": "startup|resume|clear"}
    ]
  }
}`

// A config.toml whose [hooks] only holds the trust Codex records.
const existingCodexConfig = `# my codex config
model = "gpt-5.5"
notify = ["/Applications/Other.app/notify", "turn-ended"]

[features]
hooks = true

[hooks.state."/home/me/.codex/hooks.json:permission_request:0:0"]
trusted_hash = "sha256:974c"
`

type codexHooksFile struct {
	Hooks map[string][]struct {
		Matcher string `json:"matcher"`
		Hooks   []struct {
			Type    string `json:"type"`
			Command string `json:"command"`
			Timeout int    `json:"timeout"`
		} `json:"hooks"`
	} `json:"hooks"`
}

func TestInstallCodexHooksGoesInHooksJSON(t *testing.T) {
	out, err := InstallCodexHooks(CodexFiles{HooksJSON: []byte(existingCodexHooks), ConfigTOML: []byte(existingCodexConfig)}, "/bin/ksesh")
	require.NoError(t, err)
	assert.Equal(t, existingCodexConfig, string(out.ConfigTOML), "config.toml, notify included, is left alone")

	var parsed codexHooksFile
	require.NoError(t, json.Unmarshal(out.HooksJSON, &parsed))
	assert.Len(t, parsed.Hooks, len(CodexHookEvents))
	perm := parsed.Hooks["PermissionRequest"]
	require.Len(t, perm, 2)
	assert.Equal(t, "workmux set-window-status waiting", perm[0].Hooks[0].Command, "the user's hook keeps its place, which Codex trusts it by")
	assert.Equal(t, "/bin/ksesh agent hook --agent codex", perm[1].Hooks[0].Command)
	assert.Equal(t, 3, perm[1].Hooks[0].Timeout)
	assert.Equal(t, "startup|resume|clear|fork", parsed.Hooks["SessionStart"][1].Matcher)
	assert.Less(t, strings.Index(string(out.HooksJSON), `"command": "workmux register-agent"`), strings.Index(string(out.HooksJSON), `"matcher": "startup|resume|clear"`),
		"the user's keys keep their order")
}

func TestInstallCodexHooksIsIdempotentAndUninstallRestores(t *testing.T) {
	in := CodexFiles{HooksJSON: []byte(existingCodexHooks), ConfigTOML: []byte(existingCodexConfig)}
	once, err := InstallCodexHooks(in, "/bin/ksesh")
	require.NoError(t, err)
	twice, err := InstallCodexHooks(once, "/bin/ksesh")
	require.NoError(t, err)
	assert.Equal(t, string(once.HooksJSON), string(twice.HooksJSON))

	moved, err := InstallCodexHooks(once, "/other/ksesh")
	require.NoError(t, err)
	assert.NotContains(t, string(moved.HooksJSON), "/bin/ksesh")
	assert.Equal(t, len(CodexHookEvents), strings.Count(string(moved.HooksJSON), "agent hook --agent codex"))

	removed, err := UninstallCodexHooks(moved)
	require.NoError(t, err)
	assert.NotContains(t, string(removed.HooksJSON), "agent hook")
	assert.Contains(t, string(removed.HooksJSON), "workmux register-agent")
	assert.NotContains(t, string(removed.HooksJSON), "Interrupt", "events left empty are dropped")
	assert.Equal(t, existingCodexConfig, string(removed.ConfigTOML))
}

func TestInstallCodexHooksIntoNothing(t *testing.T) {
	out, err := InstallCodexHooks(CodexFiles{}, "/Applications/My Tools/ksesh")
	require.NoError(t, err)
	assert.Nil(t, out.ConfigTOML, "no config.toml is created")
	assert.Contains(t, string(out.HooksJSON), `"command": "'/Applications/My Tools/ksesh' agent hook --agent codex"`, "a path with a space is quoted")

	removed, err := UninstallCodexHooks(out)
	require.NoError(t, err)
	assert.Equal(t, "{}\n", string(removed.HooksJSON))

	untouched, err := UninstallCodexHooks(CodexFiles{HooksJSON: []byte(existingCodexHooks)})
	require.NoError(t, err)
	assert.Equal(t, existingCodexHooks, string(untouched.HooksJSON), "a file without ksesh's hooks isn't rewritten")
}

// With hooks of its own in config.toml, Codex would warn about hooks.json at
// every start: ksesh's join them there.
const inlineCodexConfig = `model = "gpt-5.5"

[[hooks.Stop]]
[[hooks.Stop.hooks]]
type = "command"
command = "say done" # spoken
`

func TestInstallCodexHooksJoinsInlineHooks(t *testing.T) {
	out, err := InstallCodexHooks(CodexFiles{ConfigTOML: []byte(inlineCodexConfig)}, "/bin/ksesh")
	require.NoError(t, err)
	assert.Nil(t, out.HooksJSON, "no hooks.json beside inline hooks")
	text := string(out.ConfigTOML)
	assert.True(t, strings.HasPrefix(text, inlineCodexConfig), "the user's config, comments and all, comes first, as it was")

	var parsed struct {
		Hooks map[string][]struct {
			Matcher string `toml:"matcher"`
			Hooks   []struct {
				Command string `toml:"command"`
				Timeout int    `toml:"timeout"`
			} `toml:"hooks"`
		} `toml:"hooks"`
	}
	require.NoError(t, toml.Unmarshal(out.ConfigTOML, &parsed))
	require.Len(t, parsed.Hooks["Stop"], 2)
	assert.Equal(t, "say done", parsed.Hooks["Stop"][0].Hooks[0].Command)
	assert.Equal(t, "/bin/ksesh agent hook --agent codex", parsed.Hooks["Stop"][1].Hooks[0].Command)
	assert.Equal(t, "startup|resume|clear|fork", parsed.Hooks["SessionStart"][0].Matcher)
	assert.Len(t, parsed.Hooks, len(CodexHookEvents))

	twice, err := InstallCodexHooks(out, "/bin/ksesh")
	require.NoError(t, err)
	assert.Equal(t, text, string(twice.ConfigTOML))

	removed, err := UninstallCodexHooks(out)
	require.NoError(t, err)
	assert.Equal(t, inlineCodexConfig, string(removed.ConfigTOML))
}

func TestUninstallCodexKeepsTrustCodexWroteIntoTheBlock(t *testing.T) {
	out, err := InstallCodexHooks(CodexFiles{ConfigTOML: []byte(inlineCodexConfig)}, "/bin/ksesh")
	require.NoError(t, err)
	// Codex records trust as it likes, possibly right after a hook it trusts.
	trust := "[hooks.state.\"/home/me/.codex/config.toml:stop:0:0\"]\ntrusted_hash = \"sha256:abc\"\n\n"
	edited := strings.Replace(string(out.ConfigTOML), "[[hooks.Stop]]\n[[hooks.Stop.hooks]]\ntype = \"command\"\ncommand = \"/bin", trust+"[[hooks.Stop]]\n[[hooks.Stop.hooks]]\ntype = \"command\"\ncommand = \"/bin", 1)
	require.NotEqual(t, string(out.ConfigTOML), edited)

	removed, err := UninstallCodexHooks(CodexFiles{ConfigTOML: []byte(edited)})
	require.NoError(t, err)
	assert.NotContains(t, string(removed.ConfigTOML), "agent hook")
	assert.Contains(t, string(removed.ConfigTOML), `trusted_hash = "sha256:abc"`)
	var check map[string]any
	assert.NoError(t, toml.Unmarshal(removed.ConfigTOML, &check))
}

func TestInstallCodexHooksRefusesWhatItCantExtend(t *testing.T) {
	// A static array can't be extended by an array of tables.
	config := "[hooks]\nStop = [{ hooks = [{ type = \"command\", command = \"say done\" }] }]\n"
	_, err := InstallCodexHooks(CodexFiles{ConfigTOML: []byte(config)}, "/bin/ksesh")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "hooks.json")

	_, err = InstallCodexHooks(CodexFiles{ConfigTOML: []byte("model = ")}, "/bin/ksesh")
	assert.Error(t, err)
	_, err = InstallCodexHooks(CodexFiles{HooksJSON: []byte(`{"hooks": [}`)}, "/bin/ksesh")
	assert.Error(t, err)
}

func TestCodexHooksDisabled(t *testing.T) {
	assert.False(t, CodexHooksDisabled([]byte(existingCodexConfig)))
	assert.False(t, CodexHooksDisabled(nil))
	assert.True(t, CodexHooksDisabled([]byte("[features]\nhooks = false\n")))
	assert.True(t, CodexHooksDisabled([]byte("[features]\ncodex_hooks = false\n")))
}

func TestIsOwnHookCommand(t *testing.T) {
	assert.True(t, IsOwnHookCommand("/bin/ksesh agent hook"))
	assert.True(t, IsOwnHookCommand("'/My Tools/ksesh' agent hook --agent codex"))
	assert.False(t, IsOwnHookCommand("/bin/ksesh agent hook --agent codex && rm -rf /"))
	assert.False(t, IsOwnHookCommand("workmux agent hook"))
	assert.False(t, IsOwnHookCommand("/bin/ksesh agent list"))
}
