package seshcli

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/xheisenbugx/ksesh/agent"
	"github.com/xheisenbugx/ksesh/kitty"
	"github.com/xheisenbugx/ksesh/model"
	"github.com/xheisenbugx/ksesh/threads"
)

var hookNow = time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC)

func hookUpdate(t *testing.T, input string) agent.Update {
	t.Helper()
	u, ok := agent.FromClaudeHook([]byte(input))
	require.True(t, ok)
	return u
}

func TestHookStartsARecordOfTheConversation(t *testing.T) {
	store := threads.NewStore(t.TempDir())
	k := kitty.NewMockKitty(t)
	u := hookUpdate(t, `{"hook_event_name":"SessionStart","session_id":"abc","cwd":"/c/api","transcript_path":"/t/abc.jsonl"}`)
	k.EXPECT().UpdateAgent(0, mock.MatchedBy(func(u agent.Update) bool { return u.Conversation == "abc" && u.Carry == nil })).
		Return(&model.Agent{WindowID: 7, Title: "✳ Claude Code", Session: "api"}, nil)

	recordHook(k, store, u, hookNow)

	rec, ok := store.Get("abc")
	require.True(t, ok)
	assert.Equal(t, model.ThreadRecord{Conversation: "abc", Agent: "claude", Cwd: "/c/api", Transcript: "/t/abc.jsonl",
		Session: "api", WindowID: 7, Status: "idle", StatusAt: hookNow, UpdatedAt: hookNow}, rec)
}

func TestHookCarriesAResumedThreadOver(t *testing.T) {
	store := threads.NewStore(t.TempDir())
	require.NoError(t, store.Update("abc", func(r *model.ThreadRecord) {
		r.Agent, r.Thread = "claude", model.Thread{Pin: 3, SettledAt: hookNow.Add(-time.Hour)}
	}))
	k := kitty.NewMockKitty(t)
	want := model.Thread{Pin: 3, Unsettled: true}
	k.EXPECT().UpdateAgent(0, mock.MatchedBy(func(u agent.Update) bool { return u.Carry != nil && *u.Carry == want })).
		Return(&model.Agent{WindowID: 8, Session: "api", Thread: want}, nil)

	recordHook(k, store, hookUpdate(t, `{"hook_event_name":"SessionStart","session_id":"abc","cwd":"/c/api"}`), hookNow)

	rec, _ := store.Get("abc")
	assert.Equal(t, want, rec.Thread, "pinned still, out of Settled")
	assert.Equal(t, 8, rec.WindowID, "in its new window")
}

func TestHookKeepsTheHistoryWhenKittyIsGone(t *testing.T) {
	store := threads.NewStore(t.TempDir())
	k := kitty.NewMockKitty(t)
	k.EXPECT().UpdateAgent(0, mock.Anything).Return(nil, errors.New("kitty quit"))

	recordHook(k, store, hookUpdate(t, `{"hook_event_name":"SessionEnd","session_id":"abc","cwd":"/c/api"}`), hookNow)

	rec, ok := store.Get("abc")
	require.True(t, ok)
	assert.Equal(t, hookNow, rec.EndedAt)
}

func TestHookWithoutAConversationKeepsNoHistory(t *testing.T) {
	k := kitty.NewMockKitty(t)
	k.EXPECT().UpdateAgent(0, mock.Anything).Return(nil, nil)

	recordHook(k, nil, hookUpdate(t, `{"hook_event_name":"Stop"}`), hookNow)
}
