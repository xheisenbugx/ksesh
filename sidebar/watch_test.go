package sidebar

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xheisenbugx/ksesh/agent"
	"github.com/xheisenbugx/ksesh/model"
)

// fakeWatch is kitty's snapshot as the sidebar sees it.
type fakeWatch struct {
	changed, live bool
	calls         int
}

func (w *fakeWatch) Changed() (bool, bool) {
	w.calls++
	return w.changed, w.live
}

func newWatchedModel(t *testing.T, w *fakeWatch) (Model, *MockBackend) {
	t.Helper()
	b := NewMockBackend(t)
	m := New(Options{
		Backend:    b,
		Self:       9,
		ThreadOpts: agent.ThreadOpts{AutoSettleAfter: agent.DefaultAutoSettleAfter},
		StatePath:  filepath.Join(t.TempDir(), "sidebar.json"),
		Now:        func() time.Time { return now },
		Watch:      w.Changed,
	})
	return m, b
}

func expectLoad(b *MockBackend) {
	b.EXPECT().Refresh().Return().Once()
	b.EXPECT().ListAgents().Return([]*model.Agent{}, nil).Once()
	b.EXPECT().LastWorkWindow(9).Return(1, nil).Once()
	b.EXPECT().WindowState(9).Return(true, true, nil).Once()
	b.EXPECT().GetCurrentSession().Return("api", nil).Once()
}

func TestAChangedSnapshotRefreshesAtOnce(t *testing.T) {
	m, b := newWatchedModel(t, &fakeWatch{})
	expectLoad(b)

	_, cmd := m.Update(watchMsg{changed: true, live: true, due: now.Add(time.Hour)})

	require.NotNil(t, cmd)
	msg, ok := cmd().(loadedMsg)
	require.True(t, ok, "a load")
	assert.True(t, msg.tick, "which goes on watching")
}

func TestAnUnchangedLiveSnapshotWaits(t *testing.T) {
	w := &fakeWatch{live: true}
	m, _ := newWatchedModel(t, w)

	// Even past the timer: while kitty keeps the snapshot up, a change shows
	// there.
	_, cmd := m.Update(watchMsg{live: true, due: now.Add(-time.Minute)})

	require.NotNil(t, cmd)
	start := time.Now()
	msg, ok := cmd().(watchMsg)
	require.True(t, ok, "another look, not a load: the backend mock allows no call")
	assert.GreaterOrEqual(t, time.Since(start), watchEvery)
	assert.Equal(t, 1, w.calls)
	assert.Equal(t, now.Add(-time.Minute), msg.due)
}

func TestWithoutALiveSnapshotTheTimerRefreshes(t *testing.T) {
	m, b := newWatchedModel(t, &fakeWatch{})

	_, cmd := m.Update(watchMsg{due: now.Add(time.Second)})
	require.NotNil(t, cmd)
	_, ok := cmd().(watchMsg)
	assert.True(t, ok, "not due yet")

	expectLoad(b)
	_, cmd = m.Update(watchMsg{due: now})
	require.NotNil(t, cmd)
	_, ok = cmd().(loadedMsg)
	assert.True(t, ok, "due: the old fixed refresh")
}

func TestALoadGoesOnWatching(t *testing.T) {
	w := &fakeWatch{changed: true, live: true}
	m, _ := newWatchedModel(t, w)

	_, cmd := m.Update(loadedMsg{agents: []*model.Agent{}, tick: true, shown: true})

	require.NotNil(t, cmd)
	msg, ok := cmd().(watchMsg)
	require.True(t, ok, "the next look is at the snapshot, not a tick")
	assert.True(t, msg.changed)
	assert.Equal(t, now.Add(refreshShown), msg.due)
}

func TestABackgroundSidebarRefreshesAtItsPace(t *testing.T) {
	w := &fakeWatch{live: true}
	m, b := newWatchedModel(t, w)
	m.shown = false

	_, cmd := m.Update(watchMsg{changed: true, live: true, due: now.Add(time.Second)})
	require.NotNil(t, cmd)
	msg, ok := cmd().(watchMsg)
	require.True(t, ok, "nobody looks: not before it is due")
	assert.True(t, msg.changed, "but the change isn't forgotten")

	expectLoad(b)
	_, cmd = m.Update(watchMsg{changed: true, live: true, due: now})
	_, ok = cmd().(loadedMsg)
	assert.True(t, ok)
}

func TestABackgroundSidebarBroughtToTheFrontRefreshesOnChange(t *testing.T) {
	m, b := newWatchedModel(t, &fakeWatch{})
	m.shown = false
	next, _ := m.Update(loadedMsg{agents: []*model.Agent{}, shown: true}) // RefreshMsg's load
	m = next.(Model)

	expectLoad(b)
	_, cmd := m.Update(watchMsg{changed: true, live: true, due: now.Add(5 * time.Second)})
	_, ok := cmd().(loadedMsg)
	assert.True(t, ok, "on screen again: no waiting out the background's pace")
}
