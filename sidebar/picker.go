package sidebar

import (
	"os/exec"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/xheisenbugx/ksesh/agent"
	"github.com/xheisenbugx/ksesh/model"
	"github.com/xheisenbugx/ksesh/ui"
)

// The agent picker is the sidebar's list as a full window over the window
// you're in (Options.Picker): the same sections, rows, menus and actions,
// with a filter you type into at once, the selected thread's screen or diff
// in a pane beside the list, and keys that leave the letters to the filter.

// Widths of the picker's list beside its preview.
const (
	// previewMinWidth is the narrowest window that gets a preview beside the
	// list.
	previewMinWidth = 80
	listMinWidth    = 40
	listMaxWidth    = 72
	// previewDebounce is how long the cursor has to rest on a thread before
	// its preview is fetched, so holding a key down doesn't capture every
	// window passed over.
	previewDebounce = 60 * time.Millisecond
)

type textinputModel = textinput.Model

// Preview is what the picker shows of a thread: a few lines that stay put
// (its status, where it runs), over a body that scrolls (its screen, which
// follows its bottom, or its review).
type Preview struct {
	Header string
	Body   string
}

type (
	// previewTickMsg fetches the preview of the thread with key once the
	// cursor rested on it; seq tells whether it still is.
	previewTickMsg struct {
		seq int
		key string
	}
	previewMsg struct {
		seq     int
		key     string
		preview Preview
		err     error
	}
)

// name is what the masthead says.
func (m Model) name() string {
	if m.opts.Picker {
		return "Agents"
	}
	return "Threads"
}

// previewShown reports whether the preview pane is drawn beside the list.
func (m Model) previewShown() bool {
	return m.opts.Picker && m.previewOn && m.opts.Preview != nil && m.mode != modeHelp && m.width >= previewMinWidth
}

// cols is the width of the list: the window's, less the preview's.
func (m Model) cols() int {
	if !m.previewShown() {
		return m.width
	}
	return min(max(m.width*45/100, listMinWidth), listMaxWidth)
}

// previewWidth is the preview's text, right of its border and a space.
func (m Model) previewWidth() int {
	return max(m.width-m.cols()-2, 1)
}

// sizePreview fits the preview's viewport to the room beside the list, less
// its label and header.
func (m *Model) sizePreview() {
	m.preview.SetWidth(m.previewWidth())
	m.preview.SetHeight(max(m.listHeight()-1-len(m.previewHeader), 1))
}

// styleInputs draws the prompts in the theme.
func (m *Model) styleInputs() {
	for _, in := range []*textinputModel{&m.input, &m.query} {
		st := in.Styles()
		st.Focused.Prompt = lipgloss.NewStyle().Foreground(m.th.Colors.Accent).Bold(true)
		st.Blurred.Prompt = st.Focused.Prompt
		st.Focused.Placeholder = lipgloss.NewStyle().Foreground(m.th.Colors.Muted)
		st.Blurred.Placeholder = st.Focused.Placeholder
		in.SetStyles(st)
	}
}

// typeQuery types a key into the picker's filter, filtering as you type,
// the cursor on the first thread that matches.
func (m Model) typeQuery(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.query, cmd = m.query.Update(msg)
	if f := strings.TrimSpace(m.query.Value()); f != m.filter {
		m.filter = f
		m.offset = 0
		m.firstMatch()
	}
	return m, cmd
}

// firstMatch puts the cursor on the best match in view: the first thread
// with every word of the filter as typed, else the first thread, even in a
// shelf below an empty inbox, else the first row that can be selected. The
// rows keep the sidebar's order; only the cursor goes to the closest match,
// as "old" finds Old before Long build.
func (m *Model) firstMatch() {
	best, first := -1, -1
	for i, r := range m.rows() {
		if r.kind != rowThread {
			continue
		}
		if first < 0 {
			first = i
		}
		if best < 0 && m.matchesWhole(r.agent) {
			best = i
		}
	}
	if best < 0 {
		best = first
	}
	if best < 0 {
		m.jump(false)
		return
	}
	m.cursor, m.cursorIdx = rowKey(m.rows()[best]), best
	m.scrollToCursor()
}

// matchesWhole reports whether a thread has every word of the filter in it
// as typed, not only its letters in order.
func (m Model) matchesWhole(a *model.Agent) bool {
	text := strings.ToLower(m.searchText(a))
	for _, word := range strings.Fields(strings.ToLower(m.filter)) {
		if !strings.Contains(text, word) {
			return false
		}
	}
	return true
}

// fuzzy reports whether every word of the filter is in text, its letters in
// order if not together, as fzf matches: "lgn" finds "login".
func fuzzy(text, filter string) bool {
	text = strings.ToLower(text)
	for _, word := range strings.Fields(strings.ToLower(filter)) {
		rest := text
		for _, r := range word {
			i := strings.IndexRune(rest, r)
			if i < 0 {
				return false
			}
			rest = rest[i+len(string(r)):]
		}
	}
	return true
}

// syncPreview fetches the preview of the selected thread when it isn't the
// one shown, once the cursor rests on it.
func (m Model) syncPreview() (Model, tea.Cmd) {
	if !m.previewShown() {
		return m, nil
	}
	m.sizePreview()
	key := ""
	if r, _, ok := m.selected(); ok && r.kind == rowThread {
		key = rowKey(r)
	}
	if key == m.previewKey {
		return m, nil
	}
	m.previewKey = key
	m.previewSeq++
	if key == "" {
		m.preview.SetContent("")
		m.previewFor, m.previewHeader = "", nil
		return m, nil
	}
	seq := m.previewSeq
	return m, tea.Tick(previewDebounce, func(time.Time) tea.Msg { return previewTickMsg{seq: seq, key: key} })
}

