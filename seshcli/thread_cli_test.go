package seshcli

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/xheisenbugx/ksesh/model"
)

// unixArg matches a NAME=<unix seconds> assignment within slack of want.
func unixArg(name string, want time.Time, slack time.Duration) any {
	return mock.MatchedBy(func(s string) bool {
		v, ok := strings.CutPrefix(s, name+"=")
		if !ok {
			return false
		}
		at, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return false
		}
		d := time.Unix(at, 0).Sub(want)
		return d >= -slack && d <= slack
	})
}

// setThread expects the set-user-vars writing a thread onto window id: the
// pin, snooze until, snoozed at, settled at, unsettled and woke variables.
func (c *cli) setThread(id int, vars ...any) {
	c.kitten("", append([]any{"set-user-vars", "--match", "id:" + strconv.Itoa(id)}, vars...)...)
}

func TestThreadCommands(t *testing.T) {
	now := time.Now()
	for _, tc := range []struct {
		name   string
		args   []string
		window int
		vars   []any
		out    string
	}{
		{"pin pins at the bottom of the pinned", []string{"pin", "2"}, 2,
			[]any{mock.MatchedBy(func(s string) bool {
				pin, err := strconv.ParseInt(strings.TrimPrefix(s, "ksesh_pin="), 10, 64)
				return strings.HasPrefix(s, "ksesh_pin=") && err == nil && pin >= now.UnixNano()
			}), "ksesh_snooze_until", "ksesh_snoozed_at", "ksesh_settled_at", "ksesh_unsettled", "ksesh_woke"},
			"pinned Migrations\n"},
		{"pin takes a line of agent list --ids", []string{"pin", "2\twaiting\tapi › Migrations"}, 2,
			[]any{mock.AnythingOfType("string"), "ksesh_snooze_until", "ksesh_snoozed_at", "ksesh_settled_at", "ksesh_unsettled", "ksesh_woke"},
			"pinned Migrations\n"},
		{"pin on a pinned thread unpins it", []string{"pin", "6"}, 6,
			[]any{"ksesh_pin", "ksesh_snooze_until", "ksesh_snoozed_at", "ksesh_settled_at", "ksesh_unsettled", "ksesh_woke"},
			"unpinned Keep\n"},
		{"pin on a settled thread takes it out of Settled for good", []string{"pin", "5"}, 5,
			[]any{mock.AnythingOfType("string"), "ksesh_snooze_until", "ksesh_snoozed_at", "ksesh_settled_at", "ksesh_unsettled=1", "ksesh_woke"},
			"pinned Old\n"},
		{"settle shelves a finished thread", []string{"settle", "3"}, 3,
			[]any{"ksesh_pin", "ksesh_snooze_until", "ksesh_snoozed_at", unixArg("ksesh_settled_at", now, 5*time.Second), "ksesh_unsettled", "ksesh_woke"},
			"settled Lint\n"},
		{"settle on a settled thread takes it back to the inbox", []string{"settle", "5"}, 5,
			[]any{"ksesh_pin", "ksesh_snooze_until", "ksesh_snoozed_at", "ksesh_settled_at", "ksesh_unsettled=1", "ksesh_woke"},
			"un-settled Old\n"},
		{"snooze hides it until when, the window last", []string{"snooze", "2h", "3"}, 3,
			[]any{"ksesh_pin", unixArg("ksesh_snooze_until", now.Add(2*time.Hour), 5*time.Second),
				unixArg("ksesh_snoozed_at", now, 5*time.Second), "ksesh_settled_at", "ksesh_unsettled=1", "ksesh_woke"},
			"snoozed until " + now.Add(2*time.Hour).Format("Mon 15:04") + ": Lint\n"},
		{"snooze keeps a pin", []string{"snooze", "1d", "6"}, 6,
			[]any{"ksesh_pin=1", unixArg("ksesh_snooze_until", now.AddDate(0, 0, 1), 5*time.Second),
				unixArg("ksesh_snoozed_at", now, 5*time.Second), "ksesh_settled_at", "ksesh_unsettled=1", "ksesh_woke"},
			"snoozed until " + now.AddDate(0, 0, 1).Format("Mon 15:04") + ": Keep\n"},
		{"wake brings a snoozed thread back", []string{"wake", "4"}, 4,
			[]any{"ksesh_pin", "ksesh_snooze_until", "ksesh_snoozed_at", "ksesh_settled_at", "ksesh_unsettled", "ksesh_woke"},
			"woke Later\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newCLI(t, threadsLs(1))
			c.setThread(tc.window, tc.vars...)

			stdout, _, err := c.run(append([]string{"agent"}, tc.args...)...)

			require.NoError(t, err)
			if strings.HasPrefix(tc.args[0], "snooze") {
				// The minute can turn between the test's clock and ksesh's.
				assert.Regexp(t, regexp.MustCompile(`^snoozed until \w{3} \d\d:\d\d: `+strings.Fields(tc.out)[4]), stdout)
			} else {
				assert.Equal(t, tc.out, stdout)
			}
		})
	}
}

