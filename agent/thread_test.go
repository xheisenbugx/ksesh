package agent

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xheisenbugx/ksesh/model"
)

// now is a Sunday morning.
var now = time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC)

var opts = ThreadOpts{AutoSettleAfter: DefaultAutoSettleAfter}

func thread(id int, status Status, ago time.Duration, t model.Thread) *model.Agent {
	since := now.Add(-ago)
	return &model.Agent{WindowID: id, Status: string(status), Since: &since, Thread: t}
}

func TestSectionOf(t *testing.T) {
	tests := []struct {
		name  string
		agent *model.Agent
		want  Section
	}{
		{"idle goes to the inbox", thread(1, Idle, time.Minute, model.Thread{}), Active},
		{"waiting goes to the inbox", thread(1, Waiting, time.Minute, model.Thread{}), Active},
		{"working goes to its shelf", thread(1, Working, time.Minute, model.Thread{}), SectionWorking},
		{"pinned beats working", thread(1, Working, time.Minute, model.Thread{Pin: 1}), Pinned},
		{"snoozed beats pinned", thread(1, Idle, time.Minute, model.Thread{Pin: 1, SnoozedUntil: now.Add(time.Hour)}), Snoozed},
		{"a snooze that is due is over", thread(1, Idle, time.Minute, model.Thread{SnoozedUntil: now.Add(-time.Second)}), Active},
		{"settled by hand", thread(1, Done, time.Minute, model.Thread{SettledAt: now}), Settled},
		{"settled after a week idle", thread(1, Idle, 8*24*time.Hour, model.Thread{}), Settled},
		{"done for a week settles too", thread(1, Done, 8*24*time.Hour, model.Thread{}), Settled},
		{"not before a week", thread(1, Idle, 6*24*time.Hour, model.Thread{}), Active},
		{"a waiting agent never settles on its own", thread(1, Waiting, 30*24*time.Hour, model.Thread{}), Active},
		{"nor a pinned one", thread(1, Idle, 30*24*time.Hour, model.Thread{Pin: 1}), Pinned},
		{"nor one un-settled by hand", thread(1, Idle, 30*24*time.Hour, model.Thread{Unsettled: true}), Active},
		{"nor one back from a snooze", thread(1, Idle, 30*24*time.Hour, model.Thread{Woke: true}), Active},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, SectionOf(tt.agent, now, opts))
		})
	}
}

func TestAutoSettleCanBeTurnedOff(t *testing.T) {
	a := thread(1, Idle, 300*24*time.Hour, model.Thread{})
	assert.Equal(t, Active, SectionOf(a, now, ThreadOpts{}))
}

func TestAutoSettledSinceLastActivity(t *testing.T) {
	a := thread(1, Idle, 10*24*time.Hour, model.Thread{})
	at, ok := SettledSince(a, now, opts)
	require.True(t, ok)
	assert.Equal(t, now.Add(-10*24*time.Hour), at, "settled as of its last activity, not now")
}

func TestArrange(t *testing.T) {
	agents := []*model.Agent{
		thread(1, Idle, time.Hour, model.Thread{}),
		thread(2, Done, time.Minute, model.Thread{}),
		thread(3, Waiting, 10*time.Minute, model.Thread{}),
		thread(4, Idle, time.Hour, model.Thread{Pin: 20}),
		thread(5, Working, time.Hour, model.Thread{Pin: 10}),
		thread(6, Working, time.Minute, model.Thread{}),
		thread(7, Idle, time.Minute, model.Thread{SnoozedUntil: now.Add(3 * time.Hour)}),
		thread(8, Idle, time.Minute, model.Thread{SnoozedUntil: now.Add(time.Hour)}),
		thread(9, Idle, time.Minute, model.Thread{SettledAt: now.Add(-time.Hour)}),
		thread(10, Idle, time.Minute, model.Thread{SettledAt: now.Add(-time.Minute)}),
	}
	turn := now.Add(-2 * time.Hour)
	agents[5].TurnAt = &turn
	working := thread(11, Working, time.Hour, model.Thread{})
	recent := now.Add(-time.Minute)
	working.TurnAt = &recent
	agents = append(agents, working)

	board := Arrange(agents, now, opts)

	assert.Equal(t, []int{5, 4}, ids(board[Pinned]), "pin order")
	assert.Equal(t, []int{2, 3, 1}, ids(board[Active]), "latest activity first, whatever the status")
	assert.Equal(t, []int{11, 6}, ids(board[SectionWorking]), "latest prompt first")
	assert.Equal(t, []int{8, 7}, ids(board[Snoozed]), "soonest to wake first")
	assert.Equal(t, []int{10, 9}, ids(board[Settled]), "most recently settled first")
	assert.Equal(t, []int{5, 4, 2, 3, 1}, ids(board.Ordered(Pinned, Active)))
}

