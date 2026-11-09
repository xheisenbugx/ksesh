package sidebar

import (
	"fmt"
	"image/color"
	"slices"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/xheisenbugx/ksesh/agent"
	"github.com/xheisenbugx/ksesh/model"
)

// Backend is the part of kitty the sidebar drives. kitty.Kitty is one.
type Backend interface {
	ListAgents() ([]*model.Agent, error)
	SetThread(windowID int, t model.Thread) error
	FocusWindow(windowID int) error
	FocusPlain(windowID int) error
	LastWorkWindow(windowID int) (int, error)
	WindowState(windowID int) (shown, focused bool, err error)
	GetCurrentSession() (string, error)
	SendKey(windowID int, keys ...string) error
	SendText(windowID int, text string, enter bool) error
	CloseWindow(windowID int) error
	CloseSidebars() error
	Refresh()
}

type Options struct {
	Backend Backend
	// Self is the sidebar's own kitty window.
	Self       int
	ThreadOpts agent.ThreadOpts
	StatePath  string
	// Now is the clock, for tests.
	Now func() time.Time
	// Changed reports whether ksesh was rebuilt since the sidebar started,
	// so it can restart into the new one. Nil never restarts.
	Changed func() bool
}

// How often the sidebar asks kitty what changed: often while it is on
// screen, rarely from a tab in the background, which nobody is looking at.
const (
	refreshShown  = time.Second
	refreshHidden = 5 * time.Second
	flashFor      = 4 * time.Second
	maxUndo       = 20
)

type mode int

const (
	modeNormal mode = iota
	modeSnooze
	modeSnoozeCustom
	modeReply
	modeConfirmClose
	modeHelp
	// modeMenu is a popup open over the list: a thread's context menu, or
	// the snooze times.
	modeMenu
)

type undoEntry struct {
	windowID int
	thread   model.Thread
	what     string
}

type Model struct {
	opts Options

	agents  []*model.Agent
	current int    // the thread you're on: the last window you were in, in this tab
	session string // this tab's session
	shown   bool
	focused bool
	state   State
	loaded  bool
	err     error
	now     time.Time

	allSettled bool
	cursor     string // key of the selected row
	cursorIdx  int    // where it was, for when its row goes away
	offset     int    // first line shown
	width      int
	height     int

	mode  mode
	input textinput.Model
	undo  []undoEntry
	flash string
	until time.Time // the flash's

	// stateSeq counts the sidebar's own changes to the shared state. A
	// refresh started before the latest one read the file before it was
	// written, so its copy of the state is stale.
	stateSeq int

	th      theme
	bg, fgc color.Color // the terminal's, once it says
	hover   string      // key of the row under the mouse
	// hoverButton is which of the hovered thread's buttons the mouse is on,
	// -1 for none.
	hoverButton int
	menu        popup
	frame       int // of the working spinner
	animating   bool
	restart     bool
}

func New(opts Options) Model {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	in := textinput.New()
	in.Prompt = "› "
	return Model{opts: opts, input: in, now: opts.Now(), shown: true, th: themeFor(nil, nil), hoverButton: -1}
}

// Restart reports whether the sidebar quit to restart into a new ksesh.
func (m Model) Restart() bool {
	return m.restart
}

type (
	loadedMsg struct {
		agents   []*model.Agent
		current  int
		session  string
		shown    bool
		focused  bool
		state    State
		stateSeq int
		err      error
		tick     bool
		restart  bool
	}
	tickMsg struct{}
	animMsg struct{}
	// RefreshMsg asks the sidebar to look at kitty again now: sent when its
	// tab comes to the front, so it doesn't show what it saw while hidden.
	RefreshMsg struct{}
	doneMsg    struct {
		flash string
		err   error
		undo  *undoEntry
		// quit closes the sidebar once done.
		quit bool
	}
)

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.load(true), tea.RequestBackgroundColor, tea.RequestForegroundColor)
}

