package seshcli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommandError(t *testing.T) {
	assert.Equal(t, "invalid branch name \"/\"", commandError("Error: invalid branch name \"/\"\nUsage:\n  ksesh worktree new <branch> [flags]\n"))
	assert.Equal(t, "couldn't remove /w (uncommitted changes?)", commandError("removed /a\ncouldn't remove /w (uncommitted changes?)\n"))
}

func TestRunKsesh(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, runKsesh([]string{"/bin/sh", "-c"})(dir, `test "$(pwd -P)" = "$(cd "$0" && pwd -P)"`, dir), "runs in dir")
	err := runKsesh([]string{"/bin/sh", "-c"})(dir, "echo 'Error: it broke' >&2; exit 1")
	assert.EqualError(t, err, "it broke")
}