func TestTogglePin(t *testing.T) {
	a := thread(1, Idle, time.Minute, model.Thread{SettledAt: now, SnoozedUntil: now.Add(time.Hour), Woke: true})

	pinned := TogglePin(a, now, opts, 42)

	assert.Equal(t, model.Thread{Pin: 42, Unsettled: true}, pinned, "pinning un-settles and wakes")
	a.Thread = pinned
	assert.Equal(t, int64(0), TogglePin(a, now, opts, 43).Pin)
}

func TestNextPin(t *testing.T) {
	agents := []*model.Agent{thread(1, Idle, 0, model.Thread{Pin: now.UnixNano() + 5})}
	assert.Equal(t, now.UnixNano()+6, NextPin(agents, now))
	assert.Equal(t, now.UnixNano(), NextPin(nil, now))
}

func TestToggleSettle(t *testing.T) {
	a := thread(1, Done, time.Minute, model.Thread{Pin: 3, SnoozedUntil: now.Add(time.Hour)})

	settled, err := ToggleSettle(a, now, opts)
	require.NoError(t, err)
	assert.Equal(t, model.Thread{SettledAt: now}, settled, "settling unpins and wakes")

	a.Thread = settled
	back, err := ToggleSettle(a, now, opts)
	require.NoError(t, err)
	assert.Equal(t, model.Thread{Unsettled: true}, back)
	assert.Equal(t, Active, SectionOf(&model.Agent{Status: "idle", Since: a.Since, Thread: back}, now.Add(30*24*time.Hour), opts),
		"un-settled by hand, it doesn't settle again for being idle")
}

func TestToggleSettleUnsettlesAnAutoSettledThread(t *testing.T) {
	a := thread(1, Idle, 10*24*time.Hour, model.Thread{})
	back, err := ToggleSettle(a, now, opts)
	require.NoError(t, err)
	assert.True(t, back.Unsettled)
}

func TestToggleSettleRefusesABusyAgent(t *testing.T) {
	for _, s := range []Status{Working, Waiting} {
		_, err := ToggleSettle(thread(1, s, time.Minute, model.Thread{}), now, opts)
		assert.Error(t, err, s)
	}
}

func TestSnoozeAndWake(t *testing.T) {
	a := thread(1, Idle, time.Minute, model.Thread{Pin: 2, SettledAt: now})

	snoozed, err := Snooze(a, now, now.Add(time.Hour))
	require.NoError(t, err)
	assert.Equal(t, model.Thread{Pin: 2, SnoozedUntil: now.Add(time.Hour), SnoozedAt: now, Unsettled: true}, snoozed)

	_, err = Snooze(a, now, now.Add(-time.Minute))
	assert.Error(t, err)

	a.Thread = snoozed
	assert.Equal(t, model.Thread{Pin: 2, Unsettled: true}, Wake(a))
}

