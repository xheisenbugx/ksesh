package lister

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xheisenbugx/ksesh/kitty"
	"github.com/xheisenbugx/ksesh/model"
)

func TestListingPutsSessionsThatNeedYouFirst(t *testing.T) {
	sessions := []*model.KittySession{
		{Name: "current", Attached: 1},
		{Name: "busy", AgentStatus: "working"},
		{Name: "finished", AgentStatus: "done"},
		{Name: "plain"},
		{Name: "blocked", AgentStatus: "waiting"},
	}
	mockKitty := kitty.NewMockKitty(t)
	mockKitty.EXPECT().ListSessions().Return(sessions, nil)
	l := NewLister(model.Config{}, nil, mockKitty, nil, nil, nil, nil).(*RealLister)

	listed, err := listKittyByAttention(l)
	require.NoError(t, err)
	assert.Equal(t, []string{"kitty:blocked", "kitty:finished", "kitty:current", "kitty:busy", "kitty:plain"}, listed.OrderedIndex)

	last, ok := l.GetLastKittySession()
	require.True(t, ok)
	assert.Equal(t, "busy", last.Name, "`ksesh last` still goes by recency")
}
