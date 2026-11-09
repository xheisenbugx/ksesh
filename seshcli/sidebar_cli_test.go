package seshcli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xheisenbugx/ksesh/kitty"
	"github.com/xheisenbugx/ksesh/model"
	"github.com/xheisenbugx/ksesh/sidebar"
)

// plainLs is a tab without a sidebar: 1 and 2 in session api, 1 in front.
func plainLs(sidebars ...win) string {
	return kittyLs(tab{id: 10, title: "api", active: true, wins: append([]win{
		{id: 1, session: "api", cwd: "/c/api", active: true, focused: 50},
		{id: 2, session: "api", cwd: "/c/api", focused: 40},
	}, sidebars...)})
}

func sidebarState(t *testing.T) sidebar.State {
	t.Helper()
	path, err := sidebar.StatePath()
	require.NoError(t, err)
	return sidebar.LoadState(path)
}

func setSidebarState(t *testing.T, change func(*sidebar.State)) {
	t.Helper()
	path, err := sidebar.StatePath()
	require.NoError(t, err)
	_, err = sidebar.UpdateState(path, change)
	require.NoError(t, err)
}

func executable(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	require.NoError(t, err)
	return exe
}

func TestSidebarToggle(t *testing.T) {
	launch := func(c *cli, t *testing.T, edge, size string) []any {
		return []any{"launch", "--match", "window_id:1", "--type=tab-dock", "--dock-edge=" + edge, "--dock-size=" + size,
			"--var=ksesh_role=sidebar", "--keep-focus", "--title=threads", "--", executable(t), "--config", c.config, "sidebar"}
	}

	t.Run("on docks one to this tab and keeps you where you were", func(t *testing.T) {
		c := newCLI(t, plainLs()).withConfig(t, "[sidebar]\nedge = \"right\"\nwidth = \"30\"\n")
		c.kitten("12\n", launch(c, t, "right", "30")...)
		c.kitten("", "focus-window", "--match", "id:1")

		_, _, err := c.run("sidebar", "toggle", "on")

		require.NoError(t, err)
		st := sidebarState(t)
		assert.True(t, st.Enabled)
		assert.Equal(t, kitty.SidebarLaunchArgs(kitty.SidebarOpts{Edge: "right", Size: "30",
			Command: []string{executable(t), "--config", c.config, "sidebar"}}), st.Launch,
			"what kitty's watcher runs to give the other tabs theirs")
	})

	t.Run("on is as wide as you last made one", func(t *testing.T) {
		c := newCLI(t, plainLs()).withConfig(t, "[sidebar]\nwidth = \"30\"\n")
		setSidebarState(t, func(s *sidebar.State) { s.Width = "50" })
		c.kitten("12\n", launch(c, t, "left", "50")...)
		c.kitten("", "focus-window", "--match", "id:1")

		_, _, err := c.run("sidebar", "toggle", "on")

		require.NoError(t, err)
	})

	t.Run("on leaves a tab that has one alone", func(t *testing.T) {
		c := newCLI(t, plainLs(sidebarWin(9, "api", 10, false))).withConfig(t, "")

		_, _, err := c.run("sidebar", "toggle", "on")

		require.NoError(t, err)
		assert.True(t, sidebarState(t).Enabled)
	})

	t.Run("without an argument it flips: off closes every sidebar", func(t *testing.T) {
		ls := kittyLs(
			tab{id: 10, active: true, wins: []win{{id: 1, session: "api", active: true}, sidebarWin(9, "api", 0, false)}},
			tab{id: 11, wins: []win{{id: 3, session: "web", active: true}, sidebarWin(19, "web", 0, false)}},
		)
		c := newCLI(t, ls).withConfig(t, "")
		setSidebarState(t, func(s *sidebar.State) { s.Enabled, s.Width = true, "44" })
		c.kitten("", "close-window", "--ignore-no-match", "--match", "id:9 or id:19")

		_, _, err := c.run("sidebar", "toggle")

		require.NoError(t, err)
		st := sidebarState(t)
		assert.False(t, st.Enabled)
		assert.Equal(t, "44", st.Width, "the width you chose is kept for next time")
	})

	t.Run("without an argument it flips: on", func(t *testing.T) {
		c := newCLI(t, plainLs()).withConfig(t, "")
		c.kitten("12\n", launch(c, t, "left", "38")...)
		c.kitten("", "focus-window", "--match", "id:1")

		_, _, err := c.run("sidebar", "toggle")

		require.NoError(t, err)
		assert.True(t, sidebarState(t).Enabled)
	})

	t.Run("off without sidebars closes nothing", func(t *testing.T) {
		c := newCLI(t, plainLs()).withConfig(t, "")
		setSidebarState(t, func(s *sidebar.State) { s.Enabled = true })

		_, _, err := c.run("sidebar", "toggle", "off")

		require.NoError(t, err)
		assert.False(t, sidebarState(t).Enabled)
	})

	t.Run("a launch kitty refuses is the error", func(t *testing.T) {
		c := newCLI(t, plainLs()).withConfig(t, "")
		c.sh.EXPECT().CmdCapture("kitten", append([]any{"@"}, launch(c, t, "left", "38")...)...).Return("", assert.AnError).Once()

		_, _, err := c.run("sidebar", "toggle", "on")

		assert.ErrorIs(t, err, assert.AnError)
	})
}

