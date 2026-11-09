package threads

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xheisenbugx/ksesh/kitty"
	"github.com/xheisenbugx/ksesh/model"
	"github.com/xheisenbugx/ksesh/namer"
)

func newKitty(t *testing.T) (*Kitty, *kitty.MockKitty, *namer.MockNamer) {
	t.Helper()
	mk := kitty.NewMockKitty(t)
	mn := namer.NewMockNamer(t)
	k := NewKitty(mk, NewStore(t.TempDir()), mn, Retention{Keep: DefaultKeep, MaxAge: DefaultMaxAge})
	k.Now = func() time.Time { return now }
	return k, mk, mn
}

func liveAgent(id int, conv, title string, th model.Thread) *model.Agent {
	return &model.Agent{WindowID: id, Name: "claude", Status: "idle", Title: title, Session: "api", Conversation: conv, Thread: th}
}

func TestListAgentsAddsTheClosedThreads(t *testing.T) {
	k, mk, _ := newKitty(t)
	for _, r := range []model.ThreadRecord{
		{Conversation: "live", Agent: "claude", Title: "Old title", Session: "api", StatusAt: now.Add(-time.Hour)},
		{Conversation: "gone", Agent: "claude", Title: "Write docs", Session: "docs", Status: "done", Message: "Added a section", StatusAt: now.Add(-2 * time.Hour)},
		{Conversation: "ancient", Agent: "claude", StatusAt: now.Add(-60 * 24 * time.Hour)},
	} {
		require.NoError(t, k.store.Update(r.Conversation, func(x *model.ThreadRecord) { *x = r }))
	}
	mk.EXPECT().ListAgents().Return([]*model.Agent{
		liveAgent(1, "live", "✳ Fix the login", model.Thread{Pin: 4}),
		{WindowID: 2, Name: "codex", Status: "idle"},
	}, nil)

	agents, err := k.ListAgents()

	require.NoError(t, err)
	require.Len(t, agents, 3, "the live ones, and the closed one still kept")
	assert.Equal(t, 1, agents[0].WindowID)
	assert.False(t, agents[0].Closed)
	closed := agents[2]
	assert.True(t, closed.Closed)
	assert.Equal(t, "gone", closed.Conversation)
	assert.Equal(t, "Write docs", closed.Title)
	assert.Equal(t, "docs", closed.Session)
	assert.Equal(t, "Added a section", closed.Message)

	live, _ := k.store.Get("live")
	assert.Equal(t, "Fix the login", live.Title, "what the live window shows is kept")
	assert.Equal(t, model.Thread{Pin: 4}, live.Thread)
	assert.Equal(t, 1, live.WindowID)
	_, ok := k.store.Get("ancient")
	assert.False(t, ok, "past retention, forgotten")
	_, ok = k.store.Get("")
	assert.False(t, ok)
}

func TestListAgentsDoesNotRecordAgentsWithoutAHistory(t *testing.T) {
	k, mk, _ := newKitty(t)
	mk.EXPECT().ListAgents().Return([]*model.Agent{liveAgent(1, "new", "Fix it", model.Thread{})}, nil)

	_, err := k.ListAgents()

	require.NoError(t, err)
	_, ok := k.store.Get("new")
	assert.False(t, ok, "the hooks start a record, with where the agent runs")
}

func TestListAgentsLeavesARecordUpToDateAlone(t *testing.T) {
	k, mk, _ := newKitty(t)
	settled := model.Thread{SettledAt: time.Unix(1700000000, 0)}
	require.NoError(t, k.store.Update("abc", func(r *model.ThreadRecord) {
		r.Agent, r.Title, r.Session, r.WindowID, r.Thread = "claude", "Fix it", "api", 1, settled
	}))
	path := filepath.Join(k.store.dir, "abc.json")
	old := time.Now().Add(-time.Hour)
	require.NoError(t, os.Chtimes(path, old, old))
	// kitty's times are local, the file's as read back carry an offset.
	mk.EXPECT().ListAgents().Return([]*model.Agent{liveAgent(1, "abc", "Fix it", model.Thread{SettledAt: time.Unix(1700000000, 0).Local()})}, nil)

	_, err := k.ListAgents()

	require.NoError(t, err)
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.True(t, info.ModTime().Equal(old), "not written every refresh")
}

func TestSetThreadWritesThrough(t *testing.T) {
	k, mk, _ := newKitty(t)
	require.NoError(t, k.store.Update("abc", func(r *model.ThreadRecord) { r.Agent = "claude" }))
	mk.EXPECT().ListAgents().Return([]*model.Agent{liveAgent(1, "abc", "", model.Thread{})}, nil)
	_, err := k.ListAgents()
	require.NoError(t, err)

	settled := model.Thread{SettledAt: now}
	mk.EXPECT().SetThread(1, settled).Return(nil)
	require.NoError(t, k.SetThread(1, settled))

	r, _ := k.store.Get("abc")
	assert.Equal(t, settled, r.Thread, "there when the window goes")
}

func closedRecord(t *testing.T, k *Kitty, session string) {
	t.Helper()
	cwd := t.TempDir()
	require.NoError(t, k.store.Update("abc-123", func(r *model.ThreadRecord) {
		r.Agent, r.Cwd, r.Session = "claude", cwd, session
		r.Thread = model.Thread{Pin: 3, SettledAt: now}
	}))
}

