package seshcli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runSetup runs `ksesh setup` with args, outside kitty, with HOME and the
// kitty folder in temp directories and no agent on PATH.
func runSetup(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := NewRootCommand("test")
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(append([]string{"setup"}, args...))
	err := cmd.Execute()
	return out.String(), err
}

func setupEnv(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("KITTY_CONFIG_DIRECTORY", filepath.Join(root, "kitty"))
	t.Setenv("KITTY_WINDOW_ID", "")
	t.Setenv("KITTY_LISTEN_ON", "")
	t.Setenv("PATH", "/usr/bin:/bin")
	return filepath.Join(root, "kitty")
}

func TestSetupInstallsKittysSideOnce(t *testing.T) {
	dir := setupEnv(t)
	out, err := runSetup(t, "--ksesh", "/opt/ksesh/ksesh", "--no-hooks")
	require.NoError(t, err)
	assert.Contains(t, out, "  add ksesh.conf (ksesh's settings and keys)\n")
	assert.Contains(t, out, "  add kitty.conf (includes ksesh.conf)\n")
	assert.NotContains(t, out, "ksesh-setup.json", "setup's own bookkeeping isn't worth saying")
	assert.Contains(t, out, "start kitty again to load it")

	conf, err := os.ReadFile(filepath.Join(dir, "ksesh.conf"))
	require.NoError(t, err)
	assert.Contains(t, string(conf), "--env PATH=/opt/ksesh:/usr/bin:/bin:/usr/local/bin:/usr/sbin:/sbin /opt/ksesh/ksesh\n")

	out, err = runSetup(t, "--ksesh", "/opt/ksesh/ksesh", "--no-hooks")
	require.NoError(t, err)
	assert.Contains(t, out, "  up to date\n")
	assert.NotContains(t, out, "start kitty again", "nothing new to load")
}

func TestSetupDryRunChangesNothing(t *testing.T) {
	dir := setupEnv(t)
	out, err := runSetup(t, "--ksesh", "/opt/ksesh/ksesh", "--dry-run")
	require.NoError(t, err)
	assert.Contains(t, out, "  would add ksesh.conf")
	assert.Contains(t, out, "hooks: no agent to report on")
	assert.NoDirExists(t, dir)
}

func TestSetupSaysWhereYourFilesWent(t *testing.T) {
	dir := setupEnv(t)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "kitty.conf"), []byte("font_size 14\n"), 0o644))
	out, err := runSetup(t, "--ksesh", "/opt/ksesh/ksesh", "--no-hooks")
	require.NoError(t, err)
	assert.Contains(t, out, "  update kitty.conf (includes ksesh.conf)\n    previous version kept in kitty.conf.ksesh-")
}

func TestSetupInstallsClaudesHooksWhenItIsThere(t *testing.T) {
	setupEnv(t)
	home := os.Getenv("HOME")
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".claude"), 0o755))
	out, err := runSetup(t, "--ksesh", "/opt/ksesh/ksesh")
	require.NoError(t, err)
	assert.Contains(t, out, "hooks:\n  updated "+filepath.Join(home, ".claude", "settings.json"))
	settings, err := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	require.NoError(t, err)
	assert.Contains(t, string(settings), "/opt/ksesh/ksesh agent hook")
}

func TestSetupRefusesATemporaryBuild(t *testing.T) {
	setupEnv(t)
	_, err := runSetup(t, "--ksesh", "/tmp/go-build123/b001/exe/ksesh")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "install it first")
}

func TestSetupUninstall(t *testing.T) {
	dir := setupEnv(t)
	_, err := runSetup(t, "--ksesh", "/opt/ksesh/ksesh", "--no-hooks")
	require.NoError(t, err)
	out, err := runSetup(t, "--uninstall")
	require.NoError(t, err)
	assert.Contains(t, out, "  remove ksesh.conf\n")
	conf, err := os.ReadFile(filepath.Join(dir, "kitty.conf"))
	require.NoError(t, err)
	assert.Empty(t, strings.TrimSpace(string(conf)))

	out, err = runSetup(t, "--uninstall")
	require.NoError(t, err)
	assert.Contains(t, out, "ksesh isn't set up in")
}
