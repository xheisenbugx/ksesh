package lister

import (
	"sync"

	"github.com/xheisenbugx/ksesh/cache"
	"github.com/xheisenbugx/ksesh/formatter"
	"github.com/xheisenbugx/ksesh/github"
	"github.com/xheisenbugx/ksesh/home"
	"github.com/xheisenbugx/ksesh/kitty"
	"github.com/xheisenbugx/ksesh/model"
	"github.com/xheisenbugx/ksesh/remote"
	"github.com/xheisenbugx/ksesh/sessionfile"
	"github.com/xheisenbugx/ksesh/zoxide"
)

type Lister interface {
	List(opts ListOptions) (model.SeshSessions, error)
	ListKittyWindows() (model.SeshSessions, error)
	FindKittySession(name string) (model.SeshSession, bool)
	FindKittySessionByBase(base string) (model.SeshSession, bool)
	GetAttachedKittySession() (model.SeshSession, bool)
	GetLastKittySession() (model.SeshSession, bool)
	FindConfigSession(name string) (model.SeshSession, bool)
	FindConfigWildcard(path string) (model.WildcardConfig, bool)
	FindZoxideSession(name string) (model.SeshSession, bool)
	FindSessionFileConfig(name string) (model.SeshSession, bool)
	Format(sessions model.SeshSessions, opts ListOptions) (model.SeshSessions, error)
}

type RealLister struct {
	config      model.Config
	home        home.Home
	kitty       kitty.Kitty
	zoxide      zoxide.Zoxide
	sessionfile sessionfile.SessionFile
	formatter   formatter.Formatter
	issues      *cache.Namespace[github.Issue]
	// remote lists the [[host]]s' directories; nil lists none.
	remote remote.Remote

	// wildcards caches config.WildcardConfigs with their patterns expanded, so
	// resolving a wildcard for every session in a list expands each pattern
	// once. See expandedWildcards.
	wildcardsOnce sync.Once
	wildcards     []expandedWildcard
}

func NewLister(
	config model.Config,
	home home.Home,
	kitty kitty.Kitty,
	zoxide zoxide.Zoxide,
	sessionfile sessionfile.SessionFile,
	formatter formatter.Formatter,
	issues *cache.Namespace[github.Issue],
	options ...Option,
) Lister {
	l := &RealLister{
		config:      config,
		home:        home,
		kitty:       kitty,
		zoxide:      zoxide,
		sessionfile: sessionfile,
		formatter:   formatter,
		issues:      issues,
	}
	for _, o := range options {
		o(l)
	}
	return l
}

// Option sets up a lister beyond what every lister needs.
type Option func(*RealLister)

// WithRemote lists the [[host]]s' directories through r.
func WithRemote(r remote.Remote) Option {
	return func(l *RealLister) { l.remote = r }
}
