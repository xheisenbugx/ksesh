package sidebar

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/xheisenbugx/ksesh/agent"
	"github.com/xheisenbugx/ksesh/model"
)

var now = time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC)

func thread(id int, title, status string, ago time.Duration, t model.Thread) *model.Agent {
	since := now.Add(-ago)
	return &model.Agent{WindowID: id, Title: title, Session: "api", Name: "claude", Status: status, Since: &since, Thread: t}
}

func fixtures() []*model.Agent {
	waiting := thread(1, "✳ Fix the login", "waiting", 2*time.Minute, model.Thread{})
	waiting.Message = "Allow Bash: npm test"
	return []*model.Agent{
		waiting,
		thread(2, "Write docs", "done", time.Minute, model.Thread{}),
		thread(3, "Refactor", "idle", time.Hour, model.Thread{Pin: 1}),
		thread(4, "Long build", "working", time.Minute, model.Thread{}),
		thread(5, "Later", "idle", time.Minute, model.Thread{SnoozedUntil: now.Add(time.Hour)}),
		thread(6, "Old", "idle", 30*24*time.Hour, model.Thread{}),
	}
}

func kinds(rows []row) string {
	var b strings.Builder
	for _, r := range rows {
		switch r.kind {
		case rowThread:
			b.WriteString(string(rune('0' + r.agent.WindowID)))
		case rowHeader:
			b.WriteString("[" + string(r.section[0]) + "]")
		case rowMore:
			b.WriteString("+")
		case rowEmpty:
			b.WriteString("!")
		case rowFill:
			b.WriteString("~")
		case rowRule:
			b.WriteString("-")
		case rowGroup:
			b.WriteString("{" + r.session + "}")
		}
	}
	return b.String()
}

func TestLayout(t *testing.T) {
	board := agent.Arrange(fixtures(), now, agent.ThreadOpts{AutoSettleAfter: agent.DefaultAutoSettleAfter})

	assert.Equal(t, "[p]3[a]21~-[w][s][s]", kinds(layout(board, nil, false, 0)), "shelves start closed, under the fill")
	folded := layout(board, nil, false, 4)
	assert.Equal(t, "[p]3[a]21~-[w][s][s]", kinds(folded), "a closed shelf hides even the thread you're on")
	assert.True(t, folded[len(folded)-3].holdsCurrent, "but its header says it holds it")
	assert.False(t, folded[len(folded)-2].holdsCurrent)
	open := map[agent.Section]bool{agent.SectionWorking: true, agent.Snoozed: true, agent.Settled: true}
	assert.Equal(t, "[p]3[a]21~-[w]4[s]5[s]6", kinds(layout(board, open, false, 0)))

	board[agent.Pinned] = nil
	assert.Equal(t, "21~-[w][s][s]", kinds(layout(board, nil, false, 0)), "no inbox title without pins above it")
}

func TestLayoutPagesSettled(t *testing.T) {
	var agents []*model.Agent
	for i := range 12 {
		agents = append(agents, thread(i+1, "t", "idle", time.Minute, model.Thread{SettledAt: now.Add(-time.Duration(i) * time.Minute)}))
	}
	board := agent.Arrange(agents, now, agent.ThreadOpts{})
	open := map[agent.Section]bool{agent.Settled: true}

	rows := layout(board, open, false, 0)
	assert.Equal(t, rowMore, rows[len(rows)-1].kind)
	assert.Equal(t, 2, rows[len(rows)-1].count)
	assert.Equal(t, "!~-[s]", kinds(layout(board, nil, false, 0)), "an empty inbox says so")

	rows = layout(board, open, true, 0)
	assert.Equal(t, rowThread, rows[len(rows)-1].kind, "all of them once asked")
}

func newModel(t *testing.T) (Model, *MockBackend) {
	t.Helper()
	b := NewMockBackend(t)
	m := New(Options{
		Backend:    b,
		Self:       9,
		ThreadOpts: agent.ThreadOpts{AutoSettleAfter: agent.DefaultAutoSettleAfter},
		StatePath:  filepath.Join(t.TempDir(), "sidebar.json"),
		Now:        func() time.Time { return now },
	})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 34, Height: 26})
	next, _ = next.Update(loadedMsg{agents: fixtures(), current: 1, session: "api", shown: true, focused: true})
	return next.(Model), b
}

