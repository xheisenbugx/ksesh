package agent

import (
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/xheisenbugx/ksesh/model"
)

// Sort orders agents most urgent first and, within a status, longest in it
// first: the agent that has been waiting on you the longest is the one to
// answer next.
func Sort(agents []*model.Agent) {
	slices.SortStableFunc(agents, func(a, b *model.Agent) int {
		pa, pb := Status(a.Status).Priority(), Status(b.Status).Priority()
		if pa != pb {
			return pb - pa
		}
		if c := since(a).Compare(since(b)); c != 0 {
			return c
		}
		// Agents with no time to go by stay grouped by where they are.
		if c := strings.Compare(a.Session, b.Session); c != 0 {
			return c
		}
		return strings.Compare(CleanTitle(a.TabTitle), CleanTitle(b.TabTitle))
	})
}

// CleanTitle drops the status glyph agents put in front of their window title
// (Claude Code's "✳ " or spinner frame), which ksesh shows as an icon of its
// own.
func CleanTitle(title string) string {
	trimmed := strings.TrimLeftFunc(title, func(r rune) bool {
		return unicode.IsSpace(r) || (r > unicode.MaxASCII && (unicode.IsSymbol(r) || r == '·' || (r >= 0x2800 && r <= 0x28ff)))
	})
	if trimmed == "" {
		return title
	}
	return trimmed
}

func since(a *model.Agent) time.Time {
	if a.Since == nil {
		return time.Time{}
	}
	return *a.Since
}

// Next picks the agent to go to: the most urgent one needing attention, after
// the current window when that is one of them, so calling it again cycles
// through all of them. ok is false when nothing needs attention, or the
// current window is the only one that does.
func Next(agents []*model.Agent, currentWindowID int) (*model.Agent, bool) {
	var pending []*model.Agent
	for _, a := range agents {
		if Status(a.Status).NeedsAttention() {
			pending = append(pending, a)
		}
	}
	Sort(pending)
	if len(pending) == 0 {
		return nil, false
	}
	i := slices.IndexFunc(pending, func(a *model.Agent) bool { return a.WindowID == currentWindowID })
	if i < 0 {
		return pending[0], true
	}
	if len(pending) == 1 {
		return nil, false
	}
	return pending[(i+1)%len(pending)], true
}

// Last picks the agent window focused most recently other than the current
// one, so it toggles between two agents the way `goto_session -1` toggles
// between sessions.
func Last(agents []*model.Agent, currentWindowID int) (*model.Agent, bool) {
	var best *model.Agent
	for _, a := range agents {
		if a.WindowID == currentWindowID || a.Focused {
			continue
		}
		if best == nil || a.LastFocused > best.LastFocused {
			best = a
		}
	}
	return best, best != nil
}

// Counts tallies agents by status.
func Counts(agents []*model.Agent) map[Status]int {
	counts := map[Status]int{}
	for _, a := range agents {
		counts[Status(a.Status)]++
	}
	return counts
}
