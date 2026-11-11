package sidebar

import (
	"cmp"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/xheisenbugx/ksesh/kitty"
	"github.com/xheisenbugx/ksesh/model"
	"github.com/xheisenbugx/ksesh/remote"
)

// ksesh is the command line running this ksesh with args.
func (m Model) ksesh(args ...string) []string {
	exe := m.opts.Ksesh
	if len(exe) == 0 {
		exe = []string{"ksesh"}
	}
	return append(append([]string{}, exe...), args...)
}

// newThread is where a new thread starts, like T3 Code's "New thread" in a
// project: in the session and directory of the thread selected, or of the
// tab's.
type newThread struct {
	session, dir string
	host         string // the machine dir is on, when it isn't this one
	branch       string // the new worktree's, once asked
}

// startNewThread asks for the new thread's prompt, or with inWorktree first
// for the branch of the worktree it gets.
func (m Model) startNewThread(inWorktree bool) (tea.Model, tea.Cmd) {
	m.fresh = newThread{session: m.session}
	a, ok := m.selectedAgent()
	if !ok {
		// Nothing selected: the thread you're on, if the tab's window is one.
		for _, other := range m.agents {
			if other.WindowID == m.current {
				a, ok = other, true
			}
		}
	}
	if ok {
		m.fresh = newThread{session: a.Session, dir: a.Path, host: a.Host}
	}
	where := cmp.Or(m.fresh.session, "this tab")
	if !inWorktree {
		m, cmd := m.prompt(modeNewThread, "")
		m.ask = "New thread in " + where
		return m, cmd
	}
	if m.fresh.dir == "" || m.opts.Run == nil {
		m.say("select a thread in the repo to branch off")
		return m, nil
	}
	if m.fresh.host != "" {
		m.say("worktrees on " + m.fresh.host + " aren't supported yet")
		return m, nil
	}
	m, cmd := m.prompt(modeNewBranch, "")
	m.ask = "New worktree off " + filepath.Base(m.fresh.dir)
	return m, cmd
}

// launchThread starts the agent with a prompt in a new tab of the thread's
// session, as `ksesh tab connect` would, and goes to it.
func (m Model) launchThread(prompt string) tea.Cmd {
	at, agentCommand := m.fresh, m.opts.AgentCommand
	b := m.opts.Backend
	return func() tea.Msg {
		command := agentCommand
		if command == nil {
			command = func(string) string { return defaultAgent }
		}
		agentCmd := command(at.dir)
		line := agentCmd
		if prompt != "" {
			line += " " + shellQuote(prompt)
		}
		opts := model.KittyTabOpts{TargetSession: at.session, StartDir: at.dir, Background: true}
		if at.host != "" {
			// On another machine: a `kitten ssh` window there, the agent
			// typed in once its shell is up.
			opts.StartDir = ""
			opts.Command = remote.Join(remote.Window(at.host, at.dir)...)
			opts.Vars = map[string]string{kitty.HostVar: at.host, kitty.CwdVar: at.dir}
		}
		target, err := b.NewWindowInSession(opts)
		if err != nil {
			return doneMsg{err: err}
		}
		if at.host != "" {
			if err := b.WaitForPrompt(target, remoteShellWait); err != nil {
				return doneMsg{err: err}
			}
		}
		// Typed into the tab's shell, so the shell is still there when the
		// agent exits, as in a session ksesh wt new starts.
		if _, err := b.SendKeys(target, line); err != nil {
			return doneMsg{err: err}
		}
		flash := "started " + agentCmd
		if at.session != "" {
			flash += " in " + at.session
		}
		id, err := strconv.Atoi(strings.TrimPrefix(target, "window:"))
		if err != nil {
			return doneMsg{flash: flash}
		}
		return doneMsg{flash: flash, err: b.FocusWindow(id)}
	}
}

// remoteShellWait is how long a new window on another machine is given to
// show its shell's prompt.
const remoteShellWait = 20 * time.Second

// launchWorktree runs `ksesh wt new <branch> -p <prompt>` in the thread's
// directory: a worktree for the branch, a session in it and the agent.
func (m Model) launchWorktree(prompt string) tea.Cmd {
	at, run := m.fresh, m.opts.Run
	args := []string{"wt", "new", at.branch}
	if prompt != "" {
		args = append(args, "--prompt", prompt)
	} else {
		args = append(args, "--agent")
	}
	return func() tea.Msg {
		if err := run(at.dir, args...); err != nil {
			return doneMsg{err: err}
		}
		return doneMsg{flash: "started a thread on " + at.branch}
	}
}

