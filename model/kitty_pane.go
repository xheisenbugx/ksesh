package model

// KittyWindow is a kitty window (a split inside a tab), the kitty equivalent of
// a tmux pane.
type KittyWindow struct {
	TabIndex int    // 1-based position of the tab among the session's tabs
	TabTitle string // title of the tab holding the window
	ID       int    // kitty window id
	Title    string
	Command  string // foreground program
	Path     string // working directory
}
