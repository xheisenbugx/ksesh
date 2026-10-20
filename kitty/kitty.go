// Package kitty drives kitty (https://sw.kovidgoyal.net/kitty/) over remote
// control, mapping sesh's session model onto kitty's native sessions.
//
// Kitty has no session object of its own: a session is the set of windows kitty
// has tagged with the same session_name, whether they were created from a
// session file (goto_session) or by `launch --add-to-session`. Everything this
// package knows comes from `kitten @ ls`, so it maps:
//
//	tmux session -> kitty session (windows sharing a session_name)
//	tmux window  -> kitty tab holding the session's windows
//	tmux pane    -> kitty window
package kitty

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/xheisenbugx/ksesh/model"
	"github.com/xheisenbugx/ksesh/oswrap"
	"github.com/xheisenbugx/ksesh/shell"
)

// User variables sesh sets on the windows it creates. Kitty shows them in
// `kitten @ ls` and they can be matched with `--match var:NAME=VALUE`.
const (
	// RootVar records the session root, so `sesh root` and the session's
	// path survive the first window cd-ing elsewhere or closing.
	RootVar = "ksesh_root"
	// NameVar overrides the session's name. Kitty can't rename a session, so
	// `sesh rename` stores the new name here instead.
	NameVar = "ksesh_name"
)

type Kitty interface {
	ListSessions() ([]*model.KittySession, error)
	// ListWindows lists the tabs of a session, the current one when empty.
	ListWindows(targetSession string) ([]*model.KittyTab, error)
	ListAllWindowNames(format string) (map[string][]string, error)
	ListAllWindows() (map[string][]model.KittyTab, error)
	// NewSession creates a session with one tab rooted at startDir, without
	// focusing it. It returns the "window:<id>" target of its window.
	NewSession(sessionName string, startDir string) (string, error)
	// NewWindowInSession creates a tab in a session and returns the
	// "window:<id>" target of its window.
	NewWindowInSession(opts model.KittyTabOpts) (string, error)
	// IsAttached reports whether sesh runs inside kitty, the equivalent of
	// being inside tmux.
	IsAttached() bool
	// Reachable reports whether kitty can be reached over remote control.
	Reachable() bool
	// SendKeys types a command followed by enter into the target, which is a
	// "window:<id>" or "tab:<id>" target or a session name.
	SendKeys(target string, command string) (string, error)
	CapturePane(targetSession string) (string, error)
	// SelectWindow focuses a "window:<id>" or "tab:<id>" target.
	SelectWindow(target string) (string, error)
	SwitchOrAttach(name string, opts model.ConnectOpts) (string, error)
	ListKittyWindows() ([]*model.KittyWindow, error)
	SelectPane(windowID int) (string, error)
	GetCurrentSession() (string, error)
	FocusedSessionPath() (string, error)
	RenameSession(target string, newName string) (string, error)
	KillSession(target string) (string, error)
	// GotoSessionFile loads a kitty session file, or switches to its session
	// when it is already running, and returns the session's name.
	GotoSessionFile(path string) (string, error)
	// SetSessionName gives a running session the name sesh knows it by.
	SetSessionName(nativeName string, name string) error
	// StartKitty launches a new kitty instance listening on a private
	// socket, for when sesh runs outside kitty and none can be reached.
	// Remote control then goes to the new instance. It returns the id of
	// the window kitty opened with, which the caller closes once it has
	// created its session.
	StartKitty(startDir string) (int, error)
	CloseWindow(windowID int) error
	// Freeze and Thaw bracket a TUI: see freeze.go.
	Freeze(captures bool) error
	Thaw() error
}

type RealKitty struct {
	os       oswrap.Os
	shell    shell.Shell
	bin      string
	kittyBin string
	socket   string
	newOSWin bool

	toOnce sync.Once
	to     string // --to address; empty talks to kitty over the tty

	mu    sync.Mutex
	state *state // cached `kitten @ ls`, dropped by every mutation

	// See Freeze.
	frozen       bool
	captures     map[int]string
	pendingKills [][]int
}

