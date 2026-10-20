package seshcli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xheisenbugx/ksesh/model"
)

func TestAgentInstallKeepsEveryBackup(t *testing.T) {
	dir := t.TempDir()
	settings := filepath.Join(dir, "settings.json")
	require.NoError(t, os.WriteFile(settings, []byte(`{"model":"opus"}`), 0o600))
	require.NoError(t, os.WriteFile(settings+".bak", []byte("the user's own"), 0o600))

	for range 2 {
		cmd := newAgentInstallCommand()
		cmd.SetArgs([]string{"--settings", settings, "--command", "/bin/ksesh"})
		cmd.SetOut(&strings.Builder{})
		require.NoError(t, cmd.Execute())
	}

	own, err := os.ReadFile(settings + ".bak")
	require.NoError(t, err)
	assert.Equal(t, "the user's own", string(own))
	backups, _ := filepath.Glob(settings + ".ksesh-*.bak")
	assert.Len(t, backups, 2)
	info, err := os.Stat(settings)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	updated, _ := os.ReadFile(settings)
	assert.Contains(t, string(updated), "/bin/ksesh agent hook")
}

func TestAgentLine(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	since := now.Add(-4 * time.Minute)
	line := agentLine(&model.Agent{
		WindowID: 3, Session: "api", TabTitle: "2", Status: "waiting", Since: &since,
		Message: "Needs permission: Bash", Prompt: "fix it",
	}, now, false)
	assert.Contains(t, line, "waiting")
	assert.Contains(t, line, "4m")
	assert.Contains(t, line, "api › 2")
	assert.True(t, strings.HasSuffix(line, "Needs permission: Bash"))

	working := agentLine(&model.Agent{WindowID: 9, Status: "working", Message: "old", Prompt: "fix it"}, now, false)
	assert.Contains(t, working, "window 9")
	assert.True(t, strings.HasSuffix(working, "fix it"), "working shows the prompt it works on")
}

func TestAge(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	ago := func(d time.Duration) *time.Time { t := now.Add(-d); return &t }
	assert.Equal(t, "-", Age(nil, now))
	assert.Equal(t, "30s", Age(ago(30*time.Second), now))
	assert.Equal(t, "2h", Age(ago(2*time.Hour+5*time.Minute), now))
	assert.Equal(t, "3d", Age(ago(72*time.Hour), now))
}
