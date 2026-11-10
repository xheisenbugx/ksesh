package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var now = time.Date(2026, 11, 10, 9, 30, 0, 0, time.UTC)

func opts(t *testing.T) Options {
	t.Helper()
	return Options{KittyDir: t.TempDir(), Ksesh: "/home/me/.local/bin/ksesh", Path: "/home/me/.local/bin:/usr/bin:/bin", Now: now}
}

func install(t *testing.T, o Options) []Change {
	t.Helper()
	changes, err := Install(o)
	require.NoError(t, err)
	_, err = Apply(changes, now)
	require.NoError(t, err)
	return changes
}

func names(changes []Change) []string {
	var out []string
	for _, c := range changes {
		out = append(out, filepath.Base(c.Path))
	}
	return out
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(b)
}

func TestInstallIntoAFreshKitty(t *testing.T) {
	o := opts(t)
	changes := install(t, o)
	assert.ElementsMatch(t, []string{
		"ksesh.conf", "ksesh_watcher.py", "ksesh_agents.py", "ksesh_snapshot.py", "ksesh_reload.py",
		"tab_bar.py", "ksesh-fzf", "ksesh-agents-fzf", "main.kitty-session", "kitty.conf", "ksesh-setup.json",
	}, names(changes))
	for _, c := range changes {
		assert.False(t, c.Backup, "nothing to keep: %s", c.Path)
	}

	assert.Equal(t, blockStart+"\ninclude ksesh.conf\n"+blockEnd+"\n", readFile(t, filepath.Join(o.KittyDir, "kitty.conf")))
	info, err := os.Stat(filepath.Join(o.KittyDir, "ksesh-fzf"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), info.Mode().Perm(), "the pickers run as programs")
	assert.FileExists(t, filepath.Join(o.KittyDir, "sessions", "main.kitty-session"))

	conf := readFile(t, filepath.Join(o.KittyDir, "ksesh.conf"))
	assert.Contains(t, conf, "action_alias ksesh         launch --type=background --allow-remote-control --env PATH=/home/me/.local/bin:/usr/bin:/bin /home/me/.local/bin/ksesh\n")
	assert.Contains(t, conf, "map super+b       ksesh sidebar toggle\n")
	assert.Contains(t, conf, "map super+ctrl+9  ksesh sidebar goto 9\n")
	assert.Contains(t, conf, "super+a ksesh_overlay "+o.KittyDir+"/ksesh-agents-fzf\n")
	assert.Contains(t, conf, "watcher ksesh_watcher.py\n")
	assert.True(t, strings.HasSuffix(conf, "globinclude ksesh-local.conf\n"), "your changes are read last")
	assert.NotContains(t, conf, "{{", "the template is filled in")
}

func TestInstallAgainChangesNothing(t *testing.T) {
	o := opts(t)
	install(t, o)
	changes, err := Install(o)
	require.NoError(t, err)
	assert.Empty(t, changes)
}

func TestANewBinaryRewritesOnlyKseshConf(t *testing.T) {
	o := opts(t)
	install(t, o)
	o.Ksesh = "/opt/ksesh/bin/ksesh"
	changes := install(t, o)
	assert.Equal(t, []string{"ksesh.conf", "ksesh-setup.json"}, names(changes))
	assert.False(t, changes[0].Backup, "it is as setup wrote it: no need to keep it")
	assert.Contains(t, readFile(t, filepath.Join(o.KittyDir, "ksesh.conf")), "/opt/ksesh/bin/ksesh")
}

func TestAFileYouChangedIsKeptBeforeBeingReplaced(t *testing.T) {
	o := opts(t)
	install(t, o)
	tabBar := filepath.Join(o.KittyDir, "tab_bar.py")
	require.NoError(t, os.WriteFile(tabBar, []byte("# mine\n"), 0o644))

	changes, err := Install(o)
	require.NoError(t, err)
	require.Equal(t, []string{"tab_bar.py", "ksesh-setup.json"}, names(changes))
	assert.True(t, changes[0].Backup)
	backups, err := Apply(changes, now)
	require.NoError(t, err)
	assert.Equal(t, "# mine\n", readFile(t, backups[tabBar]))
	assert.Equal(t, tabBar+".ksesh-20261110-093000.bak", backups[tabBar])
	assert.NotEqual(t, "# mine\n", readFile(t, tabBar))
}

