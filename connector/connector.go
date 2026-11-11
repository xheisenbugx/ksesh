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
	// hosts are the [[host]]s, whose directories connect opens over ssh.
	hosts []model.HostConfig
}

// Option sets up a connector beyond what every connector needs.
type Option func(*RealConnector)

// WithHosts connects to directories on these machines, as host:path.
func WithHosts(hosts []model.HostConfig) Option {
	return func(c *RealConnector) { c.hosts = hosts }
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
	options ...Option,
) Connector {
	c := &RealConnector{
		config:  config,
		dir:     dir,
		home:    home,
		lister:  lister,
		namer:   namer,
		startup: startup,
		kitty:   kitty,
		zoxide:  zoxide,
		focuser: focuser,
	}
	for _, o := range options {
		o(c)
	}
	return c
}
