package agent

import (
	"cmp"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xheisenbugx/ksesh/model"
)

func closed(conv string, ago time.Duration, t model.Thread) *model.Agent {
	a := thread(0, Working, ago, t)
	a.Conversation, a.Closed = conv, true
	return a
}

func TestAClosedThreadIsSettled(t *testing.T) {
	a := closed("c", time.Minute, model.Thread{Pin: 1, SnoozedUntil: now.Add(time.Hour)})
	assert.Equal(t, Settled, SectionOf(a, now, opts), "even working, pinned or snoozed when its window went")
	assert.Equal(t, Settled, SectionOf(a, now, ThreadOpts{}), "with auto-settling off too")
	at, ok := SettledSince(a, now, opts)
	require.True(t, ok)
	assert.Equal(t, now.Add(-time.Minute), at, "as of its last activity")

	a.Thread.SettledAt = now.Add(-time.Second)
	at, _ = SettledSince(a, now, opts)
	assert.Equal(t, now.Add(-time.Second), at, "or settled by hand since")
	assert.Equal(t, BadgeClosed, BadgeOf(a, now))
}

func TestClosedThreadsSortWithTheSettledByLastActivity(t *testing.T) {
	board := Arrange([]*model.Agent{
		thread(1, Done, time.Minute, model.Thread{SettledAt: now.Add(-3 * time.Hour)}),
		closed("b", 2*time.Hour, model.Thread{}),
		closed("a", time.Hour, model.Thread{}),
		thread(2, Idle, time.Minute, model.Thread{}),
	}, now, opts)

	var order []string
	for _, a := range board[Settled] {
		order = append(order, cmp.Or(a.Conversation, strconv.Itoa(a.WindowID)))
	}
	assert.Equal(t, []string{"a", "b", "1"}, order)
	assert.Len(t, board[Active], 1)
}

func TestAClosedThreadCantBeTriaged(t *testing.T) {
	a := closed("c", time.Minute, model.Thread{})
	_, err := ToggleSettle(a, now, opts)
	assert.Error(t, err)
	_, err = Snooze(a, now, now.Add(time.Hour))
	assert.Error(t, err)
}

func TestResume(t *testing.T) {
	assert.Equal(t, model.Thread{Pin: 3, Unsettled: true}, Resume(model.Thread{Pin: 3, SettledAt: now, Woke: true}),
		"out of Settled, and not settling again for being old: you came back to it")
	assert.Equal(t, model.Thread{}, Resume(model.Thread{SnoozedUntil: now.Add(time.Hour), SnoozedAt: now}))
}

func TestWithClosed(t *testing.T) {
	live := thread(1, Idle, time.Minute, model.Thread{})
	live.Conversation = "open"
	all := WithClosed([]*model.Agent{live}, []model.ThreadRecord{
		{Conversation: "open", Title: "stale"},
		{Conversation: "gone", Agent: "claude", Title: "Write docs", Cwd: "/c/docs", Session: "docs", Status: "done",
			StatusAt: now.Add(-time.Hour), Thread: model.Thread{Pin: 2}},
	})

	require.Len(t, all, 2, "a conversation open in a window isn't closed")
	assert.Same(t, live, all[0])
	gone := all[1]
	assert.True(t, gone.Closed)
	assert.Equal(t, 0, gone.WindowID)
	assert.Equal(t, "Write docs", gone.Title)
	assert.Equal(t, "/c/docs", gone.Path)
	assert.Equal(t, "claude", gone.Name)
	assert.Equal(t, now.Add(-time.Hour), *gone.Since)
	assert.Equal(t, model.Thread{Pin: 2}, gone.Thread)
}

func TestThreadTitle(t *testing.T) {
	assert.Equal(t, "Fix the login", ThreadTitle("✳ Fix the login", "claude"))
	assert.Equal(t, "", ThreadTitle("✳ Claude Code", "claude"))
	assert.Equal(t, "", ThreadTitle("claude", "claude"))
	assert.Equal(t, "", ThreadTitle("  ", "claude"))
}

func TestResumeCommand(t *testing.T) {
	cmd, ok := ResumeCommand("claude", "0b5e-41")
	assert.True(t, ok)
	assert.Equal(t, "claude --resume 0b5e-41", cmd)
	cmd, _ = ResumeCommand("claude", "it's")
	assert.Equal(t, `claude --resume 'it'\''s'`, cmd)
	cmd, _ = ResumeCommand("codex", "019a-77")
	assert.Equal(t, "codex resume 019a-77", cmd)
	cmd, _ = ResumeCommand("gemini", "5f2c-11")
	assert.Equal(t, "gemini --resume 5f2c-11", cmd)
	_, ok = ResumeCommand("aider", "x")
	assert.False(t, ok)
	_, ok = ResumeCommand("claude", "")
	assert.False(t, ok)
}
