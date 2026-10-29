package lister

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/xheisenbugx/ksesh/agent"
	"github.com/xheisenbugx/ksesh/model"
)

func kittyKey(name string) string {
	return fmt.Sprintf("kitty:%s", name)
}

// aliasesByName builds a map from configured session name to its alias.
func aliasesByName(config model.Config) map[string]string {
	aliases := make(map[string]string)
	for _, session := range config.SessionConfigs {
		if session.Alias != "" && session.Name != "" {
			aliases[session.Name] = session.Alias
		}
	}
	return aliases
}

// kittyToSesh maps a kitty session onto a SeshSession, copying the time fields
// nil-safely so the two models never alias mutable pointers. If the kitty
// session shares a name with a configured session that has an alias, the alias
// is copied over.
func kittyToSesh(session *model.KittySession, aliases map[string]string) model.SeshSession {
	return model.SeshSession{
		Src:          "kitty",
		Name:         session.Name,
		Alias:        aliases[session.Name],
		Path:         session.Path,
		Attached:     session.Attached,
		Windows:      session.Windows,
		Created:      cloneTime(session.Created),
		LastAttached: cloneTime(session.LastAttached),
		Activity:     cloneTime(session.Activity),
		Alerts:       session.Alerts,
		AgentStatus:  session.AgentStatus,
	}
}

// cloneTime returns a fresh copy of t, or nil when t is nil.
func cloneTime(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	c := *t
	return &c
}

func listKitty(l *RealLister) (model.SeshSessions, error) {
	kittySessions, err := l.kitty.ListSessions()
	if err != nil {
		return model.SeshSessions{}, fmt.Errorf("couldn't list kitty sessions: %q", err)
	}

	aliases := aliasesByName(l.config)
	directory := make(map[string]model.SeshSession)
	orderedIndex := []string{}

	for _, session := range kittySessions {
		key := kittyKey(session.Name)
		orderedIndex = append(orderedIndex, key)
		directory[key] = kittyToSesh(session, aliases)
	}

	return model.SeshSessions{
		Directory:    directory,
		OrderedIndex: orderedIndex,
	}, nil
}

// listKittyByAttention lists kitty sessions with the ones whose agents need
// you (waiting, failed, done) first, most urgent first, and the rest in the
// usual most-recently-focused order. Only listings are reordered:
// GetLastKittySession reads listKitty, where recency alone decides.
func listKittyByAttention(l *RealLister) (model.SeshSessions, error) {
	sessions, err := listKitty(l)
	if err != nil {
		return sessions, err
	}
	urgency := func(key string) int {
		status := agent.Status(sessions.Directory[key].AgentStatus)
		if !status.NeedsAttention() {
			return 0
		}
		return status.Priority()
	}
	slices.SortStableFunc(sessions.OrderedIndex, func(a, b string) int {
		return urgency(b) - urgency(a)
	})
	return sessions, nil
}

// kittyWindowNames fetches the window names of every live kitty session in a
// single kitty call. Failures are non-fatal: the picker simply renders sessions
// without their window names.
func kittyWindowNames(l *RealLister) map[string][]string {
	windowNames, err := l.kitty.ListAllWindowNames(l.config.TUI.WindowNameFormat)
	if err != nil {
		return nil
	}
	return windowNames
}

// attachWindowNames fills in WindowNames for kitty sessions from a session name
// keyed map. Sessions from other sources keep whatever they already have.
func attachWindowNames(sessions model.SeshSessions, windowNames map[string][]string) {
	if len(windowNames) == 0 {
		return
	}
	for _, key := range sessions.OrderedIndex {
		session, ok := sessions.Directory[key]
		if !ok || session.Src != "kitty" {
			continue
		}
		if names, ok := windowNames[session.Name]; ok {
			session.WindowNames = names
			sessions.Directory[key] = session
		}
	}
}

// FindKittySession finds a live kitty session by the name sesh shows, falling
// back to kitty's own name: a renamed session keeps its kitty name, and that is
// the name the namer still derives from its directory.
func (l *RealLister) FindKittySession(name string) (model.SeshSession, bool) {
	kittySessions, err := l.kitty.ListSessions()
	if err != nil {
		return model.SeshSession{}, false
	}
	aliases := aliasesByName(l.config)
	for _, session := range kittySessions {
		if session.Name == name {
			return kittyToSesh(session, aliases), true
		}
	}
	for _, session := range kittySessions {
		if name != "" && session.NativeName == name {
			return kittyToSesh(session, aliases), true
		}
	}
	return model.SeshSession{}, false
}

// FindKittySessionByBase finds a live kitty session for a namer-produced base
// name. It prefers an exact name match; failing that it returns the first
// session whose name is the base followed by the enrichment separator
// (e.g. "base — issue title"). This keeps reconnection working after a
// session has been renamed to include its issue title.
func (l *RealLister) FindKittySessionByBase(base string) (model.SeshSession, bool) {
	if session, exists := l.FindKittySession(base); exists {
		return session, true
	}
	sessions, err := listKitty(l)
	if err != nil {
		return model.SeshSession{}, false
	}
	prefix := base + model.SessionNameSeparator
	for _, key := range sessions.OrderedIndex {
		session := sessions.Directory[key]
		if strings.HasPrefix(session.Name, prefix) {
			return session, true
		}
	}
	return model.SeshSession{}, false
}

// GetLastKittySession returns the session focused most recently before the
// current one. Sessions are listed most recently focused first; the session
// the user is in is skipped rather than assumed to be first, since kitty can
// have focus in a window that belongs to no session.
func (l *RealLister) GetLastKittySession() (model.SeshSession, bool) {
	sessions, err := listKitty(l)
	if err != nil {
		return model.SeshSession{}, false
	}

	var compiled []*regexp.Regexp
	if len(l.config.Blacklist) > 0 {
		compiled = compileBlacklist(l.config.Blacklist)
	}
	for _, index := range sessions.OrderedIndex {
		session := sessions.Directory[index]
		if session.Attached != 0 || isBlacklisted(compiled, session.Name) {
			continue
		}
		return session, true
	}
	return model.SeshSession{}, false
}

func (l *RealLister) GetAttachedKittySession() (model.SeshSession, bool) {
	return GetAttachedKittySession(l)
}

func GetAttachedKittySession(l *RealLister) (model.SeshSession, bool) {
	kittySessions, err := l.kitty.ListSessions()
	if err != nil {
		return model.SeshSession{}, false
	}
	aliases := aliasesByName(l.config)
	for _, session := range kittySessions {
		if session.Attached != 0 {
			return kittyToSesh(session, aliases), true
		}
	}
	return model.SeshSession{}, false
}
