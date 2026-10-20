package lister

import (
	"testing"

	"github.com/xheisenbugx/ksesh/home"
	"github.com/xheisenbugx/ksesh/kitty"
	"github.com/xheisenbugx/ksesh/model"
	"github.com/xheisenbugx/ksesh/sessionfile"
	"github.com/xheisenbugx/ksesh/zoxide"
	"github.com/stretchr/testify/assert"
)

func TestHideDuplicates(t *testing.T) {
	tests := []struct {
		name               string
		kittySessions      []*model.KittySession
		zoxideResults      []*model.ZoxideResult
		configSessions     []model.SessionConfig
		sessionfileConfigs []*model.SessionFileConfig
		homeShortenHome    map[string]string
		expectedNames      []string
	}{
		{
			name: "no duplicates",
			kittySessions: []*model.KittySession{
				{Name: "session1", Path: "/path/to/session1"},
			},
			zoxideResults: []*model.ZoxideResult{
				{Path: "/path/to/session2", Score: 1.0},
			},
			homeShortenHome: map[string]string{
				"/path/to/session2": "session2",
			},
			expectedNames: []string{"session1", "session2"},
		},
		{
			name: "name_collision_kept_when_paths_differ",
			kittySessions: []*model.KittySession{
				{Name: "dev", Path: "/path/to/dev1"},
			},
			zoxideResults: []*model.ZoxideResult{
				{Path: "/path/to/dev2", Score: 1.0},
			},
			homeShortenHome: map[string]string{
				"/path/to/dev2": "dev",
			},
			expectedNames: []string{"dev", "dev"},
		},
		{
			name: "path duplicates only",
			kittySessions: []*model.KittySession{
				{Name: "dev1", Path: "/path/to/dev"},
			},
			zoxideResults: []*model.ZoxideResult{
				{Path: "/path/to/dev", Score: 1.0},
			},
			homeShortenHome: map[string]string{
				"/path/to/dev": "dev2",
			},
			expectedNames: []string{"dev1"},
		},
		{
			name: "sessionfile_and_zoxide_kept_no_rule_between_them",
			sessionfileConfigs: []*model.SessionFileConfig{
				{Name: "dev"},
			},
			zoxideResults: []*model.ZoxideResult{
				{Path: "/path/to/dev", Score: 1.0},
			},
			homeShortenHome: map[string]string{
				"/path/to/dev": "dev",
			},
			expectedNames: []string{"dev", "dev"},
		},
		{
			name: "order preservation",
			kittySessions: []*model.KittySession{
				{Name: "a", Path: "/path/to/a"},
			},
			zoxideResults: []*model.ZoxideResult{
				{Path: "/path/to/b", Score: 1.0},
			},
			configSessions: []model.SessionConfig{
				{Name: "a", Path: "/path/to/c"},
			},
			homeShortenHome: map[string]string{
				"/path/to/b": "b",
			},
			expectedNames: []string{"a", "b"},
		},
		{
			name:               "empty input",
			kittySessions:      []*model.KittySession{},
			zoxideResults:      []*model.ZoxideResult{},
			configSessions:     []model.SessionConfig{},
			sessionfileConfigs: []*model.SessionFileConfig{},
			expectedNames:      []string{},
		},
		{
			name: "single session",
			kittySessions: []*model.KittySession{
				{Name: "single", Path: "/path/to/single"},
			},
			expectedNames: []string{"single"},
		},
		{
			name: "all duplicates",
			kittySessions: []*model.KittySession{
				{Name: "dup", Path: "/path/to/dup"},
			},
			zoxideResults: []*model.ZoxideResult{
				{Path: "/path/to/dup", Score: 1.0},
			},
			configSessions: []model.SessionConfig{
				{Name: "dup", Path: "/path/to/dup"},
			},
			homeShortenHome: map[string]string{
				"/path/to/dup": "dup",
			},
			expectedNames: []string{"dup"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mocks
			mockKitty := new(kitty.MockKitty)
			mockZoxide := new(zoxide.MockZoxide)
			mockHome := new(home.MockHome)
			mockSessionFile := new(sessionfile.MockSessionFile)

			// Set up mocks for all sources that have data (including empty data)
			if tt.kittySessions != nil {
				mockKitty.On("ListSessions").Return(tt.kittySessions, nil)
			}
			if tt.zoxideResults != nil {
				mockZoxide.On("ListResults").Return(tt.zoxideResults, nil)
				for path, shortened := range tt.homeShortenHome {
					mockHome.On("ShortenHome", path).Return(shortened, nil)
				}
			}
			if tt.configSessions != nil {
				for _, session := range tt.configSessions {
					mockHome.On("ExpandPath", session.Path).Return(session.Path, nil)
				}
			}
			if tt.sessionfileConfigs != nil {
				mockSessionFile.On("List").Return(tt.sessionfileConfigs, nil)
			}

			config := model.Config{
				SessionConfigs: tt.configSessions,
			}

			lister := NewLister(config, mockHome, mockKitty, mockZoxide, mockSessionFile, nil, nil)

			// Call the actual List function with HideDuplicates
			result, err := lister.List(ListOptions{
				Kitty:          tt.kittySessions != nil,
				Zoxide:         tt.zoxideResults != nil,
				Config:         tt.configSessions != nil,
				SessionFile:    tt.sessionfileConfigs != nil,
				HideDuplicates: true,
			})

			assert.NoError(t, err)
			assert.Equal(t, len(tt.expectedNames), len(result.OrderedIndex))

			for i, expectedName := range tt.expectedNames {
				if i < len(result.OrderedIndex) {
					session := result.Directory[result.OrderedIndex[i]]
					assert.Equal(t, expectedName, session.Name)
				}
			}

			// Verify all mocks were called
			mockKitty.AssertExpectations(t)
			mockZoxide.AssertExpectations(t)
			mockSessionFile.AssertExpectations(t)
		})
	}
}

