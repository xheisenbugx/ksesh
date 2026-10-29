package kitty

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/xheisenbugx/ksesh/agent"
	"github.com/xheisenbugx/ksesh/model"
)

// The user variables an agent's window carries. See the agent package.
const (
	AgentVar    = "ksesh_agent"
	StatusVar   = "ksesh_status"
	StatusAtVar = "ksesh_status_at"
	MsgVar      = "ksesh_msg"
	PromptVar   = "ksesh_prompt"
	TurnAtVar   = "ksesh_turn_at"
)

// agentPrograms are the agents recognized by the program running in a window,
// so one started without ksesh's hooks still shows up in `ksesh agent list`.
var agentPrograms = []string{"claude", "codex", "gemini", "opencode", "aider", "amp", "pi", "cursor-agent", "copilot"}

// SetAgentStatus writes an agent's status onto a window. Without a window id
// it is the window ksesh runs in, which is where an agent's hooks run.
func (k *RealKitty) SetAgentStatus(windowID int, u agent.Update) error {
	if windowID == 0 {
		windowID = k.selfWindowID()
	}
	if windowID == 0 {
		return fmt.Errorf("not inside a kitty window")
	}
	var assignments []string
	set := func(name, value string) {
		if value == "" {
			assignments = append(assignments, name) // a bare name unsets it
		} else {
			assignments = append(assignments, name+"="+value)
		}
	}
	if u.Clear {
		for _, name := range []string{AgentVar, StatusVar, StatusAtVar, MsgVar, PromptVar, TurnAtVar} {
			set(name, "")
		}
	} else {
		if u.Agent != "" {
			set(AgentVar, u.Agent)
		}
		if u.SetMessage {
			set(MsgVar, u.Message)
		}
		if u.SetPrompt {
			set(PromptVar, u.Prompt)
		}
		now := strconv.FormatInt(time.Now().Unix(), 10)
		if u.StartTurn {
			set(TurnAtVar, now)
		}
		set(StatusAtVar, now)
		// The status goes last: kitty calls the watcher once per variable, and
		// contrib/ksesh_agents.py notifies on the status, reading the message
		// the same call has set by then.
		set(StatusVar, string(u.Status))
	}
	args := append([]string{"set-user-vars", "--match", "id:" + strconv.Itoa(windowID)}, assignments...)
	_, err := k.mutate(args...)
	return err
}

// ListAgents lists the agents in every kitty window: those reporting through
// their hooks, and, as idle, those only recognized by their program.
func (k *RealKitty) ListAgents() ([]*model.Agent, error) {
	st, err := k.load()
	if err != nil {
		return nil, err
	}
	sessionByNative := map[string]string{}
	for _, sess := range st.sessions {
		sessionByNative[sess.native] = sess.name
	}
	self := k.selfWindowID()
	var agents []*model.Agent
	for _, osw := range st.osWindows {
		for _, tab := range osw.Tabs {
			for _, w := range tab.Windows {
				if w.ID == self {
					continue
				}
				a, ok := agentOf(w)
				if !ok {
					continue
				}
				a.TabID, a.TabTitle = tab.ID, tab.Title
				a.Session = sessionByNative[w.SessionName]
				a.Focused = w.ID == st.currentID
				agents = append(agents, a)
			}
		}
	}
	return agents, nil
}

// agentOf reads the agent in a window, if there is one.
func agentOf(w lsWindow) (*model.Agent, bool) {
	a := &model.Agent{
		WindowID:    w.ID,
		Path:        w.dir(),
		LastFocused: w.LastFocusedAt,
	}
	status := w.UserVars[StatusVar]
	if status == "" {
		// No hooks: fall back to recognizing the program, which can't tell
		// what it is doing.
		cmd := w.command()
		if !slices.Contains(agentPrograms, cmd) {
			return nil, false
		}
		a.Name, a.Status = cmd, string(agent.Idle)
		return a, true
	}
	a.Name = w.UserVars[AgentVar]
	a.Status = status
	a.Message = w.UserVars[MsgVar]
	a.Prompt = w.UserVars[PromptVar]
	if at, err := strconv.ParseInt(w.UserVars[StatusAtVar], 10, 64); err == nil {
		since := time.Unix(at, 0)
		a.Since = &since
	}
	return a, true
}

// windowStatus is the agent status a window reports through its hooks.
func windowStatus(w lsWindow) agent.Status {
	status, _ := agent.Parse(w.UserVars[StatusVar])
	return status
}

// FocusWindow switches to a window, going through its session first so the
// switch lands in kitty's session history like any other.
func (k *RealKitty) FocusWindow(windowID int) error {
	st, err := k.load()
	if err != nil {
		return err
	}
	for _, sess := range st.sessions {
		if slices.Contains(sess.windowIDs(), windowID) && sess != st.current {
			if err := k.focusSession(sess); err != nil {
				return err
			}
			break
		}
	}
	_, err = k.mutate("focus-window", "--match", "id:"+strconv.Itoa(windowID))
	return err
}

// CurrentWindowID is the window the user is in: the one ksesh runs in, or the
// focused one outside kitty.
func (k *RealKitty) CurrentWindowID() (int, error) {
	st, err := k.load()
	if err != nil {
		return 0, err
	}
	return st.currentID, nil
}

// SendText types text into a window, and presses enter after it when enter is
// set: how a reply reaches an agent without switching to it.
func (k *RealKitty) SendText(windowID int, text string, enter bool) error {
	// send-text decodes backslash escapes, so literal backslashes are doubled
	// and \r is the enter key.
	escaped := strings.ReplaceAll(text, `\`, `\\`)
	if enter {
		escaped += `\r`
	}
	_, err := k.rc("send-text", "--match", "id:"+strconv.Itoa(windowID), "--", escaped)
	return err
}

// SendKey presses keys (kitty key names: enter, escape, ctrl+c, 1, …) in a
// window, one after the other.
func (k *RealKitty) SendKey(windowID int, keys ...string) error {
	args := append([]string{"send-key", "--match", "id:" + strconv.Itoa(windowID)}, keys...)
	_, err := k.rc(args...)
	return err
}

// CaptureWindow returns a window's screen, with its colors.
func (k *RealKitty) CaptureWindow(windowID int) (string, error) {
	return k.rc("get-text", "--ansi", "--match", "id:"+strconv.Itoa(windowID))
}