func TestThreadCommandsActOnYourWindowAndKeepTheHistory(t *testing.T) {
	c := newCLI(t, threadsLs(1))
	closedRecord(t, c, "conv-1", func(r *model.ThreadRecord) { r.WindowID = 1 })
	c.setThread(1, mock.AnythingOfType("string"), "ksesh_snooze_until", "ksesh_snoozed_at", "ksesh_settled_at", "ksesh_unsettled", "ksesh_woke")

	stdout, _, err := c.run("agent", "pin")

	require.NoError(t, err)
	assert.Equal(t, "pinned Fix the login\n", stdout)
	rec, ok := c.store().Get("conv-1")
	require.True(t, ok)
	assert.NotZero(t, rec.Thread.Pin, "the pin outlives the window, to come back with a resume")
}

func TestThreadCommandsRefuse(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		err  string
	}{
		{"snooze without a time", []string{"snooze"}, "snooze until when? e.g. 1h, tomorrow, 15:04"},
		{"two windows", []string{"pin", "1", "2"}, "takes at most one window id"},
		{"a window without an agent", []string{"settle", "8"}, "no agent in kitty window 8"},
		{"a window that isn't one", []string{"wake", "x"}, `not a window id: "x"`},
		{"settling a working agent", []string{"settle", "7"}, "the agent is still working"},
		{"settling an agent waiting on you", []string{"settle", "2"}, "the agent is waiting on you"},
		{"snoozing until a time that has passed", []string{"snooze", "2000-01-01 09:00", "3"}, "can't snooze until Sat 09:00, it has passed"},
		{"the sidebar", []string{"pin", "9"}, "no agent in kitty window 9"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newCLI(t, threadsLs(1))

			_, _, err := c.run(append([]string{"agent"}, tc.args...)...)

			assert.EqualError(t, err, tc.err, "and nothing is written")
		})
	}
}

func TestThreadCommandsReportWhatKittyRefused(t *testing.T) {
	c := newCLI(t, threadsLs(1))
	c.sh.EXPECT().CmdCapture("kitten", "@", "set-user-vars", "--match", "id:4",
		"ksesh_pin", "ksesh_snooze_until", "ksesh_snoozed_at", "ksesh_settled_at", "ksesh_unsettled", "ksesh_woke").Return("", assert.AnError)

	stdout, _, err := c.run("agent", "wake", "4")

	assert.ErrorIs(t, err, assert.AnError)
	assert.Empty(t, stdout)
}

