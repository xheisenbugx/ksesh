package sidebar

import (
	"cmp"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/xheisenbugx/ksesh/agent"
	ansicolor "github.com/xheisenbugx/ksesh/ansi"
	"github.com/xheisenbugx/ksesh/model"
)

// headerLines are the title, the line under it and a blank one.
const headerLines = 3

// newButtonWidth is the cells of the new thread button at the right end of
// the title.
const newButtonWidth = 3

// onNewButton reports whether x, y of the screen is on the new thread button.
func (m Model) onNewButton(x, y int) bool {
	return y == 0 && x >= m.width-newButtonWidth && x < m.width
}

// Icons beyond the status ones in agent.GlyphFor: single-cell Font Awesome
// glyphs from a nerd font, like those.
const (
	iconPin    = "" // thumbtack
	iconInbox  = "" // inbox
	iconClock  = "" // clock: snoozed, and back from a snooze
	iconSettle = "" // archive
	iconDone   = "" // check in a circle
	iconRobot  = "" // robot
)

// here marks the thread you're on, and the closed shelf hiding it;
// unreadMark a thread that finished while you weren't looking.
const (
	here       = "●"
	unreadMark = "•"
)

// spinner animates the icon of working threads.
var spinner = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

const spinEvery = 100 * time.Millisecond

// SGR pieces that only undo what they set, so a row's background survives
// the colors drawn on top of it.
func fg(code int, s string) string { return fmt.Sprintf("\033[%dm%s\033[39m", code, s) }
func bold(s string) string         { return "\033[1m" + s + "\033[22m" }

// paint pads a line to the full width, on a background if one is given.
func paint(content string, bg string, width int) string {
	content = ansi.Truncate(content, width, "…")
	pad := strings.Repeat(" ", max(width-ansi.StringWidth(content), 0))
	if bg == "" {
		return content + pad
	}
	return bg + content + pad + "\033[49m"
}

func (m Model) listHeight() int {
	return max(m.height-headerLines-len(m.footer()), 0)
}

// lineRows maps every line of the list to the row drawn on it, with the
// fill expanded to the room left over.
func (m Model) lineRows() []int {
	rows := m.rows()
	fixed := 0
	for _, r := range rows {
		fixed += r.height()
	}
	var lines []int
	for i, r := range rows {
		n := r.height()
		if r.kind == rowFill {
			n = max(m.listHeight()-fixed, 1)
		}
		for range n {
			lines = append(lines, i)
		}
	}
	return lines
}

