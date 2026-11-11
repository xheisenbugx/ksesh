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

func pickerModel(t *testing.T) (Model, *MockBackend) {
	t.Helper()
	b := NewMockBackend(t)
	m := New(Options{
		Backend:     b,
		Self:        9,
		ThreadOpts:  agent.ThreadOpts{AutoSettleAfter: agent.DefaultAutoSettleAfter},
		StatePath:   filepath.Join(t.TempDir(), "sidebar.json"),
		Now:         func() time.Time { return now },
		Picker:      true,
		OpenShelves: []agent.Section{agent.SectionWorking},
		Preview: func(a *model.Agent, diff bool, width int) (Preview, error) {
			if diff {
				return Preview{Body: "diff of " + title(a)}, nil
			}
			body := strings.Repeat("scrolled away\n", 40) + "screen of " + title(a) + "\n"
			return Preview{Header: "status of " + title(a), Body: body}, nil
		},
	})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	next, _ = next.Update(loadedMsg{agents: fixtures(), current: 1, session: "api", shown: true})
	return next.(Model), b
}

func ctrl(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Mod: tea.ModCtrl} }
func alt(r rune) tea.KeyPressMsg  { return tea.KeyPressMsg{Code: r, Mod: tea.ModAlt} }

// fetched feeds the picker the preview of the thread it is on, as the
// debounce tick would.
func fetched(t *testing.T, m Model) Model {
	t.Helper()
	cmd := m.fetchPreview(previewTickMsg{seq: m.previewSeq, key: m.previewKey})
	require.NotNil(t, cmd)
	next, _ := m.Update(cmd())
	return next.(Model)
}

func TestThePickerHasTheFocusAndTheWholeWindow(t *testing.T) {
	m, _ := pickerModel(t)
	assert.True(t, m.focused, "a picker has the focus without kitty saying so")
	assert.Equal(t, 54, m.cols(), "the list leaves the rest to the preview")
	view := ansi.Strip(m.View().Content)
	assert.Contains(t, view, "Agents")
	assert.Contains(t, view, "filter by title, session or branch")
	assert.Contains(t, view, "SCREEN · FIX THE LOGIN")
	assert.Contains(t, view, "^y approve", "the picker's keys in the hints")
	assert.Len(t, strings.Split(m.View().Content, "\n"), 30, "exactly the window's height")
}

func TestTypingInThePickerFilters(t *testing.T) {
	m, _ := pickerModel(t)
	m = typeText(t, m, "rfct")
	assert.Equal(t, "rfct", m.filter, "letters are typed, not keys")
	a, ok := m.selectedAgent()
	require.True(t, ok)
	assert.Equal(t, "Refactor", title(a), "fuzzily, the cursor on the first match")
	assert.NotContains(t, ansi.Strip(m.View().Content), "Write docs")

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(Model)
	assert.Empty(t, m.filter, "esc clears the filter first")
	assert.Empty(t, m.query.Value())
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	require.NotNil(t, cmd)
	assert.IsType(t, tea.QuitMsg{}, cmd(), "then quits")
}

func TestThePickersActionsAreCtrlAndAltKeys(t *testing.T) {
	m, b := pickerModel(t)
	b.EXPECT().SendKey(1, "enter").Return(nil).Once()
	_, cmd := m.Update(ctrl('y'))
	require.NotNil(t, cmd)
	runBatch(cmd)

	next, _ := m.Update(alt('z'))
	assert.Equal(t, modeSnooze, next.(Model).mode)
}

func TestEnterInThePickerGoesAndQuits(t *testing.T) {
	m, b := pickerModel(t)
	b.EXPECT().FocusWindow(1).Return(nil).Once()
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	var quit tea.Cmd
	for _, msg := range runBatch(cmd) {
		if done, ok := msg.(doneMsg); ok {
			assert.True(t, done.quit)
			_, quit = next.Update(done)
		}
	}
	require.NotNil(t, quit)
	assert.IsType(t, tea.QuitMsg{}, quit())
}

