package lister

import (
	"fmt"
	"log"
	"os"
	"testing"

	"github.com/xheisenbugx/ksesh/home"
	"github.com/xheisenbugx/ksesh/kitty"
	"github.com/xheisenbugx/ksesh/model"
	"github.com/xheisenbugx/ksesh/sessionfile"
	"github.com/xheisenbugx/ksesh/zoxide"
	"github.com/stretchr/testify/assert"
)

func makeKittyWindow(tabIndex int, tabTitle string, id int, title, command, path string) *model.KittyWindow {
	return &model.KittyWindow{
		TabIndex: tabIndex,
		TabTitle: tabTitle,
		ID:       id,
		Title:    title,
		Command:  command,
		Path:     path,
	}
}

func TestListKittyWindows(t *testing.T) {
	hostname, _ := os.Hostname()

	t.Run("should list kitty windows", func(t *testing.T) {
		mockKitty := new(kitty.MockKitty)
		mockKitty.On("ListKittyWindows").Return([]*model.KittyWindow{
			makeKittyWindow(1, "editor", 0, hostname, "vim", "/home/user/project"),
			makeKittyWindow(1, "editor", 1, hostname, "zsh", "/home/user/project"),
			makeKittyWindow(2, "tests", 2, hostname, "go", "/home/user/project"),
		}, nil)

		lister := NewLister(model.Config{}, new(home.MockHome), mockKitty, new(zoxide.MockZoxide), new(sessionfile.MockSessionFile), nil, nil)
		realLister, ok := lister.(*RealLister)
		if !ok {
			log.Fatal("Cannot convert lister to *RealLister")
		}

		sessions, err := listKittyWindows(realLister)
		assert.Nil(t, err)
		assert.Equal(t, 3, len(sessions.OrderedIndex))
		assert.Equal(t, "kitty-window:0", sessions.OrderedIndex[0])
		assert.Equal(t, "editor/vim", sessions.Directory["kitty-window:0"].Name)
		assert.Equal(t, "kitty-window", sessions.Directory["kitty-window:0"].Src)
		assert.Equal(t, "kitty-window:1", sessions.OrderedIndex[1])
		assert.Equal(t, "editor/zsh", sessions.Directory["kitty-window:1"].Name)
		assert.Equal(t, "kitty-window:2", sessions.OrderedIndex[2])
		assert.Equal(t, "tests/go", sessions.Directory["kitty-window:2"].Name)
	})

	t.Run("should use the window title when explicitly set", func(t *testing.T) {
		mockKitty := new(kitty.MockKitty)
		mockKitty.On("ListKittyWindows").Return([]*model.KittyWindow{
			makeKittyWindow(1, "editor", 0, "my-editor", "vim", "/home/user/project"),
			makeKittyWindow(1, "editor", 1, hostname, "zsh", "/home/user/project"),
		}, nil)

		lister := NewLister(model.Config{}, new(home.MockHome), mockKitty, new(zoxide.MockZoxide), new(sessionfile.MockSessionFile), nil, nil)
		realLister, ok := lister.(*RealLister)
		if !ok {
			log.Fatal("Cannot convert lister to *RealLister")
		}

		sessions, err := listKittyWindows(realLister)
		assert.Nil(t, err)
		assert.Equal(t, "editor/my-editor", sessions.Directory["kitty-window:0"].Name)
		assert.Equal(t, "editor/zsh", sessions.Directory["kitty-window:1"].Name)
	})

	t.Run("should disambiguate duplicate names with suffixes", func(t *testing.T) {
		mockKitty := new(kitty.MockKitty)
		mockKitty.On("ListKittyWindows").Return([]*model.KittyWindow{
			makeKittyWindow(1, "editor", 0, hostname, "zsh", "/home/user/project"),
			makeKittyWindow(1, "editor", 1, hostname, "zsh", "/home/user/project"),
			makeKittyWindow(2, "tests", 2, hostname, "zsh", "/tmp"),
		}, nil)

		lister := NewLister(model.Config{}, new(home.MockHome), mockKitty, new(zoxide.MockZoxide), new(sessionfile.MockSessionFile), nil, nil)
		realLister, ok := lister.(*RealLister)
		if !ok {
			log.Fatal("Cannot convert lister to *RealLister")
		}

		sessions, err := listKittyWindows(realLister)
		assert.Nil(t, err)
		assert.Equal(t, "editor/zsh.0", sessions.Directory["kitty-window:0"].Name)
		assert.Equal(t, "editor/zsh.1", sessions.Directory["kitty-window:1"].Name)
		assert.Equal(t, "tests/zsh", sessions.Directory["kitty-window:2"].Name)
	})

	t.Run("should return empty sessions on error", func(t *testing.T) {
		mockKitty := new(kitty.MockKitty)
		mockKitty.On("ListKittyWindows").Return(nil, fmt.Errorf("some error"))

		lister := NewLister(model.Config{}, new(home.MockHome), mockKitty, new(zoxide.MockZoxide), new(sessionfile.MockSessionFile), nil, nil)
		realLister, ok := lister.(*RealLister)
		if !ok {
			log.Fatal("Cannot convert lister to *RealLister")
		}

		sessions, err := listKittyWindows(realLister)
		assert.Equal(t, model.SeshSessions{}, sessions)
		assert.NotNil(t, err)
		assert.Contains(t, err.Error(), "couldn't list kitty windows")
	})
}
