package sidebar

import (
	"charm.land/bubbles/v2/key"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/xheisenbugx/ksesh/agent"
	"github.com/xheisenbugx/ksesh/model"
	"github.com/xheisenbugx/ksesh/ui"
)

// Icons of the actions on a thread, in its menu and on its buttons.
const (
	iconGo        = "" // arrow right
	iconWake      = "" // sun
	iconRestore   = "" // undo
	iconApprove   = "" // check
	iconDeny      = "" // times
	iconInterrupt = "" // stop
	iconReply     = "" // reply
	iconRename    = "" // pencil
	iconReview    = "" // eye
	iconNew       = "" // plus
	iconBranch    = "" // code fork
	iconClose     = "" // window close
	iconUp        = "" // arrow up
	iconDown      = "" // arrow down
	iconResume    = "" // play
	iconForget    = "" // trash
)

// action is something to do to a thread, from its menu or one of its
// buttons.
type action struct {
	icon, label string
	// do is the act it runs, whose keys a menu shows at its right. key is
	// a key of the menu's own instead (a snooze time's number); hint, if set,
	// is shown in place of either.
	do        act
	key, hint string
	on        bool // a button showing a state: a pinned thread's pin
	danger    bool
	sep       bool // a line between groups in a menu, not an action
	run       func(Model) (tea.Model, tea.Cmd)
}

// byAct is an action that runs an act, as its keys do.
func byAct(icon, label string, do act) action {
	return action{icon: icon, label: label, do: do, run: func(m Model) (tea.Model, tea.Cmd) { return m.run(do) }}
}

var separator = action{sep: true}

// popup is a menu drawn over the list, as a card with a rounded border.
type popup struct {
	title string
	items []action
	idx   int // the highlighted item
	x, y  int // its top left corner, on the screen
	w     int
}

func (p popup) height() int { return len(p.items) + 2 }

// itemAt is the item drawn at x, y of the screen.
func (p popup) itemAt(x, y int) (int, bool) {
	i := y - p.y - 1
	if x < p.x || x >= p.x+p.w || i < 0 || i >= len(p.items) || p.items[i].sep {
		return 0, false
	}
	return i, true
}

// threadMenu is everything that can be done to a thread, what you're likely
// to want first.
//
//	╭──────────────────────╮
//	│  Go to thread     ↵ │
//	│  Pin              p │
//	│  Snooze           › │
//	│  Settle           s │
//	├──────────────────────┤
//	│  Approve          a │
//	│  Deny             x │
//	│  Reply…           r │
//	│  Review changes   d │
//	│  Open PR #12      P │
//	├──────────────────────┤
//	│  Close window…    X │
//	╰──────────────────────╯
//
// A closed thread can only be resumed or forgotten:
//
//	╭──────────────────────╮
//	│  Resume           ↵ │
//	├──────────────────────┤
//	│  Forget…          X │
//	╰──────────────────────╯
func (m Model) threadMenu(a *model.Agent) []action {
	if a.Closed {
		resume := byAct(iconResume, "Resume", actGo)
		forget := byAct(iconForget, "Forget…", actClose)
		forget.danger = true
		items := []action{resume}
		if pr, ok := m.pullRequest(a); ok {
			items = append(items, m.prAction(pr))
		}
		return append(items, separator, forget)
	}
	section := agent.SectionOf(a, m.now, m.opts.ThreadOpts)
	status := agent.Status(a.Status)
	items := []action{byAct(iconGo, "Go to thread", actGo)}

	if section != agent.Snoozed && section != agent.Settled {
		pin := byAct(iconPin, "Pin", actPin)
		if a.Thread.Pin != 0 {
			pin.label = "Unpin"
		}
		items = append(items, pin)
		if a.Thread.Pin != 0 && len(m.board()[agent.Pinned]) > 1 {
			items = append(items, byAct(iconUp, "Move up", actPinUp), byAct(iconDown, "Move down", actPinDown))
		}
	}
	switch {
	case section == agent.Snoozed:
		items = append(items, byAct(iconWake, "Wake now", actWake))
	case agent.BadgeOf(a, m.now) == agent.BadgeWoke:
		items = append(items, byAct(iconWake, "Dismiss", actWake))
	case section != agent.Settled:
		items = append(items, m.snoozeAction())
	}
	switch {
	case section == agent.Settled:
		items = append(items, byAct(iconRestore, "Restore", actSettle))
	case status != agent.Working && status != agent.Waiting:
		items = append(items, byAct(iconSettle, "Settle", actSettle))
	}

	items = append(items, separator)
	switch status {
	case agent.Waiting:
		items = append(items, byAct(iconApprove, "Approve", actApprove), byAct(iconDeny, "Deny", actDeny))
	case agent.Working:
		items = append(items, byAct(iconInterrupt, "Interrupt", actDeny))
	}
	items = append(items, byAct(iconReply, "Reply…", actReply), byAct(iconReview, "Review changes", actReview))
	if pr, ok := m.pullRequest(a); ok {
		items = append(items, m.prAction(pr))
	}
	items = append(items, byAct(iconRename, "Rename…", actRename), separator)
	items = append(items, byAct(iconNew, "New thread…", actNew))
	if m.opts.Run != nil && a.Path != "" {
		items = append(items, byAct(iconBranch, "New in a worktree…", actNewWorktree))
	}
	items = append(items, separator)
	closeIt := byAct(iconClose, "Close window…", actClose)
	closeIt.danger = true
	items = append(items, closeIt)
	if _, ok := m.archivable(a); ok {
		archive := byAct(iconSettle, "Archive worktree…", actArchive)
		archive.danger = true
		items = append(items, archive)
	}
	return items
}

