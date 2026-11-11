package lister

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xheisenbugx/ksesh/model"
	"github.com/xheisenbugx/ksesh/remote"
)

func TestListHosts(t *testing.T) {
	rm := remote.NewMockRemote(t)
	rm.EXPECT().Hosts().Return([]model.HostConfig{{Name: "devbox"}, {Name: "down"}, {Name: "ci"}})
	rm.EXPECT().Dirs("devbox").Return("/home/me", []string{"/home/me/c/api", "/srv/logs"}, nil)
	rm.EXPECT().Dirs("down").Return("", nil, errors.New("no route to host"))
	rm.EXPECT().Dirs("ci").Return("/root", []string{"/root"}, nil)
	l := &RealLister{remote: rm}

	got, err := listHosts(l)
	require.NoError(t, err, "a host that can't be reached is left out, not an error")
	assert.Equal(t, []string{"host:devbox:~/c/api", "host:devbox:/srv/logs", "host:ci:~"}, got.OrderedIndex, "in the order the hosts are configured")
	assert.Equal(t, model.SeshSession{Src: "host", Name: "devbox:~/c/api", Path: "/home/me/c/api", Host: "devbox"}, got.Directory["host:devbox:~/c/api"])
}

func TestHostsAreOnlyListedWhenAskedFor(t *testing.T) {
	assert.NotContains(t, srcs(ListOptions{}), "host", "every host is an ssh round trip")
	assert.Equal(t, []string{"host"}, srcs(ListOptions{Hosts: true}))
	got, err := listHosts(&RealLister{})
	require.NoError(t, err)
	assert.Empty(t, got.OrderedIndex, "without a remote, nothing")
}
