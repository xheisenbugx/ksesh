package kitty

import (
	"strconv"
)

// The thread sidebar (`ksesh sidebar`) runs in a kitty window docked to the
// left of a tab (`launch --type=tab-dock`, kitty 0.50+). Each tab gets one of
// its own: a tab-dock belongs to the tab it was opened in. The window is
// marked with RoleVar=SidebarRole so ksesh can find it again, and so it never
// counts as one of the session's windows: it isn't where you work.
const (
	RoleVar     = "ksesh_role"
	SidebarRole = "sidebar"
)

func isSidebar(w lsWindow) bool {
	return w.UserVars[RoleVar] == SidebarRole
}

// SidebarOpts is how a sidebar is docked.
type SidebarOpts struct {
	// Edge is left or right.
	Edge string
	// Size is a number of columns, or a percentage of the tab ("20%").
	Size string
	// Command is what runs in it, e.g. ["/path/to/ksesh", "sidebar"].
	Command []string
}

// SidebarLaunchArgs are the `kitten @` arguments that dock a sidebar to the
// tab holding windowID, without taking the focus from it. contrib's watcher
// runs the same ones to give every tab you go to a sidebar.
func SidebarLaunchArgs(opts SidebarOpts) []string {
	edge, size := opts.Edge, opts.Size
	if edge == "" {
		edge = "left"
	}
	if size == "" {
		size = "38"
	}
	args := []string{
		"launch", "--type=tab-dock", "--dock-edge=" + edge, "--dock-size=" + size,
		"--var=" + RoleVar + "=" + SidebarRole, "--keep-focus", "--title=threads",
	}
	return append(append(args, "--"), opts.Command...)
}

// SidebarTab reports the sidebar of the tab holding windowID, 0 if it has
// none.
func (k *RealKitty) SidebarTab(windowID int) (sidebarID int, err error) {
	st, err := k.load()
	if err != nil {
		return 0, err
	}
	for _, osw := range st.osWindows {
		for _, tab := range osw.Tabs {
			holds, sidebar := false, 0
			for _, w := range tab.Windows {
				holds = holds || w.ID == windowID
				if isSidebar(w) {
					sidebar = w.ID
				}
			}
			if holds {
				return sidebar, nil
			}
		}
	}
	return 0, nil
}

// OpenSidebar docks a sidebar to the tab holding windowID and returns its
// window id. It doesn't check whether the tab has one already.
func (k *RealKitty) OpenSidebar(windowID int, opts SidebarOpts) (int, error) {
	args := SidebarLaunchArgs(opts)
	// --match picks the tab to open in; it goes before the "--" ending the
	// options.
	args = append([]string{args[0], "--match", "window_id:" + strconv.Itoa(windowID)}, args[1:]...)
	out, err := k.mutate(args...)
	if err != nil {
		return 0, err
	}
	target, err := windowTargetFromLaunch(out)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(target[len(windowTargetPrefix):])
}

// CloseSidebars closes every sidebar, in every tab.
func (k *RealKitty) CloseSidebars() error {
	st, err := k.load()
	if err != nil {
		return err
	}
	var ids []int
	for _, osw := range st.osWindows {
		for _, tab := range osw.Tabs {
			for _, w := range tab.Windows {
				if isSidebar(w) {
					ids = append(ids, w.ID)
				}
			}
		}
	}
	if len(ids) == 0 {
		return nil
	}
	_, err = k.mutate("close-window", "--ignore-no-match", "--match", matchIDs(ids))
	return err
}

// FocusPlain focuses a window directly, without going through its session:
// for moving between a sidebar and the windows of its own tab.
func (k *RealKitty) FocusPlain(windowID int) error {
	_, err := k.mutate("focus-window", "--match", "id:"+strconv.Itoa(windowID))
	return err
}

// LastWorkWindow is the window of windowID's tab focused most recently that
// isn't a sidebar: where to go back to from the sidebar.
func (k *RealKitty) LastWorkWindow(windowID int) (int, error) {
	st, err := k.load()
	if err != nil {
		return 0, err
	}
	for _, osw := range st.osWindows {
		for _, tab := range osw.Tabs {
			best, holds := lsWindow{}, false
			for _, w := range tab.Windows {
				holds = holds || w.ID == windowID
				if !isSidebar(w) && w.ID != windowID && (best.ID == 0 || w.LastFocusedAt > best.LastFocusedAt) {
					best = w
				}
			}
			if holds {
				return best.ID, nil
			}
		}
	}
	return 0, nil
}

// WindowState reports whether a window can be seen, its tab being the active
// one of an OS window (a sidebar in a tab in the background has nobody to
// draw for), and whether it has the keyboard focus.
func (k *RealKitty) WindowState(windowID int) (shown, focused bool, err error) {
	st, err := k.load()
	if err != nil {
		return false, false, err
	}
	for _, osw := range st.osWindows {
		for _, tab := range osw.Tabs {
			for _, w := range tab.Windows {
				if w.ID == windowID {
					// A window's is_focused only says it is its tab's active
					// window; every tab has one.
					return tab.IsActive, w.IsFocused && tab.IsFocused && osw.IsFocused, nil
				}
			}
		}
	}
	return false, false, nil
}

// OverlayOn runs a command as an overlay over a window, in the window's
// working directory: how a picker bound to a key opens over your window when
// the key was pressed in the sidebar, too narrow to hold it. env is NAME=VALUE
// pairs for the command.
func (k *RealKitty) OverlayOn(windowID int, env []string, command []string) error {
	st, err := k.load()
	if err != nil {
		return err
	}
	cwd := ""
	for _, osw := range st.osWindows {
		for _, tab := range osw.Tabs {
			for _, w := range tab.Windows {
				if w.ID == windowID {
					cwd = w.Cwd
					// The shell's cwd is the window's; a program in it may
					// have moved on.
					if len(w.ForegroundProcesses) > 0 && w.ForegroundProcesses[0].Cwd != "" {
						cwd = w.ForegroundProcesses[0].Cwd
					}
				}
			}
		}
	}
	id := "id:" + strconv.Itoa(windowID)
	// --match picks the tab, --next-to the window in it to cover.
	args := []string{"launch", "--match", id, "--next-to", id, "--type=overlay"}
	if cwd != "" {
		args = append(args, "--cwd="+cwd)
	}
	for _, e := range env {
		args = append(args, "--env", e)
	}
	args = append(args, "--")
	_, err = k.mutate(append(args, command...)...)
	return err
}

// Refresh drops what ksesh knows of kitty, so the next call asks it again:
// for a program that stays up, like the sidebar.
func (k *RealKitty) Refresh() {
	k.invalidate()
}
