package seshcli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xheisenbugx/ksesh/agent"
	"github.com/xheisenbugx/ksesh/model"
)

func TestPickerShelves(t *testing.T) {
	open := func(names ...string) model.Config {
		var cfg model.Config
		cfg.Picker.OpenShelves = &names
		return cfg
	}

	shelves, err := pickerShelves(model.Config{})
	require.NoError(t, err)
	assert.Equal(t, []agent.Section{agent.SectionWorking}, shelves, "only Working, unset")

	shelves, err = pickerShelves(open())
	require.NoError(t, err)
	assert.Empty(t, shelves, "an empty list keeps them all closed")

	shelves, err = pickerShelves(open("Snoozed", " settled "))
	require.NoError(t, err)
	assert.Equal(t, []agent.Section{agent.Snoozed, agent.Settled}, shelves)

	_, err = pickerShelves(open("inbox"))
	assert.ErrorContains(t, err, `no shelf "inbox"`)
}
