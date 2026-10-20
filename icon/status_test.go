package icon

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/xheisenbugx/ksesh/agent"
	"github.com/xheisenbugx/ksesh/model"
)

func TestAgentStatusReplacesTheKittyIcon(t *testing.T) {
	i := NewIcon(model.Config{})
	waiting, _ := agent.GlyphFor(agent.Waiting)

	line := i.AddIconNoColor(model.SeshSession{Src: "kitty", Name: "api", AgentStatus: "waiting"})
	assert.Equal(t, waiting.Icon+" api", line)
	assert.Equal(t, "api", i.RemoveIcon(line), "a picker line with a status icon still connects")

	assert.Equal(t, kittyIcon+" web", i.AddIconNoColor(model.SeshSession{Src: "kitty", Name: "web"}))
	assert.Equal(t, zoxideIcon+" ~/c", i.AddIconNoColor(model.SeshSession{Src: "zoxide", Name: "~/c", AgentStatus: "waiting"}),
		"only live sessions have agents")
}
