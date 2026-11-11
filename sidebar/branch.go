package sidebar

import (
	"strconv"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/xheisenbugx/ksesh/agent"
	ansicolor "github.com/xheisenbugx/ksesh/ansi"
	"github.com/xheisenbugx/ksesh/model"
	"github.com/xheisenbugx/ksesh/ui"
)

// Repos tells the sidebar about the branch a thread is on. Both can be slow
// (git, and gh over the network): the sidebar asks off its UI loop, and only
// as often as gitTTL and prTTL let it.
type Repos interface {
	// BranchStatus is where dir's branch stands against its base.
	BranchStatus(dir string) (model.BranchStatus, error)
	// PullRequest is the pull request of branch, checked out in dir. fresh
	// asks GitHub even when a recent answer is cached.
	PullRequest(dir, branch string, fresh bool) (model.PullRequestState, bool, error)
}

// How long what the sidebar knows of a thread's branch is good for: git is
// local and cheap, a pull request is a request to GitHub.
const (
	gitTTL = 30 * time.Second
	prTTL  = 5 * time.Minute
	// prMissTTL is how soon a branch without a pull request is asked about
	// again: an agent may open one any time.
	prMissTTL = time.Minute
)

// branchInfo is what the sidebar knows of the branch checked out in a
// directory.
type branchInfo struct {
	status model.BranchStatus
	ok     bool // the directory is in a repo
	pr     model.PullRequestState
	hasPR  bool
	// prBranch is the branch pr and hasPR are about: once another branch is
	// checked out, they say nothing of it.
	prBranch string
	// prErr is why the last look for its pull request failed.
	prErr    error
	statusAt time.Time
	prAt     time.Time
	busy     bool // being asked about now
}

// branchMsg is the answer about one directory.
type branchMsg struct {
	dir               string
	at                time.Time
	status            model.BranchStatus
	ok, checkedStatus bool
	pr                model.PullRequestState
	hasPR, checkedPR  bool
	prErr             error
}

// refreshBranches asks about the branches of every thread whose answer is
// out of date, one command per directory, while the sidebar is on screen.
func (m Model) refreshBranches() []tea.Cmd {
	repos := m.opts.Repos
	if repos == nil || !m.shown {
		return nil
	}
	now := m.opts.Now()
	var cmds []tea.Cmd
	for _, a := range m.agents {
		dir := a.Path
		if dir == "" {
			continue
		}
		info := m.branches[dir]
		if info == nil {
			info = &branchInfo{}
			m.branches[dir] = info
		}
		ttl := prTTL
		if _, ok := info.knownPR(); !ok {
			ttl = prMissTTL
		}
		wantStatus, wantPR := now.Sub(info.statusAt) >= gitTTL, now.Sub(info.prAt) >= ttl
		if info.busy || (!wantStatus && !wantPR) {
			continue
		}
		info.busy = true
		known, knownOK, clock := info.status, info.ok, m.opts.Now
		cmds = append(cmds, func() tea.Msg {
			msg := branchMsg{dir: dir, status: known, ok: knownOK}
			if wantStatus {
				st, err := repos.BranchStatus(dir)
				msg.status, msg.ok, msg.checkedStatus = st, err == nil, true
				// Another branch was checked out: what is known of the last
				// one's pull request isn't this one's.
				wantPR = wantPR || st.Branch != known.Branch
			}
			if wantPR {
				msg.checkedPR = true
				if msg.ownBranch() {
					msg.pr, msg.hasPR, msg.prErr = repos.PullRequest(dir, msg.status.Branch, false)
				}
			}
			msg.at = clock()
			return msg
		})
	}
	return cmds
}

// ownBranch reports whether the answer is about a branch that can have a
// pull request: not the base branch, nor a detached HEAD.
func (msg branchMsg) ownBranch() bool {
	return msg.ok && msg.status.Branch != "" && !msg.status.OnBase
}

// knownPR is the pull request of the branch checked out now, when known.
func (info *branchInfo) knownPR() (model.PullRequestState, bool) {
	if info == nil || !info.hasPR || info.prBranch != info.status.Branch {
		return model.PullRequestState{}, false
	}
	return info.pr, true
}

