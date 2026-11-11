package picker

import (
	"strings"

	"charm.land/bubbles/v2/key"

	"github.com/xheisenbugx/ksesh/ui"
)

// keyMap is the keys the pickers take. Typing filters, so every action is on
// a key that types nothing: enter, esc, the arrows and ctrl.
type keyMap struct {
	Choose   key.Binding
	Quit     key.Binding
	Up       key.Binding
	Down     key.Binding
	HalfUp   key.Binding
	HalfDown key.Binding
	// Remove kills a kitty session or forgets a zoxide entry (session picker).
	Remove key.Binding
	// Preview shows or hides the preview pane (session picker).
	Preview key.Binding
	// Refresh refetches every worktree (worktree picker).
	Refresh key.Binding
}

func newKeyMap(choose string) keyMap {
	return keyMap{
		Choose:   ui.Bind("↵", choose, "enter"),
		Quit:     ui.Bind("esc", "quit", "esc", "ctrl+c"),
		Up:       ui.Bind("↑", "up", "up", "ctrl+k", "ctrl+p"),
		Down:     ui.Bind("↓", "down", "down", "ctrl+j", "ctrl+n"),
		HalfUp:   ui.Bind("^u", "half a page up", "ctrl+u"),
		HalfDown: ui.Bind("^d", "half a page down", "ctrl+d"),
		Remove:   ui.Bind("^x", "remove", "ctrl+x"),
		Preview:  ui.Bind("^o", "preview", "ctrl+o"),
		Refresh:  ui.Bind("^r", "refresh", "ctrl+r"),
	}
}

// shortKey is a key as the hints show it: ctrl+t as ^t.
func shortKey(k string) string {
	return strings.ReplaceAll(k, "ctrl+", "^")
}
