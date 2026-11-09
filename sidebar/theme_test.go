package sidebar

import (
	"image/color"
	"regexp"
	"strconv"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"
)

var sgrColor = regexp.MustCompile(`\[(?:38|48);2;(\d+);(\d+);(\d+)m`)

// colorOf is the color an SGR sequence of the theme sets.
func colorOf(t *testing.T, sgr string) color.Color {
	t.Helper()
	m := sgrColor.FindStringSubmatch(sgr)
	if m == nil {
		t.Fatalf("no color in %q", sgr)
	}
	c := [3]uint8{}
	for i := range c {
		n, _ := strconv.Atoi(m[i+1])
		c[i] = uint8(n)
	}
	return color.RGBA{R: c[0], G: c[1], B: c[2], A: 0xff}
}

func TestTheThemeStaysLegible(t *testing.T) {
	schemes := map[string][2]string{
		"dark":             {"#1c1c22", "#e4e2ea"},
		"white":            {"#ffffff", "#26252b"},
		"one light":        {"#fafafa", "#383a42"},
		"catppuccin latte": {"#eff1f5", "#4c4f69"},
		"solarized light":  {"#fdf6e3", "#657b83"},
	}
	for name, s := range schemes {
		t.Run(name, func(t *testing.T) {
			bg, fg := lipgloss.Color(s[0]), lipgloss.Color(s[1])
			th := themeFor(bg, fg)
			own := contrast(fg, bg)
			// Secondary text reads as well as small text should (WCAG AA), or
			// as well as the scheme's own text when that is lower.
			assert.GreaterOrEqual(t, contrast(colorOf(t, th.muted), bg), min(4.5, own))
			assert.GreaterOrEqual(t, contrast(colorOf(t, th.heading), bg), min(6, own))
			assert.GreaterOrEqual(t, contrast(colorOf(t, th.subtle), bg), min(1.8, own))
			// The text stays quieter than the scheme's own.
			assert.Less(t, contrast(colorOf(t, th.muted), bg), own+0.01)
		})
	}
}

func TestADarkThemeKeepsItsMix(t *testing.T) {
	bg, fg := lipgloss.Color("#1c1c22"), lipgloss.Color("#e4e2ea")
	th := themeFor(bg, fg)
	assert.Equal(t, fgSGR(mix(bg, fg, 0.58)), th.muted)
	assert.Equal(t, fgSGR(mix(bg, fg, 0.24)), th.subtle)
	assert.Equal(t, fgSGR(mix(bg, fg, 0.72)), th.heading)
}
