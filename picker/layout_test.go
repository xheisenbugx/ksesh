package picker

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xheisenbugx/ksesh/model"
	"github.com/xheisenbugx/ksesh/ui"
)

// only is the test sessions from one source.
func only(src string) model.SeshSessions {
	all := testSessions()
	out := model.SeshSessions{Directory: model.SeshSessionMap{}}
	for _, key := range all.OrderedIndex {
		if all.Directory[key].Src == src {
			out.OrderedIndex = append(out.OrderedIndex, key)
			out.Directory[key] = all.Directory[key]
		}
	}
	return out
}

func sourcesModel() Model {
	sources := []Source{
		{Key: "ctrl+a", Label: "all", Fetch: testFetchFunc(testSessions())},
		{Key: "ctrl+t", Label: "kitty", Fetch: testFetchFunc(only("kitty"))},
		{Key: "ctrl+z", Label: "zoxide", Fetch: testFetchFunc(only("zoxide"))},
	}
	m := New(sources[0].Fetch, testOptionsWith(func(o *Options) { o.Sources = sources }))
	result, _ := m.Update(sessionsLoadedMsg{sessions: testSessions()})
	m = result.(Model)
	m.width, m.height = 80, 20
	return m
}

func names(m Model) []string {
	var out []string
	for _, f := range m.filtered {
		out = append(out, f.item.name)
	}
	return out
}

func TestSources_KeySwitchesTheList(t *testing.T) {
	m := sourcesModel()
	m.cursorDown(2)

	result, cmd := m.Update(tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl})
	m = result.(Model)
	require.NotNil(t, cmd, "switching fetches the source's list")
	assert.Equal(t, 1, m.source)
	assert.True(t, m.loading)
	assert.Equal(t, 0, m.cursor, "a new list starts at its top")

	result, _ = m.Update(cmd())
	m = result.(Model)
	assert.False(t, m.loading)
	assert.Equal(t, []string{"my-project", "notes"}, names(m))

	tabs := ansi.Strip(strings.Split(m.View().Content, "\n")[1])
	assert.Contains(t, tabs, "all · kitty · zoxide")
	assert.Contains(t, tabs, "2", "the count of rows at the right")
}

func TestSources_KeepTheQuery(t *testing.T) {
	m := sourcesModel()
	for _, r := range "app" {
		result, _ := m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = result.(Model)
	}
	result, cmd := m.Update(tea.KeyPressMsg{Code: 'z', Mod: tea.ModCtrl})
	result, _ = result.(Model).Update(cmd())
	m = result.(Model)

	assert.Equal(t, "app", m.filterInput.Value())
	assert.Equal(t, []string{"~/code/app"}, names(m))
}

func TestSources_ALateAnswerIsDropped(t *testing.T) {
	m := sourcesModel()
	result, toKitty := m.Update(tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl})
	result, toZoxide := result.(Model).Update(tea.KeyPressMsg{Code: 'z', Mod: tea.ModCtrl})
	m = result.(Model)

	// The kitty list arrives after the switch to zoxide: it is stale.
	result, _ = m.Update(toKitty())
	m = result.(Model)
	assert.True(t, m.loading, "still waiting on zoxide")

	result, _ = m.Update(toZoxide())
	m = result.(Model)
	assert.Equal(t, []string{"~/code/app"}, names(m))
}

func TestSources_AFailedSwitchKeepsThePicker(t *testing.T) {
	m := sourcesModel()
	m.sources[1].Fetch = func() (model.SeshSessions, error) { return model.SeshSessions{}, errors.New("kitty is gone") }

	result, cmd := m.Update(tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl})
	result, quit := result.(Model).Update(cmd())
	m = result.(Model)

	assert.Nil(t, quit, "the picker stays up")
	assert.NoError(t, m.LoadErr())
	assert.Len(t, m.filtered, 5, "with the list it had")
	assert.Contains(t, ansi.Strip(m.View().Content), "Couldn't list kitty: kitty is gone")
}

func TestSources_NoTabsWithoutSources(t *testing.T) {
	m := newTestModel()
	m.width, m.height = 60, 20
	tabs := ansi.Strip(strings.Split(m.View().Content, "\n")[1])
	assert.Equal(t, "5", strings.TrimSpace(tabs), "only the count")
	assert.NotContains(t, ansi.Strip(m.View().Content), "source")
}

func TestView_Frame(t *testing.T) {
	m := sourcesModel()
	m.cursorDown(1)
	lines := strings.Split(ansi.Strip(m.View().Content), "\n")
	require.Len(t, lines, 20)

	assert.True(t, strings.HasPrefix(lines[0], " Sessions ╱"), "the masthead")
	assert.True(t, strings.HasPrefix(lines[filterRow], filterIndent+"> "), "the filter")
	assert.True(t, strings.HasPrefix(lines[headerLines], "  my-project"))
	assert.True(t, strings.HasPrefix(lines[headerLines+1], "▌ dotfiles"), "the bar on the selected row")
	assert.Contains(t, lines[len(lines)-2], "───", "a rule over the status bar")
	hints := lines[len(lines)-1]
	assert.Contains(t, hints, "↵ connect")
	assert.Contains(t, hints, "^a ^t ^z source")
	assert.Contains(t, hints, "esc quit")
}

func TestView_SelectedRowIsTintedAcrossTheList(t *testing.T) {
	m := sourcesModel()
	th := ui.ThemeFor(nil, nil)
	row := strings.Split(m.View().Content, "\n")[headerLines]
	assert.True(t, strings.HasPrefix(row, th.Selected), "on the selection background")
	assert.Equal(t, m.contentWidth(), lipgloss.Width(row))
}

func TestHints_RemoveSaysWhatItDoes(t *testing.T) {
	m := New(testFetchFunc(testSessions()), testOptionsWith(func(o *Options) {
		o.Kill = func(string) error { return nil }
		o.Remove = func(string) error { return nil }
	}))
	result, _ := m.Update(sessionsLoadedMsg{sessions: testSessions()})
	m = result.(Model)
	m.width, m.height = 100, 20
	footer := func() string {
		lines := strings.Split(ansi.Strip(m.View().Content), "\n")
		return lines[len(lines)-1]
	}

	assert.Contains(t, footer(), "^x kill", "on a kitty session")
	m.cursorDown(1)
	assert.NotContains(t, footer(), "^x", "nothing to do to a config")
	m.cursorDown(1)
	assert.Contains(t, footer(), "^x remove", "on a zoxide entry")
}

func TestView_TakesTheTerminalsColors(t *testing.T) {
	m := newTestModel()
	result, _ := m.Update(tea.BackgroundColorMsg{Color: lipgloss.Color("#ffffff")})
	m = result.(Model)
	assert.Equal(t, ui.ThemeFor(lipgloss.Color("#ffffff"), nil), m.th, "a light theme on a light terminal")
}
