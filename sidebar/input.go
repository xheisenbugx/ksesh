package sidebar

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/xheisenbugx/ksesh/agent"
)

// inputs are the questions the footer asks: its label over the line you
// type on, and what the line says while empty.
var inputs = map[mode]struct{ label, placeholder string }{
	modeSnoozeCustom: {"Snooze for, or until", "90m, 2d, 15:04, 2026-12-01 09:00"},
	modeReply:        {"Reply", "reply, enter sends"},
	modeRename:       {"Rename", "a title, empty for the agent's"},
}

// prompt asks in the footer, starting from value.
func (m Model) prompt(md mode, value string) (Model, tea.Cmd) {
	m.mode = md
	m.input.Placeholder = inputs[md].placeholder
	m.input.SetValue(value)
	m.input.CursorEnd()
	return m, m.input.Focus()
}

func (m Model) keyInput(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
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
	return m, cmd
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
	}
	return m, nil
}
