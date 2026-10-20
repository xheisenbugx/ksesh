package lister

import (
	"log"
	"testing"

	"github.com/xheisenbugx/ksesh/home"
	"github.com/xheisenbugx/ksesh/kitty"
	"github.com/xheisenbugx/ksesh/model"
	"github.com/xheisenbugx/ksesh/sessionfile"
	"github.com/xheisenbugx/ksesh/zoxide"
	"github.com/stretchr/testify/assert"
)

func TestListZoxideSessions(t *testing.T) {
	t.Run("should list zoxide sessions", func(t *testing.T) {
		mockConfig := model.Config{}
		mockHome := new(home.MockHome)
		mockZoxide := new(zoxide.MockZoxide)
		mockKitty := new(kitty.MockKitty)
		mockSessionFile := new(sessionfile.MockSessionFile)
		mockHome.On("ShortenHome", "/Users/joshmedeski/.config/sesh").Return("~/.config/sesh", nil)
		mockHome.On("ShortenHome", "/Users/joshmedeski/.config/fish").Return("~/.config/fish", nil)
		mockZoxide.On("ListResults").Return([]*model.ZoxideResult{
			{
				Score: 0.5,
				Path:  "/Users/joshmedeski/.config/sesh",
			},
			{
				Score: 0.3,
				Path:  "/Users/joshmedeski/.config/fish",
			},
		}, nil)

		lister := NewLister(mockConfig, mockHome, mockKitty, mockZoxide, mockSessionFile, nil, nil)

		realLister, ok := lister.(*RealLister)
		if !ok {
			log.Fatal("Cannot convert lister to *RealLister")
		}
		sessions, err := listZoxide(realLister)
		assert.Equal(t, "zoxide:~/.config/sesh", sessions.OrderedIndex[0])
		assert.Equal(t, "zoxide:~/.config/fish", sessions.OrderedIndex[1])
		assert.Nil(t, err)
	})
}
