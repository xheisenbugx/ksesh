package model

// KittyTab is a kitty tab belonging to a session, the kitty equivalent of a
// tmux window.
type KittyTab struct {
	Name   string // tab title
	Path   string // working directory of the tab's active window
	Index  int    // 1-based position among the session's tabs
	ID     int    // kitty tab id
	Active bool   // the tab the session would show when switched to
}
