package seshcli

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/xheisenbugx/ksesh/model"
)

// threadsLs's agents that need you, as agent.Next ranks them: waiting 2,
// failed 3, then done, longest first: 5 (settled), 4 (snoozed), 1. You put
// 4 and 5 away, so next leaves them out.
func TestAgentNext(t *testing.T) {
	for _, tc := range []struct {
		name    string
		current int
		want    []any
	}{
		{"skips the snoozed and settled threads", 3, []any{"goto api", "id:1"}},
		{"cycles back to the one waiting longest", 1, []any{"id:2"}},
		{"from elsewhere, the most urgent", 7, []any{"goto api", "id:2"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newCLI(t, threadsLs(tc.current))
			for _, call := range tc.want {
				if call == "goto api" {
					c.kitten("", "action", "goto_session '/api.kitty-session'")
				} else {
					c.kitten("", "focus-window", "--match", call)
				}
			}

			_, _, err := c.run("agent", "next")

			require.NoError(t, err)
		})
	}

	t.Run("nothing needs you", func(t *testing.T) {
		c := newCLI(t, plainLs())

		_, stderr, err := c.run("agent", "next")

		require.NoError(t, err)
		assert.Equal(t, "no agent needs you\n", stderr)
	})
}

func TestAgentLast(t *testing.T) {
	t.Run("goes to the agent focused last before this one", func(t *testing.T) {
		c := newCLI(t, threadsLs(1))
		c.kitten("", "focus-window", "--match", "id:2")

		_, _, err := c.run("agent", "last")

		require.NoError(t, err)
	})

	t.Run("no other agent", func(t *testing.T) {
		c := newCLI(t, kittyLs(tab{id: 10, active: true, wins: []win{
			{id: 1, session: "api", active: true, program: "claude"},
		}}))

		_, stderr, err := c.run("agent", "last")

		require.NoError(t, err)
		assert.Equal(t, "no other agent\n", stderr)
	})
}

func TestAgentList(t *testing.T) {
	ids := func(t *testing.T, stdout string) []string {
		t.Helper()
		var got []string
		for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
			id, status, _ := strings.Cut(line, "\t")
			status, _, _ = strings.Cut(status, "\t")
			got = append(got, id+" "+status)
		}
		return got
	}

	t.Run("with ids, the ones needing you first", func(t *testing.T) {
		c := newCLI(t, threadsLs(1))

		stdout, _, err := c.run("agent", "list", "--ids", "--no-color")

		require.NoError(t, err)
		assert.Equal(t, []string{"2 waiting", "3 error", "5 done", "4 done", "1 done", "7 working", "6 idle"}, ids(t, stdout))
	})

	t.Run("only those needing you", func(t *testing.T) {
		c := newCLI(t, threadsLs(1))

		stdout, _, err := c.run("agent", "list", "-A", "--ids")

		require.NoError(t, err)
		assert.Equal(t, []string{"2 waiting", "3 error", "5 done", "4 done", "1 done"}, ids(t, stdout))
	})

	t.Run("as json", func(t *testing.T) {
		c := newCLI(t, threadsLs(1))

		stdout, _, err := c.run("agent", "list", "--json", "--attention")

		require.NoError(t, err)
		var agents []model.Agent
		require.NoError(t, json.Unmarshal([]byte(stdout), &agents))
		require.Len(t, agents, 5)
		assert.Equal(t, "Allow Bash", agents[0].Message)
	})

	t.Run("as json, none is an empty list", func(t *testing.T) {
		c := newCLI(t, plainLs())

		stdout, _, err := c.run("agent", "list", "-j")

		require.NoError(t, err)
		assert.Equal(t, "[]\n", stdout)
	})
}

