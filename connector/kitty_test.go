package connector

import (
	"errors"
	"testing"

	"github.com/xheisenbugx/ksesh/focuser"
	"github.com/xheisenbugx/ksesh/kitty"
	"github.com/xheisenbugx/ksesh/lister"
	"github.com/xheisenbugx/ksesh/model"
	"github.com/xheisenbugx/ksesh/startup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKittyStrategy(t *testing.T) {
	mLister := lister.NewMockLister(t)
	c := NewConnector(model.Config{}, nil, nil, mLister, nil, nil, nil, nil, nil).(*RealConnector)

	t.Run("finds a running session", func(t *testing.T) {
		mLister.EXPECT().FindKittySession("dotfiles").Return(model.SeshSession{
			Src: "kitty", Name: "dotfiles", Path: "/home/dev/c/dotfiles",
		}, true).Once()

		connection, err := kittyStrategy(c, "dotfiles")

		require.NoError(t, err)
		assert.True(t, connection.Found)
		assert.False(t, connection.New)
		assert.Equal(t, "dotfiles", connection.Session.Name)
	})

	t.Run("passes on a name that isn't running", func(t *testing.T) {
		mLister.EXPECT().FindKittySession("nope").Return(model.SeshSession{}, false).Once()

		connection, err := kittyStrategy(c, "nope")

		require.NoError(t, err)
		assert.False(t, connection.Found)
	})
}

var newDirSession = model.Connection{
	Found: true, New: true,
	Session: model.SeshSession{Src: "dir", Name: "nutiliti/2345", Path: "/repo/w/2345"},
}

func TestConnectToKitty(t *testing.T) {
	t.Run("creates a session, runs its startup in its window, then switches", func(t *testing.T) {
		mKitty := kitty.NewMockKitty(t)
		mStartup := startup.NewMockStartup(t)
		mKitty.EXPECT().Reachable().Return(true)
		mKitty.EXPECT().NewSession("nutiliti/2345", "/repo/w/2345").Return("window:7", nil)
		mStartup.EXPECT().Exec(newDirSession.Session, "window:7").Return("", nil)
		mKitty.EXPECT().SwitchOrAttach("nutiliti/2345", model.ConnectOpts{}).Return("switching to kitty session: nutiliti/2345", nil)
		mKitty.EXPECT().IsAttached().Return(true)
		c := NewConnector(model.Config{}, nil, nil, nil, nil, mStartup, mKitty, nil, nil).(*RealConnector)

		msg, err := connectToKitty(c, newDirSession, model.ConnectOpts{})

		require.NoError(t, err)
		assert.Equal(t, "switching to kitty session: nutiliti/2345", msg)
	})

	t.Run("--command replaces the startup", func(t *testing.T) {
		mKitty := kitty.NewMockKitty(t)
		mKitty.EXPECT().Reachable().Return(true)
		mKitty.EXPECT().NewSession("nutiliti/2345", "/repo/w/2345").Return("window:7", nil)
		mKitty.EXPECT().SendKeys("window:7", "nvim").Return("", nil)
		mKitty.EXPECT().SwitchOrAttach("nutiliti/2345", model.ConnectOpts{}).Return("", nil)
		mKitty.EXPECT().IsAttached().Return(true)
		c := NewConnector(model.Config{}, nil, nil, nil, nil, nil, mKitty, nil, nil).(*RealConnector)

		_, err := connectToKitty(c, newDirSession, model.ConnectOpts{Command: "nvim"})

		require.NoError(t, err)
	})

	t.Run("switches to a running session without creating anything", func(t *testing.T) {
		mKitty := kitty.NewMockKitty(t)
		mKitty.EXPECT().SwitchOrAttach("dotfiles", model.ConnectOpts{}).Return("", nil)
		mKitty.EXPECT().IsAttached().Return(true)
		c := NewConnector(model.Config{}, nil, nil, nil, nil, nil, mKitty, nil, nil).(*RealConnector)

		_, err := connectToKitty(c, model.Connection{Found: true, Session: model.SeshSession{Src: "kitty", Name: "dotfiles"}}, model.ConnectOpts{})

		require.NoError(t, err)
	})

	t.Run("brings the configured terminal forward from outside kitty", func(t *testing.T) {
		mKitty := kitty.NewMockKitty(t)
		mFocuser := focuser.NewMockFocuser(t)
		mKitty.EXPECT().SwitchOrAttach("dotfiles", model.ConnectOpts{}).Return("", nil)
		mKitty.EXPECT().IsAttached().Return(false)
		mFocuser.EXPECT().Activate("wezterm").Return(true, nil)
		c := NewConnector(model.Config{Terminal: "wezterm"}, nil, nil, nil, nil, nil, mKitty, nil, mFocuser).(*RealConnector)

		_, err := connectToKitty(c, model.Connection{Found: true, Session: model.SeshSession{Src: "kitty", Name: "dotfiles"}}, model.ConnectOpts{Switch: true})

		require.NoError(t, err)
	})

	t.Run("starts kitty when none is reachable and closes its stand-in window", func(t *testing.T) {
		mKitty := kitty.NewMockKitty(t)
		mStartup := startup.NewMockStartup(t)
		mFocuser := focuser.NewMockFocuser(t)
		mKitty.EXPECT().Reachable().Return(false)
		mKitty.EXPECT().StartKitty("/repo/w/2345").Return(1, nil)
		mKitty.EXPECT().NewSession("nutiliti/2345", "/repo/w/2345").Return("window:2", nil)
		mKitty.EXPECT().CloseWindow(1).Return(nil)
		mStartup.EXPECT().Exec(newDirSession.Session, "window:2").Return("", nil)
		mKitty.EXPECT().SwitchOrAttach("nutiliti/2345", model.ConnectOpts{}).Return("", nil)
		mKitty.EXPECT().IsAttached().Return(false)
		mFocuser.EXPECT().Activate("kitty").Return(true, nil)
		c := NewConnector(model.Config{}, nil, nil, nil, nil, mStartup, mKitty, nil, mFocuser).(*RealConnector)

		_, err := connectToKitty(c, newDirSession, model.ConnectOpts{})

		require.NoError(t, err)
	})

	t.Run("reports a session kitty wouldn't create", func(t *testing.T) {
		mKitty := kitty.NewMockKitty(t)
		mKitty.EXPECT().Reachable().Return(true)
		mKitty.EXPECT().NewSession("nutiliti/2345", "/repo/w/2345").Return("", errors.New("remote control is disabled"))
		c := NewConnector(model.Config{}, nil, nil, nil, nil, nil, mKitty, nil, nil).(*RealConnector)

		_, err := connectToKitty(c, newDirSession, model.ConnectOpts{})

		assert.ErrorContains(t, err, `failed to create kitty session "nutiliti/2345"`)
		assert.ErrorContains(t, err, "remote control is disabled")
	})
}
