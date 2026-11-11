package sidebar

import (
	"fmt"
	"image/color"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/xheisenbugx/ksesh/agent"
	"github.com/xheisenbugx/ksesh/kitty"
	"github.com/xheisenbugx/ksesh/model"
	"github.com/xheisenbugx/ksesh/ui"
)

// Backend is the part of kitty the sidebar drives. kitty.Kitty is one.
type Backend interface {
	ListAgents() ([]*model.Agent, error)
	SetThread(windowID int, t model.Thread) error
	SetTitle(windowID int, title string) error
	FocusWindow(windowID int) error
	FocusPlain(windowID int) error
	LastWorkWindow(windowID int) (int, error)
	WindowState(windowID int) (shown, focused bool, err error)
	GetCurrentSession() (string, error)
	SendKey(windowID int, keys ...string) error
	SendText(windowID int, text string, enter bool) error
	CloseWindow(windowID int) error
	CloseSidebars() error
	OverlayOn(windowID int, env []string, command []string) error
	NewWindowInSession(opts model.KittyTabOpts) (string, error)
	SendKeys(target string, command string) (string, error)
	OpenSidebar(windowID int, opts kitty.SidebarOpts) (int, error)
	Refresh()
	// ResumeThread opens a closed thread again, in a new window running its
	// agent on the same conversation.
	ResumeThread(conversation string) error
	// ForgetThread drops a closed thread from the thread history.
	ForgetThread(conversation string) error
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
	// Watch reports whether kitty's snapshot changed since it was last
	// asked, and whether it is live (kitty keeps it up). While it is, the
	// sidebar looks at kitty again as soon as it changes rather than on a
	// timer. Nil, or not live, refreshes on the timer.
	Watch func() (changed, live bool)
	// Ksesh runs this ksesh: the binary and the flags it was given (--config),
	// for what the sidebar opens in other windows, like a review.
	Ksesh []string
	// Env is NAME=VALUE pairs for those, such as the sidebar's PATH.
	Env []string
	// AgentCommand is the agent a new thread in dir starts: its repo's
	// agent_command, else claude. Nil always starts claude.
	AgentCommand func(dir string) string
	// Run runs ksesh with args in dir and returns what went wrong, for what
	// the sidebar leaves to ksesh's own commands (ksesh wt new, wt rm). Nil
	// can't.
	Run func(dir string, args ...string) error
	// Repos tells about the branch each thread is on. Nil shows none.
	Repos Repos
	// OpenURL opens a URL in the browser, such as a thread's pull request.
	// Nil can't.
	OpenURL func(url string) error
	// Dock is how the sidebar is docked, for < and > to dock it again at a
	// new width: kitty can't resize a dock. Without a Command it can't.
	Dock kitty.SidebarOpts

	// Picker runs the list as a full window picker over a window, rather
	// than docked: typing filters, the keys are ctrl and alt ones, the
	// selected thread's screen (or its diff) shows beside the list, and
	// going to a thread quits.
	Picker bool
	// OpenShelves are the shelves the picker opens with. Typing a filter
	// opens every shelf until the filter is cleared.
	OpenShelves []agent.Section
	// Preview draws a thread for the picker's preview pane, width columns
	// wide: its screen, or with diff, its review. Nil shows none.
	Preview func(a *model.Agent, diff bool, width int) (Preview, error)
}

// How often the sidebar asks kitty what changed: often while it is on
// screen, rarely from a tab in the background, which nobody is looking at.
const (
	refreshShown  = time.Second
	refreshHidden = 5 * time.Second
	flashFor      = 4 * time.Second
	maxUndo       = 20
	// How often a sidebar checks kitty's snapshot for a change: a stat, so
	// it can be often. In the background it still only refreshes every
	// refreshHidden, but notices at once when its tab comes to the front.
	watchEvery = 200 * time.Millisecond
)

type mode int