// prAction opens a thread's pull request in the browser.
func (m Model) prAction(pr model.PullRequestState) action {
	icon, _ := prIcon(pr)
	return byAct(icon, "Open PR #"+strconv.Itoa(pr.Number), actPR)
}

// snoozeAction opens the snooze times in place of the menu it is in.
func (m Model) snoozeAction() action {
	return action{icon: iconClock, label: "Snooze", do: actSnooze, hint: "›", run: func(m Model) (tea.Model, tea.Cmd) {
		return m.openSnoozeMenu(m.menu.x, m.menu.y-1), nil
	}}
}

// snoozeMenu is the times a thread can be snoozed until, by number.
//
//	╭─ Snooze until ───────╮
//	│ 1 In an hour   11:00 │
//	│ …                    │
//	│ c Other time…        │
//	╰──────────────────────╯
func (m Model) snoozeMenu() []action {
	var items []action
	for i, p := range agent.SnoozePresets(m.now) {
		until := p.Until
		key := string(rune('1' + i))
		items = append(items, action{icon: key, label: p.Label, key: key, hint: when(until, m.now), run: func(m Model) (tea.Model, tea.Cmd) {
			return m.snooze(until)
		}})
	}
	return append(items, separator, action{icon: "c", label: "Other time…", key: "c", hint: " ", run: func(m Model) (tea.Model, tea.Cmd) {
		return m.prompt(modeSnoozeCustom, "")
	}})
}

func (m Model) openThreadMenu(a *model.Agent, x, y int) Model {
	return m.openMenu("", m.threadMenu(a), x, y)
}

func (m Model) openSnoozeMenu(x, y int) Model {
	if a, ok := m.selectedAgent(); !ok || agent.IsSnoozed(a.Thread, m.now) {
		return m
	}
	return m.openMenu("Snooze until", m.snoozeMenu(), x, y)
}

// openMenu opens a popup at x, y: under that line if it fits, else above it,
// and moved left as far as it must be to fit.
func (m Model) openMenu(title string, items []action, x, y int) Model {
	p := popup{title: title, items: items}
	p.w = min(ui.CardWidth(title, m.menuItems(items)), max(m.width, 1))
	p.x = max(min(x, m.width-p.w), 0)
	p.y = y + 1
	if p.y+p.height() > m.height {
		p.y = max(y-p.height(), 0)
	}
	m.menu, m.mode = p, modeMenu
	m.hover, m.hoverButton = "", -1
	return m
}

// right is what a menu shows at the right of an action: its hint, or the
// keys that do it.
func (m Model) right(a action) string {
	switch {
	case a.hint != "":
		return a.hint
	case a.key != "":
		return a.key
	}
	return m.keys.label(a.do)
}

// menuItems are actions as a card draws them.
func (m Model) menuItems(items []action) []ui.MenuItem {
	out := make([]ui.MenuItem, len(items))
	for i, it := range items {
		out[i] = ui.MenuItem{Icon: it.icon, Label: it.label, Right: m.right(it), Danger: it.danger, Sep: it.sep}
	}
	return out
}

