package sidebar

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/xheisenbugx/ksesh/ui"
)

// act is something the sidebar does: what a key, a menu item or a button
// asks for. Keys are bound to acts by a keyMap, so the same list can run in
// the sidebar, where letters are keys, and as a full window picker, where
// typing filters and the keys are ctrl and alt ones.
type act int

const (
	actNone act = iota
	actDown
	actUp
	actTop
	actBottom
	actNextSection
	actPrevSection
	actGo
	// actOpen opens or closes a shelf (space), and on a thread opens its
	// actions, as actMenu does: space is the sidebar's leader key.
	actOpen
	actBack
	actFilter
	actNth
	actNarrow
	actWiden
	actGroup
	actScope
	actPin
	actSettle
	actSnooze
	actWake
	actPinUp
	actPinDown
	actUndo
	actApprove
	actDeny
	actReply
	actReview
	actPR
	actNew
	actNewWorktree
	actRename
	actClose
	actArchive
	actHide
	actHelp
	actMenu
	// The picker's own.
	actQuit
	actPager
	actTogglePreview
	actPreviewDown
	actPreviewUp
	// actSeen marks a finished or failed agent seen: idle, off the
	// attention list, without going to it.
	actSeen
	actCount
)

// keyMap binds keys to acts. An act without a binding can't be done by key.
type keyMap map[act]key.Binding

// match is the act a key press asks for.
func (k keyMap) match(msg tea.KeyPressMsg) act {
	for a := actNone + 1; a < actCount; a++ {
		if b, ok := k[a]; ok && key.Matches(msg, b) {
			return a
		}
	}
	return actNone
}

// label is how the keys of an act are shown, "" for none.
func (k keyMap) label(a act) string {
	b, ok := k[a]
	if !ok {
		return ""
	}
	return b.Help().Key
}

// says is the binding of an act described as desc, for a hint.
func (k keyMap) says(a act, desc string) key.Binding {
	b, ok := k[a]
	if !ok {
		return key.NewBinding(key.WithDisabled())
	}
	return ui.Says(b, desc)
}

// sidebarKeys are the sidebar's: single letters, vi style.
func sidebarKeys() keyMap {
	return keyMap{
		actDown:        ui.Bind("j", "down", "j", "down", "ctrl+n"),
		actUp:          ui.Bind("k", "up", "k", "up", "ctrl+p"),
		actTop:         ui.Bind("g", "first", "g", "home"),
		actBottom:      ui.Bind("G", "last", "G", "end"),
		actNextSection: ui.Bind("tab", "next section", "tab"),
		actPrevSection: ui.Bind("shift+tab", "previous section", "shift+tab"),
		actGo:          ui.Bind("↵", "go", "enter", "l", "right"),
		actOpen:        ui.Bind("space", "actions", "space"),
		actBack:        ui.Bind("esc", "back", "esc", "h", "left"),
		actFilter:      ui.Bind("/", "filter", "/"),
		actNth:         ui.Bind("1-9", "go to the nth thread", "1", "2", "3", "4", "5", "6", "7", "8", "9"),
		actNarrow:      ui.Bind("<", "narrower", "<"),
		actWiden:       ui.Bind(">", "wider", ">"),
		actGroup:       ui.Bind("o", "group by session", "o"),
		actScope:       ui.Bind("f", "this session, all", "f"),
		actPin:         ui.Bind("p", "pin", "p"),
		actSettle:      ui.Bind("s", "settle", "s"),
		actSnooze:      ui.Bind("z", "snooze", "z"),
		actWake:        ui.Bind("w", "wake", "w"),
		actPinUp:       ui.Bind("K", "move up", "K", "shift+up"),
		actPinDown:     ui.Bind("J", "move down", "J", "shift+down"),
		actUndo:        ui.Bind("u", "undo", "u", "ctrl+z"),
		actApprove:     ui.Bind("a", "approve", "a", "y"),
		actDeny:        ui.Bind("x", "deny", "x"),
		actReply:       ui.Bind("r", "reply", "r"),
		actReview:      ui.Bind("d", "review", "d"),
		actPR:          ui.Bind("P", "pr", "P"),
		actNew:         ui.Bind("n", "new thread", "n"),
		actNewWorktree: ui.Bind("N", "new worktree", "N"),
		actRename:      ui.Bind("R", "rename", "R"),
		actClose:       ui.Bind("X", "close", "X"),
		actArchive:     ui.Bind("A", "archive", "A"),
		actHide:        ui.Bind("q", "hide", "q"),
		actHelp:        ui.Bind("?", "keys", "?"),
		actMenu:        ui.Bind("m", "menu", "m", "menu"),
	}
}