const (
	modeNormal mode = iota
	modeSnooze
	modeSnoozeCustom
	modeReply
	modeRename
	// modeNewThread asks for the prompt of a new thread; modeNewBranch for
	// the branch of a new worktree, then modeNewWorktree for its prompt.
	modeNewThread
	modeNewBranch
	modeNewWorktree
	// modeFilter filters the threads as you type.
	modeFilter
	modeConfirmClose
	// modeConfirmArchive asks before removing a thread's worktree.
	modeConfirmArchive
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
	// ask, when set, is the question the footer's input asks, in place of
	// its mode's.
	ask string
	// fresh is where a new thread is being started.
	fresh newThread
	// filter keeps only the threads whose title, session or branch has
	// every one of its words.
	filter string
	// branches is what is known of the branch checked out in each thread's
	// directory, by directory.
	branches map[string]*branchInfo
	// autoSettled is when the sidebar settled a thread for its merged pull
	// request, by window.
	autoSettled map[int]time.Time

	undo  []undoEntry
	flash string
	until time.Time // the flash's
	// failed marks the flash as what went wrong with an action, rather than
	// what it did.
	failed bool

	// stateSeq counts the sidebar's own changes to the shared state. A
	// refresh started before the latest one read the file before it was
	// written, so its copy of the state is stale.
	stateSeq int

	th      ui.Theme
	bg, fgc color.Color // the terminal's, once it says
	hover   string      // key of the row under the mouse
	// hoverButton is which of the hovered thread's buttons the mouse is on,
	// -1 for none.
	hoverButton int
	hoverNew    bool // the mouse is on the new thread button
	menu        popup
	frame       int // of the working spinner
	animating   bool
	restart     bool
	// resizing is set while the sidebar docks itself again at a new width.
	resizing bool
	// helpOffset is the first line of the help screen shown.
	helpOffset int

	keys keyMap
	// query is the picker's filter, typed into at any time.
	query textinput.Model
	// The picker's preview pane: whether it is on, showing the diff rather
	// than the screen, the thread it shows (by row key), and a count of the
	// fetches asked for, so a late one can be told apart.
	preview     viewport.Model
	previewOn   bool
	previewDiff bool
	previewKey  string
	previewSeq  int
	// previewFor is the thread whose preview the pane holds, previewHeader
	// the lines kept over its viewport.
	previewFor    string
	previewHeader []string
	// opened are the shelves open in the picker, which leaves the sidebars'
	// own alone; searched those open while it filters, every one to start
	// with.
	opened, searched map[agent.Section]bool
}

func New(opts Options) Model {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	in := textinput.New()
	in.Prompt = "› "
	m := Model{opts: opts, input: in, now: opts.Now(), shown: true, th: ui.ThemeFor(nil, nil), hoverButton: -1,
		branches: map[string]*branchInfo{}, autoSettled: map[int]time.Time{}, keys: sidebarKeys()}
	if opts.Picker {
		m.keys = pickerKeys()
		m.query = textinput.New()
		m.query.Prompt = "› "
		m.query.Placeholder = "filter by title, session or branch"
		m.query.Focus()
		m.preview = viewport.New()
		m.previewOn = opts.Preview != nil
		m.focused = true
		m.opened = map[agent.Section]bool{}
		for _, s := range opts.OpenShelves {
			m.opened[s] = true
		}
	}
	m.styleInputs()
	return m
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
	// watchMsg is a check of kitty's snapshot, changed if it changed since
	// the last load. due is when the timer would have refreshed: what a
	// sidebar in the background waits for, and one without a live snapshot.
	watchMsg struct {
		changed, live bool
		due           time.Time
	}
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
	cmds := []tea.Cmd{m.load(true), tea.RequestBackgroundColor, tea.RequestForegroundColor}
	if m.opts.Picker {
		cmds = append(cmds, m.query.Focus())
	}
	return tea.Batch(cmds...)
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
	next, cmd := m.update(msg)
	if m, ok := next.(Model); ok && m.opts.Picker {
		var preview tea.Cmd
		m, preview = m.syncPreview()
		return m, tea.Batch(cmd, preview)
	}
	return next, cmd
}