func press(t *testing.T, m Model, keys ...string) (Model, tea.Cmd) {
	t.Helper()
	var cmd tea.Cmd
	for _, k := range keys {
		var next tea.Model
		code := []rune(k)[0]
		msg := tea.KeyPressMsg{Code: code, Text: k}
		switch k {
		case "enter":
			msg = tea.KeyPressMsg{Code: tea.KeyEnter}
		case "esc":
			msg = tea.KeyPressMsg{Code: tea.KeyEscape}
		}
		next, cmd = m.Update(msg)
		m = next.(Model)
	}
	return m, cmd
}

// run runs a command and feeds its message back, the way bubbletea would.
func run(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	require.NotNil(t, cmd)
	next, _ := m.Update(cmd())
	return next.(Model)
}

func TestCursorStartsOnTheThreadYouAreOn(t *testing.T) {
	m, _ := newModel(t)
	a, ok := m.selectedAgent()
	require.True(t, ok)
	assert.Equal(t, 1, a.WindowID)
}

func TestSettleThenUndo(t *testing.T) {
	m, b := newModel(t)
	m, _ = press(t, m, "k") // up to thread 2, the newest in the inbox
	a, _ := m.selectedAgent()
	require.Equal(t, 2, a.WindowID)

	b.EXPECT().SetThread(2, model.Thread{SettledAt: now}).Return(nil).Once()
	b.EXPECT().Refresh().Return().Maybe()
	b.EXPECT().ListAgents().Return(fixtures(), nil).Maybe()
	b.EXPECT().LastWorkWindow(9).Return(1, nil).Maybe()
	b.EXPECT().WindowState(9).Return(true, true, nil).Maybe()
	b.EXPECT().GetCurrentSession().Return("api", nil).Maybe()

	m, cmd := press(t, m, "s")
	assert.Equal(t, agent.Settled, agent.SectionOf(a, now, m.opts.ThreadOpts), "shown settled at once")
	m = run(t, m, cmd)
	assert.Contains(t, m.flash, "settled Write docs")
	require.Len(t, m.undo, 1)

	b.EXPECT().SetThread(2, model.Thread{}).Return(nil).Once()
	m, cmd = press(t, m, "u")
	m = run(t, m, cmd)
	assert.Contains(t, m.flash, "undid settled")
}

func TestSettleRefusesAWaitingAgent(t *testing.T) {
	m, _ := newModel(t)
	m, cmd := press(t, m, "s")
	assert.Nil(t, cmd)
	assert.Equal(t, "the agent is waiting on you", m.flash)
}

func TestSnoozeFromTheMenu(t *testing.T) {
	m, b := newModel(t)
	m, _ = press(t, m, "z")
	assert.Equal(t, modeSnooze, m.mode)
	assert.Contains(t, ansi.Strip(m.View().Content), "In 3 hours")

	b.EXPECT().SetThread(1, mock.MatchedBy(func(th model.Thread) bool {
		return th.SnoozedUntil.Equal(now.Add(3*time.Hour)) && th.SnoozedAt.Equal(now)
	})).Return(nil).Once()
	_, cmd := press(t, m, "2")
	cmd()
}

func TestOpenAShelf(t *testing.T) {
	m, _ := newModel(t)
	m, _ = press(t, m, "G") // the last shelf header, Settled
	r, _, ok := m.selected()
	require.True(t, ok)
	require.Equal(t, rowHeader, r.kind)
	require.Equal(t, agent.Settled, r.section)

	m, _ = press(t, m, "enter")
	assert.True(t, m.state.Expanded[agent.Settled])
	assert.True(t, LoadState(m.opts.StatePath).Expanded[agent.Settled], "shared with the other sidebars")
	assert.Contains(t, ansi.Strip(m.View().Content), "Old")
}

func TestEnterGoesToTheThread(t *testing.T) {
	m, b := newModel(t)
	b.EXPECT().FocusWindow(1).Return(nil).Once()
	_, cmd := press(t, m, "enter")
	cmd()
}