// archivable is the linked worktree a thread works in, which archiving it
// removes: known once the sidebar asked git, and only with Run to run
// `ksesh wt rm`.
func (m Model) archivable(a *model.Agent) (string, bool) {
	info, ok := m.branchOf(a)
	// ksesh wt rm runs here, so only for a worktree on this machine.
	if !ok || info.status.Worktree == "" || m.opts.Run == nil || a.Host != "" {
		return "", false
	}
	return info.status.Worktree, true
}

func (m Model) keyConfirmArchive(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m.mode = modeNormal
	a, ok := m.selectedAgent()
	if msg.String() != "y" || !ok {
		return m, nil
	}
	wt, ok := m.archivable(a)
	if !ok {
		return m, nil
	}
	return m, m.archive(a, wt)
}

// archive is done with a thread for good: `ksesh wt rm` removes its
// worktree, branch and session, then its window closes. wt rm refuses a
// worktree with uncommitted changes, and the window then stays, with what
// wt rm said in the status bar.
func (m Model) archive(a *model.Agent, worktree string) tea.Cmd {
	run, id, dir, name := m.opts.Run, a.WindowID, a.Path, title(a)
	return m.do("archived "+name+", removed "+filepath.Base(worktree), nil, func(b Backend) error {
		if err := run(dir, "wt", "rm", worktree); err != nil {
			return err
		}
		// Closing its session may have closed it already.
		return b.CloseWindow(id)
	})
}

// gotoNth goes to the nth thread (from 1) of the sidebar's Order, as
// `ksesh sidebar goto` does, of the threads it shows.
func (m Model) gotoNth(n int) (tea.Model, tea.Cmd) {
	threads := Order(m.board(), m.state.Group)
	if n < 1 || n > len(threads) {
		m.say(fmt.Sprintf("no thread %d: %d in view", n, len(threads)))
		return m, nil
	}
	a := threads[n-1]
	m.cursor = "t" + strconv.Itoa(a.WindowID)
	m.keepCursor()
	id := a.WindowID
	return m, m.do("", nil, func(b Backend) error { return b.FocusWindow(id) })
}

// resizeStep is the columns < and > take from or give to the sidebar;
// minWidth the fewest it keeps.
const (
	resizeStep = 4
	minWidth   = 24
)

// resize makes the sidebar delta columns wider, and remembers the width
// for every sidebar opened after. kitty 0.50 can't resize a dock (they are
// outside the layout `kitten @ resize-window` works on), so the sidebar
// docks a new one at the new width, with the focus if it had it, and quits.
func (m Model) resize(delta int) (tea.Model, tea.Cmd) {
	if m.resizing || m.width == 0 {
		return m, nil
	}
	if len(m.opts.Dock.Command) == 0 {
		m.say("can't dock a sidebar from here")
		return m, nil
	}
	width := max(m.width+delta, minWidth)
	if width == m.width {
		m.say("as narrow as it goes")
		return m, nil
	}
	m.resizing = true
	dock := m.opts.Dock
	dock.Size = strconv.Itoa(width)
	path, self, focused, b := m.opts.StatePath, m.opts.Self, m.focused, m.opts.Backend
	return m, func() tea.Msg {
		if _, err := UpdateState(path, func(s *State) {
			s.Width = dock.Size
			s.Launch = kitty.SidebarLaunchArgs(dock)
		}); err != nil {
			return doneMsg{err: err}
		}
		work, err := b.LastWorkWindow(self)
		if err != nil {
			return doneMsg{err: err}
		}
		if work == 0 {
			return doneMsg{flash: dock.Size + " columns from the next sidebar on"}
		}
		id, err := b.OpenSidebar(work, dock)
		if err != nil {
			return doneMsg{err: err}
		}
		if focused {
			if err := b.FocusPlain(id); err != nil {
				return doneMsg{err: err}
			}
		}
		// Its window closes with it.
		return doneMsg{quit: true}
	}
}

// defaultAgent is what a new thread starts without AgentCommand.
const defaultAgent = "claude"

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