// refetchPreview fetches the preview shown again, at once: the screen moved
// on.
func (m *Model) refetchPreview() tea.Cmd {
	if !m.previewShown() || m.previewKey == "" {
		return nil
	}
	m.previewSeq++
	return m.fetchPreview(previewTickMsg{seq: m.previewSeq, key: m.previewKey})
}

// fetchPreview draws a thread's preview off the UI loop: kitty and git are
// asked for it.
func (m Model) fetchPreview(msg previewTickMsg) tea.Cmd {
	if msg.seq != m.previewSeq || m.opts.Preview == nil {
		return nil
	}
	var a *model.Agent
	for _, other := range m.agents {
		if threadKey(other) == msg.key {
			a = other
		}
	}
	if a == nil {
		return nil
	}
	preview, diff, width := m.opts.Preview, m.previewDiff, m.previewWidth()
	return func() tea.Msg {
		p, err := preview(a, diff, width)
		return previewMsg{seq: msg.seq, key: msg.key, preview: p, err: err}
	}
}

// showPreview puts a fetched preview in the pane. A screen follows its
// bottom, where the agent is, unless you scrolled up; a diff starts at its
// top.
func (m Model) showPreview(msg previewMsg) Model {
	if msg.seq != m.previewSeq {
		return m
	}
	fresh := msg.key != m.previewFor
	atBottom := m.preview.AtBottom()
	m.previewHeader = nil
	if h := strings.TrimRight(msg.preview.Header, " \n"); h != "" {
		m.previewHeader = strings.Split(h, "\n")
	}
	content := strings.TrimRight(msg.preview.Body, " \n")
	if msg.err != nil {
		content, m.previewHeader = m.th.Mute("no preview: "+msg.err.Error()), nil
	}
	m.sizePreview()
	m.preview.SetContent(content)
	m.previewFor = msg.key
	switch {
	case m.previewDiff && fresh:
		m.preview.GotoTop()
	case !m.previewDiff && (fresh || atBottom):
		m.preview.GotoBottom()
	}
	return m
}

// toggleDiff swaps the preview between the thread's screen and its diff.
func (m Model) toggleDiff() (tea.Model, tea.Cmd) {
	if !m.previewShown() {
		m.previewOn = m.opts.Preview != nil
	}
	m.previewDiff = !m.previewDiff
	m.previewKey, m.previewFor, m.previewHeader = "", "", nil
	return m, nil
}

// togglePreview shows or hides the preview pane.
func (m Model) togglePreview() (tea.Model, tea.Cmd) {
	m.previewOn = !m.previewOn && m.opts.Preview != nil
	m.previewKey, m.previewFor, m.previewHeader = "", "", nil
	m.scrollToCursor()
	return m, nil
}

// pager opens the selected thread's review full screen, in less, and comes
// back to the picker after.
func (m Model) pager() (tea.Model, tea.Cmd) {
	a, ok := m.selectedAgent()
	if !ok {
		return m, nil
	}
	args := m.ksesh("agent", "diff", strconv.Itoa(a.WindowID), "--pager")
	cmd := exec.Command(args[0], args[1:]...)
	return m, tea.ExecProcess(cmd, func(err error) tea.Msg { return doneMsg{err: err} })
}

// besidePreview draws the preview pane at the right of the list's lines:
// a label, then the viewport.
//
//	…list…  │ SCREEN ─────────────────────
//	        │ the agent's screen
func (m Model) besidePreview(out []string) []string {
	pw := m.previewWidth()
	label := "SCREEN"
	if m.previewDiff {
		label = "DIFF"
	}
	if a, ok := m.selectedAgent(); ok {
		label += " · " + strings.ToUpper(title(a))
	}
	pane := []string{m.th.LabelRule(m.th.Head(label)+" ", -1, pw, "")}
	if m.previewFor != "" || m.previewKey == "" {
		pane = append(pane, m.previewHeader...)
		pv := m.preview
		pv.SetHeight(max(m.listHeight()-len(pane), 1))
		pane = append(pane, strings.Split(pv.View(), "\n")...)
	} else {
		pane = append(pane, m.th.Mute("…"))
	}
	border := m.th.Dim("│")
	top := m.top()
	for i := range m.listHeight() {
		y := top + i
		if y >= len(out) {
			break
		}
		line := ""
		if i < len(pane) {
			line = pane[i]
		}
		out[y] = ui.Paint(out[y], "", m.cols()) + border + " " + ui.Paint(line, "", pw) + "\033[0m"
	}
	return out
}

// markSeen turns a done or failed agent idle without going to it, as going
// to it would: `ksesh agent set idle`.
func (m Model) markSeen() (tea.Model, tea.Cmd) {
	a, ok := m.selectedAgent()
	if !ok {
		return m, nil
	}
	if s := agent.Status(a.Status); s != agent.Done && s != agent.Error {
		m.say("only a finished or failed agent can be marked seen")
		return m, nil
	}
	if m.opts.Run == nil {
		m.say("can't run ksesh from here")
		return m, nil
	}
	run, dir, id := m.opts.Run, a.Path, strconv.Itoa(a.WindowID)
	return m, m.do("marked "+title(a)+" seen", nil, func(Backend) error {
		return run(dir, "agent", "set", "idle", "--window", id)
	})
}