// load asks kitty for every agent and where the sidebar is. Only the load a
// tick started schedules the next tick, so actions don't multiply them.
func (m Model) load(tick bool) tea.Cmd {
	b, self, path, changed, seq := m.opts.Backend, m.opts.Self, m.opts.StatePath, m.opts.Changed, m.stateSeq
	return func() tea.Msg {
		if tick && changed != nil && changed() {
			return loadedMsg{restart: true}
		}
		b.Refresh()
		agents, err := b.ListAgents()
		if err != nil {
			return loadedMsg{err: err, tick: tick, shown: true}
		}
		msg := loadedMsg{agents: agents, tick: tick, state: LoadState(path), stateSeq: seq}
		msg.current, _ = b.LastWorkWindow(self)
		msg.shown, msg.focused, _ = b.WindowState(self)
		msg.session, _ = b.GetCurrentSession()
		return msg
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.input.SetWidth(max(m.width-4, 1))
		m.scrollToCursor()
		return m, nil
	case tea.FocusMsg:
		m.focused = true
		return m, m.load(false)
	case tea.BackgroundColorMsg:
		m.bg = msg.Color
		m.th = themeFor(m.bg, m.fgc)
		return m, nil
	case tea.ForegroundColorMsg:
		m.fgc = msg.Color
		m.th = themeFor(m.bg, m.fgc)
		return m, nil
	case tea.MouseMotionMsg:
		if m.mode == modeMenu {
			if i, ok := m.menu.itemAt(msg.X, msg.Y); ok {
				m.menu.idx = i
			}
			return m, nil
		}
		m.hover, m.hoverButton = "", -1
		if r, nth, ok := m.lineAt(msg.Y); ok && r.selectable() {
			m.hover = rowKey(r)
			if nth == 0 {
				m.hoverButton = m.buttonAt(r, msg.X)
			}
		}
		return m, nil
	case animMsg:
		m.frame++
		if !m.spinning() {
			m.animating = false
			return m, nil
		}
		return m, tea.Tick(spinEvery, func(time.Time) tea.Msg { return animMsg{} })
	case tea.BlurMsg:
		m.hover, m.hoverButton = "", -1
		m.focused = false
		if m.mode != modeNormal && m.mode != modeHelp {
			m.mode = modeNormal
		}
		return m, nil
	case tickMsg:
		return m, m.load(true)
	case RefreshMsg:
		return m, m.load(false)
	case loadedMsg:
		if msg.restart {
			m.restart = true
			return m, tea.Quit
		}
		m.now = m.opts.Now()
		m.err = msg.err
		if msg.err == nil {
			m.agents, m.current, m.session, m.shown = msg.agents, msg.current, msg.session, msg.shown
			if msg.stateSeq == m.stateSeq {
				m.state = msg.state
			}
			// kitty says who has the focus too: a sidebar opened beside your
			// window, without the focus, gets no focus event to say so.
			m.focused = msg.focused
			m.loaded = true
			m.keepCursor()
		}
		var cmds []tea.Cmd
		if m.spinning() && !m.animating {
			m.animating = true
			cmds = append(cmds, tea.Tick(spinEvery, func(time.Time) tea.Msg { return animMsg{} }))
		}
		if msg.tick {
			every := refreshShown
			if !msg.shown {
				every = refreshHidden
			}
			cmds = append(cmds, tea.Tick(every, func(time.Time) tea.Msg { return tickMsg{} }))
		}
		return m, tea.Batch(cmds...)
	case doneMsg:
		if msg.err != nil {
			m.say(msg.err.Error())
		} else if msg.flash != "" {
			m.say(msg.flash)
		}
		if msg.undo != nil && msg.err == nil {
			m.undo = append(m.undo, *msg.undo)
			if len(m.undo) > maxUndo {
				m.undo = m.undo[1:]
			}
		}
		if msg.quit {
			return m, tea.Quit
		}
		return m, m.load(false)
	case tea.MouseClickMsg:
		return m.click(msg)
	case tea.MouseWheelMsg:
		if msg.Button == tea.MouseWheelUp {
			m.move(-1)
		} else if msg.Button == tea.MouseWheelDown {
			m.move(1)
		}
		return m, nil
	case tea.KeyPressMsg:
		switch m.mode {
		case modeSnooze:
			return m.keySnooze(msg)
		case modeSnoozeCustom, modeReply:
			return m.keyInput(msg)
		case modeConfirmClose:
			return m.keyConfirmClose(msg)
		case modeHelp:
			m.mode = modeNormal
			return m, nil
		case modeMenu:
			return m.keyMenu(msg)
		}
		return m.keyNormal(msg)
	}
	return m, nil
}

