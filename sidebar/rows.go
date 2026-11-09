package sidebar

import (
	"slices"

	"github.com/xheisenbugx/ksesh/agent"
	"github.com/xheisenbugx/ksesh/model"
)

type rowKind int

const (
	rowThread rowKind = iota
	// rowHeader is a section's title; for a shelf, the line that opens and
	// closes it.
	rowHeader
	// rowMore stands for the settled threads past the first settledPage.
	rowMore
	// rowEmpty is the inbox with nothing in it.
	rowEmpty
	// rowFill takes up whatever room is left, pushing the shelves to the
	// bottom of the sidebar.
	rowFill
	// rowRule is the line over the shelves.
	rowRule
	// rowGroup names the session of the inbox threads under it, when the
	// inbox is grouped by session.
	rowGroup
)

type row struct {
	kind    rowKind
	section agent.Section
	agent   *model.Agent
	count   int  // a header's number of threads, rowMore's hidden ones
	none    bool // rowEmpty: no agents at all, rather than none needing you
	// holdsCurrent marks a closed shelf's header when the thread you're on
	// is folded away in it.
	holdsCurrent bool
	// session is a rowGroup's.
	session string
}

func (r row) selectable() bool {
	return r.kind == rowThread || r.kind == rowHeader && isShelf(r.section) || r.kind == rowMore
}

// height is how many lines a row takes; rowFill's is decided when drawing.
func (r row) height() int {
	switch r.kind {
	case rowThread:
		return 3 // and a blank line
	case rowEmpty:
		return 4
	case rowFill:
		return 0
	}
	return 1
}

// The shelves are the collapsible sections at the bottom.
var shelves = []agent.Section{agent.SectionWorking, agent.Snoozed, agent.Settled}

func isShelf(s agent.Section) bool {
	return s == agent.SectionWorking || s == agent.Snoozed || s == agent.Settled
}

// settledPage is how many settled threads an open Settled shelf shows before
// a "more" row, like T3 Code's.
const settledPage = 10

// layout lays the board out as the sidebar's rows, top to bottom:
//
//	PINNED     its threads, when there are some
//	INBOX      the active threads; it only needs a title under Pinned
//	(fill)     the room left, so the shelves sit at the bottom
//	─────
//	▸ Working  the shelves, closed unless expanded. A closed shelf holding
//	▸ Snoozed  the thread you are on says so on its header, so you can tell
//	▸ Settled  where it went
func layout(board agent.Board, expanded map[agent.Section]bool, allSettled bool, currentID int) []row {
	var rows []row
	pinned, inbox := board[agent.Pinned], board[agent.Active]
	if len(pinned) > 0 {
		rows = append(rows, row{kind: rowHeader, section: agent.Pinned, count: len(pinned)})
		for _, a := range pinned {
			rows = append(rows, row{kind: rowThread, section: agent.Pinned, agent: a})
		}
		if len(inbox) > 0 {
			rows = append(rows, row{kind: rowHeader, section: agent.Active, count: len(inbox)})
		}
	}
	for _, a := range inbox {
		rows = append(rows, row{kind: rowThread, section: agent.Active, agent: a})
	}
	if len(inbox) == 0 && len(pinned) == 0 {
		rows = append(rows, row{kind: rowEmpty, none: len(board.Ordered(agent.Sections...)) == 0})
	}

	var tray []row
	for _, s := range shelves {
		threads := board[s]
		if len(threads) == 0 {
			continue
		}
		header := row{kind: rowHeader, section: s, count: len(threads)}
		if !expanded[s] {
			for _, a := range threads {
				header.holdsCurrent = header.holdsCurrent || a.WindowID == currentID
			}
			tray = append(tray, header)
			continue
		}
		tray = append(tray, header)
		shown := threads
		if s == agent.Settled && !allSettled && len(threads) > settledPage {
			shown = threads[:settledPage]
		}
		for _, a := range shown {
			tray = append(tray, row{kind: rowThread, section: s, agent: a})
		}
		if hidden := len(threads) - len(shown); hidden > 0 {
			tray = append(tray, row{kind: rowMore, section: s, count: hidden})
		}
	}
	if len(tray) > 0 {
		rows = append(rows, row{kind: rowFill}, row{kind: rowRule})
		rows = append(rows, tray...)
	}
	return rows
}

// groupInbox groups the inbox's threads by session, under a row naming each,
// the way T3 Code groups threads by project. The session with the latest
// activity comes first, and each keeps its threads in inbox order.
//
//	INBOX ─────────────────── 3
//	 api ──────────────────── 2
//	  …its threads
//	 docs ─────────────────── 1
func groupInbox(rows []row) []row {
	start := slices.IndexFunc(rows, func(r row) bool { return r.kind == rowThread && r.section == agent.Active })
	if start < 0 {
		return rows
	}
	end := start
	for end < len(rows) && rows[end].kind == rowThread && rows[end].section == agent.Active {
		end++
	}
	var order []string
	bySession := map[string][]row{}
	for _, r := range rows[start:end] {
		s := r.agent.Session
		if _, ok := bySession[s]; !ok {
			order = append(order, s)
		}
		bySession[s] = append(bySession[s], r)
	}
	grouped := slices.Clone(rows[:start])
	for _, s := range order {
		grouped = append(grouped, row{kind: rowGroup, section: agent.Active, session: s, count: len(bySession[s])})
		grouped = append(grouped, bySession[s]...)
	}
	return append(grouped, rows[end:]...)
}
