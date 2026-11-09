package sidebar

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/xheisenbugx/ksesh/model"
)

// lineOf is the screen line a text is drawn on.
func lineOf(t *testing.T, m Model, text string) int {
	t.Helper()
	for i, line := range strings.Split(ansi.Strip(m.View().Content), "\n") {
		if strings.Contains(line, text) {
			return i
		}
	}
	require.Failf(t, "not on screen", "%q", text)
	return -1
}

func update(m Model, msg tea.Msg) (Model, tea.Cmd) {
	next, cmd := m.Update(msg)
	return next.(Model), cmd
}

func TestRightClickOpensTheThreadsMenu(t *testing.T) {
	m, b := newModel(t)
	y := lineOf(t, m, "Write docs")
	m, _ = update(m, tea.MouseClickMsg{X: 5, Y: y, Button: tea.MouseRight})
	require.Equal(t, modeMenu, m.mode)
	a, _ := m.selectedAgent()
	assert.Equal(t, 2, a.WindowID, "the thread clicked is the one the menu is for")

	view := ansi.Strip(m.View().Content)
	for _, item := range []string{"Go to thread", "Pin", "Snooze", "Settle", "Reply…", "Close window…"} {
		assert.Contains(t, view, item)
	}
	assert.NotContains(t, view, "Approve", "it isn't waiting")
	for i, line := range strings.Split(view, "\n") {
		assert.LessOrEqual(t, ansi.StringWidth(line), 34, "line %d is too wide: %q", i, line)
	}

	b.EXPECT().SetThread(2, mock.MatchedBy(func(th model.Thread) bool { return th.Pin != 0 })).Return(nil).Once()
	m, cmd := update(m, tea.MouseClickMsg{X: m.menu.x + 3, Y: lineOf(t, m, "Pin"), Button: tea.MouseLeft})
	assert.Equal(t, modeNormal, m.mode, "picking closes the menu")
	cmd()
}

func TestTheMenuByKeys(t *testing.T) {
	m, b := newModel(t)
	m, _ = press(t, m, "m")
	require.Equal(t, modeMenu, m.mode)
	assert.Contains(t, ansi.Strip(m.View().Content), "Approve", "the waiting thread offers approve")

	m, _ = press(t, m, "z")
	require.Equal(t, modeMenu, m.mode, "snooze opens its times in the menu")
	assert.Equal(t, "Snooze until", m.menu.title)
	b.EXPECT().SetThread(1, mock.MatchedBy(func(th model.Thread) bool { return th.SnoozedUntil.After(now) })).Return(nil).Once()
	m, cmd := press(t, m, "1")
	assert.Equal(t, modeNormal, m.mode)
	cmd()
}

func TestAClickOutsideClosesTheMenu(t *testing.T) {
	m, _ := newModel(t)
	m, _ = press(t, m, "m")
	m, cmd := update(m, tea.MouseClickMsg{X: 0, Y: 0, Button: tea.MouseLeft})
	assert.Equal(t, modeNormal, m.mode)
	assert.Nil(t, cmd)
}

func TestHoveringAThreadShowsItsButtons(t *testing.T) {
	m, b := newModel(t)
	y := lineOf(t, m, "Write docs")
	before := strings.Split(ansi.Strip(m.View().Content), "\n")[y]
	assert.Contains(t, before, "1m", "its time, until the mouse is on it")

	settleAt := m.width - 1 - buttonWidth + 1 // the last of pin, snooze, settle
	m, _ = update(m, tea.MouseMotionMsg{X: settleAt, Y: y})
	line := strings.Split(ansi.Strip(m.View().Content), "\n")[y]
	assert.NotContains(t, line, "1m", "the buttons take the time's place")
	assert.Contains(t, line, iconPin)
	assert.Contains(t, line, iconSettle)
	assert.Contains(t, ansi.Strip(m.View().Content), "settle  s", "the footer says what the button does")

	b.EXPECT().SetThread(2, model.Thread{SettledAt: now}).Return(nil).Once()
	_, cmd := update(m, tea.MouseClickMsg{X: settleAt, Y: y, Button: tea.MouseLeft})
	cmd()
}

func TestTheSnoozeButtonOpensTheTimes(t *testing.T) {
	m, _ := newModel(t)
	y := lineOf(t, m, "Write docs")
	snoozeAt := m.width - 1 - 2*buttonWidth + 1
	m, _ = update(m, tea.MouseMotionMsg{X: snoozeAt, Y: y})
	m, _ = update(m, tea.MouseClickMsg{X: snoozeAt, Y: y, Button: tea.MouseLeft})
	require.Equal(t, modeMenu, m.mode)
	assert.Equal(t, "Snooze until", m.menu.title)
	assert.Contains(t, ansi.Strip(m.View().Content), "Other time…")
}
