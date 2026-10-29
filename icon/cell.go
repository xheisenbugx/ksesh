package icon

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/xheisenbugx/ksesh/model"
)

// SourceGlyph returns the nerd font icon and color for a session source. The
// icon carries a trailing space, so it is the whole icon cell for the
// single-width glyphs it comes from; Cell pads it when a wider icon is in play.
func SourceGlyph(src string) (string, color.Color) {
	if g, ok := Glyphs[src]; ok {
		return g.Icon + " ", ansiColor(g.ColorCode)
	}
	return "? ", lipgloss.ANSIColor(8)
}

// ansiColor turns an SGR foreground code (31, 94, …) into the palette color.
func ansiColor(sgr int) color.Color {
	switch {
	case sgr >= 90 && sgr <= 97:
		return lipgloss.ANSIColor(sgr - 82)
	case sgr >= 30 && sgr <= 37:
		return lipgloss.ANSIColor(sgr - 30)
	}
	return lipgloss.ANSIColor(sgr)
}

// ColumnWidth is the display width an icon column needs: one cell for the
// single-width source glyphs, more when a configured icon is wider (emoji are
// two cells). It is measured across the whole config rather than the visible
// rows so the column keeps one width.
func ColumnWidth(config model.Config) int {
	width := 1
	for _, session := range config.SessionConfigs {
		width = max(width, Width(session.Icon))
	}
	for _, wildcard := range config.WildcardConfigs {
		width = max(width, Width(wildcard.Icon))
	}
	return width
}

// Width is the display width of a configured icon. Trailing spaces are the
// user's own width override and are not counted.
func Width(icn string) int {
	return lipgloss.Width(strings.TrimRight(icn, " "))
}

// Cell renders an icon column cell of width+1 cells: the configured icon, or
// the colored source glyph when there is none, followed by the gap before the
// next column. A configured icon is left unstyled.
func Cell(custom, src string, width int) string {
	if custom == "" {
		glyph, clr := SourceGlyph(src)
		cell := lipgloss.NewStyle().Foreground(clr).Render(glyph)
		return cell + pad(width+1-lipgloss.Width(cell))
	}
	return custom + " " + pad(width+1-lipgloss.Width(strings.TrimRight(custom, " ")+" "))
}

// StatusCell renders an agent status glyph in the icon column, colored and
// padded like a source glyph.
func StatusCell(glyph string, colorCode int, width int) string {
	cell := lipgloss.NewStyle().Foreground(ansiColor(colorCode)).Render(glyph + " ")
	return cell + pad(width+1-lipgloss.Width(cell))
}

func pad(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat(" ", n)
}
