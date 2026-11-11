package agent

import (
	"cmp"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/xheisenbugx/ksesh/model"
)

// Update is what one hook event changes on the agent's window. Message and
// Prompt are only written when their Set flag is on, so an event that knows
// nothing about them (a tool call) leaves them as they were.
type Update struct {
	Agent      string
	Status     Status
	Message    string
	SetMessage bool
	Prompt     string
	SetPrompt  bool
	// StartTurn marks when the agent started on a prompt, so a turn that ends
	// quickly can finish without a notification.
	StartTurn bool
	// Clear drops every agent variable: the agent is gone.
	Clear bool
	// Start marks the agent's session starting: a new conversation, or one
	// resumed, in the window.
	Start bool

	// Conversation is the agent's id for the conversation (Claude Code's
	// session_id), which resumes it; Cwd is where it runs and Transcript
	// where it is written. Only agents that report one are kept in the
	// thread history, and can be resumed.
	Conversation string
	Cwd          string
	Transcript   string
	// Host names the machine the hook runs on ($KSESH_HOST, else its
	// hostname), for kitty to tell an agent on another machine, reached
	// with `kitten ssh`, from one on its own.
	Host string
	// Carry is the triage state the thread history has for the
	// conversation, given to the window when the conversation first shows
	// up in it: the thread was resumed there.
	Carry *model.Thread
	// CarryTitle is the title you gave the thread, which a window it is
	// resumed in takes on unless it has one of its own.
	CarryTitle string
}

// claudeHookInput is the subset of a Claude Code hook's stdin ksesh reads.
// See https://code.claude.com/docs/en/hooks.
type claudeHookInput struct {
	HookEventName    string `json:"hook_event_name"`
	SessionID        string `json:"session_id"`
	TranscriptPath   string `json:"transcript_path"`
	Cwd              string `json:"cwd"`
	AgentID          string `json:"agent_id"`
	Prompt           string `json:"prompt"`
	Message          string `json:"message"`
	NotificationType string `json:"notification_type"`
	ToolName         string `json:"tool_name"`
	ToolInput        struct {
		Command  string `json:"command"`
		FilePath string `json:"file_path"`
		URL      string `json:"url"`
		Pattern  string `json:"pattern"`
		Query    string `json:"query"`
	} `json:"tool_input"`
	LastAssistantMessage string `json:"last_assistant_message"`
	Error                string `json:"error"`
}

// waitingNotifications are the Notification types that mean Claude is blocked
// on you. The others (idle_prompt, auth_success, …) arrive after it already
// stopped, or aren't about you at all.
var waitingNotifications = map[string]bool{
	"permission_prompt":      true,
	"elicitation_dialog":     true,
	"elicitation_url_dialog": true,
	"agent_needs_input":      true,
}

// MaxMessage is how long a message or prompt kept in a user variable may be.
// It is shown in a tab bar and a picker row, so a line is plenty.
const MaxMessage = 120

// FromClaudeHook maps a Claude Code hook event onto a status update. ok is
// false for events that don't change anything, including every event from a
// subagent: the subagent runs inside the main agent's turn, whose own events
// already say what the window is doing.
func FromClaudeHook(raw []byte) (Update, bool) {
	var in claudeHookInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return Update{}, false
	}
	if in.AgentID != "" {
		return Update{}, false
	}
	u := Update{Agent: "claude", Conversation: in.SessionID, Cwd: in.Cwd, Transcript: in.TranscriptPath}
	switch in.HookEventName {
	case "SessionStart":
		u.Status = Idle
		u.SetMessage, u.SetPrompt = true, true
		u.Start = true
	case "UserPromptSubmit":
		u.Status = Working
		u.Prompt, u.SetPrompt = Summarize(in.Prompt), true
		u.SetMessage = true
		u.StartTurn = true
	case "PostToolUse", "PostToolUseFailure", "PermissionDenied":
		// After a permission prompt is answered, this is the first sign the
		// agent is running again; after a denial by auto mode it carries on.
		u.Status = Working
		u.SetMessage = true
	case "PermissionRequest":
		u.Status = Waiting
		u.Message, u.SetMessage = permissionMessage(in), true
	case "Notification":
		if in.NotificationType != "" && !waitingNotifications[in.NotificationType] {
			return Update{}, false
		}
		u.Status = Waiting
		// The permission prompt notification follows PermissionRequest, whose
		// message says what the tool would do; this one only names the tool.
		if in.NotificationType != "permission_prompt" {
			u.Message, u.SetMessage = Summarize(in.Message), true
		}
	case "Stop":
		u.Status = Done
		u.Message, u.SetMessage = Summarize(in.LastAssistantMessage), true
	case "StopFailure":
		u.Status = Error
		u.Message, u.SetMessage = Summarize(cmp.Or(in.Error, in.Message, "Stopped with an error")), true
	case "SessionEnd":
		u.Clear = true
	default:
		return Update{}, false
	}
	return u, true
}

// ResumeCommand is the shell command that resumes an agent's conversation,
// run in the directory it ran in. ok is false for an agent ksesh doesn't
// know how to resume.
func ResumeCommand(agentName, conversation string) (string, bool) {
	if conversation == "" {
		return "", false
	}
	switch agentName {
	case "claude":
		return "claude --resume " + shellQuote(conversation), true
	case "codex":
		return "codex resume " + shellQuote(conversation), true
	case "gemini":
		return "gemini --resume " + shellQuote(conversation), true
	}
	return "", false
}

// permissionMessage says what a permission prompt would allow, e.g. "Allow
// Bash: npm test", so it can be answered without looking at the agent.
func permissionMessage(in claudeHookInput) string {
	if in.ToolName == "" {
		return "Needs permission"
	}
	ti := in.ToolInput
	if detail := Summarize(cmp.Or(ti.Command, ti.FilePath, ti.URL, ti.Pattern, ti.Query)); detail != "" {
		return Summarize("Allow " + in.ToolName + ": " + detail)
	}
	return "Allow " + in.ToolName
}

// Summarize squeezes text into one line of at most MaxMessage characters: the
// first non-empty line, with markdown emphasis and runs of whitespace dropped.
func Summarize(text string) string {
	line := ""
	for l := range strings.SplitSeq(text, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			line = l
			break
		}
	}
	line = strings.TrimLeft(line, "#>-* ")
	line = strings.NewReplacer("**", "", "`", "").Replace(line)
	line = strings.Join(strings.Fields(line), " ")
	if utf8.RuneCountInString(line) <= MaxMessage {
		return line
	}
	runes := []rune(line)
	return strings.TrimSpace(string(runes[:MaxMessage-1])) + "…"
}