func TestHideAttachedBeforeHideDuplicates(t *testing.T) {
	mockKitty := new(kitty.MockKitty)
	mockZoxide := new(zoxide.MockZoxide)
	mockHome := new(home.MockHome)
	mockSessionFile := new(sessionfile.MockSessionFile)

	mockKitty.On("ListSessions").Return([]*model.KittySession{
		{Name: "project", Path: "/p", Attached: 1},
	}, nil)
	mockHome.On("ExpandPath", "/p").Return("/p", nil)

	config := model.Config{
		SessionConfigs: []model.SessionConfig{{Name: "project", Path: "/p"}},
	}
	l := NewLister(config, mockHome, mockKitty, mockZoxide, mockSessionFile, nil, nil)

	result, err := l.List(ListOptions{
		Kitty:          true,
		Config:         true,
		HideAttached:   true,
		HideDuplicates: true,
	})
	assert.NoError(t, err)
	assert.Equal(t, 1, len(result.OrderedIndex))
	kept := result.Directory[result.OrderedIndex[0]]
	assert.Equal(t, "config", kept.Src)
	assert.Equal(t, "project", kept.Name)
}

func TestBlacklistedFlag(t *testing.T) {
	tests := []struct {
		name          string
		blacklist     []string
		kittySessions []*model.KittySession
		blacklisted   bool
		expectedNames []string
	}{
		{
			name:      "flag on, some sessions match patterns",
			blacklist: []string{"^scratch$", "^temp"},
			kittySessions: []*model.KittySession{
				{Name: "scratch", Path: "/path/to/scratch"},
				{Name: "work", Path: "/path/to/work"},
				{Name: "temporary", Path: "/path/to/temporary"},
			},
			blacklisted:   true,
			expectedNames: []string{"scratch", "temporary"},
		},
		{
			name:      "flag on, no sessions match patterns",
			blacklist: []string{"^nomatch$"},
			kittySessions: []*model.KittySession{
				{Name: "alpha", Path: "/path/to/alpha"},
				{Name: "beta", Path: "/path/to/beta"},
			},
			blacklisted:   true,
			expectedNames: []string{},
		},
		{
			name:      "flag on, empty blacklist in config",
			blacklist: []string{},
			kittySessions: []*model.KittySession{
				{Name: "alpha", Path: "/path/to/alpha"},
			},
			blacklisted:   true,
			expectedNames: []string{},
		},
		{
			name:      "flag off preserves existing hide-blacklisted behavior",
			blacklist: []string{"^scratch$"},
			kittySessions: []*model.KittySession{
				{Name: "scratch", Path: "/path/to/scratch"},
				{Name: "work", Path: "/path/to/work"},
			},
			blacklisted:   false,
			expectedNames: []string{"work"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockKitty := new(kitty.MockKitty)
			mockZoxide := new(zoxide.MockZoxide)
			mockHome := new(home.MockHome)
			mockSessionFile := new(sessionfile.MockSessionFile)

			mockKitty.On("ListSessions").Return(tt.kittySessions, nil)

			config := model.Config{Blacklist: tt.blacklist}
			lister := NewLister(config, mockHome, mockKitty, mockZoxide, mockSessionFile, nil, nil)

			result, err := lister.List(ListOptions{
				Kitty:       true,
				Blacklisted: tt.blacklisted,
			})

			assert.NoError(t, err)
			assert.Equal(t, len(tt.expectedNames), len(result.OrderedIndex))
			for i, expectedName := range tt.expectedNames {
				if i < len(result.OrderedIndex) {
					session := result.Directory[result.OrderedIndex[i]]
					assert.Equal(t, expectedName, session.Name)
				}
			}

			mockKitty.AssertExpectations(t)
		})
	}
}

