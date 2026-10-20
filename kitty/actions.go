package kitty

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/xheisenbugx/ksesh/model"
)

// Target prefixes for the strings NewSession, NewWindowInSession, SendKeys and
// SelectWindow pass around. Anything without one is a session name.
const (
	windowTargetPrefix = "window:"
	tabTargetPrefix    = "tab:"
)

// WindowTarget names a kitty window as a target.
func WindowTarget(id int) string { return windowTargetPrefix + strconv.Itoa(id) }

// TabTarget names a kitty tab as a target.
func TabTarget(id int) string { return tabTargetPrefix + strconv.Itoa(id) }

func (k *RealKitty) NewSession(sessionName string, startDir string) (string, error) {
	if sessionName == "" || sessionName == "." || sessionName == "!" {
		// kitty reads these as "the source window's session" and "no session".
		return "", fmt.Errorf("invalid kitty session name: %q", sessionName)
	}
	launchType := "--type=tab"
	if k.newOSWin {
		launchType = "--type=os-window"
	}
	native := NativeName(sessionName)
	args := []string{"launch", launchType, "--keep-focus", "--add-to-session", native}
	if startDir != "" {
		args = append(args, "--cwd", startDir, "--var", RootVar+"="+startDir)
	}
	if native != sessionName {
		args = append(args, "--var", NameVar+"="+sessionName)
	}
	out, err := k.mutate(args...)
	if err != nil {
		return "", err
	}
	return windowTargetFromLaunch(out)
}

func (k *RealKitty) NewWindowInSession(opts model.KittyTabOpts) (string, error) {
	_, sess, err := k.sessionFor(opts.TargetSession)
	if err != nil {
		return "", err
	}
	args := []string{
		"launch", "--type=tab",
		"--add-to-session", sess.native,
		// Opening the tab next to one of the session's windows puts it in the
		// session's OS window rather than whichever one has focus.
		"--match", "id:" + strconv.Itoa(sess.focusID),
	}
	if opts.Background {
		args = append(args, "--keep-focus")
	}
	if opts.Name != "" {
		args = append(args, "--tab-title", opts.Name)
	}
	if opts.StartDir != "" {
		args = append(args, "--cwd", opts.StartDir)
	}
	if sess.root != "" {
		args = append(args, "--var", RootVar+"="+sess.root)
	}
	if sess.name != sess.native {
		args = append(args, "--var", NameVar+"="+sess.name)
	}
	if opts.Command != "" {
		args = append(args, "--", userShell(), "-c", opts.Command)
	}
	out, err := k.mutate(args...)
	if err != nil {
		return "", err
	}
	return windowTargetFromLaunch(out)
}

// windowTargetFromLaunch reads the window id `kitten @ launch` prints.
func windowTargetFromLaunch(out string) (string, error) {
	id, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		return "", fmt.Errorf("unexpected output from kitten @ launch: %q", out)
	}
	return WindowTarget(id), nil
}

func userShell() string {
	if sh := os.Getenv("SHELL"); sh != "" {
		return sh
	}
	return "/bin/sh"
}

// resolveTarget turns a target into a --match expression for a window, or for
// a tab when tab is true.
func (k *RealKitty) resolveTarget(target string) (match string, tab bool, err error) {
	if id, ok := strings.CutPrefix(target, windowTargetPrefix); ok {
		return "id:" + id, false, nil
	}
	if id, ok := strings.CutPrefix(target, tabTargetPrefix); ok {
		return "id:" + id, true, nil
	}
	_, sess, err := k.sessionFor(target)
	if err != nil {
		return "", false, err
	}
	return "id:" + strconv.Itoa(sess.focusID), false, nil
}

// SendKeys types command into the target and presses enter, the way tmux
// send-keys does: the shell already running there runs it, and is still there
// when it exits.
func (k *RealKitty) SendKeys(target string, command string) (string, error) {
	match, tab, err := k.resolveTarget(target)
	if err != nil {
		return "", err
	}
	matchFlag := "--match"
	if tab {
		matchFlag = "--match-tab"
	}
	// send-text decodes backslash escapes, so literal backslashes are doubled
	// and \r is the enter key.
	text := strings.ReplaceAll(command, `\`, `\\`) + `\r`
	return k.rc("send-text", matchFlag, match, "--", text)
}

func (k *RealKitty) SelectWindow(target string) (string, error) {
	match, tab, err := k.resolveTarget(target)
	if err != nil {
		return "", err
	}
	if tab {
		return k.mutate("focus-tab", "--match", match)
	}
	return k.mutate("focus-window", "--match", match)
}

func (k *RealKitty) SelectPane(windowID int) (string, error) {
	if _, err := k.mutate("focus-window", "--match", "id:"+strconv.Itoa(windowID)); err != nil {
		return "", fmt.Errorf("failed to focus window %d: %w", windowID, err)
	}
	return fmt.Sprintf("focused window %d", windowID), nil
}

// SwitchOrAttach brings kitty to a running session. There is nothing to attach
// to outside kitty, so both switch to it; the caller brings kitty forward when
// sesh runs elsewhere.
func (k *RealKitty) SwitchOrAttach(name string, _ model.ConnectOpts) (string, error) {
	_, sess, err := k.sessionFor(name)
	if err != nil {
		return "", err
	}
	if err := k.focusSession(sess); err != nil {
		return "", fmt.Errorf("failed to switch to kitty session: %w", err)
	}
	return fmt.Sprintf("switching to kitty session: %s", sess.name), nil
}

// focusSession switches to a session. goto_session is preferred because it
// also records the switch in kitty's session history, which `goto_session -1`
// and tab_bar_filter's "session:~" read; it only takes names that survive
// being read back from a file path, so other names focus the window directly.
func (k *RealKitty) focusSession(sess *sessionState) error {
	if gotoSafe(sess.native) {
		path := "/" + sess.native + ".kitty-session"
		if _, err := k.mutate("action", "goto_session "+shellQuote(path)); err == nil {
			return nil
		}
	}
	_, err := k.mutate("focus-window", "--match", "id:"+strconv.Itoa(sess.focusID))
	return err
}

// NativeName is the name kitty knows a new session by. goto_session reads a
// session's name back from a file path, so a name a path can't carry (such as
// "webapp/backend") gets a path-safe stand-in, and keeps its own name as a
// rename does. Without goto_session the switch isn't recorded in kitty's
// session history, and tab_bar_filter's "session:~" hides the session's tabs
// the moment focus moves to a tab outside any session.
func NativeName(name string) string {
	if gotoSafe(name) {
		return name
	}
	native := strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', '\'', '"':
			return '-'
		}
		return r
	}, strings.TrimSpace(name))
	native = strings.TrimLeft(native, "-")
	if native == "" || native == "." || native == "!" {
		return "session"
	}
	return native
}

func gotoSafe(name string) bool {
	return name != "" && !strings.ContainsAny(name, `/\'"`) && !strings.HasPrefix(name, "-") && strings.TrimSpace(name) == name
}

// shellQuote quotes s for kitty's action parser, which splits like a POSIX
// shell.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

func (k *RealKitty) CapturePane(targetSession string) (string, error) {
	_, sess, err := k.sessionFor(targetSession)
	if err != nil {
		return "", err
	}
	if k.isFrozen() {
		return k.frozenCapture(sess), nil
	}
	return k.rc("get-text", "--ansi", "--match", "id:"+strconv.Itoa(sess.focusID))
}

// matchIDs builds a --match expression for several windows.
func matchIDs(ids []int) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = "id:" + strconv.Itoa(id)
	}
	return strings.Join(parts, " or ")
}

