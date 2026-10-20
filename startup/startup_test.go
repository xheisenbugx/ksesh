package startup

import (
	"testing"

	"github.com/xheisenbugx/ksesh/home"
	"github.com/xheisenbugx/ksesh/kitty"
	"github.com/xheisenbugx/ksesh/lister"
	"github.com/xheisenbugx/ksesh/model"
	"github.com/xheisenbugx/ksesh/replacer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExec(t *testing.T) {
	t.Run("opens configured windows as background tabs and types the startup command into the first window", func(t *testing.T) {
		mLister := lister.NewMockLister(t)
		mKitty := kitty.NewMockKitty(t)
		mHome := home.NewMockHome(t)
		mReplacer := replacer.NewMockReplacer(t)
		config := model.Config{
			WindowConfigs: []model.WindowConfig{
				{Name: "agent", StartupScript: "codex"},
				{Name: "server"},
			},
		}
		s := NewStartup(config, mLister, mKitty, mHome, mReplacer)
		session := model.SeshSession{Name: "api", Path: "~/c/api", WindowNames: []string{"agent", "server"}}

		mHome.EXPECT().ExpandPath("~/c/api").Return("/home/dev/c/api", nil)
		mKitty.EXPECT().NewWindowInSession(model.KittyTabOpts{
			Name: "agent", StartDir: "/home/dev/c/api", TargetSession: "api", Background: true,
		}).Return("window:8", nil)
		mKitty.EXPECT().SendKeys("window:8", "codex").Return("", nil)
		mKitty.EXPECT().NewWindowInSession(model.KittyTabOpts{
			Name: "server", StartDir: "/home/dev/c/api", TargetSession: "api", Background: true,
		}).Return("window:9", nil)
		mLister.EXPECT().FindConfigSession("api").Return(model.SeshSession{StartupCommand: "nvim {}"}, true)
		mReplacer.EXPECT().Replace("nvim {}", map[string]string{"{}": "~/c/api"}).Return("nvim ~/c/api")
		mKitty.EXPECT().SendKeys("window:7", "nvim ~/c/api").Return("", nil)

		msg, err := s.Exec(session, "window:7")

		require.NoError(t, err)
		assert.Equal(t, "executing startup command: nvim ~/c/api", msg)
	})

	t.Run("rejects a window that isn't configured", func(t *testing.T) {
		s := NewStartup(model.Config{}, lister.NewMockLister(t), kitty.NewMockKitty(t), home.NewMockHome(t), replacer.NewMockReplacer(t))

		_, err := s.Exec(model.SeshSession{Name: "api", WindowNames: []string{"nope"}}, "window:7")

		assert.ErrorContains(t, err, "window nope is not defined in config")
	})
}
