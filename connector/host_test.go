package connector

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xheisenbugx/ksesh/kitty"
	"github.com/xheisenbugx/ksesh/lister"
	"github.com/xheisenbugx/ksesh/model"
	"github.com/xheisenbugx/ksesh/remote"
)

var hosts = []model.HostConfig{{Name: "devbox"}}

func TestHostStrategy(t *testing.T) {
	ml := lister.NewMockLister(t)
	c := &RealConnector{lister: ml, hosts: hosts}

	conn, err := hostStrategy(c, "elsewhere:/x")
	require.NoError(t, err)
	assert.False(t, conn.Found, "only a configured host")
	conn, _ = hostStrategy(c, "devbox")
	assert.False(t, conn.Found, "not without a directory")

	ml.EXPECT().FindKittySession("api@devbox").Return(model.SeshSession{}, false).Once()
	conn, err = hostStrategy(c, "devbox:~/c/api")
	require.NoError(t, err)
	assert.True(t, conn.Found)
	assert.True(t, conn.New)
	assert.Equal(t, model.SeshSession{Src: "host", Name: "api@devbox", Path: "~/c/api", Host: "devbox"}, conn.Session)

	ml.EXPECT().FindKittySession("api@devbox").Return(model.SeshSession{Src: "kitty", Name: "api@devbox"}, true).Once()
	conn, _ = hostStrategy(c, "devbox:~/c/api")
	assert.False(t, conn.New, "one already open is gone to")
	assert.Equal(t, "kitty", conn.Session.Src)
}

func TestConnectToHostOpensAKittySessionOverSSH(t *testing.T) {
	mk := kitty.NewMockKitty(t)
	c := &RealConnector{kitty: mk, hosts: hosts, config: model.Config{}}
	window := remote.Join(remote.Window("devbox", "$HOME/c/api")...)
	mk.EXPECT().NewSessionWith("api@devbox", "", window, map[string]string{kitty.HostVar: "devbox", kitty.CwdVar: "~/c/api"}).
		Return("window:7", nil)
	mk.EXPECT().SwitchOrAttach("api@devbox", model.ConnectOpts{}).Return("", nil)
	mk.EXPECT().IsAttached().Return(true).Maybe()

	_, err := connectToHost(c, model.Connection{Found: true, New: true,
		Session: model.SeshSession{Src: "host", Name: "api@devbox", Path: "~/c/api", Host: "devbox"}}, model.ConnectOpts{})
	require.NoError(t, err)
}
