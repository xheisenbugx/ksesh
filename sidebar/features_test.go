package sidebar

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// typeText types text into whatever has the keyboard, a key at a time.
func typeText(t *testing.T, m Model, text string) Model {
	t.Helper()
	for _, r := range text {
		next, _ := m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = next.(Model)
	}
	return m
}

// refreshes lets the refresh after an action find kitty as it was.
func refreshes(b *MockBackend) {
	b.EXPECT().Refresh().Return().Maybe()
	b.EXPECT().ListAgents().Return(fixtures(), nil).Maybe()
	b.EXPECT().LastWorkWindow(9).Return(1, nil).Maybe()
	b.EXPECT().WindowState(9).Return(true, true, nil).Maybe()
	b.EXPECT().GetCurrentSession().Return("api", nil).Maybe()
}

func TestRename(t *testing.T) {
	m, b := newModel(t)
	m, _ = press(t, m, "R")
	require.Equal(t, modeRename, m.mode)
	assert.Contains(t, ansi.Strip(m.View().Content), "Rename")

	m = typeText(t, m, "Auth rework")
	b.EXPECT().SetTitle(1, "Auth rework").Return(nil).Once()
	m, cmd := press(t, m, "enter")
	assert.Equal(t, modeNormal, m.mode)
	assert.Contains(t, ansi.Strip(m.View().Content), "Auth rework", "shown at once")
	assert.NotContains(t, ansi.Strip(m.View().Content), "Fix the login")
	refreshes(b)
	m = run(t, m, cmd)
	assert.Equal(t, "renamed Auth rework", m.flash)
}

func TestRenameToNothingGoesBackToTheAgentsTitle(t *testing.T) {
	m, b := newModel(t)
	a, _ := m.selectedAgent()
	a.CustomTitle = "Mine"
	m, _ = press(t, m, "R")
	assert.Equal(t, "Mine", m.input.Value(), "starts from the title you gave it")

	for range len("Mine") {
		m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyBackspace})
	}
	b.EXPECT().SetTitle(1, "").Return(nil).Once()
	m, cmd := press(t, m, "enter")
	assert.Contains(t, ansi.Strip(m.View().Content), "Fix the login")
	cmd()
}
