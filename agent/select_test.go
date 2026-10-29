package agent

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xheisenbugx/ksesh/model"
)

func at(minutesAgo int) *time.Time {
	t := time.Unix(1_700_000_000, 0).Add(-time.Duration(minutesAgo) * time.Minute)
	return &t
}

func agents() []*model.Agent {
	return []*model.Agent{
		{WindowID: 1, Status: "working", Since: at(1), LastFocused: 10},
		{WindowID: 2, Status: "done", Since: at(5), LastFocused: 30},
		{WindowID: 3, Status: "waiting", Since: at(2), LastFocused: 20},
		{WindowID: 4, Status: "waiting", Since: at(9), LastFocused: 5},
		{WindowID: 5, Status: "error", Since: at(1), LastFocused: 1},
		{WindowID: 6, Status: "idle", LastFocused: 40},
	}
}

func ids(list []*model.Agent) []int {
	var out []int
	for _, a := range list {
		out = append(out, a.WindowID)
	}
	return out
}

func TestSortMostUrgentAndLongestWaitingFirst(t *testing.T) {
	list := agents()
	Sort(list)
	assert.Equal(t, []int{4, 3, 5, 2, 1, 6}, ids(list))
}

func TestNextCyclesThroughWhatNeedsAttention(t *testing.T) {
	list := agents()

	next, ok := Next(list, 1)
	require.True(t, ok)
	assert.Equal(t, 4, next.WindowID, "from elsewhere, the longest waiting")

	order := []int{}
	current := 4
	for range 4 {
		order = append(order, current)
		next, ok := Next(list, current)
		require.True(t, ok)
		current = next.WindowID
	}
	assert.Equal(t, []int{4, 3, 5, 2}, order)
	assert.Equal(t, 4, current, "and back to the start")
}

func TestNextWithNothingToDo(t *testing.T) {
	_, ok := Next([]*model.Agent{{WindowID: 1, Status: "working"}}, 0)
	assert.False(t, ok)

	_, ok = Next([]*model.Agent{{WindowID: 1, Status: "done"}}, 1)
	assert.False(t, ok, "the only one is the one you're at")
}

func TestLastIsTheMostRecentlyFocusedOtherAgent(t *testing.T) {
	list := agents()
	last, ok := Last(list, 6)
	require.True(t, ok)
	assert.Equal(t, 2, last.WindowID)

	list[1].Focused = true
	last, ok = Last(list, 0)
	require.True(t, ok)
	assert.Equal(t, 6, last.WindowID, "the focused window is skipped even without its id")
}

func TestStatusPriority(t *testing.T) {
	assert.Equal(t, Waiting, MostUrgent(Done, Waiting))
	assert.Equal(t, Error, MostUrgent(Error, Working))
	assert.Equal(t, Idle, MostUrgent("", Idle))
	assert.True(t, Done.NeedsAttention())
	assert.False(t, Working.NeedsAttention())
	_, ok := Parse("busy")
	assert.False(t, ok)
}

func TestCleanTitle(t *testing.T) {
	assert.Equal(t, "Claude Code", CleanTitle("✳ Claude Code"))
	assert.Equal(t, "Fix the build", CleanTitle("⠂ Fix the build"))
	assert.Equal(t, "~/c/api", CleanTitle("~/c/api"), "a path keeps its tilde")
	assert.Equal(t, "✳", CleanTitle("✳"), "nothing left means nothing to clean")
}

func TestSortGroupsAgentsWithoutTimesBySession(t *testing.T) {
	list := []*model.Agent{
		{WindowID: 1, Status: "idle", Session: "web", TabTitle: "b"},
		{WindowID: 2, Status: "idle", Session: "api", TabTitle: "✳ z"},
		{WindowID: 3, Status: "idle", Session: "api", TabTitle: "a"},
	}
	Sort(list)
	assert.Equal(t, []int{3, 2, 1}, ids(list))
}
