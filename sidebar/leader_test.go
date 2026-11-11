package sidebar

import (
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xheisenbugx/ksesh/model"
)

func TestSpaceOpensAThreadsActions(t *testing.T) {
	m, _ := newModel(t)
	m, _ = press(t, m, " ")
	require.Equal(t, modeMenu, m.mode, "space is the sidebar's leader")
	assert.Contains(t, ansi.Strip(m.View().Content), "Approve")
}

func TestThePickersActionsTakeTheSidebarsLetters(t *testing.T) {
	m, b := pickerModel(t)
	m, _ = update(m, alt(' '))
	require.Equal(t, modeMenu, m.mode, "alt+space is the picker's leader")

	i := slices.IndexFunc(m.menu.items, func(it action) bool { return it.label == "Approve" })
	require.GreaterOrEqual(t, i, 0)
	assert.Equal(t, "a", m.right(m.menu.items[i]), "the menu shows the letter, not ^y")

	b.EXPECT().SendKey(1, "enter").Return(nil).Once()
	m, cmd := update(m, tea.KeyPressMsg{Code: 'a', Text: "a"})
	assert.Equal(t, modeNormal, m.mode)
	require.NotNil(t, cmd)
	cmd()
}

func TestThePickersActionsStillTakeItsOwnKeys(t *testing.T) {
	m, b := pickerModel(t)
	m, _ = update(m, alt(' '))
	b.EXPECT().SendKey(1, "enter").Return(nil).Once()
	m, cmd := update(m, ctrl('y'))
	assert.Equal(t, modeNormal, m.mode)
	require.NotNil(t, cmd)
	cmd()
}

func TestTheSidebarsOwnActions(t *testing.T) {
	m, _ := newModel(t)
	m, _ = press(t, m, "m")
	i := slices.IndexFunc(m.menu.items, func(it action) bool { return it.label == "Sidebar" })
	require.GreaterOrEqual(t, i, 0, "a thread's actions lead to the sidebar's")
	next, _ := m.pick(i)
	m = next.(Model)
	require.Equal(t, modeMenu, m.mode)
	assert.Equal(t, "Sidebar", m.menu.title)
	view := ansi.Strip(m.View().Content)
	for _, item := range []string{"Group by session", "This session only", "Keys", "Hide sidebars"} {
		assert.Contains(t, view, item)
	}

	m, _ = press(t, m, "o")
	assert.Equal(t, modeNormal, m.mode)
	assert.True(t, m.state.Group, "o groups by session from the menu too")
}

func TestQHidesFromTheSidebarsActions(t *testing.T) {
	m, b := newModel(t)
	m = m.openMenu("Sidebar", m.viewMenu(), 4, 0)
	b.EXPECT().CloseSidebars().Return(nil).Once()
	m, cmd := press(t, m, "q")
	assert.Equal(t, modeNormal, m.mode)
	require.NotNil(t, cmd)
	cmd()

	m = m.openThreadMenu(m.agents[0], 4, 0)
	m, cmd = press(t, m, "q")
	assert.Equal(t, modeNormal, m.mode, "q closes a menu that can't hide")
	assert.Nil(t, cmd)
}

func TestTheHintsEndWithMore(t *testing.T) {
	m, _ := newModel(t)
	hints := m.hints()
	assert.Equal(t, "m", hints[len(hints)-1].Help().Key)
	assert.Equal(t, "more", hints[len(hints)-1].Help().Desc)
}

func TestAFailedThreadOffersAReply(t *testing.T) {
	m, _ := newModel(t)
	failed := thread(1, "Broke", "error", time.Minute, model.Thread{})
	next, _ := m.Update(loadedMsg{agents: []*model.Agent{failed}, current: 1, session: "api", shown: true, focused: true})
	m = next.(Model)
	var says []string
	for _, h := range m.hints() {
		says = append(says, h.Help().Desc)
	}
	assert.Contains(t, says, "reply")
}