// learn takes in an answer about a directory's branch.
func (m *Model) learn(msg branchMsg) {
	info := m.branches[msg.dir]
	if info == nil {
		info = &branchInfo{}
		m.branches[msg.dir] = info
	}
	info.busy = false
	if msg.checkedStatus {
		info.status, info.ok, info.statusAt = msg.status, msg.ok, msg.at
	}
	if msg.checkedPR {
		// A failed request keeps what was known, and waits its turn to ask
		// again.
		info.prErr = msg.prErr
		if msg.prErr == nil {
			info.pr, info.hasPR, info.prBranch = msg.pr, msg.hasPR, msg.status.Branch
		}
		info.prAt = msg.at
	}
}

// settleAgain is how long the sidebar leaves a thread it settled for its
// merged pull request before it would do it again: the time it takes kitty
// to show the change.
const settleAgain = 30 * time.Second

// settleMerged settles the threads whose branch's pull request was merged,
// on agent.SettleMerged's terms, writing it to their windows.
func (m *Model) settleMerged() []tea.Cmd {
	var cmds []tea.Cmd
	now := m.opts.Now()
	for _, a := range m.agents {
		info, ok := m.branchOf(a)
		if !ok {
			continue
		}
		if pr, ok := info.knownPR(); !ok || pr.State != model.PRMerged {
			continue
		}
		if at, ok := m.autoSettled[a.WindowID]; ok && now.Sub(at) < settleAgain {
			continue
		}
		t, ok := agent.SettleMerged(a, now, m.opts.ThreadOpts)
		if !ok {
			continue
		}
		m.autoSettled[a.WindowID] = now
		a.Thread = t
		id := a.WindowID
		cmds = append(cmds, m.do("settled "+title(a)+": its pull request was merged", nil, func(b Backend) error { return b.SetThread(id, t) }))
	}
	if len(cmds) > 0 {
		m.keepCursor()
	}
	return cmds
}

// branchOf is what is known of the branch a thread is on.
func (m Model) branchOf(a *model.Agent) (*branchInfo, bool) {
	info := m.branches[a.Path]
	if info == nil || !info.ok {
		return nil, false
	}
	return info, true
}

// branchMax is the most of a branch name the subtitle shows.
const branchMax = 16

// PR icons: octicons from a nerd font.
const (
	iconPR       = "" // git pull request
	iconPRMerged = "" // git merge
	iconPRClosed = "" // pull request closed
	iconPRDraft  = "" // pull request draft
)

// branchLabel is a thread's branch, for the end of its second line, when it
// is on one of its own: its name, the commits it is ahead (↑) and behind (↓)
// its base, and its pull request, colored by state.
//
//	feat/login ↑2↓1  #12
func (m Model) branchLabel(a *model.Agent) string {
	info, ok := m.branchOf(a)
	pr, hasPR := info.knownPR()
	if !ok || info.status.Branch == "" || info.status.OnBase || (info.status.Base == "" && !hasPR) {
		return ""
	}
	st := info.status
	out := m.th.Mute(ansi.Truncate(st.Branch, branchMax, "…"))
	counts := ""
	if st.Ahead > 0 {
		counts += "↑" + strconv.Itoa(st.Ahead)
	}
	if st.Behind > 0 {
		counts += "↓" + strconv.Itoa(st.Behind)
	}
	if counts != "" {
		out += " " + m.th.Mute(counts)
	}
	if hasPR {
		icon, color := prIcon(pr)
		label := icon + " #" + strconv.Itoa(pr.Number)
		if color == 0 {
			out += " " + m.th.Mute(label)
		} else {
			out += " " + ui.Fg(color, label)
		}
	}
	return out
}

// prIcon is a pull request's icon and color: green open, muted draft,
// magenta merged, red closed.
func prIcon(pr model.PullRequestState) (string, int) {
	switch {
	case pr.State == model.PRMerged:
		return iconPRMerged, ansicolor.Magenta
	case pr.State == model.PRClosed:
		return iconPRClosed, ansicolor.Red
	case pr.Draft:
		return iconPRDraft, 0
	}
	return iconPR, ansicolor.Green
}
