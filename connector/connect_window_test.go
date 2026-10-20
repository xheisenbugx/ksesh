package connector

import (
	"testing"

	"github.com/xheisenbugx/ksesh/dir"
	"github.com/xheisenbugx/ksesh/focuser"
	"github.com/xheisenbugx/ksesh/home"
	"github.com/xheisenbugx/ksesh/kitty"
	"github.com/xheisenbugx/ksesh/lister"
	"github.com/xheisenbugx/ksesh/model"
	"github.com/xheisenbugx/ksesh/zoxide"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// windowConnector wires the mocks every window connect test needs, with the
// target session already attached so the tests only have to describe the
// window behaviour they care about.
func windowConnector(t *testing.T) (*RealConnector, *kitty.MockKitty, *lister.MockLister, *dir.MockDir, *home.MockHome) {
	mKitty := kitty.NewMockKitty(t)
	mLister := lister.NewMockLister(t)
	mDir := dir.NewMockDir(t)
	mHome := home.NewMockHome(t)

	c := NewConnector(
		model.Config{}, mDir, mHome, mLister, nil, nil, mKitty, nil, nil,
	).(*RealConnector)
	// Inside kitty unless a test says otherwise, so focusing a session
	// doesn't also bring kitty to the front.
	mKitty.EXPECT().IsAttached().Return(true).Maybe()

	return c, mKitty, mLister, mDir, mHome
}

func attachedTo(mLister *lister.MockLister, name string, path string) {
	mLister.EXPECT().GetAttachedKittySession().Return(model.SeshSession{Name: name, Path: path}, true)
}

// notADirectory keeps a plain window name from being read as a path.
func notADirectory(mHome *home.MockHome, mDir *dir.MockDir, name string) {
	mHome.EXPECT().ExpandPath(name).Return(name, nil)
	mDir.EXPECT().Dir(name).Return(false, "")
}

func TestConnectWindow(t *testing.T) {
	t.Run("sends the command into a window that already exists", func(t *testing.T) {
		c, mKitty, mLister, mDir, mHome := windowConnector(t)
		attachedTo(mLister, "second-brain", "/home/dev/brain")
		notADirectory(mHome, mDir, "notes")
		mKitty.EXPECT().ListWindows("second-brain").Return([]*model.KittyTab{
			{Name: "shell", Index: 1, ID: 1},
			{Name: "notes", Index: 4, ID: 4},
		}, nil)
		mKitty.EXPECT().SendKeys("tab:4", "claude").Return("", nil)
		mKitty.EXPECT().SelectWindow("tab:4").Return("", nil)
		mKitty.EXPECT().SwitchOrAttach("second-brain", model.ConnectOpts{}).Return("", nil)

		target, err := c.ConnectWindow(model.WindowConnectOpts{Name: "notes", Command: "claude"})

		require.NoError(t, err)
		assert.Equal(t, "tab:4", target)
	})

	t.Run("creates the window when the name doesn't match", func(t *testing.T) {
		c, mKitty, mLister, mDir, mHome := windowConnector(t)
		attachedTo(mLister, "second-brain", "/home/dev/brain")
		notADirectory(mHome, mDir, "notes")
		mKitty.EXPECT().ListWindows("second-brain").Return([]*model.KittyTab{{Name: "shell", Index: 1, ID: 1}}, nil)
		mKitty.EXPECT().NewWindowInSession(model.KittyTabOpts{
			Name:          "notes",
			StartDir:      "/home/dev/brain",
			TargetSession: "second-brain",
			Command:       "claude",
		}).Return("window:2", nil)
		mKitty.EXPECT().SwitchOrAttach("second-brain", model.ConnectOpts{}).Return("", nil)

		target, err := c.ConnectWindow(model.WindowConnectOpts{Name: "notes", Command: "claude"})

		require.NoError(t, err)
		assert.Equal(t, "window:2", target)
	})

	t.Run("always creates when no name is given", func(t *testing.T) {
		c, mKitty, mLister, _, _ := windowConnector(t)
		attachedTo(mLister, "second-brain", "/home/dev/brain")
		mKitty.EXPECT().NewWindowInSession(model.KittyTabOpts{
			StartDir:      "/home/dev/brain",
			TargetSession: "second-brain",
		}).Return("window:2", nil)
		mKitty.EXPECT().SwitchOrAttach("second-brain", model.ConnectOpts{}).Return("", nil)

		target, err := c.ConnectWindow(model.WindowConnectOpts{})

		require.NoError(t, err)
		assert.Equal(t, "window:2", target)
	})

	t.Run("--new creates even when the name matches, without listing windows", func(t *testing.T) {
		c, mKitty, mLister, mDir, mHome := windowConnector(t)
		attachedTo(mLister, "second-brain", "/home/dev/brain")
		notADirectory(mHome, mDir, "claude")
		mKitty.EXPECT().NewWindowInSession(model.KittyTabOpts{
			Name:          "claude",
			StartDir:      "/home/dev/brain",
			TargetSession: "second-brain",
			Command:       "claude",
			Background:    true,
		}).Return("window:5", nil)

		target, err := c.ConnectWindow(model.WindowConnectOpts{
			Name: "claude", Command: "claude", New: true, Background: true,
		})

		require.NoError(t, err)
		assert.Equal(t, "window:5", target)
	})

	t.Run("reuse picks the lowest-indexed window when names are shared", func(t *testing.T) {
		c, mKitty, mLister, mDir, mHome := windowConnector(t)
		attachedTo(mLister, "second-brain", "/home/dev/brain")
		notADirectory(mHome, mDir, "claude")
		mKitty.EXPECT().ListWindows("second-brain").Return([]*model.KittyTab{
			{Name: "claude", Index: 2, ID: 2},
			{Name: "claude", Index: 5, ID: 5},
		}, nil)
		mKitty.EXPECT().SendKeys("tab:2", "another prompt").Return("", nil)

		target, err := c.ConnectWindow(model.WindowConnectOpts{
			Name: "claude", Command: "another prompt", Background: true,
		})

		require.NoError(t, err)
		assert.Equal(t, "tab:2", target)
	})

	t.Run("background leaves the client where it is", func(t *testing.T) {
		c, mKitty, mLister, mDir, mHome := windowConnector(t)
		attachedTo(mLister, "second-brain", "/home/dev/brain")
		notADirectory(mHome, mDir, "notes")
		mKitty.EXPECT().ListWindows("second-brain").Return(nil, nil)
		mKitty.EXPECT().NewWindowInSession(model.KittyTabOpts{
			Name:          "notes",
			StartDir:      "/home/dev/brain",
			TargetSession: "second-brain",
			Background:    true,
		}).Return("window:2", nil)

		target, err := c.ConnectWindow(model.WindowConnectOpts{Name: "notes", Background: true})

		require.NoError(t, err)
		assert.Equal(t, "window:2", target)
	})

	t.Run("names a directory argument after its basename and roots it there", func(t *testing.T) {
		c, mKitty, mLister, mDir, mHome := windowConnector(t)
		attachedTo(mLister, "second-brain", "/home/dev/brain")
		mHome.EXPECT().ExpandPath("~/c/sesh").Return("/home/dev/c/sesh", nil)
		mDir.EXPECT().Dir("/home/dev/c/sesh").Return(true, "/home/dev/c/sesh")
		mKitty.EXPECT().ListWindows("second-brain").Return(nil, nil)
		mKitty.EXPECT().NewWindowInSession(model.KittyTabOpts{
			Name:          "sesh",
			StartDir:      "/home/dev/c/sesh",
			TargetSession: "second-brain",
		}).Return("window:2", nil)
		mKitty.EXPECT().SwitchOrAttach("second-brain", model.ConnectOpts{}).Return("", nil)

		target, err := c.ConnectWindow(model.WindowConnectOpts{Name: "~/c/sesh"})

		require.NoError(t, err)
		assert.Equal(t, "window:2", target)
	})

	t.Run("reuses the window a directory argument created on an earlier run", func(t *testing.T) {
		c, mKitty, mLister, mDir, mHome := windowConnector(t)
		attachedTo(mLister, "second-brain", "/home/dev/brain")
		mHome.EXPECT().ExpandPath("~/c/sesh").Return("/home/dev/c/sesh", nil)
		mDir.EXPECT().Dir("/home/dev/c/sesh").Return(true, "/home/dev/c/sesh")
		mKitty.EXPECT().ListWindows("second-brain").Return([]*model.KittyTab{{Name: "sesh", Index: 3, ID: 3}}, nil)
		mKitty.EXPECT().SelectWindow("tab:3").Return("", nil)
		mKitty.EXPECT().SwitchOrAttach("second-brain", model.ConnectOpts{}).Return("", nil)

		target, err := c.ConnectWindow(model.WindowConnectOpts{Name: "~/c/sesh"})

		require.NoError(t, err)
		assert.Equal(t, "tab:3", target)
	})

	t.Run("brings kitty to the front when triggered from outside kitty", func(t *testing.T) {
		mKitty := kitty.NewMockKitty(t)
		mLister := lister.NewMockLister(t)
		c := NewConnector(model.Config{}, nil, nil, mLister, nil, nil, mKitty, nil, nil).(*RealConnector)
		mLister.EXPECT().FindKittySession("second-brain").Return(model.SeshSession{
			Src: "kitty", Name: "second-brain", Path: "/home/dev/brain",
		}, true)
		mKitty.EXPECT().IsAttached().Return(false)
		mKitty.EXPECT().NewWindowInSession(model.KittyTabOpts{
			StartDir:      "/home/dev/brain",
			TargetSession: "second-brain",
			Command:       "claude",
		}).Return("window:2", nil)
		mKitty.EXPECT().SwitchOrAttach("second-brain", model.ConnectOpts{}).Return("", nil)

		mZoxide := zoxide.NewMockZoxide(t)
		mZoxide.EXPECT().Add("/home/dev/brain").Return(nil)
		c.zoxide = mZoxide
		mFocuser := focuser.NewMockFocuser(t)
		mFocuser.EXPECT().Activate("kitty").Return(true, nil)
		c.focuser = mFocuser

		target, err := c.ConnectWindow(model.WindowConnectOpts{
			Session: "second-brain", Command: "claude", Switch: true,
		})

		require.NoError(t, err)
		assert.Equal(t, "window:2", target)
	})

	t.Run("errors when no session is given and kitty isn't attached", func(t *testing.T) {
		c, _, mLister, _, _ := windowConnector(t)
		mLister.EXPECT().GetAttachedKittySession().Return(model.SeshSession{}, false)

		_, err := c.ConnectWindow(model.WindowConnectOpts{Name: "notes"})

		assert.ErrorContains(t, err, "use --target")
	})
}
