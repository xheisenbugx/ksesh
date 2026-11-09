package sidebar

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xheisenbugx/ksesh/model"
)

const prURL = "https://github.com/o/r/pull/12"

// withPR is the sidebar on thread 2 (Write docs, done), on branch docs with
// pull request #12, and the URLs it opened.
func withPR(t *testing.T) (Model, *[]string) {
	t.Helper()
	m, _ := newModel(t)
	var opened []string
	m.opts.OpenURL = func(url string) error {
		opened = append(opened, url)
		return nil
	}
	for _, a := range m.agents {
		if a.WindowID == 2 {
			a.Path = "/w/2"
		}
	}
	m.branches["/w/2"] = &branchInfo{
		ok:     true,
		status: model.BranchStatus{Branch: "docs", Base: "origin/main"},
		hasPR:  true,
		pr:     model.PullRequestState{Number: 12, State: model.PROpen, URL: prURL},
	}
	m, _ = press(t, m, "k")
	a, ok := m.selectedAgent()
	require.True(t, ok)
	require.Equal(t, 2, a.WindowID)
	return m, &opened
}

func TestPOpensTheThreadsPullRequest(t *testing.T) {
	m, opened := withPR(t)
	assert.Contains(t, ansi.Strip(m.footer()[1]), "P pr", "offered for a finished thread with one")

	m, cmd := press(t, m, "P")
	m = run(t, m, cmd)
	assert.Equal(t, []string{prURL}, *opened)
	assert.Contains(t, ansi.Strip(m.footer()[1]), "opened #12")
}

func TestPWithoutAPullRequest(t *testing.T) {
	m, opened := withPR(t)
	m, _ = press(t, m, "j") // thread 1: no branch known
	m, cmd := press(t, m, "P")
	assert.Nil(t, cmd)
	assert.Empty(t, *opened)
	assert.Contains(t, ansi.Strip(m.footer()[1]), "no pull request")
}

func TestThePullRequestIsInTheMenu(t *testing.T) {
	m, opened := withPR(t)
	a, _ := m.selectedAgent()
	labels := func(items []action) []string {
		var out []string
		for _, it := range items {
			out = append(out, it.label)
		}
		return out
	}
	assert.Contains(t, labels(m.threadMenu(a)), "Open PR #12")

	a.Closed = true
	assert.Contains(t, labels(m.threadMenu(a)), "Open PR #12", "a closed thread's pull request too")
	a.Closed = false

	m, _ = update(m, tea.MouseClickMsg{X: 5, Y: lineOf(t, m, "Write docs"), Button: tea.MouseRight})
	require.Equal(t, modeMenu, m.mode)
	m, cmd := update(m, tea.MouseClickMsg{X: m.menu.x + 3, Y: lineOf(t, m, "Open PR #12"), Button: tea.MouseLeft})
	run(t, m, cmd)
	assert.Equal(t, []string{prURL}, *opened)
}

func TestClickingThePullRequestOpensIt(t *testing.T) {
	m, opened := withPR(t)
	y := lineOf(t, m, "#12")
	line := strings.TrimRight(ansi.Strip(strings.Split(m.View().Content, "\n")[y]), " ")
	end := ansi.StringWidth(line)

	_, cmd := update(m, tea.MouseClickMsg{X: end - 1, Y: y, Button: tea.MouseLeft})
	run(t, m, cmd)
	assert.Equal(t, []string{prURL}, *opened, "a click on #12")

	m.opts.Backend.(*MockBackend).EXPECT().FocusWindow(2).Return(nil).Maybe()
	_, cmd = update(m, tea.MouseClickMsg{X: 5, Y: y, Button: tea.MouseLeft})
	if cmd != nil {
		cmd()
	}
	assert.Len(t, *opened, 1, "anywhere else on the line goes to the thread")
}