func TestSidebarFocus(t *testing.T) {
	t.Run("goes to the tab's sidebar", func(t *testing.T) {
		c := newCLI(t, threadsLs(1))
		c.kitten("", "focus-window", "--match", "id:9")

		_, _, err := c.run("sidebar", "focus")

		require.NoError(t, err)
	})

	t.Run("from the sidebar, goes back to the window you were in", func(t *testing.T) {
		c := newCLI(t, threadsLs(9))
		c.kitten("", "focus-window", "--match", "id:1")

		_, _, err := c.run("sidebar", "focus")

		require.NoError(t, err)
	})

	t.Run("from a sidebar alone in its tab, stays", func(t *testing.T) {
		c := newCLI(t, kittyLs(tab{id: 10, active: true, wins: []win{sidebarWin(9, "", 0, true)}}))

		_, _, err := c.run("sidebar", "focus")

		require.NoError(t, err)
	})

	t.Run("opens one in a tab without, and goes to it", func(t *testing.T) {
		c := newCLI(t, plainLs()).withConfig(t, "")
		c.kitten("12\n", "launch", "--match", "window_id:1", "--type=tab-dock", "--dock-edge=left", "--dock-size=38",
			"--var=ksesh_role=sidebar", "--keep-focus", "--title=threads", "--", executable(t), "--config", c.config, "sidebar")
		c.kitten("", "focus-window", "--match", "id:12")

		_, _, err := c.run("sidebar", "focus")

		require.NoError(t, err)
	})

	t.Run("kitty out of reach is the error", func(t *testing.T) {
		c := newCLI(t, "")
		c.sh.EXPECT().CmdCapture("kitten", "@", "ls").Return("", assert.AnError)

		_, _, err := c.run("sidebar", "focus")

		assert.ErrorIs(t, err, assert.AnError)
	})
}

// The sidebar's order of threadsLs: pinned 6, the inbox 3 1 2 newest first,
// working 7; the snoozed 4 and settled 5 are left out.
func TestSidebarStep(t *testing.T) {
	for _, tc := range []struct {
		name    string
		step    string
		current int
		want    []any // the kitten @ calls that go there
	}{
		{"next goes down", "next", 1, []any{"id:2"}},
		{"next wraps around to the top", "next", 7, []any{"id:6"}},
		{"prev goes up, through the other session", "prev", 1, []any{"goto web", "id:3"}},
		{"prev wraps around to the bottom", "prev", 6, []any{"id:7"}},
		{"next from the sidebar starts at the top", "next", 9, []any{"goto web", "id:6"}},
		{"prev from the sidebar starts at the bottom", "prev", 9, []any{"goto web", "id:7"}},
		{"next from a thread the sidebar leaves out starts at the top", "next", 4, []any{"id:6"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newCLI(t, threadsLs(tc.current))
			for _, call := range tc.want {
				if call == "goto web" {
					c.kitten("", "action", "goto_session '/web.kitty-session'")
				} else {
					c.kitten("", "focus-window", "--match", call)
				}
			}

			_, _, err := c.run("sidebar", tc.step)

			require.NoError(t, err)
		})
	}

	t.Run("without threads, says so", func(t *testing.T) {
		c := newCLI(t, plainLs())

		_, stderr, err := c.run("sidebar", "next")

		require.NoError(t, err)
		assert.Equal(t, "no threads\n", stderr)
	})
}