func TestResume(t *testing.T) {
	t.Run("opens a tab in the thread's session running the agent on its conversation", func(t *testing.T) {
		c := newCLI(t, threadsLs(1))
		dir := t.TempDir()
		closedRecord(t, c, "conv-gone", func(r *model.ThreadRecord) { r.Cwd, r.Session = dir, "web" })
		c.kitten("42\n", "launch", "--type=tab", "--add-to-session", "web", "--match", "id:3", "--cwd", dir, "--var", "ksesh_root=/c/web")
		c.kitten("", "send-text", "--match", "id:42", "--", `claude --resume conv-gone\r`)
		c.kitten("", "action", "goto_session '/web.kitty-session'")

		_, _, err := c.run("agent", "resume", "conv-g")

		require.NoError(t, err)
	})

	t.Run("starts the session again when it is gone", func(t *testing.T) {
		c := newCLI(t, "")
		dir := t.TempDir()
		closedRecord(t, c, "conv-gone", func(r *model.ThreadRecord) { r.Cwd, r.Session = dir, "docs" })
		c.sh.EXPECT().CmdCapture("kitten", "@", "ls").Return(threadsLs(1), nil).Once()
		c.kitten("42\n", "launch", "--type=tab", "--keep-focus", "--add-to-session", "docs", "--cwd", dir, "--var", "ksesh_root="+dir)
		c.kitten("", "send-text", "--match", "id:42", "--", `claude --resume conv-gone\r`)
		// kitty, once the session is started again.
		c.sh.EXPECT().CmdCapture("kitten", "@", "ls").Return(kittyLs(tab{id: 12, active: true, wins: []win{
			{id: 42, session: "docs", cwd: dir, active: true, vars: map[string]string{"ksesh_root": dir}},
		}}), nil)
		c.kitten("", "action", "goto_session '/docs.kitty-session'")

		_, _, err := c.run("agent", "resume", "conv-gone")

		require.NoError(t, err)
	})

	t.Run("a thread still open is gone to instead", func(t *testing.T) {
		c := newCLI(t, threadsLs(2))
		c.kitten("", "focus-window", "--match", "id:1")

		_, _, err := c.run("agent", "resume", "conv-1")

		require.NoError(t, err)
	})

	for _, tc := range []struct {
		name    string
		records []string
		arg     string
		err     string
	}{
		{"no such thread", []string{"conv-a"}, "conv-b", "no closed thread conv-b (see 'ksesh sidebar print')"},
		{"a prefix two threads share", []string{"conv-a1", "conv-a2"}, "conv-a", "2 closed threads start with conv-a"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newCLI(t, threadsLs(1))
			for _, conv := range tc.records {
				closedRecord(t, c, conv, nil)
			}

			_, _, err := c.run("agent", "resume", tc.arg)

			assert.EqualError(t, err, tc.err)
		})
	}

	t.Run("a directory that is gone", func(t *testing.T) {
		c := newCLI(t, threadsLs(1))
		closedRecord(t, c, "conv-gone", func(r *model.ThreadRecord) { r.Cwd = "/nonexistent/ksesh-test" })

		_, _, err := c.run("agent", "resume", "conv-gone")

		assert.EqualError(t, err, "the thread's directory is gone: /nonexistent/ksesh-test")
	})
}

func TestForget(t *testing.T) {
	c := newCLI(t, threadsLs(1))
	closedRecord(t, c, "conv-gone", func(r *model.ThreadRecord) { r.Title = "Old refactor" })
	closedRecord(t, c, "conv-kept", nil)

	stdout, _, err := c.run("agent", "forget", "conv-g")

	require.NoError(t, err)
	assert.Equal(t, "forgot Old refactor\n", stdout)
	_, ok := c.store().Get("conv-gone")
	assert.False(t, ok)
	_, ok = c.store().Get("conv-kept")
	assert.True(t, ok)
}

func TestForgetLeavesAnOpenThreadAlone(t *testing.T) {
	t.Skip("bug: closedThread (seshcli/sidebar.go) matches a conversation exactly whether or not it is closed, " +
		"which resume relies on to go to an open thread; forget uses it too, so 'ksesh agent forget <open conversation>' " +
		"deletes the history of a thread still open in a window (its pin, title and status outlive the window no more) " +
		"and prints \"forgot …\" for a thread that isn't closed")
	c := newCLI(t, threadsLs(2))
	closedRecord(t, c, "conv-1", func(r *model.ThreadRecord) { r.WindowID = 1 })

	_, _, err := c.run("agent", "forget", "conv-1")

	assert.Error(t, err, "conv-1 is open in window 1")
	_, ok := c.store().Get("conv-1")
	assert.True(t, ok, "its record is kept")
}

func TestPrintNamesAClosedThreadWithoutATitle(t *testing.T) {
	t.Skip("bug: threadLabel falls back to agentWhere (seshcli/agent_view.go), which names a thread without a title, " +
		"custom title or session by its window: a closed thread has none, so 'ksesh sidebar print' (and 'agent forget') " +
		"call it \"window 0\"; its directory or conversation would say which thread it is")
	c := newCLI(t, plainLs())
	closedRecord(t, c, "conv-untitled", func(r *model.ThreadRecord) { r.Cwd = "/c/api" })

	stdout, _, err := c.run("sidebar", "print")

	require.NoError(t, err)
	assert.NotContains(t, stdout, "window 0")
}
