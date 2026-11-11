package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	ansicolor "github.com/xheisenbugx/ksesh/ansi"
)

// MenuItem is a line of a popup card: an action with its icon and, at the
// right, the key that does it; or a separator between groups.
type MenuItem struct {
	Icon, Label, Right string
	Danger             bool // in red: it closes or removes something
	Sep                bool
}

// CardWidth is how wide a card with this title and these items is drawn,
// borders included.
func CardWidth(title string, items []MenuItem) int {
	inner := ansi.StringWidth(title) + 4
	for _, it := range items {
		if !it.Sep {
			inner = max(inner, 1+ansi.StringWidth(it.Icon)+2+ansi.StringWidth(it.Label)+3+ansi.StringWidth(it.Right)+1)
		}
	}
	return inner + 2
}

// Card draws a popup menu as a card with a rounded border, w wide, the item
// at idx highlighted:
//
//	╭─ Snooze until ───────╮
//	│ 1 In an hour   11:00 │
//	├──────────────────────┤
//	│ c Other time…        │
//	╰──────────────────────╯
func (t Theme) Card(title string, items []MenuItem, idx, w int) []string {
	inner := w - 2
	border := func(s string) string { return t.Surface + t.Accent + s + "\033[39m" }
	top := "─" + strings.Repeat("─", max(inner-1, 0))
	if title != "" {
		tt := ansi.Truncate(" "+title+" ", inner-1, "…")
		top = "─" + t.Heading + Bold(tt) + t.Accent + strings.Repeat("─", max(inner-1-ansi.StringWidth(tt), 0))
	}
	lines := []string{border("╭" + top + "╮")}
	for i, it := range items {
		if it.Sep {
			lines = append(lines, border("├"+t.Subtle+strings.Repeat("─", inner)+t.Accent+"┤"))
			continue
		}
		bg, label, icon := t.Surface, it.Label, t.Mute(it.Icon)
		if it.Danger {
			label = Fg(ansicolor.Red, label)
		}
		if i == idx {
			bg, label, icon = t.Selected, Bold(label), t.Accented(it.Icon)
		}
		left := " " + icon + "  " + label
		right := t.Mute(it.Right) + " "
		gap := max(inner-ansi.StringWidth(left)-ansi.StringWidth(right), 1)
		content := ansi.Truncate(left+strings.Repeat(" ", gap)+right, inner, "…")
		lines = append(lines, border("│")+bg+content+border("│"))
	}
	lines = append(lines, border("╰"+strings.Repeat("─", inner)+"╯"))
	for i := range lines {
		lines[i] = "\033[0m" + lines[i] + "\033[0m"
	}
	return lines
}

// Overlay draws card over the screen's lines, its top left corner at x, y.
func Overlay(lines, card []string, x, y int) []string {
	for i, l := range card {
		row := y + i
		if row < 0 || row >= len(lines) {
			continue
		}
		w := ansi.StringWidth(l)
		base := lines[row]
		if bw := ansi.StringWidth(base); bw < x+w {
			base += strings.Repeat(" ", x+w-bw)
		}
		lines[row] = ansi.Truncate(base, x, "") + l + ansi.TruncateLeft(base, x+w, "")
	}
	return lines
}