func TestSidebarGoto(t *testing.T) {
	t.Run("goes to the nth thread down the sidebar", func(t *testing.T) {
		c := newCLI(t, threadsLs(1))
		c.kitten("", "action", "goto_session '/web.kitty-session'")
		c.kitten("", "focus-window", "--match", "id:3")

		_, _, err := c.run("sidebar", "goto", "2")

		require.NoError(t, err)
	})

	t.Run("past the last thread", func(t *testing.T) {
		c := newCLI(t, threadsLs(1))

		_, _, err := c.run("sidebar", "goto", "6")

		assert.EqualError(t, err, "no thread 6: the sidebar has 5")
	})

	for _, arg := range []string{"0", "-1", "two"} {
		t.Run("refuses "+arg+" before asking kitty", func(t *testing.T) {
			c := newCLI(t, "")

			_, _, err := c.run("sidebar", "goto", "--", arg)

			assert.EqualError(t, err, `goto takes a thread's place in the sidebar, from 1: "`+arg+`"`)
		})
	}
}

func TestSidebarPrint(t *testing.T) {
	c := newCLI(t, threadsLs(1))
	closedRecord(t, c, "conv-gone", func(r *model.ThreadRecord) { r.Title, r.Session, r.Cwd = "Old refactor", "api", "/c/api" })

	stdout, _, err := c.run("sidebar", "print")

	require.NoError(t, err)
	assert.Equal(t, strings.Join([]string{
		"pinned (1)",
		"  6\tidle\tKeep\t4h ago",
		"active (3)",
		"  3\terror\tLint\t1m ago",
		"  1\tdone\tFix the login\t2m ago",
		"  2\twaiting\tMigrations\t5m ago",
		"working (1)",
		"  7\tworking\tBuild\t3m ago",
		"snoozed (1)",
		"  4\tdone\tLater\tuntil " + fixtureSnoozeUntil.Format("Mon 15:04"),
		"settled (2)",
		"  conv-gone\tclosed\tOld refactor\tclosed, last active 1h ago",
		"  5\tdone\tOld\tsettled 1h ago",
		"",
	}, "\n"), stdout, "a closed thread goes by its conversation, which resume takes")
}

func TestSidebarPrintJSON(t *testing.T) {
	sections := func(t *testing.T, stdout string) map[string][]model.Agent {
		t.Helper()
		var got map[string][]model.Agent
		require.NoError(t, json.Unmarshal([]byte(stdout), &got))
		return got
	}
	ids := func(agents []model.Agent) []string {
		out := []string{}
		for _, a := range agents {
			if a.Closed {
				out = append(out, a.Conversation)
			} else {
				out = append(out, strconv.Itoa(a.WindowID))
			}
		}
		return out
	}

	t.Run("by section", func(t *testing.T) {
		c := newCLI(t, threadsLs(1))
		closedRecord(t, c, "conv-gone", nil)

		stdout, _, err := c.run("sidebar", "print", "--json")

		require.NoError(t, err)
		got := sections(t, stdout)
		assert.Equal(t, map[string][]string{
			"pinned":  {"6"},
			"active":  {"3", "1", "2"},
			"working": {"7"},
			"snoozed": {"4"},
			"settled": {"conv-gone", "5"},
		}, map[string][]string{
			"pinned": ids(got["pinned"]), "active": ids(got["active"]), "working": ids(got["working"]),
			"snoozed": ids(got["snoozed"]), "settled": ids(got["settled"]),
		})
		assert.Equal(t, "Migrations", got["active"][2].CustomTitle)
		assert.Equal(t, "Allow Bash", got["active"][2].Message)
	})

	t.Run("an empty section is an empty list, not null", func(t *testing.T) {
		c := newCLI(t, plainLs())

		stdout, _, err := c.run("sidebar", "print", "-j")

		require.NoError(t, err)
		for _, s := range []string{"pinned", "active", "working", "snoozed", "settled"} {
			assert.Contains(t, stdout, `"`+s+`": []`)
		}
	})
}