func (m Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.input.SetWidth(max(m.width-4, 1))
		m.query.SetWidth(max(m.width-4, 1))
		m.sizePreview()
		m.scrollToCursor()
		return m, nil
	case previewMsg:
		return m.showPreview(msg), nil
	case previewTickMsg:
		return m, m.fetchPreview(msg)
	case tea.FocusMsg:
		m.focused = true
		return m, m.load(false)
	case tea.BackgroundColorMsg:
		m.bg = msg.Color
		m.th = ui.ThemeFor(m.bg, m.fgc)
		m.styleInputs()
		return m, nil
	case tea.ForegroundColorMsg:
		m.fgc = msg.Color
		m.th = ui.ThemeFor(m.bg, m.fgc)
		m.styleInputs()
		return m, nil
	case tea.PasteMsg:
		if _, ok := inputs[m.mode]; ok {
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}
		if m.opts.Picker && m.mode == modeNormal {
			return m.typeQuery(msg)
		}
		return m, nil
	case tea.MouseMotionMsg:
		if m.mode == modeMenu {
			if i, ok := m.menu.itemAt(msg.X, msg.Y); ok {
				m.menu.idx = i
			}
			return m, nil
		}
		m.hover, m.hoverButton = "", -1
		m.hoverNew = m.onNewButton(msg.X, msg.Y)
		if r, nth, ok := m.lineAt(msg.Y); ok && r.selectable() && msg.X < m.cols() {
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
		m.hover, m.hoverButton, m.hoverNew = "", -1, false
		m.focused = m.opts.Picker
		if m.mode != modeNormal && m.mode != modeHelp {
			m.mode = modeNormal
		}
		return m, nil
	case tickMsg:
		return m, m.load(true)
	case watchMsg:
		due := !m.opts.Now().Before(msg.due)
		if (msg.changed && (m.shown || due)) || (!msg.live && due) {
			return m, m.load(true)
		}
		return m, m.watch(msg.due, msg.changed)
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
			// window, without the focus, gets no focus event to say so. A
			// picker always has it.
			m.focused = msg.focused || m.opts.Picker
			m.shown = m.shown || m.opts.Picker
			m.loaded = true
			m.keepCursor()
		}
		cmds := m.refreshBranches()
		if msg.err == nil && m.opts.Picker && !m.previewDiff {
			// The screen moved on: show it again.
			if cmd := m.refetchPreview(); cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		if msg.err == nil {
			cmds = append(cmds, m.settleMerged()...)
		}
		if m.spinning() && !m.animating {
			m.animating = true
			cmds = append(cmds, tea.Tick(spinEvery, func(time.Time) tea.Msg { return animMsg{} }))
		}
		if msg.tick {
			every := refreshShown
			if !msg.shown {
				every = refreshHidden
			}
			if m.opts.Watch != nil {
				cmds = append(cmds, m.watch(m.opts.Now().Add(every), false))
			} else {
				cmds = append(cmds, tea.Tick(every, func(time.Time) tea.Msg { return tickMsg{} }))
			}
		}
		return m, tea.Batch(cmds...)
	case branchMsg:
		m.learn(msg)
		if cmds := m.settleMerged(); len(cmds) > 0 {
			return m, tea.Batch(cmds...)
		}
		return m, nil
	case doneMsg:
		m.resizing = false
		if msg.err != nil {
			m.say(msg.err.Error())
			m.failed, m.until = true, m.opts.Now().Add(2*flashFor)
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
		if m.opts.Picker && m.mode != modeHelp && msg.X >= m.cols() {
			if msg.Button == tea.MouseWheelUp {
				m.preview.ScrollUp(3)
			} else if msg.Button == tea.MouseWheelDown {
				m.preview.ScrollDown(3)
			}
			return m, nil
		}
		if m.mode == modeHelp {
			if msg.Button == tea.MouseWheelUp {
				m.scrollHelp(-1)
			} else if msg.Button == tea.MouseWheelDown {
				m.scrollHelp(1)
			}
			return m, nil
		}
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
		case modeSnoozeCustom, modeReply, modeRename, modeNewThread, modeNewBranch, modeNewWorktree, modeFilter:
			return m.keyInput(msg)
		case modeConfirmClose:
			return m.keyConfirmClose(msg)
		case modeConfirmArchive:
			return m.keyConfirmArchive(msg)
		case modeHelp:
			switch msg.String() {
			case "j", "down", "ctrl+n", "ctrl+j":
				m.scrollHelp(1)
			case "k", "up", "ctrl+p", "ctrl+k":
				m.scrollHelp(-1)
			default:
				m.mode, m.helpOffset = modeNormal, 0
			}
			return m, nil
		case modeMenu:
			return m.keyMenu(msg)
		}
		return m.keyNormal(msg)
	}
	return m, nil
}

