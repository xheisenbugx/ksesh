package sidebar

import (
	"charm.land/bubbles/v2/key"
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
	"github.com/xheisenbugx/ksesh/ui"
)

// top is the lines over the list: the title, the line under it and a blank
// one, and in the picker the filter between those.
func (m Model) top() int {
	if m.opts.Picker {
		return 4
	}
	return 3
}

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
	iconClosed = "" // history: a closed thread, to resume
)

// iconFolder stands for a session, over its threads in the grouped inbox;
// iconSearch for the filter.
const (
	iconFolder = ""
	iconSearch = ""
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

func (m Model) listHeight() int {
	return max(m.height-m.top()-len(m.footer()), 0)
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
	if m.opts.Picker {
		out = append(out, ui.Paint(" "+m.query.View(), "", m.width))
	}
	out = append(out, "")
	if m.mode == modeHelp {
		help := m.helpLines()
		for i := range m.listHeight() {
			line := ""
			if j := i + m.helpOffset; j < len(help) {
				line = ui.Paint(help[j], "", m.width)
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
		if m.previewShown() {
			out = m.besidePreview(out)
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
	if m.opts.Picker {
		v.WindowTitle = "agents"
	}
	return v
}

// title is the sidebar's masthead, in the manner of Crush's: the name on a
// gradient running out into stripes, a + for a new thread, and under it the
// scope and what needs you, by kind.
//
//	Threads ╱╱╱╱╱╱╱╱╱╱╱╱╱╱╱╱╱╱╱╱╱╱  +
//	all sessions · 6             1  1
func (m Model) title() []string {
	// At the right end, the new thread button.
	plus := " " + m.th.Accented(iconNew) + " "
	if m.hoverNew {
		plus = m.th.Button + plus + "\033[49m"
	}
	top := m.th.Masthead(m.name(), m.width, plus)

	scope := "all sessions"
	if m.state.Session && m.session != "" {
		scope = m.session
	}
	left := " " + m.th.Mute(scope)
	switch {
	case m.loaded && m.filter != "":
		// How many of them the filter keeps.
		left += m.th.Dim(" · ") + m.th.Accented("/"+m.filter) + " " + m.th.Mute(strconv.Itoa(len(m.board().Ordered(agent.Sections...))))
	case m.loaded:
		live := 0
		for _, a := range m.agents {
			if !a.Closed {
				live++
			}
		}
		left += m.th.Dim(" · ") + m.th.Mute(strconv.Itoa(live))
	}
	var right []string
	switch {
	case m.err != nil:
		right = append(right, ui.Fg(ansicolor.Red, "kitty unreachable"))
	case m.loaded:
		counts := map[agent.Badge]int{}
		for _, a := range m.board().Ordered(agent.Pinned, agent.Active) {
			counts[agent.BadgeOf(a, m.now)]++
		}
		for _, badge := range []agent.Badge{agent.Badge(agent.Waiting), agent.Badge(agent.Error), agent.BadgeWoke, agent.Badge(agent.Done)} {
			if n := counts[badge]; n > 0 {
				icon, color := badgeIcon(badge)
				right = append(right, ui.Fg(color, icon+" "+strconv.Itoa(n)))
			}
		}
	}
	return []string{top, ui.Spread(left, strings.Join(right, "  "), m.width)}
}

// background is a row's: the selected row's while the sidebar has the
// focus, the hovered row's under the mouse.
func (m Model) background(r row) string {
	key := rowKey(r)
	switch {
	case key == "":
		return ""
	case m.focused && key == m.cursor:
		return m.th.Selected
	case key == m.hover:
		return m.th.Hover
	}
	return ""
}

// bar is a row's left edge: an accent bar on the selected row.
func (m Model) bar(r row) string {
	return m.th.Bar(m.focused && rowKey(r) != "" && rowKey(r) == m.cursor)
}

func (m Model) renderRow(r row) []string {
	bg := m.background(r)
	switch r.kind {
	case rowFill:
		return []string{ui.Paint("", "", m.cols())}
	case rowRule:
		return []string{m.th.Rule(m.cols())}
	case rowEmpty:
		return m.renderEmpty(r)
	case rowMore:
		return []string{ui.Paint(m.bar(r)+"   "+m.th.Mute(fmt.Sprintf("⋯ %d more", r.count)), bg, m.cols())}
	case rowHeader:
		return []string{m.renderHeader(r, bg)}
	case rowGroup:
		return []string{m.renderGroup(r)}
	}
	return m.renderThread(r, bg)
}

// renderEmpty is the inbox with nothing in it, centered.
func (m Model) renderEmpty(r row) []string {
	icon, head, sub := ui.Fg(ansicolor.Green, iconDone), "All caught up", "nothing needs you"
	switch {
	case m.filter != "":
		icon, head, sub = m.th.Mute(iconSearch), "Nothing in the inbox", "matches "+m.filterLabel()+", esc clears it"
	case r.none:
		icon, head, sub = m.th.Mute(iconRobot), "No agents yet", m.keys.label(actNew)+" starts one here"
	}
	return []string{"", ui.Center(icon+" "+ui.Bold(head), m.cols()), ui.Center(m.th.Mute(sub), m.cols()), ""}
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
		return m.th.Section(icon, strings.ToUpper(name), r.count, m.cols(), bg)
	}
	caret := "▸"
	if m.expanded()[r.section] {
		caret = "▾"
	}
	var icon string
	switch r.section {
	case agent.SectionWorking:
		icon = ui.Fg(ansicolor.Blue, spinner[m.frame%len(spinner)])
	case agent.Snoozed:
		icon = m.th.Mute(iconClock)
	case agent.Settled:
		icon = m.th.Mute(iconSettle)
	}
	right := m.th.Mute(strconv.Itoa(r.count))
	if r.holdsCurrent {
		right = m.th.Accented(here) + " " + right
	}
	left := m.bar(r) + m.th.Mute(caret) + " " + icon + " " + name
	return ui.Paint(ui.Spread(left, right, m.cols()), bg, m.cols())
}

// renderGroup names the session of the inbox threads under it, its rule
// running to their count:
//
//	api ─────────────────── 2
func (m Model) renderGroup(r row) string {
	name := cmpOr(r.session, "no session")
	return m.th.LabelRule("  "+m.th.Mute(iconFolder)+" "+m.th.Head(name)+" ", r.count, m.cols(), "")
}

func badgeIcon(badge agent.Badge) (string, int) {
	switch badge {
	case agent.BadgeWoke:
		return iconClock, ansicolor.Magenta
	case agent.BadgeClosed:
		return iconClosed, 0
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
	iconText := m.th.Mute(icon)
	if color != 0 {
		iconText = ui.Fg(color, icon)
	}

	dot := " "
	if !a.Closed && a.WindowID == m.current {
		dot = m.th.Accented(here)
	}
	bar := m.bar(r)
	// At the right, the time and the dot; under the mouse, the buttons.
	right := m.th.Mute(m.meta(a, section)) + " " + dot + " "
	if m.mode == modeNormal && rowKey(r) == m.hover {
		right = m.renderButtons(r, bg)
	}
	// bar, space, icon, space | title | space, right
	room := max(m.cols()-4-1-ansi.StringWidth(right), 1)
	name := ansi.Truncate(title(a), room, "…")
	unread := !putAway && agent.IsUnread(a)
	needsYou := unread || badge == agent.Badge(agent.Waiting) || badge == agent.Badge(agent.Error) || badge == agent.BadgeWoke
	switch {
	case putAway:
		name = m.th.Mute(name)
	case needsYou:
		name = ui.Bold(name)
	}
	mark := " "
	if unread {
		mark = m.th.Accented(unreadMark)
	}
	gap := strings.Repeat(" ", max(room-ansi.StringWidth(ansi.Strip(name)), 0)+1)
	first := bar + mark + iconText + " " + name + gap + right
	second := bar + "   " + m.subtitle(a, section, badge)
	return []string{ui.Paint(first, bg, m.cols()), ui.Paint(second, bg, m.cols()), ui.Paint("", "", m.cols())}
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
	// Grouped, the inbox says each thread's session once, over them.
	if where == title(a) || (m.state.Group && section == agent.Active) {
		where = ""
	}
	what, color := "", 0
	status := agent.Status(a.Status)
	switch {
	case section == agent.Snoozed:
		what = "until " + when(a.Thread.SnoozedUntil, m.now)
	case a.Closed:
		what = "closed"
		if a.Message != "" {
			what += " · " + a.Message
		} else if a.Prompt != "" {
			what += " · › " + a.Prompt
		}
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
	room := max(m.cols()-5, 1)
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
		out = m.th.Mute(where)
	}
	if what != "" {
		if color != 0 {
			out += ui.Fg(color, what)
		} else {
			out += m.th.Mute(what)
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

// hints are the keys worth showing for what is selected, most useful first,
// the key to the actions menu (on a thread) or to the help last. The picker always offers its diff, which shows
// beside the list.
func (m Model) hints() []key.Binding {
	hints := m.threadHints()
	if !m.opts.Picker || len(hints) < 2 {
		return hints
	}
	if _, ok := m.selectedAgent(); !ok {
		return hints
	}
	diff := m.keys.says(actReview, "diff")
	if m.previewDiff {
		diff = m.keys.says(actReview, "screen")
	}
	for _, h := range hints {
		if h.Help().Key == diff.Help().Key {
			diff.SetEnabled(false)
		}
	}
	extra := []key.Binding{diff}
	if a, _ := m.selectedAgent(); agent.Status(a.Status) == agent.Done || agent.Status(a.Status) == agent.Error {
		extra = append(extra, m.keys.says(actSeen, "seen"))
	}
	last := len(hints) - 1
	return append(append(hints[:last:last], extra...), hints[last])
}

func (m Model) threadHints() []key.Binding {
	k := m.keys
	help := k.says(actHelp, "keys")
	r, _, ok := m.selected()
	if !ok {
		return []key.Binding{k.says(actNew, "new thread"), help}
	}
	switch r.kind {
	case rowHeader:
		does := "open"
		if m.expanded()[r.section] {
			does = "close"
		}
		return []key.Binding{k.says(actGo, does), help}
	case rowMore:
		return []key.Binding{k.says(actGo, "show all"), help}
	}
	// On a thread the last hint is its actions menu, which has the rest,
	// the help among them. The sidebar names m for it, as short as ?, not
	// space, its leader, which the help shows.
	more := k.says(actMenu, "more")
	a := r.agent
	_, hasPR := m.pullRequest(a)
	switch {
	case a.Closed:
		if hasPR {
			return []key.Binding{k.says(actGo, "resume"), k.says(actPR, "pr"), k.says(actClose, "forget"), more}
		}
		return []key.Binding{k.says(actGo, "resume"), k.says(actClose, "forget"), more}
	case r.section == agent.Snoozed:
		return []key.Binding{k.says(actGo, "go"), k.says(actWake, "wake"), k.says(actUndo, "undo"), more}
	case r.section == agent.Settled:
		return []key.Binding{k.says(actGo, "go"), k.says(actSettle, "restore"), k.says(actUndo, "undo"), more}
	}
	pin := "pin"
	if a.Thread.Pin != 0 {
		pin = "unpin"
	}
	review := k.says(actReview, "review")
	if m.opts.Picker {
		review = k.says(actReview, "diff")
		if m.previewDiff {
			review = k.says(actReview, "screen")
		}
	}
	switch agent.Status(a.Status) {
	case agent.Waiting:
		return []key.Binding{k.says(actApprove, "approve"), k.says(actDeny, "deny"), k.says(actReply, "reply"), k.says(actGo, "go"), k.says(actSnooze, "snooze"), more}
	case agent.Working:
		return []key.Binding{k.says(actGo, "go"), k.says(actDeny, "stop"), k.says(actReply, "reply"), k.says(actPin, pin), more}
	case agent.Error:
		// It stopped on a failure: tell it what to do, or see what it left.
		return []key.Binding{k.says(actGo, "go"), k.says(actReply, "reply"), review, k.says(actSettle, "settle"), more}
	}
	if agent.BadgeOf(a, m.now) == agent.BadgeWoke {
		return []key.Binding{k.says(actGo, "go"), k.says(actWake, "dismiss"), k.says(actSettle, "settle"), more}
	}
	if agent.Status(a.Status) == agent.Done {
		if hasPR {
			return []key.Binding{k.says(actGo, "go"), review, k.says(actPR, "pr"), k.says(actSettle, "settle"), more}
		}
		return []key.Binding{k.says(actGo, "go"), review, k.says(actSettle, "settle"), k.says(actPin, pin), more}
	}
	return []key.Binding{k.says(actGo, "go"), k.says(actSettle, "settle"), k.says(actPin, pin), k.says(actSnooze, "snooze"), review, more}
}

// key is a key as the footer and the help show it.
func (m Model) key(k string) string { return m.th.Key(k) }

// footer is the status bar under the list: a rule, then the keys for what is
// selected, the menu or prompt open, or what was just done.
func (m Model) footer() []string {
	line := func(s string) string { return ui.Paint(s, "", m.width) }
	lines := []string{m.th.Rule(m.width)}
	if in, ok := inputs[m.mode]; ok {
		return append(lines, line(" "+m.th.Accented(ui.Bold(cmp.Or(m.ask, in.label)))), line(" "+m.input.View()))
	}
	switch m.mode {
	case modeSnooze:
		lines = append(lines, line(" "+m.th.Accented(ui.Bold("Snooze until"))))
		for i, p := range agent.SnoozePresets(m.now) {
			lines = append(lines, ui.Spread(" "+m.key(strconv.Itoa(i+1))+" "+p.Label, m.th.Mute(when(p.Until, m.now)), m.width))
		}
		return append(lines, line(" "+m.key("c")+" "+m.th.Mute("other time")+m.th.Dim(" · ")+m.key("esc")+" "+m.th.Mute("cancel")))
	case modeMenu:
		return append(lines, m.th.Hints([]key.Binding{ui.Bind("↵", "pick", "enter"), ui.Bind("↑ ↓", "move", "up"), ui.Bind("esc", "close", "esc")}, m.width))
	case modeConfirmClose:
		ask := "Close the agent's window?"
		if a, ok := m.selectedAgent(); ok && a.Closed {
			ask = "Forget the closed thread?"
		}
		return append(lines, line(" "+ui.Fg(ansicolor.Red, ask)+" "+m.key("y")+m.th.Mute("/n")))
	case modeConfirmArchive:
		return append(lines, line(" "+ui.Fg(ansicolor.Red, "Remove its worktree, branch")), line(" "+ui.Fg(ansicolor.Red, "and window?")+" "+m.key("y")+m.th.Mute("/n")))
	}
	if m.flash != "" && m.opts.Now().Before(m.until) {
		if m.failed {
			// What went wrong is worth reading whole: on up to three lines.
			wrapped := strings.Split(ansi.Wrap(m.flash, max(m.width-4, 1), ""), "\n")
			for i, l := range wrapped[:min(len(wrapped), 3)] {
				mark := "  "
				if i == 0 {
					mark = ui.Fg(ansicolor.Red, "✗") + " "
				}
				lines = append(lines, line(" "+mark+l))
			}
			return lines
		}
		return append(lines, line(" "+ui.Fg(ansicolor.Green, "✓")+" "+m.flash))
	}
	if b, ok := m.hoveredButton(); ok {
		return append(lines, line(" "+m.th.Accented(b.icon)+"  "+b.label+"  "+m.key(m.right(b))))
	}
	if m.hoverNew && m.mode == modeNormal {
		return append(lines, line(" "+m.th.Accented(iconNew)+"  new thread  "+m.key(m.keys.label(actNew))))
	}
	if !m.focused {
		return append(lines, line(""))
	}
	hints := m.hints()
	if m.filter != "" {
		hints = append([]key.Binding{m.keys.says(actBack, "unfilter")}, hints...)
	}
	return append(lines, m.th.Hints(hints, m.width))
}

// helpLines is the help screen, shown in place of the list: one column in
// the sidebar, as many as fit in the picker.
func (m Model) helpLines() []string {
	var lines []string
	if m.opts.Picker {
		lines = m.th.HelpColumns(m.helpSections(), m.width)
	} else {
		lines = m.th.HelpList(m.helpSections())
	}
	return append(lines, "", "  "+m.th.Mute("↑ ↓ scroll, other keys: back"))
}

// scrollHelp moves the help screen by delta lines, as far as it goes.
func (m *Model) scrollHelp(delta int) {
	m.helpOffset = max(min(m.helpOffset+delta, len(m.helpLines())-m.listHeight()), 0)
}

// filterLabel is the filter as the sidebar shows it: after the / that
// started it, or as typed in the picker.
func (m Model) filterLabel() string {
	if m.opts.Picker {
		return m.filter
	}
	return "/" + m.filter
}
