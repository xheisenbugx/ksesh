package agent

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFromClaudeHook(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  Update
		ok    bool
	}{
		{
			name:  "a prompt starts work and replaces the last message",
			input: `{"hook_event_name":"UserPromptSubmit","prompt":"\n  add a **login** page\nwith tests"}`,
			want:  Update{Agent: "claude", Status: Working, Prompt: "add a login page", SetPrompt: true, SetMessage: true},
			ok:    true,
		},
		{
			name:  "a tool call after a permission prompt is working again",
			input: `{"hook_event_name":"PostToolUse","tool_name":"Bash"}`,
			want:  Update{Agent: "claude", Status: Working, SetMessage: true},
			ok:    true,
		},
		{
			name:  "a permission request waits on you",
			input: `{"hook_event_name":"PermissionRequest","tool_name":"Bash"}`,
			want:  Update{Agent: "claude", Status: Waiting, Message: "Needs permission: Bash", SetMessage: true},
			ok:    true,
		},
		{
			name:  "a permission prompt notification waits on you",
			input: `{"hook_event_name":"Notification","notification_type":"permission_prompt","message":"Claude needs your permission to use Bash"}`,
			want:  Update{Agent: "claude", Status: Waiting, Message: "Claude needs your permission to use Bash", SetMessage: true},
			ok:    true,
		},
		{
			name:  "a notification without a type is trusted to the matcher",
			input: `{"hook_event_name":"Notification","message":"Claude is waiting for your input"}`,
			want:  Update{Agent: "claude", Status: Waiting, Message: "Claude is waiting for your input", SetMessage: true},
			ok:    true,
		},
		{
			name:  "the idle reminder after a stop changes nothing",
			input: `{"hook_event_name":"Notification","notification_type":"idle_prompt","message":"Claude is waiting for your input"}`,
		},
		{
			name:  "stop is done, with the last message",
			input: `{"hook_event_name":"Stop","last_assistant_message":"## Done\nAdded the page."}`,
			want:  Update{Agent: "claude", Status: Done, Message: "Done", SetMessage: true},
			ok:    true,
		},
		{
			name:  "a failed stop is an error",
			input: `{"hook_event_name":"StopFailure","error":"rate_limit"}`,
			want:  Update{Agent: "claude", Status: Error, Message: "rate_limit", SetMessage: true},
			ok:    true,
		},
		{
			name:  "a session start registers the agent as idle",
			input: `{"hook_event_name":"SessionStart","source":"startup"}`,
			want:  Update{Agent: "claude", Status: Idle, SetMessage: true, SetPrompt: true},
			ok:    true,
		},
		{
			name:  "a session end clears the window",
			input: `{"hook_event_name":"SessionEnd"}`,
			want:  Update{Agent: "claude", Clear: true},
			ok:    true,
		},
		{
			name:  "subagents don't speak for the window",
			input: `{"hook_event_name":"Stop","agent_id":"abc"}`,
		},
		{
			name:  "unknown events are ignored",
			input: `{"hook_event_name":"PreCompact"}`,
		},
		{
			name:  "garbage is ignored",
			input: `not json`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := FromClaudeHook([]byte(tt.input))
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSummarize(t *testing.T) {
	assert.Equal(t, "", Summarize("  \n\n"))
	assert.Equal(t, "first line", Summarize("\n  first   line \nsecond"))
	assert.Equal(t, "run go test", Summarize("- run `go test`"))
	long := strings.Repeat("a", MaxMessage+10)
	got := Summarize(long)
	assert.Equal(t, MaxMessage, len([]rune(got)))
	assert.True(t, strings.HasSuffix(got, "…"))
}
