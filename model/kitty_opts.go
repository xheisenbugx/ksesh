package model

// KittyTabOpts are the arguments for creating a tab in a session.
type KittyTabOpts struct {
	Name          string // tab title; empty lets kitty title it from the program
	StartDir      string
	TargetSession string
	// Command becomes the new window's process, so the tab closes when the
	// command exits. Empty starts the default shell.
	Command    string
	Background bool // create the tab without focusing it
}