// review opens `ksesh agent diff` on the selected thread, paged, over the
// window beside the sidebar: the sidebar is too narrow for a diff.
func (m Model) review() (tea.Model, tea.Cmd) {
	a, ok := m.selectedAgent()
	if !ok {
		return m, nil
	}
	self, env := m.opts.Self, m.opts.Env
	command := m.ksesh("agent", "diff", strconv.Itoa(a.WindowID), "--pager")
	return m, m.do("", nil, func(b Backend) error {
		target, err := b.LastWorkWindow(self)
		if err != nil {
			return err
		}
		if target == 0 {
			return fmt.Errorf("no window beside the sidebar to open the review over")
		}
		return b.OverlayOn(target, env, command)
	})
}

// pullRequest is the pull request of the branch a thread is on, once the
// sidebar has found one with a URL.
func (m Model) pullRequest(a *model.Agent) (model.PullRequestState, bool) {
	info, ok := m.branchOf(a)
	if !ok {
		return model.PullRequestState{}, false
	}
	pr, ok := info.knownPR()
	if !ok || pr.URL == "" {
		return model.PullRequestState{}, false
	}
	return pr, true
}

// prLookupMsg is the answer to looking for a thread's pull request when
// asked to open it.
type prLookupMsg struct{ branchMsg }

// openPR opens the pull request of the selected thread's branch in the
// browser. When the sidebar doesn't know of one (it asks GitHub every few
// minutes, and not yet for a thread just opened), it asks git and GitHub
// there and then, and says why there is none.
func (m Model) openPR() (tea.Model, tea.Cmd) {
	a, ok := m.selectedAgent()
	if !ok {
		return m, nil
	}
	open := m.opts.OpenURL
	if open == nil {
		m.say("can't open a browser from here")
		return m, nil
	}
	if pr, ok := m.pullRequest(a); ok {
		return m, m.do("opened #"+strconv.Itoa(pr.Number), nil, func(Backend) error { return open(pr.URL) })
	}
	repos, host, dir, clock := m.opts.Repos, a.Host, a.Path, m.opts.Now
	if repos == nil || dir == "" {
		m.say("no pull request for its branch")
		return m, nil
	}
	m.say("looking for its pull request…")
	return m, func() tea.Msg {
		msg := branchMsg{host: host, dir: dir, checkedStatus: true, checkedPR: true}
		st, err := repos.BranchStatus(host, dir)
		msg.status, msg.ok = st, err == nil
		if msg.ownBranch() {
			msg.pr, msg.hasPR, msg.prErr = repos.PullRequest(host, dir, st.Branch, true)
		}
		msg.at = clock()
		return prLookupMsg{msg}
	}
}

// foundPR takes in the answer to openPR's lookup: opens the pull request
// found, or says why there is none.
func (m Model) foundPR(msg prLookupMsg) (tea.Model, tea.Cmd) {
	m.learn(msg.branchMsg)
	cmds := m.settleMerged()
	st, where := msg.status, filepath.Base(msg.dir)
	switch {
	case !msg.ok:
		m.say(where + " isn't in a git repo")
	case st.Branch == "":
		m.say(where + " has no branch checked out (detached HEAD)")
	case st.OnBase:
		m.say(where + " is on " + st.Branch + ", the base branch: no pull request")
	case msg.prErr != nil:
		m.say("couldn't ask GitHub: " + msg.prErr.Error())
		m.failed, m.until = true, m.opts.Now().Add(2*flashFor)
	case !msg.hasPR || msg.pr.URL == "":
		m.say("no pull request for " + st.Branch)
	default:
		open, pr := m.opts.OpenURL, msg.pr
		cmds = append(cmds, m.do("opened #"+strconv.Itoa(pr.Number), nil, func(Backend) error { return open(pr.URL) }))
	}
	return m, tea.Batch(cmds...)
}

// onPRLabel reports whether column x of a thread's second line is on its
// pull request (" #12" at the end of the line), which a click opens.
func (m Model) onPRLabel(r row, x int) bool {
	pr, ok := m.pullRequest(r.agent)
	if !ok {
		return false
	}
	line := m.renderThread(r, "")[1]
	line = strings.TrimRight(ansi.Strip(line), " ")
	icon, _ := prIcon(pr)
	text := icon + " #" + strconv.Itoa(pr.Number)
	if !strings.HasSuffix(line, text) {
		return false
	}
	end := ansi.StringWidth(line)
	return x >= end-ansi.StringWidth(text) && x < end
}
