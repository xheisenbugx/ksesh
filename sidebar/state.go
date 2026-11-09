// Package sidebar is the thread sidebar: a list of every coding agent in
// kitty, triaged the way T3 Code triages its threads (see agent/thread.go),
// that runs in a window docked to the edge of a kitty tab.
package sidebar

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/xheisenbugx/ksesh/agent"
)

// State is what every sidebar shares, kept in a small file so the sidebars of
// all tabs agree, and so contrib/ksesh_agents.py knows whether to give a tab
// you go to a sidebar, and how to launch it.
type State struct {
	// Enabled gives every tab a sidebar as you go to it.
	Enabled bool `json:"enabled"`
	// Launch is the `kitten @` command that docks a sidebar to a tab, for the
	// watcher to run with --match added.
	Launch []string `json:"launch,omitempty"`
	// Expanded lists the open shelves (working, snoozed, settled). They start
	// closed, as in T3 Code.
	Expanded map[agent.Section]bool `json:"expanded,omitempty"`
	// Session shows only the threads of the tab's own session.
	Session bool `json:"session,omitempty"`
	// Group groups the inbox by session, like T3 Code's projects.
	Group bool `json:"group,omitempty"`
}

// StatePath is $XDG_STATE_HOME/ksesh/sidebar.json, ~/.local/state by default.
func StatePath() (string, error) {
	dir := os.Getenv("XDG_STATE_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(dir, "ksesh", "sidebar.json"), nil
}

// LoadState reads the state; a missing or unreadable file is the default.
func LoadState(path string) State {
	var st State
	raw, err := os.ReadFile(path)
	if err != nil {
		return st
	}
	_ = json.Unmarshal(raw, &st)
	return st
}

// SaveState writes the state atomically: other sidebars and kitty's watcher
// read it at any time.
func SaveState(path string, st State) error {
	raw, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".sidebar-*.json")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(append(raw, '\n'))
	if err := errors.Join(werr, tmp.Close()); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// UpdateState changes the state in place, keeping what it doesn't touch.
func UpdateState(path string, change func(*State)) (State, error) {
	st := LoadState(path)
	change(&st)
	return st, SaveState(path, st)
}
