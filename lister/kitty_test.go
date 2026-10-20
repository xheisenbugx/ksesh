package lister

import (
	"fmt"
	"log"
	"testing"
	"time"

	"github.com/xheisenbugx/ksesh/home"
	"github.com/xheisenbugx/ksesh/kitty"
	"github.com/xheisenbugx/ksesh/model"
	"github.com/xheisenbugx/ksesh/sessionfile"
	"github.com/xheisenbugx/ksesh/zoxide"
	"github.com/stretchr/testify/assert"
)

func TestListKittySessions(t *testing.T) {
	mockKitty := new(kitty.MockKitty)
	t.Run("should list kitty sessions", func(t *testing.T) {
		const timeFormat = "2006-01-02 15:04:05 -0700 MST"
		createdFA, _ := time.Parse(timeFormat, "2024-04-25 19:02:45 -0500 CDT")
		lastAttachedFA, _ := time.Parse(timeFormat, "2024-04-25 19:30:06 -0500 CDT")
		activityFA, _ := time.Parse(timeFormat, "2024-04-25 19:44:06 -0500 CDT")
		firstAttached := model.KittySession{
			Created:      &createdFA,
			LastAttached: &lastAttachedFA,
			Activity:     &activityFA,
			Path:         "/Users/joshmedeski/c/sesh/main",
			Name:         "sesh/main",
			Alerts:       []string{},
			Attached:     0,
			Windows:      2,
		}

		createdLA, _ := time.Parse(timeFormat, "2024-04-25 19:02:45 -0500 CDT")
		lastAttachedLA, _ := time.Parse(timeFormat, "2024-04-25 19:44:06 -0500 CDT")
		activityLA, _ := time.Parse(timeFormat, "2024-04-25 19:44:06 -0500 CDT")
		lastAttached := model.KittySession{
			Created:      &createdLA,
			LastAttached: &lastAttachedLA,
			Activity:     &activityLA,
			Path:         "/Users/joshmedeski/c/sesh/v2",
			Name:         "sesh/v2",
			Alerts:       []string{},
			Attached:     0,
			Windows:      2,
		}
		mockKitty.On("ListSessions").Return([]*model.KittySession{&firstAttached, &lastAttached}, nil)

		mockConfig := model.Config{}
		mockHome := new(home.MockHome)
		mockZoxide := new(zoxide.MockZoxide)
		mockSessionFile := new(sessionfile.MockSessionFile)
		lister := NewLister(mockConfig, mockHome, mockKitty, mockZoxide, mockSessionFile, nil, nil)

		realLister, ok := lister.(*RealLister)
		if !ok {
			log.Fatal("Cannot convert lister to *RealLister")
		}

		sessions, err := listKitty(realLister)
		assert.Equal(t, "kitty:sesh/main", sessions.OrderedIndex[0])
		assert.Equal(t, "sesh/main", sessions.Directory["kitty:sesh/main"].Name)
		assert.Equal(t, "kitty:sesh/v2", sessions.OrderedIndex[1])
		assert.Equal(t, nil, err)
	})
}

func makeKittySession(name, path string) *model.KittySession {
	return &model.KittySession{
		Name:    name,
		Path:    path,
		Windows: 1,
	}
}

// makeAttachedKittySession is the session the user is in.
func makeAttachedKittySession(name, path string) *model.KittySession {
	s := makeKittySession(name, path)
	s.Attached = 1
	return s
}

func TestGetLastKittySession(t *testing.T) {
	tests := []struct {
		name      string
		sessions  []*model.KittySession
		blacklist []string
		wantName  string
		wantOk    bool
	}{
		{
			name: "no blacklist returns second session",
			sessions: []*model.KittySession{
				makeAttachedKittySession("current", "/tmp/current"),
				makeKittySession("previous", "/tmp/previous"),
				makeKittySession("oldest", "/tmp/oldest"),
			},
			blacklist: nil,
			wantName:  "previous",
			wantOk:    true,
		},
		{
			name: "second session blacklisted skips to next",
			sessions: []*model.KittySession{
				makeAttachedKittySession("current", "/tmp/current"),
				makeKittySession("blocked", "/tmp/blocked"),
				makeKittySession("fallback", "/tmp/fallback"),
			},
			blacklist: []string{"blocked"},
			wantName:  "fallback",
			wantOk:    true,
		},
		{
			name: "all sessions blacklisted returns false",
			sessions: []*model.KittySession{
				makeKittySession("a", "/tmp/a"),
				makeKittySession("b", "/tmp/b"),
			},
			blacklist: []string{"a", "b"},
			wantOk:    false,
		},
		{
			name: "only one non-blacklisted session returns false",
			sessions: []*model.KittySession{
				makeAttachedKittySession("keep", "/tmp/keep"),
				makeKittySession("blocked", "/tmp/blocked"),
			},
			blacklist: []string{"blocked"},
			wantOk:    false,
		},
		{
			// Focus can be in a window that belongs to no session, and then
			// the most recently focused session is the last one.
			name: "no attached session returns the most recent session",
			sessions: []*model.KittySession{
				makeKittySession("recent", "/tmp/recent"),
				makeKittySession("older", "/tmp/older"),
			},
			wantName: "recent",
			wantOk:   true,
		},
		{
			name: "regex blacklist pattern matches",
			sessions: []*model.KittySession{
				makeAttachedKittySession("current", "/tmp/current"),
				makeKittySession("scratch-123", "/tmp/scratch-123"),
				makeKittySession("work", "/tmp/work"),
			},
			blacklist: []string{"^scratch-.*"},
			wantName:  "work",
			wantOk:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockKitty := new(kitty.MockKitty)
			mockKitty.On("ListSessions").Return(tt.sessions, nil)

			config := model.Config{Blacklist: tt.blacklist}
			lister := NewLister(config, new(home.MockHome), mockKitty, new(zoxide.MockZoxide), new(sessionfile.MockSessionFile), nil, nil)

			session, ok := lister.GetLastKittySession()
			assert.Equal(t, tt.wantOk, ok)
			if tt.wantOk {
				assert.Equal(t, tt.wantName, session.Name)
			}
		})
	}
}