func TestList_ShowWindows(t *testing.T) {
	newLister := func(showWindows bool, mockKitty *kitty.MockKitty) Lister {
		config := model.Config{}
		config.TUI.ShowWindows = showWindows
		return NewLister(config, new(home.MockHome), mockKitty, new(zoxide.MockZoxide), new(sessionfile.MockSessionFile), nil, nil)
	}

	t.Run("attaches window names when enabled", func(t *testing.T) {
		mockKitty := new(kitty.MockKitty)
		mockKitty.On("ListSessions").Return([]*model.KittySession{
			{Name: "sesh", Path: "/p"},
		}, nil)
		mockKitty.EXPECT().ListAllWindowNames("").Return(map[string][]string{
			"sesh": {"editor", "server"},
		}, nil).Once()

		result, err := newLister(true, mockKitty).List(ListOptions{Kitty: true})
		assert.NoError(t, err)
		session := result.Directory[result.OrderedIndex[0]]
		assert.Equal(t, []string{"editor", "server"}, session.WindowNames)
		// One kitty call for all sessions, not one per session.
		mockKitty.AssertExpectations(t)
	})

	t.Run("does not call kitty when disabled", func(t *testing.T) {
		mockKitty := new(kitty.MockKitty)
		mockKitty.On("ListSessions").Return([]*model.KittySession{
			{Name: "sesh", Path: "/p"},
		}, nil)

		result, err := newLister(false, mockKitty).List(ListOptions{Kitty: true})
		assert.NoError(t, err)
		session := result.Directory[result.OrderedIndex[0]]
		assert.Nil(t, session.WindowNames)
		mockKitty.AssertNotCalled(t, "ListAllWindowNames")
	})

	t.Run("does not call kitty when kitty is not a listed source", func(t *testing.T) {
		mockKitty := new(kitty.MockKitty)

		_, err := newLister(true, mockKitty).List(ListOptions{Config: true})
		assert.NoError(t, err)
		mockKitty.AssertNotCalled(t, "ListAllWindowNames")
	})
}

func TestMergedSortOrderGroup(t *testing.T) {
	mockKitty := new(kitty.MockKitty)
	mockZoxide := new(zoxide.MockZoxide)
	mockHome := new(home.MockHome)
	mockSessionFile := new(sessionfile.MockSessionFile)

	mockKitty.On("ListSessions").Return([]*model.KittySession{
		{Name: "live", Path: "/live"},
	}, nil)
	mockZoxide.On("ListResults").Return([]*model.ZoxideResult{
		{Path: "/hot", Score: 90},
		{Path: "/notes", Score: 20},
		{Path: "/cold", Score: 1},
	}, nil)
	for path, short := range map[string]string{"/hot": "hot", "/notes": "notes", "/cold": "cold"} {
		mockHome.On("ShortenHome", path).Return(short, nil)
	}
	mockHome.On("ExpandPath", "/notes").Return("/notes", nil)
	mockHome.On("ExpandPath", "/never").Return("/never", nil)

	config := model.Config{
		// "notes" is a config session zoxide also knows: it should sort by the
		// score zoxide has for its path, not by coming from the config source.
		SessionConfigs: []model.SessionConfig{
			{Name: "notes-cfg", Path: "/notes"},
			{Name: "never-cfg", Path: "/never"},
		},
		SortOrder: model.SortOrder{"kitty", []string{"config", "zoxide"}},
	}
	l := NewLister(config, mockHome, mockKitty, mockZoxide, mockSessionFile, nil, nil)

	result, err := l.List(ListOptions{Kitty: true, Config: true, Zoxide: true})
	assert.NoError(t, err)

	names := make([]string, 0, len(result.OrderedIndex))
	groups := make([]int, 0, len(result.OrderedIndex))
	for _, key := range result.OrderedIndex {
		names = append(names, result.Directory[key].Name)
		groups = append(groups, result.Directory[key].Group)
	}

	// kitty stays pinned in its own group; config and zoxide interleave by
	// score, and the config session zoxide has never seen trails the group.
	assert.Equal(t, []string{"live", "hot", "notes-cfg", "notes", "cold", "never-cfg"}, names)
	assert.Equal(t, []int{0, 1, 1, 1, 1, 1}, groups)
}

