package kitty

import (
	"cmp"
	"encoding/json"
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
	// ConvVar is the agent's conversation id (Claude Code's session_id),
	// which ties the window to its record in the thread history.
	ConvVar = "ksesh_conv"
	// TitleVar is a title you gave the thread, shown instead of the window's.
	TitleVar = "ksesh_title"
	// SeenAtVar is when you last focused the window, set by kitty's watcher
	// (contrib/ksesh_agents.py): what tells a failure you saw from one you
	// didn't.
	SeenAtVar = "ksesh_seen_at"

	// Thread triage (see agent/thread.go). Times are unix seconds.
	PinVar         = "ksesh_pin"
	SnoozeUntilVar = "ksesh_snooze_until"
	SnoozedAtVar   = "ksesh_snoozed_at"
	SettledAtVar   = "ksesh_settled_at"
	UnsettledVar   = "ksesh_unsettled"
	WokeVar        = "ksesh_woke"
)

var threadVars = []string{PinVar, SnoozeUntilVar, SnoozedAtVar, SettledAtVar, UnsettledVar, WokeVar}

// threadAssignments are the set-user-vars arguments that write a thread's
// state; a bare name unsets the variable.
func threadAssignments(t model.Thread) []string {
	unix := func(at time.Time) string {
		if at.IsZero() {
			return ""
		}
		return strconv.FormatInt(at.Unix(), 10)
	}
	flag := func(on bool) string {
		if on {
			return "1"
		}
		return ""
	}
	pin := ""
	if t.Pin != 0 {
		pin = strconv.FormatInt(t.Pin, 10)
	}
	values := map[string]string{
		PinVar:         pin,
		SnoozeUntilVar: unix(t.SnoozedUntil),
		SnoozedAtVar:   unix(t.SnoozedAt),
		SettledAtVar:   unix(t.SettledAt),
		UnsettledVar:   flag(t.Unsettled),
		WokeVar:        flag(t.Woke),
	}
	var out []string
	for _, name := range threadVars {
		if v := values[name]; v != "" {
			out = append(out, name+"="+v)
		} else {
			out = append(out, name)
		}
	}
	return out
}

// threadOf reads a thread's state from its window's user variables.
func threadOf(vars map[string]string) model.Thread {
	unix := func(name string) time.Time {
		at, err := strconv.ParseInt(vars[name], 10, 64)
		if err != nil || at <= 0 {
			return time.Time{}
		}
		return time.Unix(at, 0)
	}
	pin, _ := strconv.ParseInt(vars[PinVar], 10, 64)
	return model.Thread{
		Pin:          pin,
		SnoozedUntil: unix(SnoozeUntilVar),
		SnoozedAt:    unix(SnoozedAtVar),
		SettledAt:    unix(SettledAtVar),
		Unsettled:    vars[UnsettledVar] != "",
		Woke:         vars[WokeVar] != "",
	}
}

// SetThread writes a thread's triage state onto its window.
func (k *RealKitty) SetThread(windowID int, t model.Thread) error {
	args := append([]string{"set-user-vars", "--match", "id:" + strconv.Itoa(windowID)}, threadAssignments(t)...)
	_, err := k.mutate(args...)
	return err
}

// SetTitle gives a thread a title of your own, or with an empty one goes
// back to the window's.
func (k *RealKitty) SetTitle(windowID int, title string) error {
	assignment := TitleVar // a bare name unsets it
	if title != "" {
		assignment += "=" + title
	}
	_, err := k.mutate("set-user-vars", "--match", "id:"+strconv.Itoa(windowID), assignment)
	return err
}

// window reads one window.
func (k *RealKitty) window(windowID int) (lsWindow, error) {
	out, err := k.rc("ls", "--match", "id:"+strconv.Itoa(windowID))
	if err != nil {
		return lsWindow{}, err
	}
	var osWindows []lsOSWindow
	if err := json.Unmarshal([]byte(out), &osWindows); err != nil {
		return lsWindow{}, fmt.Errorf("couldn't parse kitten @ ls output: %w", err)
	}
	for _, osw := range osWindows {
		for _, tab := range osw.Tabs {
			for _, w := range tab.Windows {
				if w.ID == windowID {
					return w, nil
				}
			}
		}
	}
	return lsWindow{}, fmt.Errorf("no kitty window %d", windowID)
}

