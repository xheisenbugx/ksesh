package model

import "time"

// Agent is a coding agent running in a kitty window, as reported by the user
// variables `ksesh agent hook` sets on it (see the agent package).
type Agent struct {
	WindowID int    `json:"window_id"`
	TabID    int    `json:"tab_id"`
	TabTitle string `json:"tab_title"`
	// Title is the window's own title, which agents such as Claude Code set
	// to a summary of what they are on.
	Title string `json:"title,omitempty"`
	// CustomTitle is the title you gave the thread, which wins over Title.
	CustomTitle string `json:"custom_title,omitempty"`
	// Session is the name ksesh shows for the window's kitty session, empty
	// for a window outside any session.
	Session string `json:"session"`
	Path    string `json:"path"`
	// Name is the agent ("claude"), or the program running in the window when
	// the agent was found by its process rather than its hooks.
	Name    string     `json:"name"`
	Status  string     `json:"status"`
	Since   *time.Time `json:"since,omitempty"`
	Message string     `json:"message,omitempty"`
	Prompt  string     `json:"prompt,omitempty"`
	// TurnAt is when the agent started on its current prompt.
	TurnAt *time.Time `json:"turn_at,omitempty"`
	// Created is when the window was opened.
	Created time.Time `json:"created,omitzero"`
	// Focused is the window the user is in; LastFocused orders windows by
	// recency, on kitty's monotonic clock.
	Focused     bool    `json:"focused"`
	LastFocused float64 `json:"-"`
	// Thread is what you decided about the agent: pinned, snoozed, settled.
	Thread Thread `json:"thread,omitzero"`
}

// Thread is the triage state of an agent, the way a thread is triaged in an
// inbox: pinned to the top, snoozed until later, or settled out of the way.
// It lives in user variables on the agent's window next to its status (see
// the agent package for how each changes).
type Thread struct {
	// Pin orders pinned threads, lowest first; 0 is not pinned.
	Pin int64 `json:"pin,omitempty"`
	// SnoozedUntil hides the thread until then; SnoozedAt is when it was
	// snoozed, so only what happens after that wakes it early.
	SnoozedUntil time.Time `json:"snoozed_until,omitzero"`
	SnoozedAt    time.Time `json:"snoozed_at,omitzero"`
	// SettledAt is when the thread was settled by hand.
	SettledAt time.Time `json:"settled_at,omitzero"`
	// Unsettled is set when you take a thread out of Settled, so it isn't
	// settled again for inactivity until it next does something.
	Unsettled bool `json:"unsettled,omitempty"`
	// Woke is set when a snoozed thread woke early, because the agent needs
	// you, until you look at it.
	Woke bool `json:"woke,omitempty"`
}