func TestOnActivity(t *testing.T) {
	snoozed := model.Thread{Pin: 1, SnoozedUntil: now.Add(time.Hour), SnoozedAt: now.Add(-time.Hour), SettledAt: now}

	t.Run("a prompt un-settles and ends the snooze", func(t *testing.T) {
		got := OnActivity(model.Thread{Pin: 1, SettledAt: now, Unsettled: true, Woke: true, SnoozedUntil: now.Add(time.Hour)}, Update{Status: Working, StartTurn: true}, now)
		assert.Equal(t, model.Thread{Pin: 1}, got)
	})
	t.Run("waiting wakes a snoozed thread early", func(t *testing.T) {
		got := OnActivity(snoozed, Update{Status: Waiting}, now)
		assert.Equal(t, model.Thread{Pin: 1, Woke: true}, got)
	})
	t.Run("so does failing", func(t *testing.T) {
		assert.True(t, OnActivity(snoozed, Update{Status: Error}, now).Woke)
	})
	t.Run("finishing doesn't", func(t *testing.T) {
		assert.Equal(t, snoozed, OnActivity(snoozed, Update{Status: Done}, now))
	})
	t.Run("nor a tool call", func(t *testing.T) {
		assert.Equal(t, snoozed, OnActivity(snoozed, Update{Status: Working}, now))
	})
	t.Run("waiting on a thread that isn't snoozed doesn't mark it woken", func(t *testing.T) {
		assert.False(t, OnActivity(model.Thread{}, Update{Status: Waiting}, now).Woke)
	})
	t.Run("the agent leaving clears it all", func(t *testing.T) {
		assert.Equal(t, model.Thread{}, OnActivity(snoozed, Update{Clear: true}, now))
	})
}

func TestBadgeOf(t *testing.T) {
	assert.Equal(t, Badge(Waiting), BadgeOf(thread(1, Waiting, 0, model.Thread{Woke: true}), now), "what blocks the agent comes first")
	assert.Equal(t, BadgeWoke, BadgeOf(thread(1, Done, 0, model.Thread{Woke: true}), now))
	assert.Equal(t, BadgeWoke, BadgeOf(thread(1, Idle, 0, model.Thread{SnoozedUntil: now.Add(-time.Minute)}), now), "its time came")
	assert.Equal(t, Badge(Idle), BadgeOf(thread(1, Idle, 0, model.Thread{SnoozedUntil: now.Add(time.Minute)}), now), "still snoozed")
	assert.Equal(t, Badge(Done), BadgeOf(thread(1, Done, 0, model.Thread{}), now))
}

func TestSnoozePresets(t *testing.T) {
	keys := func(presets []SnoozePreset) []string {
		var out []string
		for _, p := range presets {
			out = append(out, p.Key)
		}
		return out
	}
	presets := SnoozePresets(now)
	assert.Equal(t, []string{"1h", "3h", "evening", "tomorrow", "next-week"}, keys(presets))
	assert.Equal(t, time.Date(2026, 11, 8, 18, 0, 0, 0, time.UTC), presets[2].Until)
	assert.Equal(t, time.Date(2026, 11, 9, 9, 0, 0, 0, time.UTC), presets[3].Until)
	assert.Equal(t, time.Date(2026, 11, 9, 9, 0, 0, 0, time.UTC), presets[4].Until, "next Monday is tomorrow on a Sunday")

	late := time.Date(2026, 11, 9, 16, 0, 0, 0, time.UTC) // a Monday
	presets = SnoozePresets(late)
	assert.Equal(t, []string{"1h", "3h", "tomorrow", "next-week"}, keys(presets), "no evening once it is nearly evening")
	assert.Equal(t, time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC), presets[3].Until, "a Monday's next week is a week away")
}

func TestParseSnooze(t *testing.T) {
	tests := map[string]time.Time{
		"3h":               now.Add(3 * time.Hour),
		"90m":              now.Add(90 * time.Minute),
		"2d":               now.AddDate(0, 0, 2),
		"evening":          time.Date(2026, 11, 8, 18, 0, 0, 0, time.UTC),
		"tomorrow":         time.Date(2026, 11, 9, 9, 0, 0, 0, time.UTC),
		"14:30":            time.Date(2026, 11, 8, 14, 30, 0, 0, time.UTC),
		"09:00":            time.Date(2026, 11, 9, 9, 0, 0, 0, time.UTC),
		"2026-12-01 08:15": time.Date(2026, 12, 1, 8, 15, 0, 0, time.UTC),
		"2026-12-01":       time.Date(2026, 12, 1, 9, 0, 0, 0, time.UTC),
	}
	for in, want := range tests {
		got, err := ParseSnooze(in, now)
		require.NoError(t, err, in)
		assert.Equal(t, want, got, in)
	}
	for _, bad := range []string{"", "soon", "-1h", "0d"} {
		_, err := ParseSnooze(bad, now)
		assert.Error(t, err, bad)
	}
}
