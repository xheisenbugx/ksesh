package sidebar

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/xheisenbugx/ksesh/agent"
	"github.com/xheisenbugx/ksesh/model"
)

// inputs are the questions the footer asks: its label over the line you
// type on, and what the line says while empty.
var inputs = map[mode]struct{ label, placeholder string }{
	modeSnoozeCustom: {"Snooze for, or until", "90m, 2d, 15:04, 2026-12-01 09:00"},
	modeReply:        {"Reply", "reply, enter sends"},
	modeRename:       {"Rename", "a title, empty for the agent's"},
	modeNewThread:    {"New thread", "what to do, or empty"},
	modeNewBranch:    {"New worktree", "its branch, e.g. feat/login"},
	modeNewWorktree:  {"New thread", "what to do, or empty"},
	modeFilter:       {"Filter", "title, session or branch"},
}

// prompt asks in the footer, starting from value.
func (m Model) prompt(md mode, value string) (Model, tea.Cmd) {
	m.mode, m.ask = md, ""
	m.input.Placeholder = inputs[md].placeholder
	m.input.SetValue(value)
	m.input.CursorEnd()
	return m, m.input.Focus()
}

func (m Model) keyInput(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if m.mode == modeFilter {
			m.filter = ""
			m.keepCursor()
		}
		m.mode = modeNormal
		m.input.Blur()
		return m, nil
	case "enter":
		text := m.input.Value()
		m.input.Blur()
		md := m.mode
		m.mode = modeNormal
		return m.submit(md, text)
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if m.mode == modeFilter {
		// As you type.
		m.filter = strings.TrimSpace(m.input.Value())
		m.keepCursor()
	}
	return m, cmd
}

// matches reports whether a thread has every word of the filter in its
// title, session or branch, in any case.
func (m Model) matches(a *model.Agent) bool {
	text := m.searchText(a)
	if m.opts.Picker {
		return fuzzy(text, m.filter)
	}
	text = strings.ToLower(text)
	for _, word := range strings.Fields(strings.ToLower(m.filter)) {
		if !strings.Contains(text, word) {
			return false
		}
	}
	return true
}

// searchText is what a filter looks in: a thread's title, session and
// branch.
func (m Model) searchText(a *model.Agent) string {
	text := title(a) + " " + a.Session
	if info, ok := m.branchOf(a); ok {
		text += " " + info.status.Branch
	}
	return text
}

// submit does what was typed in answer to an input mode's question.
func (m Model) submit(md mode, text string) (tea.Model, tea.Cmd) {
	switch md {
	case modeSnoozeCustom:
		until, err := agent.ParseSnooze(text, m.now)
		if err != nil {
			m.say(err.Error())
			return m, nil
		}
		return m.snooze(until)
	case modeReply:
		a, ok := m.selectedAgent()
		if !ok || text == "" {
			return m, nil
		}
		return m, m.do("sent to "+title(a), nil, func(b Backend) error { return b.SendText(a.WindowID, text, true) })
	case modeRename:
		a, ok := m.selectedAgent()
		if !ok {
			return m, nil
		}
		text = strings.TrimSpace(text)
		if text == a.CustomTitle {
			return m, nil
		}
		// Shown at once, rather than at the next refresh.
		a.CustomTitle = text
		flash := "renamed " + text
		if text == "" {
			flash = "back to " + title(a)
		}
		id := a.WindowID
		return m, m.do(flash, nil, func(b Backend) error { return b.SetTitle(id, text) })
	case modeNewThread:
		return m, m.launchThread(strings.TrimSpace(text))
	case modeNewBranch:
		branch := strings.TrimSpace(text)
		if branch == "" {
			return m, nil
		}
		m.fresh.branch = branch
		m, cmd := m.prompt(modeNewWorktree, "")
		m.ask = "New thread on " + branch
		return m, cmd
	case modeNewWorktree:
		return m, m.launchWorktree(strings.TrimSpace(text))
	case modeFilter:
		m.filter = strings.TrimSpace(text)
		m.keepCursor()
	}
	return m, nil
}
