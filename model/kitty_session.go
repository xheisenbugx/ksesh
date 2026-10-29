package model

import "time"

// KittySession is a kitty session: every window kitty has tagged with the same
// session_name (see https://sw.kovidgoyal.net/kitty/sessions/). Kitty itself
// has no session object, so all of this is derived from `kitten @ ls`.
type KittySession struct {
	// Name is the name sesh shows and matches on. It is NativeName unless the
	// session was renamed, which kitty cannot do natively; a rename is stored
	// as a user variable on the session's windows instead.
	Name string
	// NativeName is kitty's own session_name, the one goto_session,
	// close_session and `--match session:` understand.
	NativeName string
	// Path is the session root: the directory sesh created it in, or the
	// working directory of its first window when kitty created it.
	Path string
	// Attached is 1 for the session the user is in, 0 otherwise.
	Attached int
	// Windows is the number of kitty tabs the session spans.
	Windows int
	// FocusWindowID is the window kitty focuses when switching to the session:
	// the one focused most recently.
	FocusWindowID int
	// LastFocused orders sessions by recency. It is kitty's monotonic clock,
	// only meaningful relative to other sessions of the same kitty instance.
	LastFocused  float64
	Created      *time.Time
	LastAttached *time.Time
	Activity     *time.Time
	Alerts       []string
	// AgentStatus is the most urgent status of the agents in the session
	// (see the agent package), empty when none reports one.
	AgentStatus string
}
