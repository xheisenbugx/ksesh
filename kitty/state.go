package kitty

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/xheisenbugx/ksesh/model"
)

// The subset of `kitten @ ls` sesh reads.
type (
	lsOSWindow struct {
		ID        int     `json:"id"`
		IsActive  bool    `json:"is_active"`
		IsFocused bool    `json:"is_focused"`
		Tabs      []lsTab `json:"tabs"`
	}

	lsTab struct {
		ID       int        `json:"id"`
		IsActive bool       `json:"is_active"`
		Title    string     `json:"title"`
		Windows  []lsWindow `json:"windows"`
	}

	lsWindow struct {
		ID                  int               `json:"id"`
		IsActive            bool              `json:"is_active"`
		Title               string            `json:"title"`
		Cwd                 string            `json:"cwd"`
		Cmdline             []string          `json:"cmdline"`
		LastFocusedAt       float64           `json:"last_focused_at"`
		CreatedAt           int64             `json:"created_at"`
		SessionName         string            `json:"session_name"`
		UserVars            map[string]string `json:"user_vars"`
		NeedsAttention      bool              `json:"needs_attention"`
		HasActivity         bool              `json:"has_activity_since_last_focus"`
		ForegroundProcesses []lsProcess       `json:"foreground_processes"`
	}

	lsProcess struct {
		Cmdline []string `json:"cmdline"`
		Cwd     string   `json:"cwd"`
	}
)

// dir is the window's working directory. The foreground process's is the
// live one; the window's own cwd can lag behind a shell that has cd'd.
func (w lsWindow) dir() string {
	for i := len(w.ForegroundProcesses) - 1; i >= 0; i-- {
		if cwd := w.ForegroundProcesses[i].Cwd; cwd != "" {
			return cwd
		}
	}
	return w.Cwd
}

// command is the name of the program running in the window.
func (w lsWindow) command() string {
	cmdline := w.Cmdline
	if n := len(w.ForegroundProcesses); n > 0 && len(w.ForegroundProcesses[n-1].Cmdline) > 0 {
		cmdline = w.ForegroundProcesses[n-1].Cmdline
	}
	if len(cmdline) == 0 {
		return ""
	}
	// Login shells show up as "-zsh".
	name := filepath.Base(cmdline[0])
	if len(name) > 1 && name[0] == '-' {
		name = name[1:]
	}
	return name
}

type tabState struct {
	tab     lsTab
	windows []lsWindow // the tab's windows that belong to the session
}

type sessionState struct {
	name        string
	native      string
	root        string
	tabs        []*tabState
	focusID     int
	focusTabID  int
	focusAt     float64
	lastFocused float64
	created     int64
	alerts      []string
}

type state struct {
	osWindows []lsOSWindow
	sessions  []*sessionState
	current   *sessionState // the session the user is in, if any
	currentID int           // the window the user is in, if known
}

func (k *RealKitty) load() (*state, error) {
	k.mu.Lock()
	cached := k.state
	k.mu.Unlock()
	if cached != nil {
		return cached, nil
	}
	out, err := k.rc("ls")
	if err != nil {
		return nil, err
	}
	st, err := parseState(out, k.selfWindowID())
	if err != nil {
		return nil, err
	}
	k.mu.Lock()
	k.state = st
	k.mu.Unlock()
	return st, nil
}

func parseState(raw string, selfID int) (*state, error) {
	var osWindows []lsOSWindow
	if err := json.Unmarshal([]byte(raw), &osWindows); err != nil {
		return nil, fmt.Errorf("couldn't parse kitten @ ls output: %w", err)
	}
	st := &state{osWindows: osWindows}

	byNative := map[string]*sessionState{}
	var order []*sessionState
	for _, osw := range osWindows {
		for _, tab := range osw.Tabs {
			var tabOf map[*sessionState]*tabState
			for _, w := range tab.Windows {
				if w.SessionName == "" {
					continue
				}
				sess, ok := byNative[w.SessionName]
				if !ok {
					sess = &sessionState{name: w.SessionName, native: w.SessionName, created: w.CreatedAt}
					byNative[w.SessionName] = sess
					order = append(order, sess)
				}
				if tabOf == nil {
					tabOf = map[*sessionState]*tabState{}
				}
				ts, ok := tabOf[sess]
				if !ok {
					ts = &tabState{tab: tab}
					tabOf[sess] = ts
					sess.tabs = append(sess.tabs, ts)
				}
				ts.windows = append(ts.windows, w)
				sess.add(w, tab.ID, selfID)
			}
		}
	}
	for _, sess := range order {
		sess.finish()
	}
	sortSessions(order)
	st.sessions = order
	st.currentID, st.current = st.findCurrent(selfID, byNative)
	return st, nil
}

