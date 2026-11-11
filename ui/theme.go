// Package ui is what ksesh's TUIs share: the palette, worked out from the
// terminal's own colors, and the pieces drawn with it (the masthead, section
// rules, the key hints and help, popup cards), so the sidebar and the pickers
// look like one program.
package ui

import (
	"fmt"
	"image/color"
	"math"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/exp/charmtone"
)

// Theme is the palette, worked out from the terminal's own
// background and foreground so it suits any color scheme: text is mixed
// between the two rather than drawn faint, which some terminals wash out,
// and the accents are Charm's, as in Crush.
//
// Every field is an SGR sequence; the foreground ones are undone with 39 and
// the background ones are left for the caller, so the colors drawn on a row
// never clear its background.
type Theme struct {
	Muted   string // secondary text: sessions, times, counts
	Subtle  string // rules and the title's stripes
	Accent  string // the selection bar, keys, the dot on the thread you're on
	Heading string // section names

	Selected string // the selected row's background
	Hover    string // the background under the mouse
	Surface  string // a popup's background
	Button   string // the background of the button under the mouse

	// From and To are the ends of the gradient across the masthead.
	From, To color.Color
	Bg       color.Color

	// Colors are the foreground colors above, for lipgloss styles.
	Colors Colors
}

// Colors are a theme's foreground colors.
type Colors struct {
	Muted, Subtle, Accent, Heading color.Color
}

// The accents are Charm's, from charmtone, as in Crush.
var (
	charple color.Color = charmtone.Charple
	dolly   color.Color = charmtone.Dolly
)

// ThemeFor builds the palette for a background and foreground; either may be
// nil until the terminal says, then a dark theme is assumed.
func ThemeFor(bg, fgc color.Color) Theme {
	dark := bg == nil || isDark(bg)
	if bg == nil {
		bg = lipgloss.Color("#1c1c22")
	}
	if fgc == nil {
		fgc = lipgloss.Color("#e4e2ea")
		if !dark {
			fgc = lipgloss.Color("#26252b")
		}
	}
	accent, to := lipgloss.Lighten(charple, 0.18), dolly
	if !dark {
		accent, to = charple, lipgloss.Darken(dolly, 0.15)
	}
	selectBy, hoverBy := 0.22, 0.07
	if !dark {
		selectBy, hoverBy = 0.14, 0.06
	}
	colors := Colors{
		Muted:   legible(bg, fgc, 0.58, 4.5),
		Subtle:  legible(bg, fgc, 0.24, 1.8),
		Heading: legible(bg, fgc, 0.72, 6),
		Accent:  accent,
	}
	return Theme{
		Colors:   colors,
		Muted:    fgSGR(colors.Muted),
		Subtle:   fgSGR(colors.Subtle),
		Heading:  fgSGR(colors.Heading),
		Accent:   fgSGR(accent),
		Selected: bgSGR(mix(bg, accent, selectBy)),
		Hover:    bgSGR(mix(bg, fgc, hoverBy)),
		Surface:  bgSGR(mix(bg, fgc, hoverBy*0.8)),
		Button:   bgSGR(mix(bg, accent, selectBy*1.6)),
		From:     accent,
		To:       to,
		Bg:       bg,
	}
}

// legible is fg mixed t of the way from bg, or further toward fg if that
// isn't enough to read it against bg: a contrast of at least want, or as much
// as fg itself has. Mixed text fades faster on light backgrounds, and on
// low-contrast schemes like Solarized, than on dark ones.
func legible(bg, fg color.Color, t, want float64) color.Color {
	want = min(want, contrast(fg, bg))
	c := mix(bg, fg, t)
	for t < 1 && contrast(c, bg) < want {
		t = min(t+0.02, 1)
		c = mix(bg, fg, t)
	}
	return c
}

// contrast is the WCAG contrast ratio of two colors, from 1 to 21.
func contrast(a, b color.Color) float64 {
	x, y := luminance(a), luminance(b)
	if x < y {
		x, y = y, x
	}
	return (x + 0.05) / (y + 0.05)
}

// luminance is a color's WCAG relative luminance.
func luminance(c color.Color) float64 {
	r, g, b, _ := c.RGBA()
	lin := func(v uint32) float64 {
		x := float64(v>>8) / 255
		if x <= 0.03928 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b)
}

func isDark(c color.Color) bool {
	r, g, b, _ := c.RGBA()
	// Rec. 601 luma, on 16-bit channels.
	return (299*r+587*g+114*b)/1000 < 0x8000
}

// mix is a and b blended, t of the way to b.
func mix(a, b color.Color, t float64) color.Color {
	ar, ag, ab, _ := a.RGBA()
	br, bg, bb, _ := b.RGBA()
	at := func(x, y uint32) uint8 {
		return uint8((float64(x>>8)*(1-t) + float64(y>>8)*t) + 0.5)
	}
	return color.RGBA{R: at(ar, br), G: at(ag, bg), B: at(ab, bb), A: 0xff}
}

// FgSGR and BgSGR are the SGR sequences that set c as the foreground or the
// background.
func FgSGR(c color.Color) string { return fgSGR(c) }
func BgSGR(c color.Color) string { return bgSGR(c) }

// Mix is a and b blended, t of the way to b.
func Mix(a, b color.Color, t float64) color.Color { return mix(a, b, t) }

// IsDark reports whether a background is dark.
func IsDark(c color.Color) bool { return isDark(c) }

func fgSGR(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("\033[38;2;%d;%d;%dm", r>>8, g>>8, b>>8)
}

func bgSGR(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("\033[48;2;%d;%d;%dm", r>>8, g>>8, b>>8)
}

// Mute, Dim and Accented color s as secondary text, as a rule, or with the
// accent; Head as a section's name, bold. Each undoes only the foreground, so
// a row's background survives them.
func (t Theme) Mute(s string) string     { return t.Muted + s + "\033[39m" }
func (t Theme) Dim(s string) string      { return t.Subtle + s + "\033[39m" }
func (t Theme) Accented(s string) string { return t.Accent + s + "\033[39m" }
func (t Theme) Head(s string) string     { return t.Heading + Bold(s) + "\033[39m" }

// Key is a key as the hints and the help show it.
func (t Theme) Key(k string) string { return t.Accented(Bold(k)) }

// Gradient colors s cell by cell along the title's gradient: bold up to
// fadeFrom, then faded toward the background.
func (t Theme) Gradient(s string, fadeFrom int) string {
	cells := []rune(s)
	if len(cells) == 0 {
		return ""
	}
	stops := lipgloss.Blend1D(max(len(cells), 2), t.From, t.To)
	var b strings.Builder
	b.WriteString("\033[1m")
	for i, c := range cells {
		col := stops[i]
		if i == fadeFrom {
			b.WriteString("\033[22m")
		}
		if i >= fadeFrom {
			col = mix(col, t.Bg, 0.5)
		}
		b.WriteString(fgSGR(col))
		b.WriteRune(c)
	}
	b.WriteString("\033[22;39m")
	return b.String()
}
