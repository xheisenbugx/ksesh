package seshcli

import (
	"testing"

	"github.com/xheisenbugx/ksesh/icon"
	"github.com/xheisenbugx/ksesh/kitty"
	"github.com/stretchr/testify/assert"
)

func TestKillSession(t *testing.T) {
	t.Run("kills the session named by a picker line", func(t *testing.T) {
		mockKitty := new(kitty.MockKitty)
		mockIcon := new(icon.MockIcon)
		mockIcon.On("RemoveIcon", "  api").Return("api")
		mockKitty.On("KillSession", "api").Return("", nil)

		deps := &Deps{Icon: mockIcon, Kitty: mockKitty}

		assert.NoError(t, killSession(deps, "  api"))
		mockKitty.AssertExpectations(t)
	})

	t.Run("returns the kitty error", func(t *testing.T) {
		mockKitty := new(kitty.MockKitty)
		mockIcon := new(icon.MockIcon)
		mockIcon.On("RemoveIcon", "gone").Return("gone")
		mockKitty.On("KillSession", "gone").Return("", assert.AnError)

		deps := &Deps{Icon: mockIcon, Kitty: mockKitty}

		assert.ErrorIs(t, killSession(deps, "gone"), assert.AnError)
	})
}
