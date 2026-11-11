package ui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// SGR pieces that only undo what they set, so a row's background survives
// the colors drawn on top of it.

// Fg colors s with one of the terminal's 16 colors (an ansi package code).
func Fg(code int, s string) string { return fmt.Sprintf("\033[%dm%s\033[39m", code, s) }

// Bold makes s bold.
func Bold(s string) string { return "\033[1m" + s + "\033[22m" }

// Paint pads a line to the full width, cut to it, on a background if one is
// given.
func Paint(content string, bg string, width int) string {
	content = ansi.Truncate(content, width, "…")
	pad := strings.Repeat(" ", max(width-ansi.StringWidth(content), 0))
	if bg == "" {
		return content + pad
	}
	return bg + content + pad + "\033[49m"
}

// Spread puts left and right at the two ends of a line width wide, cutting
// left when they don't both fit.
func Spread(left, right string, width int) string {
	if right == "" {
		return Paint(left, "", width)
	}
	right += " "
	lw, rw := ansi.StringWidth(left), ansi.StringWidth(right)
	if lw+1+rw > width {
		left = ansi.Truncate(left, max(width-rw-1, 0), "…")
		lw = ansi.StringWidth(left)
	}
	return left + strings.Repeat(" ", max(width-lw-rw, 1)) + right
}

// Center centers s on a line width wide.
func Center(s string, width int) string {
	pad := max((width-ansi.StringWidth(s))/2, 0)
	return Paint(strings.Repeat(" ", pad)+s, "", width)
}

// Rule is a line across width, inset by a cell at each end: the line over the
// shelves, and over the status bar.
func (t Theme) Rule(width int) string {
	return " " + t.Dim(strings.Repeat("─", max(width-2, 0))) + " "
}

// LabelRule is a section's title, its rule running to its count:
//
//	PINNED ───────────────────── 2
//
// label is drawn as it comes; a count below 0 leaves the count out.
func (t Theme) LabelRule(label string, count, width int, bg string) string {
	tail := " "
	if count >= 0 {
		tail = " " + t.Mute(strconv.Itoa(count)) + " "
	}
	rule := max(width-ansi.StringWidth(label)-ansi.StringWidth(tail), 0)
	return Paint(label+t.Dim(strings.Repeat("─", rule))+tail, bg, width)
}

// Section is LabelRule for a section named name, with an icon before it.
func (t Theme) Section(icon, name string, count, width int, bg string) string {
	label := " "
	if icon != "" {
		label += t.Mute(icon) + " "
	}
	return t.LabelRule(label+t.Head(name)+" ", count, width, bg)
}

// Masthead is the top line of a ksesh TUI, in the manner of Crush's: the name
// on a gradient that runs out into stripes, up to right, drawn as it comes at
// the right end.
//
//	Threads ╱╱╱╱╱╱╱╱╱╱╱╱╱╱╱╱╱╱╱╱╱╱  +
func (t Theme) Masthead(name string, width int, right string) string {
	name = " " + name + " "
	stripes := strings.Repeat("╱", max(width-ansi.StringWidth(name)-ansi.StringWidth(right), 0))
	return Paint(t.Gradient(name+stripes, len([]rune(name)))+right, "", width)
}

// Bar is a row's left edge: an accent bar when it is selected.
func (t Theme) Bar(selected bool) string {
	if selected {
		return t.Accented("▌")
	}
	return " "
}