func (m *Model) say(text string) {
	m.flash, m.until = text, m.opts.Now().Add(flashFor)
}

// board is the threads in view, by section.
func (m Model) board() agent.Board {
	agents := m.agents
	if m.state.Session && m.session != "" {
		agents = slices.DeleteFunc(slices.Clone(agents), func(a *model.Agent) bool { return a.Session != m.session })
	}
	return agent.Arrange(agents, m.now, m.opts.ThreadOpts)
}

func (m Model) rows() []row {
	return layout(m.board(), m.state.Expanded, m.allSettled, m.current)
}

// spinning reports whether a working spinner is on screen to animate.
func (m Model) spinning() bool {
	if !m.shown {
		return false
	}
	for _, a := range m.agents {
		if agent.Status(a.Status) == agent.Working {
			return true
		}
	}
	return false
}

// rowAt is the row drawn on line y of the screen.
func (m Model) rowAt(y int) (row, bool) {
	r, _, ok := m.lineAt(y)
	return r, ok
}

// lineAt is the row drawn on line y of the screen, and which of its lines y
// is.
func (m Model) lineAt(y int) (row, int, bool) {
	lines := m.lineRows()
	i := y - headerLines + m.offset
	if y < headerLines || i < 0 || i >= len(lines) || y >= headerLines+m.listHeight() {
		return row{}, 0, false
	}
	nth := 0
	for j := i - 1; j >= 0 && lines[j] == lines[i]; j-- {
		nth++
	}
	return m.rows()[lines[i]], nth, true
}

// rowY is the screen line a row starts on, or the list's last if it is out
// of view.
func (m Model) rowY(key string) int {
	rows := m.rows()
	for i, ri := range m.lineRows() {
		if rowKey(rows[ri]) == key && i >= m.offset && i < m.offset+m.listHeight() {
			return headerLines + i - m.offset
		}
	}
	return headerLines + m.listHeight() - 1
}

func (m Model) selected() (row, int, bool) {
	rows := m.rows()
	for i, r := range rows {
		if r.selectable() && rowKey(r) == m.cursor {
			return r, i, true
		}
	}
	return row{}, -1, false
}

func (m Model) selectedAgent() (*model.Agent, bool) {
	r, _, ok := m.selected()
	if !ok || r.kind != rowThread {
		return nil, false
	}
	return r.agent, true
}

func rowKey(r row) string {
	switch r.kind {
	case rowThread:
		return "t" + strconv.Itoa(r.agent.WindowID)
	case rowHeader:
		return "h" + string(r.section)
	case rowMore:
		return "m"
	}
	return ""
}

// keepCursor keeps the cursor on its row after a refresh. A thread that
// moved to another section is followed there; one that went away leaves the
// cursor where it was. With no cursor yet, it starts on the thread you're on.
func (m *Model) keepCursor() {
	rows := m.rows()
	if m.cursor == "" && m.current != 0 {
		m.cursor = "t" + strconv.Itoa(m.current)
	}
	for i, r := range rows {
		if r.selectable() && rowKey(r) == m.cursor {
			m.cursorIdx = i
			m.scrollToCursor()
			return
		}
	}
	// A thread folded away in a closed shelf: stay on the shelf.
	if id, ok := strings.CutPrefix(m.cursor, "t"); ok {
		for _, a := range m.agents {
			if strconv.Itoa(a.WindowID) != id {
				continue
			}
			header := "h" + string(agent.SectionOf(a, m.now, m.opts.ThreadOpts))
			for i, r := range rows {
				if r.selectable() && rowKey(r) == header {
					m.cursor, m.cursorIdx = header, i
					m.scrollToCursor()
					return
				}
			}
		}
	}
	m.cursorIdx = min(m.cursorIdx, len(rows)-1)
	m.cursor = ""
	for d := 0; d < len(rows); d++ {
		for _, i := range []int{m.cursorIdx + d, m.cursorIdx - d} {
			if i >= 0 && i < len(rows) && rows[i].selectable() {
				m.cursor, m.cursorIdx = rowKey(rows[i]), i
				m.scrollToCursor()
				return
			}
		}
	}
}

