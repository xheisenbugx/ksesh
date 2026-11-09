package sidebar

import (
	"fmt"
	"image/color"
	"math"
	"strings"

	"charm.land/lipgloss/v2"
)

// theme is the sidebar's palette, worked out from the terminal's own
// background and foreground so it suits any color scheme: text is mixed
// between the two rather than drawn faint, which some terminals wash out,
// and the accents are Charm's, as in Crush.
//
// Every field is an SGR sequence; the foreground ones are undone with 39 and
// the background ones are left for the caller, so the colors drawn on a row
// never clear its background.
type theme struct {
	muted   string // secondary text: sessions, times, counts
	subtle  string // rules and the title's stripes
	accent  string // the selection bar, keys, the dot on the thread you're on
	heading string // section names

	selected string // the selected row's background
	hover    string // the background under the mouse
	surface  string // a popup's background
	button   string // the background of the button under the mouse

	// gradient runs across the title.
	from, to color.Color
	bg       color.Color
}

var (
	charple = lipgloss.Color("#6B50FF")
	dolly   = lipgloss.Color("#FF60FF")
)

// themeFor builds the palette for a background and foreground; either may be
// nil until the terminal says, then a dark theme is assumed.
func themeFor(bg, fgc color.Color) theme {
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
	return theme{
		muted:    fgSGR(legible(bg, fgc, 0.58, 4.5)),
		subtle:   fgSGR(legible(bg, fgc, 0.24, 1.8)),
		heading:  fgSGR(legible(bg, fgc, 0.72, 6)),
		accent:   fgSGR(accent),
		selected: bgSGR(mix(bg, accent, selectBy)),
		hover:    bgSGR(mix(bg, fgc, hoverBy)),
		surface:  bgSGR(mix(bg, fgc, hoverBy*0.8)),
		button:   bgSGR(mix(bg, accent, selectBy*1.6)),
		from:     accent,
		to:       to,
		bg:       bg,
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

func fgSGR(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("\033[38;2;%d;%d;%dm", r>>8, g>>8, b>>8)
}

func bgSGR(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("\033[48;2;%d;%d;%dm", r>>8, g>>8, b>>8)
}

func (t theme) mute(s string) string     { return t.muted + s + "\033[39m" }
func (t theme) dim(s string) string      { return t.subtle + s + "\033[39m" }
func (t theme) accented(s string) string { return t.accent + s + "\033[39m" }

// head is a section's name: upper case, in the heading color.
func (t theme) head(s string) string { return t.heading + bold(s) + "\033[39m" }

// gradient colors s cell by cell along the title's gradient: bold up to
// fadeFrom, then faded toward the background.
func (t theme) gradient(s string, fadeFrom int) string {
	cells := []rune(s)
	if len(cells) == 0 {
		return ""
	}
	stops := lipgloss.Blend1D(max(len(cells), 2), t.from, t.to)
	var b strings.Builder
	b.WriteString("\033[1m")
	for i, c := range cells {
		col := stops[i]
		if i == fadeFrom {
			b.WriteString("\033[22m")
		}
		if i >= fadeFrom {
			col = mix(col, t.bg, 0.5)
		}
		b.WriteString(fgSGR(col))
		b.WriteRune(c)
	}
	b.WriteString("\033[22;39m")
	return b.String()
}