func TestListKittySessionsError(t *testing.T) {
	mockKitty := new(kitty.MockKitty)
	t.Run("should return error when unable to list kitty sessions", func(t *testing.T) {
		mockKitty.On("ListSessions").Return(nil, fmt.Errorf("some error"))

		mockConfig := model.Config{}
		mockHome := new(home.MockHome)
		mockZoxide := new(zoxide.MockZoxide)
		mockSessionFile := new(sessionfile.MockSessionFile)
		lister := NewLister(mockConfig, mockHome, mockKitty, mockZoxide, mockSessionFile, nil, nil)

		realLister, ok := lister.(*RealLister)
		if !ok {
			log.Fatal("Cannot convert lister to *RealLister")
		}

		sessions, err := listKitty(realLister)
		assert.Equal(t, model.SeshSessions{}, sessions)
		assert.NotNil(t, err)
		assert.Contains(t, err.Error(), "couldn't list kitty sessions")
	})
}

func TestListKittyInheritsAliasFromConfig(t *testing.T) {
	mockKitty := new(kitty.MockKitty)
	mockKitty.On("ListSessions").Return([]*model.KittySession{
		makeKittySession("wallpaper", "/home/user/wallpaper"),
		makeKittySession("dotfiles", "/home/user/dotfiles"),
		makeKittySession("notes", "/home/user/notes"),
	}, nil)

	config := model.Config{
		SessionConfigs: []model.SessionConfig{
			{Name: "wallpaper", Alias: "wp"},
			{Name: "dotfiles", Alias: "DOT"},
		},
	}
	lister := NewLister(config, new(home.MockHome), mockKitty, new(zoxide.MockZoxide), new(sessionfile.MockSessionFile), nil, nil)

	realLister, ok := lister.(*RealLister)
	if !ok {
		log.Fatal("Cannot convert lister to *RealLister")
	}

	sessions, err := listKitty(realLister)
	assert.Nil(t, err)
	assert.Equal(t, "wp", sessions.Directory["kitty:wallpaper"].Alias)
	assert.Equal(t, "DOT", sessions.Directory["kitty:dotfiles"].Alias)
	assert.Equal(t, "", sessions.Directory["kitty:notes"].Alias)
}

func TestAttachWindowNames(t *testing.T) {
	t.Run("fills in window names for kitty sessions only", func(t *testing.T) {
		sessions := model.SeshSessions{
			OrderedIndex: []string{"kitty:sesh", "config:sesh", "kitty:unknown"},
			Directory: model.SeshSessionMap{
				"kitty:sesh":    {Src: "kitty", Name: "sesh"},
				"config:sesh":   {Src: "config", Name: "sesh", WindowNames: []string{"configured"}},
				"kitty:unknown": {Src: "kitty", Name: "unknown"},
			},
		}
		attachWindowNames(sessions, map[string][]string{
			"sesh": {"editor", "server"},
		})

		assert.Equal(t, []string{"editor", "server"}, sessions.Directory["kitty:sesh"].WindowNames)
		assert.Equal(t, []string{"configured"}, sessions.Directory["config:sesh"].WindowNames,
			"config sessions keep their configured window names")
		assert.Nil(t, sessions.Directory["kitty:unknown"].WindowNames)
	})

	t.Run("no-op when there are no window names", func(t *testing.T) {
		sessions := model.SeshSessions{
			OrderedIndex: []string{"kitty:sesh"},
			Directory:    model.SeshSessionMap{"kitty:sesh": {Src: "kitty", Name: "sesh"}},
		}
		attachWindowNames(sessions, nil)
		assert.Nil(t, sessions.Directory["kitty:sesh"].WindowNames)
	})
}

func TestKittyTabNames(t *testing.T) {
	t.Run("returns nil when kitty fails", func(t *testing.T) {
		mockKitty := new(kitty.MockKitty)
		mockKitty.EXPECT().ListAllWindowNames("").Return(nil, assert.AnError)
		l := &RealLister{kitty: mockKitty}
		assert.Nil(t, kittyWindowNames(l))
	})

	t.Run("returns the window names", func(t *testing.T) {
		mockKitty := new(kitty.MockKitty)
		format := "#{?#{pane_title},#{pane_title},#{window_name}}"
		mockKitty.EXPECT().ListAllWindowNames(format).Return(map[string][]string{"sesh": {"editor"}}, nil)
		l := &RealLister{
			config: model.Config{TUI: model.TUIConfig{WindowNameFormat: format}},
			kitty:  mockKitty,
		}
		assert.Equal(t, map[string][]string{"sesh": {"editor"}}, kittyWindowNames(l))
	})
}