// watch checks kitty's snapshot for a change in a moment. pending carries a
// change seen while waiting for due.
func (m Model) watch(due time.Time, pending bool) tea.Cmd {
	watch := m.opts.Watch
	return tea.Tick(watchEvery, func(time.Time) tea.Msg {
		changed, live := watch()
		return watchMsg{changed: changed || pending, live: live, due: due}
	})
}

func (m *Model) say(text string) {
	m.flash, m.until, m.failed = text, m.opts.Now().Add(flashFor), false
}

// board is the threads in view, by section.
func (m Model) board() agent.Board {
	agents := m.agents
	if m.state.Session && m.session != "" {
		agents = slices.DeleteFunc(slices.Clone(agents), func(a *model.Agent) bool { return a.Session != m.session })
	}
	if m.filter != "" {
		agents = slices.DeleteFunc(slices.Clone(agents), func(a *model.Agent) bool { return !m.matches(a) })
	}
	return agent.Arrange(agents, m.now, m.opts.ThreadOpts)
}

// expanded is which shelves are open: the sidebars', or the picker's own.
func (m Model) expanded() map[agent.Section]bool {
	switch {
	case m.opts.Picker && m.filter != "":
		return m.searched
	case m.opts.Picker:
		return m.opened
	}
	return m.state.Expanded
}

func (m Model) rows() []row {
	rows := layout(m.board(), m.expanded(), m.allSettled, m.current)
	if m.state.Group {
		rows = groupInbox(rows)
	}
	return rows
}

// spinning reports whether a working spinner is on screen to animate.
func (m Model) spinning() bool {
	if !m.shown {
		return false
	}
	for _, a := range m.agents {
		if agent.Status(a.Status) == agent.Working && !a.Closed {
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
	i := y - m.top() + m.offset
	if y < m.top() || i < 0 || i >= len(lines) || y >= m.top()+m.listHeight() {
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
			return m.top() + i - m.offset
		}
	}
	return m.top() + m.listHeight() - 1
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
		return threadKey(r.agent)
	case rowHeader:
		return "h" + string(r.section)
	case rowMore:
		return "m"
	}
	return ""
}

// threadKey is a thread row's key: its window, or for a closed thread,
// which has none, its conversation.
func threadKey(a *model.Agent) string {
	if a.Closed {
		return "c" + a.Conversation
	}
	return "t" + strconv.Itoa(a.WindowID)
}

