package model

import "time"

// ThreadRecord is what the thread history keeps of a conversation with an
// agent, so its thread outlives its kitty window: enough to show it as a
// closed thread and to resume it. See the threads package.
type ThreadRecord struct {
	// Conversation is the agent's id for it (Claude Code's session_id).
	Conversation string `json:"conversation"`
	// Agent is the agent's name ("claude").
	Agent string `json:"agent"`
	// Cwd is the directory the agent runs in, where it resumes: Claude Code
	// keeps its conversations by project directory.
	Cwd string `json:"cwd,omitempty"`
	// Host is the machine Cwd is on, as ssh reaches it, when it isn't this
	// one.
	Host       string `json:"host,omitempty"`
	Transcript string `json:"transcript,omitempty"`
	// Title is the last title the agent gave its window worth keeping.
	Title string `json:"title,omitempty"`
	// CustomTitle is a title you gave the thread (the sidebar's R), which
	// it keeps when it is resumed.
	CustomTitle string `json:"custom_title,omitempty"`
	// Session is the ksesh session its window was in, empty for none.
	Session string `json:"session,omitempty"`
	// WindowID is the kitty window it was last seen in.
	WindowID int    `json:"window_id,omitempty"`
	Status   string `json:"status,omitempty"`
	Message  string `json:"message,omitempty"`
	Prompt   string `json:"prompt,omitempty"`
	// StatusAt is when the status last changed, TurnAt when the agent
	// started on its last prompt.
	StatusAt time.Time `json:"status_at,omitzero"`
	TurnAt   time.Time `json:"turn_at,omitzero"`
	// EndedAt is when the agent's session ended (it exited), if it said so.
	EndedAt time.Time `json:"ended_at,omitzero"`
	// UpdatedAt is when the record was last written.
	UpdatedAt time.Time `json:"updated_at,omitzero"`
	// Thread is its triage state, carried over to the window it resumes in.
	Thread Thread `json:"thread,omitzero"`
}