func (m Model) pick(i int) (tea.Model, tea.Cmd) {
	it := m.menu.items[i]
	m.mode = modeNormal
	return it.run(m)
}

func (m Model) keyMenu(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	step := func(d int) {
		n := len(m.menu.items)
		for j, i := 0, m.menu.idx; j < n; j++ {
			i = (i + d + n) % n
			if !m.menu.items[i].sep {
				m.menu.idx = i
				return
			}
		}
	}
	switch k := msg.String(); {
	case k == "j" && !m.opts.Picker, k == "down", k == "ctrl+n", k == "tab":
		step(1)
	case k == "k" && !m.opts.Picker, k == "up", k == "ctrl+p", k == "shift+tab":
		step(-1)
	case k == "enter", k == "space", k == "right", k == "l" && !m.opts.Picker:
		return m.pick(m.menu.idx)
	case k == "esc", k == "left", key.Matches(msg, m.keys[actMenu]),
		!m.opts.Picker && (k == "q" || k == "h"):
		m.mode, m.menu = modeNormal, popup{}
	default:
		for i, it := range m.menu.items {
			if it.sep {
				continue
			}
			if it.key != "" && it.key == k {
				return m.pick(i)
			}
			if b, ok := m.keys[it.do]; ok && it.key == "" && it.do != actNone && key.Matches(msg, b) {
				return m.pick(i)
			}
		}
	}
	return m, nil
}

// overlay draws the popup over the screen's lines.
func (m Model) overlay(lines []string) []string {
	p := m.menu
	return ui.Overlay(lines, m.th.Card(p.title, m.menuItems(p.items), p.idx, p.w), p.x, p.y)
}

// buttons are the actions shown on a thread under the mouse, in place of
// its time, as T3 Code shows them: pin, snooze and settle, or what undoes
// one.
func (m Model) buttons(r row) []action {
	if r.kind != rowThread {
		return nil
	}
	a := r.agent
	if a.Closed {
		return []action{byAct(iconResume, "resume", actGo)}
	}
	settle := byAct(iconSettle, "settle", actSettle)
	switch r.section {
	case agent.Snoozed:
		return []action{byAct(iconWake, "wake", actWake), settle}
	case agent.Settled:
		return []action{byAct(iconRestore, "restore", actSettle)}
	}
	pin := byAct(iconPin, "pin", actPin)
	if a.Thread.Pin != 0 {
		pin.label, pin.on = "unpin", true
	}
	snooze := m.snoozeAction()
	snooze.label = "snooze"
	key := rowKey(r)
	snooze.run = func(m Model) (tea.Model, tea.Cmd) {
		return m.openSnoozeMenu(m.width, m.rowY(key)), nil
	}
	if s := agent.Status(a.Status); s == agent.Working || s == agent.Waiting {
		return []action{pin, snooze}
	}
	return []action{pin, snooze, settle}
}

// buttonWidth is the cells a button takes: its icon, padded.
const buttonWidth = 3

// buttonAt is which of a thread's buttons is at column x, -1 for none. They
// sit at the right of the thread's first line.
func (m Model) buttonAt(r row, x int) int {
	n := len(m.buttons(r))
	start := m.cols() - 1 - n*buttonWidth
	if n == 0 || x < start || x >= start+n*buttonWidth {
		return -1
	}
	return (x - start) / buttonWidth
}

// renderButtons draws a thread's buttons on a row of background bg, the one
// under the mouse lit up.
func (m Model) renderButtons(r row, bg string) string {
	back := bg
	if back == "" {
		back = "\033[49m"
	}
	var b strings.Builder
	for i, btn := range m.buttons(r) {
		switch {
		case i == m.hoverButton:
			b.WriteString(m.th.Button + m.th.Accented(" "+btn.icon+" ") + back)
		case btn.on:
			b.WriteString(" " + m.th.Accented(btn.icon) + " ")
		default:
			b.WriteString(" " + m.th.Mute(btn.icon) + " ")
		}
	}
	return b.String() + " "
}

// hoveredButton is the button under the mouse, if any.
func (m Model) hoveredButton() (action, bool) {
	if m.mode != modeNormal || m.hoverButton < 0 {
		return action{}, false
	}
	for _, r := range m.rows() {
		if rowKey(r) == m.hover {
			if btns := m.buttons(r); m.hoverButton < len(btns) {
				return btns[m.hoverButton], true
			}
		}
	}
	return action{}, false
}
