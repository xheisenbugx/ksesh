package connector

import (
	"github.com/xheisenbugx/ksesh/dir"
	"github.com/xheisenbugx/ksesh/focuser"
	"github.com/xheisenbugx/ksesh/home"
	"github.com/xheisenbugx/ksesh/kitty"
	"github.com/xheisenbugx/ksesh/lister"
	"github.com/xheisenbugx/ksesh/model"
	"github.com/xheisenbugx/ksesh/namer"
	"github.com/xheisenbugx/ksesh/startup"
	"github.com/xheisenbugx/ksesh/zoxide"
)

type Connector interface {
	Connect(name string, opts model.ConnectOpts) (string, error)
	ConnectWindow(opts model.WindowConnectOpts) (string, error)
}

type RealConnector struct {
	config  model.Config
	dir     dir.Dir
	home    home.Home
	lister  lister.Lister
	namer   namer.Namer
	startup startup.Startup
	kitty   kitty.Kitty
	zoxide  zoxide.Zoxide
	focuser focuser.Focuser
}

func NewConnector(
	config model.Config,
	dir dir.Dir,
	home home.Home,
	lister lister.Lister,
	namer namer.Namer,
	startup startup.Startup,
	kitty kitty.Kitty,
	zoxide zoxide.Zoxide,
	focuser focuser.Focuser,
) Connector {
	return &RealConnector{
		config,
		dir,
		home,
		lister,
		namer,
		startup,
		kitty,
		zoxide,
		focuser,
	}
}