func (m *Model) move(delta int) {
	rows := m.rows()
	_, i, ok := m.selected()
	if !ok {
		i = m.cursorIdx - delta
	}
	for j := i + delta; j >= 0 && j < len(rows); j += delta {
		if rows[j].selectable() {
			m.cursor, m.cursorIdx = rowKey(rows[j]), j
			break
		}
	}
	m.scrollToCursor()
}

func (m *Model) jump(last bool) {
	rows := m.rows()
	order := func(yield func(int) bool) {
		for i := range rows {
			j := i
			if last {
				j = len(rows) - 1 - i
			}
			if !yield(j) {
				return
			}
		}
	}
	for i := range order {
		if rows[i].selectable() {
			m.cursor, m.cursorIdx = rowKey(rows[i]), i
			break
		}
	}
	m.scrollToCursor()
}

// nextSection moves to the first row of the next section with a header.
func (m *Model) nextSection(delta int) {
	rows := m.rows()
	_, i, _ := m.selected()
	for j := i + delta; j >= 0 && j < len(rows); j += delta {
		if rows[j].kind == rowHeader && rows[j].selectable() {
			m.cursor, m.cursorIdx = rowKey(rows[j]), j
			break
		}
	}
	m.scrollToCursor()
}

func (m Model) keyNormal(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	return m.run(msg.String())
}

// run does what a key does. The context menu and the buttons on a thread
// run their actions through it too, so each is done one way.
func (m Model) run(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "j", "down", "ctrl+n":
		m.move(1)
	case "k", "up", "ctrl+p":
		m.move(-1)
	case "g", "home":
		m.jump(false)
	case "G", "end":
		m.jump(true)
	case "tab":
		m.nextSection(1)
	case "shift+tab":
		m.nextSection(-1)
	case "enter", "l", "right":
		return m.activate()
	case "space":
		if r, _, ok := m.selected(); ok && r.kind != rowThread {
			return m.activate()
		}
	case "esc", "h", "left":
		return m, m.backToWork()
	case "p":
		return m.pin()
	case "s":
		return m.settle()
	case "z":
		if a, ok := m.selectedAgent(); ok {
			if agent.IsSnoozed(a.Thread, m.now) {
				m.say("w wakes it")
				return m, nil
			}
			m.mode = modeSnooze
		}
	case "w":
		return m.wake()
	case "K", "shift+up":
		return m.movePin(-1)
	case "J", "shift+down":
		return m.movePin(1)
	case "u", "ctrl+z":
		return m.undoLast()
	case "a", "y":
		if a, ok := m.selectedAgent(); ok {
			if agent.Status(a.Status) != agent.Waiting {
				m.say("it isn't waiting on you")
				return m, nil
			}
			return m, m.do(fmt.Sprintf("approved %s", title(a)), nil, func(b Backend) error { return b.SendKey(a.WindowID, "enter") })
		}
	case "x":
		if a, ok := m.selectedAgent(); ok {
			what := "interrupted"
			if agent.Status(a.Status) == agent.Waiting {
				what = "denied"
			}
			return m, m.do(what+" "+title(a), nil, func(b Backend) error { return b.SendKey(a.WindowID, "escape") })
		}
	case "r":
		if _, ok := m.selectedAgent(); ok {
			m.mode = modeReply
			m.input.Placeholder = "reply, enter sends"
			m.input.SetValue("")
			return m, m.input.Focus()
		}
	case "X":
		if _, ok := m.selectedAgent(); ok {
			m.mode = modeConfirmClose
		}
	case "f":
		m.stateSeq++
		st, err := UpdateState(m.opts.StatePath, func(s *State) { s.Session = !s.Session })
		if err != nil {
			m.say(err.Error())
		} else {
			m.state = st
			m.keepCursor()
		}
	case "q":
		path := m.opts.StatePath
		return m, m.do("", nil, func(b Backend) error {
			if _, err := UpdateState(path, func(s *State) { s.Enabled = false }); err != nil {
				return err
			}
			return b.CloseSidebars()
		})
	case "?":
		m.mode = modeHelp
	case "m", "menu":
		if r, _, ok := m.selected(); ok && r.kind == rowThread {
			return m.openThreadMenu(r.agent, 4, m.rowY(rowKey(r))), nil
		}
	}
	return m, nil
}

