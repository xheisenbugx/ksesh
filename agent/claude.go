package agent

import (
	"cmp"
	"encoding/json"
	"strings"
	"unicode/utf8"
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
}

// claudeHookInput is the subset of a Claude Code hook's stdin ksesh reads.
// See https://code.claude.com/docs/en/hooks.
type claudeHookInput struct {
	HookEventName        string `json:"hook_event_name"`
	AgentID              string `json:"agent_id"`
	Prompt               string `json:"prompt"`
	Message              string `json:"message"`
	NotificationType     string `json:"notification_type"`
	ToolName             string `json:"tool_name"`
	ToolInput            struct {
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
	u := Update{Agent: "claude"}
	switch in.HookEventName {
	case "SessionStart":
		u.Status = Idle
		u.SetMessage, u.SetPrompt = true, true
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