// agentPrograms are the agents recognized by the program running in a window,
// so one started without ksesh's hooks still shows up in `ksesh agent list`.
var agentPrograms = []string{"claude", "codex", "gemini", "opencode", "aider", "amp", "pi", "cursor-agent", "copilot"}

// SetAgentStatus writes an agent's status onto a window. Without a window id
// it is the window ksesh runs in, which is where an agent's hooks run.
func (k *RealKitty) SetAgentStatus(windowID int, u agent.Update) error {
	_, err := k.UpdateAgent(windowID, u)
	return err
}

// readsWindow reports whether an update has to read the window first. What
// the agent does moves its thread: a new prompt un-settles and wakes it,
// needing you wakes it early. And for the thread history, a conversation
// starting, ending or finishing a turn is when its window's title, session
// and thread are worth keeping; and when a resumed one takes on its thread.
// Only those pay for the read, once a turn rather than once a tool call.
func readsWindow(u agent.Update) bool {
	if u.StartTurn || (!u.Clear && (u.Status == agent.Waiting || u.Status == agent.Error)) {
		return true
	}
	return u.Conversation != "" && (u.Start || u.Clear || u.Status == agent.Done)
}

func (k *RealKitty) UpdateAgent(windowID int, u agent.Update) (*model.Agent, error) {
	if windowID == 0 {
		windowID = k.selfWindowID()
	}
	if windowID == 0 {
		return nil, fmt.Errorf("not inside a kitty window")
	}
	var seen *model.Agent
	var assignments []string
	set := func(name, value string) {
		if value == "" {
			assignments = append(assignments, name) // a bare name unsets it
		} else {
			assignments = append(assignments, name+"="+value)
		}
	}
	if readsWindow(u) {
		w, err := k.window(windowID)
		if err != nil {
			return nil, err
		}
		old := threadOf(w.UserVars)
		t := old
		// A conversation new to the window: one resumed there takes on the
		// thread it had, one that replaced another in it starts afresh.
		if conv := w.UserVars[ConvVar]; u.Conversation != "" && conv != u.Conversation {
			switch {
			case u.Carry != nil:
				t = *u.Carry
			case conv != "":
				t = model.Thread{}
			}
		}
		seen = &model.Agent{WindowID: w.ID, Title: w.Title, Path: w.dir(), Session: cmp.Or(w.UserVars[NameVar], w.SessionName), Thread: t}
		if !u.Clear {
			t = agent.OnActivity(t, u, time.Now())
			seen.Thread = t
			if t != old {
				assignments = append(assignments, threadAssignments(t)...)
			}
		}
	}
	if u.Clear {
		for _, name := range append([]string{AgentVar, StatusVar, StatusAtVar, MsgVar, PromptVar, TurnAtVar, ConvVar}, threadVars...) {
			set(name, "")
		}
	} else {
		if u.Conversation != "" {
			set(ConvVar, u.Conversation)
		}
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
	return seen, err
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
				if w.ID == self || isSidebar(w) {
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
		Title:       w.Title,
		Path:        w.dir(),
		LastFocused: w.LastFocusedAt,
		Thread:      threadOf(w.UserVars),
		CustomTitle: w.UserVars[TitleVar],
	}
	if w.CreatedAt > 0 {
		a.Created = time.Unix(0, w.CreatedAt)
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
	a.Conversation = w.UserVars[ConvVar]
	a.Status = status
	a.Message = w.UserVars[MsgVar]
	a.Prompt = w.UserVars[PromptVar]
	if at, err := strconv.ParseInt(w.UserVars[StatusAtVar], 10, 64); err == nil {
		since := time.Unix(at, 0)
		a.Since = &since
	}
	if at, err := strconv.ParseInt(w.UserVars[TurnAtVar], 10, 64); err == nil {
		turn := time.Unix(at, 0)
		a.TurnAt = &turn
	}
	if at, err := strconv.ParseInt(w.UserVars[SeenAtVar], 10, 64); err == nil {
		a.SeenAt = time.Unix(at, 0)
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
