package model

import "time"

type ConnectOpts struct {
	Command string
	Switch  bool
}

type WorktreeConnectOpts struct {
	Number      int
	Repo        string // GitHub "org/repo" override; empty => detect from cwd
	Pr          bool   // force the pull-request path
	Switch      bool
	FromBrowser bool // resolve Number/Repo/Pr from the active browser tab URL
}

// WorktreeListOpts selects which repo's worktrees to list. Path and Repo are
// two ways to pick the same [[worktree]] block; when both are empty the repo is
// detected from the current directory.
type WorktreeListOpts struct {
	Path string // worktree root, e.g. "~/c/nu/w"
	Repo string // GitHub "org/repo"
	// Refresh refetches every title and state, ignoring how fresh the cached
	// ones are. It is how a title edited or an issue closed since the last
	// listing is picked up before the TTL is up.
	Refresh bool
}

// WorktreeEntry is one worktree directory paired with the issue it maps to.
type WorktreeEntry struct {
	Number int    // issue/PR number, taken from the directory name
	Path   string // absolute path to the worktree
	Title  string // issue title; empty when unknown or not yet fetched
	State  string // "OPEN" | "CLOSED" | "MERGED"; empty when unknown
	Icon   string
	// Created is when the worktree was added; zero when unknown.
	Created time.Time
}

// WindowConnectOpts identifies a window to connect to and what to run in it.
// Name is the window's identity: when it matches an existing window that window
// is reused, and when it is empty there is nothing to match on so a window is
// always created.
type WindowConnectOpts struct {
	Name string
	// New skips the match and always creates, for when the name is wanted as a
	// label rather than as an identity to reuse.
	New        bool
	Session    string // target session; empty => the attached session
	Path       string // start directory; empty => the target session's root
	Command    string
	Background bool // create without selecting it or moving the client
	Switch     bool // switch rather than attach, for triggers outside kitty
}

// WorktreeNewOpts describes a branch worktree for `ksesh worktree new`.
type WorktreeNewOpts struct {
	Branch string
	Base   string // ref a new branch starts from; empty picks the default
	Fetch  bool   // fetch before creating, even without a [[worktree]] block
	// StartAgent starts an agent in the session: Agent, or the repo's
	// agent_command when empty, given Prompt as its first prompt.
	StartAgent bool
	Agent      string
	Prompt     string
	Switch     bool
}

// GitWorktree is one entry of `git worktree list`, with its state when asked
// for.
type GitWorktree struct {
	Path     string `json:"path"`
	Branch   string `json:"branch,omitempty"` // short name; empty when detached
	Head     string `json:"head"`
	Main     bool   `json:"main"`
	Detached bool   `json:"detached"`
	Repo     string `json:"repo"` // the main worktree, which git commands run in
	// Base is what Ahead and Behind count against.
	Base      string `json:"base,omitempty"`
	Ahead     int    `json:"ahead"`
	Behind    int    `json:"behind"`
	Staged    int    `json:"staged"`
	Unstaged  int    `json:"unstaged"`
	Untracked int    `json:"untracked"`
	// Reason is why `worktree rm` picked it: "merged", "gone" or "".
	Reason string `json:"reason,omitempty"`
}

// WorktreeRemoveOpts selects the worktrees `ksesh worktree rm` removes.
type WorktreeRemoveOpts struct {
	// Targets are worktree paths, directory names or branches.
	Targets []string
	Merged  bool // every worktree whose branch is merged into the base
	Gone    bool // every worktree whose branch's upstream was deleted
}
