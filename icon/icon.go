package icon

import (
	"fmt"
	"strings"

	"github.com/xheisenbugx/ksesh/agent"
	"github.com/xheisenbugx/ksesh/ansi"
	"github.com/xheisenbugx/ksesh/model"
)

type Icon interface {
	AddIcon(session model.SeshSession) string
	AddIconNoColor(session model.SeshSession) string
	RemoveIcon(name string) string
}

type RealIcon struct {
	config model.Config
}

func NewIcon(config model.Config) Icon {
	return &RealIcon{config}
}

var (
	zoxideIcon      string = ""
	kittyIcon       string = ""
	configIcon      string = ""
	sessionfileIcon string = ""
	kittyWindowIcon string = ""
)

// Glyph holds the icon character and ANSI color code for a session source.
type Glyph struct {
	Icon      string
	ColorCode int
}

// Glyphs maps session source names to their icon and color.
var Glyphs = map[string]Glyph{
	"kitty":        {Icon: kittyIcon, ColorCode: ansi.Blue},
	"config":       {Icon: configIcon, ColorCode: ansi.BrightBlack},
	"zoxide":       {Icon: zoxideIcon, ColorCode: ansi.Cyan},
	"sessionfile":  {Icon: sessionfileIcon, ColorCode: ansi.Yellow},
	"kitty-window": {Icon: kittyWindowIcon, ColorCode: ansi.Green},
}

func ansiString(code int, s string) string {
	return fmt.Sprintf("\033[%dm%s\033[39m", code, s)
}

// glyphFor is the icon a session is listed with: its agents' status when
// there is one, so a session waiting on you stands out in the list, and its
// source otherwise.
func glyphFor(s model.SeshSession) (Glyph, bool) {
	if s.Src == "kitty" {
		if g, ok := agent.GlyphFor(agent.Status(s.AgentStatus)); ok {
			return Glyph{Icon: g.Icon, ColorCode: g.ColorCode}, true
		}
	}
	g, ok := Glyphs[s.Src]
	return g, ok
}

func (i *RealIcon) AddIcon(s model.SeshSession) string {
	if g, ok := glyphFor(s); ok {
		return fmt.Sprintf("%s %s", ansiString(g.ColorCode, g.Icon), s.Name)
	}
	return s.Name
}

func (i *RealIcon) AddIconNoColor(s model.SeshSession) string {
	if g, ok := glyphFor(s); ok {
		return fmt.Sprintf("%s %s", g.Icon, s.Name)
	}
	return s.Name
}

func (i *RealIcon) RemoveIcon(name string) string {
	for _, status := range agent.Icons() {
		if rest, ok := strings.CutPrefix(name, status+" "); ok {
			return rest
		}
	}
	if strings.HasPrefix(name, kittyIcon) || strings.HasPrefix(name, zoxideIcon) || strings.HasPrefix(name, configIcon) || strings.HasPrefix(name, sessionfileIcon) || strings.HasPrefix(name, kittyWindowIcon) {
		return name[4:]
	}
	return name
}
