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

func TestListSessionFileConfigs(t *testing.T) {
	t.Run("should list sessionfile configs", func(t *testing.T) {
		mockConfig := model.Config{}
		mockHome := new(home.MockHome)
		mockZoxide := new(zoxide.MockZoxide)
		mockKitty := new(kitty.MockKitty)
		mockSessionFile := new(sessionfile.MockSessionFile)
		mockSessionFile.On("List").Return([]*model.SessionFileConfig{
			{Name: "sesh"},
			{Name: "dotfiles"},
		}, nil)

		lister := NewLister(mockConfig, mockHome, mockKitty, mockZoxide, mockSessionFile, nil, nil)

		realLister, ok := lister.(*RealLister)
		if !ok {
			log.Fatal("Cannot convert lister to *RealLister")
		}
		sessions, err := listSessionFile(realLister)
		assert.Equal(t, "sessionfile:sesh", sessions.OrderedIndex[0])
		assert.Equal(t, "sessionfile:dotfiles", sessions.OrderedIndex[1])
		assert.Nil(t, err)
	})
}
