package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The payloads follow https://geminicli.com/docs/hooks/reference/ and the
// details packages/core/src/hooks/hookSystem.ts serializes for a tool
// confirmation.
func TestFromGeminiHook(t *testing.T) {
	const common = `"session_id":"s1","transcript_path":"/t.json","cwd":"/src/app","timestamp":"2026-01-01T00:00:00Z"`
	tests := []struct {
		name  string
		input string
		want  Update
		ok    bool
	}{
		{
			name:  "a new session is idle",
			input: `{` + common + `,"hook_event_name":"SessionStart","source":"startup"}`,
			want:  Update{Agent: "gemini", Status: Idle, SetMessage: true, SetPrompt: true},
			ok:    true,
		},
		{
			name:  "a prompt starts work",
			input: `{` + common + `,"hook_event_name":"BeforeAgent","prompt":"Summarize the README"}`,
			want:  Update{Agent: "gemini", Status: Working, Prompt: "Summarize the README", SetPrompt: true, SetMessage: true, StartTurn: true},
			ok:    true,
		},
		{
			name:  "a shell confirmation says what it would run",
			input: `{` + common + `,"hook_event_name":"Notification","notification_type":"ToolPermission","message":"Tool Confirm Shell Command requires execution","details":{"type":"exec","title":"Confirm Shell Command","command":"npm test","rootCommand":"npm"}}`,
			want:  Update{Agent: "gemini", Status: Waiting, Message: "Allow shell: npm test", SetMessage: true},
			ok:    true,
		},
		{
			name:  "an edit confirmation names the file",
			input: `{` + common + `,"hook_event_name":"Notification","notification_type":"ToolPermission","message":"Tool Confirm Edit requires editing","details":{"type":"edit","title":"Confirm Edit","fileName":"main.go","filePath":"/src/app/main.go","fileDiff":"@@ -1 +1 @@","originalContent":"a","newContent":"b","isModifying":false}}`,
			want:  Update{Agent: "gemini", Status: Waiting, Message: "Allow edit: main.go", SetMessage: true},
			ok:    true,
		},
		{
			name:  "an MCP confirmation names the server and tool",
			input: `{` + common + `,"hook_event_name":"Notification","notification_type":"ToolPermission","message":"Tool create_issue requires MCP","details":{"type":"mcp","title":"Confirm MCP Tool","serverName":"github","toolName":"create_issue","toolDisplayName":"Create issue"}}`,
			want:  Update{Agent: "gemini", Status: Waiting, Message: "Allow github: create_issue", SetMessage: true},
			ok:    true,
		},
		{
			name:  "a fetch confirmation names the URL",
			input: `{` + common + `,"hook_event_name":"Notification","notification_type":"ToolPermission","message":"Tool Confirm Web Fetch requires information","details":{"type":"info","title":"Confirm Web Fetch","prompt":"Fetch the docs","urls":["https://example.com/docs"]}}`,
			want:  Update{Agent: "gemini", Status: Waiting, Message: "Allow fetch: https://example.com/docs", SetMessage: true},
			ok:    true,
		},
		{
			name:  "an unknown confirmation falls back to Gemini's message",
			input: `{` + common + `,"hook_event_name":"Notification","notification_type":"ToolPermission","message":"Tool requires confirmation","details":{"type":"ask_user","title":"Question"}}`,
			want:  Update{Agent: "gemini", Status: Waiting, Message: "Tool requires confirmation", SetMessage: true},
			ok:    true,
		},
		{
			name:  "other notifications aren't about you",
			input: `{` + common + `,"hook_event_name":"Notification","notification_type":"SomethingElse","message":"hi","details":{}}`,
		},
		{
			name:  "a tool starting means it was confirmed: working again",
			input: `{` + common + `,"hook_event_name":"BeforeTool","tool_name":"run_shell_command","tool_input":{"command":"npm test"}}`,
			want:  Update{Agent: "gemini", Status: Working, SetMessage: true},
			ok:    true,
		},
		{
			name:  "the end of the agent loop is done, with the response",
			input: `{` + common + `,"hook_event_name":"AfterAgent","prompt":"Summarize the README","prompt_response":"The README describes ksesh.\nMore.","stop_hook_active":false}`,
			want:  Update{Agent: "gemini", Status: Done, Message: "The README describes ksesh.", SetMessage: true},
			ok:    true,
		},
		{
			name:  "the session ending clears everything",
			input: `{` + common + `,"hook_event_name":"SessionEnd","reason":"exit"}`,
			want:  Update{Agent: "gemini", Clear: true},
			ok:    true,
		},
		{
			name:  "model events are ignored",
			input: `{` + common + `,"hook_event_name":"AfterModel","llm_request":{},"llm_response":{}}`,
		},
		{
			name:  "garbage is ignored",
			input: `{`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := FromGeminiHook([]byte(tt.input))
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}