func TestThePreviewShowsTheScreenThenTheDiff(t *testing.T) {
	m, _ := pickerModel(t)
	m = fetched(t, m)
	view := ansi.Strip(m.View().Content)
	assert.Contains(t, view, "screen of Fix the login", "the screen follows its bottom")
	assert.Contains(t, view, "status of Fix the login", "under its header, which stays")
	assert.Len(t, strings.Split(m.View().Content, "\n"), 30)

	next, _ := m.Update(ctrl('g'))
	m = fetched(t, next.(Model))
	assert.Contains(t, ansi.Strip(m.View().Content), "diff of Fix the login")
	assert.Contains(t, ansi.Strip(m.View().Content), "^g screen", "and says how to go back")

	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	m = fetched(t, next.(Model))
	assert.Contains(t, ansi.Strip(m.View().Content), "diff of Write docs", "the diff stays on for the next thread")
}

func TestALatePreviewIsDropped(t *testing.T) {
	m, _ := pickerModel(t)
	late := m.fetchPreview(previewTickMsg{seq: m.previewSeq, key: m.previewKey})
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = next.(Model)
	next, _ = m.Update(late())
	assert.NotContains(t, ansi.Strip(next.(Model).View().Content), "screen of Fix the login")
}

func TestThePickersHelpSaysItsKeys(t *testing.T) {
	m, _ := pickerModel(t)
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyF1})
	view := ansi.Strip(next.(Model).View().Content)
	assert.Contains(t, view, "alt+p")
	assert.Contains(t, view, "^y")
	assert.NotContains(t, view, "narrower", "nothing the picker doesn't do")
}

func TestAFilterMatchingOnlyAShelfSelectsItsThread(t *testing.T) {
	m, _ := pickerModel(t)
	m = typeText(t, m, "old")
	a, ok := m.selectedAgent()
	require.True(t, ok, "a thread, not the shelf's header")
	assert.Equal(t, "Old", title(a))
}

func TestThePickersShelvesAreItsOwn(t *testing.T) {
	m, _ := pickerModel(t)
	view := ansi.Strip(m.View().Content)
	assert.Contains(t, view, "Long build", "the shelves it is told to open")
	assert.NotContains(t, view, "Later", "and no others")
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	m = next.(Model)
	r, _, _ := m.selected()
	require.Equal(t, rowHeader, r.kind)
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	assert.Contains(t, ansi.Strip(m.View().Content), "Old", "enter opens a shelf")
	assert.Nil(t, LoadState(m.opts.StatePath).Expanded, "the sidebars' shelves untouched")
}

func TestASearchLooksInEveryShelf(t *testing.T) {
	m, _ := pickerModel(t)
	m.opts.OpenShelves = nil
	m.opened = map[agent.Section]bool{}
	assert.NotContains(t, ansi.Strip(m.View().Content), "Later")

	m = typeText(t, m, "later")
	assert.Contains(t, ansi.Strip(m.View().Content), "Later", "a snoozed thread is found")
	a, ok := m.selectedAgent()
	require.True(t, ok)
	assert.Equal(t, "Later", title(a))

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	assert.NotContains(t, ansi.Strip(next.(Model).View().Content), "Later", "closed again once the filter is cleared")
}

// runBatch runs a command and what it batches, returning their messages.
func runBatch(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, runBatch(c)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

func TestThePickerMarksAFinishedAgentSeen(t *testing.T) {
	m, _ := pickerModel(t)
	var ran []string
	m.opts.Run = func(dir string, args ...string) error {
		ran = args
		return nil
	}
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyUp}) // Write docs, done
	m = next.(Model)
	assert.Contains(t, ansi.Strip(m.View().Content), "^s seen", "offered for a finished agent")
	_, cmd := m.Update(ctrl('s'))
	runBatch(cmd)
	assert.Equal(t, []string{"agent", "set", "idle", "--window", "2"}, ran)

	ran = nil
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown}) // Fix the login, waiting
	next, cmd = next.Update(ctrl('s'))
	runBatch(cmd)
	assert.Nil(t, ran, "a waiting agent isn't marked")
	assert.Contains(t, next.(Model).flash, "only a finished or failed agent")
}