func TestAgentFocus(t *testing.T) {
	t.Run("takes a line of agent list --ids", func(t *testing.T) {
		c := newCLI(t, threadsLs(1))
		c.kitten("", "action", "goto_session '/web.kitty-session'")
		c.kitten("", "focus-window", "--match", "id:3")

		_, _, err := c.run("agent", "focus", "3\terror\tweb › Lint")

		require.NoError(t, err)
	})

	t.Run("refuses what isn't a window before asking kitty", func(t *testing.T) {
		c := newCLI(t, "")

		_, _, err := c.run("agent", "focus", "lint")

		assert.EqualError(t, err, `not a window id: "lint"`)
	})
}

func TestAgentSet(t *testing.T) {
	statusAt := mock.MatchedBy(func(s string) bool { return strings.HasPrefix(s, "ksesh_status_at=") })

	t.Run("a status, on another window", func(t *testing.T) {
		c := newCLI(t, threadsLs(1))
		c.kitten("", "set-user-vars", "--match", "id:6", "ksesh_agent=aider", "ksesh_msg=all tests pass", statusAt, "ksesh_status=done")

		_, _, err := c.run("agent", "set", "done", "-a", "aider", "-m", "all tests pass", "-w", "6")

		require.NoError(t, err)
	})

	t.Run("clear", func(t *testing.T) {
		c := newCLI(t, threadsLs(1))
		c.kitten("", "set-user-vars", "--match", "id:3",
			"ksesh_agent", "ksesh_status", "ksesh_status_at", "ksesh_msg", "ksesh_prompt", "ksesh_turn_at", "ksesh_conv",
			"ksesh_pin", "ksesh_snooze_until", "ksesh_snoozed_at", "ksesh_settled_at", "ksesh_unsettled", "ksesh_woke")

		_, _, err := c.run("agent", "set", "clear", "--window", "3")

		require.NoError(t, err)
	})

	t.Run("an unknown status", func(t *testing.T) {
		c := newCLI(t, threadsLs(1))

		_, _, err := c.run("agent", "set", "thinking")

		assert.EqualError(t, err, `unknown status "thinking"`)
	})
}

