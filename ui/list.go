package ui

import "strings"

// KeepBg puts bg back after every full reset in s, so text styled with
// lipgloss, whose styles end in one, doesn't clear a row's background
// halfway across.
func KeepBg(s, bg string) string {
	if bg == "" {
		return s
	}
	s = strings.ReplaceAll(s, "\033[0m", "\033[m")
	return strings.ReplaceAll(s, "\033[m", "\033[m"+bg)
}

// Row is a line of a list, width wide: the selection bar at its left edge,
// a space, then content; the selected row on the selection background.
//
//	▌ dotfiles
//	  notes
func (t Theme) Row(content string, selected bool, width int) string {
	line := t.Bar(selected) + " " + content
	if !selected {
		return Paint(line, "", width)
	}
	return Paint(KeepBg(line, t.Selected), t.Selected, width)
}

// Tabs is a row of labels with the current one picked out, accented and
// bold, the others muted, a dim dot between them:
//
//	all · kitty · configs
func (t Theme) Tabs(labels []string, current int) string {
	parts := make([]string, len(labels))
	for i, l := range labels {
		if i == current {
			parts[i] = t.Accented(Bold(l))
		} else {
			parts[i] = t.Mute(l)
		}
	}
	return strings.Join(parts, t.Dim(" · "))
}
