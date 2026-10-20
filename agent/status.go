// Package agent tracks the coding agents (Claude Code, Codex, …) running in
// kitty windows, so ksesh can tell which of them needs you.
//
// An agent reports its state through user variables on its own kitty window,
// set by `ksesh agent hook` from the agent's hooks:
//
//	ksesh_agent      the agent's name ("claude")
//	ksesh_status     working | waiting | done | error | idle
//	ksesh_status_at  unix seconds of the last change
//	ksesh_msg        why it is waiting, what it finished with, or the error
//	ksesh_prompt     the prompt it is working on
//
// User variables live and die with the window, show up in `kitten @ ls`, and
// reach kitty's watchers and tab bar (contrib/ksesh_agents.py), so nothing has
// to keep state anywhere else.
package agent

import (
	"github.com/xheisenbugx/ksesh/ansi"
)

type Status string

const (
	Working Status = "working"
	Waiting Status = "waiting"
	Done    Status = "done"
	Error   Status = "error"
	Idle    Status = "idle"
)

// Statuses is every status, most urgent first.
var Statuses = []Status{Waiting, Error, Done, Working, Idle}

// Parse reads a status from a user variable or the command line.
func Parse(s string) (Status, bool) {
	for _, status := range Statuses {
		if string(status) == s {
			return status, true
		}
	}
	return "", false
}

// Priority ranks statuses by how much they need you: waiting blocks the agent
// on you, an error stopped it, done hands it back, and working or idle don't
// need you at all. Unknown statuses rank lowest.
func (s Status) Priority() int {
	switch s {
	case Waiting:
		return 5
	case Error:
		return 4
	case Done:
		return 3
	case Working:
		return 2
	case Idle:
		return 1
	}
	return 0
}

// NeedsAttention reports whether the agent is waiting on you to look at it.
func (s Status) NeedsAttention() bool {
	return s == Waiting || s == Error || s == Done
}

// MostUrgent is the status a session or tab shows for all of its agents.
func MostUrgent(a, b Status) Status {
	if b.Priority() > a.Priority() {
		return b
	}
	return a
}

// Glyph is the nerd font icon and ANSI color a status is drawn with. The icons
// are single-cell Font Awesome glyphs, so they line up with the source icons
// they stand in for, and are the same ones contrib/ksesh_agents.py draws in the
// tab bar.
type Glyph struct {
	Icon      string
	ColorCode int
}

var glyphs = map[Status]Glyph{
	Waiting: {Icon: "", ColorCode: ansi.Yellow},      // bell
	Error:   {Icon: "", ColorCode: ansi.Red},         // warning
	Done:    {Icon: "", ColorCode: ansi.Green},       // check
	Working: {Icon: "", ColorCode: ansi.Blue},        // spinner
	Idle:    {Icon: "", ColorCode: ansi.BrightBlack}, // circle
}

// GlyphFor returns the glyph for a status, and false for no or an unknown one.
func GlyphFor(s Status) (Glyph, bool) {
	g, ok := glyphs[s]
	return g, ok
}

// Icons lists every status icon, for stripping them off a picker line.
func Icons() []string {
	icons := make([]string, 0, len(glyphs))
	for _, status := range Statuses {
		icons = append(icons, glyphs[status].Icon)
	}
	return icons
}