// RenameSession gives a session a new name. Kitty can't rename a session, so
// the name is kept in a user variable on its windows; the session keeps its
// kitty name for goto_session and tab_bar_filter. Renaming it back to the kitty
// name drops the variable.
func (k *RealKitty) RenameSession(target string, newName string) (string, error) {
	_, sess, err := k.sessionFor(target)
	if err != nil {
		return "", err
	}
	if err := k.setName(sess, newName); err != nil {
		return "", err
	}
	return fmt.Sprintf("renamed kitty session %s to %s", sess.name, newName), nil
}

func (k *RealKitty) SetSessionName(nativeName string, name string) error {
	st, err := k.load()
	if err != nil {
		return err
	}
	for _, sess := range st.sessions {
		if sess.native == nativeName {
			return k.setName(sess, name)
		}
	}
	return fmt.Errorf("can't find kitty session: %s", nativeName)
}

func (k *RealKitty) setName(sess *sessionState, name string) error {
	assignment := NameVar + "=" + name
	if name == sess.native {
		assignment = NameVar
	}
	_, err := k.mutate("set-user-vars", "--match", matchIDs(sess.windowIDs()), assignment)
	return err
}

func (k *RealKitty) KillSession(target string) (string, error) {
	_, sess, err := k.sessionFor(target)
	if err != nil {
		return "", err
	}
	if k.isFrozen() {
		k.deferKill(sess)
		return "", nil
	}
	return k.mutate("close-window", "--ignore-no-match", "--match", matchIDs(sess.windowIDs()))
}

func (k *RealKitty) CloseWindow(windowID int) error {
	_, err := k.mutate("close-window", "--ignore-no-match", "--match", "id:"+strconv.Itoa(windowID))
	return err
}

func (k *RealKitty) GotoSessionFile(path string) (string, error) {
	if _, err := k.mutate("action", "goto_session "+shellQuote(path)); err != nil {
		return "", fmt.Errorf("failed to load kitty session file %s: %w", path, err)
	}
	return SessionFileName(path), nil
}

// SessionFileName is the name kitty gives the session a session file creates:
// the file name, less a session file extension.
func SessionFileName(path string) string {
	name := filepath.Base(path)
	for _, ext := range []string{".kitty-session", ".kitty_session", ".session"} {
		if trimmed, ok := strings.CutSuffix(name, ext); ok {
			return trimmed
		}
	}
	return name
}

func (k *RealKitty) StartKitty(startDir string) (int, error) {
	socket := filepath.Join(os.TempDir(), fmt.Sprintf("ksesh-%d.sock", os.Getpid()))
	_ = os.Remove(socket)
	args := []string{
		"--detach",
		"-o", "allow_remote_control=socket-only",
		"--listen-on", "unix:" + socket,
	}
	if startDir != "" {
		args = append(args, "--directory", startDir)
	}
	if _, err := k.shell.CmdCapture(k.kittyBin, args...); err != nil {
		return 0, fmt.Errorf("failed to start kitty: %w", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(socket); err == nil {
			break
		}
		if time.Now().After(deadline) {
			return 0, fmt.Errorf("kitty didn't start listening on %s", socket)
		}
		time.Sleep(50 * time.Millisecond)
	}
	k.toOnce.Do(func() {})
	k.to = "unix:" + socket
	k.invalidate()

	// Kitty opens with a plain window (or the user's startup_session). Only a
	// lone window outside any session is a stand-in the caller may close.
	st, err := k.load()
	if err != nil {
		return 0, err
	}
	var ids []int
	for _, osw := range st.osWindows {
		for _, tab := range osw.Tabs {
			for _, w := range tab.Windows {
				if w.SessionName != "" {
					return 0, nil
				}
				ids = append(ids, w.ID)
			}
		}
	}
	if len(ids) != 1 {
		return 0, nil
	}
	return ids[0], nil
}