func TestListOptionsSortOrderOverridesConfig(t *testing.T) {
	mockKitty := new(kitty.MockKitty)
	mockZoxide := new(zoxide.MockZoxide)
	mockHome := new(home.MockHome)
	mockSessionFile := new(sessionfile.MockSessionFile)

	mockKitty.On("ListSessions").Return([]*model.KittySession{{Name: "live", Path: "/live"}}, nil)
	mockZoxide.On("ListResults").Return([]*model.ZoxideResult{
		{Path: "/hot", Score: 90},
		{Path: "/cold", Score: 1},
	}, nil)
	mockHome.On("ShortenHome", "/hot").Return("hot", nil)
	mockHome.On("ShortenHome", "/cold").Return("cold", nil)
	mockHome.On("ExpandPath", "/hot").Return("/hot", nil)

	config := model.Config{
		SessionConfigs: []model.SessionConfig{{Name: "notes-cfg", Path: "/hot"}},
		SortOrder:      model.SortOrder{"zoxide", "config", "kitty"},
	}
	l := NewLister(config, mockHome, mockKitty, mockZoxide, mockSessionFile, nil, nil)

	result, err := l.List(ListOptions{
		Kitty: true, Config: true, Zoxide: true,
		SortOrder: model.SortOrder{"kitty", []any{"config", "zoxide"}},
	})
	assert.NoError(t, err)

	names := make([]string, 0, len(result.OrderedIndex))
	groups := make([]int, 0, len(result.OrderedIndex))
	for _, key := range result.OrderedIndex {
		names = append(names, result.Directory[key].Name)
		groups = append(groups, result.Directory[key].Group)
	}
	assert.Equal(t, []string{"live", "notes-cfg", "hot", "cold"}, names)
	assert.Equal(t, []int{0, 1, 1, 1}, groups)
}

func TestFlatSortOrderKeepsSourceBlocks(t *testing.T) {
	mockKitty := new(kitty.MockKitty)
	mockZoxide := new(zoxide.MockZoxide)
	mockHome := new(home.MockHome)
	mockSessionFile := new(sessionfile.MockSessionFile)

	mockZoxide.On("ListResults").Return([]*model.ZoxideResult{
		{Path: "/hot", Score: 90},
	}, nil)
	mockHome.On("ShortenHome", "/hot").Return("hot", nil)
	mockHome.On("ExpandPath", "/never").Return("/never", nil)

	config := model.Config{
		SessionConfigs: []model.SessionConfig{{Name: "never-cfg", Path: "/never"}},
		SortOrder:      model.SortOrder{"config", "zoxide"},
	}
	l := NewLister(config, mockHome, mockKitty, mockZoxide, mockSessionFile, nil, nil)

	result, err := l.List(ListOptions{Config: true, Zoxide: true})
	assert.NoError(t, err)

	names := make([]string, 0, len(result.OrderedIndex))
	groups := make([]int, 0, len(result.OrderedIndex))
	for _, key := range result.OrderedIndex {
		names = append(names, result.Directory[key].Name)
		groups = append(groups, result.Directory[key].Group)
	}
	// Unmerged, the unscored config session still leads its own block.
	assert.Equal(t, []string{"never-cfg", "hot"}, names)
	assert.Equal(t, []int{0, 1}, groups)
}
