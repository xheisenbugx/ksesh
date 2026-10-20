package model

import "time"

// Agent is a coding agent running in a kitty window, as reported by the user
// variables `ksesh agent hook` sets on it (see the agent package).
type Agent struct {
	WindowID int    `json:"window_id"`
	TabID    int    `json:"tab_id"`
	TabTitle string `json:"tab_title"`
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
	// Focused is the window the user is in; LastFocused orders windows by
	// recency, on kitty's monotonic clock.
	Focused     bool    `json:"focused"`
	LastFocused float64 `json:"-"`
}