// activate is enter: go to a thread, open or close a shelf, show the rest
// of the settled threads.
func (m Model) activate() (tea.Model, tea.Cmd) {
	r, _, ok := m.selected()
	if !ok {
		return m, nil
	}
	switch r.kind {
	case rowThread:
		id := r.agent.WindowID
		return m, m.do("", nil, func(b Backend) error { return b.FocusWindow(id) })
	case rowHeader:
		open := !m.state.Expanded[r.section]
		m.stateSeq++
		st, err := UpdateState(m.opts.StatePath, func(s *State) {
			if s.Expanded == nil {
				s.Expanded = map[agent.Section]bool{}
			}
			s.Expanded[r.section] = open
		})
		if err != nil {
			m.say(err.Error())
			return m, nil
		}
		m.state = st
		if !open && r.section == agent.Settled {
			m.allSettled = false
		}
	case rowMore:
		m.allSettled = true
	}
	m.keepCursor()
	return m, nil
}

// backToWork leaves the sidebar for the window you were in before.
func (m Model) backToWork() tea.Cmd {
	self := m.opts.Self
	return m.do("", nil, func(b Backend) error {
		id, err := b.LastWorkWindow(self)
		if err != nil || id == 0 {
			return err
		}
		return b.FocusPlain(id)
	})
}

// do runs an action against kitty off the UI loop, then refreshes.
func (m Model) do(flash string, undo *undoEntry, action func(Backend) error) tea.Cmd {
	b := m.opts.Backend
	return func() tea.Msg {
		return doneMsg{flash: flash, err: action(b), undo: undo}
	}
}

// setThread writes a thread's new state, showing it at once rather than at
// the next refresh, and remembers the old one for undo.
func (m *Model) setThread(a *model.Agent, t model.Thread, what string) tea.Cmd {
	undo := &undoEntry{windowID: a.WindowID, thread: a.Thread, what: what}
	a.Thread = t
	m.keepCursor()
	id := a.WindowID
	return m.do(what+" "+title(a), undo, func(b Backend) error { return b.SetThread(id, t) })
}

func (m Model) pin() (tea.Model, tea.Cmd) {
	a, ok := m.selectedAgent()
	if !ok {
		return m, nil
	}
	what := "pinned"
	if a.Thread.Pin != 0 {
		what = "unpinned"
	}
	t := agent.TogglePin(a, m.now, m.opts.ThreadOpts, agent.NextPin(m.agents, m.now))
	return m, m.setThread(a, t, what)
}

func (m Model) settle() (tea.Model, tea.Cmd) {
	a, ok := m.selectedAgent()
	if !ok {
		return m, nil
	}
	_, wasSettled := agent.SettledSince(a, m.now, m.opts.ThreadOpts)
	t, err := agent.ToggleSettle(a, m.now, m.opts.ThreadOpts)
	if err != nil {
		m.say(err.Error())
		return m, nil
	}
	what := "settled"
	if wasSettled {
		what = "un-settled"
	}
	return m, m.setThread(a, t, what)
}

func (m Model) wake() (tea.Model, tea.Cmd) {
	a, ok := m.selectedAgent()
	if !ok || (!agent.IsSnoozed(a.Thread, m.now) && !agent.HasWoken(a.Thread, m.now)) {
		return m, nil
	}
	what := "woke"
	if !agent.IsSnoozed(a.Thread, m.now) {
		what = "dismissed"
	}
	return m, m.setThread(a, agent.Wake(a), what)
}

func (m Model) snooze(until time.Time) (tea.Model, tea.Cmd) {
	m.mode = modeNormal
	a, ok := m.selectedAgent()
	if !ok {
		return m, nil
	}
	t, err := agent.Snooze(a, m.now, until)
	if err != nil {
		m.say(err.Error())
		return m, nil
	}
	return m, m.setThread(a, t, "snoozed")
}