// add folds one of the session's windows into it.
func (s *sessionState) add(w lsWindow, tabID int, selfID int) {
	if w.CreatedAt < s.created {
		s.created = w.CreatedAt
	}
	if name := w.UserVars[NameVar]; name != "" && s.name == s.native {
		s.name = name
	}
	if w.LastFocusedAt > s.lastFocused {
		s.lastFocused = w.LastFocusedAt
	}
	// The window sesh runs in is skipped as a focus target: it is usually a
	// picker overlay that is about to close.
	if w.ID != selfID {
		if s.focusID == 0 || w.LastFocusedAt > s.focusAt || (w.LastFocusedAt == s.focusAt && w.ID < s.focusID) {
			s.focusID, s.focusTabID, s.focusAt = w.ID, tabID, w.LastFocusedAt
		}
	}
	if w.NeedsAttention && !contains(s.alerts, "bell") {
		s.alerts = append(s.alerts, "bell")
	}
	if w.HasActivity && !contains(s.alerts, "activity") {
		s.alerts = append(s.alerts, "activity")
	}
}

// finish settles what can only be known once every window has been seen.
func (s *sessionState) finish() {
	first := lsWindow{CreatedAt: -1}
	for _, ts := range s.tabs {
		for _, w := range ts.windows {
			if root := w.UserVars[RootVar]; root != "" && s.root == "" {
				s.root = root
			}
			if first.CreatedAt == -1 || w.CreatedAt < first.CreatedAt {
				first = w
			}
		}
	}
	if s.root == "" {
		s.root = first.dir()
	}
	// A session whose windows are all the one sesh runs in still needs a
	// target.
	if s.focusID == 0 && len(s.tabs) > 0 && len(s.tabs[0].windows) > 0 {
		s.focusID, s.focusTabID = s.tabs[0].windows[0].ID, s.tabs[0].tab.ID
	}
}

// findCurrent works out the window, and from it the session, the user is in:
// the window sesh runs in when inside kitty, otherwise the focused window of
// the active OS window.
func (st *state) findCurrent(selfID int, byNative map[string]*sessionState) (int, *sessionState) {
	if selfID != 0 {
		for _, osw := range st.osWindows {
			for _, tab := range osw.Tabs {
				for _, w := range tab.Windows {
					if w.ID == selfID {
						return w.ID, sessionOf(w, tab, byNative)
					}
				}
			}
		}
	}
	var active *lsOSWindow
	for i := range st.osWindows {
		osw := &st.osWindows[i]
		if osw.IsFocused {
			active = osw
			break
		}
		if osw.IsActive && active == nil {
			active = osw
		}
	}
	if active == nil {
		return 0, nil
	}
	for _, tab := range active.Tabs {
		if !tab.IsActive {
			continue
		}
		for _, w := range tab.Windows {
			if w.IsActive {
				return w.ID, sessionOf(w, tab, byNative)
			}
		}
	}
	return 0, nil
}

// sessionOf is the session a window is in. A window kitty didn't add to any
// session takes the session of its tab: that is what an overlay opened from a
// key binding (`launch --type=overlay ksesh picker`) looks like, since kitty
// only carries the session over to windows launched with --cwd=current.
func sessionOf(w lsWindow, tab lsTab, byNative map[string]*sessionState) *sessionState {
	if w.SessionName != "" {
		return byNative[w.SessionName]
	}
	var best *lsWindow
	for i := range tab.Windows {
		other := &tab.Windows[i]
		if other.SessionName == "" {
			continue
		}
		if best == nil || other.LastFocusedAt > best.LastFocusedAt {
			best = other
		}
	}
	if best == nil {
		return nil
	}
	return byNative[best.SessionName]
}

func (s *sessionState) model(attached bool) *model.KittySession {
	session := &model.KittySession{
		Name:          s.name,
		NativeName:    s.native,
		Path:          s.root,
		Windows:       len(s.tabs),
		FocusWindowID: s.focusID,
		LastFocused:   s.lastFocused,
		Alerts:        s.alerts,
	}
	if attached {
		session.Attached = 1
	}
	if s.created > 0 {
		created := time.Unix(0, s.created)
		session.Created = &created
	}
	return session
}

func (s *sessionState) tabModels() []model.KittyTab {
	tabs := make([]model.KittyTab, 0, len(s.tabs))
	for i, ts := range s.tabs {
		path := ""
		for _, w := range ts.windows {
			if path == "" || w.IsActive {
				path = w.dir()
			}
		}
		tabs = append(tabs, model.KittyTab{
			Name:   ts.tab.Title,
			Path:   path,
			Index:  i + 1,
			ID:     ts.tab.ID,
			Active: ts.tab.ID == s.focusTabID,
		})
	}
	return tabs
}

// windowIDs lists every window of the session.
func (s *sessionState) windowIDs() []int {
	var ids []int
	for _, ts := range s.tabs {
		for _, w := range ts.windows {
			ids = append(ids, w.ID)
		}
	}
	return ids
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
