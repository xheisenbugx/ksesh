package sidebar

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/xheisenbugx/ksesh/agent"
	ansicolor "github.com/xheisenbugx/ksesh/ansi"
	"github.com/xheisenbugx/ksesh/model"
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
	// key is the key that does it, shown at the right of a menu; hint, if
	// set, is shown there instead.
	key, hint string
	on        bool // a button showing a state: a pinned thread's pin
	danger    bool
	sep       bool // a line between groups in a menu, not an action
	run       func(Model) (tea.Model, tea.Cmd)
}

// byKey is an action done as its key is.
func byKey(icon, label, key string) action {
	return action{icon: icon, label: label, key: key, run: func(m Model) (tea.Model, tea.Cmd) { return m.run(key) }}
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
		resume := byKey(iconResume, "Resume", "enter")
		resume.hint = "↵"
		forget := byKey(iconForget, "Forget…", "X")
		forget.danger = true
		items := []action{resume}
		if pr, ok := m.pullRequest(a); ok {
			items = append(items, m.prAction(pr))
		}
		return append(items, separator, forget)
	}
	section := agent.SectionOf(a, m.now, m.opts.ThreadOpts)
	status := agent.Status(a.Status)
	items := []action{byKey(iconGo, "Go to thread", "enter")}
	items[0].hint = "↵"

	if section != agent.Snoozed && section != agent.Settled {
		pin := byKey(iconPin, "Pin", "p")
		if a.Thread.Pin != 0 {
			pin.label = "Unpin"
		}
		items = append(items, pin)
		if a.Thread.Pin != 0 && len(m.board()[agent.Pinned]) > 1 {
			items = append(items, byKey(iconUp, "Move up", "K"), byKey(iconDown, "Move down", "J"))
		}
	}
	switch {
	case section == agent.Snoozed:
		items = append(items, byKey(iconWake, "Wake now", "w"))
	case agent.BadgeOf(a, m.now) == agent.BadgeWoke:
		items = append(items, byKey(iconWake, "Dismiss", "w"))
	case section != agent.Settled:
		items = append(items, m.snoozeAction())
	}
	switch {
	case section == agent.Settled:
		items = append(items, byKey(iconRestore, "Restore", "s"))
	case status != agent.Working && status != agent.Waiting:
		items = append(items, byKey(iconSettle, "Settle", "s"))
	}

	items = append(items, separator)
	switch status {
	case agent.Waiting:
		items = append(items, byKey(iconApprove, "Approve", "a"), byKey(iconDeny, "Deny", "x"))
	case agent.Working:
		items = append(items, byKey(iconInterrupt, "Interrupt", "x"))
	}
	items = append(items, byKey(iconReply, "Reply…", "r"), byKey(iconReview, "Review changes", "d"))
	if pr, ok := m.pullRequest(a); ok {
		items = append(items, m.prAction(pr))
	}
	items = append(items, byKey(iconRename, "Rename…", "R"), separator)
	items = append(items, byKey(iconNew, "New thread…", "n"))
	if m.opts.Run != nil && a.Path != "" {
		items = append(items, byKey(iconBranch, "New in a worktree…", "N"))
	}
	items = append(items, separator)
	closeIt := byKey(iconClose, "Close window…", "X")
	closeIt.danger = true
	items = append(items, closeIt)
	if _, ok := m.archivable(a); ok {
		archive := byKey(iconSettle, "Archive worktree…", "A")
		archive.danger = true
		items = append(items, archive)
	}
	return items
}

// prAction opens a thread's pull request in the browser.
func (m Model) prAction(pr model.PullRequestState) action {
	icon, _ := prIcon(pr)
	return byKey(icon, "Open PR #"+strconv.Itoa(pr.Number), "P")
}

