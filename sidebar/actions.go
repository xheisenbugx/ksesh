package sidebar

import (
	"fmt"
	"strconv"

	tea "charm.land/bubbletea/v2"
)

// ksesh is the command line running this ksesh with args.
func (m Model) ksesh(args ...string) []string {
	exe := m.opts.Ksesh
	if len(exe) == 0 {
		exe = []string{"ksesh"}
	}
	return append(append([]string{}, exe...), args...)
}

// review opens `ksesh agent diff` on the selected thread, paged, over the
// window beside the sidebar: the sidebar is too narrow for a diff.
func (m Model) review() (tea.Model, tea.Cmd) {
	a, ok := m.selectedAgent()
	if !ok {
		return m, nil
	}
	self, env := m.opts.Self, m.opts.Env
	command := m.ksesh("agent", "diff", strconv.Itoa(a.WindowID), "--pager")
	return m, m.do("", nil, func(b Backend) error {
		target, err := b.LastWorkWindow(self)
		if err != nil {
			return err
		}
		if target == 0 {
			return fmt.Errorf("no window beside the sidebar to open the review over")
		}
		return b.OverlayOn(target, env, command)
	})
}
