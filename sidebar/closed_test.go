package sidebar

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xheisenbugx/ksesh/agent"
	"github.com/xheisenbugx/ksesh/model"
)

// withClosed is the fixtures and a closed thread, whose window is gone.
func withClosed() []*model.Agent {
	since := now.Add(-time.Hour)
	gone := &model.Agent{Title: "Write the README", Session: "docs", Name: "claude", Status: "working",
		Message: "Added a Threads section", Since: &since, Conversation: "gone-1", Closed: true}
	return append(fixtures(), gone)
}

// closedModel is the sidebar with Settled open and the closed thread
// selected.
func closedModel(t *testing.T) (Model, *MockBackend) {
	t.Helper()
	b := NewMockBackend(t)
	m := New(Options{
		Backend:    b,
		Self:       9,
		ThreadOpts: agent.ThreadOpts{AutoSettleAfter: agent.DefaultAutoSettleAfter},
		StatePath:  filepath.Join(t.TempDir(), "sidebar.json"),
		Now:        func() time.Time { return now },
	})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: 40})
	next, _ = next.Update(loadedMsg{agents: withClosed(), current: 1, session: "api", shown: true, focused: true,
		state: State{Expanded: map[agent.Section]bool{agent.Settled: true}}})
	m = next.(Model)
	m.cursor = "cgone-1"
	m.keepCursor()
	a, ok := m.selectedAgent()
	require.True(t, ok)
	require.True(t, a.Closed)
	return m, b
}

func TestAClosedThreadIsDrawnInSettled(t *testing.T) {
	m, _ := closedModel(t)
	view := ansi.Strip(m.View().Content)
	lines := strings.Split(view, "\n")

	assert.Contains(t, lines[1], "all sessions · 6", "only the live threads count")
	assert.Regexp(t, `Settled +2`, view, "with the settled threads")
	i := lineOf(t, m, "Write the README")
	assert.Contains(t, lines[i], iconClosed, "marked as closed")
	assert.Contains(t, lines[i], "1h", "by when it last did something")
	assert.Contains(t, lines[i+1], "docs · closed · Added a")
	assert.Less(t, i, lineOf(t, m, "Old"), "the most recently active settled thread first")
	assert.NotContains(t, m.renderThread(row{kind: rowThread, section: agent.Settled, agent: m.agents[len(m.agents)-1]}, "")[0], "\033[1m",
		"muted, never bold")
	assert.Contains(t, lines[len(lines)-1], "↵ resume", "the status bar offers what can be done to it")
	m.agents = m.agents[len(m.agents)-1:]
	assert.False(t, m.spinning(), "a closed thread doesn't spin, whatever it was doing")
}

func TestEnterResumesAClosedThread(t *testing.T) {
	m, b := closedModel(t)
	b.EXPECT().ResumeThread("gone-1").Return(nil).Once()

	m, cmd := press(t, m, "enter")
	msg := cmd().(doneMsg)

	assert.NoError(t, msg.err)
	assert.Equal(t, "resuming Write the README", msg.flash)
}

func TestTheCursorFollowsAResumedThreadToItsWindow(t *testing.T) {
	m, _ := closedModel(t)
	agents := fixtures()
	resumed := thread(8, "Write the README", "idle", 0, model.Thread{Unsettled: true})
	resumed.Conversation = "gone-1"
	m, _ = update(m, loadedMsg{agents: append(agents, resumed), current: 1, session: "api", shown: true, focused: true, state: m.state})

	a, ok := m.selectedAgent()
	require.True(t, ok)
	assert.Equal(t, 8, a.WindowID)
}

func TestClickResumesAClosedThread(t *testing.T) {
	m, b := closedModel(t)
	b.EXPECT().ResumeThread("gone-1").Return(nil).Once()

	_, cmd := update(m, tea.MouseClickMsg{X: 3, Y: lineOf(t, m, "Write the README"), Button: tea.MouseLeft})
	cmd()
}

func TestForgetAClosedThread(t *testing.T) {
	m, b := closedModel(t)
	m, _ = press(t, m, "X")
	require.Equal(t, modeConfirmClose, m.mode)
	assert.Contains(t, ansi.Strip(m.View().Content), "Forget the closed thread?")

	b.EXPECT().ForgetThread("gone-1").Return(nil).Once()
	_, cmd := press(t, m, "y")
	cmd()
}

func TestAClosedThreadRefusesTriage(t *testing.T) {
	m, _ := closedModel(t)
	for _, key := range []string{"s", "p", "z", "a", "r"} {
		next, cmd := press(t, m, key)
		assert.Nil(t, cmd, key)
		assert.Equal(t, modeNormal, next.mode, key)
		assert.Equal(t, "it's closed: ↵ resumes it", next.flash, key)
	}
}

func TestTheMenuOfAClosedThread(t *testing.T) {
	m, b := closedModel(t)
	m, _ = press(t, m, "m")
	require.Equal(t, modeMenu, m.mode)
	var labels []string
	for _, it := range m.menu.items {
		if !it.sep {
			labels = append(labels, it.label)
		}
	}
	assert.Equal(t, []string{"Resume", "Forget…"}, labels)

	b.EXPECT().ResumeThread("gone-1").Return(nil).Once()
	_, cmd := press(t, m, "enter")
	cmd()
}

func TestTheCursorFollowsAClosedThreadIntoItsFoldedShelf(t *testing.T) {
	m, _ := closedModel(t)
	m.state.Expanded = nil
	m.keepCursor()
	r, _, ok := m.selected()
	require.True(t, ok)
	assert.Equal(t, "hsettled", rowKey(r))
	assert.False(t, layout(m.board(), nil, false, 0)[len(m.rows())-1].holdsCurrent,
		"with no thread you're on, a closed thread isn't it")
}
