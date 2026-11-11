package ui

import (
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Keys are bound with bubbles/key: a key.Binding is the keys that do
// something and the help that names them, so the status bar's hints, the help
// screen and the menus all say the keys the program actually takes. A TUI
// that runs in two places (the sidebar, and the same list as a full window
// picker, where typing filters) binds the same actions to different keys.

// Bind is a binding of keys, shown as label (the first key when empty) and
// described as desc.
func Bind(label, desc string, keys ...string) key.Binding {
	if label == "" && len(keys) > 0 {
		label = keys[0]
	}
	return key.NewBinding(key.WithKeys(keys...), key.WithHelp(label, desc))
}

// Says is b described as desc, for a hint whose words depend on what is
// selected: pin or unpin.
func Says(b key.Binding, desc string) key.Binding {
	b.SetHelp(b.Help().Key, desc)
	return b
}

// HelpModel is a bubbles/help model drawn in the theme: accented keys, muted
// descriptions, a dim dot between them.
func (t Theme) HelpModel() help.Model {
	h := help.New()
	h.ShortSeparator = " · "
	key := lipgloss.NewStyle().Foreground(t.Colors.Accent).Bold(true)
	desc := lipgloss.NewStyle().Foreground(t.Colors.Muted)
	sep := lipgloss.NewStyle().Foreground(t.Colors.Subtle)
	h.Styles = help.Styles{
		Ellipsis:       sep,
		ShortKey:       key,
		ShortDesc:      desc,
		ShortSeparator: sep,
		FullKey:        key,
		FullDesc:       desc,
		FullSeparator:  sep,
	}
	return h
}

// Hints is the status bar's line of keys: as many of bindings as fit in
// width, in order, always keeping the last one (the key to the help, so the
// rest can be found).
func (t Theme) Hints(bindings []key.Binding, width int) string {
	var shown []key.Binding
	for _, b := range bindings {
		if b.Enabled() {
			shown = append(shown, b)
		}
	}
	h := t.HelpModel()
	for len(shown) > 1 {
		if line := " " + h.ShortHelpView(shown); ansi.StringWidth(line) <= width {
			return line
		}
		shown = append(shown[:len(shown)-2:len(shown)-2], shown[len(shown)-1])
	}
	return " " + h.ShortHelpView(shown)
}

// HelpSection is a titled group of keys on a help screen.
type HelpSection struct {
	Title string
	Keys  []key.Binding
}

// HelpList is a help screen as one column, for the narrow sidebar:
//
//	THREADS
//	 ↵ l      go to, or resume, a thread
//	 j k g G  move
func (t Theme) HelpList(sections []HelpSection) []string {
	col := 9
	for _, s := range sections {
		for _, b := range s.Keys {
			col = max(col, ansi.StringWidth(b.Help().Key)+2)
		}
	}
	var lines []string
	for i, s := range sections {
		if i > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, " "+t.Head(strings.ToUpper(s.Title)))
		for _, b := range s.Keys {
			if !b.Enabled() {
				continue
			}
			k := b.Help().Key
			lines = append(lines, "  "+t.Key(k)+strings.Repeat(" ", max(col-ansi.StringWidth(k), 1))+b.Help().Desc)
		}
	}
	return lines
}

// HelpColumns is a help screen laid out in as many columns as fit in width,
// for the full window pickers. Each section stays whole in its column.
func (t Theme) HelpColumns(sections []HelpSection, width int) []string {
	blocks := make([][]string, len(sections))
	colWidth := 0
	for i, s := range sections {
		blocks[i] = t.HelpList([]HelpSection{s})
		for _, l := range blocks[i] {
			colWidth = max(colWidth, ansi.StringWidth(l))
		}
	}
	colWidth += 3
	n := max(min(width/max(colWidth, 1), len(blocks)), 1)
	total := 0
	for _, b := range blocks {
		total += len(b) + 1
	}
	// Fill each column to about an nth of the lines, in order.
	cols := make([][]string, n)
	c := 0
	for _, b := range blocks {
		if c < n-1 && len(cols[c]) > 0 && len(cols[c])+len(b) > (total+n-1)/n {
			c++
		}
		if len(cols[c]) > 0 {
			cols[c] = append(cols[c], "")
		}
		cols[c] = append(cols[c], b...)
	}
	height := 0
	for _, col := range cols {
		height = max(height, len(col))
	}
	lines := make([]string, height)
	for i := range lines {
		var b strings.Builder
		for _, col := range cols {
			cell := ""
			if i < len(col) {
				cell = col[i]
			}
			b.WriteString(Paint(cell, "", colWidth))
		}
		lines[i] = strings.TrimRight(b.String(), " ")
	}
	return lines
}
