package agent

// Threads: triaging agents the way T3 Code triages its threads.
//
// Every agent is a thread, and every thread is in exactly one section of the
// sidebar, the first of these that applies:
//
//	Snoozed  hidden until a time you picked, or until the agent needs you
//	Settled  done with: settled by hand, or after days without activity
//	Pinned   kept at the top, in the order you put them
//	Working  the agent is running and doesn't need you
//	Active   the inbox: everything else, the latest activity first
//
// Threads move on their own as the agent works:
//
//   - A new prompt (you sent the agent a message) un-settles a thread and ends
//     its snooze.
//   - An agent that starts waiting on you or fails un-settles its thread, and
//     wakes it early when snoozed, with a "woke" badge until you look at it.
//     Finishing a turn doesn't: a long job you snoozed stays out of the way.
//   - A thread nobody touched for Opts.AutoSettleAfter settles itself, unless
//     it is pinned, snoozed, working, waiting or failed, or you un-settled it
//     by hand since it last did something.
//   - So does a thread whose branch's pull request was merged, on the same
//     terms (SettleMerged): the sidebar, which knows about pull requests,
//     settles it.
//
// and by hand: Pin, Settle, Snooze and Wake. Pinning un-settles and wakes a
// thread; settling unpins and wakes it.

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/xheisenbugx/ksesh/model"
)

type Section string

const (
	Pinned Section = "pinned"
	Active Section = "active"
	// SectionWorking is the Working shelf, named apart from the Working
	// status it holds.
	SectionWorking Section = "working"
	Snoozed        Section = "snoozed"
	Settled        Section = "settled"
)

// Sections is every section, in the order the sidebar shows them.
var Sections = []Section{Pinned, Active, SectionWorking, Snoozed, Settled}

// DefaultAutoSettleAfter is how long a thread can go without activity before
// it settles itself, unless configured otherwise.
const DefaultAutoSettleAfter = 7 * 24 * time.Hour

type ThreadOpts struct {
	// AutoSettleAfter settles threads inactive for this long. 0 never does.
	AutoSettleAfter time.Duration
}

// IsSnoozed reports whether a thread is snoozed and not due yet.
func IsSnoozed(t model.Thread, now time.Time) bool {
	return !t.SnoozedUntil.IsZero() && now.Before(t.SnoozedUntil)
}

// HasWoken reports whether a thread is back from a snooze you haven't looked
// at since: it woke early, or its time came.
func HasWoken(t model.Thread, now time.Time) bool {
	return t.Woke || (!t.SnoozedUntil.IsZero() && !now.Before(t.SnoozedUntil))
}

// LastActivity is the last time a thread did something: its status changed,
// or, for one that never reported a status, its window opened.
func LastActivity(a *model.Agent) time.Time {
	if a.Since != nil {
		return *a.Since
	}
	return a.Created
}

// SettledSince reports whether a thread is settled, and since when: the time
// you settled it, or for one settled for inactivity, its last activity.
func SettledSince(a *model.Agent, now time.Time, opts ThreadOpts) (time.Time, bool) {
	t := a.Thread
	if !t.SettledAt.IsZero() {
		return t.SettledAt, true
	}
	if opts.AutoSettleAfter <= 0 || t.Unsettled || t.Pin != 0 || IsSnoozed(t, now) || HasWoken(t, now) {
		return time.Time{}, false
	}
	switch Status(a.Status) {
	case Working, Waiting, Error:
		return time.Time{}, false
	}
	last := LastActivity(a)
	if last.IsZero() || now.Sub(last) < opts.AutoSettleAfter {
		return time.Time{}, false
	}
	return last, true
}

// SectionOf is the section a thread is in.
func SectionOf(a *model.Agent, now time.Time, opts ThreadOpts) Section {
	switch {
	case IsSnoozed(a.Thread, now):
		return Snoozed
	case isSettled(a, now, opts):
		return Settled
	case a.Thread.Pin != 0:
		return Pinned
	case Status(a.Status) == Working:
		return SectionWorking
	}
	return Active
}

func isSettled(a *model.Agent, now time.Time, opts ThreadOpts) bool {
	_, ok := SettledSince(a, now, opts)
	return ok
}

// Board is every thread, by section, each in the order the sidebar lists it.
type Board map[Section][]*model.Agent

