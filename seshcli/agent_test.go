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

func TestAgentRows(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	since := now.Add(-4 * time.Minute)
	agents := []*model.Agent{
		{WindowID: 3, Session: "api", TabTitle: "✳ Fix login", Status: "waiting", Since: &since,
			Message: "Needs permission: Bash", Prompt: "fix it"},
		{WindowID: 9, Status: "working", Message: "old", Prompt: "fix it"},
		{WindowID: 5, Session: "web", TabTitle: "claude", Status: "idle"},
	}

	plain := agentRows(agents, now, false, "  ")
	assert.Contains(t, plain[0], "waiting  api › Fix login   4m  Needs permission: Bash", "aligned, without Claude's title glyph")
	assert.True(t, strings.HasSuffix(plain[1], "fix it"), "working shows the prompt it works on")
	assert.Contains(t, plain[1], "window 9")
	assert.Contains(t, plain[2], "idle", "without color the status is spelled out")

	colored := agentRows(agents, now, true, "  ")
	assert.NotContains(t, colored[2], "idle", "the dimmed gray circle says it")
	assert.True(t, strings.HasPrefix(colored[2], "\033[2m"))
	assert.True(t, strings.HasPrefix(colored[0], "\033[33m"))
}

func TestAgentRowsForFzfEndTheSearchedColumnWithATab(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	since := now.Add(-4 * time.Minute)
	rows := agentRows([]*model.Agent{{WindowID: 3, Session: "api", TabTitle: "claude", Status: "done", Since: &since, Message: "public"}}, now, false, "\t ")
	searched, shown, ok := strings.Cut(rows[0], "\t")
	require.True(t, ok)
	assert.True(t, strings.HasSuffix(searched, " done  api › claude"), searched)
	assert.Equal(t, "  4m  public", shown)
}

func TestWindowIDArg(t *testing.T) {
	id, err := windowIDArg("42\t waiting api")
	require.NoError(t, err)
	assert.Equal(t, 42, id)
	_, err = windowIDArg("api")
	assert.Error(t, err)
}

func TestAge(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	ago := func(d time.Duration) *time.Time { t := now.Add(-d); return &t }
	assert.Equal(t, "-", Age(nil, now))
	assert.Equal(t, "30s", Age(ago(30*time.Second), now))
	assert.Equal(t, "2h", Age(ago(2*time.Hour+5*time.Minute), now))
	assert.Equal(t, "3d", Age(ago(72*time.Hour), now))
}

func TestAgentRowsWithOnlyIdleAgentsHaveNoStatusColumn(t *testing.T) {
	rows := agentRows([]*model.Agent{{WindowID: 1, Session: "api", TabTitle: "claude", Status: "idle"}}, time.Now(), true, "  ")
	assert.Equal(t, "\033[2m  api › claude\033[22m", rows[0])
}