// snoozeAction opens the snooze times in place of the menu it is in.
func (m Model) snoozeAction() action {
	return action{icon: iconClock, label: "Snooze", key: "z", hint: "›", run: func(m Model) (tea.Model, tea.Cmd) {
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
	inner := ansi.StringWidth(title) + 4
	for _, it := range items {
		if !it.sep {
			inner = max(inner, 1+ansi.StringWidth(it.icon)+2+ansi.StringWidth(it.label)+3+ansi.StringWidth(it.right())+1)
		}
	}
	p.w = min(inner+2, max(m.width, 1))
	p.x = max(min(x, m.width-p.w), 0)
	p.y = y + 1
	if p.y+p.height() > m.height {
		p.y = max(y-p.height(), 0)
	}
	m.menu, m.mode = p, modeMenu
	m.hover, m.hoverButton = "", -1
	return m
}

func (a action) right() string {
	if a.hint != "" {
		return a.hint
	}
	return a.key
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
	switch key := msg.String(); key {
	case "j", "down", "ctrl+n", "tab":
		step(1)
	case "k", "up", "ctrl+p", "shift+tab":
		step(-1)
	case "enter", "space", "l", "right":
		return m.pick(m.menu.idx)
	case "esc", "q", "h", "left", "m":
		m.mode, m.menu = modeNormal, popup{}
	default:
		for i, it := range m.menu.items {
			if !it.sep && it.key == key {
				return m.pick(i)
			}
		}
	}
	return m, nil
}

// renderMenu draws the popup's lines, each m.menu.w wide.
func (m Model) renderMenu() []string {
	p := m.menu
	inner := p.w - 2
	border := func(s string) string { return m.th.surface + m.th.accent + s + "\033[39m" }
	top := "─" + strings.Repeat("─", inner-1)
	if p.title != "" {
		t := ansi.Truncate(" "+p.title+" ", inner-1, "…")
		top = "─" + m.th.heading + bold(t) + m.th.accent + strings.Repeat("─", max(inner-1-ansi.StringWidth(t), 0))
	}
	lines := []string{border("╭" + top + "╮")}
	for i, it := range p.items {
		if it.sep {
			lines = append(lines, border("├"+m.th.subtle+strings.Repeat("─", inner)+m.th.accent+"┤"))
			continue
		}
		bg, label, icon := m.th.surface, it.label, m.th.mute(it.icon)
		if it.danger {
			label = fg(ansicolor.Red, label)
		}
		if i == p.idx {
			bg, label, icon = m.th.selected, bold(label), m.th.accented(it.icon)
		}
		left := " " + icon + "  " + label
		right := m.th.mute(it.right()) + " "
		gap := max(inner-ansi.StringWidth(left)-ansi.StringWidth(right), 1)
		content := ansi.Truncate(left+strings.Repeat(" ", gap)+right, inner, "…")
		lines = append(lines, border("│")+bg+content+border("│"))
	}
	lines = append(lines, border("╰"+strings.Repeat("─", inner)+"╯"))
	for i := range lines {
		lines[i] = "\033[0m" + lines[i] + "\033[0m"
	}
	return lines
}

// overlay draws the popup over the screen's lines.
func (m Model) overlay(lines []string) []string {
	p := m.menu
	for i, l := range m.renderMenu() {
		y := p.y + i
		if y < 0 || y >= len(lines) {
			continue
		}
		base := lines[y]
		if w := ansi.StringWidth(base); w < p.x+p.w {
			base += strings.Repeat(" ", p.x+p.w-w)
		}
		lines[y] = ansi.Truncate(base, p.x, "") + l + ansi.TruncateLeft(base, p.x+p.w, "")
	}
	return lines
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
		return []action{byKey(iconResume, "resume", "enter")}
	}
	settle := byKey(iconSettle, "settle", "s")
	switch r.section {
	case agent.Snoozed:
		return []action{byKey(iconWake, "wake", "w"), settle}
	case agent.Settled:
		return []action{byKey(iconRestore, "restore", "s")}
	}
	pin := byKey(iconPin, "pin", "p")
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
	start := m.width - 1 - n*buttonWidth
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
			b.WriteString(m.th.button + m.th.accented(" "+btn.icon+" ") + back)
		case btn.on:
			b.WriteString(" " + m.th.accented(btn.icon) + " ")
		default:
			b.WriteString(" " + m.th.mute(btn.icon) + " ")
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
