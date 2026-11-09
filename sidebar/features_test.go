package sidebar

import (
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

func TestUnreadThreadsAreMarked(t *testing.T) {
	m, _ := newModel(t)
	view := ansi.Strip(m.View().Content)
	check, _ := agent.GlyphFor(agent.Done)
	assert.Contains(t, view, unreadMark+check.Icon+" Write docs", "finished while you were away")
	assert.NotContains(t, view, unreadMark+" ")
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "Fix the login") || strings.Contains(line, "Refactor") {
			assert.NotContains(t, line, unreadMark)
		}
	}
	a := m.agents[1]
	require.Equal(t, 2, a.WindowID)
	assert.Contains(t, strings.Join(m.renderThread(row{kind: rowThread, section: agent.Active, agent: a}, ""), "\n"), bold("Write docs"))
}

func TestReviewOpensOverTheWindowBesideTheSidebar(t *testing.T) {
	m, b := newModel(t)
	m.opts.Ksesh = []string{"/bin/ksesh", "--config", "/c.toml"}
	m.opts.Env = []string{"PATH=/bin"}
	m, _ = press(t, m, "k") // thread 2, done
	assert.Contains(t, ansi.Strip(m.footer()[1]), "d review", "offered for a finished thread")

	b.EXPECT().LastWorkWindow(9).Return(5, nil).Once()
	b.EXPECT().OverlayOn(5, []string{"PATH=/bin"}, []string{"/bin/ksesh", "--config", "/c.toml", "agent", "diff", "2", "--pager"}).Return(nil).Once()
	_, cmd := press(t, m, "d")
	msg := cmd().(doneMsg)
	assert.NoError(t, msg.err)
}

func TestReviewNeedsAWindowToOpenOver(t *testing.T) {
	m, b := newModel(t)
	b.EXPECT().LastWorkWindow(9).Return(0, nil).Once()
	_, cmd := press(t, m, "d")
	assert.ErrorContains(t, cmd().(doneMsg).err, "no window beside the sidebar")
}

func TestNewThread(t *testing.T) {
	m, b := newModel(t)
	a, _ := m.selectedAgent()
	a.Path = "/c/api"
	var asked string
	m.opts.AgentCommand = func(dir string) string { asked = dir; return "codex" }

	m, _ = press(t, m, "n")
	require.Equal(t, modeNewThread, m.mode)
	assert.Contains(t, ansi.Strip(m.View().Content), "New thread in api", "in the selected thread's session")
	m = typeText(t, m, "fix the user's bug")

	b.EXPECT().NewWindowInSession(model.KittyTabOpts{TargetSession: "api", StartDir: "/c/api", Background: true}).Return("window:42", nil).Once()
	b.EXPECT().SendKeys("window:42", `codex 'fix the user'"'"'s bug'`).Return("", nil).Once()
	b.EXPECT().FocusWindow(42).Return(nil).Once()
	m, cmd := press(t, m, "enter")
	assert.Equal(t, modeNormal, m.mode)
	msg := cmd().(doneMsg)
	require.NoError(t, msg.err)
	assert.Equal(t, "started codex in api", msg.flash)
	assert.Equal(t, "/c/api", asked, "the agent of the thread's repo")
}

func TestNewThreadWithoutAPromptJustStartsTheAgent(t *testing.T) {
	m, b := newModel(t)
	m, _ = press(t, m, "G") // a shelf: no thread selected, so the one you're on
	b.EXPECT().NewWindowInSession(model.KittyTabOpts{TargetSession: "api", Background: true}).Return("window:42", nil).Once()
	b.EXPECT().SendKeys("window:42", "claude").Return("", nil).Once()
	b.EXPECT().FocusWindow(42).Return(nil).Once()
	m, _ = press(t, m, "n")
	_, cmd := press(t, m, "enter")
	cmd()
}

func TestNewThreadInAWorktree(t *testing.T) {
	m, _ := newModel(t)
	a, _ := m.selectedAgent()
	a.Path = "/c/api"
	var ran []string
	var dir string
	m.opts.Run = func(d string, args ...string) error { dir, ran = d, args; return nil }

	m, _ = press(t, m, "N")
	require.Equal(t, modeNewBranch, m.mode)
	assert.Contains(t, ansi.Strip(m.View().Content), "New worktree off api")
	m = typeText(t, m, "feat/login")
	m, _ = press(t, m, "enter")
	require.Equal(t, modeNewWorktree, m.mode, "then what to do")
	assert.Contains(t, ansi.Strip(m.View().Content), "New thread on feat/login")
	m = typeText(t, m, "add a login page")
	m, cmd := press(t, m, "enter")
	msg := cmd().(doneMsg)
	require.NoError(t, msg.err)
	assert.Equal(t, "/c/api", dir, "in the thread's repo")
	assert.Equal(t, []string{"wt", "new", "feat/login", "--prompt", "add a login page"}, ran)
	assert.Equal(t, "started a thread on feat/login", msg.flash)
}

