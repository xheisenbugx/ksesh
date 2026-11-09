package agent

import (
	"cmp"
	"encoding/json"
	"strings"
)

// codexHookInput is the subset of a Codex hook's stdin ksesh reads. See
// https://developers.openai.com/codex/hooks and the schemas in
// codex-rs/hooks/schema/generated. It also reads the JSON Codex's older
// `notify` program gets as its last argument, whose keys are kebab-case.
type codexHookInput struct {
	HookEventName        string          `json:"hook_event_name"`
	AgentID              string          `json:"agent_id"`
	Source               string          `json:"source"`
	Prompt               string          `json:"prompt"`
	ToolName             string          `json:"tool_name"`
	ToolInput            json.RawMessage `json:"tool_input"`
	LastAssistantMessage string          `json:"last_assistant_message"`
	// PostModelRequest: completed, failed or cancelled.
	Status string `json:"status"`
	Error  string `json:"error"`

	// notify
	Type                       string `json:"type"`
	NotifyLastAssistantMessage string `json:"last-assistant-message"`
}

// FromCodexHook maps a Codex hook event, or a `notify` notification, onto a
// status update. ok is false for events that don't change anything.
//
// A subagent's events carry its agent_id. Like Claude's, they are left out,
// except for an approval it asks for: that blocks on you all the same. What
// it does after the approval isn't taken as the window working again, since
// another subagent may still be waiting on you; the main thread's next event
// says so.
func FromCodexHook(raw []byte) (Update, bool) {
	var in codexHookInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return Update{}, false
	}
	if in.AgentID != "" && in.HookEventName != "PermissionRequest" {
		return Update{}, false
	}
	u := Update{Agent: "codex"}
	switch in.HookEventName {
	case "SessionStart":
		// Compaction restarts the session in the middle of a turn.
		if in.Source == "compact" {
			return Update{}, false
		}
		u.Status = Idle
		u.SetMessage, u.SetPrompt = true, true
	case "UserPromptSubmit":
		u.Status = Working
		u.Prompt, u.SetPrompt = Summarize(in.Prompt), true
		u.SetMessage = true
		u.StartTurn = true
	case "PostToolUse":
		// After an approval, the first sign the agent runs again.
		u.Status = Working
		u.SetMessage = true
	case "PostModelRequest":
		switch in.Status {
		case "failed":
			// Codex retries a failed request, and the retry's own event puts
			// the window back to working.
			u.Status = Error
			u.Message, u.SetMessage = Summarize(cmp.Or(in.Error, "Model request failed")), true
		case "completed":
			u.Status = Working
			u.SetMessage = true
		default: // cancelled: Interrupt says what happened
			return Update{}, false
		}
	case "PermissionRequest":
		u.Status = Waiting
		u.Message, u.SetMessage = codexPermissionMessage(in.ToolName, in.ToolInput), true
	case "Stop":
		u.Status = Done
		u.Message, u.SetMessage = Summarize(in.LastAssistantMessage), true
	case "Interrupt":
		// You stopped it, so you are looking at it: nothing to tell you.
		u.Status = Idle
		u.SetMessage = true
	case "SessionEnd":
		u.Clear = true
	case "":
		if in.Type != "agent-turn-complete" {
			return Update{}, false
		}
		u.Status = Done
		u.Message, u.SetMessage = Summarize(in.NotifyLastAssistantMessage), true
	default:
		return Update{}, false
	}
	return u, true
}

// codexToolInput is what ksesh reads from the tool_input of Codex's approval
// requests: Bash and apply_patch put the command or patch in command, network
// access says "network-access <host>" in description, MCP tools send their
// own arguments.
type codexToolInput struct {
	Command     json.RawMessage `json:"command"`
	Description string          `json:"description"`
	Chars       string          `json:"chars"`
	Reason      string          `json:"reason"`
	FilePath    string          `json:"file_path"`
	Path        string          `json:"path"`
	URL         string          `json:"url"`
	Query       string          `json:"query"`
}

// codexPermissionMessage says what an approval would allow: "Allow shell: npm
// test", "Allow edit: src/main.go", "Allow network: github.com".
func codexPermissionMessage(tool string, raw json.RawMessage) string {
	var ti codexToolInput
	_ = json.Unmarshal(raw, &ti)
	command := commandString(ti.Command)
	allow := func(what, detail string) string {
		if detail = Summarize(detail); detail == "" {
			return "Allow " + what
		}
		return Summarize("Allow " + what + ": " + detail)
	}
	switch tool {
	case "":
		return "Needs approval"
	case "Bash":
		if host, ok := strings.CutPrefix(ti.Description, "network-access "); ok {
			return allow("network", host)
		}
		return allow("shell", command)
	case "apply_patch":
		return allow("edit", strings.Join(patchFiles(command), ", "))
	case "write_stdin":
		return allow("input", ti.Chars)
	case "request_permissions":
		return allow("permissions", ti.Reason)
	}
	return allow(tool, cmp.Or(command, ti.FilePath, ti.Path, ti.URL, ti.Query))
}

// commandString reads a command given as a string or as argv.
func commandString(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var argv []string
	if json.Unmarshal(raw, &argv) == nil {
		return strings.Join(argv, " ")
	}
	return ""
}

// patchFiles lists the files an apply_patch patch touches.
func patchFiles(patch string) []string {
	var files []string
	for line := range strings.SplitSeq(patch, "\n") {
		for _, prefix := range []string{"*** Add File: ", "*** Update File: ", "*** Delete File: "} {
			if file, ok := strings.CutPrefix(strings.TrimSpace(line), prefix); ok {
				files = append(files, strings.TrimSpace(file))
			}
		}
	}
	return files
}