// pickerKeys are the agent picker's, where typing filters: the arrows to
// move, and ctrl and alt for the rest, those of contrib/ksesh-agents-fzf
// where it had them.
func pickerKeys() keyMap {
	return keyMap{
		actDown:          ui.Bind("↓", "down", "down", "ctrl+n", "ctrl+j"),
		actUp:            ui.Bind("↑", "up", "up", "ctrl+p", "ctrl+k"),
		actTop:           ui.Bind("home", "first", "home"),
		actBottom:        ui.Bind("end", "last", "end"),
		actNextSection:   ui.Bind("tab", "next section", "tab"),
		actPrevSection:   ui.Bind("shift+tab", "previous section", "shift+tab"),
		actGo:            ui.Bind("↵", "go", "enter"),
		actBack:          ui.Bind("esc", "quit", "esc"),
		actQuit:          ui.Bind("ctrl+c", "quit", "ctrl+c"),
		actGroup:         ui.Bind("alt+o", "group by session", "alt+o"),
		actScope:         ui.Bind("alt+f", "this session, all", "alt+f"),
		actPin:           ui.Bind("alt+p", "pin", "alt+p"),
		actSettle:        ui.Bind("alt+s", "settle", "alt+s"),
		actSnooze:        ui.Bind("alt+z", "snooze", "alt+z"),
		actWake:          ui.Bind("alt+e", "wake", "alt+e"),
		actPinUp:         ui.Bind("alt+k", "move up", "alt+k", "shift+up"),
		actPinDown:       ui.Bind("alt+j", "move down", "alt+j", "shift+down"),
		actUndo:          ui.Bind("ctrl+z", "undo", "ctrl+z"),
		actApprove:       ui.Bind("^y", "approve", "ctrl+y"),
		actDeny:          ui.Bind("^x", "deny", "ctrl+x"),
		actReply:         ui.Bind("^o", "reply", "ctrl+o"),
		actReview:        ui.Bind("^g", "diff", "ctrl+g"),
		actPager:         ui.Bind("alt+d", "review full screen", "alt+d"),
		actPR:            ui.Bind("alt+b", "pr", "alt+b"),
		actNew:           ui.Bind("alt+n", "new thread", "alt+n"),
		actNewWorktree:   ui.Bind("alt+t", "new worktree", "alt+t"),
		actRename:        ui.Bind("alt+r", "rename", "alt+r"),
		actClose:         ui.Bind("alt+w", "close", "alt+w"),
		actArchive:       ui.Bind("alt+a", "archive", "alt+a"),
		actHelp:          ui.Bind("f1", "keys", "f1"),
		actMenu:          ui.Bind("alt+space", "actions", "alt+space", "alt+m", "menu"),
		actTogglePreview: ui.Bind("^/", "preview", "ctrl+/", "ctrl+_"),
		actPreviewDown:   ui.Bind("^d", "scroll preview", "ctrl+d", "pgdown"),
		actPreviewUp:     ui.Bind("^u", "scroll preview up", "ctrl+u", "pgup"),
		actSeen:          ui.Bind("^s", "seen", "ctrl+s"),
	}
}

// helpEntry is a line of the help screen: one or more acts, their keys
// shown together.
type helpEntry struct {
	acts []act
	desc string
}

// helpSections is the help screen, in the keys of the keyMap, leaving out
// what it doesn't bind.
func (m Model) helpSections() []ui.HelpSection {
	e := func(desc string, acts ...act) helpEntry { return helpEntry{acts, desc} }
	groups := []struct {
		title   string
		entries []helpEntry
	}{
		{"Threads", []helpEntry{
			e("go to, or resume, a thread", actGo),
			e("back to your window", actBack),
			e("move", actDown, actUp, actTop, actBottom),
			e("next section", actNextSection),
			e("go to the nth thread", actNth),
			e("new thread, worktree", actNew, actNewWorktree),
		}},
		{"Triage", []helpEntry{
			e("pin, unpin", actPin),
			e("move a pin up, down", actPinUp, actPinDown),
			e("settle, restore", actSettle),
			e("snooze, wake", actSnooze, actWake),
			e("undo", actUndo),
			e("actions, by letter; or right click", actOpen, actMenu),
		}},
		{"The agent", []helpEntry{
			e("approve", actApprove),
			e("mark seen: back to idle", actSeen),
			e("deny, interrupt", actDeny),
			e("reply", actReply),
			e("review its changes", actReview),
			e("the review, full screen", actPager),
			e("open its pull request", actPR),
			e("rename the thread", actRename),
			e("close its window, forget", actClose),
			e("archive its worktree", actArchive),
		}},
		{m.chrome(), []helpEntry{
			e("filter; esc clears it", actFilter),
			e("show or hide the preview", actTogglePreview),
			e("scroll the preview", actPreviewDown, actPreviewUp),
			e("group by session", actGroup),
			e("this session, all", actScope),
			e("narrower, wider", actNarrow, actWiden),
			e("hide everywhere", actHide),
			e("quit", actQuit),
		}},
	}
	if m.opts.Picker {
		groups[0].entries[1] = e("clear the filter, quit", actBack)
	}
	var sections []ui.HelpSection
	for _, g := range groups {
		s := ui.HelpSection{Title: g.title}
		for _, en := range g.entries {
			var labels []string
			for _, a := range en.acts {
				if l := m.keys.label(a); l != "" {
					labels = append(labels, l)
				}
			}
			if len(labels) == 0 {
				continue
			}
			label := strings.Join(labels, " ")
			s.Keys = append(s.Keys, ui.Bind(label, en.desc, label))
		}
		sections = append(sections, s)
	}
	return sections
}

// chrome names what the last help section is about.
func (m Model) chrome() string {
	if m.opts.Picker {
		return "Picker"
	}
	return "Sidebar"
}
