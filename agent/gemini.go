package agent

import (
	"cmp"
	"encoding/json"
)

// geminiHookInput is the subset of a Gemini CLI hook's stdin ksesh reads. See
// https://geminicli.com/docs/hooks/reference/.
type geminiHookInput struct {
	HookEventName    string `json:"hook_event_name"`
	SessionID        string `json:"session_id"`
	Cwd              string `json:"cwd"`
	TranscriptPath   string `json:"transcript_path"`
	Prompt           string `json:"prompt"`
	PromptResponse   string `json:"prompt_response"`
	NotificationType string `json:"notification_type"`
	Message          string `json:"message"`
	// Details are the tool's confirmation details: type is exec, edit, mcp or
	// info, each with its own fields.
	Details struct {
		Type       string   `json:"type"`
		Title      string   `json:"title"`
		Command    string   `json:"command"`
		FileName   string   `json:"fileName"`
		FilePath   string   `json:"filePath"`
		ServerName string   `json:"serverName"`
		ToolName   string   `json:"toolName"`
		Prompt     string   `json:"prompt"`
		URLs       []string `json:"urls"`
	} `json:"details"`
}

// FromGeminiHook maps a Gemini CLI hook event onto a status update. ok is
// false for events that don't change anything.
//
// Gemini CLI has no event for a turn that fails, so it never reports error.
func FromGeminiHook(raw []byte) (Update, bool) {
	var in geminiHookInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return Update{}, false
	}
	u := Update{Agent: "gemini", Conversation: in.SessionID, Cwd: in.Cwd, Transcript: in.TranscriptPath}
	switch in.HookEventName {
	case "SessionStart":
		u.Start = true
		u.Status = Idle
		u.SetMessage, u.SetPrompt = true, true
	case "BeforeAgent":
		// Once per prompt, before the model sees it.
		u.Status = Working
		u.Prompt, u.SetPrompt = Summarize(in.Prompt), true
		u.SetMessage = true
		u.StartTurn = true
	case "BeforeTool":
		// A tool runs once it is confirmed, so this is the first sign the
		// agent runs again after asking you.
		u.Status = Working
		u.SetMessage = true
	case "Notification":
		if in.NotificationType != "" && in.NotificationType != "ToolPermission" {
			return Update{}, false
		}
		u.Status = Waiting
		u.Message, u.SetMessage = geminiPermissionMessage(in), true
	case "AfterAgent":
		u.Status = Done
		u.Message, u.SetMessage = Summarize(in.PromptResponse), true
	case "SessionEnd":
		u.Clear = true
	default:
		return Update{}, false
	}
	return u, true
}

// geminiPermissionMessage says what a tool confirmation would allow: "Allow
// shell: npm test", "Allow edit: main.go", "Allow github: create_issue".
func geminiPermissionMessage(in geminiHookInput) string {
	d := in.Details
	allow := func(what, detail string) string {
		if detail = Summarize(detail); detail == "" {
			return "Allow " + what
		}
		return Summarize("Allow " + what + ": " + detail)
	}
	switch d.Type {
	case "exec":
		return allow("shell", d.Command)
	case "edit":
		return allow("edit", cmp.Or(d.FileName, d.FilePath))
	case "mcp":
		return allow(cmp.Or(d.ServerName, "MCP"), d.ToolName)
	case "info":
		url := ""
		if len(d.URLs) > 0 {
			url = d.URLs[0]
		}
		if url != "" {
			return allow("fetch", url)
		}
		return allow(cmp.Or(d.Title, "tool"), d.Prompt)
	}
	return cmp.Or(Summarize(in.Message), "Needs permission")
}
