package lister

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/xheisenbugx/ksesh/model"
)

func TestSortSources(t *testing.T) {
	defaultSources := []string{"kitty", "config", "sessionfile", "zoxide"}
	tests := map[string]struct {
		sortOrder model.SortOrder
		expected  []string
	}{
		"a normal configuration": {
			sortOrder: model.SortOrder{"sessionfile", "zoxide", "config", "kitty"},
			expected:  []string{"sessionfile", "zoxide", "config", "kitty"},
		},
		"empty configuration": {
			sortOrder: model.SortOrder{},
			expected:  []string{"kitty", "config", "sessionfile", "zoxide"},
		},
		"partial configuration": {
			sortOrder: model.SortOrder{"sessionfile"},
			expected:  []string{"sessionfile", "kitty", "config", "zoxide"},
		},
		"superfluous elements": {
			sortOrder: model.SortOrder{"sessionfile", "apple", "zoxide", "banana", "config", "chocolate", "kitty"},
			expected:  []string{"sessionfile", "zoxide", "config", "kitty"},
		},
		"configuration with capitalization": {
			sortOrder: model.SortOrder{"SessionFile", "Zoxide", "conFIg", "KITTY"},
			expected:  []string{"sessionfile", "zoxide", "config", "kitty"},
		},
		"sesh source names": {
			sortOrder: model.SortOrder{"tmuxinator", "zoxide", "tmux", "config"},
			expected:  []string{"sessionfile", "zoxide", "kitty", "config"},
		},
		"configuration with duplicate elements": {
			sortOrder: model.SortOrder{"sessionfile", "zoxide", "sessionfile", "config", "sessionfile", "kitty", "sessionfile", "sessionfile"},
			expected:  []string{"zoxide", "config", "kitty", "sessionfile"},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			actual := sortSources(defaultSources, tt.sortOrder)
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func TestGroupSources(t *testing.T) {
	defaultSources := []string{"kitty", "config", "sessionfile", "zoxide"}
	tests := map[string]struct {
		sortOrder model.SortOrder
		expected  [][]string
	}{
		"no sort order leaves every source on its own": {
			sortOrder: nil,
			expected:  [][]string{{"kitty"}, {"config"}, {"sessionfile"}, {"zoxide"}},
		},
		"a flat sort order leaves every source on its own": {
			sortOrder: model.SortOrder{"zoxide", "kitty", "config", "sessionfile"},
			expected:  [][]string{{"zoxide"}, {"kitty"}, {"config"}, {"sessionfile"}},
		},
		"a nested entry merges its sources into one group": {
			sortOrder: model.SortOrder{"kitty", []string{"config", "zoxide"}, "sessionfile"},
			expected:  [][]string{{"kitty"}, {"config", "zoxide"}, {"sessionfile"}},
		},
		"a nested entry decoded from toml merges too": {
			sortOrder: model.SortOrder{"kitty", []any{"config", "zoxide"}},
			expected:  [][]string{{"kitty"}, {"config", "zoxide"}, {"sessionfile"}},
		},
		"unlisted sources trail as groups of one": {
			sortOrder: model.SortOrder{[]string{"zoxide", "kitty"}},
			expected:  [][]string{{"zoxide", "kitty"}, {"config"}, {"sessionfile"}},
		},
		"a source named twice joins the last group that claims it": {
			sortOrder: model.SortOrder{[]string{"kitty", "zoxide"}, []string{"config", "zoxide"}},
			expected:  [][]string{{"kitty"}, {"config", "zoxide"}, {"sessionfile"}},
		},
		"a group naming only inactive sources is skipped": {
			sortOrder: model.SortOrder{[]string{"apple", "banana"}, "zoxide"},
			expected:  [][]string{{"zoxide"}, {"kitty"}, {"config"}, {"sessionfile"}},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tt.expected, groupSources(defaultSources, tt.sortOrder))
		})
	}
}

func TestSrcs(t *testing.T) {
	tests := []struct {
		name     string
		expected []string
		opts     ListOptions
	}{
		{
			name:     "All options are false",
			opts:     ListOptions{},
			expected: []string{"kitty", "config", "sessionfile", "zoxide"},
		},
		{
			name:     "Only Kitty is true",
			opts:     ListOptions{Kitty: true},
			expected: []string{"kitty"},
		},
		{
			name:     "Only Config is true",
			opts:     ListOptions{Config: true},
			expected: []string{"config"},
		},
		{
			name:     "Only Zoxide is true",
			opts:     ListOptions{Zoxide: true},
			expected: []string{"zoxide"},
		},
		{
			name:     "Kitty and Config are true",
			opts:     ListOptions{Kitty: true, Config: true},
			expected: []string{"kitty", "config"},
		},
		{
			name:     "Kitty and Zoxide are true",
			opts:     ListOptions{Kitty: true, Zoxide: true},
			expected: []string{"kitty", "zoxide"},
		},
		{
			name:     "Config and Zoxide are true",
			opts:     ListOptions{Config: true, Zoxide: true},
			expected: []string{"config", "zoxide"},
		},
		{
			name:     "All options are true",
			opts:     ListOptions{Kitty: true, Config: true, Zoxide: true},
			expected: []string{"kitty", "config", "zoxide"},
		},
		{
			name:     "Only Panes is true",
			opts:     ListOptions{Panes: true},
			expected: []string{"kitty-window"},
		},
		{
			name:     "Panes with Kitty",
			opts:     ListOptions{Panes: true, Kitty: true},
			expected: []string{"kitty", "kitty-window"},
		},
		{
			name:     "Panes with Zoxide",
			opts:     ListOptions{Panes: true, Zoxide: true},
			expected: []string{"zoxide", "kitty-window"},
		},
		{
			name:     "Panes with all sources",
			opts:     ListOptions{Panes: true, Kitty: true, Config: true, Zoxide: true, SessionFile: true},
			expected: []string{"kitty", "config", "sessionfile", "zoxide", "kitty-window"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := srcs(tt.opts)
			assert.Equal(t, tt.expected, result)
		})
	}
}
