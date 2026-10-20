package connector

import (
	"testing"

	"github.com/xheisenbugx/ksesh/dir"
	"github.com/xheisenbugx/ksesh/home"
	"github.com/xheisenbugx/ksesh/kitty"
	"github.com/xheisenbugx/ksesh/lister"
	"github.com/xheisenbugx/ksesh/model"
	"github.com/xheisenbugx/ksesh/namer"
	"github.com/xheisenbugx/ksesh/startup"
	"github.com/xheisenbugx/ksesh/zoxide"
	"github.com/stretchr/testify/assert"
	mock "github.com/stretchr/testify/mock"
)

func TestConfigStrategy(t *testing.T) {
	mockDir := new(dir.MockDir)
	mockHome := new(home.MockHome)
	mockLister := new(lister.MockLister)
	mockNamer := new(namer.MockNamer)
	mockStartup := new(startup.MockStartup)
	mockKitty := new(kitty.MockKitty)
	mockZoxide := new(zoxide.MockZoxide)

	c := &RealConnector{
		model.Config{},
		mockDir,
		mockHome,
		mockLister,
		mockNamer,
		mockStartup,
		mockKitty,
		mockZoxide,
		nil,
	}
	mockKitty.On("AttachSession", mock.Anything).Return("attaching", nil)
	mockZoxide.On("Add", mock.Anything).Return(nil)

	t.Run("should create and attach to config session", func(t *testing.T) {
		mockKitty.On("IsAttached").Return(false)
		mockLister.On("FindConfigSession", "kitty config").Return(model.SeshSession{
			Name: "kitty config",
			Path: "/Users/joshmedeski/c/dotfiles/.config/kitty",
		}, true)
		connection, err := configStrategy(c, "kitty config")
		assert.Nil(t, err)
		assert.Equal(t, "kitty config", connection.Session.Name)
	})
}