func TestFilesThatWereThereBeforeAreKept(t *testing.T) {
	o := opts(t)
	require.NoError(t, os.WriteFile(filepath.Join(o.KittyDir, "tab_bar.py"), []byte("# my own tab bar\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(o.KittyDir, "sessions"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(o.KittyDir, "sessions", "main.kitty-session"), []byte("cd ~/src\nlaunch\n"), 0o644))

	changes, err := Install(o)
	require.NoError(t, err)
	for _, c := range changes {
		if filepath.Base(c.Path) == "tab_bar.py" {
			assert.True(t, c.Backup, "a tab bar of your own")
		}
	}
	assert.NotContains(t, names(changes), "main.kitty-session", "the session is yours once there")
}

func TestTheIncludeGoesAtTheEndOfYourKittyConf(t *testing.T) {
	o := opts(t)
	kittyConf := filepath.Join(o.KittyDir, "kitty.conf")
	mine := "font_size 14\nmap super+b toggle_layout stack\n"
	require.NoError(t, os.WriteFile(kittyConf, []byte(mine), 0o644))

	changes, err := Install(o)
	require.NoError(t, err)
	backups, err := Apply(changes, now)
	require.NoError(t, err)
	assert.Equal(t, mine+"\n"+blockStart+"\ninclude ksesh.conf\n"+blockEnd+"\n", readFile(t, kittyConf))
	assert.Equal(t, mine, readFile(t, backups[kittyConf]), "kitty.conf is always kept")

	// Something added after it: the include moves back to the end, where
	// ksesh's keys win.
	require.NoError(t, os.WriteFile(kittyConf, []byte(readFile(t, kittyConf)+"font_size 16\n"), 0o644))
	install(t, o)
	assert.Equal(t, mine+"font_size 16\n\n"+blockStart+"\ninclude ksesh.conf\n"+blockEnd+"\n", readFile(t, kittyConf))
}

func TestUninstall(t *testing.T) {
	o := opts(t)
	kittyConf := filepath.Join(o.KittyDir, "kitty.conf")
	require.NoError(t, os.WriteFile(kittyConf, []byte("font_size 14\n"), 0o644))
	install(t, o)
	agents := filepath.Join(o.KittyDir, "ksesh-agents-fzf")
	require.NoError(t, os.WriteFile(agents, []byte("# tweaked\n"), 0o755))

	changes, kept, err := Uninstall(o.KittyDir)
	require.NoError(t, err)
	assert.Equal(t, []string{agents}, kept, "a file you changed stays")
	_, err = Apply(changes, now)
	require.NoError(t, err)

	assert.Equal(t, "font_size 14\n", readFile(t, kittyConf))
	assert.NoFileExists(t, filepath.Join(o.KittyDir, "ksesh.conf"))
	assert.NoFileExists(t, filepath.Join(o.KittyDir, "tab_bar.py"))
	assert.NoFileExists(t, filepath.Join(o.KittyDir, manifestName))
	assert.FileExists(t, agents)
	assert.FileExists(t, filepath.Join(o.KittyDir, "sessions", "main.kitty-session"))

	changes, _, err = Uninstall(o.KittyDir)
	require.NoError(t, err)
	assert.Empty(t, changes, "nothing left to take out")
}

func TestWithoutIncludeLeavesAnUnfinishedBlock(t *testing.T) {
	conf := []byte("a\n" + blockStart + "\ninclude ksesh.conf\n")
	assert.Equal(t, conf, withoutInclude(conf))
}

func TestPathsWithSpacesAreQuoted(t *testing.T) {
	o := opts(t)
	o.Ksesh = "/Users/Jane Doe/bin/ksesh"
	conf, err := Conf(o)
	require.NoError(t, err)
	assert.Contains(t, string(conf), "--env PATH=/home/me/.local/bin:/usr/bin:/bin '/Users/Jane Doe/bin/ksesh'\n")
	assert.Equal(t, `'it'\''s'`, quote("it's"))
}

func TestLaunchPath(t *testing.T) {
	got := LaunchPath("/home/me/.local/bin/ksesh", "/opt/homebrew/bin:node_modules/.bin:/usr/bin:/opt/homebrew/bin/:")
	assert.Equal(t, "/home/me/.local/bin:/opt/homebrew/bin:/usr/bin:/usr/local/bin:/bin:/usr/sbin:/sbin", got,
		"ksesh's folder first, relative and repeated folders dropped, the system's last")
}

func TestKittyDir(t *testing.T) {
	env := func(vars map[string]string) func(string) string { return func(k string) string { return vars[k] } }
	assert.Equal(t, "/k", KittyDir(env(map[string]string{"KITTY_CONFIG_DIRECTORY": "/k", "XDG_CONFIG_HOME": "/x"}), "/home/me", "linux"))
	assert.Equal(t, "/x/kitty", KittyDir(env(map[string]string{"XDG_CONFIG_HOME": "/x"}), "/home/me", "linux"))
	assert.Equal(t, "/home/me/.config/kitty", KittyDir(env(nil), "/home/me", "linux"))

	home := t.TempDir()
	mac := filepath.Join(home, "Library", "Preferences", "kitty")
	require.NoError(t, os.MkdirAll(mac, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(mac, "kitty.conf"), nil, 0o644))
	assert.Equal(t, mac, KittyDir(env(nil), home, "darwin"), "macOS's own folder, when kitty.conf is only there")
	assert.Equal(t, filepath.Join(home, ".config", "kitty"), KittyDir(env(nil), home, "linux"))
}