func TestResumeInTheThreadsSession(t *testing.T) {
	k, mk, _ := newKitty(t)
	closedRecord(t, k, "api")
	r, _ := k.store.Get("abc-123")
	mk.EXPECT().ListAgents().Return(nil, nil)
	mk.EXPECT().ListSessions().Return([]*model.KittySession{{Name: "web"}, {Name: "api"}}, nil)
	mk.EXPECT().NewWindowInSession(model.KittyTabOpts{TargetSession: "api", StartDir: r.Cwd}).Return("window:42", nil)
	mk.EXPECT().SendKeys("window:42", "claude --resume abc-123").Return("", nil)
	mk.EXPECT().SwitchOrAttach("api", model.ConnectOpts{Switch: true}).Return("", nil)

	require.NoError(t, k.ResumeThread("abc-123"))
}

func TestResumeStartsTheSessionAgain(t *testing.T) {
	k, mk, _ := newKitty(t)
	closedRecord(t, k, "api")
	r, _ := k.store.Get("abc-123")
	mk.EXPECT().ListAgents().Return(nil, nil)
	mk.EXPECT().ListSessions().Return([]*model.KittySession{{Name: "web"}}, nil)
	mk.EXPECT().NewSession("api", r.Cwd).Return("window:42", nil).Once()
	mk.EXPECT().SendKeys("window:42", "claude --resume abc-123").Return("", nil).Once()
	mk.EXPECT().SwitchOrAttach("api", model.ConnectOpts{Switch: true}).Return("", nil)

	require.NoError(t, k.ResumeThread("abc-123"))

	assert.ErrorContains(t, k.ResumeThread("abc-123"), "already resuming",
		"asked again before its agent is up, it doesn't open a second window")
	k.Now = func() time.Time { return now.Add(time.Minute) }
	mk.EXPECT().NewSession("api", r.Cwd).Return("window:43", nil)
	mk.EXPECT().SendKeys("window:43", "claude --resume abc-123").Return("", nil)
	require.NoError(t, k.ResumeThread("abc-123"), "but it does once that should have happened")
}

func TestResumeOutsideAnySessionNamesOneFromTheDirectory(t *testing.T) {
	k, mk, mn := newKitty(t)
	closedRecord(t, k, "")
	r, _ := k.store.Get("abc-123")
	mk.EXPECT().ListAgents().Return(nil, nil)
	mn.EXPECT().Name(r.Cwd).Return("kitty-sesh", nil)
	mk.EXPECT().ListSessions().Return(nil, nil)
	mk.EXPECT().NewSession("kitty-sesh", r.Cwd).Return("window:42", nil)
	mk.EXPECT().SendKeys("window:42", "claude --resume abc-123").Return("", nil)
	mk.EXPECT().SwitchOrAttach("kitty-sesh", model.ConnectOpts{Switch: true}).Return("", nil)

	require.NoError(t, k.ResumeThread("abc-123"))
}

func TestResumeAThreadStillOpenGoesToIt(t *testing.T) {
	k, mk, _ := newKitty(t)
	closedRecord(t, k, "api")
	mk.EXPECT().ListAgents().Return([]*model.Agent{liveAgent(8, "abc-123", "", model.Thread{})}, nil)
	mk.EXPECT().FocusWindow(8).Return(nil)

	require.NoError(t, k.ResumeThread("abc-123"))
}

func TestResumeRefusesAThreadWhoseDirectoryIsGone(t *testing.T) {
	k, mk, _ := newKitty(t)
	require.NoError(t, k.store.Update("abc", func(r *model.ThreadRecord) { r.Agent, r.Cwd = "claude", "/no/such/dir" }))
	mk.EXPECT().ListAgents().Return(nil, nil)

	assert.ErrorContains(t, k.ResumeThread("abc"), "directory is gone")
}

func TestForgetThread(t *testing.T) {
	k, _, _ := newKitty(t)
	closedRecord(t, k, "api")

	require.NoError(t, k.ForgetThread("abc-123"))

	_, ok := k.store.Get("abc-123")
	assert.False(t, ok)
}

func TestRetentionOf(t *testing.T) {
	zero, days := 0, 5
	assert.Equal(t, Retention{Keep: 200, MaxAge: 30 * 24 * time.Hour}, RetentionOf(model.SidebarConfig{}))
	assert.Equal(t, Retention{Keep: 0, MaxAge: 5 * 24 * time.Hour}, RetentionOf(model.SidebarConfig{KeepClosed: &zero, KeepClosedDays: &days}))
}

func TestARenamedThreadKeepsItsTitleOnceClosed(t *testing.T) {
	k, mk, _ := newKitty(t)
	require.NoError(t, k.store.Update("conv", func(r *model.ThreadRecord) {
		*r = model.ThreadRecord{Conversation: "conv", Agent: "claude", Status: "done", StatusAt: now.Add(-time.Hour)}
	}))
	renamed := liveAgent(1, "conv", "✳ Fix the login", model.Thread{})
	renamed.CustomTitle = "Login redirect"
	mk.EXPECT().ListAgents().Return([]*model.Agent{renamed}, nil).Once()
	_, err := k.ListAgents()
	require.NoError(t, err)
	r, _ := k.store.Get("conv")
	assert.Equal(t, "Login redirect", r.CustomTitle)

	// The window closes.
	mk.EXPECT().ListAgents().Return(nil, nil).Once()
	agents, err := k.ListAgents()
	require.NoError(t, err)
	require.Len(t, agents, 1)
	assert.True(t, agents[0].Closed)
	assert.Equal(t, "Login redirect", agents[0].CustomTitle)
}
