package seshcli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xheisenbugx/ksesh/lister"
	"github.com/xheisenbugx/ksesh/model"
)

func TestPickerSourcesMirrorTheFzfPicker(t *testing.T) {
	l := lister.NewMockLister(t)
	base := lister.ListOptions{HideAttached: true}
	sources := pickerSources(l, base)

	require.Len(t, sources, 5)
	var keys, labels []string
	for _, s := range sources {
		keys = append(keys, s.Key)
		labels = append(labels, s.Label)
	}
	assert.Equal(t, []string{"ctrl+a", "ctrl+t", "ctrl+g", "ctrl+s", "ctrl+z"}, keys, "ctrl+x stays the picker's kill")
	assert.Equal(t, []string{"all", "kitty", "configs", "files", "zoxide"}, labels)

	// Each lists with its own flag on top of the command's options.
	want := []lister.ListOptions{
		{HideAttached: true},
		{HideAttached: true, Kitty: true},
		{HideAttached: true, Config: true},
		{HideAttached: true, SessionFile: true},
		{HideAttached: true, Zoxide: true},
	}
	for i, s := range sources {
		l.EXPECT().List(want[i]).Return(model.SeshSessions{}, nil).Once()
		_, err := s.Fetch()
		assert.NoError(t, err)
	}
}