func (m *Model) scrollToCursor() {
	if m.height == 0 {
		return
	}
	lines := m.lineRows()
	room := m.listHeight()
	first, last := -1, -1
	for i, r := range lines {
		if r == m.cursorIdx {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	if first < 0 {
		m.offset = 0
		return
	}
	if first < m.offset {
		m.offset = first
	}
	if last >= m.offset+room {
		m.offset = last - room + 1
	}
	m.offset = max(min(m.offset, len(lines)-room), 0)
}

func (m Model) View() tea.View {
	out := m.title()
	out = append(out, "")
	if m.mode == modeHelp {
		help := m.helpLines()
		for i := range m.listHeight() {
			line := ""
			if i < len(help) {
				line = paint(help[i], "", m.width)
			}
			out = append(out, line)
		}
	} else {
		rows := m.rows()
		lines := m.lineRows()
		drawn := map[int][]string{}
		for i := m.offset; i < m.offset+m.listHeight(); i++ {
			if i >= len(lines) {
				out = append(out, "")
				continue
			}
			ri := lines[i]
			if _, ok := drawn[ri]; !ok {
				drawn[ri] = m.renderRow(rows[ri])
			}
			// Which of the row's lines this is.
			nth := 0
			for j := i - 1; j >= 0 && lines[j] == ri; j-- {
				nth++
			}
			line := ""
			if nth < len(drawn[ri]) {
				line = drawn[ri][nth]
			}
			out = append(out, line)
		}
	}
	out = append(out, m.footer()...)
	if m.mode == modeMenu {
		out = m.overlay(out)
	}
	v := tea.NewView(strings.Join(out, "\n"))
	v.AltScreen = true
	v.MouseMode = tea.MouseModeAllMotion
	v.ReportFocus = true
	v.WindowTitle = "threads"
	return v
}

// title is the sidebar's masthead, in the manner of Crush's: the name on a
// gradient running out into stripes, a + for a new thread, and under it the
// scope and what needs you, by kind.
//
//	Threads ╱╱╱╱╱╱╱╱╱╱╱╱╱╱╱╱╱╱╱╱╱╱  +
//	all sessions · 6             1  1
func (m Model) title() []string {
	name := " Threads "
	// At the right end, the new thread button.
	stripes := strings.Repeat("╱", max(m.width-ansi.StringWidth(name)-newButtonWidth, 0))
	plus := " " + m.th.accented(iconNew) + " "
	if m.hoverNew {
		plus = m.th.button + plus + "\033[49m"
	}
	top := m.th.gradient(name+stripes, len([]rune(name))) + plus

	scope := "all sessions"
	if m.state.Session && m.session != "" {
		scope = m.session
	}
	left := " " + m.th.mute(scope)
	if m.loaded {
		left += m.th.dim(" · ") + m.th.mute(strconv.Itoa(len(m.agents)))
	}
	var right []string
	switch {
	case m.err != nil:
		right = append(right, fg(ansicolor.Red, "kitty unreachable"))
	case m.loaded:
		counts := map[agent.Badge]int{}
		for _, a := range m.board().Ordered(agent.Pinned, agent.Active) {
			counts[agent.BadgeOf(a, m.now)]++
		}
		for _, badge := range []agent.Badge{agent.Badge(agent.Waiting), agent.Badge(agent.Error), agent.BadgeWoke, agent.Badge(agent.Done)} {
			if n := counts[badge]; n > 0 {
				icon, color := badgeIcon(badge)
				right = append(right, fg(color, icon+" "+strconv.Itoa(n)))
			}
		}
	}
	return []string{paint(top, "", m.width), spread(left, strings.Join(right, "  "), m.width)}
}

// spread puts left and right at the two ends of a line width wide, cutting
// left when they don't both fit.
func spread(left, right string, width int) string {
	if right == "" {
		return paint(left, "", width)
	}
	right += " "
	lw, rw := ansi.StringWidth(left), ansi.StringWidth(right)
	if lw+1+rw > width {
		left = ansi.Truncate(left, max(width-rw-1, 0), "…")
		lw = ansi.StringWidth(left)
	}
	return left + strings.Repeat(" ", max(width-lw-rw, 1)) + right
}

// background is a row's: the selected row's while the sidebar has the
// focus, the hovered row's under the mouse.
func (m Model) background(r row) string {
	key := rowKey(r)
	switch {
	case key == "":
		return ""
	case m.focused && key == m.cursor:
		return m.th.selected
	case key == m.hover:
		return m.th.hover
	}
	return ""
}

// bar is a row's left edge: an accent bar on the selected row.
func (m Model) bar(r row) string {
	if m.focused && rowKey(r) != "" && rowKey(r) == m.cursor {
		return m.th.accented("▌")
	}
	return " "
}

func (m Model) renderRow(r row) []string {
	bg := m.background(r)
	switch r.kind {
	case rowFill:
		return []string{paint("", "", m.width)}
	case rowRule:
		return []string{" " + m.th.dim(strings.Repeat("─", max(m.width-2, 0))) + " "}
	case rowEmpty:
		return m.renderEmpty(r)
	case rowMore:
		return []string{paint(m.bar(r)+"   "+m.th.mute(fmt.Sprintf("⋯ %d more", r.count)), bg, m.width)}
	case rowHeader:
		return []string{m.renderHeader(r, bg)}
	}
	return m.renderThread(r, bg)
}

// renderEmpty is the inbox with nothing in it, centered.
func (m Model) renderEmpty(r row) []string {
	icon, head, sub := fg(ansicolor.Green, iconDone), "All caught up", "nothing needs you"
	if r.none {
		icon, head, sub = m.th.mute(iconRobot), "No agents yet", "n starts one here"
	}
	center := func(s string) string {
		pad := max((m.width-ansi.StringWidth(s))/2, 0)
		return paint(strings.Repeat(" ", pad)+s, "", m.width)
	}
	return []string{"", center(icon + " " + bold(head)), center(m.th.mute(sub)), ""}
}

var sectionNames = map[agent.Section]string{
	agent.Pinned:         "Pinned",
	agent.Active:         "Inbox",
	agent.SectionWorking: "Working",
	agent.Snoozed:        "Snoozed",
	agent.Settled:        "Settled",
}

// renderHeader draws a section's title. Pinned and Inbox are labels with a
// rule running to their count; the shelves are rows of their own, with a
// caret.
//
//	 PINNED ───────────────────── 2
//	▌▸ ⠹ Working                ● 1
func (m Model) renderHeader(r row, bg string) string {
	name := sectionNames[r.section]
	if !isShelf(r.section) {
		icon := iconPin
		if r.section == agent.Active {
			icon = iconInbox
		}
		label := " " + m.th.mute(icon) + " " + m.th.head(strings.ToUpper(name)) + " "
		count := " " + m.th.mute(strconv.Itoa(r.count)) + " "
		rule := max(m.width-ansi.StringWidth(label)-ansi.StringWidth(count), 0)
		return paint(label+m.th.dim(strings.Repeat("─", rule))+count, bg, m.width)
	}
	caret := "▸"
	if m.state.Expanded[r.section] {
		caret = "▾"
	}
	var icon string
	switch r.section {
	case agent.SectionWorking:
		icon = fg(ansicolor.Blue, spinner[m.frame%len(spinner)])
	case agent.Snoozed:
		icon = m.th.mute(iconClock)
	case agent.Settled:
		icon = m.th.mute(iconSettle)
	}
	right := m.th.mute(strconv.Itoa(r.count))
	if r.holdsCurrent {
		right = m.th.accented(here) + " " + right
	}
	left := m.bar(r) + m.th.mute(caret) + " " + icon + " " + name
	return paint(spread(left, right, m.width), bg, m.width)
}

func badgeIcon(badge agent.Badge) (string, int) {
	if badge == agent.BadgeWoke {
		return iconClock, ansicolor.Magenta
	}
	if g, ok := agent.GlyphFor(agent.Status(badge)); ok {
		return g.Icon, g.ColorCode
	}
	return " ", 0
}

// renderThread draws a thread on two lines and a blank one, to breathe:
//
//	▌ ⠹ Fix the login flow        2m ●
//	    api · Allow Bash: npm test
//	 •✓ Write the README          5m
//
// The accent bar is the selection, the dot at the right the thread you're
// on, the one at the left a thread that finished while you weren't looking
// (unread); the title is bold when it needs you, muted when put away
// (snoozed, settled).
func (m Model) renderThread(r row, bg string) []string {
	a, section := r.agent, r.section
	badge := agent.BadgeOf(a, m.now)
	icon, color := badgeIcon(badge)
	if badge == agent.Badge(agent.Working) {
		icon = spinner[m.frame%len(spinner)]
	}
	putAway := section == agent.Snoozed || section == agent.Settled
	switch {
	case putAway && section == agent.Snoozed:
		icon, color = iconClock, 0
	case putAway:
		color = 0
	}
	iconText := m.th.mute(icon)
	if color != 0 {
		iconText = fg(color, icon)
	}

	dot := " "
	if a.WindowID == m.current {
		dot = m.th.accented(here)
	}
	bar := m.bar(r)
	// At the right, the time and the dot; under the mouse, the buttons.
	right := m.th.mute(m.meta(a, section)) + " " + dot + " "
	if m.mode == modeNormal && rowKey(r) == m.hover {
		right = m.renderButtons(r, bg)
	}
	// bar, space, icon, space | title | space, right
	room := max(m.width-4-1-ansi.StringWidth(right), 1)
	name := ansi.Truncate(title(a), room, "…")
	unread := !putAway && agent.IsUnread(a)
	needsYou := unread || badge == agent.Badge(agent.Waiting) || badge == agent.Badge(agent.Error) || badge == agent.BadgeWoke
	switch {
	case putAway:
		name = m.th.mute(name)
	case needsYou:
		name = bold(name)
	}
	mark := " "
	if unread {
		mark = m.th.accented(unreadMark)
	}
	gap := strings.Repeat(" ", max(room-ansi.StringWidth(ansi.Strip(name)), 0)+1)
	first := bar + mark + iconText + " " + name + gap + right
	second := bar + "   " + m.subtitle(a, section, badge)
	return []string{paint(first, bg, m.width), paint(second, bg, m.width), paint("", "", m.width)}
}

// minSubtitle is the least of a thread's session and what it is on that the
// branch at the right of its second line leaves.
const minSubtitle = 10

// subtitle is a thread's second line: its session, and what it needs, did or
// is on, then at the right the branch it is on, when that fits.
//
//	api · Added a Threads section   docs ↑2  #12
func (m Model) subtitle(a *model.Agent, section agent.Section, badge agent.Badge) string {
	where := a.Session
	if where == title(a) {
		where = ""
	}
	what, color := "", 0
	status := agent.Status(a.Status)
	switch {
	case section == agent.Snoozed:
		what = "until " + when(a.Thread.SnoozedUntil, m.now)
	case section == agent.Settled:
		what = "settled"
		if a.Message != "" {
			what = a.Message
		}
	case badge == agent.BadgeWoke:
		what, color = "back from snooze", ansicolor.Magenta
	case status == agent.Waiting:
		what, color = cmpOr(a.Message, "waiting on you"), ansicolor.Yellow
	case status == agent.Error:
		what, color = cmpOr(a.Message, "failed"), ansicolor.Red
	case status == agent.Working && a.Prompt != "":
		what = "› " + a.Prompt
	case a.Message != "":
		what = a.Message
	case a.Prompt != "":
		what = "› " + a.Prompt
	}
	room := max(m.width-5, 1)
	if where != "" && what != "" {
		where += " · "
	}
	// The branch goes at the right, when there is room for it: what the
	// agent needs comes first, and anything else is cut to make room, down
	// to a few cells.
	label := m.branchLabel(a)
	if label != "" {
		need := ansi.StringWidth(label) + 2
		full := ansi.StringWidth(where) + ansi.StringWidth(what)
		switch {
		case full+need <= room:
		case color != 0 || room-need < minSubtitle:
			label = ""
		}
		if label != "" {
			room -= need
		}
	}
	// Where it runs stays quiet; only what it needs is colored.
	where = ansi.Truncate(where, room, "…")
	what = ansi.Truncate(what, max(room-ansi.StringWidth(where), 0), "…")
	out := ""
	if where != "" {
		out = m.th.mute(where)
	}
	if what != "" {
		if color != 0 {
			out += fg(color, what)
		} else {
			out += m.th.mute(what)
		}
	}
	if label != "" {
		out += strings.Repeat(" ", max(room-ansi.StringWidth(out), 0)+2) + label
	}
	return out
}

func cmpOr(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// when is a time to come, as briefly as it can be said: 15:04 today, Mon
// 09:00 this week, Jan 2 after.
func when(t, now time.Time) string {
	switch {
	case t.YearDay() == now.YearDay() && t.Year() == now.Year():
		return t.Format("15:04")
	case t.Sub(now) < 6*24*time.Hour:
		return t.Format("Mon 15:04")
	}
	return t.Format("Jan 2")
}

// meta is the time at the right of a row: how long until a snoozed thread
// wakes, how long ago the others did something or were settled.
func (m Model) meta(a *model.Agent, section agent.Section) string {
	switch section {
	case agent.Snoozed:
		return "in " + short(a.Thread.SnoozedUntil.Sub(m.now))
	case agent.Settled:
		at, _ := agent.SettledSince(a, m.now, m.opts.ThreadOpts)
		return short(m.now.Sub(at))
	}
	if last := agent.LastActivity(a); !last.IsZero() {
		return short(m.now.Sub(last))
	}
	return ""
}

// short renders a duration in its largest unit: 45s, 12m, 3h, 2d.
func short(d time.Duration) string {
	switch {
	case d < time.Minute:
		return strconv.Itoa(max(int(d.Seconds()), 0)) + "s"
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())) + "m"
	case d < 24*time.Hour:
		return strconv.Itoa(int(d.Hours())) + "h"
	}
	return strconv.Itoa(int(d.Hours()/24)) + "d"
}

// title is what a thread is called: see agent.Title.
func title(a *model.Agent) string { return agent.Title(a) }

// hint is a key and what it does, for the footer.
type hint struct{ key, does string }

// hints are the keys worth showing for what is selected, most useful first.
func (m Model) hints() []hint {
	r, _, ok := m.selected()
	if !ok {
		return []hint{{"n", "new thread"}, {"?", "keys"}}
	}
	switch r.kind {
	case rowHeader:
		does := "open"
		if m.state.Expanded[r.section] {
			does = "close"
		}
		return []hint{{"↵", does}, {"?", "keys"}}
	case rowMore:
		return []hint{{"↵", "show all"}, {"?", "keys"}}
	}
	a := r.agent
	switch {
	case r.section == agent.Snoozed:
		return []hint{{"↵", "go"}, {"w", "wake"}, {"u", "undo"}, {"?", "keys"}}
	case r.section == agent.Settled:
		return []hint{{"↵", "go"}, {"s", "restore"}, {"u", "undo"}, {"?", "keys"}}
	}
	pin := "pin"
	if a.Thread.Pin != 0 {
		pin = "unpin"
	}
	switch agent.Status(a.Status) {
	case agent.Waiting:
		return []hint{{"a", "approve"}, {"x", "deny"}, {"r", "reply"}, {"↵", "go"}, {"z", "snooze"}, {"?", "keys"}}
	case agent.Working:
		return []hint{{"↵", "go"}, {"x", "stop"}, {"r", "reply"}, {"p", pin}, {"?", "keys"}}
	}
	if agent.BadgeOf(a, m.now) == agent.BadgeWoke {
		return []hint{{"↵", "go"}, {"w", "dismiss"}, {"s", "settle"}, {"?", "keys"}}
	}
	if agent.Status(a.Status) == agent.Done {
		return []hint{{"↵", "go"}, {"d", "review"}, {"s", "settle"}, {"p", pin}, {"?", "keys"}}
	}
	return []hint{{"↵", "go"}, {"s", "settle"}, {"p", pin}, {"z", "snooze"}, {"d", "review"}, {"?", "keys"}}
}

// key is a key as the footer and the help show it.
func (m Model) key(k string) string { return m.th.accented(bold(k)) }

// hintLine fits as many hints as there is room for, keeping the last one
// ("? keys") so the rest can always be found.
func (m Model) hintLine(hints []hint) string {
	render := func(hs []hint) string {
		parts := make([]string, len(hs))
		for i, h := range hs {
			parts[i] = m.key(h.key) + " " + m.th.mute(h.does)
		}
		return " " + strings.Join(parts, m.th.dim(" · "))
	}
	for len(hints) > 1 {
		if line := render(hints); ansi.StringWidth(line) <= m.width {
			return line
		}
		hints = append(hints[:len(hints)-2:len(hints)-2], hints[len(hints)-1])
	}
	return render(hints)
}

// footer is the status bar under the list: a rule, then the keys for what is
// selected, the menu or prompt open, or what was just done.
func (m Model) footer() []string {
	line := func(s string) string { return paint(s, "", m.width) }
	lines := []string{" " + m.th.dim(strings.Repeat("─", max(m.width-2, 0))) + " "}
	if in, ok := inputs[m.mode]; ok {
		return append(lines, line(" "+m.th.accented(bold(cmp.Or(m.ask, in.label)))), line(" "+m.input.View()))
	}
	switch m.mode {
	case modeSnooze:
		lines = append(lines, line(" "+m.th.accented(bold("Snooze until"))))
		for i, p := range agent.SnoozePresets(m.now) {
			lines = append(lines, spread(" "+m.key(strconv.Itoa(i+1))+" "+p.Label, m.th.mute(when(p.Until, m.now)), m.width))
		}
		return append(lines, line(" "+m.key("c")+" "+m.th.mute("other time")+m.th.dim(" · ")+m.key("esc")+" "+m.th.mute("cancel")))
	case modeMenu:
		return append(lines, m.hintLine([]hint{{"↵", "pick"}, {"j k", "move"}, {"esc", "close"}}))
	case modeConfirmClose:
		return append(lines, line(" "+fg(ansicolor.Red, "Close the agent's window?")+" "+m.key("y")+m.th.mute("/n")))
	case modeConfirmArchive:
		return append(lines, line(" "+fg(ansicolor.Red, "Remove its worktree, branch")), line(" "+fg(ansicolor.Red, "and window?")+" "+m.key("y")+m.th.mute("/n")))
	}
	if m.flash != "" && m.opts.Now().Before(m.until) {
		if m.failed {
			// What went wrong is worth reading whole: on up to three lines.
			wrapped := strings.Split(ansi.Wrap(m.flash, max(m.width-4, 1), ""), "\n")
			for i, l := range wrapped[:min(len(wrapped), 3)] {
				mark := "  "
				if i == 0 {
					mark = fg(ansicolor.Red, "✗") + " "
				}
				lines = append(lines, line(" "+mark+l))
			}
			return lines
		}
		return append(lines, line(" "+fg(ansicolor.Green, "✓")+" "+m.flash))
	}
	if b, ok := m.hoveredButton(); ok {
		return append(lines, line(" "+m.th.accented(b.icon)+"  "+b.label+"  "+m.key(b.key)))
	}
	if m.hoverNew && m.mode == modeNormal {
		return append(lines, line(" "+m.th.accented(iconNew)+"  new thread  "+m.key("n")))
	}
	if !m.focused {
		return append(lines, line(""))
	}
	return append(lines, m.hintLine(m.hints()))
}

// helpLines is the help screen, shown in place of the list.
func (m Model) helpLines() []string {
	section := func(s string) string { return " " + m.th.head(strings.ToUpper(s)) }
	entry := func(keys, does string) string {
		return "  " + m.key(keys) + strings.Repeat(" ", max(9-ansi.StringWidth(keys), 1)) + does
	}
	return []string{
		section("Threads"),
		entry("↵ l", "go to the thread"),
		entry("esc h", "back to your window"),
		entry("j k g G", "move"),
		entry("tab", "next section"),
		entry("n N", "new thread, in a worktree"),
		"",
		section("Triage"),
		entry("p", "pin, unpin"),
		entry("K J", "move a pin up, down"),
		entry("s", "settle, restore"),
		entry("z w", "snooze, wake"),
		entry("u", "undo"),
		entry("m", "menu, or right click"),
		"",
		section("The agent"),
		entry("a", "approve"),
		entry("x", "deny, interrupt"),
		entry("r", "reply"),
		entry("d", "review its changes"),
		entry("R", "rename the thread"),
		entry("X", "close its window"),
		entry("A", "archive: remove its worktree"),
		"",
		section("Sidebar"),
		entry("f", "this session, all"),
		entry("q", "hide everywhere"),
		"",
		"  " + m.th.mute("any key to go back"),
	}
}