func TestAgentSend(t *testing.T) {
	for _, tc := range []struct {
		name  string
		args  []string
		calls [][]any
		err   string
	}{
		{"types a reply and presses enter", []string{"2", "use", "the", `C:\helper`}, [][]any{
			{"send-text", "--match", "id:2", "--", `use the C:\\helper\r`},
		}, ""},
		{"presses keys when the status allows", []string{"2", "--key", "enter", "--if-status", "waiting,error"}, [][]any{
			{"send-key", "--match", "id:2", "enter"},
		}, ""},
		{"types, then presses keys", []string{"3", "retry", "-k", "escape", "-k", "enter"}, [][]any{
			{"send-text", "--match", "id:3", "--", `retry\r`},
			{"send-key", "--match", "id:3", "escape", "enter"},
		}, ""},
		{"not while the agent is in another status", []string{"3", "--key", "enter", "--if-status", "waiting"}, nil,
			"the agent is error, not waiting"},
		{"nothing to send", []string{"2"}, nil, "nothing to send: give text or --key"},
		{"not an agent", []string{"8", "hi"}, nil, "no agent in kitty window 8"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newCLI(t, threadsLs(1))
			for _, call := range tc.calls {
				c.kitten("", call...)
			}

			_, _, err := c.run(append([]string{"agent", "send"}, tc.args...)...)

			if tc.err != "" {
				assert.EqualError(t, err, tc.err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestAgentPath(t *testing.T) {
	c := newCLI(t, threadsLs(1))

	stdout, _, err := c.run("agent", "path", "3")

	require.NoError(t, err)
	assert.Equal(t, "/c/web\n", stdout)
}

func TestAgentShow(t *testing.T) {
	t.Run("the header, then the screen without its trailing blank lines", func(t *testing.T) {
		c := newCLI(t, threadsLs(1))
		c.sh.EXPECT().Cmd("git", "-C", "/c/api", "rev-parse", "--abbrev-ref", "HEAD").Return("feat/migrations", nil)
		c.sh.EXPECT().Cmd("git", "-C", "/c/api", "status", "--porcelain").Return(" M db.go\n?? new.sql\n", nil)
		c.kitten("Allow Bash?\n  1. Yes\n\n   \n\n", "get-text", "--ansi", "--match", "id:2")

		stdout, _, err := c.run("agent", "show", "2")

		require.NoError(t, err)
		lines := strings.Split(stdout, "\n")
		require.Len(t, lines, ShowHeaderLines+3, "the header, the screen's two lines and the final newline")
		assert.Contains(t, lines[0], "waiting")
		assert.Contains(t, lines[0], "· 5m · api › Migrations · claude")
		assert.Contains(t, lines[1], "/c/api  feat/migrations")
		assert.Equal(t, "Allow Bash", lines[2])
		assert.Equal(t, []string{"Allow Bash?", "  1. Yes", ""}, lines[ShowHeaderLines:])
	})

	t.Run("outside a repo, the path alone; a working agent shows its prompt", func(t *testing.T) {
		c := newCLI(t, kittyLs(tab{id: 10, active: true, wins: []win{
			{id: 1, session: "api", cwd: "/c/api", active: true, vars: map[string]string{
				"ksesh_agent": "claude", "ksesh_status": "working", "ksesh_prompt": "add a test", "ksesh_msg": "Bash"}},
		}}))
		c.sh.EXPECT().Cmd("git", "-C", "/c/api", "rev-parse", "--abbrev-ref", "HEAD").Return("", errors.New("not a git repository"))
		c.kitten("$ go test\n", "get-text", "--ansi", "--match", "id:1")

		stdout, _, err := c.run("agent", "show", "1")

		require.NoError(t, err)
		lines := strings.Split(stdout, "\n")
		assert.Equal(t, "\033[2m/c/api\033[22m", lines[1])
		assert.Equal(t, "› add a test", lines[2], "what it works on, not its last message")
	})
}

func TestAgentHookCommand(t *testing.T) {
	stop := `{"hook_event_name":"Stop","session_id":"conv-new","cwd":"/c/api"}`

	t.Run("outside kitty it does nothing", func(t *testing.T) {
		c := newCLI(t, "")

		_, _, err := c.run("agent", "hook", stop)

		require.NoError(t, err)
		_, ok := c.store().Get("conv-new")
		assert.False(t, ok)
	})

	t.Run("without listen_on it does nothing: it would write into the agent's terminal", func(t *testing.T) {
		c := newCLI(t, "")
		t.Setenv("KITTY_WINDOW_ID", "1")

		_, _, err := c.run("agent", "hook", stop)

		require.NoError(t, err)
	})

	t.Run("records the event on the agent's window and in the history", func(t *testing.T) {
		c := newCLI(t, "")
		t.Setenv("KITTY_WINDOW_ID", "1")
		t.Setenv("KITTY_LISTEN_ON", "unix:/nonexistent/kitty.sock")
		t.Setenv("KSESH_HOST", "box")
		c.kitten(`[{"id":1,"tabs":[{"id":10,"windows":[{"id":1,"title":"✳ Fix the login","session_name":"api","user_vars":{}}]}]}]`,
			"ls", "--match", "id:1")
		c.kitten("", "set-user-vars", "--match", "id:1", "ksesh_conv=conv-new", "ksesh_host=box", "ksesh_cwd=/c/api", "ksesh_agent=claude", "ksesh_msg",
			mock.MatchedBy(func(s string) bool { return strings.HasPrefix(s, "ksesh_status_at=") }), "ksesh_status=done")
		_, stderr, err := c.run("agent", "hook", stop)

		require.NoError(t, err)
		assert.Empty(t, stderr)
		rec, ok := c.store().Get("conv-new")
		require.True(t, ok, "kitty or not, the history is kept")
		assert.Equal(t, "done", rec.Status)
		assert.Equal(t, "/c/api", rec.Cwd)
		assert.Equal(t, "Fix the login", rec.Title, "the window's title, read for the history")
		assert.Equal(t, 1, rec.WindowID)
		assert.WithinDuration(t, time.Now(), rec.StatusAt, time.Minute)
	})
}
