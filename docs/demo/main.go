// Command demo runs ksesh's TUIs on made-up agents and sessions, without
// kitty, for the README's screenshots (docs/demo/*.tape, `just screenshots`):
//
//	go run ./docs/demo sidebar   # the thread sidebar
//	go run ./docs/demo agents    # the agent picker
//	go run ./docs/demo sessions  # the session picker
//
// Nothing it shows is real, so screenshots never leak anyone's work.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/xheisenbugx/ksesh/agent"
	"github.com/xheisenbugx/ksesh/icon"
	"github.com/xheisenbugx/ksesh/kitty"
	"github.com/xheisenbugx/ksesh/model"
	"github.com/xheisenbugx/ksesh/picker"
	"github.com/xheisenbugx/ksesh/sidebar"
)

func main() {
	what := "sidebar"
	if len(os.Args) > 1 {
		what = os.Args[1]
	}
	var m tea.Model
	switch what {
	case "sidebar":
		m = sidebar.New(threadOptions(false))
	case "agents":
		m = sidebar.New(threadOptions(true))
	case "sessions":
		m = sessionPicker()
	default:
		fmt.Fprintln(os.Stderr, "usage: demo sidebar|agents|sessions")
		os.Exit(2)
	}
	if _, err := tea.NewProgram(m).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// current is the window of the thread "you're in".
const current = 11

var start = time.Now()

func ago(d time.Duration) *time.Time {
	t := start.Add(-d)
	return &t
}

// agents are the made-up threads: one in each section, and one of each
// status.
func agents() []*model.Agent {
	return []*model.Agent{
		{WindowID: 10, Title: "Refactor the auth middleware", Session: "api", Path: "/code/api", Name: "claude", Status: "idle", Since: ago(time.Hour),
			Prompt: "split the session store out of the middleware", Thread: model.Thread{Pin: 1}},
		{WindowID: 11, Title: "Fix the login redirect", Session: "api", Path: "/code/api-login", Name: "claude", Status: "waiting", Since: ago(2 * time.Minute),
			Message: "Allow Bash: npm test"},
		{WindowID: 12, Title: "Write the getting started guide", Session: "docs", Path: "/code/docs", Name: "claude", Status: "done", Since: ago(5 * time.Minute),
			Message: "Added a Quick start section"},
		{WindowID: 13, Title: "Dark mode for the dashboard", Session: "web", Path: "/code/web-dark", Name: "codex", Status: "error", Since: ago(9 * time.Minute),
			Message: "model request failed"},
		{WindowID: 14, Title: "Add rate limiting", Session: "api", Path: "/code/api-rate", Name: "claude", Status: "working", Since: ago(40 * time.Second),
			TurnAt: ago(3 * time.Minute), Prompt: "add a token bucket per API key"},
		{WindowID: 15, Title: "Upgrade to Go 1.26", Session: "api", Path: "/code/api", Name: "claude", Status: "idle", Since: ago(3 * time.Hour),
			Thread: model.Thread{SnoozedUntil: start.Add(20 * time.Hour), SnoozedAt: start.Add(-time.Hour)}},
		{Conversation: "6f0c", Closed: true, Title: "Release notes for v2.4", Session: "docs", Path: "/code/docs", Name: "claude", Status: "done", Since: ago(26 * time.Hour),
			Message: "Drafted the notes from the merged PRs"},
		{WindowID: 16, Title: "Bump dependencies", Session: "web", Path: "/code/web", Name: "claude", Status: "idle", Since: ago(50 * time.Hour),
			Thread: model.Thread{SettledAt: start.Add(-48 * time.Hour)}},
	}
}

func threadOptions(asPicker bool) sidebar.Options {
	opts := sidebar.Options{
		Backend:    &backend{agents: agents()},
		Self:       99,
		ThreadOpts: agent.ThreadOpts{AutoSettleAfter: agent.DefaultAutoSettleAfter},
		StatePath:  filepath.Join(os.TempDir(), "ksesh-demo-sidebar.json"),
		Repos:      repos{},
		Picker:     asPicker,
	}
	if asPicker {
		opts.OpenShelves = []agent.Section{agent.SectionWorking}
		opts.Preview = preview
	}
	_ = os.Remove(opts.StatePath)
	return opts
}

// backend is kitty, as far as the sidebar asks it: everything succeeds, and
// nothing changes.
type backend struct{ agents []*model.Agent }

func (b *backend) ListAgents() ([]*model.Agent, error)                   { return b.agents, nil }
func (b *backend) SetThread(int, model.Thread) error                     { return nil }
func (b *backend) SetTitle(int, string) error                            { return nil }
func (b *backend) FocusWindow(int) error                                 { return nil }
func (b *backend) FocusPlain(int) error                                  { return nil }
func (b *backend) LastWorkWindow(int) (int, error)                       { return current, nil }
func (b *backend) WindowState(int) (bool, bool, error)                   { return true, true, nil }
func (b *backend) GetCurrentSession() (string, error)                    { return "api", nil }
func (b *backend) SendKey(int, ...string) error                          { return nil }
func (b *backend) SendText(int, string, bool) error                      { return nil }
func (b *backend) CloseWindow(int) error                                 { return nil }
func (b *backend) CloseSidebars() error                                  { return nil }
func (b *backend) OverlayOn(int, []string, []string) error               { return nil }
func (b *backend) NewWindowInSession(model.KittyTabOpts) (string, error) { return "", nil }
func (b *backend) SendKeys(string, string) (string, error)               { return "", nil }
func (b *backend) OpenSidebar(int, kitty.SidebarOpts) (int, error)       { return 0, nil }
func (b *backend) Refresh()                                              {}
func (b *backend) ResumeThread(string) error                             { return nil }
func (b *backend) ForgetThread(string) error                             { return nil }

// repos are the threads' branches and pull requests.
type repos struct{}

var branches = map[string]struct {
	status model.BranchStatus
	pr     model.PullRequestState
}{
	"/code/api":       {status: model.BranchStatus{Branch: "main", OnBase: true}},
	"/code/api-login": {status: model.BranchStatus{Branch: "fix/login", Base: "origin/main", Ahead: 2}},
	"/code/api-rate":  {status: model.BranchStatus{Branch: "feat/rate-limit", Base: "origin/main", Ahead: 4, Behind: 1}},
	"/code/docs":      {status: model.BranchStatus{Branch: "docs/guide", Base: "origin/main", Ahead: 3}, pr: model.PullRequestState{Number: 42, State: model.PROpen, URL: "https://example.com"}},
	"/code/web-dark":  {status: model.BranchStatus{Branch: "feat/dark", Base: "origin/main", Ahead: 1}, pr: model.PullRequestState{Number: 38, State: model.PROpen, Draft: true, URL: "https://example.com"}},
	"/code/web":       {status: model.BranchStatus{Branch: "main", OnBase: true}},
}

func (repos) BranchStatus(dir string) (model.BranchStatus, error) {
	return branches[dir].status, nil
}

func (repos) PullRequest(dir, _ string, _ bool) (model.PullRequestState, bool, error) {
	pr := branches[dir].pr
	return pr, pr.Number != 0, nil
}

// SGR for the made-up screens.
const (
	dim    = "\033[2m"
	bold   = "\033[1m"
	reset  = "\033[0m"
	orange = "\033[38;5;209m"
	green  = "\033[32m"
	yellow = "\033[33m"
	blue   = "\033[34m"
	gray   = "\033[90m"
)

// preview is the agent picker's pane: a header like `ksesh agent show`'s,
// over a Claude Code screen asking for a permission.
func preview(a *model.Agent, diff bool, width int) (sidebar.Preview, error) {
	rule := dim + strings.Repeat("─", max(width, 1)) + reset
	if diff {
		return sidebar.Preview{Body: strings.Join([]string{
			bold + "fix/login" + reset + " → origin/main  " + green + "↑2" + reset,
			"",
			yellow + "a1f3c9e" + reset + " Keep the return URL through the login redirect",
			yellow + "77be012" + reset + " Test the redirect with a deep link",
			"",
			" src/auth/redirect.ts   " + green + "+18" + reset + " " + "\033[31m-4" + reset,
			" test/redirect.test.ts  " + green + "+41" + reset,
			"",
			rule,
			blue + "src/auth/redirect.ts" + reset,
			gray + " 12 " + reset + "  export function loginRedirect(req: Request) {",
			gray + " 13 " + reset + "\033[31m-   return '/login'" + reset,
			gray + " 13 " + reset + green + "+   const back = encodeURIComponent(req.originalUrl)" + reset,
			gray + " 14 " + reset + green + "+   return `/login?next=${back}`" + reset,
			gray + " 15 " + reset + "  }",
		}, "\n")}, nil
	}
	icon, color := "?", yellow
	if g, ok := agent.GlyphFor(agent.Status(a.Status)); ok {
		icon, color = g.Icon, fmt.Sprintf("\033[%dm", g.ColorCode)
	}
	header := strings.Join([]string{
		color + icon + " " + a.Status + reset + " · 2m · " + a.Session + " › " + agent.Title(a) + " · " + a.Name,
		dim + "~" + a.Path + "  " + branches[a.Path].status.Branch + "  2 changed" + reset,
		cmpOr(a.Message, "› "+a.Prompt),
		rule,
	}, "\n")
	body := strings.Join([]string{
		orange + "⏺" + reset + " The redirect drops the page you came from, so after logging in",
		"  everyone lands on the dashboard. I'll keep it in a " + bold + "next" + reset + " parameter.",
		"",
		orange + "⏺" + reset + " " + bold + "Update" + reset + "(src/auth/redirect.ts)",
		"  ⎿  Updated src/auth/redirect.ts with 3 additions and 1 removal",
		"",
		orange + "⏺" + reset + " Now let me run the tests.",
		"",
		"╭" + strings.Repeat("─", 50) + "╮",
		"│ " + bold + "Bash command" + reset + strings.Repeat(" ", 37) + "│",
		"│                                                  │",
		"│   npm test -- redirect                           │",
		"│   Run the redirect tests                         │",
		"│                                                  │",
		"│ Do you want to proceed?                          │",
		"│ " + blue + "❯ 1. Yes" + reset + "                                         │",
		"│   2. Yes, and don't ask again for npm test       │",
		"│   3. No, and tell Claude what to do differently  │",
		"╰" + strings.Repeat("─", 50) + "╯",
	}, "\n")
	return sidebar.Preview{Header: header, Body: body}, nil
}

func cmpOr(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// sessions are the made-up session picker's: running kitty sessions, a
// configured one, a session file and directories from zoxide.
func sessions(src string) model.SeshSessions {
	all := []model.SeshSession{
		{Src: "kitty", Name: "api", Path: "/code/api", AgentStatus: "waiting"},
		{Src: "kitty", Name: "web", Path: "/code/web", AgentStatus: "error"},
		{Src: "kitty", Name: "docs", Path: "/code/docs", AgentStatus: "done"},
		{Src: "kitty", Name: "dotfiles", Path: "~/dotfiles"},
		{Src: "config", Name: "notes", Path: "~/notes"},
		{Src: "sessionfile", Name: "monitoring", Path: "~/.config/kitty/sessions/monitoring.kitty-session"},
		{Src: "zoxide", Name: "~/code/api", Path: "/code/api"},
		{Src: "zoxide", Name: "~/code/mobile", Path: "/code/mobile"},
		{Src: "zoxide", Name: "~/code/infra/terraform", Path: "/code/infra/terraform"},
		{Src: "zoxide", Name: "~/code/design-system", Path: "/code/design-system"},
		{Src: "zoxide", Name: "~/Downloads", Path: "~/Downloads"},
	}
	out := model.SeshSessions{Directory: model.SeshSessionMap{}}
	for _, s := range all {
		if src != "" && s.Src != src {
			continue
		}
		key := s.Src + ":" + s.Name
		out.Directory[key] = s
		out.OrderedIndex = append(out.OrderedIndex, key)
	}
	return out
}

func sessionPicker() picker.Model {
	source := func(key, label, src string) picker.Source {
		return picker.Source{Key: key, Label: label, Icon: icon.Glyphs[src].Icon,
			Fetch: func() (model.SeshSessions, error) { return sessions(src), nil }}
	}
	return picker.New(func() (model.SeshSessions, error) { return sessions(""), nil }, picker.Options{
		ShowIcons:       true,
		Prompt:          "› ",
		Placeholder:     "Filter sessions...",
		Preview:         true,
		PreviewWidth:    model.DefaultPreviewWidth,
		PreviewMinWidth: model.DefaultPreviewMinWidth,
		PreviewFunc:     sessionPreview,
		Sources: []picker.Source{
			source("ctrl+a", "all", ""),
			source("ctrl+t", "kitty", "kitty"),
			source("ctrl+g", "configs", "config"),
			source("ctrl+s", "files", "sessionfile"),
			source("ctrl+z", "zoxide", "zoxide"),
		},
	})
}

// sessionPreview is a running session's screen: an editor on a file.
func sessionPreview(name string) (string, error) {
	lines := []string{
		gray + "  1 " + reset + blue + "import" + reset + " { Router } " + blue + "from" + reset + " " + green + "'express'" + reset,
		gray + "  2 " + reset + blue + "import" + reset + " { loginRedirect } " + blue + "from" + reset + " " + green + "'./auth/redirect'" + reset,
		gray + "  3 " + reset,
		gray + "  4 " + reset + blue + "export const" + reset + " routes = Router()",
		gray + "  5 " + reset,
		gray + "  6 " + reset + "routes.get(" + green + "'/account'" + reset + ", requireLogin, (req, res) => {",
		gray + "  7 " + reset + "  res.render(" + green + "'account'" + reset + ", { user: req.user })",
		gray + "  8 " + reset + "})",
		gray + "  9 " + reset,
		gray + " 10 " + reset + blue + "function" + reset + " requireLogin(req, res, next) {",
		gray + " 11 " + reset + "  " + blue + "if" + reset + " (req.user) " + blue + "return" + reset + " next()",
		gray + " 12 " + reset + "  res.redirect(loginRedirect(req))",
		gray + " 13 " + reset + "}",
		"",
		dim + " NORMAL  src/routes.ts" + strings.Repeat(" ", 20) + "12:3  " + reset,
	}
	if name != "api" {
		lines = []string{dim + "  " + name + reset}
	}
	return strings.Join(lines, "\n"), nil
}
