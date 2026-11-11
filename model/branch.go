package model

// BranchStatus is where the branch checked out in a directory stands: the
// sidebar shows it under a thread.
type BranchStatus struct {
	// Branch is empty when HEAD is detached.
	Branch string `json:"branch,omitempty"`
	// Base is what Ahead and Behind count against, empty on the base branch
	// itself, and when git couldn't count them.
	Base string `json:"base,omitempty"`
	// OnBase is set on the base branch itself, which has no pull request.
	OnBase bool `json:"on_base,omitempty"`
	Ahead  int  `json:"ahead,omitempty"`
	Behind int  `json:"behind,omitempty"`
	// Worktree is the root of the linked worktree the directory is in, empty
	// in the repo's main one.
	Worktree string `json:"worktree,omitempty"`
}

// The states of a pull request, as gh reports them.
const (
	PROpen   = "OPEN"
	PRMerged = "MERGED"
	PRClosed = "CLOSED"
)

// PullRequestState is the pull request of a branch.
type PullRequestState struct {
	Number int    `json:"number"`
	State  string `json:"state"` // PROpen, PRMerged or PRClosed
	Draft  bool   `json:"draft,omitempty"`
	URL    string `json:"url,omitempty"`
}