func TestNewThreadInAWorktreeSaysWhatWentWrong(t *testing.T) {
	m, _ := newModel(t)
	m, _ = press(t, m, "N")
	assert.Equal(t, modeNormal, m.mode, "no repo to branch off: the thread has no directory")
	assert.Contains(t, m.flash, "select a thread in the repo")

	a, _ := m.selectedAgent()
	a.Path = "/c/api"
	m.opts.Run = func(string, ...string) error { return assert.AnError }
	m, _ = press(t, m, "N")
	m = typeText(t, m, "x")
	m, _ = press(t, m, "enter")
	_, cmd := press(t, m, "enter")
	assert.Equal(t, assert.AnError, cmd().(doneMsg).err)
}

func TestTheMastheadsPlusStartsANewThread(t *testing.T) {
	m, _ := newModel(t)
	m, _ = update(m, tea.MouseMotionMsg{X: 33, Y: 0})
	assert.True(t, m.hoverNew)
	assert.Contains(t, ansi.Strip(m.footer()[1]), "new thread")
	m, _ = update(m, tea.MouseClickMsg{X: 33, Y: 0, Button: tea.MouseLeft})
	assert.Equal(t, modeNewThread, m.mode)
}

func TestFilterAsYouType(t *testing.T) {
	m, _ := newModel(t)
	m, _ = press(t, m, "/")
	require.Equal(t, modeFilter, m.mode)
	m = typeText(t, m, "DOC")
	view := ansi.Strip(m.View().Content)
	assert.Contains(t, view, "Write docs", "in any case")
	assert.NotContains(t, view, "Fix the login")
	assert.Contains(t, view, "/DOC 1", "the masthead says how many it keeps")
	a, ok := m.selectedAgent()
	require.True(t, ok, "the cursor moves to what is left")
	assert.Equal(t, 2, a.WindowID)

	m, _ = press(t, m, "enter")
	assert.Equal(t, modeNormal, m.mode)
	assert.Equal(t, "DOC", m.filter, "enter keeps it")
	assert.Contains(t, ansi.Strip(m.footer()[1]), "esc unfilter")

	m, cmd := press(t, m, "esc")
	assert.Nil(t, cmd, "esc clears the filter before it leaves the sidebar")
	assert.Empty(t, m.filter)
	assert.Contains(t, ansi.Strip(m.View().Content), "Fix the login")
}

func TestEscInTheFilterClearsIt(t *testing.T) {
	m, _ := newModel(t)
	m, _ = press(t, m, "/")
	m = typeText(t, m, "nothing like it")
	assert.Contains(t, ansi.Strip(m.View().Content), "Nothing in the inbox")
	m, _ = press(t, m, "esc")
	assert.Equal(t, modeNormal, m.mode)
	assert.Empty(t, m.filter)
}

func TestFilterMatchesSessionsAndBranches(t *testing.T) {
	m, _ := newModel(t)
	m.agents[1].Session = "web"
	m.agents[1].Path = "/w/2"
	m.branches["/w/2"] = &branchInfo{ok: true, status: model.BranchStatus{Branch: "feat/login-redirect"}}
	m.filter = "web redirect"
	got := m.board().Ordered(agent.Sections...)
	require.Len(t, got, 1, "every word, in the session or the branch")
	assert.Equal(t, 2, got[0].WindowID)
}

func TestGroupTheInboxBySession(t *testing.T) {
	m, _ := newModel(t)
	m.agents[1].Session = "docs" // 2, the newest
	m.agents = append(m.agents, thread(7, "Old login bug", "idle", 3*time.Minute, model.Thread{}))

	m, _ = press(t, m, "o")
	assert.True(t, m.state.Group)
	assert.True(t, LoadState(m.opts.StatePath).Group, "remembered for every sidebar")
	assert.Equal(t, "[p]3[a]{docs}2{api}17~-[w][s][s]", kinds(m.rows()), "the session with the latest activity first")

	view := ansi.Strip(m.View().Content)
	assert.Contains(t, view, " docs ─")
	assert.NotContains(t, view, "api · Allow Bash", "the group says the session, the threads don't")
	assert.Contains(t, view, "Allow Bash")

	m, _ = press(t, m, "o")
	assert.Equal(t, "[p]3[a]217~-[w][s][s]", kinds(m.rows()))
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