// Arrange sorts threads into sections:
//
//	Pinned   in pin order
//	Active   the latest activity first, so a thread that just finished or
//	         started waiting on you comes to the top
//	Working  the latest prompt first
//	Snoozed  the soonest to wake first
//	Settled  the most recently settled first
func Arrange(agents []*model.Agent, now time.Time, opts ThreadOpts) Board {
	board := Board{}
	for _, a := range agents {
		s := SectionOf(a, now, opts)
		board[s] = append(board[s], a)
	}
	byTime := func(get func(*model.Agent) time.Time, newestFirst bool) func(a, b *model.Agent) int {
		return func(a, b *model.Agent) int {
			c := get(a).Compare(get(b))
			if newestFirst {
				c = -c
			}
			return cmp.Or(c, cmp.Compare(a.WindowID, b.WindowID))
		}
	}
	slices.SortStableFunc(board[Pinned], func(a, b *model.Agent) int {
		return cmp.Or(cmp.Compare(a.Thread.Pin, b.Thread.Pin), cmp.Compare(a.WindowID, b.WindowID))
	})
	slices.SortStableFunc(board[Active], byTime(LastActivity, true))
	slices.SortStableFunc(board[SectionWorking], byTime(func(a *model.Agent) time.Time {
		if a.TurnAt != nil {
			return *a.TurnAt
		}
		return LastActivity(a)
	}, true))
	slices.SortStableFunc(board[Snoozed], byTime(func(a *model.Agent) time.Time { return a.Thread.SnoozedUntil }, false))
	slices.SortStableFunc(board[Settled], byTime(func(a *model.Agent) time.Time {
		at, _ := SettledSince(a, now, opts)
		return at
	}, true))
	return board
}

// Ordered lists the threads of the given sections in sidebar order.
func (b Board) Ordered(sections ...Section) []*model.Agent {
	var all []*model.Agent
	for _, s := range sections {
		all = append(all, b[s]...)
	}
	return all
}

// TogglePin pins a thread at the bottom of the pinned ones (nextPin, larger
// than any pin in use), or unpins it. Pinning wakes and un-settles it.
func TogglePin(a *model.Agent, now time.Time, opts ThreadOpts, nextPin int64) model.Thread {
	t := a.Thread
	if t.Pin != 0 {
		t.Pin = 0
		return t
	}
	wasSettled := isSettled(a, now, opts)
	t.Pin = nextPin
	t.SettledAt = time.Time{}
	t.Unsettled = t.Unsettled || wasSettled
	t.SnoozedUntil, t.SnoozedAt, t.Woke = time.Time{}, time.Time{}, false
	return t
}

// NextPin is a pin that sorts after every pinned thread.
func NextPin(agents []*model.Agent, now time.Time) int64 {
	next := now.UnixNano()
	for _, a := range agents {
		if a.Thread.Pin >= next {
			next = a.Thread.Pin + 1
		}
	}
	return next
}

// ToggleSettle settles a thread, or takes a settled one back to the inbox.
// A thread the agent is still running in, or waiting on you in, can't be
// settled: it isn't done.
func ToggleSettle(a *model.Agent, now time.Time, opts ThreadOpts) (model.Thread, error) {
	t := a.Thread
	if isSettled(a, now, opts) {
		t.SettledAt = time.Time{}
		t.Unsettled = true
		return t, nil
	}
	switch Status(a.Status) {
	case Working:
		return t, fmt.Errorf("the agent is still working")
	case Waiting:
		return t, fmt.Errorf("the agent is waiting on you")
	}
	t.SettledAt = now
	t.Pin = 0
	t.Unsettled = false
	t.SnoozedUntil, t.SnoozedAt, t.Woke = time.Time{}, time.Time{}, false
	return t, nil
}

// SettleMerged settles a thread whose branch's pull request was merged: its
// work is done. As with settling for inactivity, a thread that is pinned,
// snoozed or back from a snooze, an agent still working, waiting on you or
// failed, and a thread you took out of Settled by hand since it last did
// something are left alone. ok is false when there is nothing to change.
func SettleMerged(a *model.Agent, now time.Time, opts ThreadOpts) (t model.Thread, ok bool) {
	t = a.Thread
	if t.Unsettled || t.Pin != 0 || IsSnoozed(t, now) || HasWoken(t, now) || isSettled(a, now, opts) {
		return t, false
	}
	switch Status(a.Status) {
	case Working, Waiting, Error:
		return t, false
	}
	t.SettledAt = now
	return t, true
}

// Snooze hides a thread until a time. A settled thread comes out of Settled,
// to return to the inbox when it wakes; a pinned one stays pinned.
func Snooze(a *model.Agent, now time.Time, until time.Time) (model.Thread, error) {
	if !until.After(now) {
		return a.Thread, fmt.Errorf("can't snooze until %s, it has passed", until.Format("Mon 15:04"))
	}
	t := a.Thread
	t.SnoozedUntil, t.SnoozedAt, t.Woke = until, now, false
	if !t.SettledAt.IsZero() {
		t.SettledAt = time.Time{}
	}
	t.Unsettled = true
	return t, nil
}

// Wake brings a snoozed thread back now, or dismisses the "woke" badge of
// one already back.
func Wake(a *model.Agent) model.Thread {
	t := a.Thread
	t.SnoozedUntil, t.SnoozedAt, t.Woke = time.Time{}, time.Time{}, false
	return t
}

