package connector

import (
	"errors"
	"testing"

	"github.com/xheisenbugx/ksesh/focuser"
	"github.com/xheisenbugx/ksesh/kitty"
	"github.com/xheisenbugx/ksesh/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConnectToSessionFile(t *testing.T) {
	sessionFile := model.SeshSession{
		Src:         "sessionfile",
		Name:        "sys",
		Path:        "/home/dev/.config/kitty/sessions/sys.kitty-session",
		SessionFile: "/home/dev/.config/kitty/sessions/sys.kitty-session",
	}

	t.Run("loads the file with goto_session", func(t *testing.T) {
		mKitty := kitty.NewMockKitty(t)
		mKitty.EXPECT().Reachable().Return(true)
		mKitty.EXPECT().GotoSessionFile(sessionFile.SessionFile).Return("sys", nil)
		mKitty.EXPECT().IsAttached().Return(true)
		c := NewConnector(model.Config{}, nil, nil, nil, nil, nil, mKitty, nil, nil).(*RealConnector)

		msg, err := connectToSessionFile(c, model.Connection{Found: true, New: true, Session: sessionFile}, model.ConnectOpts{})

		require.NoError(t, err)
		assert.Equal(t, "switching to kitty session: sys", msg)
	})

	t.Run("propagates a failure to load the file", func(t *testing.T) {
		mKitty := kitty.NewMockKitty(t)
		mKitty.EXPECT().Reachable().Return(true)
		mKitty.EXPECT().GotoSessionFile(sessionFile.SessionFile).Return("", errors.New("boom"))
		c := NewConnector(model.Config{}, nil, nil, nil, nil, nil, mKitty, nil, nil).(*RealConnector)

		_, err := connectToSessionFile(c, model.Connection{Found: true, New: true, Session: sessionFile}, model.ConnectOpts{})

		assert.ErrorContains(t, err, "boom")
	})

	t.Run("names the session after its [[session]] entry", func(t *testing.T) {
		mKitty := kitty.NewMockKitty(t)
		mKitty.EXPECT().Reachable().Return(true)
		mKitty.EXPECT().GotoSessionFile("/p/work.kitty-session").Return("work", nil)
		mKitty.EXPECT().SetSessionName("work", "Work Project").Return(nil)
		mKitty.EXPECT().IsAttached().Return(true)
		c := NewConnector(model.Config{}, nil, nil, nil, nil, nil, mKitty, nil, nil).(*RealConnector)

		_, err := connectToKitty(c, model.Connection{Found: true, New: true, Session: model.SeshSession{
			Src: "config", Name: "Work Project", Path: "/p", SessionFile: "/p/work.kitty-session",
		}}, model.ConnectOpts{})

		require.NoError(t, err)
	})

	t.Run("starts kitty when none is reachable and brings it forward", func(t *testing.T) {
		mKitty := kitty.NewMockKitty(t)
		mFocuser := focuser.NewMockFocuser(t)
		mKitty.EXPECT().Reachable().Return(false)
		mKitty.EXPECT().StartKitty("").Return(3, nil)
		mKitty.EXPECT().GotoSessionFile(sessionFile.SessionFile).Return("sys", nil)
		mKitty.EXPECT().CloseWindow(3).Return(nil)
		mKitty.EXPECT().IsAttached().Return(false)
		mFocuser.EXPECT().Activate("kitty").Return(true, nil)
		c := NewConnector(model.Config{}, nil, nil, nil, nil, nil, mKitty, nil, mFocuser).(*RealConnector)

		_, err := connectToSessionFile(c, model.Connection{Found: true, New: true, Session: sessionFile}, model.ConnectOpts{})

		require.NoError(t, err)
	})
}
