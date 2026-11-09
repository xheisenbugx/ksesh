package threads

import (
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xheisenbugx/ksesh/agent"
	"github.com/xheisenbugx/ksesh/model"
)

var now = time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC)

func TestUpdateCreatesAndChangesARecord(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "threads")
	s := NewStore(dir)

	require.NoError(t, s.Update("abc-1", func(r *model.ThreadRecord) { r.Agent, r.Cwd = "claude", "/c/api" }))
	require.NoError(t, s.Update("abc-1", func(r *model.ThreadRecord) { r.Title = "Fix the login" }))

	rec, ok := s.Get("abc-1")
	require.True(t, ok)
	assert.Equal(t, model.ThreadRecord{Conversation: "abc-1", Agent: "claude", Cwd: "/c/api", Title: "Fix the login"}, rec)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	assert.ElementsMatch(t, []string{".lock", "abc-1.json"}, names, "no temporary file is left behind")
}

func TestUpdateLeavesAnUnchangedRecordAlone(t *testing.T) {
	s := NewStore(t.TempDir())
	require.NoError(t, s.Update("abc", func(r *model.ThreadRecord) { r.Title = "x" }))
	path := filepath.Join(s.dir, "abc.json")
	old := time.Now().Add(-time.Hour)
	require.NoError(t, os.Chtimes(path, old, old))

	require.NoError(t, s.Update("abc", func(r *model.ThreadRecord) { r.Title = "x" }))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.True(t, info.ModTime().Equal(old), "not rewritten")
}

func TestConcurrentUpdatesDontLoseEachOther(t *testing.T) {
	s := NewStore(t.TempDir())
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			// Each one is its own store, as each hook is its own process.
			assert.NoError(t, NewStore(s.dir).Update("abc", func(r *model.ThreadRecord) {
				n, _ := strconv.Atoi(r.Prompt)
				r.Prompt = strconv.Itoa(n + 1)
			}))
		})
	}
	wg.Wait()
	rec, _ := s.Get("abc")
	assert.Equal(t, "20", rec.Prompt)
}

func TestIDsThatCouldLeaveTheDirectoryAreRefused(t *testing.T) {
	s := NewStore(t.TempDir())
	for _, id := range []string{"", "../x", "a/b", ".hidden", "..", "a b"} {
		assert.Error(t, s.Update(id, func(r *model.ThreadRecord) {}), id)
		_, ok := s.Get(id)
		assert.False(t, ok, id)
	}
	_, err := os.Stat(filepath.Join(filepath.Dir(s.dir), "x.json"))
	assert.True(t, os.IsNotExist(err))
}

func TestListSeesChangesAndForgetting(t *testing.T) {
	s := NewStore(t.TempDir())
	records, err := s.List()
	require.NoError(t, err)
	assert.Empty(t, records)

	require.NoError(t, s.Update("a", func(r *model.ThreadRecord) { r.Title = "one" }))
	require.NoError(t, s.Update("b", func(r *model.ThreadRecord) { r.Title = "two" }))
	records, err = s.List()
	require.NoError(t, err)
	assert.Len(t, records, 2)

	require.NoError(t, s.Update("a", func(r *model.ThreadRecord) { r.Title = "one, renamed" }))
	require.NoError(t, s.Forget("b"))
	records, err = s.List()
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.Equal(t, "one, renamed", records[0].Title, "a changed file is read again")

	require.NoError(t, s.Forget("b"), "forgetting twice is fine")
}

func TestListOfAMissingDirectoryIsEmpty(t *testing.T) {
	records, err := NewStore(filepath.Join(t.TempDir(), "nope")).List()
	require.NoError(t, err)
	assert.Empty(t, records)
	assert.NoError(t, NewStore(filepath.Join(t.TempDir(), "nope")).Forget("a"))
}

func TestAStoreWithoutADirKeepsNothing(t *testing.T) {
	s := NewStore("")
	require.NoError(t, s.Update("a", func(r *model.ThreadRecord) { r.Title = "x" }))
	_, ok := s.Get("a")
	assert.False(t, ok)
	_, err := os.Stat("a.json")
	assert.True(t, os.IsNotExist(err), "nothing written to the working directory")
}

func rec(conv string, ago time.Duration) model.ThreadRecord {
	return model.ThreadRecord{Conversation: conv, StatusAt: now.Add(-ago)}
}

func TestExpired(t *testing.T) {
	records := []model.ThreadRecord{
		rec("new", time.Hour),
		rec("older", 2*time.Hour),
		rec("oldest", 3*time.Hour),
		rec("ancient", 40*24*time.Hour),
		rec("live", 90*24*time.Hour),
	}
	open := map[string]bool{"live": true}

	assert.ElementsMatch(t, []string{"ancient"}, Expired(records, open, now, Retention{Keep: 200, MaxAge: DefaultMaxAge}))
	assert.ElementsMatch(t, []string{"oldest", "ancient"}, Expired(records, open, now, Retention{Keep: 2}), "the most recent first")
	assert.ElementsMatch(t, []string{"new", "older", "oldest", "ancient"}, Expired(records, open, now, Retention{}), "keeping none")
}

func TestPrune(t *testing.T) {
	s := NewStore(t.TempDir())
	for _, r := range []model.ThreadRecord{rec("a", time.Hour), rec("b", 2*time.Hour), rec("c", 3*time.Hour)} {
		require.NoError(t, s.Update(r.Conversation, func(x *model.ThreadRecord) { *x = r }))
	}

	n, err := s.Prune(map[string]bool{"c": true}, now, Retention{Keep: 1})

	require.NoError(t, err)
	assert.Equal(t, 1, n)
	_, ok := s.Get("b")
	assert.False(t, ok)
	_, ok = s.Get("c")
	assert.True(t, ok, "open in a window")
}

func TestRecord(t *testing.T) {
	var r model.ThreadRecord
	start := agent.Update{Agent: "claude", Status: agent.Idle, Start: true, SetMessage: true, SetPrompt: true,
		Conversation: "abc", Cwd: "/c/api", Transcript: "/t.jsonl"}
	Record(&r, start, &model.Agent{WindowID: 7, Title: "✳ Claude Code", Session: "api", Thread: model.Thread{Pin: 2}}, now)
	assert.Equal(t, model.ThreadRecord{Agent: "claude", Cwd: "/c/api", Transcript: "/t.jsonl", Session: "api", WindowID: 7,
		Status: "idle", StatusAt: now, UpdatedAt: now, Thread: model.Thread{Pin: 2}}, r, "a title only naming the agent isn't kept")

	later := now.Add(time.Minute)
	Record(&r, agent.Update{Agent: "claude", Status: agent.Working, Prompt: "fix it", SetPrompt: true, SetMessage: true, StartTurn: true}, nil, later)
	assert.Equal(t, "working", r.Status)
	assert.Equal(t, "fix it", r.Prompt)
	assert.Equal(t, later, r.TurnAt)
	assert.Equal(t, model.Thread{Pin: 2}, r.Thread, "without a look at the window, its thread stays as it was")

	Record(&r, agent.Update{Agent: "claude", Status: agent.Done, Message: "Fixed", SetMessage: true}, &model.Agent{WindowID: 7, Title: "✳ Fix the login", Session: "api"}, later)
	assert.Equal(t, "Fix the login", r.Title)
	assert.Equal(t, "Fixed", r.Message)

	end := later.Add(time.Minute)
	Record(&r, agent.Update{Agent: "claude", Clear: true}, nil, end)
	assert.Equal(t, "done", r.Status, "the session ending keeps what it last did")
	assert.Equal(t, end, r.EndedAt)
}
