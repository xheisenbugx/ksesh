package sidebar

import (
	"cmp"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/xheisenbugx/ksesh/model"
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
		m.fresh = newThread{session: a.Session, dir: a.Path}
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
		target, err := b.NewWindowInSession(model.KittyTabOpts{TargetSession: at.session, StartDir: at.dir, Background: true})
		if err != nil {
			return doneMsg{err: err}
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