func TestView(t *testing.T) {
	m, _ := newModel(t)
	view := ansi.Strip(m.View().Content)
	lines := strings.Split(view, "\n")

	assert.Len(t, lines, 26, "exactly the height of the dock")
	for i, line := range lines {
		assert.LessOrEqual(t, ansi.StringWidth(line), 34, "line %d is too wide: %q", i, line)
	}
	assert.Contains(t, lines[0], "Threads")
	bell, _ := agent.GlyphFor(agent.Waiting)
	check, _ := agent.GlyphFor(agent.Done)
	assert.Contains(t, lines[1], "all sessions · 6", "the scope and how many threads")
	assert.Contains(t, lines[1], bell.Icon+" 1", "one waiting")
	assert.Contains(t, lines[1], check.Icon+" 1", "one done, and the pinned idle one doesn't count")
	assert.Contains(t, view, "Fix the login", "the agent's glyph is dropped from its title")
	assert.Contains(t, view, "api · Allow Bash", "every thread says where it runs and what it needs")
	assert.Regexp(t, `Settled +1 $`, lines[len(lines)-3], "shelves at the bottom, above the footer, counts at the right")
	assert.Contains(t, lines[len(lines)-1], "a approve", "the status bar offers what a waiting agent needs")
}

func TestClickAThread(t *testing.T) {
	m, b := newModel(t)
	lines := strings.Split(ansi.Strip(m.View().Content), "\n")
	y := -1
	for i, line := range lines {
		if strings.Contains(line, "Write docs") {
			y = i
		}
	}
	require.GreaterOrEqual(t, y, 0)
	b.EXPECT().FocusWindow(2).Return(nil).Once()
	_, cmd := m.Update(tea.MouseClickMsg{X: 3, Y: y, Button: tea.MouseLeft})
	cmd()
}

func TestSubtitleColorsWhatTheAgentNeeds(t *testing.T) {
	m, _ := newModel(t)
	a := thread(7, "Fix it", "waiting", time.Minute, model.Thread{})
	a.Message = "Allow Bash: ls"

	withSession := m.subtitle(a, agent.Active, agent.BadgeOf(a, now))
	assert.Contains(t, withSession, "\033[33mAllow Bash: ls", "yellow: it waits on you")
	assert.Contains(t, ansi.Strip(withSession), "api · Allow Bash: ls")

	a.Session = ""
	alone := m.subtitle(a, agent.Active, agent.BadgeOf(a, now))
	assert.Contains(t, alone, "\033[33mAllow Bash: ls", "still yellow without a session in front")
}

// workingModel is the sidebar with the thread you're on, 4, in it.
func workingModel(t *testing.T) Model {
	t.Helper()
	m := New(Options{
		Backend:    NewMockBackend(t),
		Self:       9,
		ThreadOpts: agent.ThreadOpts{AutoSettleAfter: agent.DefaultAutoSettleAfter},
		StatePath:  filepath.Join(t.TempDir(), "sidebar.json"),
		Now:        func() time.Time { return now },
	})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 34, Height: 24})
	next, _ = next.Update(loadedMsg{agents: fixtures(), current: 4, session: "api", shown: true, focused: true})
	return next.(Model)
}

func TestFoldingTheShelfYouAreIn(t *testing.T) {
	m := workingModel(t)
	r, _, ok := m.selected()
	require.True(t, ok)
	assert.Equal(t, "hworking", rowKey(r), "the cursor waits on the shelf the thread you're on is folded into")
	assert.NotContains(t, ansi.Strip(m.View().Content), "Long build")

	m, _ = press(t, m, "enter")
	assert.Contains(t, ansi.Strip(m.View().Content), "Long build", "open")

	m, _ = press(t, m, "j", "k", "enter")
	assert.NotContains(t, ansi.Strip(m.View().Content), "Long build", "and closed again")
}

func TestARefreshFromBeforeAFoldDoesNotUndoIt(t *testing.T) {
	m := workingModel(t)
	stale := loadedMsg{agents: fixtures(), current: 4, shown: true, focused: true, state: m.state, stateSeq: m.stateSeq}

	m, _ = press(t, m, "enter")
	require.True(t, m.state.Expanded[agent.SectionWorking])

	next, _ := m.Update(stale)
	assert.True(t, next.(Model).state.Expanded[agent.SectionWorking], "read before the fold was saved")
}