// keepCursor keeps the cursor on its row after a refresh. A thread that
// moved to another section is followed there; one that went away leaves the
// cursor where it was. With no cursor yet, it starts on the thread you're on.
func (m *Model) keepCursor() {
	rows := m.rows()
	if m.cursor == "" && m.current != 0 {
		m.cursor = "t" + strconv.Itoa(m.current)
	}
	// A closed thread resumed: follow it to its new window.
	if conv, ok := strings.CutPrefix(m.cursor, "c"); ok {
		for _, a := range m.agents {
			if !a.Closed && a.Conversation == conv {
				m.cursor = threadKey(a)
			}
		}
	}
	for i, r := range rows {
		if r.selectable() && rowKey(r) == m.cursor {
			m.cursorIdx = i
			m.scrollToCursor()
			return
		}
	}
	// A thread folded away in a closed shelf: stay on the shelf.
	if strings.HasPrefix(m.cursor, "t") || strings.HasPrefix(m.cursor, "c") {
		for _, a := range m.agents {
			if threadKey(a) != m.cursor {
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
	a := m.keys.match(msg)
	if a == actNth {
		return m.gotoNth(int(msg.String()[0] - '0'))
	}
	if a == actNone && m.opts.Picker {
		return m.typeQuery(msg)
	}
	return m.run(a)
}

// closedActs are what can't be done to a closed thread, which has no window:
// it can only be resumed or forgotten.
var closedActs = []act{actSeen, actPin, actSettle, actSnooze, actWake, actApprove, actDeny, actReply, actReview, actPager,
	actRename, actArchive, actPinUp, actPinDown}

// run does an act. Keys, the context menu and the buttons on a thread all
// run their actions through it, so each is done one way.
func (m Model) run(do act) (tea.Model, tea.Cmd) {
	if a, ok := m.selectedAgent(); ok && a.Closed && slices.Contains(closedActs, do) {
		m.say("it's closed: " + m.keys.label(actGo) + " resumes it")
		return m, nil
	}
	switch do {
	case actDown:
		m.move(1)
	case actUp:
		m.move(-1)
	case actTop:
		m.jump(false)
	case actBottom:
		m.jump(true)
	case actNextSection:
		m.nextSection(1)
	case actPrevSection:
		m.nextSection(-1)
	case actGo:
		return m.activate()
	case actOpen:
		if r, _, ok := m.selected(); ok && r.kind != rowThread {
			return m.activate()
		}
	case actBack:
		if m.filter != "" {
			m.filter = ""
			m.query.SetValue("")
			m.keepCursor()
			return m, nil
		}
		if m.opts.Picker {
			return m, tea.Quit
		}
		return m, m.backToWork()
	case actQuit:
		return m, tea.Quit
	case actFilter:
		return m.prompt(modeFilter, m.filter)
	case actNarrow:
		return m.resize(-resizeStep)
	case actWiden:
		return m.resize(resizeStep)
	case actGroup:
		m.stateSeq++
		st, err := UpdateState(m.opts.StatePath, func(s *State) { s.Group = !s.Group })
		if err != nil {
			m.say(err.Error())
		} else {
			m.state = st
			m.keepCursor()
		}
	case actPin:
		return m.pin()
	case actSettle:
		return m.settle()
	case actSnooze:
		if a, ok := m.selectedAgent(); ok {
			if agent.IsSnoozed(a.Thread, m.now) {
				m.say(m.keys.label(actWake) + " wakes it")
				return m, nil
			}
			m.mode = modeSnooze
		}
	case actWake:
		return m.wake()
	case actPinUp:
		return m.movePin(-1)
	case actPinDown:
		return m.movePin(1)
	case actUndo:
		return m.undoLast()
	case actApprove:
		if a, ok := m.selectedAgent(); ok {
			if agent.Status(a.Status) != agent.Waiting {
				m.say("it isn't waiting on you")
				return m, nil
			}
			return m, m.do(fmt.Sprintf("approved %s", title(a)), nil, func(b Backend) error { return b.SendKey(a.WindowID, "enter") })
		}
	case actDeny:
		if a, ok := m.selectedAgent(); ok {
			what := "interrupted"
			if agent.Status(a.Status) == agent.Waiting {
				what = "denied"
			}
			return m, m.do(what+" "+title(a), nil, func(b Backend) error { return b.SendKey(a.WindowID, "escape") })
		}
	case actReply:
		if _, ok := m.selectedAgent(); ok {
			return m.prompt(modeReply, "")
		}
	case actReview:
		if m.opts.Picker {
			return m.toggleDiff()
		}
		return m.review()
	case actPager:
		return m.pager()
	case actTogglePreview:
		return m.togglePreview()
	case actPreviewDown:
		m.preview.HalfPageDown()
	case actPreviewUp:
		m.preview.HalfPageUp()
	case actPR:
		return m.openPR()
	case actSeen:
		return m.markSeen()
	case actNew:
		return m.startNewThread(false)
	case actNewWorktree:
		return m.startNewThread(true)
	case actRename:
		if a, ok := m.selectedAgent(); ok {
			return m.prompt(modeRename, a.CustomTitle)
		}
	case actClose:
		if _, ok := m.selectedAgent(); ok {
			m.mode = modeConfirmClose
		}
	case actArchive:
		if a, ok := m.selectedAgent(); ok {
			if _, ok := m.archivable(a); !ok {
				m.say("not in a worktree of its own")
				return m, nil
			}
			m.mode = modeConfirmArchive
		}
	case actScope:
		m.stateSeq++
		st, err := UpdateState(m.opts.StatePath, func(s *State) { s.Session = !s.Session })
		if err != nil {
			m.say(err.Error())
		} else {
			m.state = st
			m.keepCursor()
		}
	case actHide:
		path := m.opts.StatePath
		return m, m.do("", nil, func(b Backend) error {
			if _, err := UpdateState(path, func(s *State) { s.Enabled = false }); err != nil {
				return err
			}
			return b.CloseSidebars()
		})
	case actHelp:
		m.mode = modeHelp
	case actMenu:
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
		if a := r.agent; a.Closed {
			conv := a.Conversation
			return m, m.doQuit("resuming "+title(a), func(b Backend) error { return b.ResumeThread(conv) })
		}
		id := r.agent.WindowID
		return m, m.doQuit("", func(b Backend) error { return b.FocusWindow(id) })
	case rowHeader:
		open := !m.expanded()[r.section]
		if m.opts.Picker {
			if m.filter != "" {
				m.searched = maps.Clone(m.searched)
				m.searched[r.section] = open
			} else {
				m.opened = maps.Clone(m.opened)
				m.opened[r.section] = open
			}
			if !open && r.section == agent.Settled {
				m.allSettled = false
			}
			m.keepCursor()
			return m, nil
		}
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

// doQuit is do for going somewhere: the picker quits once it went, the
// sidebar stays.
func (m Model) doQuit(flash string, action func(Backend) error) tea.Cmd {
	if !m.opts.Picker {
		return m.do(flash, nil, action)
	}
	b := m.opts.Backend
	return func() tea.Msg {
		err := action(b)
		return doneMsg{flash: flash, err: err, quit: err == nil}
	}
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
		return m.prompt(modeSnoozeCustom, "")
	}
	presets := agent.SnoozePresets(m.now)
	if n, err := strconv.Atoi(key); err == nil && n >= 1 && n <= len(presets) {
		return m.snooze(presets[n-1].Until)
	}
	return m, nil
}

func (m Model) keyConfirmClose(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m.mode = modeNormal
	a, ok := m.selectedAgent()
	if msg.String() != "y" || !ok {
		return m, nil
	}
	if a.Closed {
		return m, m.do("forgot "+title(a), nil, func(b Backend) error { return b.ForgetThread(a.Conversation) })
	}
	return m, m.do("closed "+title(a), nil, func(b Backend) error { return b.CloseWindow(a.WindowID) })
}

// click is a mouse click: on a row, it opens it; on one of a thread's
// buttons, it does what the button does; on its pull request, it opens that
// in the browser; a right click opens the thread's menu. With a menu open, a click picks from it, or closes it.
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
	if msg.Button == tea.MouseLeft && m.onNewButton(msg.X, msg.Y) {
		return m.run(actNew)
	}
	r, nth, ok := m.lineAt(msg.Y)
	if !ok || !r.selectable() || msg.X >= m.cols() {
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
	if r.kind == rowThread && nth == 1 && m.onPRLabel(r, msg.X) {
		return m.openPR()
	}
	return m.activate()
}