func NewKitty(os oswrap.Os, shell shell.Shell, config model.Config) Kitty {
	bin := config.KittenCommand
	if bin == "" {
		bin = "kitten"
	}
	kittyBin := config.KittyCommand
	if kittyBin == "" {
		kittyBin = "kitty"
	}
	return &RealKitty{
		os:       os,
		shell:    shell,
		bin:      bin,
		kittyBin: kittyBin,
		socket:   config.KittySocket,
		newOSWin: config.NewOSWindow,
	}
}

// selfWindowID is the kitty window sesh runs in, or 0 outside kitty.
func (k *RealKitty) selfWindowID() int {
	id, _ := strconv.Atoi(k.os.Getenv("KITTY_WINDOW_ID"))
	return id
}

func (k *RealKitty) IsAttached() bool {
	return k.selfWindowID() != 0
}

func (k *RealKitty) Reachable() bool {
	return k.IsAttached() || k.address() != ""
}

// address resolves where remote control requests go. Inside kitty the tty
// always reaches the right instance, so $KITTY_LISTEN_ON (which kitten reads
// on its own) or nothing is used. Outside it, the configured socket is.
func (k *RealKitty) address() string {
	k.toOnce.Do(func() {
		if k.IsAttached() {
			return
		}
		if env := k.os.Getenv("KITTY_LISTEN_ON"); env != "" {
			k.to = env
			return
		}
		k.to = resolveSocket(k.socket, k.os.Getenv("TMPDIR"))
	})
	return k.to
}

// resolveSocket turns a listen_on value into an address kitten can dial.
// Kitty resolves a relative unix socket path against the temp directory and,
// when listen_on doesn't contain {kitty_pid}, may suffix the path with the pid,
// so a path that doesn't exist falls back to the newest "<path>-*".
func resolveSocket(socket string, tmpdir string) string {
	if socket == "" {
		return ""
	}
	path, ok := strings.CutPrefix(socket, "unix:")
	if !ok || strings.HasPrefix(path, "@") {
		return socket
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, path[2:])
		}
	}
	if !filepath.IsAbs(path) {
		if tmpdir == "" {
			tmpdir = os.TempDir()
		}
		path = filepath.Join(tmpdir, path)
	}
	if _, err := os.Stat(path); err == nil {
		return "unix:" + path
	}
	matches, _ := filepath.Glob(path + "-*")
	newest, newestTime := "", time.Time{}
	for _, m := range matches {
		info, err := os.Stat(m)
		if err != nil || info.Mode()&os.ModeSocket == 0 {
			continue
		}
		if newest == "" || info.ModTime().After(newestTime) {
			newest, newestTime = m, info.ModTime()
		}
	}
	if newest == "" {
		return ""
	}
	return "unix:" + newest
}

// rc runs a remote control command and returns its output.
func (k *RealKitty) rc(args ...string) (string, error) {
	if k.isFrozen() {
		return "", errFrozen
	}
	full := []string{"@"}
	if to := k.address(); to != "" {
		full = append(full, "--to", to)
	}
	out, err := k.shell.CmdCapture(k.bin, append(full, args...)...)
	if err != nil {
		return out, fmt.Errorf("kitten @ %s: %w", args[0], err)
	}
	return out, nil
}

// mutate runs a remote control command that changes kitty's state.
func (k *RealKitty) mutate(args ...string) (string, error) {
	k.invalidate()
	out, err := k.rc(args...)
	k.invalidate()
	return out, err
}

func (k *RealKitty) invalidate() {
	k.mu.Lock()
	if !k.frozen {
		k.state = nil
	}
	k.mu.Unlock()
}

// session finds a running session by sesh name, falling back to kitty's own
// name so a renamed session can still be addressed the way kitty knows it.
func (s *state) session(name string) *sessionState {
	for _, sess := range s.sessions {
		if sess.name == name {
			return sess
		}
	}
	for _, sess := range s.sessions {
		if sess.native == name {
			return sess
		}
	}
	return nil
}