// OnActivity is how a hook event moves a thread: see the package comment at
// the top of this file.
func OnActivity(t model.Thread, u Update, now time.Time) model.Thread {
	switch {
	case u.Clear:
		return model.Thread{}
	case u.StartTurn:
		t.SettledAt, t.Unsettled = time.Time{}, false
		t.SnoozedUntil, t.SnoozedAt, t.Woke = time.Time{}, time.Time{}, false
	case u.Status == Waiting || u.Status == Error:
		t.SettledAt, t.Unsettled = time.Time{}, false
		if IsSnoozed(t, now) {
			t.SnoozedUntil, t.SnoozedAt, t.Woke = time.Time{}, time.Time{}, true
		}
	}
	return t
}

// Badge is the one marker a thread row carries: its status, or that it
// came back from a snooze.
type Badge string

const BadgeWoke Badge = "woke"

// BadgeOf picks a thread's badge. What blocks the agent beats everything; a
// thread back from a snooze says so before it says it finished.
func BadgeOf(a *model.Agent, now time.Time) Badge {
	switch s := Status(a.Status); s {
	case Waiting, Error, Working:
		return Badge(s)
	}
	if HasWoken(a.Thread, now) && !IsSnoozed(a.Thread, now) {
		return BadgeWoke
	}
	return Badge(a.Status)
}

// IsUnread reports whether a thread finished, or failed, while you weren't
// looking at it, and you haven't been to its window since: like an unread
// thread in T3 Code. A finished one is "done" until then (kitty's watcher,
// contrib/ksesh_agents.py, turns it idle when you focus the window, or as it
// finishes in front of you). A failed one stays failed until the agent moves
// on, so it is unread until the watcher saw you at its window after it
// failed.
func IsUnread(a *model.Agent) bool {
	switch Status(a.Status) {
	case Done:
		return true
	case Error:
		return a.Since != nil && a.SeenAt.Before(*a.Since)
	}
	return false
}

// SnoozePreset is one of the "snooze until" choices.
type SnoozePreset struct {
	Key   string // what `ksesh agent snooze` takes
	Label string
	Until time.Time
}

// SnoozePresets are the usual choices, soonest first: in an hour, in three,
// this evening (until it is evening), tomorrow morning and next Monday.
func SnoozePresets(now time.Time) []SnoozePreset {
	at := func(day time.Time, hour int) time.Time {
		return time.Date(day.Year(), day.Month(), day.Day(), hour, 0, 0, 0, now.Location())
	}
	presets := []SnoozePreset{
		{Key: "1h", Label: "In 1 hour", Until: now.Add(time.Hour)},
		{Key: "3h", Label: "In 3 hours", Until: now.Add(3 * time.Hour)},
	}
	if evening := at(now, 18); now.Before(evening) && evening.After(now.Add(3*time.Hour)) {
		presets = append(presets, SnoozePreset{Key: "evening", Label: "This evening", Until: evening})
	}
	presets = append(presets, SnoozePreset{Key: "tomorrow", Label: "Tomorrow", Until: at(now.AddDate(0, 0, 1), 9)})
	days := (int(time.Monday) - int(now.Weekday()) + 7) % 7
	if days == 0 {
		days = 7
	}
	presets = append(presets, SnoozePreset{Key: "next-week", Label: "Next week", Until: at(now.AddDate(0, 0, days), 9)})
	return presets
}

// ParseSnooze reads when to snooze until: a preset key (1h, 3h, evening,
// tomorrow, next-week), a duration (90m, 2h30m, 2d), a time today or
// tomorrow if it has passed (15:04), or a date and time (2006-01-02 15:04).
func ParseSnooze(s string, now time.Time) (time.Time, error) {
	s = strings.TrimSpace(s)
	for _, p := range SnoozePresets(now) {
		if p.Key == s {
			return p.Until, nil
		}
	}
	switch s {
	case "evening":
		return time.Time{}, fmt.Errorf("it is evening already")
	}
	if days, ok := strings.CutSuffix(s, "d"); ok {
		if n, err := strconv.Atoi(days); err == nil && n > 0 {
			return now.AddDate(0, 0, n), nil
		}
	}
	if d, err := time.ParseDuration(s); err == nil {
		if d <= 0 {
			return time.Time{}, fmt.Errorf("snooze for a positive duration, not %s", s)
		}
		return now.Add(d), nil
	}
	if t, err := time.ParseInLocation("15:04", s, now.Location()); err == nil {
		until := time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, now.Location())
		if !until.After(now) {
			until = until.AddDate(0, 0, 1)
		}
		return until, nil
	}
	for _, layout := range []string{"2006-01-02 15:04", "2006-01-02T15:04", time.RFC3339, "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, s, now.Location()); err == nil {
			if layout == "2006-01-02" {
				t = t.Add(9 * time.Hour)
			}
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("don't know when %q is: use 1h, 3h, evening, tomorrow, next-week, a duration (90m, 2d) or a time (15:04, 2006-01-02 15:04)", s)
}