func TestSidebarOverlay(t *testing.T) {
	t.Setenv("PATH", "/usr/bin:/bin")

	t.Run("opens over the window you are in, in its directory", func(t *testing.T) {
		c := newCLI(t, threadsLs(2))
		c.kitten("", "launch", "--match", "id:2", "--next-to", "id:2", "--type=overlay", "--cwd=/c/api",
			"--env", "PATH=/usr/bin:/bin", "--", "fzf-agents", "--all")

		_, _, err := c.run("sidebar", "overlay", "--", "fzf-agents", "--all")

		require.NoError(t, err)
	})

	t.Run("from the sidebar, opens over the window you were in", func(t *testing.T) {
		c := newCLI(t, threadsLs(9))
		c.kitten("", "launch", "--match", "id:1", "--next-to", "id:1", "--type=overlay", "--cwd=/c/api",
			"--env", "PATH=/usr/bin:/bin", "--", "fzf-agents")

		_, _, err := c.run("sidebar", "overlay", "--", "fzf-agents")

		require.NoError(t, err)
	})

	t.Run("from a sidebar alone in its tab there is nothing to open over", func(t *testing.T) {
		c := newCLI(t, kittyLs(tab{id: 10, active: true, wins: []win{sidebarWin(9, "", 0, true)}}))

		_, _, err := c.run("sidebar", "overlay", "--", "fzf-agents")

		assert.EqualError(t, err, "no window to open over")
	})

	t.Run("needs a command", func(t *testing.T) {
		c := newCLI(t, "")

		_, _, err := c.run("sidebar", "overlay")

		assert.Error(t, err)
	})
}

// The sidebar itself is a TUI; what is tested here is that it refuses to run
// where it can't, before it takes over the terminal.
func TestSidebarRefusesToRunWhereItCant(t *testing.T) {
	t.Run("outside a kitty window", func(t *testing.T) {
		c := newCLI(t, threadsLs(1))

		_, _, err := c.run("sidebar")

		assert.EqualError(t, err, "the sidebar runs inside a kitty window: open it with 'ksesh sidebar toggle'")
	})

	t.Run("without listen_on", func(t *testing.T) {
		c := newCLI(t, threadsLs(9))
		t.Setenv("KITTY_WINDOW_ID", "9")

		_, _, err := c.run("sidebar")

		assert.EqualError(t, err, "the sidebar needs kitty's listen_on (see the README's kitty setup)")
	})
}

// rebuilt is how a sidebar notices ksesh was rebuilt under it, to restart
// as the new binary.
func TestRebuilt(t *testing.T) {
	assert.Nil(t, rebuilt(""), "no binary to watch")
	assert.Nil(t, rebuilt(filepath.Join(t.TempDir(), "gone")))

	write := func(t *testing.T, path, content string, mode os.FileMode, age time.Duration) {
		t.Helper()
		require.NoError(t, os.WriteFile(path, []byte(content), mode))
		require.NoError(t, os.Chmod(path, mode))
		at := time.Now().Add(-age)
		require.NoError(t, os.Chtimes(path, at, at))
	}
	for _, tc := range []struct {
		name    string
		content string
		mode    os.FileMode
		age     time.Duration
		want    bool
	}{
		{"replaced a while ago", "ksesh v2", 0o755, 10 * time.Second, true},
		{"still being written", "ksesh v2", 0o755, 0, false},
		{"not executable yet", "ksesh v2", 0o644, 10 * time.Second, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			exe := filepath.Join(t.TempDir(), "ksesh")
			write(t, exe, "ksesh v1", 0o755, time.Hour)
			changed := rebuilt(exe)
			require.NotNil(t, changed)
			assert.False(t, changed(), "unchanged")

			write(t, exe, tc.content, tc.mode, tc.age)

			assert.Equal(t, tc.want, changed())
		})
	}

	t.Run("deleted", func(t *testing.T) {
		exe := filepath.Join(t.TempDir(), "ksesh")
		write(t, exe, "ksesh v1", 0o755, time.Hour)
		changed := rebuilt(exe)
		require.NoError(t, os.Remove(exe))
		assert.False(t, changed())
	})
}
