package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The payloads follow codex-rs/hooks/schema/generated/*.command.input.schema.json
// and what codex-rs/core/src/tools/approvals.rs puts in a PermissionRequest.
func TestFromCodexHook(t *testing.T) {
	const common = `"session_id":"019a","transcript_path":null,"cwd":"/src/app","model":"gpt-5.5","permission_mode":"default"`
	tests := []struct {
		name  string
		input string
		want  Update
		ok    bool
	}{
		{
			name:  "a new session is idle, with nothing left of the last one",
			input: `{` + common + `,"hook_event_name":"SessionStart","source":"startup"}`,
			want:  Update{Agent: "codex", Status: Idle, SetMessage: true, SetPrompt: true},
			ok:    true,
		},
		{
			name:  "compaction restarting the session mid-turn changes nothing",
			input: `{` + common + `,"hook_event_name":"SessionStart","source":"compact"}`,
		},
		{
			name:  "a prompt starts work",
			input: `{` + common + `,"hook_event_name":"UserPromptSubmit","turn_id":"t1","prompt":"fix the **flaky** test\nand push"}`,
			want:  Update{Agent: "codex", Status: Working, Prompt: "fix the flaky test", SetPrompt: true, SetMessage: true, StartTurn: true},
			ok:    true,
		},
		{
			name:  "a shell approval says what it would run",
			input: `{` + common + `,"hook_event_name":"PermissionRequest","turn_id":"t1","tool_name":"Bash","tool_input":{"command":"npm test","description":"Run the test suite"}}`,
			want:  Update{Agent: "codex", Status: Waiting, Message: "Allow shell: npm test", SetMessage: true},
			ok:    true,
		},
		{
			name:  "a network approval names the host",
			input: `{` + common + `,"hook_event_name":"PermissionRequest","turn_id":"t1","tool_name":"Bash","tool_input":{"command":"curl https://api.github.com","description":"network-access api.github.com"}}`,
			want:  Update{Agent: "codex", Status: Waiting, Message: "Allow network: api.github.com", SetMessage: true},
			ok:    true,
		},
		{
			name:  "a patch approval names the files, not the patch",
			input: `{` + common + `,"hook_event_name":"PermissionRequest","turn_id":"t1","tool_name":"apply_patch","tool_input":{"command":"*** Begin Patch\n*** Update File: src/main.go\n@@\n-a\n+b\n*** Add File: src/new.go\n+package main\n*** End Patch"}}`,
			want:  Update{Agent: "codex", Status: Waiting, Message: "Allow edit: src/main.go, src/new.go", SetMessage: true},
			ok:    true,
		},
		{
			name:  "an MCP tool approval names the tool and what it is given",
			input: `{` + common + `,"hook_event_name":"PermissionRequest","turn_id":"t1","tool_name":"mcp__filesystem__read_file","tool_input":{"path":"/etc/hosts"}}`,
			want:  Update{Agent: "codex", Status: Waiting, Message: "Allow mcp__filesystem__read_file: /etc/hosts", SetMessage: true},
			ok:    true,
		},
		{
			name:  "asking for more permissions gives the reason",
			input: `{` + common + `,"hook_event_name":"PermissionRequest","turn_id":"t1","tool_name":"request_permissions","tool_input":{"reason":"Needs to write outside the workspace","permissions":{}}}`,
			want:  Update{Agent: "codex", Status: Waiting, Message: "Allow permissions: Needs to write outside the workspace", SetMessage: true},
			ok:    true,
		},
		{
			name:  "a subagent asking for approval blocks on you all the same",
			input: `{` + common + `,"hook_event_name":"PermissionRequest","turn_id":"t2","agent_id":"a1","agent_type":"worker","tool_name":"Bash","tool_input":{"command":["cargo","build"]}}`,
			want:  Update{Agent: "codex", Status: Waiting, Message: "Allow shell: cargo build", SetMessage: true},
			ok:    true,
		},
		{
			name:  "a tool that ran means it is working again",
			input: `{` + common + `,"hook_event_name":"PostToolUse","turn_id":"t1","tool_name":"Bash","tool_use_id":"c1","tool_input":{"command":"npm test"},"tool_response":"ok"}`,
			want:  Update{Agent: "codex", Status: Working, SetMessage: true},
			ok:    true,
		},
		{
			name:  "a subagent's tool doesn't say the window is working",
			input: `{` + common + `,"hook_event_name":"PostToolUse","turn_id":"t2","agent_id":"a1","tool_name":"Bash","tool_use_id":"c1","tool_input":{},"tool_response":"ok"}`,
		},
		{
			name:  "a failed model request is an error",
			input: `{"session_id":"019a","transcript_path":null,"cwd":"/src/app","model":"gpt-5.5","turn_id":"t1","hook_event_name":"PostModelRequest","status":"failed","error":"stream disconnected before completion","token_usage":null}`,
			want:  Update{Agent: "codex", Status: Error, Message: "stream disconnected before completion", SetMessage: true},
			ok:    true,
		},
		{
			name:  "a completed model request is working, past any error",
			input: `{"session_id":"019a","transcript_path":null,"cwd":"/src/app","model":"gpt-5.5","turn_id":"t1","hook_event_name":"PostModelRequest","status":"completed","error":null,"token_usage":null}`,
			want:  Update{Agent: "codex", Status: Working, SetMessage: true},
			ok:    true,
		},
		{
			name:  "a cancelled model request is left to Interrupt",
			input: `{"session_id":"019a","transcript_path":null,"cwd":"/src/app","model":"gpt-5.5","turn_id":"t1","hook_event_name":"PostModelRequest","status":"cancelled","error":null,"token_usage":null}`,
		},
		{
			name:  "a subagent's model requests are its own",
			input: `{"session_id":"019a","transcript_path":null,"cwd":"/src/app","model":"gpt-5.5","turn_id":"t2","agent_id":"a1","hook_event_name":"PostModelRequest","status":"failed","error":"boom","token_usage":null}`,
		},
		{
			name:  "stop is done, with the last message",
			input: `{` + common + `,"hook_event_name":"Stop","turn_id":"t1","stop_hook_active":false,"last_assistant_message":"All tests pass.\n\nDetails…"}`,
			want:  Update{Agent: "codex", Status: Done, Message: "All tests pass.", SetMessage: true},
			ok:    true,
		},
		{
			name:  "stop without a last message",
			input: `{` + common + `,"hook_event_name":"Stop","turn_id":"t1","stop_hook_active":false,"last_assistant_message":null}`,
			want:  Update{Agent: "codex", Status: Done, SetMessage: true},
			ok:    true,
		},
		{
			name:  "an interrupt is idle: you are the one who stopped it",
			input: `{` + common + `,"hook_event_name":"Interrupt","turn_id":"t1"}`,
			want:  Update{Agent: "codex", Status: Idle, SetMessage: true},
			ok:    true,
		},
		{
			name:  "the session ending clears everything",
			input: `{"session_id":"019a","transcript_path":null,"cwd":"/src/app","hook_event_name":"SessionEnd","reason":"other"}`,
			want:  Update{Agent: "codex", Clear: true},
			ok:    true,
		},
		{
			name:  "notify's turn complete is done",
			input: `{"type":"agent-turn-complete","thread-id":"019a","turn-id":"t1","cwd":"/src/app","input-messages":["fix it"],"last-assistant-message":"Fixed the flaky test."}`,
			want:  Update{Agent: "codex", Status: Done, Message: "Fixed the flaky test.", SetMessage: true},
			ok:    true,
		},
		{
			name:  "other notify types are ignored",
			input: `{"type":"something-new"}`,
		},
		{
			name:  "events ksesh doesn't listen to are ignored",
			input: `{` + common + `,"hook_event_name":"PreCompact","turn_id":"t1","trigger":"auto"}`,
		},
		{
			name:  "garbage is ignored",
			input: `not json`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := FromCodexHook([]byte(tt.input))
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFromHookDispatchesByAgent(t *testing.T) {
	stop := []byte(`{"hook_event_name":"Stop","last_assistant_message":"done"}`)
	for _, name := range []string{"", "claude", "codex"} {
		u, ok := FromHook(name, stop)
		assert.True(t, ok, name)
		assert.Equal(t, Done, u.Status, name)
	}
	u, _ := FromHook("codex", stop)
	assert.Equal(t, "codex", u.Agent)
	_, ok := FromHook("gemini", stop)
	assert.False(t, ok, "Gemini has no Stop event")
	_, ok = FromHook("aider", stop)
	assert.False(t, ok)
}