// sessionFor resolves a session by name, the current one when empty.
func (k *RealKitty) sessionFor(name string) (*state, *sessionState, error) {
	st, err := k.load()
	if err != nil {
		return nil, nil, err
	}
	if name == "" {
		if st.current == nil {
			return st, nil, fmt.Errorf("not inside a kitty session")
		}
		return st, st.current, nil
	}
	sess := st.session(name)
	if sess == nil {
		return st, nil, fmt.Errorf("can't find kitty session: %s", name)
	}
	return st, sess, nil
}

func (k *RealKitty) ListSessions() ([]*model.KittySession, error) {
	st, err := k.load()
	if err != nil {
		// Like `tmux ls` without a server: no kitty means no sessions.
		return []*model.KittySession{}, nil
	}
	sessions := make([]*model.KittySession, 0, len(st.sessions))
	for _, sess := range st.sessions {
		sessions = append(sessions, sess.model(sess == st.current))
	}
	return sessions, nil
}

func (k *RealKitty) GetCurrentSession() (string, error) {
	st, err := k.load()
	if err != nil {
		return "", err
	}
	if st.current == nil {
		return "", fmt.Errorf("not inside a kitty session")
	}
	return st.current.name, nil
}

func (k *RealKitty) FocusedSessionPath() (string, error) {
	st, err := k.load()
	if err != nil {
		return "", err
	}
	if st.current == nil {
		return "", fmt.Errorf("not inside a kitty session")
	}
	return st.current.root, nil
}

func (k *RealKitty) ListWindows(targetSession string) ([]*model.KittyTab, error) {
	_, sess, err := k.sessionFor(targetSession)
	if err != nil {
		return nil, err
	}
	tabs := make([]*model.KittyTab, 0, len(sess.tabs))
	for _, tab := range sess.tabModels() {
		tabs = append(tabs, &tab)
	}
	return tabs, nil
}

func (k *RealKitty) ListAllWindows() (map[string][]model.KittyTab, error) {
	st, err := k.load()
	if err != nil {
		return nil, err
	}
	all := make(map[string][]model.KittyTab, len(st.sessions))
	for _, sess := range st.sessions {
		all[sess.name] = sess.tabModels()
	}
	return all, nil
}

// ListAllWindowNames renders every session's tabs through format, keyed by
// session name. Tabs that render empty are left out, which is how a format
// such as "{active_title}" picks out the active tab alone.
func (k *RealKitty) ListAllWindowNames(format string) (map[string][]string, error) {
	all, err := k.ListAllWindows()
	if err != nil {
		return nil, err
	}
	names := make(map[string][]string, len(all))
	for session, tabs := range all {
		for _, tab := range tabs {
			if name := FormatTab(format, tab); name != "" {
				names[session] = append(names[session], name)
			}
		}
	}
	return names, nil
}

func (k *RealKitty) ListKittyWindows() ([]*model.KittyWindow, error) {
	st, err := k.load()
	if err != nil {
		return []*model.KittyWindow{}, nil
	}
	if st.current == nil {
		return []*model.KittyWindow{}, nil
	}
	self := k.selfWindowID()
	windows := []*model.KittyWindow{}
	for i, tab := range st.current.tabs {
		for _, w := range tab.windows {
			if w.ID == self {
				continue
			}
			windows = append(windows, &model.KittyWindow{
				TabIndex: i + 1,
				TabTitle: tab.tab.Title,
				ID:       w.ID,
				Title:    w.Title,
				Command:  w.command(),
				Path:     w.dir(),
			})
		}
	}
	return windows, nil
}

// sortSessions orders sessions most recently focused first, the order tmux
// lists sessions by last attach.
func sortSessions(sessions []*sessionState) {
	sort.SliceStable(sessions, func(i, j int) bool {
		if sessions[i].lastFocused != sessions[j].lastFocused {
			return sessions[i].lastFocused > sessions[j].lastFocused
		}
		return sessions[i].created < sessions[j].created
	})
}