// movePin swaps a pinned thread with the one above or below it.
func (m Model) movePin(delta int) (tea.Model, tea.Cmd) {
	a, ok := m.selectedAgent()
	if !ok || a.Thread.Pin == 0 {
		return m, nil
	}
	pinned := m.board()[agent.Pinned]
	i := slices.Index(pinned, a)
	j := i + delta
	if i < 0 || j < 0 || j >= len(pinned) {
		return m, nil
	}
	other := pinned[j]
	ta, to := a.Thread, other.Thread
	ta.Pin, to.Pin = to.Pin, ta.Pin
	a.Thread, other.Thread = ta, to
	m.keepCursor()
	return m, m.do("", nil, func(b Backend) error {
		if err := b.SetThread(a.WindowID, ta); err != nil {
			return err
		}
		return b.SetThread(other.WindowID, to)
	})
}

func (m Model) undoLast() (tea.Model, tea.Cmd) {
	if len(m.undo) == 0 {
		m.say("nothing to undo")
		return m, nil
	}
	last := m.undo[len(m.undo)-1]
	m.undo = m.undo[:len(m.undo)-1]
	for _, a := range m.agents {
		if a.WindowID == last.windowID {
			a.Thread = last.thread
		}
	}
	m.keepCursor()
	return m, m.do("undid "+last.what, nil, func(b Backend) error { return b.SetThread(last.windowID, last.thread) })
}

func (m Model) keySnooze(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc", "q", "z":
		m.mode = modeNormal
		return m, nil
	case "c":
		m.mode = modeSnoozeCustom
		m.input.Placeholder = "90m, 2d, 15:04, 2026-12-01 09:00"
		m.input.SetValue("")
		return m, m.input.Focus()
	}
	presets := agent.SnoozePresets(m.now)
	if n, err := strconv.Atoi(key); err == nil && n >= 1 && n <= len(presets) {
		return m.snooze(presets[n-1].Until)
	}
	return m, nil
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
		if m.mode == modeSnoozeCustom {
			until, err := agent.ParseSnooze(text, m.now)
			if err != nil {
				m.mode = modeNormal
				m.say(err.Error())
				return m, nil
			}
			return m.snooze(until)
		}
		m.mode = modeNormal
		a, ok := m.selectedAgent()
		if !ok || text == "" {
			return m, nil
		}
		return m, m.do("sent to "+title(a), nil, func(b Backend) error { return b.SendText(a.WindowID, text, true) })
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m Model) keyConfirmClose(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m.mode = modeNormal
	a, ok := m.selectedAgent()
	if msg.String() != "y" || !ok {
		return m, nil
	}
	return m, m.do("closed "+title(a), nil, func(b Backend) error { return b.CloseWindow(a.WindowID) })
}

// click is a mouse click: on a row, it opens it; on one of a thread's
// buttons, it does what the button does; a right click opens the thread's
// menu. With a menu open, a click picks from it, or closes it.
func (m Model) click(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	if m.mode == modeMenu {
		if i, ok := m.menu.itemAt(msg.X, msg.Y); ok {
			if msg.Button == tea.MouseLeft {
				return m.pick(i)
			}
			return m, nil
		}
		m.mode, m.menu = modeNormal, popup{}
		if msg.Button != tea.MouseRight {
			return m, nil
		}
	}
	if m.mode != modeNormal || (msg.Button != tea.MouseLeft && msg.Button != tea.MouseRight) {
		return m, nil
	}
	r, nth, ok := m.lineAt(msg.Y)
	if !ok || !r.selectable() {
		return m, nil
	}
	m.cursor = rowKey(r)
	m.keepCursor()
	if msg.Button == tea.MouseRight {
		if r.kind != rowThread {
			return m, nil
		}
		return m.openThreadMenu(r.agent, msg.X, msg.Y), nil
	}
	if r.kind == rowThread && nth == 0 {
		if i := m.buttonAt(r, msg.X); i >= 0 {
			return m.buttons(r)[i].run(m)
		}
	}
	return m.activate()
}
